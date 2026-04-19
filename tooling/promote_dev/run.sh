#!/bin/sh
# Promote Dev Stage: Promote to dev environment
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
    ledger_append "promote_dev" "$(jq -n --arg status "failed" --arg summary "$1" '{type: "promote_dev_record", status: $status, summary: $summary}')"
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
PREPARE_STATUS=$(echo "$LEDGER_CONTENT" | grep '^release_prepare ' | tail -n 1 | cut -d' ' -f2- | jq -r '.status')

if [ "$PREPARE_STATUS" != "succeeded" ]; then
    fail "release_prepare stage did not succeed (Status: $PREPARE_STATUS). Cannot proceed."
fi

# 4. Dummy GitOps Promotion
# Perform dummy file mutation and git commit as a placeholder
echo "Promoting to dev..."
# Assuming workspace is a git repository or linked to one
touch /workspace/dev-promotion.txt
# git add /workspace/dev-promotion.txt || true
# git commit -m "chore: promote to dev" || true

# 5. Record to Ledger
LEDGER_MSG=$(jq -n --arg status "succeeded" --arg summary "Promotion to dev successful" '{type: "promote_dev_record", status: $status, summary: $summary}')
ledger_append "promote_dev" "$LEDGER_MSG"

# 6. Emit result
# Ensure it strictly follows internal/contracts/schemas/stage-result.schema.json
jq -n \
  --arg status "succeeded" \
  --arg summary "Promotion to dev successful" \
  --argjson outputs '{}' \
  '{status: $status, summary: $summary, outputs: $outputs, next_signals: []}' \
  > "$RESULT_FILE"

exit 0
