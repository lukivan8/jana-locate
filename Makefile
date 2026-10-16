.PHONY: up down test generate check smoke reference offline-smoke nlp-test
up:
	docker compose up --build --wait
down:
	docker compose down
test:
	cd backend && go test -race ./...
check: test
	cd backend && go vet ./...
	cd frontend && npm ci && npm run build && npm run typecheck
	cmp backend/api/openapi.json frontend/src/api/openapi.json
generate:
	./scripts/sync-contract.sh
smoke:
	./scripts/compose-smoke.sh
reference:
	$(MAKE) -C backend reference
offline-smoke:
	./scripts/offline-smoke.sh
nlp-test:
	nlp/.venv/bin/python -m unittest discover -s nlp/tests
