package catalog

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lukivan8/jana-address/pkg/api"
)

func newJobs(t *testing.T) *JobStore {
	t.Helper()
	s, err := OpenJobs(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func completedJob(t *testing.T, s *JobStore) api.NormalizationJob {
	t.Helper()
	records := []api.InputRecord{{Id: json.Number("9223372036854775807"), RawAddress: "  Астана\nТуран 55/6  "}, {Id: json.Number("9223372036854775807"), RawAddress: ""}}
	j, err := s.Create(records)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(); err != nil {
		t.Fatal(err)
	}
	results := []api.Result{}
	for _, r := range records {
		results = append(results, api.Result{Id: r.Id, RawAddress: r.RawAddress, Status: api.Unmatched, Warnings: []string{"coordinates_unknown"}})
	}
	if err = s.Finish(j.Id, results, ""); err != nil {
		t.Fatal(err)
	}
	j, err = s.Get(j.Id)
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func TestJobsPersistReviewRestartAndIsolateRecords(t *testing.T) {
	s := newJobs(t)
	j := completedJob(t, s)
	apartment := "99"
	city := "Проверенный город"
	first, err := s.Review(j.Id, 0, j.Revision, api.AddressFields{City: &city, Apartment: &apartment}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Review(j.Id, 1, first.Revision, api.AddressFields{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.ReviewedCount != 2 || second.Results[0].Apartment == nil || *second.Results[0].Apartment != "99" {
		t.Fatal("another edit lost the first correction")
	}
	reopened, err := OpenJobs(s.path)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := reopened.Get(j.Id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Results[0].Id.(json.Number).String() != "9223372036854775807" || saved.Results[0].RawAddress != j.Records[0].RawAddress || saved.Results[1].RawAddress != "" || !saved.Reviewed[0] || saved.Results[0].Coordinates != nil {
		t.Fatalf("roundtrip failed: %+v", saved)
	}
	*saved.Results[0].Apartment = "mutated caller"
	again, _ := reopened.Get(j.Id)
	if *again.Results[0].Apartment != "99" {
		t.Fatal("returned snapshot aliases store memory")
	}
	if err = reopened.Delete(j.Id); err != nil {
		t.Fatal(err)
	}
	reopened, err = OpenJobs(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.Get(j.Id); !errors.Is(err, ErrJobNotFound) {
		t.Fatal("deleted job restored")
	}
}
func TestJobsConcurrentReviewRejectsStaleRevision(t *testing.T) {
	s := newJobs(t)
	j := completedJob(t, s)
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Review(j.Id, 0, j.Revision, api.AddressFields{}, nil); out <- err }()
	}
	wg.Wait()
	close(out)
	success, conflict := 0, 0
	for err := range out {
		if err == nil {
			success++
		} else if errors.Is(err, ErrJobConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}
func TestJobsStorageFailureDoesNotCommitMemory(t *testing.T) {
	s := newJobs(t)
	j := completedJob(t, s)
	s.path = filepath.Join(t.TempDir(), "missing", "jobs.json")
	if _, err := s.Review(j.Id, 0, j.Revision, api.AddressFields{}, nil); err == nil {
		t.Fatal("expected disk failure")
	}
	after, _ := s.Get(j.Id)
	if after.Revision != j.Revision || after.ReviewedCount != 0 {
		t.Fatal("failed write changed memory")
	}
}
func TestJobsInterruptedRecoveryExpiryAndCapacity(t *testing.T) {
	s := newJobs(t)
	j, err := s.Create([]api.InputRecord{{Id: "same", RawAddress: "original"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(j.Id); !errors.Is(err, ErrJobConflict) {
		t.Fatal("active job deleted")
	}
	reopened, err := OpenJobs(s.path)
	if err != nil {
		t.Fatal(err)
	}
	restored, _ := reopened.Get(j.Id)
	if restored.Status != "queued" {
		t.Fatal("interrupted work not requeued")
	}
	if _, err = reopened.Claim(); err != nil {
		t.Fatal(err)
	}
	if err = reopened.Finish(j.Id, nil, "failure"); err != nil {
		t.Fatal(err)
	}
	reopened.now = func() time.Time { return time.Now().Add(8 * 24 * time.Hour) }
	if len(reopened.List()) != 0 {
		t.Fatal("expired job exposed")
	}
	_, _ = reopened.Claim()
	b, _ := os.ReadFile(reopened.path)
	if string(b) != "{}" {
		t.Fatal("expired raw data not removed from disk")
	}
	s = newJobs(t)
	for i := 0; i < maxJobs; i++ {
		if _, err = s.Create([]api.InputRecord{{Id: "same", RawAddress: ""}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.Create([]api.InputRecord{{Id: "same", RawAddress: ""}}); !errors.Is(err, ErrJobLimit) {
		t.Fatal("capacity unbounded")
	}
}
func TestJobsFieldCorrectionClearsStaleGeocode(t *testing.T) {
	s := newJobs(t)
	j, _ := s.Create([]api.InputRecord{{Id: "x", RawAddress: "exact raw"}})
	_, _ = s.Claim()
	city, other, street, kind, house := "Астана", "Другой город", "Туран", "проспект", "55/6"
	id := "building"
	revision := 1
	r := api.Result{Id: "x", RawAddress: "exact raw", City: &city, StreetName: &street, StreetType: &kind, House: &house, Status: api.Matched, Confidence: .7, CatalogId: &id, CatalogRevision: &revision, Coordinates: &api.NullableCoordinates{Latitude: 51, Longitude: 71}, Warnings: []string{"synthetic_catalog_coordinates"}}
	if err := s.Finish(j.Id, []api.Result{r}, ""); err != nil {
		t.Fatal(err)
	}
	j, _ = s.Get(j.Id)
	f := resultFields(r)
	f.City = &other
	edited, err := s.Review(j.Id, 0, j.Revision, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := edited.Results[0]
	if got.Coordinates != nil || got.CatalogId != nil || got.CatalogRevision != nil || got.Status != api.NeedsReview || got.Confidence != .7 {
		t.Fatalf("stale geocode retained or confidence inflated: %+v", got)
	}
}
