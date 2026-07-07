#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${KCONTEXT_URL:-http://localhost:8083}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

curl -X POST "${BASE_URL}/webhook" \
  -H 'Content-Type: application/json' \
  -d @"${SCRIPT_DIR}/webhook-payload.json"

echo
