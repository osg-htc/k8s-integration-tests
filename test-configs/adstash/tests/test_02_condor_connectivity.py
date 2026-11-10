#!/usr/bin/env -S pytest -v -s

"""
Basic connectivity check for the HTCondor mini container.
"""

import socket
import time

import pytest
import htcondor2 as htcondor


CONDOR_HOST = "condor"


@pytest.fixture(scope="module")
def schedd():
    try:
        socket.getaddrinfo(CONDOR_HOST, None)
    except socket.gaierror:
        pytest.skip(f"{CONDOR_HOST} not in DNS, container not running")

    # Wait up to 60s for the schedd to be reachable
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


class TestCondorConnectivity:

    def test_schedd_query(self, schedd):
        jobs = schedd.query()
        assert isinstance(jobs, list)
