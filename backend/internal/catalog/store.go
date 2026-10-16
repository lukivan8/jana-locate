// Package catalog owns the read-only seed plus a durable, single-writer overlay.
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/lukivan8/jana-address/pkg/api"
)

var ErrConflict = errors.New("catalog revision conflict")

type Store struct {
	mu      sync.RWMutex
	entries map[string]api.CatalogEntry
	overlay map[string]api.CatalogEntry
	path    string
}

func Text(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func Canon(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func ID(f api.AddressFields) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{Canon(Text(f.City)), Canon(Text(f.StreetType)), Canon(Text(f.StreetName)), Canon(Text(f.House))}, "\x00")))
	return "bld_" + hex.EncodeToString(sum[:12])
}

func Validate(f api.AddressFields, c api.Coordinates) error {
	for name, p := range map[string]*string{"city": f.City, "street_type": f.StreetType, "street_name": f.StreetName, "house": f.House} {
		if strings.TrimSpace(Text(p)) == "" {
			return fmt.Errorf("%s is required for a building", name)
		}
	}
	for _, p := range []*string{f.Province, f.City, f.District, f.LivingArea, f.StreetType, f.StreetName, f.House} {
		if len(Text(p)) > 256 {
			return errors.New("catalog fields must not exceed 256 bytes")
		}
	}
	if f.Apartment != nil || f.Entrance != nil || f.Floor != nil || f.Landmark != nil {
		return errors.New("catalog stores buildings: apartment, entrance, floor and landmark must be null")
	}
	if math.IsNaN(float64(c.Latitude)) || math.IsNaN(float64(c.Longitude)) || math.IsInf(float64(c.Latitude), 0) || math.IsInf(float64(c.Longitude), 0) || c.Latitude < -90 || c.Latitude > 90 || c.Longitude < -180 || c.Longitude > 180 {
		return errors.New("coordinates out of range")
	}
	return nil
}

func Open(seedPath, overlayPath string) (*Store, error) {
	s := &Store{entries: map[string]api.CatalogEntry{}, overlay: map[string]api.CatalogEntry{}, path: overlayPath}
	load := func(path string, optional bool, overlay bool) error {
		b, err := os.ReadFile(path)
		if optional && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		var snapshot api.CatalogResponse
		if err = json.Unmarshal(b, &snapshot); err != nil {
			return fmt.Errorf("invalid catalog %s: %w", path, err)
		}
		if snapshot.Entries == nil {
			return fmt.Errorf("catalog %s requires an entries array", path)
		}
		// A missing coordinate is unknown, not a legitimate zero-degree location.
		var wire struct {
			Entries []struct {
				Fields      *api.AddressFields `json:"fields"`
				Coordinates *struct {
					Latitude  *float64 `json:"latitude"`
					Longitude *float64 `json:"longitude"`
				} `json:"coordinates"`
				Revision  *int    `json:"revision"`
				Confirmed *bool   `json:"confirmed"`
				Source    *string `json:"source"`
			} `json:"entries"`
		}
		if err := json.Unmarshal(b, &wire); err != nil {
			return err
		}
		for _, entry := range wire.Entries {
			if entry.Fields == nil || entry.Coordinates == nil || entry.Coordinates.Latitude == nil || entry.Coordinates.Longitude == nil || entry.Revision == nil || entry.Confirmed == nil || entry.Source == nil {
				return fmt.Errorf("catalog %s: fields, coordinates, revision, confirmed and source are required", path)
			}
		}
		seen := map[string]bool{}
		for _, e := range snapshot.Entries {
			if err = Validate(e.Fields, e.Coordinates); err != nil {
				return fmt.Errorf("invalid catalog entry: %w", err)
			}
			e.Id = ID(e.Fields)
			if seen[e.Id] {
				return fmt.Errorf("duplicate building in %s: %s", path, e.Id)
			}
			seen[e.Id] = true
			if e.Revision < 0 || (overlay && (e.Revision < 1 || !e.Confirmed)) {
				return errors.New("invalid catalog revision or confirmation state")
			}
			s.entries[e.Id] = e
			if overlay {
				s.overlay[e.Id] = e
			}
		}
		return nil
	}
	if err := load(seedPath, false, false); err != nil {
		return nil, err
	}
	if err := load(overlayPath, true, true); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0750); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) List() []api.CatalogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]api.CatalogEntry, 0, len(s.entries))
	for _, e := range s.entries {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Id < entries[j].Id })
	return entries
}

func (s *Store) Confirm(r api.ConfirmationRequest) (api.CatalogEntry, error) {
	if err := Validate(r.Fields, r.Coordinates); err != nil {
		return api.CatalogEntry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := ID(r.Fields)
	old := s.entries[id]
	if r.ExpectedRevision < 0 || r.ExpectedRevision != old.Revision {
		return api.CatalogEntry{}, ErrConflict
	}
	entry := api.CatalogEntry{Id: id, Revision: old.Revision + 1, Fields: r.Fields, Coordinates: r.Coordinates, Confirmed: true, Source: "manual-confirmation"}
	next := make(map[string]api.CatalogEntry, len(s.overlay)+1)
	for k, v := range s.overlay {
		next[k] = v
	}
	next[id] = entry
	entries := make([]api.CatalogEntry, 0, len(next))
	for _, v := range next {
		entries = append(entries, v)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Id < entries[j].Id })
	b, err := json.MarshalIndent(api.CatalogResponse{Entries: entries}, "", "  ")
	if err != nil {
		return api.CatalogEntry{}, err
	}
	// Temp file lives on the same filesystem as the target, making rename atomic.
	f, err := os.CreateTemp(filepath.Dir(s.path), ".catalog-*.tmp")
	if err != nil {
		return api.CatalogEntry{}, err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return api.CatalogEntry{}, err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return api.CatalogEntry{}, err
	}
	if err = f.Close(); err != nil {
		return api.CatalogEntry{}, err
	}
	if err = os.Rename(f.Name(), s.path); err != nil {
		return api.CatalogEntry{}, err
	}
	s.entries[id] = entry
	s.overlay = next
	return entry, nil
}
