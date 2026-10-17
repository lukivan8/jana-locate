package normalize

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/postal"
	"github.com/lukivan8/jana-address/internal/reference"
	"github.com/lukivan8/jana-address/pkg/api"
)

type districtGraph struct {
	Reference struct {
		Cities    []struct{ ID, Name, Kind string }
		Districts []struct {
			ID, Name, Level string
			CityID          *string `json:"city_id"`
		}
		SettlementDistricts []struct {
			CityID      string   `json:"city_id"`
			DistrictIDs []string `json:"district_ids"`
		} `json:"settlement_districts"`
		Streets []struct {
			ID, Name, Coverage string
			CityID             string   `json:"city_id"`
			DistrictIDs        []string `json:"district_ids"`
		}
		Addresses []struct {
			ID, House, Source string
			StreetID          string   `json:"street_id"`
			CityID            string   `json:"city_id"`
			DistrictIDs       []string `json:"district_ids"`
		} `json:"address_evidence"`
	}
	Cases []struct {
		ID, Description string
		Query           struct {
			CityIDs   []string `json:"city_candidates"`
			StreetIDs []string `json:"street_candidates"`
			House     *string
			Explicit  []string `json:"explicit_district_ids"`
		}
		Expected struct {
			Status      string   `json:"district_status"`
			DistrictIDs []string `json:"district_candidates"`
			Diagnostics []string
		}
	}
}

func districtFixture(t testing.TB) (*reference.Directory, districtGraph) {
	return districtFixtureVersion(t, "3")
}

