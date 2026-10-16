package normalize

import (
	"context"
	"database/sql"
	"errors"
	"github.com/lukivan8/jana-address/internal/reference"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/postal"
	"github.com/lukivan8/jana-address/pkg/api"
)

type fakePostal []postal.Component

func (f fakePostal) Parse(s string) ([]postal.Component, error) { return f, nil }

type fakeExtractor struct {
	rows  [][]Entity
	texts []string
	err   error
}

func (f *fakeExtractor) Extract(ctx context.Context, texts []string) ([][]Entity, error) {
	f.texts = append([]string{}, texts...)
	return f.rows, f.err
}
func TestExtractionDoesNotRequireBuilding(t *testing.T) {
	e := engine(t)
	e.AllowDemo = false
	e.Postal = fakePostal{{Label: "road", Value: "новая"}, {Label: "house_number", Value: "12а"}, {Label: "city", Value: "алматы"}}
	r := e.Normalize(api.InputRecord{Id: "x", RawAddress: "Алматы, Новая 12а"})
	if catalog.Text(r.StreetName) != "Новая" || catalog.Text(r.House) != "12а" || catalog.Text(r.City) != "Алматы" || r.Coordinates != nil {
		t.Fatalf("extraction incorrectly needs a building: %+v", r)
	}
}
func TestDemoIsExcludedByDefault(t *testing.T) {
	e := engine(t)
	e.AllowDemo = false
	r := e.Normalize(api.InputRecord{Id: "x", RawAddress: "Алматы, улица Курильская 9"})
	if r.Coordinates != nil || r.CatalogId != nil {
		t.Fatal("demo leaked into ordinary results")
	}
}
func TestApartmentCannotSupplyHouseForGeocoding(t *testing.T) {
	e := engine(t)
	r := e.Normalize(api.InputRecord{Id: "x", RawAddress: "улица Курильская кв 9"})
	if r.Coordinates != nil || r.House != nil {
		t.Fatalf("apartment became house: %+v", r)
	}
}
func TestGLiNERGetsUnmodifiedWorkingTextAndRulesWinMarkers(t *testing.T) {
	e := engine(t)
	f := &fakeExtractor{rows: [][]Entity{{{Label: "house", Text: "1", Score: 0.9}, {Label: "street_name", Text: "Новая", Score: 0.9}}}}
	e.GLiNER = f
	r := e.Normalize(api.InputRecord{Id: "secret-id", RawAddress: "  улица\u200b Новая 12а кв 1 "})
	if len(f.texts) != 1 || f.texts[0] != "улица Новая 12а кв 1" {
		t.Fatalf("destructive model input: %#v", f.texts)
	}
	if catalog.Text(r.House) != "12а" || catalog.Text(r.Apartment) != "1" {
		t.Fatalf("model overrode explicit structure: %+v", r)
	}
}
func TestModelFailureIsVisibleAndDoesNotDropRecords(t *testing.T) {
	e := engine(t)
	e.GLiNER = &fakeExtractor{err: errors.New("timeout")}
	out := e.NormalizeBatch([]api.InputRecord{{Id: "1", RawAddress: "улица Новая 12а"}, {Id: "2", RawAddress: ""}})
	if len(out) != 2 || !hasWarning(out[0].Warnings, "gliner_unavailable") || out[1].Id != "2" {
		t.Fatal("failure was hidden or dropped inputs")
	}
}
func TestConfidenceRespondsToCorroboration(t *testing.T) {
	e := engine(t)
	e.AllowDemo = false
	input := api.InputRecord{Id: "x", RawAddress: "улица Новая 12а"}
	a := e.Normalize(input)
	e.Postal = fakePostal{{Label: "road", Value: "новая"}, {Label: "house_number", Value: "12а"}}
	b := e.Normalize(input)
	if b.Confidence <= a.Confidence {
		t.Fatalf("confidence ignores actual agreement: %f <= %f", b.Confidence, a.Confidence)
	}
}
func TestGLiNERValidatesCodepointSpansAndBatchCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[[{"start":0,"end":6,"text":"Алматы","label":"city","score":0.9}]]}`))
	}))
	defer srv.Close()
	g := GLiNER{URL: srv.URL}
	rows, err := g.Extract(context.Background(), []string{"Алматы"})
	if err != nil || rows[0][0].Text != "Алматы" {
		t.Fatal(err)
	}
	if _, err = g.Extract(context.Background(), []string{"Астана"}); err == nil {
		t.Fatal("ungrounded span accepted")
	}
	if _, err = g.Extract(context.Background(), []string{"Алматы", "Алматы"}); err == nil {
		t.Fatal("wrong batch count accepted")
	}
}
func TestFieldConflictAbstains(t *testing.T) {
	v := resolve([]evidence{{"city", "Алматы", "gliner", 0.85}, {"city", "Астана", "libpostal", 0.8}})["city"]
	if !v.Conflict || strings.TrimSpace(v.Value) != "" {
		t.Fatal("conflicting model claims did not abstain")
	}
}

func TestAdministrativeHomonymKeepsExtractedNameWithoutGeocoding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reference.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE metadata(key TEXT,value TEXT); INSERT INTO metadata VALUES('schema_version','1');
 CREATE TABLE entities(id TEXT,kind TEXT,name TEXT,parent_id TEXT,city_id TEXT,province_id TEXT,district_id TEXT,street_type TEXT,source TEXT);
 CREATE TABLE aliases(entity_id TEXT,alias TEXT);
 INSERT INTO entities VALUES('one','city','Алматы','','one','one','','','test'),('two','city','Алматинское','','two','two','','','test');
 INSERT INTO aliases VALUES('one','Алматы'),('two','Алматы'),('two','Алматинское');`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	d, err := reference.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	e := engine(t)
	e.Reference = d
	e.Postal = fakePostal{{Label: "city", Value: "алматы"}, {Label: "road", Value: "курильская"}, {Label: "house_number", Value: "9"}}
	r := e.Normalize(api.InputRecord{Id: "x", RawAddress: "Алматы, улица Курильская 9"})
	if catalog.Text(r.City) != "Алматы" || r.Coordinates != nil || !hasWarning(r.Warnings, "ambiguous_city_reference") {
		t.Fatalf("identity ambiguity lost extraction or invented a location: %+v", r)
	}
}
