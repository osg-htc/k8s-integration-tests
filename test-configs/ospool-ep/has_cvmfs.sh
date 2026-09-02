#!/bin/bash
OUTPUT=$(condor_status -const 'regexp("ospool-ep",Machine)' -af HAS_CVMFS_singularity_opensciencegrid_org)

[[ "${OUTPUT,,}" == "true" ]]
