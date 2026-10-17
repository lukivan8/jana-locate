package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/pkg/api"
)

func TestJobHTTPContract(t *testing.T) {
	buildings, err := catalog.Open("../../data/catalog.seed.json", filepath.Join(t.TempDir(), "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := catalog.OpenJobs(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := NewWithJobs(buildings, jobs, []string{"http://localhost:3000"})
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://localhost:3000")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, body := range []string{`{"records":[{"id":1}]}`, `{"records":[{"id":1,"raw_address":null}]}`, `{"records":[]}`, `[]`} {
		if w := call("POST", "/v1/normalization-jobs", body); w.Code != 400 {
			t.Fatalf("invalid input accepted: %s %d", body, w.Code)
		}
	}
	w := call("POST", "/v1/normalization-jobs", `{"records":[{"id":9223372036854775807,"raw_address":"  exact\nraw  "},{"id":9223372036854775807,"raw_address":""}]}`)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var j api.NormalizationJob
	d := json.NewDecoder(w.Body)
	d.UseNumber()
	if err = d.Decode(&j); err != nil {
		t.Fatal(err)
	}
	_, _ = jobs.Claim()
	results := []api.Result{}
	for _, r := range j.Records {
		results = append(results, api.Result{Id: r.Id, RawAddress: r.RawAddress, Status: api.Unmatched, Warnings: []string{}})
	}
	if err = jobs.Finish(j.Id, results, ""); err != nil {
		t.Fatal(err)
	}
	j, _ = jobs.Get(j.Id)
	fields := `{"province":null,"city":null,"district":null,"living_area":null,"street_type":null,"street_name":null,"house":null,"apartment":"77","entrance":null,"floor":null,"landmark":null}`
	body := func(rev int) string {
		b, _ := json.Marshal(map[string]any{"expected_revision": rev, "fields": json.RawMessage(fields)})
		return string(b)
	}
	path := "/v1/normalization-jobs/" + j.Id
	if w = call("POST", path+"/records/1/review", body(j.Revision)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("POST", path+"/records/0/review", body(j.Revision)); w.Code != 409 {
		t.Fatal("stale write accepted", w.Code)
	}
	if w = call("GET", path, ""); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("9223372036854775807")) || !bytes.Contains(w.Body.Bytes(), []byte(`"apartment":"77"`)) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("DELETE", path, ""); w.Code != http.StatusNoContent {
		t.Fatal(w.Code)
	}
	if w = call("GET", path, ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w = call("POST", "/v1/normalize", `[{"id":"old-client","raw_address":"unknown"}]`); w.Code != 200 || w.Body.Bytes()[0] != '[' {
		t.Fatal("sync contract changed", w.Code, w.Body.String())
	}
}
