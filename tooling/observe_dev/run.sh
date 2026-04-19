#!/bin/sh
# Observe Dev Stage: Observe dev environment state
# Uses the LBSM architecture

# Source ledger library
. "$(dirname "$0")/../lib/ledger/run.sh"

# Define paths
WORKSPACE_DIR="/workspace"
CONTEXT_FILE="$WORKSPACE_DIR/context.json"
RESULT_FILE="$WORKSPACE_DIR/result.json"

# Robust Error Handling
fail() {
    echo "{\"status\": \"failed\", \"summary\": \"$1\", \"outputs\": {}, \"next_signals\": []}" > "$RESULT_FILE"
    ledger_append "observe_dev" "$(jq -n --arg status "failed" --arg summary "$1" '{type: "observe_dev_record", status: $status, summary: $summary}')"
    exit 1
}

# 1. Verify context exists
if [ ! -f "$CONTEXT_FILE" ]; then
    fail "context.json not found"
fi

# 2. Check Ledger
if [ ! -f "/workspace/ledger.log" ]; then
    fail "ledger.log not found"
fi

# 3. Adjudication Logic
LEDGER_CONTENT=$(ledger_read)
PROMOTE_STATUS=$(echo "$LEDGER_CONTENT" | grep '^promote_dev ' | tail -n 1 | cut -d' ' -f2- | jq -r '.status')

if [ "$PROMOTE_STATUS" != "succeeded" ]; then
    fail "promote_dev stage did not succeed (Status: $PROMOTE_STATUS). Cannot proceed."
fi

# 4. Perform Observation (Dummy)
echo "Observing dev environment..."
# In a real scenario, this would perform actual observations/checks.
# For now, it just succeeds.

# 5. Record to Ledger
LEDGER_MSG=$(jq -n --arg status "succeeded" --arg summary "Observation of dev successful" '{type: "observe_dev_record", status: $status, summary: $summary}')
ledger_append "observe_dev" "$LEDGER_MSG"

# 6. Emit result
# Ensure it strictly follows internal/contracts/schemas/stage-result.schema.json
jq -n \
  --arg status "succeeded" \
  --arg summary "Observation of dev successful" \
  --argjson outputs '{}' \
  '{status: $status, summary: $summary, outputs: $outputs, next_signals: []}' \
  > "$RESULT_FILE"

exit 0
