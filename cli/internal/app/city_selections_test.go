package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"example.com/jana-locate-cli/internal/api"
)

func TestLocalCitySelectionPersistenceIdentityAndConcurrency(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "journal.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.DB.Close() }()
	var beforeReturn func()
	var calls atomic.Int32
	var callbackMu sync.Mutex
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer synthetic-test" {
			t.Error("missing API token")
		}
		var req struct {
			Record           api.InputRecord `json:"record"`
			ReferenceVersion string          `json:"reference_version"`
			EntityID         string          `json:"entity_id"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Record.RawAddress != "  Караганда  " || req.Record.Id != "0" {
			t.Errorf("source changed or ID leaked: %+v", req.Record)
		}
		if r.URL.Path == "/v1/normalize/candidates" {
			json.NewEncoder(w).Encode(api.RecordCityCandidates{ReferenceVersion: "fixture-v1", Candidates: []api.CityCandidate{{EntityId: "village", Name: "Караганда"}, {EntityId: "city", Name: "Караганда"}}})
			return
		}
		if req.ReferenceVersion != "fixture-v1" {
			w.WriteHeader(409)
			return
		}
		if req.EntityID == "invalid" {
			w.WriteHeader(400)
			return
		}
		province := "Выбранная область"
		result := api.Result{Id: req.Record.Id, RawAddress: req.Record.RawAddress, City: &province, Province: &province, Status: api.NeedsReview, Warnings: []string{"city_selected_by_operator", "missing_street"}, Confidence: .6}
		callbackMu.Lock()
		callback := beforeReturn
		callbackMu.Unlock()
		if callback != nil {
			callback()
		}
		json.NewEncoder(w).Encode(result)
	}))
	defer remote.Close()
	n, _ := NewNormalizer(remote.URL, "synthetic-test")
	run, err := s.ImportRecords(ctx, []api.InputRecord{{Id: json.Number("9223372036854775807"), RawAddress: "  Караганда  "}, {Id: json.Number("9223372036854775807"), RawAddress: "Караганда"}}, n.URL)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Rows(ctx, run.Id, 0, 10, false, true)
	for i := range rows {
		rows[i].Result.Status = api.NeedsReview
	}
	if err = s.SaveBatch(ctx, run.Id, rows); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.Rows(ctx, run.Id, 0, 10, false, false)
	if _, err = s.Review(ctx, run.Id, 0, rows[0].Revision, api.AddressFields{}); err != nil {
		t.Fatal(err)
	}
	choices, err := s.CityCandidates(ctx, n, run.Id, 0)
	if err != nil || len(choices.Candidates) != 2 {
		t.Fatal(choices, err)
	}
	req := api.CitySelectionRequest{ExpectedRevision: choices.Revision, ReferenceVersion: choices.ReferenceVersion, EntityId: "city"}
	bad := req
	bad.ReferenceVersion = "old"
	if _, err = s.SelectCity(ctx, n, run.Id, 0, bad); err == nil {
		t.Fatal("stale reference accepted")
	}
	row, err := s.SelectCity(ctx, n, run.Id, 0, req)
	if err != nil {
		t.Fatal(err)
	}
	if row.Reviewed || row.CitySelection == nil || row.Result.Id != json.Number("9223372036854775807") || row.Result.RawAddress != "  Караганда  " || len(row.Result.Warnings) != 2 {
		t.Fatalf("selection lost identity/ambiguity: %+v", row)
	}
	previousCalls := calls.Load()
	if _, err = s.SelectCity(ctx, n, run.Id, 0, req); err != nil || calls.Load() != previousCalls+1 {
		t.Fatal("exact retry did not validate the reference or failed", err)
	}
	s.DB.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ = s.Rows(ctx, run.Id, 0, 10, false, false)
	if rows[0].CitySelection == nil || rows[0].CitySelection.EntityId != "city" || rows[1].CitySelection != nil {
		t.Fatal("selection lost on restart or leaked into duplicate ID")
	}
	latest, _ := s.Get(ctx, run.Id)
	req.ExpectedRevision = latest.Revision
	callbackMu.Lock()
	beforeReturn = func() {
		if _, err := s.Review(ctx, run.Id, 1, rows[1].Revision, api.AddressFields{}); err != nil {
			t.Fatal(err)
		}
	}
	callbackMu.Unlock()
	if _, err = s.SelectCity(ctx, n, run.Id, 0, req); !errors.Is(err, ErrConflict) {
		t.Fatal("concurrent edit overwritten", err)
	}
	rowsAfter, _ := s.Rows(ctx, run.Id, 0, 10, false, false)
	if rowsAfter[0].Revision != rows[0].Revision {
		t.Fatal("failed commit changed row")
	}
}
