# Standalone local PostgreSQL consumer

- `cli/` must build alone. Do not import backend internals, read sibling files at runtime, or add a Go workspace/relative module replace.
- PostgreSQL credentials are resolved locally from environment / mounted secret files, or through explicit `connect --web` setup sent only to the local CLI service. Never return saved passwords to the browser or forward credentials to the normalization API.
- Backend owns normalization schemas. `api/normalization.openapi.json` is an explicitly adopted snapshot. Local orchestration endpoints belong to `api/openapi.json`; generated `internal/api/types.gen.go` must not be edited by hand.
- Local SQLite owns input snapshots, checkpoints and operator corrections. It is a work journal, never a future normalization lookup cache. Never publish journals, DB dumps or credentials.
- Original tables are read-only. Publication is explicit, transactional, checks source changes, and preserves the source primary-key type. Do not add delivery details to building defaults.
- Embedded review UI follows the repository's existing light Material Design 3 identity. Use semantic controls, explicit null/uncertainty and synthetic labels. Do not introduce runtime downloads.
- Run `go test -race ./...`, `go vet ./...`, standalone Docker smoke and the browser workflow at desktop and 390x844 with disposable data.

- All user-facing copy (prompts, help, status labels and errors) is Russian. Keep command/flag names, SQL identifiers, API JSON keys and machine status codes unchanged. Localize driver errors at the presentation boundary and never expose credentials.
