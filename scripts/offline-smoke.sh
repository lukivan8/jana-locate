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
frontend_container=$(docker compose -p "$project" -f compose.yaml -f compose.offline.yaml ps -q frontend)
SMOKE_CONTAINER="$frontend_container" SMOKE_EXPECTED_API_URL="$JANA_API_URL" python3 scripts/smoke.py http://backend:8080 http://frontend:8080 write
docker compose -p "$project" -f compose.yaml -f compose.offline.yaml restart backend
docker compose -p "$project" -f compose.yaml -f compose.offline.yaml up --no-build --pull never --wait --wait-timeout 180
SMOKE_CONTAINER="$frontend_container" SMOKE_EXPECTED_API_URL="$JANA_API_URL" python3 scripts/smoke.py http://backend:8080 http://frontend:8080 read
docker compose -p "$project" -f compose.yaml -f compose.offline.yaml ps -q | xargs docker stats --no-stream --format '{{.Name}} {{.MemUsage}} {{.CPUPerc}}'
