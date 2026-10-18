package normalize

import (
	"regexp"
	"strings"

	"github.com/lukivan8/jana-address/pkg/api"
)

var componentTokens = regexp.MustCompile(`[0-9]+(?:[/\-][0-9]+)?|[\pL]+`)
var corpusPair = regexp.MustCompile(`(?i)(?:^|[,;\s])` + number + `\s+(?:корпус|корп\.?|korpus)\s*` + number + `(?:$|[,;\s])`)
var componentRoles = map[string]string{
	"дом": "house", "д": "house", "dom": "house", "house": "house", "үй": "house", "uy": "house", "ui": "house",
	"квартира": "apartment", "квартиры": "apartment", "кв": "apartment", "kv": "apartment", "kvartira": "apartment", "flat": "apartment", "apt": "apartment", "пәтер": "apartment", "pater": "apartment", "xonadon": "apartment",
	"подъезд": "entrance", "подьезд": "entrance", "подезд": "entrance", "пд": "entrance", "под": "entrance", "podezd": "entrance", "pod": "entrance", "entrance": "entrance", "entry": "entrance", "кіреберіс": "entrance", "кіреберс": "entrance", "kirish": "entrance",
	"этаж": "floor", "эт": "floor", "etazh": "floor", "etaj": "floor", "floor": "floor", "level": "floor", "қабат": "floor", "qabat": "floor", "qavat": "floor",
}

type explicitComponent struct {
	Field string `json:"field"`
	Value string `json:"value"`
	Start int    `json:"start_byte"`
	End   int    `json:"end_byte"`
}

// Only complete two-token components qualify. Free-form prose, street names,
// landmarks and instructions are not searched for incidental number/word pairs.
func explicitComponents(raw string) []explicitComponent {
	out := []explicitComponent{}
	start := 0
	consume := func(end int) {
		clause := raw[start:end]
		spans := componentTokens.FindAllStringIndex(clause, -1)
		if len(spans) != 2 {
			return
		}
		if strings.TrimSpace(clause[:spans[0][0]]) != "" || strings.Trim(clause[spans[1][1]:], " .\t\r\n") != "" {
			return
		}
		for marker, value := range map[int]int{0: 1, 1: 0} {
			word := strings.ToLower(clause[spans[marker][0]:spans[marker][1]])
			field := componentRoles[word]
			text := clause[spans[value][0]:spans[value][1]]
			if field == "" || !numeric.MatchString(text) {
				continue
			}
			if field != "house" && strings.IndexFunc(text, func(c rune) bool { return c < '0' || c > '9' }) >= 0 {
				continue
			}
			gap := clause[spans[0][1]:spans[1][0]]
			allowed := " .:\t\r\n"
			if marker == 1 {
				allowed += "-"
			}
			if strings.Trim(gap, allowed) != "" {
				continue
			}
			out = append(out, explicitComponent{field, text, start + spans[value][0], start + spans[value][1]})
		}
	}
	for i, c := range raw {
		if c == ',' || c == ';' || c == '\n' {
			consume(i)
			start = i + len(string(c))
		}
	}
	consume(len(raw))
	return out
}

func protectedExplicitNumber(b api.Result, p explicitComponent) bool {
	text := strings.ToLower(b.RawAddress)
	for _, v := range []*string{b.StreetName, b.Landmark, b.LivingArea, b.City, b.District, b.Province} {
		if v == nil || *v == "" {
			continue
		}
		needle := strings.ToLower(*v)
		for offset := 0; offset < len(text); {
			index := strings.Index(text[offset:], needle)
			if index < 0 {
				break
			}
			a := offset + index
			if a < p.End && p.Start < a+len(needle) {
				return true
			}
			offset = a + len(needle)
		}
	}
	return false
}

func supplementExplicitComponents(b api.Result) api.Result {
	original := map[string]*string{"house": b.House, "apartment": b.Apartment, "entrance": b.Entrance, "floor": b.Floor}
	fields := map[string]**string{"house": &b.House, "apartment": &b.Apartment, "entrance": &b.Entrance, "floor": &b.Floor}
	grouped := map[string]map[string]explicitComponent{}
	for _, p := range explicitComponents(b.RawAddress) {
		if protectedExplicitNumber(b, p) {
			continue
		}
		if grouped[p.Field] == nil {
			grouped[p.Field] = map[string]explicitComponent{}
		}
		grouped[p.Field][p.Value] = p
	}
	accepted := []explicitComponent{}
	for _, field := range []string{"house", "apartment", "entrance", "floor"} {
		facts := grouped[field]
		if len(facts) > 1 {
			warning := "conflicting_explicit_" + field
			if !hasWarning(b.Warnings, warning) {
				b.Warnings = append(b.Warnings, warning)
			}
			b.Status = api.NeedsReview
			continue
		}
		if *fields[field] != nil {
			continue
		}
		for _, p := range facts {
			value := p.Value
			*fields[field] = &value
			accepted = append(accepted, p)
		}
	}
	// A following corpus marker explicitly assigns that number to the corpus.
	// Correct only the precise baseline confusion, never arbitrary house values.
	pairs := corpusPair.FindAllStringSubmatchIndex(b.RawAddress, -1)
	if len(pairs) == 1 && b.House != nil {
		m := pairs[0]
		house := b.RawAddress[m[2]:m[3]]
		corpus := b.RawAddress[m[4]:m[5]]
		if *b.House == corpus && house != corpus {
			b.House = str(house)
			accepted = append(accepted, explicitComponent{"house", house, m[2], m[3]})
		}
	}
	if len(accepted) > 0 {
		details := map[string]interface{}{}
		if b.NormalizationDetails != nil {
			for k, v := range *b.NormalizationDetails {
				details[k] = v
			}
		}
		details["explicit_components"] = map[string]interface{}{"accepted": accepted, "baseline_numeric": original, "confidence_scope": "unchanged_native_evidence"}
		b.NormalizationDetails = &details
		version := "native-explicit-components-v1"
		b.NormalizerVersion = &version
	}
	return b
}
