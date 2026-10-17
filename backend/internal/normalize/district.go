package normalize

import (
	"sort"
	"strings"

	"github.com/lukivan8/jana-address/internal/reference"
)

type textRange struct{ start, end int }

func overlaps(start, end int, ranges []textRange) bool {
	for _, span := range ranges {
		if start < span.end && end > span.start {
			return true
		}
	}
	return false
}
func rangesFor(text, value string) []textRange {
	out := []textRange{}
	value = surface(text, value)
	if value == "" {
		return out
	}
	for offset := 0; offset < len(text); {
		index := strings.Index(text[offset:], value)
		if index < 0 {
			break
		}
		start := offset + index
		out = append(out, textRange{start, start + len(value)})
		offset = start + len(value)
	}
	return out
}
func districtQualifier(value string) bool {
	for _, token := range tokens(value) {
		if token == "район" || token == "ауданы" {
			return true
		}
	}
	return false
}

// Domain role reconciliation: use parser/source spans to exclude street/venue
// names, then mask qualified district mentions only for settlement lookup.
// No country/language predictor or per-address answer cache is involved.
func (e Engine) districtMentions(text string, signals []evidence) ([]reference.Match, []evidence, string, []evidence) {
	blocked := []textRange{}
	mentions := e.Reference.Mentions(text)
	for _, signal := range signals {
		if signal.Field == "_road" || signal.Field == "_venue" || signal.Field == "landmark" {
			for _, span := range rangesFor(text, signal.Value) {
				qualifiedDistrict := false
				if signal.Source == "libpostal" {
					for _, match := range mentions {
						// libpostal may label a complete qualified district as a
						// venue. An exact source district is stronger evidence; a
						// venue containing that name ("кафе район ...") is not.
						roleCompatible := signal.Field == "_road" && span.start >= match.Start && span.end <= match.End ||
							signal.Field == "_venue" && span.start == match.Start && span.end == match.End
						if match.Entity.IsDistrict() && districtQualifier(match.Surface) && roleCompatible {
							qualifiedDistrict = true
						}
					}
				}
				if !qualifiedDistrict {
					blocked = append(blocked, span)
				}
			}
		}
	}
	model, kept := []evidence{}, []evidence{}
	for _, signal := range signals {
		if strings.HasPrefix(signal.Field, "_") {
			continue
		}
		if signal.Field == "district" {
			spans := rangesFor(text, signal.Value)
			for _, span := range spans {
				if !overlaps(span.start, span.end, blocked) {
					model = append(model, signal)
					break
				}
			}
		} else {
			kept = append(kept, signal)
		}
	}
	matches := []reference.Match{}
	cityText := []byte(text)
	for _, signal := range signals {
		if signal.Field == "_venue" {
			for _, span := range rangesFor(text, signal.Value) {
				// libpostal can label a bare settlement component as a venue
				// (e.g. "Сарань, улица Жамбыла 63"). An exact reference
				// settlement occupying the whole component remains geography.
				left := strings.LastIndexAny(text[:span.start], ",;") + 1
				right := len(text)
				if index := strings.IndexAny(text[span.end:], ",;"); index >= 0 {
					right = span.end + index
				}
				component := strings.Trim(text[left:right], " .\t\r\n")
				if signal.Source == "libpostal" && component == strings.Trim(text[span.start:span.end], " .\t\r\n") && len(e.Reference.Exact(component, "city")) > 0 {
					continue
				}
				for i := span.start; i < span.end; i++ {
					cityText[i] = ' '
				}
			}
		}
	}
	for _, match := range mentions {
		if !match.Entity.IsDistrict() || overlaps(match.Start, match.End, blocked) {
			continue
		}
		qualified := districtQualifier(match.Surface)
		grounded := false
		for _, signal := range model {
			if contains(signal.Value, match.Surface) {
				grounded = true
			}
		}
		// A reference name embedded in free-form prose is not itself a claim of
		// administrative location. Accept source-only evidence as an entire
		// comma/semicolon component; the NLP parser can support other orders.
		left := strings.LastIndexAny(text[:match.Start], ",;") + 1
		right := len(text)
		if index := strings.IndexAny(text[match.End:], ",;"); index >= 0 {
			right = match.End + index
		}
		direct := strings.Trim(text[left:right], " .\t\r\n") == strings.Trim(text[match.Start:match.End], " .\t\r\n")
		if !direct && !grounded {
			continue
		}
		if !qualified && len(e.Reference.Exact(match.Surface, "city")) > 0 {
			if !grounded {
				continue
			}
		}
		matches = append(matches, match)
		if qualified {
			for i := match.Start; i < match.End; i++ {
				cityText[i] = ' '
			}
		}
	}
	// A city named Алматы inside "район Алматы" is not a separate city mention.
	filtered := kept[:0]
	for _, signal := range kept {
		if (signal.Field == "city" || signal.Field == "province") && contains(text, signal.Value) && !contains(string(cityText), signal.Value) {
			continue
		}
		filtered = append(filtered, signal)
	}
	return matches, model, string(cityText), filtered
}

