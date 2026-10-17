package catalog

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/lukivan8/jana-address/pkg/api"
)

var (
	ErrJobNotFound = errors.New("job not found or expired")
	ErrJobConflict = errors.New("job revision conflict or job still active")
	ErrJobLimit    = errors.New("job storage limit reached")
)

const jobTTL = 7 * 24 * time.Hour
const maxJobBytes = 64 << 20
const maxJobs = 1000

// JobStore is a single-process work journal, never a normalization lookup cache.
// Jobs are immutable snapshots internally; callers always receive detached copies.
type JobStore struct {
	mu   sync.Mutex
	path string
	jobs map[string]api.NormalizationJob
	now  func() time.Time
}

func decodeJobs(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("extra job data")
	}
	return nil
}
func copyJob(j api.NormalizationJob) api.NormalizationJob {
	b, _ := json.Marshal(j)
	var out api.NormalizationJob
	if err := decodeJobs(b, &out); err != nil {
		panic(err)
	}
	return out
}
func OpenJobs(path string) (*JobStore, error) {
	s := &JobStore{path: path, jobs: map[string]api.NormalizationJob{}, now: time.Now}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxJobBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxJobBytes {
		return nil, ErrJobLimit
	}
	if err := decodeJobs(b, &s.jobs); err != nil {
		return nil, fmt.Errorf("invalid job journal: %w", err)
	}
	if s.jobs == nil || len(s.jobs) > maxJobs {
		return nil, errors.New("invalid job journal")
	}
	for id, j := range s.jobs {
		if id != j.Id || j.Revision < 1 || j.Total < 1 || j.Total > 1000 || len(j.Records) != j.Total || len(j.Reviewed) != j.Total || j.CreatedAt.IsZero() || j.ExpiresAt.IsZero() {
			return nil, errors.New("invalid job snapshot")
		}
		switch j.Status {
		case "running":
			j.Status = "queued"
			j.Revision++
			s.jobs[id] = j
		case "queued", "failed":
		case "completed":
			if len(j.Results) != j.Total {
				return nil, errors.New("incomplete job results")
			}
		default:
			return nil, errors.New("invalid job status")
		}
	}
	// Persist interrupted work as queued and remove expired data before accepting requests.
	if err := s.commit(s.live()); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *JobStore) live() map[string]api.NormalizationJob {
	next := make(map[string]api.NormalizationJob)
	for id, j := range s.jobs {
		if s.now().Before(j.ExpiresAt) {
			next[id] = j
		}
	}
	return next
}
func (s *JobStore) commit(next map[string]api.NormalizationJob) error {
	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if len(b) > maxJobBytes {
		return ErrJobLimit
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".jobs-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), s.path); err != nil {
		return err
	}
	s.jobs = next
	return nil
}
func (s *JobStore) Create(records []api.InputRecord) (api.NormalizationJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.live()
	if len(next) >= maxJobs {
		return api.NormalizationJob{}, ErrJobLimit
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return api.NormalizationJob{}, err
	}
	now := s.now().UTC()
	j := api.NormalizationJob{Id: hex.EncodeToString(key), Revision: 1, Status: "queued", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(jobTTL), Total: len(records), Records: records, Results: []api.Result{}, Reviewed: make([]bool, len(records))}
	j = copyJob(j)
	next[j.Id] = j
	if err := s.commit(next); err != nil {
		return api.NormalizationJob{}, err
	}
	return copyJob(j), nil
}
func (s *JobStore) Get(id string) (api.NormalizationJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.live()[id]
	if !ok {
		return api.NormalizationJob{}, ErrJobNotFound
	}
	return copyJob(j), nil
}
func (s *JobStore) List() []api.JobSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []api.JobSummary{}
	for _, j := range s.live() {
		out = append(out, api.JobSummary{Id: j.Id, Revision: j.Revision, Status: api.JobSummaryStatus(j.Status), CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt, ExpiresAt: j.ExpiresAt, Total: j.Total, ReviewedCount: j.ReviewedCount, Error: j.Error})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}
