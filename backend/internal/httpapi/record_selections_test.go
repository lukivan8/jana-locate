package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/normalize"
	"github.com/lukivan8/jana-address/internal/testutil"
	"github.com/lukivan8/jana-address/pkg/api"
)

func TestStatelessCitySelectionUsesJobEngine(t *testing.T) {
	buildings, err := catalog.Open("../../data/catalog.seed.json", filepath.Join(t.TempDir(), "overlay.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine := normalize.Engine{Catalog: buildings, Reference: testutil.Reference(t)}
	handler := New(buildings, nil, engine)
	call := func(path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	record := api.InputRecord{Id: json.Number("9223372036854775807"), RawAddress: "  Караганда, улица Алалыкина 999999  "}
	w := call("/v1/normalize/candidates", map[string]any{"record": record})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var choices api.RecordCityCandidates
	if err = json.Unmarshal(w.Body.Bytes(), &choices); err != nil {
		t.Fatal(err)
	}
	if len(choices.Candidates) != 2 || choices.ReferenceVersion != engine.Reference.Version() {
		t.Fatalf("choices: %+v", choices)
	}
	for _, entity := range []string{"c1", "c2"} {
		req := api.RecordCitySelectionRequest{Record: record, EntityId: entity, ReferenceVersion: choices.ReferenceVersion}
		w = call("/v1/normalize/selections", req)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var result api.Result
		d := json.NewDecoder(w.Body)
		d.UseNumber()
		if err = d.Decode(&result); err != nil {
			t.Fatal(err)
		}
		expected, err := engine.SelectCity(context.Background(), record, entity)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(result.Confidence-expected.Confidence) > 1e-12 {
			t.Fatalf("confidence differs: %v vs %v", result.Confidence, expected.Confidence)
		}
		result.Confidence = expected.Confidence
		got, _ := json.Marshal(result)
		want, _ := json.Marshal(expected)
		if !bytes.Equal(got, want) {
			t.Fatalf("different engine outcome: %s vs %s", got, want)
		}
		req.ReferenceVersion = "obsolete"
		if w = call("/v1/normalize/selections", req); w.Code != 409 {
			t.Fatal("stale reference accepted", w.Code)
		}
	}
	if w = call("/v1/normalize/selections", api.RecordCitySelectionRequest{Record: record, EntityId: "other", ReferenceVersion: choices.ReferenceVersion}); w.Code != 400 {
		t.Fatal("incompatible choice accepted")
	}
	for _, body := range []any{map[string]any{}, map[string]any{"record": map[string]any{"id": "x", "raw_address": nil}}, map[string]any{"record": map[string]any{"id": 1.5, "raw_address": "Караганда"}}} {
		if w = call("/v1/normalize/candidates", body); w.Code != 400 {
			t.Fatal("invalid input accepted", w.Code)
		}
	}
	// A stateless selection must not influence subsequent normalization.
	if engine.Normalize(record).Province != nil {
		t.Fatal("selection leaked across calls")
	}
}
