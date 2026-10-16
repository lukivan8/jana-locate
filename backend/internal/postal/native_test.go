//go:build libpostal && cgo

package postal

import (
	"os"
	"testing"
)

func TestNativeParserOffline(t *testing.T) {
	path := os.Getenv("LIBPOSTAL_TEST_DATA")
	if path == "" {
		t.Skip("native model assets not configured; set LIBPOSTAL_TEST_DATA")
	}
	p, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := p.Parse("Алматы, улица Абая 12а, квартира 4")
	if err != nil || len(rows) == 0 {
		t.Fatalf("native parse: %v %+v", err, rows)
	}
	house := false
	for _, row := range rows {
		if row.Label == "house_number" && row.Value == "12а" {
			house = true
		}
	}
	if !house {
		t.Fatalf("house fraction/letter not extracted: %+v", rows)
	}
}
