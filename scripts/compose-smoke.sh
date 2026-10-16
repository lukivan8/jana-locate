#!/bin/sh
# Runs a disposable stack on separate ports. Removes only this test's volumes.
set -eu
cd "$(dirname "$0")/.."
project="jana-smoke-$$"
export API_PORT="${SMOKE_API_PORT:-18080}" WEB_PORT="${SMOKE_WEB_PORT:-13000}"
export JANA_API_URL="http://localhost:$API_PORT" CORS_ORIGINS="http://localhost:$WEB_PORT"
cleanup() { docker compose -p "$project" down --volumes --remove-orphans; }
trap cleanup EXIT INT TERM
docker compose -p "$project" up --build --wait --wait-timeout 240
python3 scripts/smoke.py "http://localhost:$API_PORT" "http://localhost:$WEB_PORT" write
docker compose -p "$project" restart backend
docker compose -p "$project" up --wait --wait-timeout 180
python3 scripts/smoke.py "http://localhost:$API_PORT" "http://localhost:$WEB_PORT" read
