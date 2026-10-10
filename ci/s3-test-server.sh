#!/usr/bin/env bash
# Starts a throw-away S3-compatible server for the live S3 tests
# (controller/s3_live_test.go): versitygw over TLS on 127.0.0.1:7070, with
# virtual-host addressing for s3.us-east-1.amazonaws.com so the AWS-style
# code path is tested too. versitygw checks SigV4 signatures, so a signing
# bug fails the tests. The keys below are test-only values, not real
# credentials. Prints the env vars the tests need (append to $GITHUB_ENV).
#
#   ci/s3-test-server.sh >> "$GITHUB_ENV"     # CI
#   eval "$(ci/s3-test-server.sh | sed 's/^/export /')"   # local
set -euo pipefail
IMAGE=ghcr.io/versity/versitygw:v1.8.0
DIR=${S3_TEST_DIR:-${RUNNER_TEMP:-/tmp}/s3-test}
PORT=${S3_TEST_PORT:-7070}
ACCESS=ci-access-key
SECRET=ci-secret-key-not-real

mkdir -p "$DIR"
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj "/CN=s3-test" \
  -addext "subjectAltName=DNS:*.s3.us-east-1.amazonaws.com,DNS:s3.us-east-1.amazonaws.com,IP:127.0.0.1" \
  -keyout "$DIR/key.pem" -out "$DIR/cert.pem" >/dev/null 2>&1
chmod 644 "$DIR/key.pem" "$DIR/cert.pem"

docker rm -f s3-test >/dev/null 2>&1 || true
docker run -d --name s3-test -p "127.0.0.1:$PORT:7070" -v "$DIR:/certs:ro" \
  -e ROOT_ACCESS_KEY="$ACCESS" -e ROOT_SECRET_KEY="$SECRET" \
  -e VGW_BACKEND=posix -e VGW_BACKEND_ARG=/tmp \
  -e VGW_CERT=/certs/cert.pem -e VGW_KEY=/certs/key.pem \
  -e VGW_VIRTUAL_DOMAIN=s3.us-east-1.amazonaws.com \
  "$IMAGE" >/dev/null

# Unsigned requests get 403 once it is up.
for _ in $(seq 1 60); do
  code=$(curl -s -o /dev/null -w '%{http_code}' --cacert "$DIR/cert.pem" "https://127.0.0.1:$PORT/" || true)
  [ "$code" = "403" ] && break
  sleep 1
done
if [ "$code" != "403" ]; then
  docker logs s3-test >&2 || true
  echo "S3 test server did not start (last status $code)" >&2
  exit 1
fi

echo "S3_TEST_ENDPOINT=https://127.0.0.1:$PORT"
echo "S3_TEST_CA_FILE=$DIR/cert.pem"
echo "S3_TEST_ACCESS_KEY=$ACCESS"
echo "S3_TEST_SECRET_KEY=$SECRET"
