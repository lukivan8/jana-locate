#!/bin/sh
# Disposable synthetic DB; never point this test at an operational database.
set -eu
cd "$(dirname "$0")/.."
project="jana-cli-smoke-$$"
export CLI_PORT="${CLI_SMOKE_PORT:-18096}" CLI_TEST_PG_PORT="${CLI_SMOKE_PG_PORT:-15439}"
export JANA_WEB_AUTH=password JANA_PUBLIC_ORIGIN= JANA_BROWSER_URL="http://127.0.0.1:$CLI_PORT/"
export JANA_WEB_TOKEN="$(python3 -c 'import secrets; print(secrets.token_hex(24))')"
compose() { docker compose -p "$project" -f compose.yaml -f compose.demo.yaml -f tests/compose.smoke.yaml "$@"; }
cleanup() { compose down --volumes --remove-orphans; }
trap cleanup EXIT INT TERM
compose up --build -d --wait --wait-timeout 240
export JANA_TEST_DATABASE_URL="postgresql://jana_demo:synthetic-only@127.0.0.1:$CLI_TEST_PG_PORT/jana_demo?sslmode=disable"
go test -race ./...
go vet ./...
export JANA_TEST_WEB_URL="http://127.0.0.1:$CLI_PORT"
python3 tests/smoke.py write > /tmp/"$project"-run.json
compose restart workspace
compose up -d --wait --wait-timeout 30
python3 tests/smoke.py read < /tmp/"$project"-run.json
compose exec -T postgres psql -U jana_demo -d jana_demo -At -c "SELECT count(*)=5 AND count(*) FILTER (WHERE source_id=9223372036854775807 AND apartment='88' AND entrance='3')=1 FROM public.orders_jana_addresses;" | python3 -c 'import sys; assert sys.stdin.read().strip()=="t"; print("PostgreSQL JOIN key and operator corrections: OK")'
# Node is needed only for these browser tests, never for the application/image.
compose exec -T workspace /jana normalize --file /fixtures/addresses.json --quiet --no-interactive > /tmp/"$project"-file.json
npm ci
export JANA_TEST_WORKSPACE_CONTAINER="$(compose ps -q workspace)"
npm run test:browser
file_run="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])' < /tmp/"$project"-file.json)"
compose exec -T workspace /jana land --ready-only=false --run "$file_run" --output /data/normalized.json --confirm
rm -f /tmp/"$project"-run.json /tmp/"$project"-file.json
