#!/usr/bin/env bash
# API end-to-end and functional tests against a real server:
# builds the binary, starts it on $API_PORT against a TEST database and runs
# the e2e/api suite over HTTP. E2E_ZATCA=1 also runs the ZATCA sandbox tests
# (run ci/zatca-sdk.sh first; needs internet).
#
#   MONGO_HOST=127.0.0.1 MONGO_PORT=27017 REDIS_DSN=127.0.0.1:6379 e2e/run.sh [go test flags]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export MONGO_DB="${MONGO_DB:-t1_e2e}"
case "$MONGO_DB" in t1_*|test*|erp_test*) ;; *) echo "MONGO_DB must be a test database"; exit 1 ;; esac
export MONGO_HOST="${MONGO_HOST:-127.0.0.1}" MONGO_PORT="${MONGO_PORT:-27017}"
export API_PORT="${API_PORT:-2010}"
export ACCESS_SECRET="${ACCESS_SECRET:-e2e-secret}"
# every test signs up its own company and signs in its users
export ERP_LOGIN_LIMIT=1000000 ERP_SIGNUP_LIMIT=1000000
WORK="$(mktemp -d)"
trap 'kill ${API_PID:-} 2>/dev/null || true; wait ${API_PID:-} 2>/dev/null || true; cp "$WORK/api.log" "${E2E_SERVER_LOG:-/dev/null}" 2>/dev/null || true; rm -rf "$WORK"' EXIT

RACE=()
[ "${E2E_RACE:-}" = "1" ] && RACE=(-race)
(cd "$ROOT" && go build "${RACE[@]}" -o "$WORK/posrest" .)
# the server reads and writes files relative to its working directory
mkdir -p "$WORK/run/zatca"
cp "$ROOT"/localhost.*.pem "$WORK/run/"
ln -s "$ROOT/fonts" "$WORK/run/fonts"
ln -s "$ROOT/ZatcaPython" "$WORK/run/ZatcaPython"
cp "$ROOT/zatca/standard_invoice.xml" "$WORK/run/zatca/"
(cd "$WORK/run" && exec "$WORK/posrest" > "$WORK/api.log" 2>&1) & API_PID=$!
for i in $(seq 1 90); do curl -sf "http://127.0.0.1:$API_PORT/v1/erp/meta" >/dev/null && break; sleep 1; done

TAGS="e2e"
[ "${E2E_ZATCA:-}" = "1" ] && TAGS="e2e zatca"
set +e
(cd "$ROOT" && E2E_BASE_URL="http://127.0.0.1:$API_PORT" E2E_MONGO_URI="mongodb://$MONGO_HOST:$MONGO_PORT" E2E_MONGO_DB="$MONGO_DB" \
  go test -tags "$TAGS" ./e2e/... -count=1 "$@")
EXIT=$?
set -e
# a panic inside a handler is recovered by net/http and logged: report them
PANICS=$(grep -c "http: panic serving" "$WORK/api.log" || true)
echo "server: $PANICS handler panic(s) logged" >&2
if [ "${E2E_RACE:-}" = "1" ]; then
  RACES=$(grep -c "WARNING: DATA RACE" "$WORK/api.log" || true)
  echo "server: $RACES data race(s) reported" >&2
fi
exit $EXIT
