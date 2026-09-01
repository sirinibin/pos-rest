#!/usr/bin/env bash
set -euo pipefail

SSH_KEY="$HOME/Downloads/startuptech-v2.pem"
AWS_USER="ubuntu"
AWS_HOST="ec2-13-42-39-69.eu-west-2.compute.amazonaws.com"
REMOTE_DEST="/home/ubuntu/go/src/github.com/sirinibin/pos-rest"
SERVICE="start-api"
BINARY="pos-rest"

cd "$(dirname "$0")"

echo "==> Building for linux/amd64..."
GOOS=linux GOARCH=amd64 go build -o "$BINARY" .
echo "    Checksum: $(sha256sum ./$BINARY)"

echo "==> Stopping $SERVICE on server..."
ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "$AWS_USER@$AWS_HOST" \
    "sudo systemctl stop $SERVICE"

echo "==> Copying binary..."
scp -i "$SSH_KEY" -o StrictHostKeyChecking=no \
    "./$BINARY" "$AWS_USER@$AWS_HOST:$REMOTE_DEST/$BINARY"

echo "==> Starting $SERVICE..."
ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "$AWS_USER@$AWS_HOST" \
    "sudo systemctl start $SERVICE && sha256sum $REMOTE_DEST/$BINARY && sudo systemctl status $SERVICE --no-pager"

echo "==> Done. Production API deployed."
