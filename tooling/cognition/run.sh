#!/bin/sh
set -e
# Cognition Stage:
# Executes the pre-built binary in the local bin directory
DIR="$(dirname "$0")"
"$DIR/bin/cognition"
