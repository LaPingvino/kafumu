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
# People coming and going at random; a failure prints its seed to replay:
# node test/fuzz.mjs http://localhost:18081 <seed> <steps>
node test/fuzz.mjs http://localhost:18081 "" 80
# Two-browser UI test, when chromium can run (not inside the sandbox).
if command -v chromium >/dev/null 2>&1 && [ -z "$SKIP_BROWSER" ]; then
  PORT=18082 "$bin" >/dev/null 2>&1 &
  pid2=$!
  trap 'kill $pid $pid2 2>/dev/null' EXIT
  sleep 1
  node test/browser_test.mjs http://localhost:18082
fi
# Passkeys with Chrome's virtual authenticator.
if command -v chromium >/dev/null 2>&1 && [ -z "$SKIP_BROWSER" ]; then
  PORT=18095 KAFUMU_ORIGIN=http://localhost:18095 KAFUMU_PASSKEYS=1 "$bin" >/dev/null 2>&1 &
  pid3=$!
  trap 'kill $pid ${pid2:-} $pid3 2>/dev/null' EXIT
  sleep 1
  node test/passkey_test.mjs http://localhost:18095
fi

# Datastore-backed code against the emulator, when the SDK has it.
emu="$HOME/google-cloud-sdk/platform/cloud-datastore-emulator/cloud_datastore_emulator"
if [ -x "$emu" ] && command -v java >/dev/null 2>&1 && [ -z "$SKIP_EMULATOR" ]; then
  # Own process group (setsid) so the whole emulator, java included, can be
  # stopped by group; run it from TMPDIR so its WEB-INF/ lands there.
  (cd "${TMPDIR:-/tmp}" && exec setsid "$emu" start --host=localhost --port=8432 --store_on_disk=false --consistency=1.0) >/dev/null 2>&1 &
  emupid=$!
  sleep 1
  emupgid=$(ps -o pgid= -p "$emupid" 2>/dev/null | tr -d ' ')
  trap 'kill $pid ${pid2:-} ${pid3:-} 2>/dev/null; [ -n "$emupgid" ] && kill -- -"$emupgid" 2>/dev/null' EXIT
  for i in $(seq 1 30); do curl -s localhost:8432 >/dev/null 2>&1 && break; sleep 1; done
  DATASTORE_EMULATOR_HOST=localhost:8432 go test ./internal/purge/
fi
