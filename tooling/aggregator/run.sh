#!/bin/sh
set -e
# Aggregator Factory:
# Input: Results from dependency providers
# Output: Final Consensus SHA

. /tooling/lib/ledger/run.sh

cd /workspace

# In reality, the orchestrator mounts all input artifacts in /workspace
# The aggregator inspects these, calculates the consensus/fitness, 
# and produces the final aggregated SHA.

echo "Calculating consensus..."
# Mock consensus logic
echo '{"status": "succeeded", "fitness": 1.0, "consensus": "approved"}' > /workspace/result.json

NEW_SHA=$(git rev-parse HEAD)
ledger_append "aggregator" "Consensus reached for SHA: $NEW_SHA"
