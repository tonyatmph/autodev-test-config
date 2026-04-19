#!/bin/sh
# Implement Stage: Mutation Service
# Uses the Cognition Provider (the brain)

# Source ledger library
. tooling/lib/ledger/run.sh

# Define paths
CONTEXT_FILE="/workspace/context.json"
RESULT_FILE="/workspace/result.json"

# Robust Error Handling
fail() {
    echo "{\"status\": \"failed\", \"summary\": \"$1\", \"outputs\": {}, \"next_signals\": []}" > "$RESULT_FILE"
    exit 1
}

# Ensure we are in the right place
cd /workspace || fail "failed to access /workspace"

# Verify context exists
if [ ! -f "$CONTEXT_FILE" ]; then
    fail "context.json not found"
fi

# 1. Run cognition to get the implementation proposal
cognition --model gemini-3.1-flash-lite-preview --provider gemini || fail "cognition failed"

# 2. Extract content using jq
if [ ! -f "$RESULT_FILE" ]; then
    fail "cognition result file not found"
fi

RESPONSE=$(cat "$RESULT_FILE" | jq -r '.response')
FILE=$(echo "$RESPONSE" | jq -r '.file')
CONTENT=$(echo "$RESPONSE" | jq -r '.content')

if [ -z "$FILE" ] || [ -z "$CONTENT" ] || [ "$FILE" = "null" ]; then
    fail "failed to parse cognition response"
fi

# 3. Apply changes
echo "$CONTENT" > "$FILE" || fail "failed to write file"
git add "$FILE" || fail "git add failed"
git commit -m "feat: cognitive mutation applied" || fail "git commit failed"

# 4. Record to ledger
NEW_SHA=$(git rev-parse HEAD)
# Use structured JSON for ledger
LEDGER_MSG=$(jq -n --arg sha "$NEW_SHA" --arg file "$FILE" '{"action": "mutation", "sha": $sha, "file": $file}')
ledger_append "implementation" "$LEDGER_MSG" || fail "ledger update failed"

# 5. Emit result according to schema
jq -n \
  --arg status "succeeded" \
  --arg summary "Applied cognitive mutation, SHA: $NEW_SHA" \
  --arg file "$FILE" \
  '{status: $status, summary: $summary, outputs: {file: $file}, next_signals: []}' \
  > "$RESULT_FILE"

