#!/bin/sh

# Security scan: Simple check for dangerous patterns
# Adheres to LBSM architecture:
# 1. Contract-Driven: Reads context.json, writes to result.json (stage-result.schema.json)
# 2. Robust Error Handling: No set -e, explicit error checking
# 3. Forensic Evidence: Structured JSON for ledger recording

# Source ledger library
. /tooling/lib/ledger/run.sh

# Ensure workspace is target
WORKSPACE_DIR="/workspace"

# 1. Read context (ensure it exists)
if [ ! -f "$WORKSPACE_DIR/context.json" ]; then
  # If context is missing, it's a critical infrastructure failure for the stage
  echo '{"status": "failed", "summary": "context.json missing", "outputs": {"error": "Missing input context"}, "next_signals": []}' > "$WORKSPACE_DIR/result.json"
  ledger_append "security" "$(jq -n --arg status "failed" --arg summary "context.json missing" '{type: "security_scan", status: $status, summary: $summary}')"
  exit 1
fi

# 2. Perform scan
# Check for dangerous functions
# Redirect output to avoid clutter, capture exit code for analysis
grep -rE "eval\(|os.Exec" "$WORKSPACE_DIR" > /dev/null 2>&1
SCAN_EXIT_CODE=$?

# 3. Construct result
# Define variables for JSON construction
STATUS="succeeded"
SUMMARY="Security scan passed"
OUTPUTS='{}'
NEXT_SIGNALS='[]'

if [ $SCAN_EXIT_CODE -eq 0 ]; then
  # Pattern found - failure
  STATUS="failed"
  SUMMARY="Insecure pattern found"
  OUTPUTS='{"findings": "Insecure patterns found (eval or os.Exec)"}'
elif [ $SCAN_EXIT_CODE -gt 1 ]; then
  # Some other error (e.g. grep failed to run)
  STATUS="failed"
  SUMMARY="Security scan failed to execute"
  OUTPUTS='{"error": "grep utility failed with exit code '$SCAN_EXIT_CODE'"}'
fi

# 4. Construct JSON for result.json
jq -n \
  --arg status "$STATUS" \
  --arg summary "$SUMMARY" \
  --argjson outputs "$OUTPUTS" \
  --argjson next_signals "$NEXT_SIGNALS" \
  '{status: $status, summary: $summary, outputs: $outputs, next_signals: $next_signals}' > "$WORKSPACE_DIR/result.json"

# 5. Ledger append with structured JSON
LEDGER_ENTRY=$(jq -n --arg status "$STATUS" --arg summary "$SUMMARY" '{type: "security_scan", status: $status, summary: $summary}')
ledger_append "security" "$LEDGER_ENTRY"

exit 0
