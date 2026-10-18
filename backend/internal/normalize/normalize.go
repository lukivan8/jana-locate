// Package normalize reconciles libpostal, explicit rules and immutable
// address references. It does not read the building catalog or perform geocoding.
package normalize

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/postal"
	"github.com/lukivan8/jana-address/internal/reference"
	"github.com/lukivan8/jana-address/pkg/api"
)

type Engine struct {
	Contextual        *ContextualClient
	NumericSupplement *NumericClient
	// Deprecated: retained for internal fixture compatibility; never read during normalization.
	Catalog   *catalog.Store
	Postal    postal.Parser
	Reference *reference.Directory
	// Deprecated: demo buildings cannot influence address normalization.
	AllowDemo    bool
	selectedCity string
}
type evidence struct {
	Field, Value, Source string
	Score                float64
}
type resolved struct {
	Value    string
	Score    float64
	Conflict bool
}

var numeric = regexp.MustCompile(`^[0-9]+(?:[/\-][0-9]+)?[а-яА-Яa-zA-Z]?$`)
var streetMarker = regexp.MustCompile(`(?i)^(?:улица|ул|проспект|пр-т|пр|переулок|пер|микрорайон|мкр)(?:\s*\.\s*|\s+)|\s+(?:көшесі|даңғылы)$`)
var trailingHouse = regexp.MustCompile(`\s+` + number + `\s*(?:[,;]|$)`)

func Working(raw string) string {
	return strings.Join(strings.Fields(invisible.ReplaceAllString(raw, "")), " ")
}
func (e Engine) Normalize(in api.InputRecord) api.Result {
	return e.NormalizeBatch([]api.InputRecord{in})[0]
}
func (e Engine) NormalizeBatch(records []api.InputRecord) []api.Result {
	return e.NormalizeBatchContext(context.Background(), records)
}
func (e Engine) NormalizeBatchContext(ctx context.Context, records []api.InputRecord) []api.Result {
	if e.Contextual != nil {
		return e.contextualBatch(ctx, records)
	}
	texts := make([]string, len(records))
	signals := make([][]evidence, len(records))
	warnings := make([][]string, len(records))
	// libpostal and rules independently inspect the same working text.
	for i, r := range records {
		texts[i] = Working(r.RawAddress)
		warnings[i] = []string{}
		if texts[i] == "" {
			continue
		}
		signals[i], warnings[i] = e.extractionSignals(texts[i])
	}
	out := make([]api.Result, 0, len(records))
	for i, r := range records {
		out = append(out, e.reconcile(r, texts[i], signals[i], warnings[i]))
	}
	if e.NumericSupplement != nil && len(records) > 0 {
		return e.supplementNumeric(ctx, records, out)
	}
	return out
}

// Keep field extraction identical for normalization and candidate discovery.
func (e Engine) extractionSignals(work string) ([]evidence, []string) {
	signals := []evidence{}
	warnings := []string{}
	if work == "" {
		return signals, warnings
	}
	if e.Postal != nil {
		components, err := e.Postal.Parse(work)
		if err != nil {
			warnings = append(warnings, "libpostal_unavailable")
		} else {
			signals = append(signals, postalSignals(work, components)...)
		}
	} else {
		warnings = append(warnings, "libpostal_unavailable")
	}
	signals = append(signals, ruleSignals(work)...)
	return signals, warnings
}

