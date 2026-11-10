#!/usr/bin/env -S pytest -v -s

"""
Integration tests for condor_adstash.
Submits a job to a live HTCondor schedd, runs condor_adstash to push
history to the deployed search engine backend, then verifies docs landed.
"""

import json
import os
import socket
import subprocess
import time

import pytest
import htcondor2 as htcondor


CONDOR_HOST = "condor"

# The single search-engine backend deployed by
# manifests/adstash/search/deployment-{es,os}.yaml; SE_BACKEND/SE_HOST are set
# by manifests/adstash/runner/deployment.yaml to match.
SE_CLIENT_TYPE = os.environ.get("SE_BACKEND", "elasticsearch")
SE_HOST = os.environ.get("SE_HOST", "localhost:9200")
SE_INTERFACE = SE_CLIENT_TYPE
INDEX_NAME = f"adstash-test-{SE_CLIENT_TYPE}"


def get_se_client():
    # Conditional imports, only one client library is installed in each test-specific runner image
    if SE_CLIENT_TYPE == "elasticsearch":
        import elasticsearch
        return elasticsearch.Elasticsearch(f"http://{SE_HOST}", timeout=120)
    else:
        import opensearchpy
        host, port = SE_HOST.split(":")
        return opensearchpy.OpenSearch(hosts=[{"host": host, "port": int(port)}], timeout=120)


@pytest.fixture(scope="module")
def schedd():
    try:
        socket.getaddrinfo(CONDOR_HOST, None)
    except socket.gaierror:
        pytest.skip(f"{CONDOR_HOST} not in DNS, container not running")

    for attempt in range(12):
        try:
            collector = htcondor.Collector(CONDOR_HOST)
            schedd_ads = collector.query(htcondor.AdTypes.Schedd)
            if schedd_ads:
                return htcondor.Schedd(schedd_ads[0])
        except Exception:
            pass
        time.sleep(5)
    pytest.skip(f"Schedd not reachable after 60s")


@pytest.fixture(scope="module")
def completed_job(schedd):
    """Submit a short job and wait for it to complete."""
    submit = htcondor.Submit({
        "executable": "/bin/sleep",
        "arguments": "1",
        "transfer_executable": "false",
        "initialdir": "/home/submituser",
        "requirements": "!isUndefined(TARGET.Arch)",
        "log": "/tmp/test_job.log",
    })
    result = schedd.submit(submit, count=1)
    cluster_id = result.cluster()
    print(f"\nSubmitted job {cluster_id}.0")

    # Poll until the job leaves the queue
    for attempt in range(60):
        jobs = schedd.query(
            constraint=f"ClusterId == {cluster_id}",
            projection=["ClusterId", "ProcId", "JobStatus"],
        )
        if not jobs:
            print(f"Job {cluster_id}.0 has left the queue")
            break
        status = jobs[0]["JobStatus"]
        print(f"Job {cluster_id}.0 status: {status}")
        time.sleep(2)
    else:
        pytest.fail(f"Job {cluster_id}.0 did not complete within 120s")

    return cluster_id


class TestJobHistory:
    """Verify the completed job appears in schedd history."""

    def test_job_in_history(self, schedd, completed_job):
        history = list(schedd.history(
            constraint=f"ClusterId == {completed_job}",
            projection=["ClusterId", "ProcId", "JobStatus"],
            match=1,
        ))
        assert len(history) == 1
        assert history[0]["ClusterId"] == completed_job
        assert history[0]["JobStatus"] == 4  # Completed


@pytest.fixture
def se_client():
    host = SE_HOST.split(":")[0]
    try:
        socket.getaddrinfo(host, None)
    except socket.gaierror:
        pytest.skip(f"{SE_CLIENT_TYPE} ({host}) not in DNS, container not running")
    c = get_se_client()
    for attempt in range(12):
        try:
            if c.ping():
                return c
        except Exception:
            pass
        time.sleep(5)
    pytest.skip(f"{SE_CLIENT_TYPE} not reachable after 60s")


