package normalize

import (
	"context"
	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/testutil"
	"github.com/lukivan8/jana-address/pkg/api"
	"testing"
)

func TestLandmarksDoNotBecomeSettlementCandidates(t *testing.T) {
	e := engine(t)
	e.AllowDemo = false
	e.Reference = testutil.Reference(t)
	// A deliberately wrong parser city must not override an explicit landmark span.
	e.Postal = fakePostal{{Label: "city", Value: "караганда"}, {Label: "road", Value: "туран"}, {Label: "house_number", Value: "55/6"}}
	for _, raw := range []string{"Туран 55/6 магазин Караганда 4подъезд 9этаж", "Туран 55/6 школа Караганда", "Туран 55/6 аптека Караганда"} {
		in := api.InputRecord{Id: "test", RawAddress: raw}
		r := e.Normalize(in)
		if r.City != nil || r.Province != nil || r.Coordinates != nil || catalog.Text(r.StreetName) != "Туран" || r.Landmark == nil {
			t.Fatalf("landmark became location: %+v", r)
		}
		if c := e.CityCandidates(context.Background(), in); len(c) != 0 {
			t.Fatalf("landmark candidates: %+v", c)
		}
		if _, err := e.SelectCity(context.Background(), in, "c1"); err != ErrInvalidCitySelection {
			t.Fatal("landmark selection accepted", err)
		}
	}
	// The same name outside the landmark must remain an administrative mention.
	in := api.InputRecord{Id: 1, RawAddress: "Караганда, Туран 55/6, магазин Караганда"}
	if c := e.CityCandidates(context.Background(), in); len(c) != 2 {
		t.Fatalf("real city removed with repeated landmark: %+v", c)
	}
	// Another city used as a shop name must not suppress the actual city candidates.
	in.RawAddress = "Караганда, Туран 55/6, магазин Астана"
	if c := e.CityCandidates(context.Background(), in); len(c) != 2 {
		t.Fatalf("shop city introduced ambiguity: %+v", c)
	}
}

func TestBareSettlementComponentSurvivesPostalVenueLabel(t *testing.T) {
	e := engine(t)
	e.Reference = testutil.Reference(t)
	e.Postal = fakePostal{{Label: "house", Value: "астана"}, {Label: "road", Value: "улица алалыкина"}, {Label: "house_number", Value: "63"}}
	in := api.InputRecord{Id: "unique", RawAddress: "Астана, улица Алалыкина 63"}
	r := e.Normalize(in)
	if catalog.Text(r.City) != "Астана" || catalog.Text(r.Province) != "Карагандинская область" {
		t.Fatalf("unique settlement lost to venue hypothesis: %+v", r)
	}
	choices := e.CityCandidates(context.Background(), in)
	if len(choices) != 1 || choices[0].EntityId != "other" {
		t.Fatalf("candidate and normalization disagree: %+v %+v", choices, r)
	}
}

func TestStandaloneSettlementSurvivesVenueSpanningAddress(t *testing.T) {
	for _, tc := range []struct {
		raw, province string
		candidates    int
	}{
		{"г.Караганда, Алалыкина д12", "Карагандинская область", 1},
		{"Караганда, Алалыкина д12", "", 2},
		{"кафе г.Караганда, Алалыкина д12", "", 0},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			e := Engine{Reference: testutil.Reference(t), Postal: fakePostal{{Label: "house", Value: tc.raw}}}
			in := api.InputRecord{Id: "venue", RawAddress: tc.raw}
			r := e.Normalize(in)
			choices := e.CityCandidates(context.Background(), in)
			if len(choices) != tc.candidates || catalog.Text(r.Province) != tc.province || catalog.Text(r.House) != "12" || catalog.Text(r.StreetName) != "Алалыкина" {
				t.Fatalf("standalone settlement role: %+v %+v", r, choices)
			}
			if tc.candidates > 1 && (r.Status != api.NeedsReview || !hasWarning(r.Warnings, "ambiguous_city_reference")) {
				t.Fatalf("venue hypothesis suppressed homonym: %+v", r)
			}
		})
	}
}
