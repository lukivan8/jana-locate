package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	contract "github.com/lukivan8/jana-address/api"
	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/normalize"
	"github.com/lukivan8/jana-address/pkg/api"
)

func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code, msg string) {
	jsonResponse(w, status, api.Error{Code: code, Message: msg})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		fail(w, 415, "unsupported_media_type", "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	d.UseNumber()
	err = d.Decode(v)
	if err == nil {
		var extra any
		if d.Decode(&extra) != io.EOF {
			err = errors.New("expected a single JSON value")
		}
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(w, 413, "payload_too_large", "request exceeds 2 MiB")
		} else {
			fail(w, 400, "invalid_json", err.Error())
		}
		return false
	}
	return true
}

func New(store *catalog.Store, origins []string, configured ...normalize.Engine) http.Handler {
	return NewWithJobs(store, nil, origins, configured...)
}

func NewWithJobs(store *catalog.Store, jobs *catalog.JobStore, origins []string, configured ...normalize.Engine) http.Handler {
	mux := http.NewServeMux()
	engine := normalize.Engine{}
	if len(configured) > 0 {
		engine = configured[0]
	}
	health := func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, 200, api.Health{Status: "ok"}) }
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /readyz", health)
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(contract.OpenAPI)
	})
	mux.HandleFunc("GET /v1/catalog", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, 200, api.CatalogResponse{Entries: store.List()})
	})
	registerJobs(mux, jobs, store, engine)
	registerRecordCitySelections(mux, engine)
	mux.HandleFunc("POST /v1/normalize", func(w http.ResponseWriter, r *http.Request) {
		var req normalizeRequest
		if !decode(w, r, &req) {
			return
		}
		records, err := inputRecords(req)
		if err != nil {
			inputError(w, err)
			return
		}
		results := engine.NormalizeBatchContext(r.Context(), records)
		if req.array {
			jsonResponse(w, 200, results)
			return
		}
		jsonResponse(w, 200, api.NormalizeResponse{Results: results})
	})
	mux.HandleFunc("POST /v1/catalog/confirmations", func(w http.ResponseWriter, r *http.Request) {
		// Pointers distinguish missing coordinates/revision from legitimate zero values.
		var body struct {
			Fields      *api.AddressFields `json:"fields"`
			Coordinates *struct {
				Latitude  *float64 `json:"latitude"`
				Longitude *float64 `json:"longitude"`
			} `json:"coordinates"`
			ExpectedRevision *int `json:"expected_revision"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.Fields == nil || body.Coordinates == nil || body.Coordinates.Latitude == nil || body.Coordinates.Longitude == nil || body.ExpectedRevision == nil {
			fail(w, 400, "invalid_confirmation", "fields, latitude, longitude and expected_revision are required")
			return
		}
		req := api.ConfirmationRequest{Fields: *body.Fields, Coordinates: api.Coordinates{Latitude: *body.Coordinates.Latitude, Longitude: *body.Coordinates.Longitude}, ExpectedRevision: *body.ExpectedRevision}
		if err := catalog.Validate(req.Fields, req.Coordinates); err != nil {
			fail(w, 400, "invalid_confirmation", err.Error())
			return
		}
		entry, err := store.Confirm(req)
		if errors.Is(err, catalog.ErrConflict) {
			fail(w, 409, "revision_conflict", "reload the catalog and review the latest building before retrying")
			return
		}
		if err != nil {
			fail(w, 500, "storage_error", "could not persist confirmation")
			return
		}
		jsonResponse(w, 200, entry)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(r.URL.Path, "/v1/normalization-jobs") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Add("Vary", "Origin")
			allowed := false
			for _, o := range origins {
				if strings.TrimSpace(o) == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				fail(w, 403, "origin_forbidden", "origin is not allowed")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
