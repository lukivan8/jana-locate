package normalize

import (
	"context"
	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/testutil"
	"github.com/lukivan8/jana-address/pkg/api"
	"testing"
)

func TestSourceSettlementQualifierExpansion(t *testing.T) {
	for _, tc := range []struct {
		phrase, province string
		count            int
	}{
		{"город Караганда", "Карагандинская область", 1},
		{"ГОРОД Караганда", "Карагандинская область", 1},

		{"Караганда", "", 2},
	} {
		t.Run(tc.phrase, func(t *testing.T) {
			e := Engine{Reference: testutil.Reference(t), Postal: fakePostal{{Label: "city", Value: tc.phrase}, {Label: "road", Value: "улица Алалыкина"}, {Label: "house_number", Value: "12"}}}
			in := api.InputRecord{Id: 1, RawAddress: tc.phrase + ", улица Алалыкина 12"}
			r := e.Normalize(in)
			if catalog.Text(r.City) != "Караганда" || catalog.Text(r.Province) != tc.province || catalog.Text(r.House) != "12" {
				t.Fatalf("unexpected result: %+v", r)
			}
			if c := e.CityCandidates(context.Background(), in); len(c) != tc.count {
				t.Fatalf("candidates: %+v", c)
			}
			if tc.count > 1 && !hasWarning(r.Warnings, "ambiguous_city_reference") {
				t.Fatal("lost homonym ambiguity")
			}
		})
	}
}

func TestVenuePrefixDoesNotPromoteQualifiedCity(t *testing.T) {
	e := Engine{Reference: testutil.Reference(t), Postal: fakePostal{{Label: "house", Value: "кафе"}, {Label: "city", Value: "город Караганда"}, {Label: "road", Value: "улица Алалыкина"}, {Label: "house_number", Value: "12"}}}
	in := api.InputRecord{Id: 1, RawAddress: "кафе город Караганда, улица Алалыкина 12"}
	r := e.Normalize(in)
	if r.City != nil || r.Province != nil {
		t.Fatalf("venue became geography: %+v", r)
	}
	if c := e.CityCandidates(context.Background(), in); len(c) != 0 {
		t.Fatalf("venue candidates: %+v", c)
	}
}
