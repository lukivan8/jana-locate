package normalize

import (
	"github.com/lukivan8/jana-address/pkg/api"
	"regexp"
	"strings"
	"unicode"
)

func str(s string) *string {
	v := strings.TrimSpace(s)
	if v == "" {
		return nil
	}
	return &v
}

var number = `([0-9]+(?:[/\-][0-9]+)?[а-яА-Яa-zA-Z]?)`
var details = []struct {
	re    *regexp.Regexp
	field string
}{
	{regexp.MustCompile(`(?i)([0-9]+)(?:квартира|квартиры)`), "apartment"},
	{regexp.MustCompile(`(?i)([0-9]+)(?:подъезд|подьезд|подезд)`), "entrance"},
	{regexp.MustCompile(`(?i)([0-9]+)(?:этаж)`), "floor"},
	{regexp.MustCompile(`(?i)(?:квартира|квартиры|кв\.?|kv|flat)\s*([0-9]+)`), "apartment"},
	{regexp.MustCompile(`(?i)(?:подъезд|подьезд|подезд|пд\.?)\s*([0-9]+)`), "entrance"},
	{regexp.MustCompile(`(?i)(?:этаж|эт\.?)\s*([0-9]+)`), "floor"},
}
var houseRE = regexp.MustCompile(`(?i)(?:дом|д\.)\s*` + number)
var reverseHouseRE = regexp.MustCompile(`(?i)` + number + `\s*дом`)
var typedStreetRE = regexp.MustCompile(`(?i)(?:^|[,\s])(?:улица|ул\.?|проспект|пр\.?|пр-т|переулок|пер\.?|квартал|микрорайон|мкр\.?)\s*\.?\s*([^,;]+)`)
var tailHouseRE = regexp.MustCompile(`\s+` + number + `\s*$`)
var quarterRE = regexp.MustCompile(`(?i)([0-9]+)\s*(квартал|микрорайон)`)
var landmarkRE = regexp.MustCompile(`(?i)(магазин|школа|аптека)\s+([^,;0-9]+)`)
var invisible = regexp.MustCompile("[\u200b\u200c\u200d\ufeff]")

func tokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '/' && r != '-' })
}
func contains(hay, needle string) bool {
	h, n := tokens(hay), tokens(needle)
	if len(n) == 0 {
		return false
	}
	for i := 0; i+len(n) <= len(h); i++ {
		ok := true
		for j := range n {
			if h[i+j] != n[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
func typeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(s, "ул"):
		return "улица"
	case strings.HasPrefix(s, "пр"):
		return "проспект"
	case strings.HasPrefix(s, "пер"):
		return "переулок"
	case strings.HasPrefix(s, "мк") || strings.HasPrefix(s, "микро"):
		return "микрорайон"
	case strings.HasPrefix(s, "квартал"):
		return "квартал"
	}
	return s
}

// Existing explicit-marker rules remain independent evidence, not the model input.
func ruleParse(work string) api.Result {
	r := api.Result{}
	for _, d := range details {
		if m := d.re.FindStringSubmatch(work); m != nil {
			switch d.field {
			case "apartment":
				r.Apartment = str(m[1])
			case "entrance":
				r.Entrance = str(m[1])
			case "floor":
				r.Floor = str(m[1])
			}
			work = d.re.ReplaceAllString(work, " ")
		}
	}
	if m := landmarkRE.FindStringSubmatch(work); m != nil {
		r.Landmark = str(m[0])
		work = landmarkRE.ReplaceAllString(work, " ")
	}
	for _, re := range []*regexp.Regexp{houseRE, reverseHouseRE} {
		if m := re.FindStringSubmatch(work); m != nil {
			r.House = str(m[1])
			work = re.ReplaceAllString(work, " ")
			break
		}
	}
	if m := quarterRE.FindStringSubmatch(work); m != nil {
		r.StreetName = str(m[1])
		r.StreetType = str(typeName(m[2]))
	} else if m := typedStreetRE.FindStringSubmatch(work); m != nil {
		name := strings.TrimSpace(m[1])
		if tail := tailHouseRE.FindStringSubmatch(name); tail != nil {
			if r.House == nil {
				r.House = str(tail[1])
			}
			name = tailHouseRE.ReplaceAllString(name, "")
		}
		r.StreetName = str(name)
		prefix := strings.TrimSpace(strings.TrimSuffix(m[0], m[1]))
		r.StreetType = str(typeName(strings.TrimLeft(prefix, ", ")))
	}
	return r
}
