package normalize

import (
	"context"
	"encoding/json"
	"github.com/lukivan8/jana-address/pkg/api"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContextualPreservesEvidenceAndInput(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":"a","raw_address":"Қарағанды, Алалыкин көш 12","city":"Қарағанды","street_name":"Алалыкин","house":"12","confidence":null,"status":"experimental_candidate","warnings":[],"canonical_candidates":{"city":[]},"spans":[{"label":"house","start":25,"end":27,"text":"12"}]}]}`))
	}))
	defer s.Close()
	e := Engine{Contextual: &ContextualClient{URL: s.URL}}
	got := e.NormalizeBatchContext(context.Background(), []api.InputRecord{{Id: "a", RawAddress: "Қарағанды, Алалыкин көш 12"}})[0]
	if got.House == nil || *got.House != "12" || got.NormalizationDetails == nil || (*got.NormalizationDetails)["spans"] == nil || got.ConfidenceAvailable == nil || *got.ConfidenceAvailable || got.Status != api.NeedsReview {
		t.Fatalf("lost model evidence: %+v", got)
	}
	if got.NormalizerVersion == nil {
		t.Fatal("missing provenance")
	}
}
func TestContextualRejectsWrongIdentity(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"id": "other", "raw_address": "x"}}})
	}))
	defer s.Close()
	e := Engine{Contextual: &ContextualClient{URL: s.URL}}
	if !UnavailableResults(e.NormalizeBatchContext(context.Background(), []api.InputRecord{{Id: "a", RawAddress: "x"}})) {
		t.Fatal("accepted wrong record")
	}
}
func TestContextualReadinessAndFailure(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unavailable", 503) }))
	defer s.Close()
	e := Engine{Contextual: &ContextualClient{URL: s.URL}}
	if e.Ready(context.Background()) == nil {
		t.Fatal("accepted unhealthy service")
	}
	if !UnavailableResults(e.NormalizeBatchContext(context.Background(), []api.InputRecord{{Id: json.Number("9007199254740993"), RawAddress: "x"}})) {
		t.Fatal("masked failure")
	}
}

func TestCompletedJobCandidatesNeedNoNewInference(t *testing.T) {
	e := Engine{Contextual: &ContextualClient{URL: "http://127.0.0.1:1"}}
	details := map[string]interface{}{"canonical_candidates": map[string]interface{}{"city": []interface{}{map[string]interface{}{"id": "one", "name": "Караганда"}, map[string]interface{}{"id": "two", "name": "Караганда"}}}}
	version := ContextualVersion
	result := api.Result{NormalizerVersion: &version, NormalizationDetails: &details}
	candidates, available := e.CityCandidatesFromResult(result)
	if !available || len(candidates) != 2 {
		t.Fatal("lost saved ambiguity")
	}
	old := "old-version"
	result.NormalizerVersion = &old
	if _, available = e.CityCandidatesFromResult(result); available {
		t.Fatal("accepted evidence from another version")
	}
}
