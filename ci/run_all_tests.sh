#!/usr/bin/env bash
# The unit, functional and end-to-end suites of .github/workflows/tests.yml,
# on this machine. deploy.sh runs this and deploys only if it exits 0. It
# stops at the first failing stage.
#
#   ci/run_all_tests.sh          unit, functional and e2e (the default)
#   FULL=1 ci/run_all_tests.sh   also what only GitHub Actions needs to run
#                                every time: race detector, every screen
#                                size, ZATCA sandbox
#
# Stages (each on fresh, empty MongoDB and Redis):
#   1. Unit and API tests, then the ZATCA signing-key tests (Python + Fatoora SDK)
#   2. Integration tests (MongoDB, Redis and an S3 server)
#   3. API end-to-end tests against the built server (incl. the robustness
#      sweep and the restart test)
#   4. UI end-to-end tests: the frontend's full-stack Playwright suite against
#      this API serving the frontend build, on a laptop screen (FULL=1: every
#      screen size)
#   FULL=1 only:
#   5. Race detector: unit + integration with -race, then the API e2e suite
#      against a -race build of the server (server-side races: report only)
#   6. ZATCA sandbox, API and UI (needs ZATCA's sandbox online)
#
# Tests run on a copy of the committed tree (HEAD), as CI checks out the
# commit, so go mod tidy and the ZATCA SDK never touch this checkout.
#
# Needs: git, go, node 18+, npm, java 17+, python3, curl, unzip, Docker.
# The frontend comes from sirinibin/reactjs-pos at FRONTEND_REF (default
# master). FRONTEND_DIR=<path> uses that checkout as it is instead. Other
# settings: see ci/lib.sh.
set -euo pipefail

API_DIR="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=ci/lib.sh
. "$API_DIR/ci/lib.sh"
trap ci_cleanup EXIT

SKIP="TestResolveDateKeyword_TimezoneOffset_SA"   # flaky near midnight UTC
SKIP_DB="$SKIP|TestHealthCheck_ServicesDown"     # only holds with no MongoDB/Redis

cd "$API_DIR"
require_tools git go node npm java python3 curl unzip
require_node_major 18
require_docker

FRONTEND_REF="${FRONTEND_REF:-$(default_ref)}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/startpos-api-ci.XXXXXX")"
CI_LOGS="$WORK/logs"
mkdir -p "$CI_LOGS"
API="$WORK/api"
mkdir -p "$API"
git archive HEAD | tar -x -C "$API"
echo "==> Testing pos-rest $(git log --oneline -1) in $API"

cd "$API"
go mod tidy

# The Fatoora SDK (public sirinibin/zatca-sdk repo) and a Python venv, cached.
zatca_sdk() {
    local sdk="$CI_CACHE/zatca-sdk"
    mkdir -p "$CI_CACHE"
    if [ -d "$sdk/.git" ]; then
        git -C "$sdk" pull --quiet --ff-only || fail "could not update the ZATCA SDK"
    else
        git clone --depth 1 --quiet https://github.com/sirinibin/zatca-sdk.git "$sdk" || fail "could not clone the ZATCA SDK"
    fi
    unzip -q -o "$sdk/fatoora-cli-simulation.zip" -d "$API/ZatcaPython/utilities"
}

python_venv() {
    local venv="$CI_CACHE/zatca-venv"
    if [ ! -x "$venv/bin/python" ]; then
        python3 -m venv "$venv" || fail "could not create a Python venv"
    fi
    "$venv/bin/pip" install --quiet pyopenssl asn1crypto cryptography lxml pytz requests || fail "pip install failed"
    echo "$venv"
}

# ── 1. Unit and API tests (no database) ──────────────────────────────────────
stage "Unit and API tests"
go test ./... -count=1 -skip "$SKIP"

stage "ZATCA signing-key tests (Python + Fatoora SDK)"
zatca_sdk
VENV="$(python_venv)"
java -version 2>&1 | head -1
"$VENV/bin/python" -m unittest discover -s ZatcaPython/tests -v

# ── 2. Integration tests: MongoDB, Redis, S3 ─────────────────────────────────
stage "Integration tests (MongoDB + Redis + S3)"
services_up s3
MONGO_DB=startpos_integration_test \
S3_TEST_ENDPOINT="http://127.0.0.1:$CI_S3_PORT" S3_TEST_ACCESS_KEY=$S3_KEY S3_TEST_SECRET_KEY=$S3_SECRET \
    go test -tags integration ./... -count=1 -skip "$SKIP_DB"

# ── 3. API end-to-end: the built server over real HTTP ──────────────────────
stage "API end-to-end tests"
services_up
export MONGO_DB=pos_e2e
go build -o pos-rest .
go run ./e2e/seed
start_api "$API" ./pos-rest "$CI_LOGS/api-e2e-server.log"
go test -tags e2e ./e2e/... -count=1 -v 2>&1 | tee "$CI_LOGS/api-e2e.log" || fail "API e2e tests (log: $CI_LOGS/api-e2e.log)"
stop_api

