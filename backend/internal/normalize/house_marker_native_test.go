//go:build libpostal && cgo

package normalize

import (
	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/postal"
	"github.com/lukivan8/jana-address/internal/reference"
	"github.com/lukivan8/jana-address/pkg/api"
	"os"
	"testing"
)

func TestNativeHouseMarkers(t *testing.T) {
	data, ref := os.Getenv("LIBPOSTAL_TEST_DATA"), os.Getenv("REFERENCE_TEST_PATH")
	if data == "" || ref == "" {
		t.Skip("prepared native models and reference snapshot not configured")
	}
	e := engine(t)
	e.AllowDemo = false
	var err error
	e.Postal, err = postal.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	e.Reference, err = reference.Open(ref)
	if err != nil {
		t.Fatal(err)
	}
	var baseline float64
	for _, marker := range []string{"12", "д12", "д 12", "д.12", "д. 12", "дом12", "дом 12"} {
		raw := "г.Караганда, Алалыкина " + marker
		parts, _ := e.Postal.Parse(raw)
		t.Logf("native components for %s: %+v", marker, parts)
		r := e.Normalize(api.InputRecord{Id: marker, RawAddress: raw})
		if marker == "12" {
			baseline = r.Confidence
		}
		if catalog.Text(r.House) != "12" || catalog.Text(r.Province) != "Карагандинская область" || r.RawAddress != raw || r.Coordinates != nil || r.Confidence < baseline {
			t.Fatalf("native marker failed: %+v", r)
		}
		t.Logf("%s: house=%s province=%s confidence=%.4f", marker, *r.House, *r.Province, r.Confidence)
	}
}
