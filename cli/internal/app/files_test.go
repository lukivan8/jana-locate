package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"example.com/jana-locate-cli/internal/api"
)

func TestFileSnapshotDuplicateIDsAndExactExport(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	dir := t.TempDir()
	input := filepath.Join(dir, "addresses.json")
	original := `{"records":[{"id":9223372036854775807,"raw_address":"  a\nб  "},{"id":9223372036854775807,"raw_address":"second"}]}`
	if err := os.WriteFile(input, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := s.ImportFile(ctx, input, "auto", "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	remote := fakeAPI(t, &atomic.Bool{}, &atomic.Int32{})
	defer remote.Close()
	n, _ := NewNormalizer(remote.URL, "")
	rows, _ := s.Rows(ctx, r.Id, 0, 50, false, true)
	parsed, err := n.Batch(ctx, rows)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveBatch(ctx, r.Id, parsed); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.Rows(ctx, r.Id, 0, 50, false, false)
	if rows[0].SourceId == rows[1].SourceId {
		t.Fatal("duplicate IDs collapsed")
	}
	for _, row := range rows {
		if row.Result.Id != json.Number("9223372036854775807") {
			t.Fatalf("ID lost: %#v", row.Result.Id)
		}
	}
	_, err = s.Review(ctx, r.Id, 0, rows[0].Revision, api.AddressFields{Apartment: ptr("12")})
	if err != nil {
		t.Fatal(err)
	}
	r, _ = s.Get(ctx, r.Id)
	output := filepath.Join(dir, "output.json")
	if _, err = s.ExportFile(ctx, r.Id, output, r.Revision-1, false); err != ErrConflict {
		t.Fatalf("stale publication: %v", err)
	}
	r, err = s.ExportFile(ctx, r.Id, output, r.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(output)
	if strings.Count(string(b), `9223372036854775807`) != 2 || !strings.Contains(string(b), `"apartment": "12"`) {
		t.Fatalf("bad output %s", b)
	}
	if _, err = s.ExportFile(ctx, r.Id, output, r.Revision, false); err == nil {
		t.Fatal("overwrote output without consent")
	}
	unchanged, _ := os.ReadFile(input)
	if string(unchanged) != original {
		t.Fatal("input changed")
	}
}
func TestLinesPreserveWhitespaceAndRejectInvalidInput(t *testing.T) {
	s := testStore(t)
	path := filepath.Join(t.TempDir(), "lines.txt")
	os.WriteFile(path, []byte("  first  \r\n\nlast\n"), 0600)
	r, err := s.ImportFile(context.Background(), path, "auto", "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Rows(context.Background(), r.Id, 0, 50, false, true)
	if len(rows) != 3 || *rows[0].RawAddress != "  first  " || *rows[1].RawAddress != "" || *rows[2].RawAddress != "last" {
		t.Fatalf("lines lost %#v", rows)
	}
	for _, input := range []string{`[{"id":1.5,"raw_address":"a"}]`, `[{"id":true,"raw_address":"a"}]`, `[{"id":1,"raw_address":null}]`, `["a"] false`} {
		os.WriteFile(path, []byte(input), 0600)
		if _, err = s.ImportFile(context.Background(), path, "json", "http://localhost:8080"); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}
