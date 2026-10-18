package reference_test

import (
	"testing"

	"github.com/lukivan8/jana-address/internal/reference"
	"github.com/lukivan8/jana-address/internal/testutil"
)

func TestLatinAliasesRetainAllHomonyms(t *testing.T) {
	d := testutil.Reference(t)
	if matches := d.Matches("Karaganda, ulitsa Alalykina 12"); len(matches) != 0 {
		t.Fatalf("component aliases leaked into full-text geography: %+v", matches)
	}
	if cities := d.LatinExact("Karaganda", "city"); len(cities) != 2 {
		t.Fatalf("expected both homonyms: %+v", cities)
	}
	if streets := d.LatinExact("Alalykina", "street"); len(streets) != 1 || streets[0].Name != "Алалыкина" {
		t.Fatalf("missing transliterated street: %+v", streets)
	}
	if streets := d.LatinExact("Unknown Alalykina", "street"); len(streets) != 0 {
		t.Fatalf("partial name matched: %+v", streets)
	}
	for _, tc := range []struct{ source, latin string }{
		{"Жамбыла", "zhambyla"}, {"Курильская", "kurilskaya"},
		{"Тихий", "tikhiy"}, {"Қонаев", "qonaev"},
	} {
		if actual := reference.LatinKey(tc.source); actual != tc.latin {
			t.Fatalf("%s: %s", tc.source, actual)
		}
	}
}
