//go:build !libpostal || !cgo

package postal

import "errors"

func Open(dataDir string) (Parser, error) {
	return nil, errors.New("libpostal unavailable: build with CGO_ENABLED=1 and -tags libpostal; provide prepared model data")
}
