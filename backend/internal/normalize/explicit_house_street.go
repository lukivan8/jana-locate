package normalize

import "strings"

// When libpostal misses the road next to a glued house marker (Алалыкина д12),
// recover only a complete comma-separated street alias from the reference.
// A fragment such as Street inside an unknown Smoke Street is insufficient.
func (e Engine) streetWithExplicitHouse(work string) string {
	if !houseRE.MatchString(work) && !reverseHouseRE.MatchString(work) {
		return ""
	}
	reduced := administrativeText(work)
	for _, d := range details {
		reduced = d.re.ReplaceAllString(reduced, " ")
	}
	reduced = houseRE.ReplaceAllString(reduced, " ")
	reduced = reverseHouseRE.ReplaceAllString(reduced, " ")
	streets := map[string]bool{}
	for _, segment := range strings.FieldsFunc(reduced, func(r rune) bool { return r == ',' || r == ';' }) {
		value := strings.TrimSpace(segment)
		if len(e.Reference.Exact(value, "city")) > 0 || len(e.Reference.Exact(value, "province")) > 0 {
			continue
		}
		if len(e.Reference.Exact(value, "street")) > 0 {
			streets[value] = true
		}
	}
	if len(streets) == 1 {
		for value := range streets {
			return value
		}
	}
	return ""
}
