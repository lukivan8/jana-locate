package normalize

import (
	"context"
	"errors"
	"sort"

	"github.com/lukivan8/jana-address/internal/reference"
	"github.com/lukivan8/jana-address/pkg/api"
)

var ErrInvalidCitySelection = errors.New("settlement is not a compatible candidate in the original address")

// CityCandidates exposes source identities, not a popularity-ranked geocoder result.
// Explicit province mentions constrain choices; street/house absence is not negative evidence.
func (e Engine) CityCandidates(ctx context.Context, in api.InputRecord) []api.CityCandidate {
	if e.Contextual != nil {
		return e.contextualCities(e.NormalizeBatchContext(ctx, []api.InputRecord{in})[0])
	}
	out := []api.CityCandidate{}
	work := Working(in.RawAddress)
	administrative := administrativeText(work)
	signals, _ := e.extractionSignals(work)
	signals = administrativeSignals(work, administrative, signals)
	districts, _, cityText, _ := e.districtMentions(administrative, signals)
	mentions := e.Reference.Matches(cityText)
	provinces := map[string]bool{}
	cities := map[string]reference.Entity{}
	for _, m := range mentions {
		if m.Entity.Kind == "province" {
			provinces[m.Entity.ID] = true
		}
	}
	if len(provinces) > 1 {
		return out
	}
	// The same extraction as normalization prevents street names containing a city
	// from introducing unrelated candidate identities. Never reuse manually edited results.
	r := e.NormalizeBatchContext(ctx, []api.InputRecord{in})[0]
	for _, m := range mentions {
		c := m.Entity
		if c.Kind != "city" {
			continue
		}
		if r.City != nil && !e.equivalent(*r.City, c.Name, "city") {
			continue
		}
		if r.StreetName != nil && contains(*r.StreetName, m.Surface) && (r.City == nil || !e.equivalent(*r.City, c.Name, "city")) {
			continue
		}
		if len(provinces) > 0 && !provinces[c.ProvinceID] {
			continue
		}
		cities[c.ID] = c
	}
	cities = compatibleMentionedCities(e.Reference, mentions, cities)
	cities = e.citiesInDistricts(cities, districts)
	for _, c := range cities {
		var province *string
		if p, ok := e.Reference.Entity(c.ProvinceID); ok {
			province = &p.Name
		}
		out = append(out, api.CityCandidate{EntityId: c.ID, Name: c.Name, SettlementType: c.SettlementType(), Province: province, ProvinceId: c.ProvinceID, Source: c.Source})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntityId < out[j].EntityId })
	return out
}

// All retained settlement mentions must support the same identity. In particular,
// a source-qualified alias (г.Караганда) constrains its shorter bare-name match.
// Share this rule between normalization and selectable candidates.
func compatibleMentionedCities(directory *reference.Directory, mentions []reference.Match, cities map[string]reference.Entity) map[string]reference.Entity {
	surfaces := []string{}
	for _, m := range mentions {
		if _, eligible := cities[m.Entity.ID]; eligible {
			surfaces = append(surfaces, m.Surface)
		}
	}
	for _, surface := range surfaces {
		ids := map[string]bool{}
		for _, entity := range directory.Exact(surface, "city") {
			ids[entity.ID] = true
		}
		for id := range cities {
			if !ids[id] {
				delete(cities, id)
			}
		}
	}
	return cities
}

func (e Engine) SelectCity(ctx context.Context, in api.InputRecord, id string) (api.Result, error) {
	if e.Contextual != nil {
		results := e.NormalizeBatchContext(ctx, []api.InputRecord{in})
		if UnavailableResults(results) {
			return api.Result{}, errors.New("normalization service unavailable")
		}
		r := results[0]
		for _, candidate := range e.contextualCities(r) {
			if candidate.EntityId != id {
				continue
			}
			r.City = &candidate.Name
			r.Province = candidate.Province
			if r.District == nil {
				value := func(p *string) string {
					if p == nil {
						return ""
					}
					return *p
				}
				inferred := e.Reference.DistrictsForAddress(id, value(r.StreetName), value(r.House), value(r.StreetType))
				if inferred.Complete && len(inferred.Candidates) == 1 {
					r.District = &inferred.Candidates[0].Name
					r.Warnings = append(r.Warnings, "district_inferred_from_"+inferred.Source)
				}
				if len(inferred.Candidates) > 1 {
					r.Warnings = append(r.Warnings, "ambiguous_district_reference")
				}
				if !inferred.Complete && len(inferred.Candidates) > 0 {
					r.Warnings = append(r.Warnings, "district_coverage_incomplete")
				}
			}
			if r.NormalizationDetails != nil {
				(*r.NormalizationDetails)["operator_selected_city"] = id
			}
			r.Warnings = append(r.Warnings, "city_selected_by_operator")
			return r, nil
		}
		return api.Result{}, ErrInvalidCitySelection
	}

	if err := ctx.Err(); err != nil {
		return api.Result{}, err
	}
	valid := false
	for _, c := range e.CityCandidates(ctx, in) {
		if c.EntityId == id {
			valid = true
		}
	}
	if err := ctx.Err(); err != nil {
		return api.Result{}, err
	}
	if !valid {
		return api.Result{}, ErrInvalidCitySelection
	}
	e.selectedCity = id
	r := e.NormalizeBatchContext(ctx, []api.InputRecord{in})[0]
	if err := ctx.Err(); err != nil {
		return api.Result{}, err
	}
	r.Warnings = append(r.Warnings, "city_selected_by_operator")
	return r, nil
}
