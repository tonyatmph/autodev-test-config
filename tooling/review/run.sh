#!/bin/sh
# Review Stage: Synthesize prior reports and provide final adjudication
# Uses the LBSM architecture

# Source ledger library
. /tooling/lib/ledger/run.sh

# Define paths
WORKSPACE_DIR="/workspace"
CONTEXT_FILE="$WORKSPACE_DIR/context.json"
RESULT_FILE="$WORKSPACE_DIR/result.json"

# Robust Error Handling
fail() {
    echo "{\"status\": \"failed\", \"summary\": \"$1\", \"outputs\": {}, \"next_signals\": []}" > "$RESULT_FILE"
    # Even on failure, we record to ledger
    ledger_append "review" "$(jq -n --arg status "failed" --arg summary "$1" '{type: "review_adjudication", status: $status, summary: $summary}')"
    exit 1
}

# 1. Verify context exists
if [ ! -f "$CONTEXT_FILE" ]; then
    fail "context.json not found"
fi

# 2. Gather reports from ledger
# Assuming ledger.log contains lines like `type content` where content is JSON
if [ ! -f "/workspace/ledger.log" ]; then
    fail "ledger.log not found"
fi

LEDGER_CONTENT=$(ledger_read)

# 3. Synthesize summary
# Extract the status of the last security and test reports.
SECURITY_STATUS=$(echo "$LEDGER_CONTENT" | grep '^security ' | tail -n 1 | cut -d' ' -f2- | jq -r '.status')
TEST_STATUS=$(echo "$LEDGER_CONTENT" | grep '^test ' | tail -n 1 | cut -d' ' -f2- | jq -r '.status')

# Handle cases where reports might be missing
if [ -z "$SECURITY_STATUS" ] || [ "$SECURITY_STATUS" = "null" ]; then SECURITY_STATUS="unknown"; fi
if [ -z "$TEST_STATUS" ] || [ "$TEST_STATUS" = "null" ]; then TEST_STATUS="unknown"; fi

SUMMARY="Review complete. Security status: $SECURITY_STATUS. Test status: $TEST_STATUS."

# 4. Adjudication Logic
STATUS="succeeded"
if [ "$SECURITY_STATUS" != "succeeded" ] || [ "$TEST_STATUS" != "succeeded" ]; then
    STATUS="failed"
    SUMMARY="$SUMMARY Final adjudication: REJECTED."
else
    SUMMARY="$SUMMARY Final adjudication: APPROVED."
fi

# 5. Record to ledger
LEDGER_MSG=$(jq -n --arg status "$STATUS" --arg summary "$SUMMARY" '{type: "review_adjudication", status: $status, summary: $summary}')
ledger_append "review" "$LEDGER_MSG"

# 6. Emit result
jq -n \
  --arg status "$STATUS" \
  --arg summary "$SUMMARY" \
  '{status: $status, summary: $summary, outputs: {}, next_signals: []}' \
  > "$RESULT_FILE"

exit 0
