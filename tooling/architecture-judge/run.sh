#!/bin/sh
set -e
. /tooling/lib/ledger/run.sh
cd /workspace

# 1. Run sub-judges (assuming they exist in tooling/)
# For this test, we simulate/mock the scores if the tools aren't present
# In production, these are real binaries within the container
SECURITY_SCORE=$(./tooling/security/run.sh | jq -r '.fitness')
COMPLEXITY_SCORE=$(./tooling/complexity/run.sh | jq -r '.fitness')

# 2. Aggregation (simple average for this example)
# Using awk for floating point math since bc might not be installed
TOTAL_FITNESS=$(awk -v s=$SECURITY_SCORE -v c=$COMPLEXITY_SCORE 'BEGIN { print (s * 0.5) + (c * 0.5) }')

# 3. Emit Contract
if [ "$(awk -v f=$TOTAL_FITNESS 'BEGIN { print (f >= 0.9) }')" -eq 1 ]; then
    echo "{\"status\": \"succeeded\", \"fitness\": $TOTAL_FITNESS}" > /workspace/result.json
    ledger_append "architecture-judge" "Passed (Score: $TOTAL_FITNESS)"
else
    echo "{\"status\": \"failed\", \"fitness\": $TOTAL_FITNESS, \"reason\": \"Architecture failed fitness threshold\"}" > /workspace/result.json
    ledger_append "architecture-judge" "Failed (Score: $TOTAL_FITNESS)"
    exit 1
fi
