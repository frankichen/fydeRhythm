#!/bin/sh
set -eu

BASE_URL="${BASE_URL:-http://127.0.0.1:18080}"
: "${API_TOKEN:?set API_TOKEN first}"
DEVICE_ID="${DEVICE_ID:-smoke-test}"

curl -fsS "$BASE_URL/healthz"
printf '\n'

curl -fsS \
  -H "Authorization: Bearer $API_TOKEN" \
  "$BASE_URL/api/v1/stats"
printf '\n'

curl -fsS -X POST \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -H "X-Device-ID: $DEVICE_ID" \
  -d '{"phrase":"同步测试词","shortcut":"synctest","weight":100,"category":"test"}' \
  "$BASE_URL/api/v1/lexicon"
printf '\n'
