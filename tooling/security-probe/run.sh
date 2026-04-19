#!/bin/sh
set -e
# Security Researcher Provider:
# Contract: Analyze codebase for "insecure" patterns (e.g., hardcoded creds, dangerous functions)
# Fitness: 1.0 (clean), 0.0 (vulnerable)

. /tooling/lib/ledger/run.sh

cd /workspace

# Pattern-based security scan
FINDINGS_FILE="security_findings.json"
echo "[]" > "$FINDINGS_FILE"

if grep -rE "(API_KEY|SECRET_KEY|password)" . > /dev/null; then
  echo '{"type": "hardcoded-secret"}' > "$FINDINGS_FILE"
  echo '{"status": "failed", "fitness": 0.0, "reason": "hardcoded secrets found"}' > /workspace/result.json
  ledger_append "security" "Vulnerability detected: hardcoded secret"
else
  echo '{"status": "succeeded", "fitness": 1.0, "reason": "no critical patterns found"}' > /workspace/result.json
  ledger_append "security" "Security scan passed"
fi
