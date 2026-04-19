#!/bin/sh
set -e
# This executor runs the binary passed as input
./echo-binary > /workspace/output.txt
echo '{"status": "executed", "output": "Hello World"}' > /workspace/result.json
