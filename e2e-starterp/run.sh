#!/usr/bin/env bash
# StartERP UI e2e against the pos-rest /v1/erp adapter.
#
# Builds the API, seeds the legacy-shaped fixture into a TEST database
# (default t1_e2e — never a real one), starts the API on :2010 and the UI
# harness on :5180 (UI + /v1 → /v1/erp proxy), runs Playwright, stops both.
#
# Required:  UI_FILE=/path/to/prototype/index.html   (or UI_DIR=/path/to/dist)
# Optional:  MONGO_HOST MONGO_PORT REDIS_DSN MONGO_DB API_PORT UI_PORT
#            PLAYWRIGHT_BROWSERS_PATH (pre-installed browsers; this script never installs them)
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
export MONGO_DB="${MONGO_DB:-t1_e2e}"
case "$MONGO_DB" in t1_*|test*|erp_test*) ;; *) echo "MONGO_DB must be a test database (t1_*/test*/erp_test*)"; exit 1 ;; esac
export API_PORT="${API_PORT:-2010}"
UI_PORT="${UI_PORT:-5180}"
export ACCESS_SECRET="${ACCESS_SECRET:-e2e-local-secret}"
[ -n "${UI_FILE:-}${UI_DIR:-}" ] || { echo "set UI_FILE (prototype index.html) or UI_DIR (built dist)"; exit 1; }
export UI_FILE="${UI_FILE:-}" UI_DIR="${UI_DIR:-}"
WORK="$(mktemp -d)"
trap 'kill ${API_PID:-} ${UI_PID:-} 2>/dev/null || true; rm -rf "$WORK"' EXIT

(cd "$ROOT" && go build -o "$WORK/posrest" . && go run ./e2e-starterp/seed -reset > "$WORK/fixture.json")
# legacy main.go also serves HTTPS on API_PORT+1 and needs a cert pair in cwd
mkdir -p "$WORK/run"
if [ -f "$ROOT/localhost.cert.pem" ]; then cp "$ROOT"/localhost.*.pem "$WORK/run/"; else
  openssl req -x509 -newkey rsa:2048 -nodes -subj /CN=localhost -days 2 \
    -keyout "$WORK/run/localhost.key.pem" -out "$WORK/run/localhost.cert.pem" >/dev/null 2>&1; fi
# read-only legacy resources the handlers open relative to cwd (PDF fonts, ZATCA template)
ln -s "$ROOT/fonts" "$WORK/run/fonts"
mkdir -p "$WORK/run/zatca" && cp "$ROOT/zatca/standard_invoice.xml" "$WORK/run/zatca/"
(cd "$WORK/run" && exec "$WORK/posrest" > "$WORK/api.log" 2>&1) & API_PID=$!
for i in $(seq 1 60); do curl -sf "http://localhost:$API_PORT/v1/erp/meta" >/dev/null && break; sleep 1; done
API="http://localhost:$API_PORT" PORT="$UI_PORT" node "$HERE/server.mjs" > "$WORK/ui.log" 2>&1 & UI_PID=$!
sleep 1
cd "$HERE"
E2E_FIXTURE="$WORK/fixture.json" E2E_BASE="http://localhost:$UI_PORT" E2E_API="http://localhost:$API_PORT" npx playwright test "$@" || {
  echo "---- api.log (tail) ----"; tail -40 "$WORK/api.log"; exit 1; }
