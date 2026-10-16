// Package contract exposes the versioned, language-neutral HTTP contract.
package contract

import _ "embed"

//go:embed openapi.json
var OpenAPI []byte
