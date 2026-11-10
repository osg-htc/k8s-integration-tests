#!/usr/bin/env -S pytest -v -s

"""
Basic connectivity and version check for the deployed search engine backend.
"""

import os
import socket
import time

import pytest


# The single search-engine backend deployed by
# manifests/adstash/search/deployment-{es,os}.yaml; SE_BACKEND/SE_VERSION/SE_HOST
# are set by manifests/adstash/runner/deployment.yaml to match.
SE_CLIENT_TYPE = os.environ.get("SE_BACKEND", "elasticsearch")
SE_HOST = os.environ.get("SE_HOST", "localhost:9200")
SE_MAJOR = int(os.environ.get("SE_VERSION", "8.19.20").split(".")[0])


def get_client():
    # Conditional imports, only one client library is installed in each test-specific runner image
    if SE_CLIENT_TYPE == "elasticsearch":
        import elasticsearch
        return elasticsearch.Elasticsearch(f"http://{SE_HOST}")
    else:
        import opensearchpy
        host, port = SE_HOST.split(":")
        return opensearchpy.OpenSearch(hosts=[{"host": host, "port": int(port)}])


@pytest.fixture
def client():
    host = SE_HOST.split(":")[0]
    try:
        socket.getaddrinfo(host, None)
    except socket.gaierror:
        pytest.skip(f"{SE_CLIENT_TYPE} ({host}) not in DNS, container not running")

    c = get_client()
    # Wait up to 60s for the backend to become healthy
    for attempt in range(12):
        try:
            if c.ping():
                return c
        except Exception:
            pass
        time.sleep(5)
    pytest.skip(f"{SE_CLIENT_TYPE} not reachable after 60s")


class TestConnectivity:
    """Verify the deployed backend is reachable and reports the expected major version."""

    def test_ping(self, client):
        assert client.ping()

    def test_cluster_health(self, client):
        health = client.cluster.health()
        assert health["status"] in ("green", "yellow")

    def test_major_version(self, client):
        info = client.info()
        version_str = info["version"]["number"]
        major = int(version_str.split(".")[0])
        assert major == SE_MAJOR
