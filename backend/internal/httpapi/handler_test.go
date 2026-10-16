package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/pkg/api"
	"github.com/lukivan8/jana-address/pkg/client"
)

func server(t *testing.T) *httptest.Server {
	t.Helper()
	s, err := catalog.Open("../../data/catalog.seed.json", filepath.Join(t.TempDir(), "overlay.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(s, []string{"http://localhost:3000"}))
	t.Cleanup(srv.Close)
	return srv
}
func TestSDKEndToEnd(t *testing.T) {
	srv := server(t)
	sdk := client.New(srv.URL)
	ctx := context.Background()
	out, err := sdk.Normalize(ctx, api.NormalizeRequest{Records: []api.InputRecord{{Id: "a", RawAddress: "Туран 55/6 кв 1"}, {Id: 9007199254740993, RawAddress: ""}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 2 || out.Results[1].Id.(json.Number).String() != "9007199254740993" {
		t.Fatal("records or IDs not preserved")
	}
	cat, err := sdk.Catalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entry := cat.Entries[0]
	saved, err := sdk.Confirm(ctx, api.ConfirmationRequest{Fields: entry.Fields, Coordinates: entry.Coordinates, ExpectedRevision: entry.Revision})
	if err != nil || !saved.Confirmed {
		t.Fatalf("confirm: %v", err)
	}
	_, err = sdk.Confirm(ctx, api.ConfirmationRequest{Fields: entry.Fields, Coordinates: entry.Coordinates, ExpectedRevision: entry.Revision})
	if httpErr, ok := err.(*client.HTTPError); !ok || httpErr.Status != 409 {
		t.Fatalf("expected typed conflict: %v", err)
	}
}
func TestValidationAndNullWireFormat(t *testing.T) {
	srv := server(t)
	cases := []struct {
		body   string
		status int
	}{
		{`{"records":[{"id":"a","raw_address":""}]}`, 200},
		{`{"records":[]}`, 400}, {`{"records":[{"id":null,"raw_address":"a"}]}`, 400},
		{`{"records":[{"id":"a"}]}`, 400}, {`{"records":[{"id":"a","raw_address":null}]}`, 400},
		{`{"records":[{"id":1.2,"raw_address":"a"}]}`, 400}, {`{"records":[{"id":"a","raw_address":"x","extra":1}]}`, 400},
		{`{"records":[]} {}`, 400},
	}
	for _, c := range cases {
		res, err := http.Post(srv.URL+"/v1/normalize", "application/json", strings.NewReader(c.body))
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != c.status {
			t.Errorf("%s: status %d", c.body, res.StatusCode)
		}
		if c.status == 200 {
			var body map[string][]map[string]any
			json.NewDecoder(res.Body).Decode(&body)
			r := body["results"][0]
			for _, key := range []string{"city", "street_name", "apartment", "coordinates", "catalog_id"} {
				v, ok := r[key]
				if !ok || v != nil {
					t.Errorf("%s must be explicit null", key)
				}
			}
		}
		res.Body.Close()
	}
}
func TestCORSAndLimits(t *testing.T) {
	srv := server(t)
	req, _ := http.NewRequest("OPTIONS", srv.URL+"/v1/normalize", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 204 || res.Header.Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatal("preflight failed")
	}
	req.Header.Set("Origin", "https://untrusted.example")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("unexpected CORS permission")
	}
	res, err = http.Post(srv.URL+"/v1/normalize", "application/json", bytes.NewBufferString(`{"records":[{"id":"a","raw_address":"`+strings.Repeat("a", 2<<20)+`"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 413 {
		t.Fatalf("large body status: %d", res.StatusCode)
	}
}

func TestBothNormalizeFormats(t *testing.T) {
	srv := server(t)
	for _, array := range []bool{false, true} {
		t.Run(fmt.Sprintf("array=%t", array), func(t *testing.T) {
			wrap := func(records string) string {
				if array {
					return records
				}
				return `{"records":` + records + `}`
			}
			cases := []struct {
				body   string
				status int
			}{
				{`[{"id":9007199254740993,"raw_address":"  неизвестно\u200b  "},{"id":"duplicate","raw_address":""},{"id":"duplicate","raw_address":""}]`, 200},
				{`[]`, 400},
				{`[null]`, 400},
				{`[{"id":null,"raw_address":"x"}]`, 400},
				{`[{"id":1.2,"raw_address":"x"}]`, 400},
				{`[{"id":9223372036854775808,"raw_address":"x"}]`, 400},
				{`[{"id":"x"}]`, 400},
				{`[{"id":"x","raw_address":null}]`, 400},
				{`[{"id":"x","raw_address":12}]`, 400},
				{`[{"id":"x","raw_address":"x","extra":true}]`, 400},
				{`[{"id":"x","raw_address":"` + strings.Repeat("x", 4097) + `"}]`, 400},
				{`[` + strings.Repeat(`{"id":"x","raw_address":""},`, 1000) + `{"id":"x","raw_address":""}]`, 400},
				{`[{"id":"x","raw_address":"` + strings.Repeat("x", 2<<20) + `"}]`, 413},
			}
			for _, c := range cases {
				res, err := http.Post(srv.URL+"/v1/normalize", "application/json", strings.NewReader(wrap(c.body)))
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(res.Body)
				res.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if res.StatusCode != c.status {
					t.Fatalf("expected %d, got %d: %s", c.status, res.StatusCode, body)
				}
				if c.status != 200 {
					var failure api.Error
					if json.Unmarshal(body, &failure) != nil || failure.Code == "" {
						t.Fatalf("missing error object: %s", body)
					}
					continue
				}
				var results []map[string]json.RawMessage
				if array {
					err = json.Unmarshal(body, &results)
				} else {
					var envelope struct {
						Results []map[string]json.RawMessage `json:"results"`
					}
					err = json.Unmarshal(body, &envelope)
					results = envelope.Results
				}
				if err != nil || len(results) != 3 {
					t.Fatalf("wrong response shape: %s", body)
				}
				if string(results[0]["id"]) != "9007199254740993" || string(results[1]["id"]) != `"duplicate"` || string(results[2]["id"]) != `"duplicate"` {
					t.Fatal("IDs/order changed")
				}
				var raw string
				if json.Unmarshal(results[0]["raw_address"], &raw) != nil || raw != "  неизвестно\u200b  " {
					t.Fatal("raw changed")
				}
				for _, field := range []string{"province", "city", "district", "living_area", "street_type", "street_name", "house", "apartment", "entrance", "floor", "landmark"} {
					if string(results[1][field]) != "null" {
						t.Fatalf("%s must be explicit null", field)
					}
				}
			}
		})
	}
}

func TestNormalizeRejectsInvalidShapes(t *testing.T) {
	srv := server(t)
	for _, body := range []string{`null`, `"x"`, `42`, `{}`, `{"records":null}`, `{"records":[],"extra":1}`, `[] {}`, `[{"id":"x","raw_address":""}] []`, `[`, `{"records":[`} {
		res, err := http.Post(srv.URL+"/v1/normalize", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Errorf("%s: got %d", body, res.StatusCode)
		}
	}
}

func TestThousandRecordBatchRetainsOrder(t *testing.T) {
	srv := server(t)
	records := make([]api.InputRecord, 1000)
	for i := range records {
		records[i] = api.InputRecord{Id: i, RawAddress: "неизвестно"}
	}
	out, err := client.New(srv.URL).Normalize(context.Background(), api.NormalizeRequest{Records: records})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 1000 {
		t.Fatal("batch dropped records")
	}
	for i, r := range out.Results {
		if r.Id.(json.Number).String() != fmt.Sprint(i) {
			t.Fatal("batch reordered IDs")
		}
	}
}
