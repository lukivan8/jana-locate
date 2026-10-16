package normalize

// Opt-in measurements use actual prepared models. Ordinary unit tests stay offline
// and lightweight. Raw organizer data and reports must remain outside Git.
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/postal"
	"github.com/lukivan8/jana-address/internal/reference"
	"github.com/lukivan8/jana-address/pkg/api"
)

type auditCase struct {
	Name string            `json:"name"`
	Raw  string            `json:"raw"`
	Want map[string]string `json:"want"` // Empty string explicitly expects null.
}

func TestNormalizationAudit(t *testing.T) {
	output := os.Getenv("NORMALIZATION_AUDIT_OUTPUT")
	if output == "" {
		t.Skip("opt-in real-model ablation; see docs/normalization-audit.md")
	}
	read := func(path string, target any) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(data, target); err != nil {
			t.Fatal(err)
		}
	}
	var cases []auditCase
	read(os.Getenv("NORMALIZATION_AUDIT_CASES"), &cases)
	if len(cases) == 0 {
		t.Fatal("empty audit corpus")
	}
	s, err := catalog.Open("../../data/catalog.seed.json", filepath.Join(t.TempDir(), "overlay.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := postal.Open(os.Getenv("LIBPOSTAL_TEST_DATA"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := reference.Open(os.Getenv("NORMALIZATION_AUDIT_REFERENCE"))
	if err != nil {
		t.Fatal(err)
	}
	g := &GLiNER{URL: os.Getenv("NORMALIZATION_AUDIT_GLINER")}
	e := Engine{Catalog: s, Postal: p, GLiNER: g, Reference: d, AllowDemo: false}
	records := make([]api.InputRecord, len(cases))
	texts := make([]string, len(cases))
	parts := make([][]postal.Component, len(cases))
	start := time.Now()
	for i, c := range cases {
		records[i] = api.InputRecord{Id: i, RawAddress: c.Raw}
		texts[i] = Working(c.Raw)
		if texts[i] != "" {
			parts[i], err = p.Parse(texts[i])
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	postalSeconds := time.Since(start).Seconds()
	start = time.Now()
	rows, err := g.Extract(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}
	glinerSeconds := time.Since(start).Seconds()
	// Keep separate real predictions for the preprocessing counterfactual.
	rawTexts := make([]string, len(cases))
	rawParts := make([][]postal.Component, len(cases))
	for i, c := range cases {
		rawTexts[i] = c.Raw
		if c.Raw != "" {
			rawParts[i], err = p.Parse(c.Raw)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	rawRows, err := g.Extract(context.Background(), rawTexts)
	if err != nil {
		t.Fatal(err)
	}
	type mode struct {
		Name                            string
		Postal, Rules, Model, Reference bool
	}
	modes := []mode{{"full", true, true, true, true}, {"without_postal", false, true, true, true}, {"without_gliner", true, true, false, true}, {"without_rules", true, false, true, true}, {"without_reference", true, true, true, false}, {"rules_only", false, true, false, false}, {"postal_only", true, false, false, false}, {"gliner_only", false, false, true, false}, {"without_working", true, true, true, true}}
	type measurement struct {
		Fields, Correct, NonNull, NonNullCorrect, Null, NullCorrect, ExactRecords, ChangedFields int
		Seconds                                                                                  float64
		Results                                                                                  []api.Result
		Mismatches                                                                               []map[string]any
	}
	measurements := map[string]*measurement{}
	var full []api.Result
	for _, m := range modes {
		engine := e
		if !m.Reference {
			engine.Reference = nil
		}
		entry := engine.Catalog.List()
		measure := &measurement{}
		start = time.Now()
		for i, c := range cases {
			work, components, entities := texts[i], parts[i], rows[i]
			if m.Name == "without_working" {
				work, components, entities = rawTexts[i], rawParts[i], rawRows[i]
			}
			signals := []evidence{}
			warnings := []string{}
			if m.Postal {
				signals = append(signals, postalSignals(work, components)...)
			} else if texts[i] != "" {
				warnings = append(warnings, "libpostal_unavailable")
			}
			if m.Rules {
				signals = append(signals, ruleSignals(work)...)
			}
			if m.Model {
				for _, entity := range entities {
					if ev, ok := modelSignal(entity); ok {
						signals = append(signals, ev)
					}
				}
			} else if texts[i] != "" {
				warnings = append(warnings, "gliner_unavailable")
			}
			r := engine.reconcile(records[i], work, signals, entry, warnings)
			measure.Results = append(measure.Results, r)
			values := resultValues(r)
			exact := true
			for field, want := range c.Want {
				got := values[field]
				equal := reference.Canon(got) == reference.Canon(want)
				// Numeric identities must retain slash, hyphen and letter exactly.
				if field == "house" || field == "apartment" || field == "entrance" || field == "floor" {
					equal = catalog.Canon(got) == catalog.Canon(want)
				}
				measure.Fields++
				if equal {
					measure.Correct++
				} else {
					exact = false
					measure.Mismatches = append(measure.Mismatches, map[string]any{"case": c.Name, "field": field, "got": got, "want": want})
				}
				if want != "" {
					measure.NonNull++
					if equal {
						measure.NonNullCorrect++
					}
				} else {
					measure.Null++
					if equal {
						measure.NullCorrect++
					}
				}
			}
			if exact {
				measure.ExactRecords++
			}
			if r.Id != records[i].Id || r.RawAddress != c.Raw {
				t.Fatal("input identity changed")
			}
			if r.Coordinates != nil {
				t.Fatal("synthetic catalog leaked into evaluation")
			}
			if full != nil {
				baseline := resultValues(full[i])
				for field, value := range values {
					if baseline[field] != value {
						measure.ChangedFields++
					}
				}
			}
		}
		measure.Seconds = time.Since(start).Seconds()
		measurements[m.Name] = measure
		if m.Name == "full" {
			full = measure.Results
		}
	}
	// Transport batching should preserve all semantic fields. Float summation and
	// warning map iteration are intentionally excluded from exact comparisons.
	batchTimings := []map[string]any{}
	for _, size := range []int{1, 8, 32, 128, 1000} {
		if os.Getenv("NORMALIZATION_AUDIT_SKIP_BATCH") == "true" {
			break
		}
		batch := make([]api.InputRecord, size)
		for i := range batch {
			batch[i] = records[i%len(records)]
			batch[i].Id = i
		}
		start = time.Now()
		result := e.NormalizeBatch(batch)
		elapsed := time.Since(start).Seconds()
		differences := 0
		for i, r := range result {
			base := full[i%len(full)]
			if !reflect.DeepEqual(resultValues(r), resultValues(base)) {
				differences++
			}
			if r.Id != batch[i].Id || r.RawAddress != batch[i].RawAddress {
				t.Fatal("batch identity changed")
			}
			if hasWarning(r.Warnings, "gliner_unavailable") || hasWarning(r.Warnings, "libpostal_unavailable") {
				t.Fatal("degraded benchmark")
			}
		}
		batchTimings = append(batchTimings, map[string]any{"size": size, "seconds": elapsed, "records_per_second": float64(size) / elapsed, "semantic_differences": differences})
	}
	withoutModel := e
	withoutModel.GLiNER = nil
	fastRecords := make([]api.InputRecord, 1000)
	for i := range fastRecords {
		fastRecords[i] = records[i%len(records)]
		fastRecords[i].Id = i
	}
	start = time.Now()
	fastResults := withoutModel.NormalizeBatch(fastRecords)
	fastSeconds := time.Since(start).Seconds()
	fastDifferences := 0
	for i, r := range fastResults {
		if !reflect.DeepEqual(resultValues(r), resultValues(full[i%len(full)])) {
			fastDifferences++
		}
	}
	report := map[string]any{"cases": len(cases), "reference_entities": d.Count(), "allow_demo_catalog": false, "scope": "Controlled fixture ablation; frozen real model evidence, not held-out accuracy. Timings are single runs.", "postal_seconds": postalSeconds, "gliner_seconds": glinerSeconds, "modes": measurements, "batch": batchTimings, "without_gliner_batch": map[string]any{"size": 1000, "seconds": fastSeconds, "semantic_differences": fastDifferences, "note": "warnings and confidence differ; fixture-only speed comparison"}, "postal_components": parts, "gliner_entities": rows}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d-case real-model audit to %s", len(cases), output)
}

func TestGeocoderSafetyAudit(t *testing.T) {
	output := os.Getenv("GEOCODER_AUDIT_OUTPUT")
	if output == "" {
		t.Skip("opt-in geocoder safety audit with isolated synthetic confirmations")
	}
	s, err := catalog.Open("../../data/catalog.seed.json", filepath.Join(t.TempDir(), "overlay.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := postal.Open(os.Getenv("LIBPOSTAL_TEST_DATA"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := reference.Open(os.Getenv("NORMALIZATION_AUDIT_REFERENCE"))
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{Catalog: s, Postal: p, GLiNER: &GLiNER{URL: os.Getenv("NORMALIZATION_AUDIT_GLINER")}, Reference: d}
	// These coordinates are fabricated for matching tests, never geographic truth.
	for _, city := range []string{"Астана", "Алматы"} {
		_, err = s.Confirm(api.ConfirmationRequest{Fields: api.AddressFields{City: str(city), StreetType: str("улица"), StreetName: str("Тестовая"), House: str("12")}, Coordinates: api.Coordinates{Latitude: 50, Longitude: 70}})
		if err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		Name, Raw       string
		WantCoordinates bool
	}{
		{"unique_city", "Астана, улица Тестовая 12 кв 9", true},
		{"ambiguous_missing_city", "улица Тестовая 12", false},
		{"unknown_city", "Шымкент, улица Тестовая 12", false},
		{"apartment_is_not_house", "Астана, улица Тестовая кв 12", false},
		{"unknown_house", "Астана, улица Тестовая 999", false},
		{"conflicting_cities", "Астана, Алматы, улица Тестовая 12", false},
		{"conflicting_explicit_houses", "Астана, улица Тестовая дом 12 дом 13", false},
		{"conflicting_types", "Астана, проспект Тестовая 12", false},
		{"fraction_is_not_integer", "Астана, улица Тестовая 12/1", false},
		{"corpus_is_not_base_building", "Астана, улица Тестовая дом 12, корпус 2", false},
	}
	records := make([]api.InputRecord, len(tests))
	for i, c := range tests {
		records[i] = api.InputRecord{Id: c.Name, RawAddress: c.Raw}
	}
	results := e.NormalizeBatch(records)
	findings := []map[string]any{}
	failures := 0
	for i, r := range results {
		correct := (r.Coordinates != nil) == tests[i].WantCoordinates
		if !correct {
			failures++
		}
		findings = append(findings, map[string]any{"name": tests[i].Name, "want_coordinates": tests[i].WantCoordinates, "passed": correct, "result": r})
		if hasWarning(r.Warnings, "libpostal_unavailable") || hasWarning(r.Warnings, "gliner_unavailable") {
			t.Fatal("degraded safety audit")
		}
	}
	data, err := json.MarshalIndent(map[string]any{"scope": "Synthetic buildings and fabricated coordinates in a temporary overlay; tests matching safety, not geographic accuracy.", "cases": len(tests), "failures": failures, "findings": findings}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	// This is an observational audit: findings are reported, not made into a
	// production regression contract that silently blesses existing defects.
	t.Logf("geocoder safety: %d/%d mismatches; %s", failures, len(tests), output)
}

func TestBatchChunkAudit(t *testing.T) {
	output := os.Getenv("CHUNK_AUDIT_OUTPUT")
	if output == "" {
		t.Skip("opt-in real GLiNER chunk measurements")
	}
	var cases []auditCase
	data, err := os.ReadFile(os.Getenv("NORMALIZATION_AUDIT_CASES"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("empty corpus")
	}
	texts := make([]string, 128)
	for i := range texts {
		texts[i] = Working(cases[i%len(cases)].Raw)
	}
	var baseline [][]Entity
	measurements := []map[string]any{}
	for _, chunk := range []int{8, 32, 128} {
		g := &GLiNER{URL: os.Getenv("NORMALIZATION_AUDIT_GLINER"), ChunkSize: chunk}
		start := time.Now()
		rows, err := g.Extract(context.Background(), texts)
		if err != nil {
			t.Fatal(err)
		}
		elapsed := time.Since(start).Seconds()
		differences := 0
		if baseline == nil {
			baseline = rows
		} else {
			for i, row := range rows {
				if !reflect.DeepEqual(row, baseline[i]) {
					differences++
				}
			}
		}
		measurements = append(measurements, map[string]any{"chunk": chunk, "records": len(texts), "seconds": elapsed, "records_per_second": float64(len(texts)) / elapsed, "rows_different_including_scores": differences})
	}
	data, err = json.MarshalIndent(measurements, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkAuditCatalogScaling(b *testing.B) {
	for _, count := range []int{0, 100, 1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			entries := make([]api.CatalogEntry, count)
			for i := range entries {
				entries[i] = api.CatalogEntry{Fields: api.AddressFields{City: str("Астана"), StreetType: str("улица"), StreetName: str(fmt.Sprintf("Тестовая %d", i)), House: str("1")}, Coordinates: api.Coordinates{Latitude: 50, Longitude: 70}, Source: "audit-fabricated-fixture"}
			}
			data, err := json.Marshal(api.CatalogResponse{Entries: entries})
			if err != nil {
				b.Fatal(err)
			}
			seed := filepath.Join(b.TempDir(), "seed.json")
			if err = os.WriteFile(seed, data, 0600); err != nil {
				b.Fatal(err)
			}
			s, err := catalog.Open(seed, filepath.Join(b.TempDir(), "overlay.json"))
			if err != nil {
				b.Fatal(err)
			}
			e := Engine{Catalog: s}
			records := make([]api.InputRecord, 100)
			for i := range records {
				records[i] = api.InputRecord{Id: i, RawAddress: fmt.Sprintf("Астана, улица Тестовая %d дом 1 кв 4", i)}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				e.NormalizeBatch(records)
			}
		})
	}
}
