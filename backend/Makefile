.PHONY: check test build generate reference
check: test
	go vet ./...
	go build ./...
test:
	go test -race ./...
build:
	mkdir -p bin
	go build -trimpath -o bin/jana-address ./cmd/server
generate:
	go generate ./pkg/api
reference:
	python3 tools/build_reference.py --input data/reference/entities.jsonl --manifest data/reference/manifest.json --output data/reference/reference.sqlite
