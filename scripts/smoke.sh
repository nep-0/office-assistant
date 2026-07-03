#!/usr/bin/env bash
set -euo pipefail

base_url="${BASE_URL:-http://localhost:${FRONTEND_PORT:-8080}}"

curl --fail --silent --show-error "$base_url/" >/dev/null
curl --fail --silent --show-error "$base_url/api/health" | grep -q '"status":"ok"'
curl --fail --silent --show-error "$base_url/api/ready" | grep -q '"status":"ready"'

echo "Smoke check passed for $base_url"
