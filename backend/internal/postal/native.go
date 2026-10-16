//go:build libpostal && cgo

package postal

/*
#cgo LDFLAGS: -lpostal
#include <stdlib.h>
#include <libpostal/libpostal.h>
static char* jana_label(libpostal_address_parser_response_t* r, size_t i) {return r->labels[i];}
static char* jana_value(libpostal_address_parser_response_t* r, size_t i) {return r->components[i];}
*/
import "C"
import (
	"errors"
	"sync"
	"unsafe"
)

// libpostal has process-global model state and parser scratch buffers.
// Setup once and serialize parsing; do not tear down while requests are active.
var mutex sync.Mutex
var initialized bool
var setupDir string

type native struct{}

func Open(dataDir string) (Parser, error) {
	mutex.Lock()
	defer mutex.Unlock()
	if initialized {
		if dataDir != setupDir {
			return nil, errors.New("libpostal already initialized with another data directory")
		}
		return native{}, nil
	}
	var dir *C.char
	if dataDir != "" {
		dir = C.CString(dataDir)
		defer C.free(unsafe.Pointer(dir))
	}
	if !bool(C.libpostal_setup_datadir(dir)) || !bool(C.libpostal_setup_parser_datadir(dir)) {
		return nil, errors.New("libpostal model setup failed; prepare base and parser data before startup")
	}
	initialized = true
	setupDir = dataDir
	return native{}, nil
}
func (native) Parse(text string) ([]Component, error) {
	mutex.Lock()
	defer mutex.Unlock()
	input := C.CString(text)
	defer C.free(unsafe.Pointer(input))
	options := C.libpostal_get_address_parser_default_options()
	// Geographic domain is Kazakhstan; language remains auto-detected by parser.
	country := C.CString("kz")
	defer C.free(unsafe.Pointer(country))
	options.country = country
	response := C.libpostal_parse_address(input, options)
	if response == nil {
		return nil, errors.New("libpostal returned no parse")
	}
	defer C.libpostal_address_parser_response_destroy(response)
	out := make([]Component, 0, int(response.num_components))
	for i := 0; i < int(response.num_components); i++ {
		out = append(out, Component{Label: C.GoString(C.jana_label(response, C.size_t(i))), Value: C.GoString(C.jana_value(response, C.size_t(i)))})
	}
	return out, nil
}
