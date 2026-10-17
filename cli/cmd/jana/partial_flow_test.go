package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"example.com/jana-locate-cli/internal/api"
	"example.com/jana-locate-cli/internal/app"
	"github.com/jackc/pgx/v5"
)

func TestNormalizeContinuesOnlyTheSamePartiallyPublishedSource(t *testing.T) {
	dsn := os.Getenv("JANA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	var calls atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input struct{ Records []api.InputRecord }
		json.NewDecoder(r.Body).Decode(&input)
		results := []api.Result{}
		for _, record := range input.Records {
			status := api.NeedsReview
			if record.Id == "0" {
				status = api.Matched
			}
			results = append(results, api.Result{Id: record.Id, RawAddress: record.RawAddress, Status: status, Warnings: []string{}})
		}
		json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	defer remote.Close()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	schema := "flow_partial_" + strings.ReplaceAll(filepath.Base(t.TempDir()), "-", "_")
	// pg identifiers are supplied only by this synthetic fixture and quoted.
	q := pgx.Identifier{schema}.Sanitize()
	if _, err = conn.Exec(ctx, "CREATE SCHEMA "+q); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(ctx, "DROP SCHEMA "+q+" CASCADE")
	if _, err = conn.Exec(ctx, "CREATE TABLE "+q+".source(id bigint PRIMARY KEY,address text);INSERT INTO "+q+".source VALUES(1,'synthetic one'),(2,'synthetic two')"); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(t.TempDir(), "journal.sqlite")
	s, err := app.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	p := app.Postgres{DSN: dsn, APIURL: remote.URL}
	run, err := p.Import(ctx, s, api.Source{Schema: schema, Table: "source", Key: "id", Column: "address", TargetSchema: schema, TargetTable: "normalized"})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Rows(ctx, run.Id, 0, 100, false, true)
	for i := range rows {
		rows[i].Result.Status = api.NeedsReview
	}
	rows[0].Result.Status = api.Matched
	if err = s.SaveBatch(ctx, run.Id, rows); err != nil {
		t.Fatal(err)
	}
	run, _ = s.Get(ctx, run.Id)
	partial, err := p.PublishWithOptions(ctx, s, run.Id, run.Revision, app.WriteOptions{ReadyOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = writeProfile(filepath.Join(filepath.Dir(state), "connection.json"), profile{DatabaseURL: dsn, Source: partial.Source, APIURL: remote.URL, VerifiedAt: "2026-10-17T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	args := []string{"--state", state, "--listen", "127.0.0.1:0", "--no-interactive", "--quiet"}
	out, log, err := cliProcess(t, "", append([]string{"normalize"}, args...)...)
	if err != nil {
		t.Fatalf("resume: %v %s", err, log)
	}
	var resumed api.Run
	if err = json.Unmarshal(out, &resumed); err != nil || resumed.Id != run.Id || calls.Load() != 0 {
		t.Fatalf("active journal was re-normalized: %s calls=%d %v", out, calls.Load(), err)
	}
	_, log, err = cliProcess(t, "", []string{"publish", "--state", state, "--target-schema", schema, "--target-table", "normalized", "--no-interactive"}...)
	if err != nil || !strings.Contains(string(log), "#2") || strings.Contains(string(log), "#1 ") {
		t.Fatalf("remaining queue: %v %s", err, log)
	}
	out, log, err = cliProcess(t, "", append([]string{"normalize", "--new"}, args...)...)
	var fresh api.Run
	json.Unmarshal(out, &fresh)
	if err != nil || fresh.Id == run.Id || fresh.Status != api.Review || calls.Load() != 1 {
		t.Fatalf("explicit new snapshot failed: %v %s %s", err, out, log)
	}
}
