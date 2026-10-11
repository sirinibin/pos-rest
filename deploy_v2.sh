#!/usr/bin/env bash
# Deploy the Go backend v2 to https://startpos-api-v2.gulfunionozone.com
# Must be run from the v2 branch (enforced below).
#
# Runs the same gates as .github/workflows/deploy_v2.yml, one after another,
# and deploys only when every gate passes (stops at the first failure and
# prints its output). Works on macOS and Linux.
#
#   lint        go vet, staticcheck (correctness checks), build with no output
#   unit        go test -race, no database (>= 2000 passing), Card Bridge vet + tests
#   api         -tags integration vs MongoDB + Redis + S3 test server (>= 2500 passing,
#               every /v1/erp route covered)
#   smoke       ci/smoke.sh: real binary, seeded test DB, HTTP checks
#   e2e         e2e/run.sh: race-built server, API e2e + functional (>= 300 passing)
#   ui-adapter  e2e-starterp: the StartERP UI (frontend master build) against /v1/erp,
#               read back through the old v1 endpoints (not in the workflow)
#   race-db     go test -race ./erp with the DB (REPORT ONLY, as in CI)
#   zatca       TestZatcaSandbox vs ZATCA's sandbox (REPORT ONLY, as in CI)
#   deploy      build linux/amd64, Card Bridge setup files, upload, swap, restart
#   cleanup     delete merged claude/* branches older than 24 h (report only)
#
# Every test step prints passed / failed / skipped counts (repeated in the
# final summary) and fails on any failure or any skipped test that did not run
# elsewhere (the unit run's DB tests must pass in the api run).
#
# Usage:
#   ./deploy_v2.sh                  every gate, then deploy
#   ./deploy_v2.sh --check          every gate, no deploy (any branch, dirty tree allowed)
#   ./deploy_v2.sh --only unit,api  just those steps, no deploy
#   ./deploy_v2.sh --force          skip the branch check (emergencies); gates still run
#
# Needs Go, jq, curl, openssl; MongoDB on 27017 + Redis on 6379 (running ones
# are used, only t1_* test databases are touched; else started with Docker,
# else local mongod / redis-server); Docker for the S3 test server (api).
# zatca also needs Java 11+ and python3 (skipped without them).
# Logs: .deploy-logs/<time>/<step>.log
set -euo pipefail

FORCE=false
CHECK_ONLY=false
ONLY=""
while [ $# -gt 0 ]; do
    case "$1" in
        --force) FORCE=true ;;
        --check) CHECK_ONLY=true ;;
        --only) ONLY="$2"; CHECK_ONLY=true; shift ;;
        --only=*) ONLY="${1#--only=}"; CHECK_ONLY=true ;;
        -h|--help) sed -n '2,32p' "$0"; exit 0 ;;
        *) echo "unknown option: $1 (see ./deploy_v2.sh --help)"; exit 2 ;;
    esac
    shift
done

SSH_KEY="${SSH_KEY:-$HOME/Downloads/startuptech-v2.pem}"
AWS_USER="ubuntu"
AWS_HOST="ec2-13-42-39-69.eu-west-2.compute.amazonaws.com"
BINARY="pos-rest"
V2_SERVICE="start-api-v2"
V2_DEST="/home/ubuntu/go/src/github.com/sirinibin/pos-rest-v2"

cd "$(dirname "$0")"
ROOT="$(pwd)"

export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
export MONGO_HOST="${MONGO_HOST:-127.0.0.1}" MONGO_PORT="${MONGO_PORT:-27017}" REDIS_DSN="${REDIS_DSN:-127.0.0.1:6379}"
export API_PORT="${API_PORT:-2010}"
RUN_ID="$(date +%Y%m%d-%H%M%S)"
LOG_DIR="${DEPLOY_LOG_ROOT:-$ROOT/.deploy-logs}/$RUN_ID"
WORK_DIR="$LOG_DIR/work"
export RUNNER_TEMP="$WORK_DIR"
mkdir -p "$WORK_DIR/bin"
# macOS has shasum, not sha256sum (used here and by ci/zatca-sdk.sh)
if ! command -v sha256sum >/dev/null; then
    printf '#!/bin/sh\nexec shasum -a 256 "$@"\n' > "$WORK_DIR/bin/sha256sum"
    chmod +x "$WORK_DIR/bin/sha256sum"
fi
export PATH="$WORK_DIR/bin:$PATH"

# ─── step runner and summary ──────────────────────────────────────────────────