# ── 4. UI end-to-end: the frontend in Chromium against this API ─────────────
stage "Frontend build ($FRONTEND_REF)"
if [ -n "${FRONTEND_DIR:-}" ]; then
    FE="$(cd "$FRONTEND_DIR" && pwd)"
    echo "==> Using the frontend checkout at $FE as it is"
else
    FE_URL="$(origin_url "$API_DIR/../reactjs-pos" "$API_DIR/../frontend" || echo https://github.com/sirinibin/reactjs-pos.git)"
    FE="$(fetch_repo reactjs-pos "$FRONTEND_REF" "$FE_URL")"
fi
npm_install_keep_lock "$FE"
FE_BUILD="$WORK/frontend-build"
frontend_build "$FE" "" "$FE_BUILD"
playwright_setup "$FE/e2e"

if [ "${FULL:-}" = 1 ]; then
    UI_PROJECTS="" UI_LABEL="every screen size"
else
    UI_PROJECTS="--project=desktop" UI_LABEL="laptop screen; FULL=1 for every size"
fi
stage "UI end-to-end tests (Playwright, $UI_LABEL)"
services_up
go run ./e2e/seed
start_api "$API" ./pos-rest "$CI_LOGS/ui-e2e-server.log" "$FE_BUILD"
(cd "$FE/e2e" && npx playwright test -c playwright.fullstack.config.js $UI_PROJECTS) || fail "UI e2e tests (report: $FE/e2e/playwright-report-fullstack)"
stop_api

# ── 5. Race detector against MongoDB + Redis (FULL=1) ───────────────────────
if [ "${FULL:-}" = 1 ]; then
    stage "Race detector: unit and integration tests"
    services_up
    MONGO_DB=startpos_race_test go test -race -tags integration ./... -count=1 -skip "$SKIP_DB"

    stage "Race detector: API e2e against a -race server"
    services_up
    go build -race -o pos-rest-race .
    go run ./e2e/seed
    start_api "$API" ./pos-rest-race "$CI_LOGS/race-server.log" "" GORACE=halt_on_error=0
    POS_REST_BIN="$API/pos-rest-race" go test -race -tags e2e ./e2e/... -count=1
    sleep 5
    stop_api
    RACES=$(grep -c "WARNING: DATA RACE" "$CI_LOGS/race-server.log" || true)
    if [ "$RACES" != 0 ]; then
        grep -A 40 "WARNING: DATA RACE" "$CI_LOGS/race-server.log" | grep -E "pos-rest/|^(Write|Read|Previous)" | sort | uniq -c | sort -rn | head -40 || true
        note "the -race server logged $RACES data race(s) (report only, as in CI): $CI_LOGS/race-server.log"
    fi
fi

# ── 6. ZATCA Phase 2 against ZATCA's sandbox (FULL=1) ───────────────────────
# Depends on ZATCA's sandbox being up and reachable, so it is left out by
# default (the GitHub Actions pipeline still runs it). A new non-production
# store: VAT 399999999900003, CRN 4030360927, OTP 12345.
if [ "${FULL:-}" = 1 ]; then
    stage "ZATCA sandbox (API + UI)"
    rm -rf ZatcaPython/venv
    ln -s "$VENV" ZatcaPython/venv
    SDK=ZatcaPython/utilities/fatoora-cli-simulation
    find "$SDK/Configuration" -name '*_config.json' -delete
    mkdir -p "$SDK/Data/Certificates" "$SDK/Data/Input" "$SDK/Data/PIH"
    test -f "$SDK/Apps/zatca-einvoicing-sdk-238-R3.4.4.jar" || fail "the Fatoora SDK jar is missing"
    services_up
    go run ./e2e/seed
    start_api "$API" ./pos-rest "$CI_LOGS/zatca-server.log" "$FE_BUILD"
    go test -tags "e2e zatca" ./e2e/api -run TestZatcaSandbox -count=1 -v 2>&1 | tee "$CI_LOGS/zatca.log" \
        || fail "ZATCA sandbox API tests (log: $CI_LOGS/zatca.log)"
    if [ -f "$FE/e2e/fullstack/zatca.spec.js" ]; then
        (cd "$FE/e2e" && E2E_ZATCA=1 E2E_ZATCA_OTP=12345 \
            npx playwright test -c playwright.fullstack.config.js fullstack/zatca.spec.js --project=desktop) \
            || fail "ZATCA sandbox UI tests"
    fi
    stop_api
else
    stage_end
    note "race detector, other screen sizes and ZATCA sandbox not run (FULL=1 runs them; GitHub Actions runs them on every push)"
fi

ci_summary
cd "$API_DIR"
rm -rf "$WORK"
