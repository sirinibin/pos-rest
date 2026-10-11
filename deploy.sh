#!/usr/bin/env bash
# Deploy the Go backend to both test and production after every test suite
# passes (ci/run_all_tests.sh: unit + API, ZATCA signing, integration, API
# e2e, the frontend's UI e2e on every screen size, the race detector and the
# ZATCA sandbox).
#
# The only deploy path (GitHub Actions is not used). One script for both
# servers: test once, build once, deploy that binary to the test API,
# health-check it (rolled back if unhealthy), then the same binary to
# production.
#
# Usage:
#   ./deploy.sh              — test API, then production
#   ./deploy.sh --test-only  — stop after the test API
#
# Needs Docker running (throwaway MongoDB, Redis and S3 for the tests); see
# ci/run_all_tests.sh for the other tools and settings.
set -euo pipefail

# Deploy only after every suite passes: QUICK=1 (fewer suites) is for local
# iteration with ci/run_all_tests.sh, never for a deploy.
if [ -n "${QUICK:-}" ]; then
    echo "==> ABORTED: deploy.sh always runs every test suite; unset QUICK and run it again."
    exit 1
fi

SSH_KEY="$HOME/Downloads/startuptech-v2.pem"
AWS_USER="ubuntu"
AWS_HOST="ec2-13-42-39-69.eu-west-2.compute.amazonaws.com"
BINARY="pos-rest"
HEALTH_BINARY="pos-health-monitor"
HEALTH_DEST="/home/ubuntu/go/src/github.com/sirinibin/pos-rest"

cd "$(dirname "$0")"

TEST_ONLY=false
for arg in "$@"; do
    case "$arg" in
        --test-only) TEST_ONLY=true ;;
        *) echo "Unknown option: $arg"; echo "Usage: $0 [--test-only]"; exit 1 ;;
    esac
done

# ─── guard: uncommitted changes ───────────────────────────────────────────────

echo ""
echo "==> Checking for uncommitted changes..."
if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
    echo "==> ABORTED: uncommitted changes found. Commit or stash them before deploying."
    echo ""
    git status --short
    exit 1
fi
echo "==> Working tree is clean."

# ─── 1. Tests ─────────────────────────────────────────────────────────────────
# Every suite: unit + API, ZATCA signing, integration with MongoDB/Redis/S3,
# API e2e, the frontend's UI e2e on every screen size, the race detector and
# the ZATCA sandbox, on this machine (see ci/run_all_tests.sh). Nothing
# deploys unless they all pass.
echo ""
echo "==> Running the test suites (ci/run_all_tests.sh)..."
bash ci/run_all_tests.sh
echo "==> All tests passed."

# ─── 2. Build (main API + health monitor) ────────────────────────────────────
echo ""
echo "==> Building for linux/amd64..."
build_log=$(mktemp)
trap 'rm -f "$build_log" "$build_log.hm"' EXIT

GOOS=linux GOARCH=amd64 go build -o "$BINARY" . 2>&1 | tee "$build_log"
if [ -s "$build_log" ]; then
    echo "==> ABORTED: main API build produced warnings or errors. Fix them before deploying."
    exit 1
fi
echo "    Main API checksum: $(sha256sum ./$BINARY)"

GOOS=linux GOARCH=amd64 go build -o "$HEALTH_BINARY" ./health-monitor/ 2>&1 | tee "$build_log.hm"
if [ -s "$build_log.hm" ]; then
    echo "==> ABORTED: health-monitor build produced warnings or errors. Fix them before deploying."
    exit 1
fi
echo "    Health monitor checksum: $(sha256sum ./$HEALTH_BINARY)"

# ─── SSH options shared by all remote calls ───────────────────────────────────
SSH_OPTS="-i $SSH_KEY -o StrictHostKeyChecking=no -o ConnectTimeout=30 -o ServerAliveInterval=15 -o ServerAliveCountMax=3"

