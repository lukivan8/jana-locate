#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
(cd backend && go generate ./pkg/api)
cp backend/api/openapi.json frontend/src/api/openapi.json
(cd frontend && npm run contract:generate)
