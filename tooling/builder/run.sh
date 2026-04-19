#!/bin/sh
set -e
cd /workspace
# This builder produces a simple binary
echo 'package main; import "fmt"; func main() { fmt.Println("Hello World") }' > main.go
go build -o echo-binary main.go
# This is a mock: in reality, we would commit the binary hash to the ledger
echo '{"status": "built", "binary_sha": "SHA-BINARY-123"}' > /workspace/result.json
