#!/bin/sh
set -e

if [ -z "$CONFIG_SOURCE" ]; then
    echo "CONFIG_SOURCE is not set" >&2
    exit 1
fi

case "$CONFIG_SOURCE" in 
    PROD|TEST) ;; 
    *) echo "CONFIG_SOURCE must be PROD or TEST" >&2; exit 1;; 
esac

if [ -z "$CONFIG_REPO_URL" ]; then
    echo "CONFIG_REPO_URL is not set" >&2
    exit 1
fi

if [ -z "$CONFIG_COMMIT" ]; then
    echo "CONFIG_COMMIT is not set" >&2
    exit 1
fi

/usr/local/bin/prepare-workspace.sh

if [ $# -gt 0 ]; then
    exec "$@"
else
    exec control-plane
fi
