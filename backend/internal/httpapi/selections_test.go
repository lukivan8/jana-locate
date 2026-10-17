package httpapi

import (
	"bytes"
	"encoding/json"
	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/normalize"
	"github.com/lukivan8/jana-address/internal/testutil"
	"github.com/lukivan8/jana-address/pkg/api"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestJobCitySelectionContractAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	jobs, err := catalog.OpenJobs(path)
	if err != nil {
		t.Fatal(err)
	}
	buildings, err := catalog.Open("../../data/catalog.seed.json", filepath.Join(t.TempDir(), "overlay.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine := normalize.Engine{Catalog: buildings, Reference: testutil.Reference(t)}
	h := NewWithJobs(buildings, jobs, nil, engine)
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	decodeJob := func(w *httptest.ResponseRecorder) api.NormalizationJob {
		t.Helper()
		if w.Code != 200 && w.Code != 202 {
			t.Fatal(w.Code, w.Body.String())
		}
		var j api.NormalizationJob
		d := json.NewDecoder(w.Body)
		d.UseNumber()
		if err := d.Decode(&j); err != nil {
			t.Fatal(err)
		}
		return j
	}
	records := []api.InputRecord{{Id: json.Number("9223372036854775807"), RawAddress: "  Караганда, улица Алалыкина 999999  "}, {Id: json.Number("9223372036854775807"), RawAddress: "Караганда"}}
	j := decodeJob(call("POST", "/v1/normalization-jobs", records))
	if _, err = jobs.Claim(); err != nil {
		t.Fatal(err)
	}
	if err = jobs.Finish(j.Id, engine.NormalizeBatch(records), ""); err != nil {
		t.Fatal(err)
	}
	j, _ = jobs.Get(j.Id)
	base := "/v1/normalization-jobs/" + j.Id
	w := call("GET", base+"/records/0/candidates", nil)
	var c api.CityCandidates
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err = json.Unmarshal(w.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Candidates) != 2 || c.Revision != j.Revision || c.ReferenceVersion == "" {
		t.Fatalf("candidates: %+v", c)
	}
	req := api.CitySelectionRequest{ExpectedRevision: j.Revision, EntityId: "c1", ReferenceVersion: c.ReferenceVersion}
	bad := req
	bad.ReferenceVersion = "old"
	if w = call("POST", base+"/records/0/selections", bad); w.Code != 409 {
		t.Fatal("stale snapshot", w.Code)
	}
	bad = req
	bad.EntityId = "other"
	if w = call("POST", base+"/records/0/selections", bad); w.Code != 400 {
		t.Fatal("invalid entity", w.Code)
	}
	selected := decodeJob(call("POST", base+"/records/0/selections", req))
	if selected.Revision != j.Revision+1 || selected.Reviewed[0] || selected.ReviewedCount != 0 || catalog.Text(selected.Results[0].Province) != "Карагандинская область" || selected.Results[0].Coordinates != nil || selected.Results[0].RawAddress != records[0].RawAddress || selected.Results[0].Id != records[0].Id || selected.Results[1].Province != nil {
		t.Fatalf("selection: %+v", selected)
	}
	retry := decodeJob(call("POST", base+"/records/0/selections", req))
	if retry.Revision != selected.Revision {
		t.Fatal("retry changed revision")
	}
	if w = call("POST", base+"/records/1/selections", req); w.Code != 409 {
		t.Fatal("stale revision", w.Code)
	}
	if w = call("GET", base+"/records/2/candidates", nil); w.Code != 404 {
		t.Fatal("out of bounds", w.Code)
	}
	reopened, err := catalog.OpenJobs(path)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := reopened.Get(j.Id)
	if err != nil || restored.CitySelections == nil || len(*restored.CitySelections) != 1 || catalog.Text(restored.Results[0].Province) != "Карагандинская область" {
		t.Fatalf("restore: %+v %v", restored, err)
	}
	// Manual review followed by another city choice must reopen the record and discard stale dependent fields.
	district := "Старый район"
	fields := api.AddressFields{City: restored.Results[0].City, Province: restored.Results[0].Province, District: &district}
	reviewed, err := jobs.Review(j.Id, 0, restored.Revision, fields, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.ExpectedRevision = reviewed.Revision
	req.EntityId = "c2"
	changed := decodeJob(call("POST", base+"/records/0/selections", req))
	if changed.ReviewedCount != 0 || changed.Results[0].District != nil || catalog.Text(changed.Results[0].Province) != "Актюбинская область" {
		t.Fatalf("stale manual state: %+v", changed)
	}
	// A subsequent explicit manual province edit must not restore a stale checked identity.
	fields.Province = selected.Results[0].Province
	manual, err := jobs.Review(j.Id, 0, changed.Revision, fields, nil)
	if err != nil || manual.CitySelections != nil {
		t.Fatalf("stale administrative choice survived: %+v %v", manual.CitySelections, err)
	}
	// Existing synchronous object/array contracts remain synchronous, with no city constraint leak.
	for _, body := range []any{records, map[string]any{"records": records}} {
		w = call("POST", "/v1/normalize", body)
		if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte(`"province":"Карагандинская область"`)) {
			t.Fatal("sync changed", w.Code, w.Body.String())
		}
	}
}
