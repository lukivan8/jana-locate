package normalize

import (
	"context"
	"database/sql"
	"errors"
	"github.com/lukivan8/jana-address/internal/reference"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/postal"
	"github.com/lukivan8/jana-address/pkg/api"
)

type fakePostal []postal.Component

func (f fakePostal) Parse(s string) ([]postal.Component, error) { return f, nil }

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
func TestPostalFailureKeepsRecordsAndReportsOnlyRemainingParser(t *testing.T) {
	e := engine(t)
	e.Postal = failedPostal{}
	records := []api.InputRecord{{Id: "same", RawAddress: "  улица Новая 12а кв 1  "}, {Id: "same", RawAddress: ""}}
	out := e.NormalizeBatch(records)
	if len(out) != 2 || out[0].Id != records[0].Id || out[0].RawAddress != records[0].RawAddress || out[1].Id != records[1].Id || out[1].RawAddress != "" {
		t.Fatal("input identity changed", out)
	}
	if len(out[0].Warnings) != 1 || out[0].Warnings[0] != "libpostal_unavailable" || len(out[1].Warnings) != 0 {
		t.Fatal("unexpected parser diagnostics", out)
	}
	if catalog.Text(out[0].House) != "12а" || catalog.Text(out[0].Apartment) != "1" {
		t.Fatal("rules did not survive parser failure", out[0])
	}
}

type failedPostal struct{}

func (failedPostal) Parse(string) ([]postal.Component, error) {
	return nil, errors.New("parser failed")
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
func TestFieldConflictAbstains(t *testing.T) {
	v := resolve([]evidence{{"city", "Алматы", "rules", 0.85}, {"city", "Астана", "libpostal", 0.8}})["city"]
	if !v.Conflict || strings.TrimSpace(v.Value) != "" {
		t.Fatal("conflicting parser and rule claims did not abstain")
	}
}

func TestStructuredAddressPreservesHomonymsWithoutInventingCoordinates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reference.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE metadata(key TEXT,value TEXT); INSERT INTO metadata VALUES('schema_version','2');
 CREATE TABLE entities(id TEXT,kind TEXT,name TEXT,parent_id TEXT,city_id TEXT,province_id TEXT,district_id TEXT,street_type TEXT,source TEXT);
 CREATE TABLE aliases(entity_id TEXT,alias TEXT);
 CREATE TABLE addresses(id TEXT,street_id TEXT,city_id TEXT,house TEXT,source TEXT);
 INSERT INTO entities VALUES
 ('p1','province','Карагандинская область','','','p1','','','test'),
 ('p2','province','Актюбинская область','','','p2','','','test'),
 ('c1','city','Караганда','p1','c1','p1','','','test'),
 ('c2','city','Караганда','p2','c2','p2','','','test'),
 ('s1','street','Алалыкина','c1','c1','p1','','улица','test'),
 ('s2','street','Алалыкина','c2','c2','p2','','улица','test');
 INSERT INTO aliases VALUES('p1','Карагандинская область'),('p2','Актюбинская область'),
 ('c1','Караганда'),('c1','г.Караганда'),('c2','Караганда'),('c2','с.Караганда'),('s1','Алалыкина'),('s2','Алалыкина');
 INSERT INTO addresses VALUES('a1','s1','c1','12','test');`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	e := engine(t)
	e.AllowDemo = false
	e.Reference, err = reference.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		raw, house, province string
		ambiguous            bool
	}{
		{"Караганда, Алалыкина 12", "12", "", true},
		{"Караганда, Алалыкина д12", "12", "", true},
		{"Караганда, Алалыкина д 12", "12", "", true},
		{"Караганда, Алалыкина д.12", "12", "", true},
		{"Караганда, Алалыкина 13", "13", "", true},
		{"Караганда, Алалыкина 12/1", "12/1", "", true},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			e.Postal = fakePostal{{Label: "city", Value: "караганда"}, {Label: "road", Value: "алалыкина"}, {Label: "house_number", Value: tt.house}}
			if houseRE.MatchString(tt.raw) {
				// Native libpostal can miss the road when the house marker is glued.
				e.Postal = fakePostal{{Label: "city", Value: "караганда"}, {Label: "house_number", Value: tt.house}}
			}

			r := e.Normalize(api.InputRecord{Id: "case", RawAddress: tt.raw})
			if r.Status != api.NeedsReview || r.District != nil || catalog.Text(r.Province) != tt.province || hasWarning(r.Warnings, "ambiguous_city_reference") != tt.ambiguous || r.Coordinates != nil || r.CatalogId != nil || r.RawAddress != tt.raw {
				t.Fatalf("unsafe reference result: %+v", r)
			}
		})
	}
	// Explicit province/type still resolves identity, independently of address coverage.
	e.Postal = fakePostal{{Label: "road", Value: "алалыкина"}, {Label: "house_number", Value: "12"}}
	for _, qualified := range []struct{ raw, city, province string }{
		{"Карагандинская область, Караганда, Алалыкина 12", "c1", "Карагандинская область"},
		{"Актюбинская область, Караганда, Алалыкина 12", "c2", "Актюбинская область"},
		{"г.Караганда, Алалыкина 12", "c1", "Карагандинская область"},
		{"с.Караганда, Алалыкина 12", "c2", "Актюбинская область"},
	} {
		in := api.InputRecord{Id: "qualified", RawAddress: qualified.raw}
		r := e.Normalize(in)
		choices := e.CityCandidates(context.Background(), in)
		if r.Status != api.Matched || catalog.Text(r.Province) != qualified.province || len(choices) != 1 || choices[0].EntityId != qualified.city {
			t.Fatalf("explicit administrative evidence lost: %+v %+v", r, choices)
		}
	}
	// Positive facts in both settlements must still abstain.
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO addresses VALUES('a2','s2','c2','12','test')"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	e.Reference, err = reference.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	e.Postal = fakePostal{{Label: "road", Value: "алалыкина"}, {Label: "house_number", Value: "12"}}
	r := e.Normalize(api.InputRecord{Id: "case", RawAddress: "Караганда, Алалыкина 12"})
	if r.Province != nil || !hasWarning(r.Warnings, "ambiguous_city_reference") {
		t.Fatalf("duplicate structured addresses chose a city: %+v", r)
	}
	r = e.Normalize(api.InputRecord{Id: "case", RawAddress: "Алалыкина 12"})
	if r.City != nil || r.Province != nil || r.Coordinates != nil {
		t.Fatalf("partial reference invented city: %+v", r)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO entities VALUES
 ('g1','street','Street','c1','c1','p1','','улица','test'),
 ('g2','street','Street','c2','c2','p2','','улица','test');
 INSERT INTO aliases VALUES('g1','Street'),('g2','Street');`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	e.Reference, err = reference.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	e.Postal = fakePostal{{Label: "road", Value: "smoke street"}, {Label: "house_number", Value: "12"}}
	r = e.Normalize(api.InputRecord{Id: "fragment", RawAddress: "Караганда, Smoke Street 12"})
	if catalog.Text(r.StreetName) != "Smoke Street" || hasWarning(r.Warnings, "ambiguous_reference_street") || r.StreetType != nil {
		t.Fatalf("fragment alias polluted complete street extraction: %+v", r)
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
