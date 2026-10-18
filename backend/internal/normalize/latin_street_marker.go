package normalize

import (
	"strings"
	"unicode"
)

// Parse a complete type token in a road already extracted by libpostal.
// No city names, street names or address-specific answers belong here.
func latinStreetMarker(road string) (string, string) {
	end := strings.IndexFunc(road, func(c rune) bool { return unicode.IsSpace(c) || c == '.' })
	if end < 0 {
		return road, ""
	}
	kind := map[string]string{
		"ulitsa": "улица", "ul": "улица",
		"prospekt": "проспект", "pr-t": "проспект",
		"pereulok": "переулок", "per": "переулок",
	}[strings.ToLower(road[:end])]
	name := strings.TrimLeftFunc(road[end:], func(c rune) bool { return unicode.IsSpace(c) || c == '.' })
	if kind == "" || name == "" {
		return road, ""
	}
	return name, kind
}
