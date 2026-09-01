#!/usr/bin/env bash
# Deploy the Go backend to both test and production after all tests pass.
# Usage: ./deploy.sh
set -euo pipefail

SSH_KEY="$HOME/Downloads/startuptech-v2.pem"
AWS_USER="ubuntu"
AWS_HOST="ec2-13-42-39-69.eu-west-2.compute.amazonaws.com"
BINARY="pos-rest"

cd "$(dirname "$0")"

# ─── 1. Tests ─────────────────────────────────────────────────────────────────
echo "==> Running tests..."
go test ./... -count=1
echo "==> All tests passed."

# ─── 2. Build (once, reused for both environments) ────────────────────────────
echo "==> Building for linux/amd64..."
GOOS=linux GOARCH=amd64 go build -o "$BINARY" .
echo "    Checksum: $(sha256sum ./$BINARY)"

# ─── helper ───────────────────────────────────────────────────────────────────
deploy_to() {
    local service="$1"
    local remote_dest="$2"
    local label="$3"

    echo ""
    echo "==> [$label] Stopping $service..."
    ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "$AWS_USER@$AWS_HOST" \
        "sudo systemctl stop $service"

    echo "==> [$label] Copying binary..."
    scp -i "$SSH_KEY" -o StrictHostKeyChecking=no \
        "./$BINARY" "$AWS_USER@$AWS_HOST:$remote_dest/$BINARY"

    echo "==> [$label] Starting $service..."
    ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "$AWS_USER@$AWS_HOST" \
        "sudo systemctl start $service && sha256sum $remote_dest/$BINARY && sudo systemctl status $service --no-pager"

    echo "==> [$label] Done."
}

# ─── 3. Deploy ────────────────────────────────────────────────────────────────
deploy_to "start-api-test" "/home/ubuntu/go/src/github.com/sirinibin/pos-rest-test" "TEST"
deploy_to "start-api"      "/home/ubuntu/go/src/github.com/sirinibin/pos-rest"      "PRODUCTION"

echo ""
echo "==> Both test and production API deployed successfully."