STEP_NAMES=()
STEP_RESULTS=()
STEP_SECS=()

want() {
    [ -z "$ONLY" ] && return 0
    case ",$ONLY," in *",$1,"*) return 0 ;; esac
    return 1
}

record() { STEP_NAMES+=("$1"); STEP_RESULTS+=("$2"); STEP_SECS+=("$3"); }

print_summary() {
    [ ${#STEP_NAMES[@]} -eq 0 ] && return 0
    echo ""
    echo "────────────────────── deploy_v2.sh summary ──────────────────────"
    local i
    for i in "${!STEP_NAMES[@]}"; do
        printf '  %-10s %-32s %dm%02ds\n' "${STEP_NAMES[$i]}" "${STEP_RESULTS[$i]}" $((STEP_SECS[$i] / 60)) $((STEP_SECS[$i] % 60))
    done
    if [ -s "$LOG_DIR/counts.txt" ]; then
        echo "  test counts:"
        grep -E '^(TEST SUMMARY|SKIPS|SMOKE SUMMARY|COUNTS)' "$LOG_DIR/counts.txt" | sed 's/^/    /'
    fi
    echo "  logs: $LOG_DIR"
    echo "───────────────────────────────────────────────────────────────────"
}

# run_step <name> gate|report <function> [args...]
#   gate:   a failure prints the end of its output and stops the run (nothing is deployed)
#   report: a failure is recorded and the run goes on (as continue-on-error in CI)
run_step() {
    local name="$1" mode="$2"; shift 2
    want "$name" || return 0
    local log="$LOG_DIR/$name.log" start rc
    start=$(date +%s)
    echo ""
    echo "════════ [$name] $(date +%H:%M:%S) ════════"
    set +e
    ( set -e; cd "$ROOT"; "$@" ) 2>&1 | tee "$log"
    rc=${PIPESTATUS[0]}
    set -e
    local secs=$(( $(date +%s) - start ))
    if [ "$rc" -eq 0 ]; then record "$name" "passed" "$secs"; return 0; fi
    if [ "$rc" -eq 78 ]; then record "$name" "skipped ($(tail -1 "$log"))" "$secs"; return 0; fi
    if [ "$mode" = report ]; then record "$name" "FAILED (report only)" "$secs"; return 0; fi
    record "$name" "FAILED" "$secs"
    echo ""
    echo "✖ [$name] failed (exit $rc). Last 80 lines of $log:"
    echo "───────────────────────────────────────────────────────────────────"
    tail -80 "$log"
    echo "───────────────────────────────────────────────────────────────────"
    echo "✖ Not deployed: [$name] must pass first."
    exit 1
}

skip_step() { echo "$1"; exit 78; }

# summary <results.json> <exit> <label> [min]: ci/test-summary.sh, counts kept for the final summary
summary() { ci/test-summary.sh "$@" | tee -a "$LOG_DIR/counts.txt"; return "${PIPESTATUS[0]}"; }
# skips <label> <results.json> [covering.json]: ci/skip-gate.sh, fails on a test that ran nowhere
skips() { ci/skip-gate.sh "$@" | tee -a "$LOG_DIR/counts.txt"; return "${PIPESTATUS[0]}"; }

port_open() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }

cleanup() {
    local s
    if [ -f "$WORK_DIR/services" ]; then
        while IFS= read -r s; do
            case "$s" in
                docker:*) docker rm -f "${s#docker:}" >/dev/null 2>&1 || true ;;
                pid:*) kill "${s#pid:}" 2>/dev/null || true ;;
            esac
        done < "$WORK_DIR/services"
    fi
    # test byproducts, so the next deploy finds a clean tree: Python bytecode the
    # ZATCA signer rewrites, attachment tests' files under controller/tmp, dump.rdb
    if [ "${DEPLOY_NO_TREE_CLEANUP:-}" != 1 ]; then # set by ci/deploysync, which runs this script during go test
        git checkout -- ZatcaPython/utilities/__pycache__ 2>/dev/null || true
        git clean -fdq -- controller/tmp dump.rdb 2>/dev/null || true
    fi
    print_summary
}

# ─── guards (unchanged rules: v2 branch, clean tree) ──────────────────────────

