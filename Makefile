.PHONY: up down test generate check smoke reference reference-fetch reference-prepare reference-test offline-smoke showcase showcase-watch showcase-status showcase-stop
REFERENCE_PYTHON ?= .artifacts/reference-venv/bin/python
showcase:
	./scripts/showcase.sh deploy
showcase-watch:
	./scripts/install-showcase-service.sh
showcase-status:
	./scripts/showcase.sh status
showcase-stop:
	systemctl --user stop jana-showcase.service
	./scripts/showcase.sh stop
up:
	docker compose up --build --wait
down:
	docker compose down
test:
	cd backend && go test -race ./...
check: test cli-check
	cd backend && go vet ./...
	cd frontend && npm ci && npm run build && npm run typecheck
	cmp backend/api/openapi.json frontend/src/api/openapi.json
	cmp backend/api/openapi.json cli/api/normalization.openapi.json
	cmp cli/api/openapi.json frontend/src/api/local-openapi.json
	node scripts/build-cli-web.mjs --check
generate:
	./scripts/sync-contract.sh
smoke:
	./scripts/compose-smoke.sh

reference:
	python3 backend/tools/build_reference.py --input backend/data/reference/entities.jsonl --manifest backend/data/reference/manifest.json --output backend/data/reference/reference.sqlite
reference-fetch:
	$(REFERENCE_PYTHON) scripts/reference/fetch.py
reference-prepare:
	$(REFERENCE_PYTHON) scripts/reference/prepare.py --kato .artifacts/reference/kato.xlsx --pbf .artifacts/reference/kazakhstan-latest.osm.pbf --output backend/data/reference/entities.jsonl --addresses backend/data/reference/addresses.jsonl.gz --manifest backend/data/reference/manifest.json
reference-test:
	$(REFERENCE_PYTHON) -m unittest discover -s backend/tools -p 'test_*.py' -v
offline-smoke:
	./scripts/offline-smoke.sh

.PHONY: cli-check cli-smoke cli-web
cli-web:
	node scripts/build-cli-web.mjs
cli-check:
	cd cli && go test -race ./... && go vet ./...
cli-smoke:
	./cli/scripts/smoke.sh
