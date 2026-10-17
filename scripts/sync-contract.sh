#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
(cd backend && go generate ./pkg/api)
cp backend/api/openapi.json frontend/src/api/openapi.json

python3 cli/scripts/adopt-contract.py backend/api/openapi.json
(cd cli && go generate ./internal/api)

cp cli/api/openapi.json frontend/src/api/local-openapi.json
(cd frontend && npm run contract:generate)
