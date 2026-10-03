#!/bin/sh
# Runs every test: Go, the client JS units, and the pairing E2E against a
# local server. Usage: sh test/run.sh
set -e
cd "$(dirname "$0")/.."
export GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/gocache}"
go vet ./...
go test ./...
node test/geo_test.mjs
node test/device_test.mjs
bin="${TMPDIR:-/tmp}/kafumu-test"
go build -o "$bin" .
PORT=18081 "$bin" >/dev/null 2>&1 &
pid=$!
trap 'kill $pid 2>/dev/null' EXIT
sleep 1
node test/pair_test.mjs http://localhost:18081
