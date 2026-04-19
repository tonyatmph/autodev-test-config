#!/bin/bash
# Robust test runner complying with LBSM architecture

# Do not use set -e to allow manual error handling
. /tooling/lib/ledger/run.sh
cd /workspace

# 1. Contract-Driven: Verify context
if [ ! -f context.json ]; then
    echo '{"status": "failed", "summary": "Missing context.json", "outputs": {}, "next_signals": []}' > result.json
    exit 1
fi

# 2. Run tests with explicit error handling
TEST_OUTPUT=$(go test ./... 2>&1)
TEST_EXIT_CODE=$?

if [ $TEST_EXIT_CODE -eq 0 ]; then
    STATUS="succeeded"
    SUMMARY="Tests passed"
    # 3. Forensic Evidence: Structured JSON for ledger
    LEDGER_ENTRY=$(jq -n --arg status "passed" '{event: "test-execution", status: $status}')
else
    STATUS="failed"
    SUMMARY="Tests failed"
    LEDGER_ENTRY=$(jq -n --arg status "failed" --arg err "$TEST_OUTPUT" '{event: "test-execution", status: $status, error: $err}')
fi

# Write result.json (Contract: stage-result.schema.json)
jq -n \
  --arg status "$STATUS" \
  --arg summary "$SUMMARY" \
  --arg output "$TEST_OUTPUT" \
  '{status: $status, summary: $summary, outputs: {test_log: $output}, next_signals: []}' > result.json

# Record to ledger
ledger_append "test" "$LEDGER_ENTRY"

# Exit with the test result code
exit $TEST_EXIT_CODE
