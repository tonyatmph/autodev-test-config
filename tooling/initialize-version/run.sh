#!/bin/sh
set -e

# Factory: Initialize Version File
# Contract: Requires repo existence
# Mutation: Create version.json

VERSION_FILE="version.json"
if [ ! -f "$VERSION_FILE" ]; then
    echo '{"version": "0.1.0"}' > "$VERSION_FILE"
    git add "$VERSION_FILE"
    git commit -m "chore: initialize version file"
fi