func ruleSignals(work string) []evidence {
	parsed := ruleParse(work)
	out := []evidence{}
	if match := typedStreetRE.FindString(work); match != "" {
		out = append(out, evidence{"_road", match, "rules", 0})
	}
	for field, value := range resultValues(parsed) {
		if value != "" {
			out = append(out, evidence{field, value, "rules", 0.94})
		}
	}
	// A trailing house is a useful standalone signal even with no building match.
	// Explicit-marker details have already been removed in the existing rule parser;
	// use a separate reduced copy to avoid treating apartment/floor as the house.
	reduced := work
	for _, d := range details {
		reduced = d.re.ReplaceAllString(reduced, " ")
	}
	reduced = landmarkRE.ReplaceAllString(reduced, " ")
	if parsed.House == nil {
		if m := trailingHouse.FindStringSubmatch(reduced); m != nil {
			out = append(out, evidence{"house", m[1], "rules-implicit", 0.72})
		}
	}
	return out
}
func postalSignals(work string, parts []postal.Component) []evidence {
	labels := map[string]string{"state": "province", "city": "city", "city_district": "district", "suburb": "living_area", "road": "street_name", "house_number": "house", "unit": "apartment", "staircase": "entrance", "level": "floor", "house": "landmark"}
	out := []evidence{}
	for _, p := range parts {
		field, ok := labels[p.Label]
		if p.Label == "road" || p.Label == "house" {
			if value := surface(work, p.Value); value != "" {
				role := "_road"
				if p.Label == "house" {
					role = "_venue"
				}
				out = append(out, evidence{role, value, "libpostal", 0})
			}
		}
		if p.Label == "house" && !landmarkRE.MatchString(p.Value) {
			continue
		}
		if !ok {
			continue
		}
		value := surface(work, p.Value)
		if value == "" {
			continue
		}
		if field == "street_name" {
			value = strings.TrimSpace(streetMarker.ReplaceAllString(value, ""))
			if name, kind := latinStreetMarker(value); kind != "" {
				value = name
				out = append(out, evidence{"street_type", kind, "explicit-marker", 0.94})
			}
		}
		if field == "house" || field == "apartment" || field == "entrance" || field == "floor" {
			if !numeric.MatchString(value) {
				continue
			}
		}
		out = append(out, evidence{field, value, "libpostal", 0.66})
	}
	return out
}

// libpostal normalizes case and occasionally punctuation. Only accept a value
// if its token sequence can be grounded back into the working text.
var surfaceTokens = regexp.MustCompile(`[\pL\pN/\-]+`)

func surface(work, value string) string {
	spans := surfaceTokens.FindAllStringIndex(work, -1)
	needle := strings.Fields(reference.Canon(value))
	n := len(needle)
	if n == 0 {
		return ""
	}
	for i := 0; i+n <= len(spans); i++ {
		candidate := work[spans[i][0]:spans[i+n-1][1]]
		if reference.Canon(candidate) == reference.Canon(value) {
			return candidate
		}
	}
	return ""
}

