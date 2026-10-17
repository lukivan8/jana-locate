package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"example.com/jana-locate-cli/internal/api"
	"github.com/jackc/pgx/v5"
)

func TestReadyFilePublicationContinuesSameJournal(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	run, err := s.ImportRecords(ctx, []api.InputRecord{{Id: json.Number("9223372036854775807"), RawAddress: "raw one"}, {Id: "2", RawAddress: "raw two"}}, "http://normalizer.invalid")
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Rows(ctx, run.Id, 0, 100, false, true)
	rows[0].Result.Status = api.Matched
	if err = s.SaveBatch(ctx, run.Id, rows); err != nil {
		t.Fatal(err)
	}
	run, _ = s.Get(ctx, run.Id)
	output := filepath.Join(t.TempDir(), "result.json")
	progress := []int{}
	options := WriteOptions{ReadyOnly: true, Progress: func(done, total int) {
		if total != 1 {
			t.Fatalf("bad progress total %d", total)
		}
		progress = append(progress, done)
	}}
	partial, err := s.ExportFileWithOptions(ctx, run.Id, output, run.Revision, false, options)
	if err != nil || partial.Status != api.Review || partial.PublishedCount == nil || *partial.PublishedCount != 1 || partial.LastPublishedAt == nil {
		t.Fatalf("partial: %+v %v", partial, err)
	}
	b, _ := os.ReadFile(output)
	var file struct{ Results []api.Result }
	if err = json.Unmarshal(b, &file); err != nil || len(file.Results) != 1 {
		t.Fatalf("ready file: %s %v", b, err)
	}
	if len(progress) != 2 || progress[0] != 0 || progress[1] != 1 {
		t.Fatalf("progress: %+v", progress)
	}
	if _, err = s.ExportFileWithOptions(ctx, run.Id, output, partial.Revision, false, options); err == nil {
		t.Fatal("existing file overwritten")
	}
	rows, _, _ = s.Browse(ctx, run.Id, BrowseOptions{Limit: 20, ReviewOnly: true})
	if len(rows) != 1 {
		t.Fatal("remaining record lost")
	}
	if _, err = s.Review(ctx, run.Id, rows[0].Position, rows[0].Revision, fields(rows[0].Result)); err != nil {
		t.Fatal(err)
	}
	run, _ = s.Get(ctx, run.Id)
	complete, err := s.ExportFileWithOptions(ctx, run.Id, output, run.Revision, true, WriteOptions{ReadyOnly: true})
	if err != nil || complete.Status != api.Published || *complete.PublishedCount != 2 {
		t.Fatalf("completion: %+v %v", complete, err)
	}
	b, _ = os.ReadFile(output)
	if err = json.Unmarshal(b, &file); err != nil || len(file.Results) != 2 {
		t.Fatalf("incremental file: %s %v", b, err)
	}
	var raw map[string][]map[string]json.RawMessage
	json.Unmarshal(b, &raw)
	if string(raw["results"][0]["id"]) != "9223372036854775807" {
		t.Fatal("int64 input ID lost")
	}
}

func TestReadyPostgresPublicationPreservesPendingAndAvoidsDuplicates(t *testing.T) {
	dsn := os.Getenv("JANA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	schema := "partial_" + id()
	if _, err = conn.Exec(ctx, "CREATE SCHEMA "+qi(schema)); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(ctx, "DROP SCHEMA "+qi(schema)+" CASCADE")
	_, err = conn.Exec(ctx, "CREATE TABLE "+qi(schema, "source")+"(id bigint PRIMARY KEY,address text); INSERT INTO "+qi(schema, "source")+" VALUES(1,'one'),(2,'two'),(9223372036854775807,'three')")
	if err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	p := Postgres{DSN: dsn, APIURL: "http://normalizer.invalid"}
	run, err := p.Import(ctx, s, api.Source{Schema: schema, Table: "source", Key: "id", Column: "address", TargetSchema: schema, TargetTable: "result"})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Rows(ctx, run.Id, 0, 100, false, true)
	rows[0].Result.Status = api.Matched
	if err = s.SaveBatch(ctx, run.Id, rows); err != nil {
		t.Fatal(err)
	}
	run, _ = s.Get(ctx, run.Id)
	options := WriteOptions{ReadyOnly: true}
	first, err := p.PublishWithOptions(ctx, s, run.Id, run.Revision, options)
	if err != nil || first.Status != api.Review || first.PublishedCount == nil || *first.PublishedCount != 1 {
		t.Fatalf("first: %+v %v", first, err)
	}
	assertCount := func(want int) {
		t.Helper()
		var count int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+qi(schema, "result")+" WHERE run_id=$1", run.Id).Scan(&count); err != nil || count != want {
			t.Fatalf("count %d want %d: %v", count, want, err)
		}
	}
	assertCount(1)
	repeated, err := p.PublishWithOptions(ctx, s, run.Id, first.Revision, options)
	if err != nil {
		t.Fatal(err)
	}
	assertCount(1)
	if _, err = p.PublishWithOptions(ctx, s, run.Id, first.Revision, options); !errors.Is(err, ErrConflict) {
		t.Fatal("stale run accepted", err)
	}
	if _, err = s.Review(ctx, run.Id, rows[1].Position, rows[1].Revision, api.AddressFields{Apartment: ptr("77")}); err != nil {
		t.Fatal(err)
	}
	run, _ = s.Get(ctx, run.Id)
	second, err := p.PublishWithOptions(ctx, s, run.Id, run.Revision, options)
	if err != nil || second.Status != api.Review || *second.PublishedCount != 2 {
		t.Fatalf("second: %+v %v", second, err)
	}
	assertCount(2)
	if repeated.Id != second.Id {
		t.Fatal("publication created a different journal")
	}
	pending, total, err := s.Browse(ctx, run.Id, BrowseOptions{Limit: 20, ReviewOnly: true})
	if err != nil || total != 1 || pending[0].SourceId != "9223372036854775807" {
		t.Fatalf("pending: %+v %d %v", pending, total, err)
	}
	if _, err = s.Review(ctx, run.Id, pending[0].Position, pending[0].Revision, fields(pending[0].Result)); err != nil {
		t.Fatal(err)
	}
	conn.Exec(ctx, "UPDATE "+qi(schema, "source")+" SET address='changed' WHERE id=9223372036854775807")
	run, _ = s.Get(ctx, run.Id)
	if _, err = p.PublishWithOptions(ctx, s, run.Id, run.Revision, options); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("changed source accepted: %v", err)
	}
	assertCount(2)
	conn.Exec(ctx, "UPDATE "+qi(schema, "source")+" SET address='three' WHERE id=9223372036854775807")
	complete, err := p.PublishWithOptions(ctx, s, run.Id, run.Revision, options)
	if err != nil || complete.Status != api.Published || *complete.PublishedCount != 3 {
		t.Fatalf("complete: %+v %v", complete, err)
	}
	assertCount(3)
	var apartment, typ string
	if err = conn.QueryRow(ctx, "SELECT apartment,pg_typeof(source_id)::text FROM "+qi(schema, "result")+" WHERE source_id=2").Scan(&apartment, &typ); err != nil || apartment != "77" || typ != "bigint" {
		t.Fatal("correction or key type lost", apartment, typ, err)
	}
}
