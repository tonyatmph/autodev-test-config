#!/bin/sh
set -e
. /tooling/lib/ledger/run.sh
cd /workspace

# Read the Intent (Goal)
if [ -f "intent.json" ]; then
    INTENT=$(cat intent.json)
    echo "Researching: $INTENT"
else
    INTENT="General Exploration"
fi

# Simulate the research (e.g., probe an idea)
echo "Discovery: Evidence for $INTENT" > discovery.txt
git add discovery.txt
git commit -m "research: discovery committed"

NEW_SHA=$(git rev-parse HEAD)
ledger_append "research" "Discovered: $NEW_SHA"

# Emit the result (Contract)
echo '{"status": "succeeded", "new_sha": "'$NEW_SHA'", "fitness": 1.0, "discovery": "found"}' > /workspace/result.json
