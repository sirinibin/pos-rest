#!/usr/bin/env bash
# Delegates to deploy.sh — runs tests then deploys to both test and production.
set -euo pipefail
cd "$(dirname "$0")"
exec bash deploy.sh
