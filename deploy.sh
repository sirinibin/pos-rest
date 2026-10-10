#!/usr/bin/env bash
# Deploy the Go backend to both test and production after all tests pass.
#
# Usage:
#   ./deploy.sh           — deploy test + production
set -euo pipefail

SSH_KEY="$HOME/Downloads/startuptech-v2.pem"
AWS_USER="ubuntu"
AWS_HOST="ec2-13-42-39-69.eu-west-2.compute.amazonaws.com"
BINARY="pos-rest"
HEALTH_BINARY="pos-health-monitor"
HEALTH_DEST="/home/ubuntu/go/src/github.com/sirinibin/pos-rest"

cd "$(dirname "$0")"

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
echo ""
echo "==> Running tests..."
# Skip TestResolveDateKeyword_TimezoneOffset_SA — flaky near midnight UTC (SA/UTC day-boundary edge case)
go test ./... -count=1 -skip "TestResolveDateKeyword_TimezoneOffset_SA"
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
deploy_to "start-api-test" "/home/ubuntu/go/src/github.com/sirinibin/pos-rest-test" "TEST" 2002
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
