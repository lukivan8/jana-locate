package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/lukivan8/jana-address/pkg/api"
)

func TestPersistentConfirmationAndConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overlay.json")
	s, err := Open("../../data/catalog.seed.json", path)
	if err != nil {
		t.Fatal(err)
	}
	first := s.List()[0]
	req := api.ConfirmationRequest{Fields: first.Fields, Coordinates: api.Coordinates{Latitude: 44, Longitude: 77}, ExpectedRevision: first.Revision}
	saved, err := s.Confirm(req)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 1 {
		t.Fatal("revision not incremented")
	}
	if _, err = s.Confirm(req); !errors.Is(err, ErrConflict) {
		t.Fatalf("wanted conflict got %v", err)
	}
	restarted, err := Open("../../data/catalog.seed.json", path)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, v := range restarted.List() {
		if v.Id == saved.Id {
			found = v.Revision == 1 && v.Coordinates.Latitude == 44 && v.Confirmed
		}
	}
	if !found {
		t.Fatal("confirmation lost after reopen")
	}
}
func TestConcurrentStaleEditsOnlyOneWins(t *testing.T) {
	s, err := Open("../../data/catalog.seed.json", filepath.Join(t.TempDir(), "overlay.json"))
	if err != nil {
		t.Fatal(err)
	}
	entry := s.List()[0]
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Confirm(api.ConfirmationRequest{Fields: entry.Fields, Coordinates: entry.Coordinates})
			if err == nil {
				winners.Add(1)
			} else if !errors.Is(err, ErrConflict) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("got %d successful stale writes", winners.Load())
	}
}
func TestFailedWriteDoesNotChangeMemory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overlay.json")
	s, err := Open("../../data/catalog.seed.json", path)
	if err != nil {
		t.Fatal(err)
	}
	first := s.List()[0]
	if err = os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Confirm(api.ConfirmationRequest{Fields: first.Fields, Coordinates: first.Coordinates}); err == nil {
		t.Fatal("expected rename failure")
	}
	for _, v := range s.List() {
		if v.Id == first.Id && v.Revision != 0 {
			t.Fatal("memory committed failed write")
		}
	}
}
func TestCorruptOverlayFailsStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overlay.json")
	os.WriteFile(path, []byte("broken"), 0600)
	if _, err := Open("../../data/catalog.seed.json", path); err == nil {
		t.Fatal("corrupt overlay silently ignored")
	}
}

func TestMissingCoordinatesFailStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seed.json")
	raw := `{"entries":[{"fields":{"city":"X","street_type":"улица","street_name":"X","house":"1"},"source":"test","revision":0,"confirmed":false}]}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, filepath.Join(t.TempDir(), "overlay.json")); err == nil {
		t.Fatal("missing coordinates became zero-degree location")
	}
}
