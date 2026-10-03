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
# Two-browser UI test, when chromium can run (not inside the sandbox).
if command -v chromium >/dev/null 2>&1 && [ -z "$SKIP_BROWSER" ]; then
  PORT=18082 "$bin" >/dev/null 2>&1 &
  pid2=$!
  trap 'kill $pid $pid2 2>/dev/null' EXIT
  sleep 1
  node test/browser_test.mjs http://localhost:18082
fi
