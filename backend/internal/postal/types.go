// Package postal isolates the native libpostal dependency from API and domain types.
package postal

type Component struct{ Label, Value string }
type Parser interface {
	Parse(string) ([]Component, error)
}
