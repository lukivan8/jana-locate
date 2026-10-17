package httpapi

import (
	"errors"
	"net/http"

	"github.com/lukivan8/jana-address/internal/normalize"
	"github.com/lukivan8/jana-address/pkg/api"
)

// The same domain engine serves public jobs and consumers with their own journals.
func registerRecordCitySelections(mux *http.ServeMux, engine normalize.Engine) {
	for _, action := range []string{"candidates", "selections"} {
		mux.HandleFunc("POST /v1/normalize/"+action, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			var body struct {
				Record           *normalizeRecord `json:"record"`
				ReferenceVersion *string          `json:"reference_version,omitempty"`
				EntityID         *string          `json:"entity_id,omitempty"`
			}
			if !decode(w, r, &body) {
				return
			}
			if body.Record == nil {
				inputError(w, errInvalidRecord)
				return
			}
			records, err := inputRecords(normalizeRequest{Records: []normalizeRecord{*body.Record}})
			if err != nil {
				inputError(w, err)
				return
			}
			if action == "candidates" {
				if body.EntityID != nil || body.ReferenceVersion != nil {
					fail(w, 400, "invalid_request", "Для списка вариантов передайте только исходную запись.")
					return
				}
				candidates := engine.CityCandidates(r.Context(), records[0])
				if r.Context().Err() != nil {
					return
				}
				jsonResponse(w, 200, api.RecordCityCandidates{ReferenceVersion: engine.Reference.Version(), Candidates: candidates})
				return
			}
			if body.ReferenceVersion == nil || body.EntityID == nil || *body.ReferenceVersion == "" || *body.EntityID == "" {
				fail(w, 400, "invalid_selection", "Укажите entity_id и reference_version.")
				return
			}
			if *body.ReferenceVersion != engine.Reference.Version() {
				fail(w, 409, "reference_version_conflict", "Справочник изменился. Загрузите варианты заново.")
				return
			}
			result, err := engine.SelectCity(r.Context(), records[0], *body.EntityID)
			if errors.Is(err, normalize.ErrInvalidCitySelection) {
				fail(w, 400, "invalid_selection", "Населённый пункт несовместим с исходным адресом. Загрузите варианты заново.")
				return
			}
			if err != nil {
				fail(w, 500, "normalization_error", "Не удалось пересчитать адрес.")
				return
			}
			jsonResponse(w, 200, result)
		})
	}
}
