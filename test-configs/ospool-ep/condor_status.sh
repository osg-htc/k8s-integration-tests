#!/bin/bash
# Sanity check: condor_status the test CM for an ad from the ospool-ep
OUTPUT=$(condor_status -const 'regexp("ospool-ep",Machine)')
# Confirm that there is non-empty output
[ -n "$OUTPUT" ]
