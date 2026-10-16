// Package normalize reconciles libpostal, explicit rules, GLiNER and read-only
// reference evidence. Building coordinates come only from an unambiguous catalog.
package normalize

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/postal"
	"github.com/lukivan8/jana-address/internal/reference"
	"github.com/lukivan8/jana-address/pkg/api"
)

type Engine struct {
	Catalog   *catalog.Store
	Postal    postal.Parser
	GLiNER    Extractor
	Reference *reference.Directory
	AllowDemo bool
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
var streetMarker = regexp.MustCompile(`(?i)^(?:улица|ул\.?|проспект|пр\.?|пр-т|переулок|пер\.?|микрорайон|мкр\.?)\s*\.?\s*|\s+(?:көшесі|даңғылы)$`)
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
	ctx, cancel := context.WithTimeout(ctx, 270*time.Second)
	defer cancel()
	entries := e.Catalog.List()
	texts := make([]string, len(records))
	signals := make([][]evidence, len(records))
	warnings := make([][]string, len(records))
	// First pass is libpostal. Rules supplement it and do not destructively modify
	// the text passed to the second model.
	for i, r := range records {
		texts[i] = Working(r.RawAddress)
		warnings[i] = []string{}
		if texts[i] == "" {
			continue
		}
		if e.Postal != nil {
			components, err := e.Postal.Parse(texts[i])
			if err != nil {
				warnings[i] = append(warnings[i], "libpostal_unavailable")
			} else {
				signals[i] = append(signals[i], postalSignals(texts[i], components)...)
			}
		} else {
			warnings[i] = append(warnings[i], "libpostal_unavailable")
		}
		signals[i] = append(signals[i], ruleSignals(texts[i])...)
	}
	if e.GLiNER != nil {
		active := []string{}
		indices := []int{}
		for i, text := range texts {
			if text != "" {
				active = append(active, text)
				indices = append(indices, i)
			}
		}
		if len(active) > 0 {
			rows, err := e.GLiNER.Extract(ctx, active)
			if err != nil || len(rows) != len(active) {
				for _, i := range indices {
					warnings[i] = append(warnings[i], "gliner_unavailable")
				}
			} else {
				for j, row := range rows {
					for _, entity := range row {
						if ev, ok := modelSignal(entity); ok {
							signals[indices[j]] = append(signals[indices[j]], ev)
						}
					}
				}
			}
		}
	} else {
		for i, text := range texts {
			if text != "" {
				warnings[i] = append(warnings[i], "gliner_unavailable")
			}
		}
	}
	out := make([]api.Result, 0, len(records))
	for i, r := range records {
		out = append(out, e.reconcile(r, texts[i], signals[i], entries, warnings[i]))
	}
	return out
}
func ruleSignals(work string) []evidence {
	parsed := ruleParse(work)
	out := []evidence{}
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

func modelSignal(entity Entity) (evidence, bool) {
	field := entity.Label
	switch field {
	case "province", "city", "district", "living_area", "street_name", "landmark":
	case "house", "apartment", "entrance", "floor":
		if !numeric.MatchString(entity.Text) {
			return evidence{}, false
		}
	default:
		return evidence{}, false
	}
	value := strings.TrimSpace(entity.Text)
	if field == "city" {
		value = regexp.MustCompile(`(?i)^(?:город\s+|г\.\s*|қала\s+)`).ReplaceAllString(value, "")
	}
	if field == "district" {
		value = regexp.MustCompile(`(?i)\s+(?:район|ауданы)$`).ReplaceAllString(value, "")
	}
	if field == "street_name" {
		value = strings.TrimSpace(streetMarker.ReplaceAllString(value, ""))
	}
	if value == "" || entity.Score < 0.25 {
		return evidence{}, false
	}
	return evidence{field, value, "gliner", 0.45 + 0.45*entity.Score}, true
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
func (e Engine) reconcile(in api.InputRecord, work string, signals []evidence, entries []api.CatalogEntry, warnings []string) api.Result {
	r := api.Result{Id: in.Id, RawAddress: in.RawAddress, Warnings: warnings, Status: api.Unmatched}
	// Reference entity mentions are candidates, not independent votes for every
	// homonym. Resolve hierarchy first and abstain if several identities remain.
	mentions := e.Reference.Matches(work)
	explicitCities := map[string]bool{}
	for _, segment := range strings.Split(work, ",") {
		value := strings.TrimSpace(segment)
		if len(e.Reference.Exact(value, "city")) > 0 {
			explicitCities[reference.Canon(value)] = true
		}
	}
	if len(explicitCities) > 1 {
		r.Warnings = append(r.Warnings, "ambiguous_city")
	}
	var cityID string
	cities := map[string]reference.Entity{}
	preliminary := resolve(signals)
	for _, m := range mentions {
		if m.Entity.Kind == "city" {
			if street := preliminary["street_name"].Value; street != "" && contains(street, m.Surface) && !e.equivalent(preliminary["city"].Value, m.Entity.Name, "city") {
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
	provinces := map[string]reference.Entity{}
	for _, m := range mentions {
		if m.Entity.Kind == "province" {
			provinces[m.Entity.ID] = m.Entity
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
	for _, kind := range []string{"street", "district", "living_area"} {
		matches := map[string]reference.Match{}
		for _, m := range mentions {
			if m.Entity.Kind == kind && (cityID == "" || m.Entity.CityID == "" || m.Entity.CityID == cityID) {
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
	// Confirmed/demo-opted-in building cities are explicit-text signals too.
	buildingCities := map[string]string{}
	for _, c := range entries {
		if isDemo(c) && !e.AllowDemo {
			continue
		}
		name := catalog.Text(c.Fields.City)
		if contains(work, name) {
			buildingCities[reference.Canon(name)] = name
		}
	}
	if len(cities) == 0 && len(buildingCities) == 1 {
		for _, name := range buildingCities {
			signals = append(signals, evidence{"city", name, "building", 0.86})
		}
	}
	if len(buildingCities) > 1 && len(cities) == 0 {
		r.Warnings = append(r.Warnings, "ambiguous_city")
	}
	reduced := work
	for _, d := range details {
		reduced = d.re.ReplaceAllString(reduced, " ")
	}
	reduced = landmarkRE.ReplaceAllString(reduced, " ")
	// Building facts can corroborate components, but only when their street and
	// house actually occur in the working text. Demo facts require explicit opt-in.
	for _, c := range entries {
		if isDemo(c) && !e.AllowDemo {
			continue
		}
		if contains(work, catalog.Text(c.Fields.StreetName)) && contains(reduced, catalog.Text(c.Fields.House)) {
			signals = append(signals, evidence{"street_name", surface(work, catalog.Text(c.Fields.StreetName)), "building", 0.78}, evidence{"house", catalog.Text(c.Fields.House), "building", 0.78})
		}
	}
	fields := resolve(signals)
	for field, v := range fields {
		if v.Conflict {
			r.Warnings = append(r.Warnings, "conflicting_"+field)
		}
	}
	apply(&r, fields)
	if hasWarning(r.Warnings, "ambiguous_city") || (len(cities) == 0 && len(buildingCities) > 1) {
		r.City = nil
		r.Province = nil
		fields["city"] = resolved{Conflict: true}
	}
	candidates := []api.CatalogEntry{}
	for _, c := range entries {
		if isDemo(c) && !e.AllowDemo {
			continue
		}
		f := c.Fields
		if r.StreetName == nil || r.House == nil || fields["street_name"].Conflict || fields["house"].Conflict {
			continue
		}
		if !e.equivalent(*r.StreetName, catalog.Text(f.StreetName), "street") {
			continue
		}
		if catalog.Canon(*r.House) != catalog.Canon(catalog.Text(f.House)) {
			continue
		}
		if fields["city"].Conflict || hasWarning(r.Warnings, "ambiguous_city_reference") {
			continue
		}
		if r.City != nil && !e.equivalent(*r.City, catalog.Text(f.City), "city") {
			continue
		}
		if r.StreetType != nil && catalog.Canon(*r.StreetType) != catalog.Canon(catalog.Text(f.StreetType)) {
			continue
		}
		candidates = append(candidates, c)
	}
	if len(candidates) == 1 {
		c := candidates[0]
		f := c.Fields
		// Never overwrite explicit incompatible administrative evidence with a building default.
		conflict := false
		for _, pair := range [][2]*string{{r.Province, f.Province}, {r.District, f.District}, {r.LivingArea, f.LivingArea}} {
			if pair[0] != nil && pair[1] != nil && reference.Canon(*pair[0]) != reference.Canon(*pair[1]) {
				conflict = true
			}
		}
		if !conflict {
			for name, value := range map[string]*string{"city": f.City, "province": f.Province, "district": f.District, "living_area": f.LivingArea, "street_type": f.StreetType} {
				if fields[name].Value == "" && value != nil {
					fields[name] = resolved{Value: *value, Score: 0.85}
				}
			}
			apply(&r, fields)
			r.CatalogId = &c.Id
			r.CatalogRevision = &c.Revision
			r.Coordinates = &api.NullableCoordinates{Latitude: c.Coordinates.Latitude, Longitude: c.Coordinates.Longitude}
			if isDemo(c) {
				r.Warnings = append(r.Warnings, "synthetic_catalog_coordinates")
			}
		} else {
			r.Warnings = append(r.Warnings, "building_reference_conflict")
		}
	} else if len(candidates) > 1 {
		r.Warnings = append(r.Warnings, "ambiguous_catalog_match")
	}
	if r.Coordinates == nil {
		r.Warnings = append(r.Warnings, "coordinates_unknown")
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
		if warning == "libpostal_unavailable" || warning == "gliner_unavailable" {
			r.Confidence *= 0.95
		}
	}
	if r.Coordinates != nil && r.Confidence >= 0.72 && !hasWarning(r.Warnings, "synthetic_catalog_coordinates") {
		r.Status = api.Matched
	}
	if hasWarning(r.Warnings, "synthetic_catalog_coordinates") {
		r.Confidence = math.Min(r.Confidence, 0.69)
		r.Status = api.NeedsReview
	}
	return r
}
func isDemo(c api.CatalogEntry) bool {
	return !c.Confirmed && strings.HasPrefix(c.Source, "synthetic-")
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
