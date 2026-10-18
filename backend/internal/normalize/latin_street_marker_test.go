package normalize

import (
	"context"
	"testing"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/testutil"
	"github.com/lukivan8/jana-address/pkg/api"
)

func TestLatinRoadMarkerAndReferenceName(t *testing.T) {
	for _, marker := range []string{"ulitsa", "ULITSA", "ul.", "Ul ."} {
		t.Run(marker, func(t *testing.T) {
			road := marker + " Alalykina"
			e := Engine{Reference: testutil.Reference(t), Postal: fakePostal{{Label: "city", Value: "Karaganda"}, {Label: "road", Value: road}, {Label: "house_number", Value: "12"}}}
			in := api.InputRecord{Id: 1, RawAddress: "Karaganda, " + road + " 12"}
			r := e.Normalize(in)
			if catalog.Text(r.City) != "Karaganda" || catalog.Text(r.StreetName) != "Alalykina" || catalog.Text(r.StreetType) != "улица" || catalog.Text(r.House) != "12" {
				t.Fatalf("unexpected fields: %+v", r)
			}
			// The component lookup does not invent a resolved city identity.
			if r.Province != nil || r.Status != api.NeedsReview {
				t.Fatalf("unresolved city became authoritative: %+v", r)
			}
			if r.RawAddress != in.RawAddress {
				t.Fatal("raw address changed")
			}
			in.RawAddress = "Караганда, " + road + " 12"
			e.Postal = fakePostal{{Label: "city", Value: "Караганда"}, {Label: "road", Value: road}, {Label: "house_number", Value: "12"}}
			if candidates := e.CityCandidates(context.Background(), in); len(candidates) != 2 {
				t.Fatalf("homonym candidates lost: %+v", candidates)
			}
		})
	}
}

func TestLatinStreetTypesAndUnknownNames(t *testing.T) {
	for _, tc := range []struct{ road, name, kind string }{
		{"ulitsa Novaya 7", "Novaya 7", "улица"},
		{"prospekt Dostyk", "Dostyk", "проспект"},
		{"pereulok Tikhiy", "Tikhiy", "переулок"},
		{"Ulitsaev", "Ulitsaev", ""},
		{"Prospektny", "Prospektny", ""},
		{"ulitsa", "ulitsa", ""},
		{"Unknown Alalykina", "Unknown Alalykina", ""},
	} {
		t.Run(tc.road, func(t *testing.T) {
			e := Engine{Reference: testutil.Reference(t), Postal: fakePostal{{Label: "road", Value: tc.road}}}
			r := e.Normalize(api.InputRecord{Id: 1, RawAddress: tc.road})
			if catalog.Text(r.StreetName) != tc.name || catalog.Text(r.StreetType) != tc.kind {
				t.Fatalf("unexpected fields: %+v", r)
			}
		})
	}
}

// Presentation must not depend on whether a spelling has a directory hit.
func TestStreetReferenceLookupPreservesInputSpelling(t *testing.T) {
	for _, name := range []string{"Alalykina", "ALALYKINA", "alalykina", "Алалыкина", "алалыкина", "Unknown Alalykina"} {
		t.Run(name, func(t *testing.T) {
			directory := testutil.Reference(t)
			e := Engine{Reference: directory, Postal: fakePostal{{Label: "city", Value: "Караганда"}, {Label: "road", Value: name}, {Label: "house_number", Value: "12"}}}
			in := api.InputRecord{Id: "surface", RawAddress: "Караганда, " + name + " 12"}
			r := e.Normalize(in)
			if catalog.Text(r.StreetName) != name || r.RawAddress != in.RawAddress || catalog.Text(r.House) != "12" {
				t.Fatalf("reference replaced input spelling: %+v", r)
			}
			if name != "Unknown Alalykina" && len(directory.LatinExact(name, "street"))+len(directory.Exact(name, "street")) == 0 {
				t.Fatal("fixture does not exercise a reference hit")
			}
			if len(e.CityCandidates(context.Background(), in)) != 2 {
				t.Fatal("street spelling suppressed a settlement homonym")
			}
		})
	}
}
