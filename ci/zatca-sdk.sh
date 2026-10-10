#!/usr/bin/env bash
# Installs what the server's ZATCA Phase 2 code needs to run locally or in CI
# (the production server already has it):
#   - ZatcaPython/venv with the Python packages of ZatcaPython/*.py
#   - ZATCA's e-invoicing Java SDK (invoice signing) in
#     ZatcaPython/utilities/fatoora-cli-simulation: the same copy as the
#     production server (SDK 238-R3.4.4), from github.com/sirinibin/zatca-sdk,
#     checked against SDK_SHA256. ZATCA_SDK_ZIP=/path/to/zip uses a local copy.
# Needs git, python3 and Java 11+ on PATH. Run from anywhere:  ci/zatca-sdk.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SDK_REPO="${ZATCA_SDK_REPO:-https://github.com/sirinibin/zatca-sdk}"
SDK_SHA256="4b8f87b7cf831a38c05d0b16bdb8c8a50e6f3fb3d0673c0adc7cd5c7bb9e1c8e"
DEST="$ROOT/ZatcaPython/utilities/fatoora-cli-simulation"
# the signing code (ZatcaPython/utilities/einvoice_signer.py, csr_generator.py)
# calls the jar by this name
JAR_NAME="zatca-einvoicing-sdk-238-R3.4.4.jar"

if [ ! -x "$ROOT/ZatcaPython/venv/bin/python" ]; then
  python3 -m venv "$ROOT/ZatcaPython/venv"
fi
"$ROOT/ZatcaPython/venv/bin/pip" install --quiet pyopenssl asn1crypto cryptography lxml pytz requests

if [ -f "$DEST/Apps/$JAR_NAME" ]; then
  echo "ZATCA SDK already in $DEST"
  exit 0
fi
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
ZIP="${ZATCA_SDK_ZIP:-}"
if [ -z "$ZIP" ]; then
  for i in 1 2 3; do
    git clone --quiet --depth 1 "$SDK_REPO" "$WORK/repo" && break
    rm -rf "$WORK/repo"
    sleep $((i * 5))
  done
  ZIP="$WORK/repo/fatoora-cli-simulation.zip"
fi
echo "$SDK_SHA256  $ZIP" | sha256sum -c -
unzip -q "$ZIP" -d "$WORK/x"
mkdir -p "$DEST"
cp -r "$WORK/x/fatoora-cli-simulation/." "$DEST/"
test -f "$DEST/Apps/$JAR_NAME"
echo "ZATCA SDK installed in $DEST"
