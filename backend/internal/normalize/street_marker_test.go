package normalize

import (
	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/pkg/api"
	"testing"
)

func TestStreetMarkersDoNotConsumeNamePrefixes(t *testing.T) {
	for _, name := range []string{"Пришахтинск", "Ульянова", "Пермская", "Мкртчян", "Проспектный"} {
		if r := ruleParse(name); r.StreetName != nil || r.StreetType != nil {
			t.Fatalf("name became marker: %s %+v", name, r)
		}
		e := Engine{Postal: fakePostal{{Label: "road", Value: name}}}
		r := e.Normalize(api.InputRecord{Id: 1, RawAddress: name})
		if catalog.Text(r.StreetName) != name || r.StreetType != nil {
			t.Fatalf("postal name truncated: %+v", r)
		}

	}
	for _, tt := range []struct{ raw, name, kind string }{
		{"пр. Туран 12", "Туран", "проспект"},
		{"пр.Туран 12", "Туран", "проспект"},
		{"пр-т Туран 12", "Туран", "проспект"},
		{"Ул .Курильская 9", "Курильская", "улица"},
		{"ул.Курильская 9", "Курильская", "улица"},
		{"переулок Пермский 12", "Пермский", "переулок"},
		{"мкр. Аксай 12", "Аксай", "микрорайон"},
	} {
		r := ruleParse(tt.raw)
		if catalog.Text(r.StreetName) != tt.name || catalog.Text(r.StreetType) != tt.kind {
			t.Fatalf("valid marker lost: %s %+v", tt.raw, r)
		}
	}
}
