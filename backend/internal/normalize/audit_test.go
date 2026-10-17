package normalize

// Opt-in measurements use actual prepared models. Ordinary unit tests stay offline
// and lightweight. Raw organizer data and reports must remain outside Git.
import (
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
	e := Engine{Catalog: s, Postal: p, Reference: d, AllowDemo: false}
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
	type mode struct {
		Name                     string
		Postal, Rules, Reference bool
	}
	modes := []mode{{"full", true, true, true}, {"without_postal", false, true, true}, {"without_rules", true, false, true}, {"without_reference", true, true, false}, {"rules_only", false, true, false}, {"postal_only", true, false, false}, {"without_working", true, true, true}}
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
		measure := &measurement{}
		start = time.Now()
		for i, c := range cases {
			work, components := texts[i], parts[i]
			if m.Name == "without_working" {
				work, components = rawTexts[i], rawParts[i]
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
			r := engine.reconcile(records[i], work, signals, warnings)
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
			if hasWarning(r.Warnings, "libpostal_unavailable") {
				t.Fatal("degraded benchmark")
			}
		}
		batchTimings = append(batchTimings, map[string]any{"size": size, "seconds": elapsed, "records_per_second": float64(size) / elapsed, "semantic_differences": differences})
	}
	report := map[string]any{"cases": len(cases), "reference_entities": d.Count(), "allow_demo_catalog": false, "scope": "Controlled fixture ablation of libpostal, rules and references; not held-out accuracy. Timings are single runs.", "postal_seconds": postalSeconds, "modes": measurements, "batch": batchTimings, "postal_components": parts}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d-case real-model audit to %s", len(cases), output)
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
