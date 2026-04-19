#!/bin/sh
# Release Prepare Stage: Prepare for release if review succeeded
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
    # Note: Following the established pattern, we pass the type to ledger_append.
    # The actual implementation of ledger_append in the environment must be prepending the type.
    ledger_append "release_prepare" "$(jq -n --arg status "failed" --arg summary "$1" '{type: "release_prepare_record", status: $status, summary: $summary}')"
    exit 1
}

# 1. Verify context exists
if [ ! -f "$CONTEXT_FILE" ]; then
    fail "context.json not found"
fi

# 2. Check Review Stage Status
if [ ! -f "/workspace/ledger.log" ]; then
    fail "ledger.log not found"
fi

LEDGER_CONTENT=$(ledger_read)

# 3. Adjudication Logic (Check review stage status)
# Following review/run.sh pattern to extract status
REVIEW_STATUS=$(echo "$LEDGER_CONTENT" | grep '^review ' | tail -n 1 | cut -d' ' -f2- | jq -r '.status')

if [ "$REVIEW_STATUS" != "succeeded" ]; then
    fail "Review stage did not succeed (Status: $REVIEW_STATUS). Cannot proceed."
fi

# 4. Generate Release Manifest
TIMESTAMP=$(date +%s)
MANIFEST=$(jq -n --arg timestamp "$TIMESTAMP" '{version: "1.0.0", description: "Release preparation successful", timestamp: $timestamp}')

# 5. Record to Ledger
LEDGER_MSG=$(jq -n --arg status "succeeded" --arg summary "Release preparation successful" --argjson manifest "$MANIFEST" '{type: "release_prepare_record", status: $status, summary: $summary, manifest: $manifest}')
ledger_append "release_prepare" "$LEDGER_MSG"

# 6. Emit result
# Ensure it strictly follows internal/contracts/schemas/stage-result.schema.json
jq -n \
  --arg status "succeeded" \
  --arg summary "Release preparation successful" \
  --argjson outputs "$MANIFEST" \
  '{status: $status, summary: $summary, outputs: $outputs, next_signals: []}' \
  > "$RESULT_FILE"

exit 0
