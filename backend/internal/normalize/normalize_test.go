package normalize

import (
	"path/filepath"
	"testing"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/pkg/api"
)

func engine(t *testing.T) Engine {
	t.Helper()
	s, err := catalog.Open("../../data/catalog.seed.json", filepath.Join(t.TempDir(), "overlay.json"))
	if err != nil {
		t.Fatal(err)
	}
	return Engine{Catalog: s, AllowDemo: true}
}
func TestHackathonExamples(t *testing.T) {
	e := engine(t)
	tests := []struct{ raw, street, house, apartment, entrance, floor string }{
		{"Ул .Курильская 9 кв 1", "Курильская", "9", "1", "", ""},
		{"Туран 55/6 магазин Береке 4подьезд 9этаж", "Туран", "55/6", "", "4", "9"},
		{"66квартал 12дом 69квартира", "66", "12", "69", "", ""},
		{"ул. Курильская 9 flat 191", "Курильская", "9", "191", "", ""},
		{"улица Новая 12а кв 4", "Новая", "12а", "4", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			r := e.Normalize(api.InputRecord{Id: "input", RawAddress: tt.raw})
			if r.RawAddress != tt.raw {
				t.Fatal("raw_address changed")
			}
			for k, pair := range map[string][2]string{"street": {catalog.Text(r.StreetName), tt.street}, "house": {catalog.Text(r.House), tt.house}, "apartment": {catalog.Text(r.Apartment), tt.apartment}, "entrance": {catalog.Text(r.Entrance), tt.entrance}, "floor": {catalog.Text(r.Floor), tt.floor}} {
				if pair[0] != pair[1] {
					t.Errorf("%s: got %q want %q", k, pair[0], pair[1])
				}
			}
		})
	}
}
func TestUnknownHasNoInventedLocation(t *testing.T) {
	r := engine(t).Normalize(api.InputRecord{Id: 9, RawAddress: "неизвестно"})
	if r.Coordinates != nil || r.City != nil || r.StreetName != nil || r.Confidence != 0 || r.Status != api.Unmatched {
		t.Fatalf("invented fields: %+v", r)
	}
}
func TestConfirmationAppliesToBuildingNotRawOrID(t *testing.T) {
	e := engine(t)
	before := e.Normalize(api.InputRecord{Id: "a", RawAddress: "Туран 55/6 кв 1"})
	var found api.CatalogEntry
	for _, c := range e.Catalog.List() {
		if c.Id == *before.CatalogId {
			found = c
		}
	}
	_, err := e.Catalog.Confirm(api.ConfirmationRequest{Fields: found.Fields, Coordinates: api.Coordinates{Latitude: 50, Longitude: 70}, ExpectedRevision: found.Revision})
	if err != nil {
		t.Fatal(err)
	}
	r := e.Normalize(api.InputRecord{Id: "different", RawAddress: "Астана, проспект Туран 55/6 кв 99"})
	if r.Status != api.Matched || r.Coordinates.Latitude != 50 || catalog.Text(r.Apartment) != "99" {
		t.Fatalf("confirmation failed: %+v", r)
	}
}
func TestAmbiguousBuildingsDoNotGeocode(t *testing.T) {
	e := engine(t)
	var f api.AddressFields
	for _, v := range e.Catalog.List() {
		if catalog.Text(v.Fields.StreetName) == "Туран" {
			f = v.Fields
		}
	}
	city := "Алматы"
	f.City = &city
	if _, err := e.Catalog.Confirm(api.ConfirmationRequest{Fields: f, Coordinates: api.Coordinates{Latitude: 43, Longitude: 76}}); err != nil {
		t.Fatal(err)
	}
	r := e.Normalize(api.InputRecord{Id: "x", RawAddress: "Туран 55/6"})
	if r.Coordinates != nil || r.Status != api.NeedsReview {
		t.Fatalf("ambiguous geocode: %+v", r)
	}
	r = e.Normalize(api.InputRecord{Id: "x", RawAddress: "Астана Туран 55/6"})
	if r.Coordinates == nil || catalog.Text(r.City) != "Астана" {
		t.Fatalf("city constraint failed: %+v", r)
	}
}
func BenchmarkBatch1000(b *testing.B) {
	s, err := catalog.Open("../../data/catalog.seed.json", filepath.Join(b.TempDir(), "overlay.json"))
	if err != nil {
		b.Fatal(err)
	}
	e := Engine{Catalog: s}
	records := make([]api.InputRecord, 1000)
	for j := range records {
		records[j] = api.InputRecord{Id: j, RawAddress: "Туран 55/6 магазин Береке 4подьезд 9этаж"}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.NormalizeBatch(records)
	}
}
