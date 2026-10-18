package normalize

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/lukivan8/jana-address/pkg/api"
)

const NumericSupplementVersion = "native-numeric-crf-v1"

// NumericClient is an optional offline numeric-only tagger. It never replaces
// native results, changes reference identities or supplies an address confidence.
type NumericClient struct {
	URL  string
	HTTP *http.Client
}
type numericProposal struct {
	Label     string  `json:"label"`
	Start     int     `json:"start"`
	End       int     `json:"end"`
	Text      string  `json:"text"`
	Posterior float64 `json:"posterior"`
}
type numericRow struct {
	ID        json.RawMessage   `json:"id"`
	Raw       string            `json:"raw_address"`
	Proposals []numericProposal `json:"proposals"`
}

func (c *NumericClient) proposals(ctx context.Context, records []api.InputRecord) ([]numericRow, error) {
	payload, err := json.Marshal(map[string]interface{}{"records": records})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+"/numeric", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("numeric supplement status %d", resp.StatusCode)
	}
	var body struct {
		Results []numericRow `json:"results"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&body); err != nil {
		return nil, err
	}
	if len(body.Results) != len(records) {
		return nil, fmt.Errorf("numeric supplement record count mismatch")
	}
	for i, r := range body.Results {
		id, _ := json.Marshal(records[i].Id)
		if !bytes.Equal(id, r.ID) || r.Raw != records[i].RawAddress {
			return nil, fmt.Errorf("numeric supplement input mismatch")
		}
		raw := []rune(r.Raw)
		for _, p := range r.Proposals {
			if p.Start < 0 || p.End <= p.Start || p.End > len(raw) || string(raw[p.Start:p.End]) != p.Text || p.Posterior < 0 || p.Posterior > 1 {
				return nil, fmt.Errorf("numeric supplement invalid source span")
			}
		}
	}
	return body.Results, nil
}

func (e Engine) supplementNumeric(ctx context.Context, records []api.InputRecord, baseline []api.Result) []api.Result {
	rows, err := e.NumericSupplement.proposals(ctx, records)
	if err != nil {
		// Supplement failure must not turn a valid native response into a 503 or
		// discard its fields. Never log raw addresses.
		slog.Warn("numeric supplement unavailable; preserving native results", "error", err)
		return baseline
	}
	for i, row := range rows {
		baseline[i] = addNumericProposals(baseline[i], row.Proposals)
	}
	return baseline
}

func addNumericProposals(b api.Result, proposals []numericProposal) api.Result {
	for _, w := range b.Warnings {
		if strings.Contains(w, "ambig") || strings.Contains(w, "conflict") {
			return b
		}
	}
	fields := map[string]**string{"house": &b.House, "apartment": &b.Apartment, "entrance": &b.Entrance, "floor": &b.Floor}
	original := map[string]*string{"house": b.House, "apartment": b.Apartment, "entrance": b.Entrance, "floor": b.Floor}
	grouped := map[string][]numericProposal{}
	for _, p := range proposals {
		if _, ok := fields[p.Label]; ok {
			grouped[p.Label] = append(grouped[p.Label], p)
		}
	}
	accepted := []numericProposal{}
	for _, field := range []string{"house", "apartment", "entrance", "floor"} {
		ps := grouped[field]
		if original[field] != nil || len(ps) != 1 {
			continue
		}
		p := ps[0]
		if p.Posterior < .99 {
			continue
		}
		hasDigit := false
		for _, c := range p.Text {
			hasDigit = hasDigit || unicode.IsDigit(c)
		}
		if !hasDigit {
			continue
		}
		reused := false
		for k, v := range original {
			if k != field && v != nil && *v == p.Text {
				reused = true
			}
		}
		if reused {
			continue
		}
		// Protect source ranges of existing nonnumeric fields; do not select an
		// identity, reinterpret a street number, or mine a number out of a landmark.
		raw := strings.ToLower(b.RawAddress)
		overlap := false
		for _, v := range []*string{b.City, b.Province, b.District, b.LivingArea, b.StreetName, b.Landmark} {
			if v == nil {
				continue
			}
			a := strings.Index(raw, strings.ToLower(*v))
			if a < 0 {
				continue
			}
			start := len([]rune(raw[:a]))
			end := start + len([]rune(*v))
			if start < p.End && p.Start < end {
				overlap = true
			}
		}
		if overlap {
			continue
		}
		value := p.Text
		*fields[field] = &value
		accepted = append(accepted, p)
	}
	if len(accepted) > 0 {
		details := map[string]interface{}{}
		if b.NormalizationDetails != nil {
			for k, v := range *b.NormalizationDetails {
				details[k] = v
			}
		}
		details["numeric_supplement"] = map[string]interface{}{"version": NumericSupplementVersion, "accepted": accepted, "confidence_scope": "unchanged_native_evidence", "baseline_numeric": original}
		b.NormalizationDetails = &details
		version := NumericSupplementVersion
		b.NormalizerVersion = &version
	}
	return b
}
