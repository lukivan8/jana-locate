package normalize

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/postal"
	"github.com/lukivan8/jana-address/internal/testutil"
	"github.com/lukivan8/jana-address/pkg/api"
	"reflect"
	"testing"
)

func TestCitySelectionRecomputesHierarchyAndBuilding(t *testing.T) {
	e := engine(t)
	e.AllowDemo = false
	e.Reference = testutil.Reference(t)
	in := api.InputRecord{Id: json.Number("9223372036854775807"), RawAddress: "  Караганда, улица Алалыкина 7 кв 99  "}
	ctx := context.Background()
	initial := e.Normalize(in)
	if initial.Province != nil || !hasWarning(initial.Warnings, "ambiguous_city_reference") {
		t.Fatalf("ambiguity lost: %+v", initial)
	}
	choices := e.CityCandidates(ctx, in)
	if len(choices) != 2 || catalog.Text(choices[0].SettlementType) != "город" || catalog.Text(choices[1].SettlementType) != "село" {
		t.Fatalf("choices: %+v", choices)
	}
	r, err := e.SelectCity(ctx, in, "c1")
	if err != nil || catalog.Text(r.Province) != "Карагандинская область" || r.Coordinates != nil || r.District != nil || r.Confidence >= 1 || hasWarning(r.Warnings, "ambiguous_city_reference") || r.RawAddress != in.RawAddress || r.Id != in.Id {
		t.Fatalf("choice: %+v %v", r, err)
	}
	_, err = e.Catalog.Confirm(api.ConfirmationRequest{Fields: api.AddressFields{City: r.City, Province: r.Province, StreetName: r.StreetName, StreetType: r.StreetType, House: r.House}, Coordinates: api.Coordinates{Latitude: 49, Longitude: 73}})
	if err != nil {
		t.Fatal(err)
	}
	r, err = e.SelectCity(ctx, in, "c1")
	if err != nil || r.Coordinates != nil || r.CatalogId != nil {
		t.Fatalf("catalog-independent selection: %+v %v", r, err)
	}
	r, err = e.SelectCity(ctx, in, "c2")
	if err != nil || catalog.Text(r.Province) != "Актюбинская область" || r.Coordinates != nil || r.CatalogId != nil || r.CatalogRevision != nil || catalog.Text(r.Apartment) != "99" {
		t.Fatalf("stale building: %+v %v", r, err)
	}
	// Constraint is per call; the synchronous engine retains its previous behavior.
	if got := e.Normalize(in); !reflect.DeepEqual(got.Province, initial.Province) || !hasWarning(got.Warnings, "ambiguous_city_reference") {
		t.Fatalf("constraint leaked: %+v", got)
	}
	for _, bad := range []api.InputRecord{{Id: 1, RawAddress: "Карагандинская область, Караганда, улица Алалыкина 7"}, {Id: 1, RawAddress: "Астана"}, {Id: 1, RawAddress: "Караганда, Астана"}} {
		if _, err = e.SelectCity(ctx, bad, "c2"); !errors.Is(err, ErrInvalidCitySelection) {
			t.Fatalf("incompatible selection accepted: %+v %v", bad, err)
		}
	}
	if c := e.CityCandidates(ctx, api.InputRecord{Id: 1, RawAddress: "г.Караганда"}); len(c) != 1 || c[0].EntityId != "c1" {
		t.Fatalf("explicit type ignored: %+v", c)
	}
}

func TestCancelledCitySelectionDoesNotProduceReplacement(t *testing.T) {
	e := engine(t)
	e.Reference = testutil.Reference(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := e.SelectCity(ctx, api.InputRecord{Id: 1, RawAddress: "Караганда"}, "c1")
	if !errors.Is(err, context.Canceled) || r.RawAddress != "" {
		t.Fatalf("cancelled reparse produced replacement: %+v %v", r, err)
	}
}

type cancellingPostal struct{ cancel context.CancelFunc }

func (p cancellingPostal) Parse(string) ([]postal.Component, error) {
	p.cancel()
	return []postal.Component{{Label: "city", Value: "караганда"}}, nil
}
func TestCitySelectionCancelledDuringParsing(t *testing.T) {
	e := engine(t)
	e.Reference = testutil.Reference(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Postal = cancellingPostal{cancel: cancel}
	r, err := e.SelectCity(ctx, api.InputRecord{Id: 1, RawAddress: "Караганда"}, "c1")
	if !errors.Is(err, context.Canceled) || r.RawAddress != "" {
		t.Fatalf("interrupted reparse produced replacement: %+v %v", r, err)
	}
}

func TestCompleteNormalizationNeedsNoGeocoder(t *testing.T) {
	e := Engine{Reference: testutil.Reference(t), Postal: fakePostal{{Label: "state", Value: "Карагандинская область"}, {Label: "city", Value: "Караганда"}, {Label: "road", Value: "Алалыкина"}}}
	in := api.InputRecord{Id: 1, RawAddress: "Карагандинская область, Караганда, улица Алалыкина д12"}
	r := e.Normalize(in)
	if r.Status != api.Matched || r.Coordinates != nil || r.CatalogId != nil || hasWarning(r.Warnings, "coordinates_unknown") || hasWarning(r.Warnings, "synthetic_catalog_coordinates") {
		t.Fatalf("complete address depends on geocoding: %+v", r)
	}
	r = e.Normalize(api.InputRecord{Id: 2, RawAddress: "Караганда, улица Алалыкина д12"})
	if r.Status != api.NeedsReview || !hasWarning(r.Warnings, "ambiguous_city_reference") {
		t.Fatalf("ambiguity accepted: %+v", r)
	}
}
