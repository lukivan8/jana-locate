package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"example.com/jana-locate-cli/internal/api"
)

func TestNormalizationCompletionSurvivesReviewResumeAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "journal.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.DB.Close() }()
	run := testImport(t, s, "http://localhost:8080", []sourceRow{{"1", ptr("synthetic one")}, {"2", ptr("synthetic two")}})
	rows, err := s.Rows(ctx, run.Id, 0, 100, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveBatch(ctx, run.Id, rows[:1]); err != nil {
		t.Fatal(err)
	}
	partial, _ := s.Get(ctx, run.Id)
	if partial.NormalizationCompletedAt != nil {
		t.Fatal("partial batch has a completion time")
	}
	if _, err = s.Status(ctx, run.Id, 0, api.Failed, "synthetic interruption"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Status(ctx, run.Id, 0, api.Normalizing, ""); err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	if err = s.SaveBatch(ctx, run.Id, rows[1:]); err != nil {
		t.Fatal(err)
	}
	completed, _ := s.Get(ctx, run.Id)
	if completed.Status != api.Review || completed.NormalizationCompletedAt == nil || completed.CreatedAt != run.CreatedAt {
		t.Fatalf("completion not recorded: %+v", completed)
	}
	stamp := *completed.NormalizationCompletedAt
	finished, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil || finished.Before(before) || finished.After(time.Now()) {
		t.Fatalf("invalid completion timestamp %q: %v", stamp, err)
	}
	if _, err = s.Review(ctx, run.Id, 0, rows[0].Revision, api.AddressFields{Apartment: ptr("99")}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Status(ctx, run.Id, 0, api.Published, ""); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := s.Get(ctx, run.Id)
	if err != nil || restored.NormalizationCompletedAt == nil || *restored.NormalizationCompletedAt != stamp {
		t.Fatalf("timing changed after review/publication/restart: %+v %v", restored, err)
	}
	// Old journal metadata remains readable without fabricating a measurement.
	if _, err = s.DB.ExecContext(ctx, "UPDATE runs SET metadata=json_remove(metadata,'$.normalization_completed_at') WHERE id=?", run.Id); err != nil {
		t.Fatal(err)
	}
	legacy, err := s.Get(ctx, run.Id)
	if err != nil || legacy.NormalizationCompletedAt != nil {
		t.Fatalf("legacy run acquired an invented measurement: %+v %v", legacy, err)
	}
}
