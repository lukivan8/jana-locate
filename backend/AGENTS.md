# Working with only the Go backend

This directory is the complete service. Read `README.md`, `docs/consumer-guide.md`, `docs/catalog.md`, and `api/openapi.json`; these require no repository siblings.

- Module: `github.com/lukivan8/jana-address` is the historical compatibility path; the maintained source lives in jana-locate/backend. `pkg/api` and `pkg/client` are the only public Go integration packages. The implemented native libpostal adapter and pure-Go SQLite reader are justified normalization dependencies; keep the public SDK independent of them.
- `api/openapi.json` is the source of truth. Generate types using `go generate ./pkg/api`; never hand-edit `pkg/api/types.gen.go`. Make additive `/v1` changes or introduce a new API version for breaking changes.
- Keep parsing/matching in `internal/normalize`, persistence in `internal/catalog`, and transport in `internal/httpapi`. HTTP handlers must not become parser implementations.
- Always retain input order, input IDs and exact raw strings. Valid uncertain inputs get a result, with explicit null fields and no invented coordinates. Do not choose an arbitrary candidate when matching is ambiguous.
- Exposing unresolved ambiguity is a successful normalization outcome. `city` denotes any settlement, not necessarily an urban city. Keep compatible city/village homonyms as separate reference identities; LLM preference, synthetic generation provenance and annotation review closure cannot select a winner or suppress ambiguity diagnostics. Train span extraction separately from geographic identity resolution; any chosen ambiguous LLM span remains a hypothesis with its ambiguity preserved.
- Store building-level confirmations only. No lookup by raw_address or input id. Apartment, entrance, floor and landmark never become catalog defaults.
- All overlay access must use the Store mutex and revision check. File replacement must remain atomic; commit memory only after persistence succeeds. Do not introduce multiple writers to one overlay file.
- Test: `go test -race ./...`, `go vet ./...`; compile the public consumer example as part of those checks. Run benchmarks only as fixture throughput measurements.
- Hand consumers the spec and this documentation. If only this directory is available, do not assume the frontend exists or edit its files. Describe changes so the frontend can adopt the new spec independently.

- Native libpostal is pass one; regex rules and immutable KATO/OSM references provide supplementary evidence. Reconcile in Go. Never make HTTP handlers perform domain resolution.
- KATO/OSM SQLite is immutable and contains no building coordinates. Do not redirect confirmation writes into it.
- Synthetic fixtures are disabled by default. Evaluation must use `ALLOW_DEMO_CATALOG=false`; never remove their source warnings to improve reported scores.
- Confidence derives from evidence and conflicts. Do not restore obsolete fixed constants merely to satisfy old assertions.
