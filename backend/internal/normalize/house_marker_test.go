package normalize

import (
	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/pkg/api"
	"testing"
)

func TestHouseMarkerVariants(t *testing.T) {
	for _, marker := range []string{"д12", "д 12", "д.12", "д. 12", "дом12", "дом 12", "Д12"} {
		t.Run(marker, func(t *testing.T) {
			raw := "Караганда, Алалыкина " + marker
			e := engine(t)
			e.AllowDemo = false
			// libpostal can leave the marker attached to its house token. Rules must
			// recover the number even when that token fails the numeric validation.
			e.Postal = fakePostal{{Label: "city", Value: "караганда"}, {Label: "road", Value: "алалыкина"}, {Label: "house_number", Value: marker}}
			r := e.Normalize(api.InputRecord{Id: 1, RawAddress: raw})
			if catalog.Text(r.House) != "12" || r.RawAddress != raw || catalog.Text(r.StreetName) != "Алалыкина" {
				t.Fatalf("lost house: %+v", r)
			}
		})
	}
	for _, raw := range []string{"улица Под12", "улица Сад12", "проспект Абу-Даби"} {
		if r := ruleParse(raw); r.House != nil {
			t.Fatalf("street fragment became house: %s %+v", raw, r)
		}
	}
	for _, tt := range []struct{ raw, house string }{
		{"д12", "12"},
		{"Алалыкина,д12", "12"},
		{"Алалыкина д12а кв4", "12а"},
		{"Алалыкина д55/6 4подъезд 9этаж", "55/6"},
		{"Алалыкина д12-14", "12-14"},
	} {
		if r := ruleParse(tt.raw); catalog.Text(r.House) != tt.house {
			t.Fatalf("marker: %s %+v", tt.raw, r)
		}
	}
}
