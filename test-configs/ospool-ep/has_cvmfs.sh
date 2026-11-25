#!/bin/bash
OUTPUT=$(condor_status -const 'regexp("ospool-ep",Machine)' -af SINGULARITY_CAN_USE_CVMFS)

[[ "${OUTPUT,,}" == "true" ]]
