package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/normalize"
	"github.com/lukivan8/jana-address/pkg/api"
)

var errInvalidRecord = errors.New("id must be a string or int64 and raw_address must be valid UTF-8, at most 4096 bytes")

func inputRecords(req normalizeRequest) ([]api.InputRecord, error) {
	if len(req.Records) < 1 || len(req.Records) > 1000 {
		return nil, errors.New("records must contain 1 to 1000 items")
	}
	records := make([]api.InputRecord, 0, len(req.Records))
	for _, r := range req.Records {
		valid := false
		switch id := r.Id.(type) {
		case string:
			valid = true
		case json.Number:
			_, err := id.Int64()
			valid = err == nil
		}
		if !valid || r.RawAddress == nil || len(*r.RawAddress) > 4096 || !utf8.ValidString(*r.RawAddress) {
			return nil, errInvalidRecord
		}
		records = append(records, api.InputRecord{Id: r.Id, RawAddress: *r.RawAddress})
	}
	return records, nil
}
func inputError(w http.ResponseWriter, err error) {
	code := "invalid_batch"
	if errors.Is(err, errInvalidRecord) {
		code = "invalid_record"
	}
	fail(w, 400, code, err.Error())
}

func jobError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, catalog.ErrJobNotFound):
		fail(w, 404, "job_not_found", "Задание не найдено или срок хранения истёк.")
	case errors.Is(err, catalog.ErrJobConflict):
		fail(w, 409, "job_revision_conflict", "Задание изменилось или ещё обрабатывается. Откройте сохранённую версию и согласуйте правки.")
	case errors.Is(err, catalog.ErrJobLimit):
		fail(w, 507, "job_storage_full", "Хранилище заданий заполнено. Удалите ненужные завершённые задания.")
	default:
		fail(w, 500, "storage_error", "Не удалось сохранить задание. Правки остаются в форме; повторите сохранение.")
	}
}
func registerJobs(mux *http.ServeMux, jobs *catalog.JobStore, buildings *catalog.Store, engine normalize.Engine) {
	if jobs == nil {
		return
	}
	registerCitySelections(mux, jobs, engine)
	mux.HandleFunc("GET /v1/normalization-jobs", func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, 200, api.JobList{Jobs: jobs.List()}) })
	mux.HandleFunc("POST /v1/normalization-jobs", func(w http.ResponseWriter, r *http.Request) {
		var req normalizeRequest
		if !decode(w, r, &req) {
			return
		}
		records, err := inputRecords(req)
		if err != nil {
			inputError(w, err)
			return
		}
		j, err := jobs.Create(records)
		if err != nil {
			jobError(w, err)
			return
		}
		jsonResponse(w, 202, j)
	})
	mux.HandleFunc("GET /v1/normalization-jobs/{job_id}", func(w http.ResponseWriter, r *http.Request) {
		j, err := jobs.Get(r.PathValue("job_id"))
		if err != nil {
			jobError(w, err)
			return
		}
		jsonResponse(w, 200, j)
	})
	mux.HandleFunc("DELETE /v1/normalization-jobs/{job_id}", func(w http.ResponseWriter, r *http.Request) {
		if err := jobs.Delete(r.PathValue("job_id")); err != nil {
			jobError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /v1/normalization-jobs/{job_id}/records/{record_index}/review", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Fields             *api.AddressFields `json:"fields"`
			ExpectedRevision   *int               `json:"expected_revision"`
			ConfirmedCatalogId *string            `json:"confirmed_catalog_id"`
		}
		if !decode(w, r, &req) {
			return
		}
		index, err := strconv.Atoi(r.PathValue("record_index"))
		if err != nil || index < 0 || req.Fields == nil || req.ExpectedRevision == nil || *req.ExpectedRevision < 1 {
			fail(w, 400, "invalid_review", "fields, positive expected_revision and a valid record index are required")
			return
		}
		var confirmed *api.CatalogEntry
		if req.ConfirmedCatalogId != nil {
			for _, e := range buildings.List() {
				if e.Id == *req.ConfirmedCatalogId {
					v := e
					confirmed = &v
					break
				}
			}
			if confirmed == nil {
				fail(w, 400, "invalid_confirmation", "confirmed building not found")
				return
			}
		}
		// Validate caller input separately from storage errors.
		for _, v := range []*string{req.Fields.Province, req.Fields.City, req.Fields.District, req.Fields.LivingArea, req.Fields.StreetType, req.Fields.StreetName, req.Fields.House, req.Fields.Apartment, req.Fields.Entrance, req.Fields.Floor, req.Fields.Landmark} {
			if v != nil && len(*v) > 4096 {
				fail(w, 400, "invalid_review", "fields must not exceed 4096 bytes")
				return
			}
		}
		if confirmed != nil {
			f := *req.Fields
			f.Apartment = nil
			f.Entrance = nil
			f.Floor = nil
			f.Landmark = nil
			a, _ := json.Marshal(f)
			b, _ := json.Marshal(confirmed.Fields)
			if !confirmed.Confirmed || string(a) != string(b) {
				fail(w, 400, "invalid_confirmation", "confirmed building does not match reviewed fields")
				return
			}
		}
		j, err := jobs.Review(r.PathValue("job_id"), index, *req.ExpectedRevision, *req.Fields, confirmed)
		if err != nil {
			jobError(w, err)
			return
		}
		jsonResponse(w, 200, j)
	})
}

