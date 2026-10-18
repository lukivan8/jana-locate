package normalize

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lukivan8/jana-address/pkg/api"
)

const ContextualVersion = "contextual-v7-grounded-microalpha-lean-v1"

func (e Engine) ReferenceVersion() string {
	v := e.Reference.Version()
	if e.Contextual != nil {
		return v + ":" + ContextualVersion
	}
	return v
}

// ContextualClient talks only to the private, offline inference service.
// It never sends user records to an external provider.
type ContextualClient struct {
	URL  string
	HTTP *http.Client
}

func (c *ContextualClient) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 270 * time.Second}
}
func (c *ContextualClient) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.URL, "/")+"/ready", nil)
	if err != nil {
		return err
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("inference readiness: %d", resp.StatusCode)
	}
	return nil
}
func (e Engine) Ready(ctx context.Context) error {
	if e.Contextual != nil {
		return e.Contextual.Ready(ctx)
	}
	return nil
}
func (e Engine) contextualBatch(ctx context.Context, records []api.InputRecord) []api.Result {
	out, err := e.Contextual.normalize(ctx, records)
	if err != nil {
		out = make([]api.Result, len(records))
		for i, r := range records {
			out[i] = api.Result{Id: r.Id, RawAddress: r.RawAddress, Status: api.NeedsReview, Warnings: []string{"contextual_normalizer_unavailable", "confidence_unavailable"}}
			setContextualMetadata(&out[i], nil)
		}
		return out
	}
	for i := range out {
		r := &out[i]
		e.presentContextual(r)
	}
	return out
}
func setContextualMetadata(r *api.Result, details map[string]interface{}) {
	v := ContextualVersion
	available := false
	r.NormalizerVersion = &v
	r.ConfidenceAvailable = &available
	r.Confidence = 0
	if details != nil {
		r.NormalizationDetails = &details
	}
}
func (c *ContextualClient) normalize(ctx context.Context, records []api.InputRecord) ([]api.Result, error) {
	payload, err := json.Marshal(map[string]interface{}{"records": records})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 270*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+"/normalize", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("inference status %d", resp.StatusCode)
	}
	var body struct {
		Results []json.RawMessage `json:"results"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 128<<20))
	decoder.UseNumber()
	if err = decoder.Decode(&body); err != nil {
		return nil, err
	}
	if len(body.Results) != len(records) {
		return nil, fmt.Errorf("inference record count mismatch")
	}
	out := make([]api.Result, len(records))
	for i, raw := range body.Results {
		var details map[string]interface{}
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if err = d.Decode(&details); err != nil {
			return nil, err
		}
		a, _ := json.Marshal(details["id"])
		b, _ := json.Marshal(records[i].Id)
		if !bytes.Equal(a, b) || details["raw_address"] != records[i].RawAddress {
			return nil, fmt.Errorf("inference input integrity mismatch")
		}
		// Nullable model confidence is deliberately not interpreted as a probability.
		if err = json.Unmarshal(raw, &out[i]); err != nil {
			return nil, err
		}
		out[i].Id = records[i].Id
		out[i].RawAddress = records[i].RawAddress
		out[i].Status = api.NeedsReview
		out[i].Coordinates = nil
		out[i].CatalogId = nil
		out[i].CatalogRevision = nil
		out[i].Warnings = append(out[i].Warnings, "confidence_unavailable")
		delete(details, "id")
		delete(details, "raw_address")
		setContextualMetadata(&out[i], details)
	}
	return out, nil
}
func (e Engine) contextualCities(r api.Result) []api.CityCandidate {
	out := []api.CityCandidate{}
	if r.NormalizationDetails == nil {
		return out
	}
	data, _ := json.Marshal((*r.NormalizationDetails)["canonical_candidates"])
	var fields map[string][]struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		ProvinceID string `json:"province_id"`
	}
	if json.Unmarshal(data, &fields) != nil {
		return out
	}
	for _, c := range fields["city"] {
		var province *string
		var typ *string
		source := "prepared_reference"
		if p, ok := e.Reference.Entity(c.ProvinceID); ok {
			province = &p.Name
		}
		if v, ok := e.Reference.Entity(c.ID); ok {
			typ = v.SettlementType()
			source = v.Source
		}
		out = append(out, api.CityCandidate{EntityId: c.ID, Name: c.Name, ProvinceId: c.ProvinceID, Province: province, SettlementType: typ, Source: source})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntityId < out[j].EntityId })
	return out
}

// Common display names do not select an identity or remove homonyms.
func (e Engine) presentContextual(r *api.Result) {
	cities := e.contextualCities(*r)
	names := map[string]bool{}
	provinces := map[string]bool{}
	for _, c := range cities {
		names[c.Name] = true
		if c.Province != nil {
			provinces[*c.Province] = true
		} else {
			provinces[""] = true
		}
	}
	if len(names) == 1 {
		for n := range names {
			r.City = &n
		}
	}
	if r.Province == nil && len(provinces) == 1 {
		for p := range provinces {
			if p != "" {
				r.Province = &p
			}
		}
	}
	if len(cities) > 1 {
		r.Warnings = append(r.Warnings, "ambiguous_city_reference")
	}
}

// UnavailableResults distinguishes a transport failure from an ambiguous address.
func UnavailableResults(results []api.Result) bool {
	for _, r := range results {
		for _, w := range r.Warnings {
			if w == "contextual_normalizer_unavailable" {
				return true
			}
		}
	}
	return false
}

// CityCandidatesChecked never represents an inference outage as an empty candidate list.
func (e Engine) CityCandidatesChecked(ctx context.Context, in api.InputRecord) ([]api.CityCandidate, error) {
	if e.Contextual == nil {
		return e.CityCandidates(ctx, in), ctx.Err()
	}
	results, err := e.Contextual.normalize(ctx, []api.InputRecord{in})
	if err != nil {
		return nil, err
	}
	return e.contextualCities(results[0]), nil
}

// CityCandidatesFromResult presents evidence of an already completed job for review.
// NormalizeBatch never reads this journal or looks up an answer by ID/raw address.
func (e Engine) CityCandidatesFromResult(result api.Result) ([]api.CityCandidate, bool) {
	if e.Contextual == nil || result.NormalizerVersion == nil || *result.NormalizerVersion != ContextualVersion || result.NormalizationDetails == nil {
		return nil, false
	}
	if _, ok := (*result.NormalizationDetails)["canonical_candidates"]; !ok {
		return nil, false
	}
	return e.contextualCities(result), true
}
