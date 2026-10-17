package app

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/jana-locate-cli/internal/api"
)

func TestSSEFollowsPersistedJournalAcrossConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	other, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.DB.Close()
	server := httptest.NewServer(Server{Store: s, AuthDisabled: true}.Handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	connect := func() *http.Response {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/v1/events", nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
			t.Fatalf("not SSE: %d", res.StatusCode)
		}
		return res
	}
	read := func(scanner *bufio.Scanner) api.RunList {
		t.Helper()
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				var v api.RunList
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v); err != nil {
					t.Fatal(err)
				}
				return v
			}
		}
		t.Fatalf("stream ended: %v", scanner.Err())
		return api.RunList{}
	}
	res := connect()
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	if len(read(scanner).Runs) != 0 {
		t.Fatal("initial state not empty")
	}
	run := testImport(t, other, "http://normalizer.invalid", []sourceRow{{"1", ptr("synthetic")}})
	snapshot := read(scanner)
	if len(snapshot.Runs) != 1 || snapshot.Runs[0].Id != run.Id || snapshot.Runs[0].Total != 1 {
		t.Fatal("new CLI run missing")
	}
	rows, _ := other.Rows(ctx, run.Id, 0, 1, false, true)
	if err = other.SaveBatch(ctx, run.Id, rows); err != nil {
		t.Fatal(err)
	}
	snapshot = read(scanner)
	if snapshot.Runs[0].Processed != 1 || snapshot.Runs[0].Status != api.Review {
		t.Fatal("checkpoint missing")
	}
	if _, err = other.Review(ctx, run.Id, 0, 1, api.AddressFields{Apartment: ptr("42")}); err != nil {
		t.Fatal(err)
	}
	snapshot = read(scanner)
	if snapshot.Runs[0].Reviewed != 1 {
		t.Fatal("correction missing")
	}
	res.Body.Close()
	res = connect()
	defer res.Body.Close()
	snapshot = read(bufio.NewScanner(res.Body))
	if snapshot.Runs[0].Reviewed != 1 {
		t.Fatal("reconnect lost state")
	}
}
func TestExplicitPasswordFreeProxyPreservesHostAndOriginChecks(t *testing.T) {
	s := testStore(t)
	origin := "http://proxy.example:8095"
	h := Server{Store: s, PublicOrigin: origin, AuthDisabled: true}.Handler()
	for _, c := range []struct {
		method, target, origin string
		want                   int
	}{{"GET", origin + "/v1/runs", "", 200}, {"GET", "http://other.example/v1/runs", "", 403}, {"POST", origin + "/v1/runs/nonexistent/resume", "http://other.example", 403}} {
		req := httptest.NewRequest(c.method, c.target, strings.NewReader(`{}`))
		req.Header.Set("Origin", c.origin)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != c.want {
			t.Fatalf("%s %s: %d", c.method, c.target, w.Code)
		}
		if w.Header().Get("WWW-Authenticate") != "" {
			t.Fatal("password prompt still enabled")
		}
	}
}
