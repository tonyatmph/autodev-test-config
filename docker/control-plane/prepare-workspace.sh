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

BOOTSTRAP_DIR="/autodev-bootstrap"
mkdir -p "$BOOTSTRAP_DIR"

git clone "$CONFIG_REPO_URL" "$BOOTSTRAP_DIR/repo"
cd "$BOOTSTRAP_DIR/repo"
git checkout "$CONFIG_COMMIT"

mkdir -p /home/autodev/.autodev/config/stage-specs
mkdir -p /home/autodev/.autodev/config/pipelines
cp -R "$BOOTSTRAP_DIR/repo/$CONFIG_SOURCE/." "/home/autodev/.autodev/config/"
chown -R autodev:autodev "/home/autodev/.autodev"
chmod -R a-w "/home/autodev/.autodev"

# Clean up bootstrap material
rm -rf "$BOOTSTRAP_DIR"
