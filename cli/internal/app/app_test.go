package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"example.com/jana-locate-cli/internal/api"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "workspace.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	return s
}
func testImport(t *testing.T, s *Store, url string, rows []sourceRow) api.Run {
	t.Helper()
	r, err := s.Import(context.Background(), api.Source{Schema: "public", Table: "orders", Key: "id", Column: "address", KeyType: "bigint", TargetSchema: "public", TargetTable: "orders_jana_addresses"}, url, func(accept func(sourceRow) error) error {
		for _, r := range rows {
			if err := accept(r); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func ptr(s string) *string { return &s }
func fakeAPI(t *testing.T, fail *atomic.Bool, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		if r.URL.Path != "/v1/normalize" {
			t.Errorf("unexpected remote endpoint %s", r.URL.Path)
		}
		var body struct {
			Records []api.InputRecord `json:"records"`
		}
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if err := d.Decode(&body); err != nil {
			t.Error(err)
		}
		results := []api.Result{}
		for _, record := range body.Records {
			if record.Id == "9223372036854775807" {
				t.Error("source PK leaked to normalizer")
			}
			results = append(results, api.Result{Id: record.Id, RawAddress: record.RawAddress, Status: api.Unmatched, Warnings: []string{"coordinates_unknown"}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
}
func TestLocalReviewSurvivesRestartAndPreservesSource(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	fail, calls := &atomic.Bool{}, &atomic.Int32{}
	remote := fakeAPI(t, fail, calls)
	defer remote.Close()
	n, _ := NewNormalizer(remote.URL, "")
	r := testImport(t, s, n.URL, []sourceRow{{"9223372036854775807", ptr("  raw\nstring  ")}, {"2", nil}, {"3", ptr("")}, {"4", ptr("  raw\nstring  ")}})
	rows, _ := s.Rows(ctx, r.Id, 0, 100, false, true)
	parsed, err := n.Batch(ctx, rows)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveBatch(ctx, r.Id, parsed); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.Rows(ctx, r.Id, 0, 50, false, false)
	original := rows[0]
	first, err := s.Review(ctx, r.Id, 0, original.Revision, api.AddressFields{Apartment: ptr("77"), Entrance: ptr("3")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Review(ctx, r.Id, 0, original.Revision, api.AddressFields{Apartment: ptr("stale")}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale edit accepted", err)
	}
	if _, err = s.Review(ctx, r.Id, 3, rows[3].Revision, api.AddressFields{Landmark: ptr("other record")}); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Rows(ctx, r.Id, 0, 50, false, false)
	if *after[0].Result.Apartment != "77" || after[0].SourceId != "9223372036854775807" || *after[0].RawAddress != "  raw\nstring  " || after[1].RawAddress != nil || after[1].Result.Warnings[0] != "source_address_null" || after[0].Result.Id != "9223372036854775807" {
		t.Fatalf("source or corrections lost: %+v", after)
	}
	if first.Result.Coordinates != nil || first.Result.Confidence != original.Result.Confidence {
		t.Fatal("review invented coordinates or confidence")
	}
	var path string
	if err = s.DB.QueryRow("PRAGMA database_list").Scan(new(int), new(string), &path); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	restored, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.DB.Close()
	saved, _ := restored.Rows(ctx, r.Id, 0, 50, false, false)
	if *saved[0].Result.Entrance != "3" || !saved[0].Reviewed {
		t.Fatal("restart lost saved review")
	}
}
func TestNormalizerFailureLeavesCheckpointAndCanResume(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	fail, calls := &atomic.Bool{}, &atomic.Int32{}
	remote := fakeAPI(t, fail, calls)
	defer remote.Close()
	n, _ := NewNormalizer(remote.URL, "")
	input := []sourceRow{}
	for i := 0; i < 101; i++ {
		input = append(input, sourceRow{string(rune('a' + i)), ptr("same raw")})
	}
	r := testImport(t, s, n.URL, input)
	rows, _ := s.Rows(ctx, r.Id, 0, 100, false, true)
	parsed, err := n.Batch(ctx, rows)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveBatch(ctx, r.Id, parsed); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); Work(workerCtx, s, n) }()
	waitStatus(t, s, r.Id, api.Failed)
	cancel()
	<-done
	saved, _ := s.Get(ctx, r.Id)
	if saved.Processed != 100 {
		t.Fatal("saved checkpoint lost")
	}
	fail.Store(false)
	saved, err = s.Status(ctx, r.Id, saved.Revision, api.Normalizing, "")
	if err != nil {
		t.Fatal(err)
	}
	workerCtx, cancel = context.WithCancel(ctx)
	done = make(chan struct{})
	go func() { defer close(done); Work(workerCtx, s, n) }()
	waitStatus(t, s, r.Id, api.Review)
	cancel()
	<-done
	if calls.Load() != 3 {
		t.Fatal("already checkpointed rows were sent again", calls.Load())
	}
}
func waitStatus(t *testing.T, s *Store, id string, status api.RunStatus) {
	t.Helper()
	until := time.After(5 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		r, err := s.Get(context.Background(), id)
		if err == nil && r.Status == status {
			return
		}
		select {
		case <-until:
			t.Fatalf("did not reach %s", status)
		case <-tick.C:
		}
	}
}
func TestBadRemoteMappingAndLocalHTTPProtection(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":"wrong","raw_address":"raw","status":"unmatched","warnings":[]}]}`))
	}))
	defer remote.Close()
	n, _ := NewNormalizer(remote.URL, "")
	if _, err := n.Batch(context.Background(), []api.Row{{Position: 0, SourceId: "source", RawAddress: ptr("raw")}}); err == nil {
		t.Fatal("wrong remote ID accepted")
	}
	for _, u := range []string{"file:///tmp/file", "http://user:password@localhost", "http://localhost?token=secret"} {
		if _, err := NewNormalizer(u, ""); err == nil {
			t.Fatal("unsafe API URL accepted")
		}
	}
	s := testStore(t)
	h := Server{Store: s, Token: "synthetic-web-token-123", Network: true}.Handler()
	req := httptest.NewRequest("GET", "http://localhost/v1/runs", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal("workspace unauthenticated")
	}
	req = httptest.NewRequest("POST", "http://localhost/v1/runs", strings.NewReader(`{}`))
	req.SetBasicAuth("jana", "synthetic-web-token-123")
	req.Header.Set("Origin", "https://unrelated.example")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("foreign origin accepted")
	}
	req = httptest.NewRequest("GET", "http://localhost/v1/runs", nil)
	req.SetBasicAuth("jana", "synthetic-web-token-123")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("authenticated read failed")
	}
}

func TestBrowserInputUsesLocalSnapshotAndSharedSPA(t *testing.T) {
	s := testStore(t)
	h := Server{Store: s, Postgres: Postgres{APIURL: "http://normalizer:8080"}}.Handler()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	w := request("POST", "/v1/runs/input", `{"records":[{"id":"9223372036854775807","raw_address":"  raw\naddress  "},{"id":"9223372036854775807","raw_address":""}]}`)
	if w.Code != 200 {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	var run api.Run
	if err := json.Unmarshal(w.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if run.Total != 2 || !IsFile(run.Source) || run.ApiUrl != "http://normalizer:8080" {
		t.Fatal("wrong local snapshot", run)
	}
	rows, err := s.Rows(context.Background(), run.Id, 0, 50, false, false)
	if err != nil || len(rows) != 2 || *rows[0].RawAddress != "  raw\naddress  " || *rows[1].RawAddress != "" || *rows[0].InputId != "9223372036854775807" || *rows[1].InputId != *rows[0].InputId {
		t.Fatal("input changed", rows, err)
	}
	for _, body := range []string{`{"records":[]}`, `{"records":[{"id":"missing-address"}]}`, `{"records":[{"id":true,"raw_address":"x"}]}`, `{"records":[{"id":1.5,"raw_address":"x"}]}`, `{"records":[{"id":"x","raw_address":"x"}],"dsn":"private"}`} {
		if w := request("POST", "/v1/runs/input", body); w.Code == 200 {
			t.Fatal("invalid input accepted", body)
		}
	}
	shell := request("GET", "/?run="+run.Id, "")
	if shell.Code != 200 || !strings.Contains(shell.Body.String(), "/assets/index-") || !strings.Contains(shell.Body.String(), "/config.js") {
		t.Fatal("shared SPA not served")
	}
	if strings.Contains(shell.Header().Get("Content-Security-Policy"), "unsafe-inline") || !strings.Contains(shell.Header().Get("Content-Security-Policy"), "sha256-") {
		t.Fatal("SPA scripts must use hashes, not relaxed CSP")
	}
	config := request("GET", "/config.js", "")
	if !strings.Contains(config.Body.String(), `workspace: "local"`) || strings.Contains(config.Body.String(), "normalizer:8080") {
		t.Fatal("local browser config exposed remote API")
	}
}

func TestLoopbackWorkspaceBehindExplicitProxyOrigin(t *testing.T) {
	s := testStore(t)
	token := "synthetic-web-token-123"
	origin := "http://proxy.example:8095"
	h := Server{Store: s, Token: token, PublicOrigin: origin}.Handler()
	check := func(handler http.Handler, method, target, requestOrigin string, authenticated bool, want int) {
		t.Helper()
		req := httptest.NewRequest(method, target, strings.NewReader(`{"expected_revision":1}`))
		req.Header.Set("Content-Type", "application/json")
		if authenticated {
			req.SetBasicAuth("jana", token)
		}
		if requestOrigin != "" {
			req.Header.Set("Origin", requestOrigin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s origin %s: %d, want %d; %s", method, target, requestOrigin, w.Code, want, w.Body.String())
		}
	}
	check(h, "GET", origin+"/v1/runs", "", false, 401)
	check(h, "GET", origin+"/v1/runs", "", true, 200)
	check(h, "GET", "http://unrelated.example:8095/v1/runs", "", true, 403)
	check(h, "POST", origin+"/v1/runs/nonexistent/resume", "http://unrelated.example", true, 403)
	check(h, "POST", origin+"/v1/runs/nonexistent/resume", origin, true, 404)
	check(Server{Store: s, PublicOrigin: origin}.Handler(), "GET", origin+"/v1/runs", "", false, 403)
	run := testImport(t, s, "http://normalizer.invalid", []sourceRow{{"1", ptr("synthetic proxy address")}})
	rows, _ := s.Rows(context.Background(), run.Id, 0, 1, false, true)
	if err := s.SaveBatch(context.Background(), run.Id, rows); err != nil {
		t.Fatal(err)
	}
	body := `{"expected_revision":1,"fields":{"apartment":"42"}}`
	req := httptest.NewRequest("POST", origin+"/v1/runs/"+run.Id+"/rows/0/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	req.SetBasicAuth("jana", token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("proxied save: %d %s", w.Code, w.Body.String())
	}
	rows, _ = s.Rows(context.Background(), run.Id, 0, 1, false, false)
	if rows[0].Result.Apartment == nil || *rows[0].Result.Apartment != "42" || !rows[0].Reviewed {
		t.Fatal("proxied edit did not persist")
	}

}
