#!/usr/bin/env bash
# Functional smoke test: builds the real binary, seeds the legacy-shaped
# fixture into a TEST database, boots the API on $API_PORT and drives a few
# key flows over HTTP (health, login, me, list/read customers, products,
# sales, dashboard, auth/validation errors). Used by the v2 deploy workflow;
# runs locally too:
#
#   MONGO_HOST=127.0.0.1 MONGO_PORT=27017 REDIS_DSN=127.0.0.1:6379 ci/smoke.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export MONGO_DB="${MONGO_DB:-t1_smoke}"
case "$MONGO_DB" in t1_*|test*|erp_test*) ;; *) echo "MONGO_DB must be a test database"; exit 1 ;; esac
export API_PORT="${API_PORT:-2010}"
export ACCESS_SECRET="${ACCESS_SECRET:-smoke-secret}"
BASE="http://127.0.0.1:$API_PORT"
WORK="$(mktemp -d)"
trap 'kill ${API_PID:-} 2>/dev/null || true; rm -rf "$WORK"' EXIT

(cd "$ROOT" && go build -o "$WORK/posrest" . && go run ./e2e-starterp/seed -reset > "$WORK/fixture.json")
mkdir -p "$WORK/run"
# legacy main.go also serves HTTPS on API_PORT+1 and needs a cert pair in cwd
cp "$ROOT"/localhost.*.pem "$WORK/run/"
ln -s "$ROOT/fonts" "$WORK/run/fonts"
mkdir -p "$WORK/run/zatca" && cp "$ROOT/zatca/standard_invoice.xml" "$WORK/run/zatca/"
(cd "$WORK/run" && exec "$WORK/posrest" > "$WORK/api.log" 2>&1) & API_PID=$!
for i in $(seq 1 90); do curl -sf "$BASE/v1/erp/meta" >/dev/null && break; sleep 1; done

PASS=0; FAIL=0
fx() { jq -r ".$1" "$WORK/fixture.json"; }
# check NAME EXPECTED_STATUS METHOD PATH [TOKEN] [BODY] [JQ_ASSERT]
check() {
  local name=$1 want=$2 method=$3 path=$4 tok=${5:-} body=${6:-} assert=${7:-}
  local args=(-s -o "$WORK/body" -w '%{http_code}' -X "$method" -H 'Content-Type: application/json')
  [ -n "$tok" ] && args+=(-H "Authorization: Bearer $tok")
  [ -n "$body" ] && args+=(-d "$body")
  local code; code=$(curl "${args[@]}" "$BASE$path" || echo 000)
  if [ "$code" != "$want" ]; then
    echo "FAIL $name: $method $path -> $code (want $want): $(head -c 300 "$WORK/body")"; FAIL=$((FAIL+1)); return
  fi
  if [ -n "$assert" ] && ! jq -e "$assert" "$WORK/body" >/dev/null 2>&1; then
    echo "FAIL $name: response does not satisfy $assert: $(head -c 300 "$WORK/body")"; FAIL=$((FAIL+1)); return
  fi
  echo "ok   $name"; PASS=$((PASS+1))
}

STORE=$(fx storeA); ADMIN=$(fx adminEmail); PW=$(fx password)
check "legacy health"            200 GET  /v1/health
check "erp meta"                 200 GET  /v1/erp/meta
check "login wrong password"     401 POST /v1/erp/auth/login "" "{\"email\":\"$ADMIN\",\"password\":\"wrong-$PW\"}"
check "login missing fields"     400 POST /v1/erp/auth/login "" '{}'
check "login"                    200 POST /v1/erp/auth/login "" "{\"email\":\"$ADMIN\",\"password\":\"$PW\"}" '.accessToken | length > 20'
TOK=$(jq -r .accessToken "$WORK/body")
check "me"                       200 GET  /v1/erp/auth/me "$TOK" "" '.email // .user.email | length > 0'
check "me without token"         401 GET  /v1/erp/auth/me
check "customers list"           200 GET  "/v1/erp/customers?storeId=$STORE&limit=5&select=nameEn" "$TOK" "" '.data | length > 0'
check "customer read"            200 GET  "/v1/erp/customers/$(fx customerA1)" "$TOK"
check "products list"            200 GET  "/v1/erp/products?storeId=$STORE&limit=5" "$TOK" "" '.data | length > 0'
check "product read"             200 GET  "/v1/erp/products/$(fx productA1)" "$TOK"
check "sales list"               200 GET  "/v1/erp/sales?storeId=$STORE&limit=5" "$TOK"
check "sale read"                200 GET  "/v1/erp/sales/$(fx orderA1)" "$TOK"
check "sales needs storeId"      400 GET  /v1/erp/sales "$TOK"
check "dashboard bi"              200 GET  "/v1/erp/dashboard/bi?storeId=$STORE" "$TOK"
check "create customer"          201 POST /v1/erp/customers "$TOK" "{\"storeId\":\"$STORE\",\"nameEn\":\"Smoke Customer\"}"

echo "SMOKE SUMMARY: passed=$PASS failed=$FAIL"
if [ "$FAIL" -ne 0 ]; then echo "---- api.log (tail) ----"; tail -60 "$WORK/api.log"; exit 1; fi
