#!/bin/sh
# Model images must already exist. Internal bridge has no outbound connectivity.
set -eu
cd "$(dirname "$0")/.."
project="jana-offline-$$"
cleanup() { docker compose -p "$project" -f compose.yaml -f compose.offline.yaml down --volumes --remove-orphans; }
trap cleanup EXIT INT TERM
export API_PORT="${SMOKE_API_PORT:-18080}" WEB_PORT="${SMOKE_WEB_PORT:-13000}"
export JANA_API_URL="http://localhost:$API_PORT" CORS_ORIGINS="http://localhost:$WEB_PORT"
docker compose -p "$project" -f compose.yaml -f compose.offline.yaml up --no-build --pull never --wait --wait-timeout 180
python3 scripts/smoke.py "http://localhost:$API_PORT" "http://localhost:$WEB_PORT" write
docker compose -p "$project" -f compose.yaml -f compose.offline.yaml restart backend gliner
docker compose -p "$project" -f compose.yaml -f compose.offline.yaml up --no-build --pull never --wait --wait-timeout 180
python3 scripts/smoke.py "http://localhost:$API_PORT" "http://localhost:$WEB_PORT" read
docker compose -p "$project" -f compose.yaml -f compose.offline.yaml ps -q | xargs docker stats --no-stream --format '{{.Name}} {{.MemUsage}} {{.CPUPerc}}'
