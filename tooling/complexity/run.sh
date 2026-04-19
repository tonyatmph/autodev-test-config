#!/bin/sh
set -e
# Complexity Provider: Analyzes code and returns fitness
# Primitive: CC * Depth * Mutations
cd /workspace

# Simple proxy: count functions (CC proxy), nesting (grep {), mutations (grep :=)
CC=$(grep -r "func " . | wc -l)
DEPTH=$(grep -r "{" . | wc -l)
MUTATIONS=$(grep -r ":=" . | wc -l)

SCI=$((CC * DEPTH * (MUTATIONS > 0 ? MUTATIONS : 1)))
THRESHOLD=10000

if [ "$SCI" -gt "$THRESHOLD" ]; then
    echo '{"status": "failed", "fitness": 0.0, "reason": "SCI too high: '$SCI'"}' > /workspace/result.json
else
    echo '{"status": "succeeded", "fitness": 1.0, "sci": '$SCI'}' > /workspace/result.json
fi
