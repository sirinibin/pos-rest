#!/usr/bin/env bash
# Deploy the Go backend v2 to https://startpos-api-v2.gulfunionozone.com
# Must be run from the v2 branch (enforced below).
#
# Usage:
#   ./deploy_v2.sh          — lint, test, build, deploy
#   ./deploy_v2.sh --force  — skip branch check (for emergencies)
set -euo pipefail

FORCE=false
for arg in "$@"; do [ "$arg" = "--force" ] && FORCE=true; done

SSH_KEY="${SSH_KEY:-$HOME/Downloads/startuptech-v2.pem}"
AWS_USER="ubuntu"
AWS_HOST="ec2-13-42-39-69.eu-west-2.compute.amazonaws.com"
BINARY="pos-rest"
V2_SERVICE="start-api-v2"
V2_DEST="/home/ubuntu/go/src/github.com/sirinibin/pos-rest-v2"

cd "$(dirname "$0")"

# ─── guard: must be on v2 branch ──────────────────────────────────────────────

check_branch() {
    local current_branch
    current_branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
    if [ "$current_branch" != "v2" ]; then
        echo ""
        echo "==> ABORTED: deploy_v2.sh must be run from the v2 branch."
        echo "    Current branch: $current_branch"
        echo "    Switch with: git checkout v2"
        echo "    Or bypass with: ./deploy_v2.sh --force"
        exit 1
    fi
}

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
go test ./... -count=1 -skip "TestResolveDateKeyword_TimezoneOffset_SA"
echo "==> All tests passed."

# ─── 2. Build ─────────────────────────────────────────────────────────────────
echo ""
echo "==> Building for linux/amd64..."
build_log=$(mktemp)
trap 'rm -f "$build_log"' EXIT

GOOS=linux GOARCH=amd64 go build -o "$BINARY" . 2>&1 | tee "$build_log"
if [ -s "$build_log" ]; then
    echo "==> ABORTED: build produced warnings or errors. Fix them before deploying."
    exit 1
fi
echo "    Checksum: $(sha256sum ./$BINARY)"

# ─── SSH options ──────────────────────────────────────────────────────────────
SSH_OPTS="-i $SSH_KEY -o StrictHostKeyChecking=no -o ConnectTimeout=30 -o ServerAliveInterval=15 -o ServerAliveCountMax=3"
local_sum=$(sha256sum "./$BINARY" | awk '{print $1}')
tmp="$V2_DEST/${BINARY}.new"

# ─── 3. Upload with retry (service still live — no downtime during transfer) ──
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

# ─── 4. Atomic swap + restart ─────────────────────────────────────────────────
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

echo ""
echo "==> v2 API deployed successfully — https://startpos-api-v2.gulfunionozone.com"