func (s *JobStore) touch(j *api.NormalizationJob) {
	j.Revision++
	j.UpdatedAt = s.now().UTC()
	j.ExpiresAt = j.UpdatedAt.Add(jobTTL)
}
func (s *JobStore) Claim() (api.NormalizationJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.live()
	var chosen *api.NormalizationJob
	for _, j := range next {
		if j.Status == "queued" && (chosen == nil || j.CreatedAt.Before(chosen.CreatedAt)) {
			v := j
			chosen = &v
		}
	}
	if chosen == nil { // Periodic expiry also removes raw data from disk when idle.
		if len(next) != len(s.jobs) {
			if err := s.commit(next); err != nil {
				return api.NormalizationJob{}, err
			}
		}
		return api.NormalizationJob{}, ErrJobNotFound
	}
	chosen.Status = "running"
	s.touch(chosen)
	next[chosen.Id] = *chosen
	if err := s.commit(next); err != nil {
		return api.NormalizationJob{}, err
	}
	return copyJob(*chosen), nil
}
func (s *JobStore) Finish(id string, results []api.Result, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.live()
	j, ok := next[id]
	if !ok {
		return ErrJobNotFound
	}
	if j.Status != "running" {
		return ErrJobConflict
	}
	if message == "" && len(results) != j.Total {
		return errors.New("incomplete normalization")
	}
	j.Status = "completed"
	j.Results = results
	if message != "" {
		j.Status = "failed"
		j.Results = []api.Result{}
	}
	j.Error = message
	s.touch(&j)
	next[id] = copyJob(j)
	return s.commit(next)
}
func buildingOnly(f api.AddressFields) api.AddressFields {
	f.Apartment = nil
	f.Entrance = nil
	f.Floor = nil
	f.Landmark = nil
	return f
}
func resultFields(r api.Result) api.AddressFields {
	return api.AddressFields{Province: r.Province, City: r.City, District: r.District, LivingArea: r.LivingArea, StreetType: r.StreetType, StreetName: r.StreetName, House: r.House, Apartment: r.Apartment, Entrance: r.Entrance, Floor: r.Floor, Landmark: r.Landmark}
}
func (s *JobStore) Review(id string, index, revision int, fields api.AddressFields, confirmed *api.CatalogEntry) (api.NormalizationJob, error) {
	b, _ := json.Marshal(fields)
	var values map[string]*string
	_ = json.Unmarshal(b, &values)
	for _, v := range values {
		if v != nil && (len(*v) > 4096 || !utf8.ValidString(*v)) {
			return api.NormalizationJob{}, errors.New("review fields must be valid UTF-8, at most 4096 bytes")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.live()
	j, ok := next[id]
	if !ok {
		return api.NormalizationJob{}, ErrJobNotFound
	}
	if j.Status != "completed" || revision != j.Revision {
		return api.NormalizationJob{}, ErrJobConflict
	}
	if index < 0 || index >= j.Total {
		return api.NormalizationJob{}, ErrJobNotFound
	}
	j = copyJob(j)
	r := j.Results[index]
	if !reflect.DeepEqual(buildingOnly(resultFields(r)), buildingOnly(fields)) {
		r.Coordinates = nil
		r.CatalogId = nil
		r.CatalogRevision = nil
		r.Status = api.NeedsReview
		r.Warnings = append(r.Warnings, "building_fields_changed_coordinates_cleared")
	}
	if confirmed != nil {
		if !confirmed.Confirmed || !reflect.DeepEqual(confirmed.Fields, buildingOnly(fields)) {
			return api.NormalizationJob{}, errors.New("confirmed building does not match reviewed fields")
		}
		r.Coordinates = &api.NullableCoordinates{Latitude: confirmed.Coordinates.Latitude, Longitude: confirmed.Coordinates.Longitude}
		r.CatalogId = &confirmed.Id
		r.CatalogRevision = &confirmed.Revision
		r.Status = api.Matched
		// Explicit verified confirmation replaces prior matching diagnostics.
		r.Warnings = []string{"manually_reviewed"}
	} else {
		found := false
		for _, w := range r.Warnings {
			if w == "manually_reviewed_fields" {
				found = true
			}
		}
		if !found {
			r.Warnings = append(r.Warnings, "manually_reviewed_fields")
		}
	}
	// A manual administrative edit invalidates the previous identity selection;
	// otherwise the UI could restore a checked city belonging to another province.
	if j.CitySelections != nil && (!reflect.DeepEqual(r.City, fields.City) || !reflect.DeepEqual(r.Province, fields.Province)) {
		choices := []api.CitySelection{}
		for _, choice := range *j.CitySelections {
			if choice.RecordIndex != index {
				choices = append(choices, choice)
			}
		}
		if len(choices) == 0 {
			j.CitySelections = nil
		} else {
			j.CitySelections = &choices
		}
	}
	r.Province = fields.Province
	r.City = fields.City
	r.District = fields.District
	r.LivingArea = fields.LivingArea
	r.StreetType = fields.StreetType
	r.StreetName = fields.StreetName
	r.House = fields.House
	r.Apartment = fields.Apartment
	r.Entrance = fields.Entrance
	r.Floor = fields.Floor
	r.Landmark = fields.Landmark
	j.Results[index] = r
	if !j.Reviewed[index] {
		j.Reviewed[index] = true
		j.ReviewedCount++
	}
	s.touch(&j)
	next[id] = j
	if err := s.commit(next); err != nil {
		return api.NormalizationJob{}, err
	}
	return copyJob(j), nil
}
func (s *JobStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.live()
	j, ok := next[id]
	if !ok {
		return ErrJobNotFound
	}
	if j.Status == "running" || j.Status == "queued" {
		return ErrJobConflict
	}
	delete(next, id)
	return s.commit(next)
}

// SelectCity replaces one result atomically. Administrative review does not mark
// the whole address verified and never writes to the building catalog.
func (s *JobStore) SelectCity(id string, index, revision int, entityID, version string, result api.Result) (api.NormalizationJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.live()
	j, ok := next[id]
	if !ok || index < 0 || index >= j.Total {
		return api.NormalizationJob{}, ErrJobNotFound
	}
	if j.Status != "completed" {
		return api.NormalizationJob{}, ErrJobConflict
	}
	if j.CitySelections != nil {
		for _, choice := range *j.CitySelections {
			if choice.RecordIndex == index && choice.EntityId == entityID && choice.ReferenceVersion == version && choice.AppliedRevision == j.Revision && (revision == j.Revision || revision == j.Revision-1) {
				return copyJob(j), nil
			}
		}
	}
	if revision != j.Revision {
		return api.NormalizationJob{}, ErrJobConflict
	}
	j = copyJob(j)
	// Transport IDs and exact original strings always belong to the job input.
	result.Id = j.Records[index].Id
	result.RawAddress = j.Records[index].RawAddress
	j.Results[index] = result
	if j.Reviewed[index] {
		j.Reviewed[index] = false
		j.ReviewedCount--
	}
	s.touch(&j)
	choices := []api.CitySelection{}
	if j.CitySelections != nil {
		for _, c := range *j.CitySelections {
			if c.RecordIndex != index {
				choices = append(choices, c)
			}
		}
	}
	choices = append(choices, api.CitySelection{RecordIndex: index, EntityId: entityID, ReferenceVersion: version, AppliedRevision: j.Revision})
	j.CitySelections = &choices
	next[id] = j
	if err := s.commit(next); err != nil {
		return api.NormalizationJob{}, err
	}
	return copyJob(j), nil
}
