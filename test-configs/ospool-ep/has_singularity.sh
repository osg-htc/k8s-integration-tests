#!/bin/bash
OUTPUT=$(condor_status -const 'regexp("ospool-ep",Machine)' -af HAS_SINGULARITY)

[[ "${OUTPUT,,}" == "true" ]]
