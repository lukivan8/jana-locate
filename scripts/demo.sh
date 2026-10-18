#!/bin/sh
# Dedicated local demonstration. Stop preserves this project's data volume.
set -eu
cd "$(dirname "$0")/.."
export API_PORT="${DEMO_API_PORT:-18088}" WEB_PORT="${DEMO_WEB_PORT:-13008}"
export JANA_API_URL="http://localhost:$API_PORT" CORS_ORIGINS="http://localhost:$WEB_PORT"
compose() { docker compose -p jana-hackathon -f compose.yaml -f compose.numeric.yaml "$@"; }
case "${1:-up}" in
  up)
    compose up --build --wait --wait-timeout 300
    printf '\nJana Locate: http://localhost:%s\nAPI: http://localhost:%s/openapi.json\n' "$WEB_PORT" "$API_PORT"
    ;;
  stop) compose stop ;;
  status) compose ps ;;
  *) echo 'Usage: scripts/demo.sh {up|stop|status}' >&2; exit 2 ;;
esac