func registerCitySelections(mux *http.ServeMux, jobs *catalog.JobStore, engine normalize.Engine) {
	load := func(w http.ResponseWriter, r *http.Request) (api.NormalizationJob, int, bool) {
		j, err := jobs.Get(r.PathValue("job_id"))
		if err != nil {
			jobError(w, err)
			return j, 0, false
		}
		index, err := strconv.Atoi(r.PathValue("record_index"))
		if err != nil || index < 0 || index >= j.Total {
			jobError(w, catalog.ErrJobNotFound)
			return j, 0, false
		}
		if j.Status != "completed" {
			jobError(w, catalog.ErrJobConflict)
			return j, 0, false
		}
		return j, index, true
	}
	mux.HandleFunc("GET /v1/normalization-jobs/{job_id}/records/{record_index}/candidates", func(w http.ResponseWriter, r *http.Request) {
		j, index, ok := load(w, r)
		if !ok {
			return
		}
		// Review the saved extraction evidence; do not run the model again on every UI render.
		candidates, available := engine.CityCandidatesFromResult(j.Results[index])
		if !available {
			var err error
			candidates, err = engine.CityCandidatesChecked(r.Context(), j.Records[index])
			if err != nil {
				fail(w, 503, "normalizer_unavailable", "Сервис нормализации недоступен. Повторите запрос.")
				return
			}
		}
		jsonResponse(w, 200, api.CityCandidates{Revision: j.Revision, ReferenceVersion: engine.ReferenceVersion(), Candidates: candidates})
	})
	mux.HandleFunc("POST /v1/normalization-jobs/{job_id}/records/{record_index}/selections", func(w http.ResponseWriter, r *http.Request) {
		var req api.CitySelectionRequest
		if !decode(w, r, &req) {
			return
		}
		if req.ExpectedRevision < 1 || req.EntityId == "" || req.ReferenceVersion == "" {
			fail(w, 400, "invalid_selection", "entity_id, reference_version and positive expected_revision are required")
			return
		}
		j, index, ok := load(w, r)
		if !ok {
			return
		}
		if req.ReferenceVersion != engine.ReferenceVersion() {
			fail(w, 409, "reference_version_conflict", "Справочник изменился. Загрузите варианты заново.")
			return
		}
		// Reject stale edits before expensive parsing; an exact retry is handled by the store.
		retry := false
		if j.CitySelections != nil {
			for _, c := range *j.CitySelections {
				if c.RecordIndex == index && c.EntityId == req.EntityId && c.ReferenceVersion == req.ReferenceVersion && c.AppliedRevision == j.Revision && req.ExpectedRevision == j.Revision-1 {
					retry = true
				}
			}
		}
		if req.ExpectedRevision != j.Revision && !retry {
			jobError(w, catalog.ErrJobConflict)
			return
		}
		result, err := engine.SelectCity(r.Context(), j.Records[index], req.EntityId)
		if errors.Is(err, normalize.ErrInvalidCitySelection) {
			fail(w, 400, "invalid_selection", "Населённый пункт несовместим с исходным адресом. Загрузите варианты заново.")
			return
		}
		if err != nil {
			fail(w, 500, "normalization_error", "Не удалось пересчитать адрес.")
			return
		}
		updated, err := jobs.SelectCity(j.Id, index, req.ExpectedRevision, req.EntityId, req.ReferenceVersion, result)
		if err != nil {
			jobError(w, err)
			return
		}
		jsonResponse(w, 200, updated)
	})
}
