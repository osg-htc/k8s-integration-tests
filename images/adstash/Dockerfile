# Test-runner image for the adstash test suite: HTCondor CLI tools and python
# bindings, plus pytest and the elasticsearch-py/opensearch-py client libraries
# used by the tests in test-configs/adstash/tests/. SE_PY_VERSION
# select which client library versions to install.
FROM htcondor/mini:latest

ARG SE_PY_VERSION="elasticsearch>=7,<8"

# Upgrade condor_adstash to a build that supports --init_index.
RUN dnf -y --enablerepo=htcondor-snapshot upgrade condor \
    && dnf clean all

RUN pip3 install --no-cache-dir \
    "pytest" \
    "${SE_PY_VERSION}"

# TODO AdStash currently has a hard requirement on ElasticSearch, install it no matter what so imports don't fail
RUN if [[ "${SE_PY_VERSION}" == *opensearch* ]]; then \
        pip install --no-cache-dir 'elasticsearch>=7,<8'; \
    fi

# submituser is a regular user account provided by htcondor/mini, used here
# so that jobs submitted by the test suite aren't owned by root.
USER submituser
CMD ["sleep", "infinity"]