func resolve(signals []evidence) map[string]resolved {
	grouped := map[string]map[string][]evidence{}
	for _, ev := range signals {
		if ev.Value == "" {
			continue
		}
		if grouped[ev.Field] == nil {
			grouped[ev.Field] = map[string][]evidence{}
		}
		key := reference.Canon(ev.Value)
		grouped[ev.Field][key] = append(grouped[ev.Field][key], ev)
	}
	out := map[string]resolved{}
	for field, values := range grouped {
		candidates := []resolved{}
		for _, items := range values {
			sort.SliceStable(items, func(i, j int) bool { return items[i].Score > items[j].Score })
			score := items[0].Score
			sources := map[string]bool{items[0].Source: true}
			for _, item := range items[1:] {
				if !sources[item.Source] {
					score += (1 - score) * 0.25
					sources[item.Source] = true
				}
			}
			candidates = append(candidates, resolved{Value: items[0].Value, Score: math.Min(score, 0.99)})
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Score == candidates[j].Score {
				return candidates[i].Value < candidates[j].Value
			}
			return candidates[i].Score > candidates[j].Score
		})
		best := candidates[0]
		if len(candidates) > 1 && best.Score-candidates[1].Score < 0.12 && best.Score < 0.94 {
			best.Conflict = true
			best.Value = ""
		}
		out[field] = best
	}
	return out
}
func (e Engine) reconcile(in api.InputRecord, work string, signals []evidence, warnings []string) api.Result {
	r := api.Result{Id: in.Id, RawAddress: in.RawAddress, Warnings: warnings, Status: api.Unmatched}
	// Reference entity mentions are candidates, not independent votes for every
	// homonym. Resolve hierarchy first and abstain if several identities remain.
	administrative := administrativeText(work)
	signals = administrativeSignals(work, administrative, signals)
	districts, districtModel, cityText, signals := e.districtMentions(administrative, signals)
	mentions := e.Reference.Matches(cityText)
	explicitCities := map[string]bool{}
	for _, segment := range strings.Split(cityText, ",") {
		value := strings.TrimSpace(segment)
		if len(e.Reference.Exact(value, "city")) > 0 {
			explicitCities[reference.Canon(value)] = true
		}
	}
	if len(explicitCities) > 1 {
		r.Warnings = append(r.Warnings, "ambiguous_city")
	}
	provinces := map[string]reference.Entity{}
	for _, m := range mentions {
		if m.Entity.Kind == "province" {
			provinces[m.Entity.ID] = m.Entity
		}
	}
	var cityID string
	cities := map[string]reference.Entity{}
	preliminary := resolve(signals)
	if preliminary["street_name"].Value == "" && preliminary["house"].Value != "" {
		if street := e.streetWithExplicitHouse(work); street != "" {
			signals = append(signals, evidence{"street_name", street, "reference", 0.86})
			preliminary = resolve(signals)
		}
	}

	for _, m := range mentions {
		if m.Entity.Kind == "city" {
			// Match the candidate endpoint: an explicit reference province
			// constrains settlement identities before administrative inference.
			if len(provinces) == 1 {
				if _, compatible := provinces[m.Entity.ProvinceID]; !compatible {
					continue
				}
			}
			if street := preliminary["street_name"].Value; street != "" && contains(street, m.Surface) && !explicitCities[reference.Canon(m.Surface)] && !e.equivalent(preliminary["city"].Value, m.Entity.Name, "city") {
				continue
			}
			if city := preliminary["city"].Value; city != "" && !e.equivalent(city, m.Entity.Name, "city") {
				continue
			}
			if province := preliminary["province"].Value; province != "" {
				if p, ok := e.Reference.Entity(m.Entity.ProvinceID); ok && !e.equivalent(province, p.Name, "province") {
					continue
				}
			}
			cities[m.Entity.ID] = m.Entity
		}
	}
	cities = compatibleMentionedCities(e.Reference, mentions, cities)
	if e.selectedCity != "" {
		city, _ := e.Reference.Entity(e.selectedCity)
		cities = map[string]reference.Entity{city.ID: city}
		// Identity chosen by the operator replaces parser guesses for administrative fields.
		kept := signals[:0]
		for _, signal := range signals {
			if signal.Field != "city" && signal.Field != "province" {
				kept = append(kept, signal)
			}
		}
		signals = kept
	}
	// A street/house fact in a partial directory cannot exclude a settlement
	// homonym that has no address coverage. Only explicit administrative input
	// or operator selection may resolve those compatible identities.
	if len(cities) > 1 && len(explicitCities) <= 1 {
		cities = e.citiesInDistricts(cities, districts)
	}
	if len(cities) == 1 {
		for id, c := range cities {
			cityID = id
			signals = append(signals, evidence{"city", c.Name, "reference", 0.92})
			if p, ok := e.Reference.Entity(c.ProvinceID); ok {
				signals = append(signals, evidence{"province", p.Name, "reference", 0.88})
			}
		}
	}
	if len(cities) > 1 {
		names := map[string]string{}
		for _, c := range cities {
			names[reference.Canon(c.Name)] = c.Name
		}
		if len(names) == 1 || (preliminary["city"].Value != "" && !preliminary["city"].Conflict) {
			for _, name := range names {
				if preliminary["city"].Value != "" {
					name = preliminary["city"].Value
				}
				signals = append(signals, evidence{"city", name, "reference", 0.88})
			}
			r.Warnings = append(r.Warnings, "ambiguous_city_reference")
		} else {
			r.Warnings = append(r.Warnings, "ambiguous_city")
		}
	}
	if len(provinces) == 1 {
		for _, p := range provinces {
			signals = append(signals, evidence{"province", p.Name, "reference-explicit", 0.92})
		}
	}
	if len(provinces) > 1 {
		r.Warnings = append(r.Warnings, "ambiguous_province")
	}
	for _, kind := range []string{"street", "living_area"} {
		matches := map[string]reference.Match{}
		for _, m := range mentions {
			if m.Entity.Kind == kind && (cityID == "" || m.Entity.CityID == "" || m.Entity.CityID == cityID) {
				// Settlement aliases (including bilingual forms) are not
				// independent street/living-area evidence without a parser role.
				field := map[string]string{"street": "street_name", "living_area": "living_area"}[kind]
				qualifiedCity := false
				for _, cityMention := range mentions {
					if cityMention.Entity.ID == cityID && strings.HasPrefix(reference.Canon(cityMention.Surface), "город ") {
						qualifiedCity = true
					}
				}
				if preliminary[field].Value == "" && qualifiedCity {
					isCityAlias := false
					for _, c := range e.Reference.Exact(m.Surface, "city") {
						isCityAlias = isCityAlias || c.ID == cityID
					}
					if isCityAlias {
						continue
					}
				}
				// An alias that only matches a fragment (e.g. "Street" within an
				// unknown "Smoke Street") cannot contradict a complete extraction.
				if kind == "street" && preliminary["street_name"].Value != "" && !e.equivalent(preliminary["street_name"].Value, m.Entity.Name, "street") {
					continue
				}
				if m.Entity.CityID != "" && preliminary["city"].Value != "" {
					if c, ok := e.Reference.Entity(m.Entity.CityID); ok && !e.equivalent(preliminary["city"].Value, c.Name, "city") {
						continue
					}
				}
				if kind == "street" && numeric.MatchString(m.Surface) && preliminary["street_name"].Value != m.Surface {
					continue
				}
				matches[m.Entity.ID] = m
			}
		}
		if len(matches) == 1 {
			for _, m := range matches {
				field := map[string]string{"street": "street_name", "district": "district", "living_area": "living_area"}[kind]
				value := m.Surface
				if kind == "street" {
					value, _ = latinStreetMarker(value)
				}
				if kind != "street" {
					value = m.Entity.Name
				}
				signals = append(signals, evidence{field, value, "reference", 0.86})
				if kind == "street" && m.Entity.StreetType != "" {
					signals = append(signals, evidence{"street_type", m.Entity.StreetType, "reference", 0.86})
				}
				// Do not infer a city solely from a street in a geographically partial OSM snapshot.
			}
		} else if len(matches) > 1 {
			r.Warnings = append(r.Warnings, "ambiguous_reference_"+kind)
		}
	}
	signals = e.reconcileDistrict(cityID, cities, districts, districtModel, signals, &r.Warnings)
	fields := resolve(signals)
	// Canonicalize a complete Latin street component only when all compatible
	// reference candidates agree on its name. Unknown names stay as entered.
	if street := fields["street_name"]; street.Value != "" && !street.Conflict && fields["city"].Value != "" {
		name := ""
		conflict := false
		for _, candidate := range e.Reference.LatinExact(street.Value, "street") {
			if cityID != "" && candidate.CityID != "" && candidate.CityID != cityID {
				continue
			}
			if len(cities) == 0 && candidate.CityID != "" && fields["city"].Value != "" {
				city, ok := e.Reference.Entity(candidate.CityID)
				if !ok || (!e.equivalent(fields["city"].Value, city.Name, "city") && reference.LatinKey(city.Name) != reference.Canon(fields["city"].Value)) {
					continue
				}
			}
			if len(cities) > 0 && candidate.CityID != "" {
				if _, compatible := cities[candidate.CityID]; !compatible {
					continue
				}
			}
			if reference.LatinKey(candidate.Name) != reference.Canon(street.Value) {
				continue
			}
			if name != "" && name != candidate.Name {
				conflict = true
			}
			name = candidate.Name
		}
		if name != "" && !conflict {
			street.Value = name
			fields["street_name"] = street
		}
	}
	for field, v := range fields {
		if v.Conflict {
			r.Warnings = append(r.Warnings, "conflicting_"+field)
		}
	}
	apply(&r, fields)
	if hasWarning(r.Warnings, "ambiguous_city") {
		r.City = nil
		r.Province = nil
		fields["city"] = resolved{Conflict: true}
	}
	// Confidence aggregates actual field evidence, completeness and contradictions.
	// Model scores are evidence strengths, not calibrated correctness probabilities.
	var total, weight float64
	for field, w := range map[string]float64{"province": 1, "city": 2, "street_name": 3, "house": 3, "apartment": 1, "entrance": 1, "floor": 1, "district": 1, "living_area": 1, "landmark": 1} {
		v := fields[field]
		if v.Value != "" {
			total += v.Score * w
			weight += w
		}
	}
	if weight > 0 {
		r.Confidence = total / weight
		coverage := 0.55
		if r.StreetName != nil {
			coverage += 0.15
		}
		if r.House != nil {
			coverage += 0.15
		}
		if r.City != nil {
			coverage += 0.15
		}
		r.Confidence *= coverage
		r.Status = api.NeedsReview
	}
	for _, v := range fields {
		if v.Conflict {
			r.Confidence *= 0.7
		}
	}
	for _, warning := range r.Warnings {
		if strings.HasPrefix(warning, "ambiguous_") {
			r.Confidence *= 0.7
		}
		if warning == "libpostal_unavailable" {
			r.Confidence *= 0.95
		}
	}
	uncertain := false
	for _, warning := range r.Warnings {
		if strings.HasPrefix(warning, "ambiguous_") || strings.HasPrefix(warning, "conflicting_") {
			uncertain = true
		}
	}
	if !uncertain && r.Province != nil && r.City != nil && r.StreetName != nil && r.House != nil && r.Confidence >= 0.72 {
		r.Status = api.Matched
	}
	return r
}
func hasWarning(warnings []string, value string) bool {
	for _, w := range warnings {
		if w == value {
			return true
		}
	}
	return false
}
func (e Engine) equivalent(a, b, kind string) bool {
	if reference.Canon(a) == reference.Canon(b) {
		return true
	}
	ids := map[string]bool{}
	for _, v := range e.Reference.Exact(a, kind) {
		ids[v.ID] = true
	}
	for _, v := range e.Reference.Exact(b, kind) {
		if ids[v.ID] {
			return true
		}
	}
	return false
}
func resultValues(r api.Result) map[string]string {
	return map[string]string{"province": catalog.Text(r.Province), "city": catalog.Text(r.City), "district": catalog.Text(r.District), "living_area": catalog.Text(r.LivingArea), "street_type": catalog.Text(r.StreetType), "street_name": catalog.Text(r.StreetName), "house": catalog.Text(r.House), "apartment": catalog.Text(r.Apartment), "entrance": catalog.Text(r.Entrance), "floor": catalog.Text(r.Floor), "landmark": catalog.Text(r.Landmark)}
}
func apply(r *api.Result, fields map[string]resolved) {
	dest := map[string]**string{"province": &r.Province, "city": &r.City, "district": &r.District, "living_area": &r.LivingArea, "street_type": &r.StreetType, "street_name": &r.StreetName, "house": &r.House, "apartment": &r.Apartment, "entrance": &r.Entrance, "floor": &r.Floor, "landmark": &r.Landmark}
	for field, target := range dest {
		v := fields[field]
		*target = str(v.Value)
	}
}
