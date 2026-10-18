package normalize

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/lukivan8/jana-address/pkg/api"
)

func ns(v string) *string { return &v }
func TestNumericAdditionsPreserveNativeDecisions(t *testing.T) {
	b := api.Result{Id: "x", RawAddress: "Алматы дом 12 xonadon 8", House: ns("12"), City: ns("Алматы"), Status: api.Matched, Confidence: .87, Warnings: []string{}}
	a := addNumericProposals(b, []numericProposal{{Label: "apartment", Start: 21, End: 22, Text: "8", Posterior: .999}})
	if a.Apartment == nil || *a.Apartment != "8" {
		t.Fatal(a)
	}
	a.Apartment = nil
	a.NormalizationDetails = nil
	a.NormalizerVersion = nil
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("changed native fields or decision: %+v", a)
	}
}
func TestNumericNeverOverwritesOrResolvesAmbiguity(t *testing.T) {
	b := api.Result{RawAddress: "дом 12 кв 8", House: ns("12"), Apartment: ns("8"), Confidence: .9, Status: api.Matched}
	ps := []numericProposal{{Label: "house", Start: 10, End: 11, Text: "8", Posterior: 1}, {Label: "apartment", Start: 4, End: 6, Text: "12", Posterior: 1}}
	if !reflect.DeepEqual(addNumericProposals(b, ps), b) {
		t.Fatal("overwrote native fields")
	}
	b.Apartment = nil
	b.Warnings = []string{"ambiguous_city_reference"}
	if !reflect.DeepEqual(addNumericProposals(b, ps), b) {
		t.Fatal("resolved ambiguous record")
	}
}
func TestNumericProtectsLandmarkAndRepeatedRole(t *testing.T) {
	b := api.Result{RawAddress: "дом 12, магазин 8", House: ns("12"), Landmark: ns("магазин 8")}
	if !reflect.DeepEqual(addNumericProposals(b, []numericProposal{{Label: "apartment", Start: 16, End: 17, Text: "8", Posterior: 1}}), b) {
		t.Fatal("mined landmark")
	}
	b.Landmark = nil
	if !reflect.DeepEqual(addNumericProposals(b, []numericProposal{{Label: "apartment", Start: 4, End: 6, Text: "12", Posterior: 1}}), b) {
		t.Fatal("reused house as apartment")
	}
}
func TestNumericOutagePreservesBaseline(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "offline", 503) }))
	defer s.Close()
	e := Engine{NumericSupplement: &NumericClient{URL: s.URL}}
	b := []api.Result{{Id: "a", RawAddress: "x", Status: api.Matched, Confidence: .9}}
	if !reflect.DeepEqual(e.supplementNumeric(context.Background(), []api.InputRecord{{Id: "a", RawAddress: "x"}}, b), b) {
		t.Fatal("outage changed baseline")
	}
}
func TestNumericTransportRejectsInvalidSourcesAndIDs(t *testing.T) {
	for _, body := range []string{
		`{"results":[{"id":"wrong","raw_address":"дом 12","proposals":[]}]}`,
		`{"results":[{"id":9007199254740993,"raw_address":"дом 12","proposals":[{"label":"house","start":4,"end":6,"text":"99","posterior":1}]}]}`,
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		_, err := (&NumericClient{URL: s.URL}).proposals(context.Background(), []api.InputRecord{{Id: json.Number("9007199254740993"), RawAddress: "дом 12"}})
		s.Close()
		if err == nil {
			t.Fatal("accepted invalid supplement")
		}
	}
}
func TestNumericTransportPreservesInt64AndUnicodeOffsets(t *testing.T) {
	body := `{"results":[{"id":9007199254740993,"raw_address":"дом 12","proposals":[{"label":"house","start":4,"end":6,"text":"12","posterior":0.999}]}]}`
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Content-Type"), "json") {
			t.Error("content type")
		}
		w.Write([]byte(body))
	}))
	defer s.Close()
	rows, err := (&NumericClient{URL: s.URL}).proposals(context.Background(), []api.InputRecord{{Id: json.Number("9007199254740993"), RawAddress: "дом 12"}})
	if err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
}