func (e Engine) reconcileDistrict(cityID string, cities map[string]reference.Entity, matches []reference.Match, model, signals []evidence, warnings *[]string) []evidence {
	warn := func(value string) {
		if !hasWarning(*warnings, value) {
			*warnings = append(*warnings, value)
		}
	}
	fields := resolve(signals)
	compatible := func(district reference.Entity) bool {
		if cityID != "" {
			return e.Reference.DistrictContainsSettlement(cityID, district.ID)
		}
		if len(cities) > 0 {
			for _, city := range cities {
				if e.Reference.DistrictContainsSettlement(city.ID, district.ID) {
					return true
				}
			}
			return false
		}
		province := fields["province"].Value
		if province != "" {
			p, ok := e.Reference.Entity(district.ProvinceID)
			return ok && e.equivalent(province, p.Name, "province")
		}
		return true
	}
	// Resolve homonyms per mentioned phrase. A second, incompatible explicitly
	// named district remains a conflict instead of being silently discarded.
	groups := map[string]map[string]reference.Entity{}
	for _, match := range matches {
		key := reference.Canon(match.Surface)
		if groups[key] == nil {
			groups[key] = map[string]reference.Entity{}
		}
		groups[key][match.Entity.ID] = match.Entity
	}
	explicit := map[string]reference.Entity{}
	for _, group := range groups {
		retained := map[string]reference.Entity{}
		for id, district := range group {
			if compatible(district) {
				retained[id] = district
			}
		}
		if len(retained) == 0 {
			retained = group
			if cityID != "" || len(cities) > 0 || fields["province"].Value != "" {
				warn("conflicting_district_context")
			}
		}
		for id, district := range retained {
			explicit[id] = district
		}
	}
	names := map[string]string{}
	for _, district := range explicit {
		names[reference.Canon(district.Name)] = district.Name
	}
	// Unknown but grounded parser values remain explicit evidence; known aliases
	// are represented by their source identities above, not independent votes.
	for _, signal := range model {
		represented := false
		for _, district := range explicit {
			if e.equivalent(signal.Value, district.Name, "district") {
				represented = true
			}
		}
		if !represented {
			names[reference.Canon(signal.Value)] = signal.Value
		}
	}
	inferred := e.Reference.DistrictsForAddress(cityID, fields["street_name"].Value, fields["house"].Value, fields["street_type"].Value)
	if len(names) > 0 {
		if len(explicit) > 1 {
			warn("ambiguous_district_reference")
		}
		if len(names) > 1 {
			warn("conflicting_district")
			return signals
		}
		value := ""
		for _, name := range names {
			value = name
		}
		if inferred.Complete && len(inferred.Candidates) > 0 {
			agrees := false
			for _, district := range inferred.Candidates {
				if e.equivalent(value, district.Name, "district") {
					agrees = true
				}
			}
			if !agrees {
				warn("conflicting_district")
			}
		}
		score, source := 0.90, "reference-explicit"
		if len(explicit) == 0 {
			score = 0
			for _, signal := range model {
				if signal.Score > score {
					score = signal.Score
					source = signal.Source
				}
			}
		}
		return append(signals, evidence{"district", value, source, score})
	}
	if len(inferred.Candidates) == 0 {
		return signals
	}
	if !inferred.Complete {
		warn("district_coverage_incomplete")
		return signals
	}
	if len(inferred.Candidates) > 1 {
		warn("ambiguous_district_reference")
		return signals
	}
	// Exactly one identity with positive coverage, and city identity is resolved.
	sort.Slice(inferred.Candidates, func(i, j int) bool { return inferred.Candidates[i].ID < inferred.Candidates[j].ID })
	warn("district_inferred_from_" + inferred.Source)
	return append(signals, evidence{"district", inferred.Candidates[0].Name, "reference-" + inferred.Source, 0.90})
}

// Narrow only already-mentioned settlements, using positive KATO hierarchy.
// A contradictory phrase does not erase every city and fall back to an LLM guess.
func (e Engine) citiesInDistricts(cities map[string]reference.Entity, matches []reference.Match) map[string]reference.Entity {
	if len(matches) == 0 {
		return cities
	}
	groups := map[string][]string{}
	for _, match := range matches {
		key := reference.Canon(match.Surface)
		groups[key] = append(groups[key], match.Entity.ID)
	}
	compatible := map[string]reference.Entity{}
	for id, city := range cities {
		all := true
		for _, ids := range groups {
			found := false
			for _, districtID := range ids {
				if e.Reference.DistrictContainsSettlement(id, districtID) {
					found = true
				}
			}
			all = all && found
		}
		if all {
			compatible[id] = city
		}
	}
	if len(compatible) > 0 {
		return compatible
	}
	return cities
}