func districtFixtureVersion(t testing.TB, version string) (*reference.Directory, districtGraph) {
	t.Helper()
	raw, err := os.ReadFile("testdata/district-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var graph districtGraph
	if err = json.Unmarshal(raw, &graph); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "district.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE metadata(key TEXT,value TEXT);INSERT INTO metadata VALUES('schema_version','3');
 CREATE TABLE entities(id TEXT PRIMARY KEY,kind TEXT,name TEXT,parent_id TEXT,city_id TEXT,province_id TEXT,district_id TEXT,street_type TEXT,source TEXT);
 CREATE TABLE aliases(entity_id TEXT,alias TEXT);
 CREATE TABLE addresses(id TEXT PRIMARY KEY,street_id TEXT,city_id TEXT,house TEXT,source TEXT);
 CREATE TABLE district_objects(object_kind TEXT,object_id TEXT,city_id TEXT,complete INTEGER,source TEXT,PRIMARY KEY(object_kind,object_id));
 CREATE TABLE district_links(object_kind TEXT,object_id TEXT,district_id TEXT);
 INSERT INTO entities VALUES('p','province','Учебная область','','','p','','','synthetic');
 INSERT INTO aliases VALUES('p','Учебная область');`)
	if err != nil {
		t.Fatal(err)
	}
	insert := func(id, kind, name, parent, city, district, typ string) {
		if _, err = db.Exec("INSERT INTO entities VALUES(?,?,?,?,?,?,?,?,?)", id, kind, name, parent, city, "p", district, typ, "synthetic"); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("INSERT INTO aliases VALUES(?,?)", id, name); err != nil {
			t.Fatal(err)
		}
	}
	parents := map[string]string{}
	for _, link := range graph.Reference.SettlementDistricts {
		parents[link.CityID] = link.DistrictIDs[0]
	}
	for _, city := range graph.Reference.Cities {
		insert(city.ID, "city", city.Name, "p", city.ID, parents[city.ID], "")
	}
	for _, district := range graph.Reference.Districts {
		city := ""
		if district.CityID != nil {
			city = *district.CityID
		}
		insert(district.ID, "district", district.Name, "p", city, "", "")
	}
	add := func(kind, id, city string, complete bool, ids []string) {
		if _, err = db.Exec("INSERT INTO district_objects VALUES(?,?,?,?,?)", kind, id, city, complete, "synthetic geometry"); err != nil {
			t.Fatal(err)
		}
		for _, district := range ids {
			if _, err = db.Exec("INSERT INTO district_links VALUES(?,?,?)", kind, id, district); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, street := range graph.Reference.Streets {
		insert(street.ID, "street", street.Name, street.CityID, street.CityID, "", "улица")
		add("street", street.ID, street.CityID, street.Coverage == "complete_synthetic_geometry", street.DistrictIDs)
	}
	for _, address := range graph.Reference.Addresses {
		if _, err = db.Exec("INSERT INTO addresses VALUES(?,?,?,?,?)", address.ID, address.StreetID, address.CityID, address.House, address.Source); err != nil {
			t.Fatal(err)
		}
		add("address", address.ID, address.CityID, true, address.DistrictIDs)
	}
	// Source-preserving adaptation for an Astana-style urban district.
	if _, err = db.Exec("UPDATE entities SET kind='admin',name='район Учебный Север',parent_id='fixture:city:a',source='KATO:synthetic' WHERE id='fixture:district:north'"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO aliases VALUES('fixture:district:north','район Учебный Север')"); err != nil {
		t.Fatal(err)
	}
	if version != "3" {
		if _, err = db.Exec("UPDATE metadata SET value=? WHERE key='schema_version'", version); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("DROP TABLE district_links;DROP TABLE district_objects"); err != nil {
			t.Fatal(err)
		}
		if version == "1" {
			if _, err = db.Exec("DROP TABLE addresses"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	directory, err := reference.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return directory, graph
}

func TestDistrictReferenceScenarios(t *testing.T) {
	directory, graph := districtFixture(t)
	for _, tt := range graph.Cases {
		t.Run(tt.ID, func(t *testing.T) {
			street, _ := directory.Entity(tt.Query.StreetIDs[0])
			city, _ := directory.Entity(tt.Query.CityIDs[0])
			house := ""
			if tt.Query.House != nil {
				house = *tt.Query.House
			}
			raw := city.Name + ", улица " + street.Name
			parts := fakePostal{{Label: "city", Value: city.Name}, {Label: "road", Value: "улица " + street.Name}}
			if house != "" {
				raw += ", дом " + house
				parts = append(parts, postal.Component{Label: "house_number", Value: house})
			}
			for _, id := range tt.Query.Explicit {
				district, _ := directory.Entity(id)
				raw += ", " + district.Name
			}
			engine := Engine{Reference: directory, Postal: parts}
			if len(tt.Query.CityIDs) == 1 {
				engine.selectedCity = city.ID
			}
			result := engine.Normalize(api.InputRecord{Id: tt.ID, RawAddress: raw})
			want := ""
			if tt.Expected.Status == "resolved" || tt.Expected.Status == "explicit_unverified" {
				district, _ := directory.Entity(tt.Expected.DistrictIDs[0])
				want = district.Name
			}
			if tt.Expected.Status == "conflicting" && len(tt.Query.Explicit) > 0 {
				district, _ := directory.Entity(tt.Query.Explicit[0])
				want = district.Name
			}
			if tt.ID == "district_reference_12" {
				// The specification supplies a foreign street ID. The HTTP input
				// supplies only its name; the same name/house exists in the selected
				// city, so it must use that city's fact, never the foreign district.
				district, _ := directory.Entity("fixture:district:north")
				want = district.Name
			}
			if catalog.Text(result.District) != want {
				t.Fatalf("%s: district=%q want=%q warnings=%v", tt.Description, catalog.Text(result.District), want, result.Warnings)
			}
			switch tt.Expected.Status {
			case "ambiguous":
				if !hasWarning(result.Warnings, "ambiguous_district_reference") && !hasWarning(result.Warnings, "ambiguous_city_reference") {
					t.Fatalf("ambiguity hidden: %+v", result)
				}
				if result.Status == api.Matched {
					t.Fatal("ambiguous address marked matched")
				}
			case "conflicting":
				if !hasWarning(result.Warnings, "conflicting_district") && !hasWarning(result.Warnings, "ambiguous_district_reference") {
					t.Fatalf("conflict hidden: %+v", result)
				}
				if result.Status == api.Matched {
					t.Fatal("conflicting address marked matched")
				}
			}
			if result.RawAddress != raw || result.Id != tt.ID || result.Coordinates != nil {
				t.Fatal("input/provenance lost")
			}
		})
	}
}

func TestDistrictTextRolesAndConflicts(t *testing.T) {
	directory, _ := districtFixture(t)
	for _, tt := range []struct{ raw, road, venue, want, warning string }{
		{"Учебный, район Учебный Север, улица Учебный Север, дом 200", "улица Учебный Север", "", "район Учебный Север", ""},
		{"Учебный, улица Учебный Север, дом 200", "улица Учебный Север", "", "", ""},
		{"Учебный, улица Неизвестная, дом 200, кафе район Учебный Север", "улица Неизвестная", "кафе район Учебный Север", "", ""},
		{"Учебный, улица Неизвестная, дом 200, район Учебный Север", "улица Неизвестная", "район Учебный Север", "район Учебный Север", ""},
		{"Учебный, улица Неизвестная, дом 200; не ехать в район Учебный Север", "улица Неизвестная", "", "", ""},
		{"Учебный, район Учебный Север, Учебный Юг, улица Неизвестная, дом 200", "улица Неизвестная", "", "", "conflicting_district"},
		{"Учебный, Учебный Другой, улица Неизвестная, дом 200", "улица Неизвестная", "", "Учебный Другой", "conflicting_district_context"},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			parts := fakePostal{{Label: "city", Value: "Учебный"}, {Label: "road", Value: tt.road}, {Label: "house_number", Value: "200"}}
			if tt.venue != "" {
				parts = append(parts, postal.Component{Label: "house", Value: tt.venue})
			}
			engine := Engine{Reference: directory, Postal: parts, selectedCity: "fixture:city:a"}
			result := engine.Normalize(api.InputRecord{Id: 1, RawAddress: tt.raw})
			if catalog.Text(result.District) != tt.want || tt.warning != "" && !hasWarning(result.Warnings, tt.warning) {
				t.Fatalf("district=%v warnings=%v", result.District, result.Warnings)
			}
		})
	}
}

func TestOldSnapshotInfersRuralDistrictWithoutStreetGeometry(t *testing.T) {
	for _, version := range []string{"1", "2"} {
		t.Run(version, func(t *testing.T) {
			directory, _ := districtFixtureVersion(t, version)
			value := directory.DistrictsForAddress("fixture:village:a", "unknown", "999", "")
			if !value.Complete || value.Source != "kato" || len(value.Candidates) != 1 {
				t.Fatalf("hierarchy inference: %+v", value)
			}
			if value.Candidates[0].ID != "fixture:district:rural" {
				t.Fatal(value)
			}
			urban := directory.DistrictsForAddress("fixture:city:a", "Учебная Сквозная", "10", "")
			if len(urban.Candidates) != 0 {
				t.Fatal("old snapshot invented geometric district", urban)
			}
		})
	}
}

func TestDistrictContextPreservesAndRefinesSettlementHomonyms(t *testing.T) {
	directory, _ := districtFixture(t)
	engine := Engine{Reference: directory, Postal: fakePostal{{Label: "city", Value: "Учебный"}, {Label: "road", Value: "улица Неизвестная"}, {Label: "house_number", Value: "999"}}}
	input := api.InputRecord{Id: 1, RawAddress: "Учебный, улица Неизвестная, дом 999"}
	result := engine.Normalize(input)
	if !hasWarning(result.Warnings, "ambiguous_city_reference") || result.District != nil {
		t.Fatalf("homonym lost: %+v", result)
	}
	input.RawAddress += ", Учебный Сельский"
	result = engine.Normalize(input)
	if hasWarning(result.Warnings, "ambiguous_city_reference") || catalog.Text(result.District) != "Учебный Сельский" {
		t.Fatalf("district hierarchy failed to refine homonyms: %+v", result)
	}
	candidates := engine.CityCandidates(context.Background(), input)
	if len(candidates) != 1 || candidates[0].EntityId != "fixture:village:a" {
		t.Fatalf("incompatible homonym offered: %+v", candidates)
	}
	input.RawAddress = "район Учебный Север"
	if candidates := engine.CityCandidates(context.Background(), input); len(candidates) != 0 {
		t.Fatalf("district fragment offered as a settlement: %+v", candidates)
	}
}

func BenchmarkDistrictBatch1000(b *testing.B) {
	directory, _ := districtFixture(b)
	engine := Engine{Reference: directory, Postal: fakePostal{{Label: "city", Value: "Учебный"}, {Label: "road", Value: "Учебная Сквозная"}, {Label: "house_number", Value: "10"}}, selectedCity: "fixture:city:a"}
	records := make([]api.InputRecord, 1000)
	for i := range records {
		records[i] = api.InputRecord{Id: fmt.Sprint(i), RawAddress: "Учебный, улица Учебная Сквозная, дом 10"}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := engine.NormalizeBatch(records)
		if len(results) != 1000 || results[0].District == nil {
			b.Fatal("district missing")
		}
	}
}
