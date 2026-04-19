#!/bin/sh
set -e
# This script will be the shared entrypoint for stages to interact with the Git Ledger.
# It will provide standard ledger-append and ledger-read primitives.

ledger_append() {
    # Simple example: append to the work-order journal
    local entry_type=$1
    local content=$2
    # In reality, this would perform a git commit/append in the work-order repo.
    echo "Appending to ledger: $entry_type" >&2
    echo "$content" >> /workspace/ledger.log
}

ledger_read() {
    # Read the latest state from the ledger
    cat /workspace/ledger.log
}
