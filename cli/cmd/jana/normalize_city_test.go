package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"example.com/jana-locate-cli/internal/api"
	"example.com/jana-locate-cli/internal/app"
	"github.com/gdamore/tcell/v2"
)

type cityTransport func(*http.Request) (*http.Response, error)

func (f cityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestConsoleSettlementChoiceAndStaleRevision(t *testing.T) {
	u, s, run := uiFixture(t)
	u.normalize = true
	u.cursor = 1
	n, _ := app.NewNormalizer(run.ApiUrl, "")
	u.normalizer = n
	n.HTTP.Transport = cityTransport(func(r *http.Request) (*http.Response, error) {
		var req api.RecordCitySelectionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Record.Id != "1" {
			t.Errorf("wrong source position: %v", req.Record.Id)
		}
		var body any
		if strings.HasSuffix(r.URL.Path, "/candidates") {
			body = api.RecordCityCandidates{ReferenceVersion: "fixture-v1", Candidates: []api.CityCandidate{{EntityId: "city-a", Name: "Алматы", Province: stringValue("Синтетическая область A")}, {EntityId: "city-b", Name: "Алматы", Province: stringValue("Синтетическая область B")}}}
		} else {
			body = api.Result{Id: req.Record.Id, RawAddress: req.Record.RawAddress, Province: stringValue("Синтетическая область B"), City: stringValue("Алматы"), Status: api.NeedsReview, Confidence: .55, Warnings: []string{"city_selected_by_operator", "missing_street"}}
		}
		data, _ := json.Marshal(body)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
	})
	editorKey(t, u, tcell.KeyRune, 'l')
	if u.mode != "city" || !strings.Contains(textUI(u), "Синтетическая область A") {
		t.Fatal("no settlement picker")
	}
	editorKey(t, u, tcell.KeyDown, 0)
	editorKey(t, u, tcell.KeyEnter, 0)
	if err := u.refresh(); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Rows(u.ctx, run.Id, 1, 1, false, false)
	if u.mode != "list" || rows[0].Reviewed || rows[0].CitySelection == nil || rows[0].CitySelection.EntityId != "city-b" || rows[0].Result.Id != json.Number("9223372036854775807") {
		t.Fatal("console selection not persisted correctly")
	}
	editorKey(t, u, tcell.KeyRune, 'l')
	other, _ := s.Rows(u.ctx, run.Id, 2, 1, false, false)
	if _, err := s.Review(u.ctx, run.Id, other[0].Position, other[0].Revision, api.AddressFields{}); err != nil {
		t.Fatal(err)
	}
	editorKey(t, u, tcell.KeyEnter, 0)
	if !u.failed || u.mode != "city" {
		t.Fatal("stale settlement selection silently applied")
	}
	editorKey(t, u, tcell.KeyEscape, 0)
	if u.mode != "list" {
		t.Fatal("picker did not close")
	}
}