step_preflight() {
    local missing=() v
    for v in go git jq curl openssl; do command -v "$v" >/dev/null || missing+=("$v"); done
    [ ${#missing[@]} -eq 0 ] || { echo "Missing tools: ${missing[*]} (brew install ${missing[*]})"; return 1; }
    go version
    if ! $CHECK_ONLY; then
        local current_branch
        current_branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
        if [ "$current_branch" != "v2" ] && ! $FORCE; then
            echo "==> ABORTED: deploy_v2.sh must be run from the v2 branch."
            echo "    Current branch: $current_branch"
            echo "    Switch with: git checkout v2"
            echo "    Or bypass with: ./deploy_v2.sh --force"
            return 1
        fi
        if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
            echo "==> ABORTED: uncommitted changes found. Commit or stash them before deploying."
            git status --short
            return 1
        fi
        echo "Branch v2, working tree clean."
    fi
    if port_open "$API_PORT"; then echo "Port $API_PORT is in use; smoke and e2e start the API there"; return 1; fi
    if command -v docker >/dev/null && docker info >/dev/null 2>&1; then echo "Docker: running"; else echo "Docker: not running (needed for the S3 test server in api)"; fi
    if command -v java >/dev/null; then echo "Java: $(java -version 2>&1 | head -1)"; else echo "Java: missing (zatca will be skipped)"; fi
}

# ─── gates (as in deploy_v2.yml) ──────────────────────────────────────────────

step_lint() {
    echo "── go vet"
    go vet -composites=false -unreachable=false ./...
    echo "── staticcheck"
    go run honnef.co/go/tools/cmd/staticcheck@2026.2.1 -checks 'SA*,-SA1019,-SA4006,-SA9003' ./...
    echo "── build produces no output"
    local out
    out=$(go build -o "$WORK_DIR/pos-rest" . 2>&1) || { echo "$out"; return 1; }
    if [ -n "$out" ]; then echo "$out"; echo "Build produced output — failing."; return 1; fi
}

step_unit() {
    # no database, as in CI: point Mongo/Redis at a closed port so a local
    # MongoDB/Redis can't change what the unit tests see
    local exit=0
    env -u ERP_TEST_DB TZ=Europe/London MONGO_HOST=127.0.0.1 MONGO_PORT=1 REDIS_DSN=127.0.0.1:1 \
        go test -race ./... -count=1 -json > "$WORK_DIR/unit.json" || exit=$?
    summary "$WORK_DIR/unit.json" "$exit" "Unit tests (race)" 2000
    # its DB-backed tests skip here; the api step checks each one passes there
    echo "── Card Bridge program (vet + tests, race detector)"
    exit=0
    (cd cardbridge && go vet ./... && go test -race ./... -count=1 -json > "$WORK_DIR/cardbridge.json") || exit=$?
    summary "$WORK_DIR/cardbridge.json" "$exit" "Card Bridge (race)" 1
    skips "Card Bridge" "$WORK_DIR/cardbridge.json"
}

# MongoDB + Redis: running ones, else Docker, else local binaries
step_services() {
    local docker_ok=false redis_port="${REDIS_DSN##*:}" i
    command -v docker >/dev/null && docker info >/dev/null 2>&1 && docker_ok=true
    if port_open "$MONGO_PORT"; then
        echo "MongoDB: using the one on :$MONGO_PORT (test databases only)"
    elif $docker_ok; then
        docker run -d --rm --name "posrest-deploy-mongo-$RUN_ID" -p "$MONGO_PORT:27017" mirror.gcr.io/library/mongo:7 >/dev/null
        echo "docker:posrest-deploy-mongo-$RUN_ID" >> "$WORK_DIR/services"
        echo "MongoDB: started in Docker (mongo:7)"
    elif command -v mongod >/dev/null; then
        mkdir -p "$WORK_DIR/mongo"
        mongod --dbpath "$WORK_DIR/mongo" --port "$MONGO_PORT" --bind_ip 127.0.0.1 > "$WORK_DIR/mongod.log" 2>&1 &
        echo "pid:$!" >> "$WORK_DIR/services"
        echo "MongoDB: started local mongod"
    else
        echo "MongoDB is missing: start Docker Desktop, or brew install mongodb-community, or run one on :$MONGO_PORT"; return 1
    fi
    if port_open "$redis_port"; then
        echo "Redis: using the one on :$redis_port"
    elif $docker_ok; then
        docker run -d --rm --name "posrest-deploy-redis-$RUN_ID" -p "$redis_port:6379" mirror.gcr.io/library/redis:7 >/dev/null
        echo "docker:posrest-deploy-redis-$RUN_ID" >> "$WORK_DIR/services"
        echo "Redis: started in Docker (redis:7)"
    elif command -v redis-server >/dev/null; then
        redis-server --port "$redis_port" --save "" --appendonly no > "$WORK_DIR/redis.log" 2>&1 &
        echo "pid:$!" >> "$WORK_DIR/services"
        echo "Redis: started local redis-server"
    else
        echo "Redis is missing: start Docker Desktop, or brew install redis, or run one on :$redis_port"; return 1
    fi
    for i in $(seq 1 60); do port_open "$MONGO_PORT" && port_open "$redis_port" && break; sleep 1; done
    port_open "$MONGO_PORT" || { echo "MongoDB did not come up on :$MONGO_PORT"; return 1; }
    port_open "$redis_port" || { echo "Redis did not come up on :$redis_port"; return 1; }
    sleep 2 # let mongod finish starting after it opens its port
}

step_api() {
    command -v docker >/dev/null && docker info >/dev/null 2>&1 || { echo "Docker is needed for the S3 test server (ci/s3-test-server.sh): start Docker Desktop"; return 1; }
    local line exit=0
    echo "docker:s3-test" >> "$WORK_DIR/services"
    ci/s3-test-server.sh > "$WORK_DIR/s3.env"
    while IFS= read -r line; do [ -n "$line" ] && export "${line%%=*}=${line#*=}"; done < "$WORK_DIR/s3.env"
    # every /v1/erp route must be called by some test (route-coverage.json)
    ERP_TEST_DB=1 S3_TEST_REQUIRED=1 TZ=Europe/London \
    ERP_ROUTE_COVERAGE="$WORK_DIR/route-coverage.json" ERP_ROUTE_COVERAGE_MIN=100 \
        go test -tags integration ./... -count=1 -json > "$WORK_DIR/api.json" || exit=$?
    docker rm -f s3-test >/dev/null 2>&1 || true
    summary "$WORK_DIR/api.json" "$exit" "API + integration tests (MongoDB)" 2500
    skips "API + integration" "$WORK_DIR/api.json"
    if [ -f "$WORK_DIR/unit.json" ]; then skips "Unit (no DB)" "$WORK_DIR/unit.json" "$WORK_DIR/api.json"; fi
}

step_smoke() { ci/smoke.sh | tee -a "$LOG_DIR/counts.txt"; return "${PIPESTATUS[0]}"; }

step_e2e() {
    local exit=0
    E2E_RACE=1 E2E_SERVER_LOG="$WORK_DIR/e2e-server.log" E2E_ROUTE_COVERAGE="$WORK_DIR/e2e-route-coverage.json" \
        e2e/run.sh -json -timeout 20m > "$WORK_DIR/e2e.json" || exit=$?
    summary "$WORK_DIR/e2e.json" "$exit" "API e2e + functional tests (real server)" 300 \
        && skips "API e2e" "$WORK_DIR/e2e.json" && return 0
    grep -n -B2 -A25 "panic\|DATA RACE" "$WORK_DIR/e2e-server.log" | head -400 || tail -n 300 "$WORK_DIR/e2e-server.log"
    return 1
}

step_race_db() {
    local exit=0
    ERP_TEST_DB=1 TZ=Europe/London go test -race ./erp/ -count=1 -json > "$WORK_DIR/race.json" || exit=$?
    summary "$WORK_DIR/race.json" "$exit" "erp API tests (race, report only)"
}

step_zatca() {
    command -v java >/dev/null || skip_step "needs Java 11+"
    command -v python3 >/dev/null || skip_step "needs python3"
    ci/zatca-sdk.sh
    local exit=0
    E2E_ZATCA=1 E2E_SERVER_LOG="$WORK_DIR/zatca-server.log" \
        e2e/run.sh -run 'TestZatcaSandbox' -json -timeout 15m > "$WORK_DIR/zatca.json" || exit=$?
    summary "$WORK_DIR/zatca.json" "$exit" "ZATCA sandbox (report only)" 1 \
        && skips "ZATCA sandbox" "$WORK_DIR/zatca.json" && return 0
    grep -i -n "zatca\|python\|error\|panic" "$WORK_DIR/zatca-server.log" | tail -n 200 || true
    return 1
}

# StartERP UI against the /v1/erp adapter, checked through the old v1 endpoints
# (e2e-starterp; not in the workflow). Needs the frontend: FRONTEND_DIR, else a
# clone of starterp-frontend-v1 (master) in .ci/starterp-frontend-v1.
step_ui_adapter() {
    command -v node >/dev/null || { echo "Node is missing (brew install node@22)"; return 1; }
    local fe="${FRONTEND_DIR:-$ROOT/.ci/starterp-frontend-v1}"
    if [ -z "${FRONTEND_DIR:-}" ]; then
        if [ -d "$fe/.git" ]; then
            git -C "$fe" fetch --quiet origin master && git -C "$fe" reset --quiet --hard origin/master
        else
            mkdir -p "$ROOT/.ci" && git clone --quiet --branch master https://github.com/sirinibin/starterp-frontend-v1 "$fe"
        fi
    fi
    echo "frontend: $fe ($(git -C "$fe" log -1 --format='%h %s' 2>/dev/null || echo 'not a git checkout'))"
    (cd "$fe" && npm ci --silent && VITE_API_PREFIX=/v1 npx vite build --outDir dist-e2e --logLevel warn)
    (cd e2e-starterp && npm ci --silent)
    if [ "${PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD:-}" != 1 ]; then (cd e2e-starterp && npx playwright install chromium); fi
    local exit=0
    UI_DIR="$fe/dist-e2e" PLAYWRIGHT_JSON_OUTPUT_NAME="$WORK_DIR/ui-adapter.json" e2e-starterp/run.sh --reporter=list,json || exit=$?
    if [ -f "$WORK_DIR/ui-adapter.json" ]; then
        jq -r '.stats | "COUNTS UI vs adapter (e2e-starterp): passed=\(.expected + .flaky) failed=\(.unexpected) skipped=\(.skipped) flaky=\(.flaky)"' \
            "$WORK_DIR/ui-adapter.json" | tee -a "$LOG_DIR/counts.txt"
        jq -e '.stats.unexpected == 0 and .stats.skipped == 0 and .stats.expected > 0' "$WORK_DIR/ui-adapter.json" >/dev/null || exit=1
    else
        exit=1
    fi
    return $exit
}

# ─── deploy (build, upload, swap and restart: the existing steps) ─────────────

step_deploy() {
    # ─── Build ────────────────────────────────────────────────────────────────
    echo ""
    echo "==> Building for linux/amd64..."
    local build_log
    build_log="$WORK_DIR/build.log"

    GOOS=linux GOARCH=amd64 go build -o "$BINARY" . 2>&1 | tee "$build_log"
    if [ -s "$build_log" ]; then
        echo "==> ABORTED: build produced warnings or errors. Fix them before deploying."
        exit 1
    fi
    echo "    Checksum: $(sha256sum ./$BINARY)"

    # ─── Card Bridge setup files (as the workflow), served from $V2_DEST/cardbridge-dist
    echo ""
    echo "==> Building Card Bridge setup files..."
    cardbridge/build.sh "$WORK_DIR/cardbridge-dist"
    test -s "$WORK_DIR/cardbridge-dist/manifest.json"
    tar -czf "$WORK_DIR/cardbridge-dist.tgz" -C "$WORK_DIR/cardbridge-dist" .

    # ─── SSH options ──────────────────────────────────────────────────────────
    SSH_OPTS="-i $SSH_KEY -o StrictHostKeyChecking=no -o ConnectTimeout=30 -o ServerAliveInterval=15 -o ServerAliveCountMax=3"
    local_sum=$(sha256sum "./$BINARY" | awk '{print $1}')
    tmp="$V2_DEST/${BINARY}.new"

    # ─── Upload with retry (service still live — no downtime during transfer) ─
    attempt=1; max=3; delay=15
    while [ "$attempt" -le "$max" ]; do
        echo ""
        echo "==> [V2] Uploading binary (attempt $attempt/$max, service still live)..."
        [ "$attempt" -gt 1 ] && { echo "==> [V2] Waiting ${delay}s before retry..."; sleep "$delay"; delay=$((delay * 2)); }

        if scp $SSH_OPTS "./$BINARY" "$AWS_USER@$AWS_HOST:$tmp"; then
            remote_sum=$(ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" "sha256sum $tmp 2>/dev/null | awk '{print \$1}'")
            if [ "$local_sum" = "$remote_sum" ]; then
                echo "==> [V2] Checksum verified. Proceeding to swap."
                break
            fi
            echo "==> [V2] Checksum MISMATCH (local=$local_sum remote=$remote_sum) — retrying."
            ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" "rm -f $tmp" 2>/dev/null || true
        fi

        attempt=$((attempt + 1))
        if [ "$attempt" -gt "$max" ]; then
            echo "==> [V2] Upload FAILED after $max attempts. Aborting — service untouched."
            exit 1
        fi
    done

    # ─── Card Bridge files: upload, unpack beside the binary, swap ────────────
    echo "==> [V2] Uploading Card Bridge setup files..."
    scp $SSH_OPTS "$WORK_DIR/cardbridge-dist.tgz" "$AWS_USER@$AWS_HOST:$V2_DEST/cardbridge-dist.tgz"
    ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" "set -e; DEST=$V2_DEST
        rm -rf \"\$DEST/cardbridge-dist.new\" && mkdir -p \"\$DEST/cardbridge-dist.new\"
        tar -xzf \"\$DEST/cardbridge-dist.tgz\" -C \"\$DEST/cardbridge-dist.new\"
        rm -rf \"\$DEST/cardbridge-dist.old\"
        [ -d \"\$DEST/cardbridge-dist\" ] && mv \"\$DEST/cardbridge-dist\" \"\$DEST/cardbridge-dist.old\"
        mv \"\$DEST/cardbridge-dist.new\" \"\$DEST/cardbridge-dist\"
        rm -rf \"\$DEST/cardbridge-dist.old\" \"\$DEST/cardbridge-dist.tgz\""

    # ─── Atomic swap + restart ────────────────────────────────────────────────
    ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" \
        "sudo fuser -k $V2_DEST/$BINARY 2>/dev/null || true; sudo fuser -k $tmp 2>/dev/null || true"

    echo "==> [V2] Stopping $V2_SERVICE, swapping binary..."
    ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" \
        "sudo systemctl stop $V2_SERVICE || true; mv -f $tmp $V2_DEST/$BINARY && sync"

    start_ok=0; start_try=1
    while [ "$start_try" -le 3 ]; do
        echo "==> [V2] Starting $V2_SERVICE (attempt $start_try/3)..."
        if ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" \
            "sudo systemctl start $V2_SERVICE && sudo systemctl is-active --quiet $V2_SERVICE"; then
            start_ok=1; break
        fi
        echo "==> [V2] Start attempt $start_try/3 failed, retrying in 5s..."
        start_try=$((start_try + 1))
        sleep 5
    done

    if [ "$start_ok" -eq 0 ]; then
        echo "==> [V2] ERROR: $V2_SERVICE failed to start. Manual intervention required!"
        exit 1
    fi

    ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" \
        "sha256sum $V2_DEST/$BINARY && sudo systemctl status $V2_SERVICE --no-pager"
}

# ─── cleanup of merged claude/* branches (as the workflow does after a deploy)

step_cleanup() {
    git fetch --quiet --prune origin '+refs/heads/*:refs/remotes/origin/*'
    local now deleted=0 ref branch age
    now=$(date +%s)
    for ref in $(git branch -r --merged origin/v2 --format '%(refname:short)'); do
        case "$ref" in origin/claude/*) ;; *) continue ;; esac
        branch=${ref#origin/}
        case "$branch" in v2|master) continue ;; esac
        age=$(( now - $(git log -1 --format=%ct "$ref") ))
        if [ "$age" -lt 86400 ]; then
            echo "keep   $branch (last commit $((age / 3600)) h ago)"
            continue
        fi
        echo "delete $branch (merged into v2, last commit $((age / 3600)) h ago)"
        if git push origin --delete "$branch"; then deleted=$((deleted + 1)); else echo "warning: could not delete $branch"; fi
    done
    echo "Deleted $deleted merged claude/* branch(es)."
}

# ─── main ─────────────────────────────────────────────────────────────────────

trap cleanup EXIT
trap 'exit 130' INT TERM

run_step preflight gate   step_preflight
run_step lint      gate   step_lint
run_step unit      gate   step_unit
if want api || want smoke || want e2e || want ui-adapter || want race-db || want zatca; then
    run_step services gate step_services
fi
run_step api       gate   step_api
run_step smoke     gate   step_smoke
run_step e2e       gate   step_e2e
run_step ui-adapter gate  step_ui_adapter
run_step race-db   report step_race_db
run_step zatca     report step_zatca

if $CHECK_ONLY; then
    echo ""
    echo "All gates passed (--check/--only: nothing deployed)."
    exit 0
fi

run_step deploy    gate   step_deploy
run_step cleanup   report step_cleanup

echo ""
echo "==> v2 API deployed successfully — https://startpos-api-v2.gulfunionozone.com"