# ─── helper ───────────────────────────────────────────────────────────────────
deploy_to() {
    local service="$1"
    local remote_dest="$2"
    local label="$3"
    local port="$4"
    local tmp="$remote_dest/${BINARY}.new"
    local local_sum
    local_sum=$(sha256sum "./$BINARY" | awk '{print $1}')

    # ── Upload with retry (service still live — no downtime during transfer) ──
    local attempt=1 max=3 delay=15
    while [ "$attempt" -le "$max" ]; do
        echo ""
        echo "==> [$label] Uploading binary (attempt $attempt/$max, service still live)..."
        [ "$attempt" -gt 1 ] && { echo "==> [$label] Waiting ${delay}s before retry..."; sleep "$delay"; delay=$((delay * 2)); }

        if scp $SSH_OPTS "./$BINARY" "$AWS_USER@$AWS_HOST:$tmp"; then
            # ── Verify checksum before touching the running service ──────────
            # A partial upload from a dropped connection produces a corrupt binary.
            # Never swap it in — that crashes the service.
            local remote_sum
            remote_sum=$(ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" "sha256sum $tmp 2>/dev/null | awk '{print \$1}'")
            if [ "$local_sum" = "$remote_sum" ]; then
                echo "==> [$label] Checksum verified. Proceeding to swap."
                break
            fi
            echo "==> [$label] Checksum MISMATCH (local=$local_sum remote=$remote_sum) — upload was corrupted, will retry."
            ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" "rm -f $tmp" 2>/dev/null || true
        fi

        attempt=$((attempt + 1))
        if [ "$attempt" -gt "$max" ]; then
            echo "==> [$label] Upload FAILED after $max attempts. Aborting — service untouched."
            return 1
        fi
    done

    # ── Graceful, health-checked restart with automatic rollback ─────────────
    # No kill and no separate stop/start: the binary is renamed in while the
    # old process keeps serving, `systemctl restart` lets it finish in-flight
    # requests, and the previous binary comes back if /v1/health fails.
    echo "==> [$label] Restarting $service gracefully..."
    if ! ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" "bash -s -- '$remote_dest' '$service' '$port'" < deploy/remote_restart.sh; then
        echo "==> [$label] ERROR: deploy failed (see above). Manual intervention may be required!"
        return 1
    fi

    echo "==> [$label] Done."
}

# ─── 3. Deploy main API ──────────────────────────────────────────────────────
# Production only after the test API is healthy on this binary: deploy_to
# fails (and remote_restart.sh rolls the test API back) otherwise.
deploy_to "start-api-test" "/home/ubuntu/go/src/github.com/sirinibin/pos-rest-test" "TEST" 2002
if [ "$TEST_ONLY" = true ]; then
    echo ""
    echo "==> Test API deployed (--test-only: production and the health monitor left as they are)."
    exit 0
fi
deploy_to "start-api" "/home/ubuntu/go/src/github.com/sirinibin/pos-rest" "PRODUCTION" 2000

# ─── 4. Deploy health monitor (single shared instance) ────────────────────────
echo ""
echo "==> [HEALTH-MONITOR] Deploying..."
HM_TMP="$HEALTH_DEST/${HEALTH_BINARY}.new"
HM_SUM=$(sha256sum "./$HEALTH_BINARY" | awk '{print $1}')

scp $SSH_OPTS "./$HEALTH_BINARY" "$AWS_USER@$AWS_HOST:$HM_TMP"
remote_hm_sum=$(ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" "sha256sum $HM_TMP | awk '{print \$1}'")
if [ "$HM_SUM" != "$remote_hm_sum" ]; then
    echo "==> [HEALTH-MONITOR] Checksum MISMATCH — upload corrupted. Health monitor not updated."
else
    ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" \
        "sudo systemctl stop start-health-monitor.service || true; mv -f $HM_TMP $HEALTH_DEST/$HEALTH_BINARY && sync"
    ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" \
        "sudo systemctl start start-health-monitor.service && sudo systemctl is-active start-health-monitor.service"
    echo "==> [HEALTH-MONITOR] Done."
fi

echo ""
echo "==> Both test and production API + health monitor deployed successfully."
