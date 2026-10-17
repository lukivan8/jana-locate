package normalize

import (
	"math"
	"path/filepath"
	"reflect"
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
			e.Postal = fakePostal{{Label: "road", Value: tt.street}}
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
func TestBuildingCatalogCannotAffectNormalization(t *testing.T) {
	e := engine(t)
	e.Postal = fakePostal{{Label: "road", Value: "туран"}, {Label: "house_number", Value: "55/6"}}
	in := api.InputRecord{Id: "a", RawAddress: "Туран 55/6 кв 99"}
	before := e.Normalize(in)
	f := api.AddressFields{City: str("Тестовый город"), Province: str("Тестовая область"), StreetName: str("Туран"), StreetType: str("проспект"), House: str("55/6")}
	if _, err := e.Catalog.Confirm(api.ConfirmationRequest{Fields: f, Coordinates: api.Coordinates{Latitude: 50, Longitude: 70}}); err != nil {
		t.Fatal(err)
	}
	after := e.Normalize(in)
	if math.Abs(before.Confidence-after.Confidence) > 1e-12 {
		t.Fatal("catalog changed confidence")
	}
	before.Confidence = after.Confidence
	if !reflect.DeepEqual(before, after) || after.Coordinates != nil || after.CatalogId != nil || after.City != nil {
		t.Fatalf("catalog influenced normalization: before=%+v after=%+v", before, after)
	}
	// The normalizer is fully usable with no building store at all.
	e.Catalog = nil
	if got := e.Normalize(in); math.Abs(got.Confidence-after.Confidence) > 1e-12 || !reflect.DeepEqual(resultValues(got), resultValues(after)) {
		t.Fatal("normalization depends on catalog")
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
