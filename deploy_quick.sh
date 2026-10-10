#!/usr/bin/env bash
# ╔══════════════════════════════════════════════════════════════════════════╗
# ║  EMERGENCY QUICK DEPLOY — SKIPS ALL TESTS                              ║
# ║  Use only when a critical bug must be fixed immediately in production.  ║
# ║  Under normal circumstances always use deploy.sh / deploy_v2.sh.        ║
# ╚══════════════════════════════════════════════════════════════════════════╝
#
# Branch routing (auto-detected):
#   master → production API  (start-api,      port 2000)
#   test   → test API        (start-api-test,  port 2002)
#   v2     → v2 API          (start-api-v2,    port 2004)
#
# Usage:
#   ./deploy_quick.sh          — build + deploy current branch target
#   ./deploy_quick.sh --force  — also skip uncommitted-changes check

set -euo pipefail

FORCE=false
for arg in "$@"; do [ "$arg" = "--force" ] && FORCE=true; done

SSH_KEY="${SSH_KEY:-$HOME/Downloads/startuptech-v2.pem}"
AWS_USER="ubuntu"
AWS_HOST="ec2-13-42-39-69.eu-west-2.compute.amazonaws.com"
BINARY="pos-rest"

cd "$(dirname "$0")"

# ─── Detect branch → set target ───────────────────────────────────────────────
BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
case "$BRANCH" in
  master)
    SERVICE="start-api"
    PORT=2000
    REMOTE_DEST="/home/ubuntu/go/src/github.com/sirinibin/pos-rest"
    LABEL="PRODUCTION"
    ;;
  test)
    SERVICE="start-api-test"
    PORT=2002
    REMOTE_DEST="/home/ubuntu/go/src/github.com/sirinibin/pos-rest-test"
    LABEL="TEST"
    ;;
  v2)
    SERVICE="start-api-v2"
    PORT=2004
    REMOTE_DEST="/home/ubuntu/go/src/github.com/sirinibin/pos-rest-v2"
    LABEL="V2"
    ;;
  *)
    echo "==> ABORTED: deploy_quick.sh does not support branch '$BRANCH'."
    echo "    Supported branches: master, test, v2"
    exit 1
    ;;
esac

echo ""
echo "╔══════════════════════════════════════════════════════════════════════╗"
echo "║  ⚠  EMERGENCY QUICK DEPLOY — TESTS SKIPPED — TARGET: $LABEL"
echo "╚══════════════════════════════════════════════════════════════════════╝"
echo ""

# ─── guard: uncommitted changes ───────────────────────────────────────────────
if ! $FORCE; then
    echo "==> Checking for uncommitted changes..."
    if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
        echo "==> ABORTED: uncommitted changes found. Commit or stash, or use --force."
        git status --short
        exit 1
    fi
    echo "==> Working tree is clean."
fi

# ─── Build (no tests — catches compile errors) ────────────────────────────────
echo ""
echo "==> Building for linux/amd64 (tests SKIPPED)..."
build_log=$(mktemp)
trap 'rm -f "$build_log"' EXIT

GOOS=linux GOARCH=amd64 go build -o "$BINARY" . 2>&1 | tee "$build_log"
if [ -s "$build_log" ]; then
    echo "==> ABORTED: build produced warnings or errors."
    exit 1
fi
echo "    Checksum: $(sha256sum ./$BINARY)"

# ─── SSH options ──────────────────────────────────────────────────────────────
SSH_OPTS="-i $SSH_KEY -o StrictHostKeyChecking=no -o ConnectTimeout=30 -o ServerAliveInterval=15 -o ServerAliveCountMax=3"
local_sum=$(sha256sum "./$BINARY" | awk '{print $1}')
tmp="$REMOTE_DEST/${BINARY}.new"

# ─── Upload with retry ────────────────────────────────────────────────────────
attempt=1; max=3; delay=15
while [ "$attempt" -le "$max" ]; do
    echo ""
    echo "==> [$LABEL] Uploading binary (attempt $attempt/$max)..."
    [ "$attempt" -gt 1 ] && { sleep "$delay"; delay=$((delay * 2)); }

    if scp $SSH_OPTS "./$BINARY" "$AWS_USER@$AWS_HOST:$tmp"; then
        remote_sum=$(ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" "sha256sum $tmp 2>/dev/null | awk '{print \$1}'")
        if [ "$local_sum" = "$remote_sum" ]; then
            echo "==> [$LABEL] Checksum verified."
            break
        fi
        echo "==> [$LABEL] Checksum MISMATCH — retrying."
        ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" "rm -f $tmp" 2>/dev/null || true
    fi

    attempt=$((attempt + 1))
    [ "$attempt" -gt "$max" ] && { echo "==> Upload FAILED after $max attempts."; exit 1; }
done

# ─── Graceful, health-checked restart with automatic rollback ─────────────────
echo "==> [$LABEL] Restarting $SERVICE gracefully..."
ssh $SSH_OPTS "$AWS_USER@$AWS_HOST" "bash -s -- '$REMOTE_DEST' '$SERVICE' '$PORT'" < deploy/remote_restart.sh \
    || { echo "==> ERROR: deploy failed (see above)."; exit 1; }

echo ""
echo "==> [$LABEL] Quick deploy complete. Remember to run full deploy.sh when stable."