def init_and_create_index(se_client, init_dir):
    """Run condor_adstash --init_index, then push the generated JSON to the SE backend."""

    # Single-node clusters need 0 replicas
    custom_settings_path = os.path.join(init_dir, "custom_settings.json")
    os.makedirs(init_dir, exist_ok=True)
    with open(custom_settings_path, "w") as f:
        json.dump({"index": {"number_of_replicas": 0}}, f)

    # Generate index setup files (no ILM for testing)
    result = subprocess.run(
        [
            "condor_adstash",
            "--se_index_name", INDEX_NAME,
            "--init_index",
            "--custom_index_settings", custom_settings_path,
            "--init_output_directory", init_dir,
        ],
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert result.returncode == 0, f"init_index failed: {result.stdout}\n{result.stderr}"

    # Push ILM policy (ES only; OpenSearch uses ISM)
    ilm_path = os.path.join(init_dir, f"{INDEX_NAME}-ilm.json")
    if os.path.exists(ilm_path) and SE_CLIENT_TYPE == "elasticsearch":
        with open(ilm_path) as f:
            ilm_body = json.load(f)
        try:
            se_client.ilm.put_lifecycle(policy=f"{INDEX_NAME}-ilm", body=ilm_body)
        except (ValueError, TypeError):
            se_client.ilm.put_lifecycle(name=f"{INDEX_NAME}-ilm", body=ilm_body)

    # Push index template (inject 0 replicas for single-node test cluster)
    template_path = os.path.join(init_dir, f"{INDEX_NAME}-template.json")
    if os.path.exists(template_path):
        with open(template_path) as f:
            template_body = json.load(f)
        template_body["template"]["settings"]["index.number_of_replicas"] = 0
        if SE_CLIENT_TYPE == "opensearch":
            # OpenSearch doesn't support ES ILM settings
            for key in list(template_body["template"]["settings"].keys()):
                if key.startswith("index.lifecycle"):
                    del template_body["template"]["settings"][key]
            # Use legacy template format for OpenSearch
            legacy_body = {
                "index_patterns": template_body["index_patterns"],
                "settings": template_body["template"]["settings"],
                "mappings": template_body["template"]["mappings"],
            }
            se_client.indices.put_template(name=f"{INDEX_NAME}-template", body=legacy_body)
        else:
            se_client.indices.put_index_template(name=f"{INDEX_NAME}-template", body=template_body)

    # Create the initial index (inject 0 replicas for single-node test cluster)
    index_path = os.path.join(init_dir, f"{INDEX_NAME}-000001.json")
    with open(index_path) as f:
        index_body = json.load(f)
    index_body.setdefault("settings", {})["index.number_of_replicas"] = 0
    # OpenSearch legacy templates apply mappings automatically on create;
    # having them in both the template and the create body causes errors
    if SE_CLIENT_TYPE == "opensearch":
        index_body.pop("mappings", None)
    se_client.indices.create(index=f"{INDEX_NAME}-000001", body=index_body)


class TestAdstashPush:
    """Run condor_adstash to push schedd history to the deployed SE backend."""

    def test_init_index(self, se_client, tmp_path):
        try:
            init_and_create_index(se_client, str(tmp_path / SE_CLIENT_TYPE))
        except Exception as e:
            print(f"\ninit_and_create_index failed: {e.__class__.__name__}: {e}")
            raise

        # Verify the index exists (via the alias)
        assert se_client.indices.exists(index=INDEX_NAME)

    def test_adstash_push(self, completed_job, se_client):
        result = subprocess.run(
            [
                "condor_adstash",
                "--standalone",
                "--schedd_history",
                "--interface", SE_INTERFACE,
                "--se_host", SE_HOST,
                "--se_index_name", INDEX_NAME,
                "--log_level", "DEBUG",
                "--log_file", f"/tmp/adstash_{SE_CLIENT_TYPE}.log",
                "--checkpoint_file", f"/tmp/adstash_{SE_CLIENT_TYPE}_checkpoint.json",
            ],
            capture_output=True,
            text=True,
            timeout=120,
        )
        print(f"\n--- condor_adstash stdout ({SE_CLIENT_TYPE}) ---\n{result.stdout}")
        print(f"--- condor_adstash stderr ({SE_CLIENT_TYPE}) ---\n{result.stderr}")
        assert result.returncode == 0, f"condor_adstash failed: {result.stderr}"

    def test_docs_landed(self, completed_job, se_client):
        se_client.indices.refresh(index=INDEX_NAME)
        result = se_client.search(index=INDEX_NAME, body={"query": {"match_all": {}}})

        hits = result["hits"]["hits"]
        assert len(hits) > 0, f"No docs found in {INDEX_NAME}"
        print(f"\n{len(hits)} doc(s) in {INDEX_NAME}")

        doc = hits[0]["_source"]
        assert doc.get("ClusterId") == completed_job
        assert doc.get("JobStatus") == 4
        assert doc.get("Status") == "Completed"
        assert "ScheddName" in doc
        assert "RecordTime" in doc
        assert "@timestamp" in doc

    def test_cleanup_index(self, se_client):
        """Clean up the test index and template."""
        se_client.indices.delete(index=f"{INDEX_NAME}-*")
        try:
            if SE_CLIENT_TYPE == "opensearch":
                se_client.indices.delete_template(name=f"{INDEX_NAME}-template")
            else:
                se_client.indices.delete_index_template(name=f"{INDEX_NAME}-template")
        except Exception:
            pass
        if SE_CLIENT_TYPE == "elasticsearch":
            try:
                se_client.ilm.delete_lifecycle(policy=f"{INDEX_NAME}-ilm")
            except Exception:
                pass
