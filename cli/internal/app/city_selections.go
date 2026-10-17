package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"example.com/jana-locate-cli/internal/api"
)

type cityAPIError struct {
	status  int
	message string
}

func (e *cityAPIError) Error() string { return e.message }

func (n *Normalizer) cityCall(ctx context.Context, path string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", n.URL+"/v1/normalize/"+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if n.Token != "" {
		req.Header.Set("Authorization", "Bearer "+n.Token)
	}
	res, err := n.HTTP.Do(req)
	if err != nil {
		return &cityAPIError{502, "Нормализатор недоступен. Повторите загрузку вариантов."}
	}
	defer res.Body.Close()
	b, err = io.ReadAll(io.LimitReader(res.Body, (16<<20)+1))
	if err != nil || len(b) > 16<<20 {
		return &cityAPIError{502, "Некорректный ответ нормализатора."}
	}
	if res.StatusCode != 200 {
		status := 502
		message := fmt.Sprintf("Не удалось получить варианты: API вернул HTTP %d. Обновите нормализатор и повторите запрос.", res.StatusCode)
		if res.StatusCode == 409 {
			status = 409
			message = "Справочник изменился. Загрузите варианты заново."
		}
		if res.StatusCode == 400 {
			status = 400
			message = "Населённый пункт несовместим с исходным адресом."
		}
		return &cityAPIError{status, message}
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err = d.Decode(out); err != nil {
		return &cityAPIError{502, "Некорректный ответ нормализатора."}
	}
	if d.Decode(new(any)) != io.EOF {
		return &cityAPIError{502, "Некорректный ответ нормализатора."}
	}
	return nil
}
func (s *Store) cityRow(ctx context.Context, id string, position int) (api.Run, api.Row, error) {
	run, err := s.Get(ctx, id)
	if err != nil {
		return run, api.Row{}, err
	}
	if run.Status != api.Review {
		return run, api.Row{}, ErrConflict
	}
	if position < 0 || position >= run.Total {
		return run, api.Row{}, ErrNotFound
	}
	rows, err := s.Rows(ctx, id, position, 1, false, false)
	if err != nil {
		return run, api.Row{}, err
	}
	if len(rows) != 1 || rows[0].Position != position {
		return run, api.Row{}, ErrNotFound
	}
	return run, rows[0], nil
}
func (s *Store) CityCandidates(ctx context.Context, n *Normalizer, id string, position int) (api.CityCandidates, error) {
	run, row, err := s.cityRow(ctx, id, position)
	if err != nil {
		return api.CityCandidates{}, err
	}
	if n == nil || n.URL != run.ApiUrl {
		return api.CityCandidates{}, ErrConflict
	}
	out := api.CityCandidates{Revision: run.Revision, Candidates: []api.CityCandidate{}}
	if row.RawAddress == nil {
		return out, nil
	}
	var response api.RecordCityCandidates
	err = n.cityCall(ctx, "candidates", api.RecordCityCandidatesRequest{Record: api.InputRecord{Id: strconv.Itoa(position), RawAddress: *row.RawAddress}}, &response)
	if err != nil {
		return out, err
	}
	if response.ReferenceVersion == "" || response.Candidates == nil {
		return out, &cityAPIError{502, "Нормализатор не вернул версию справочника или список вариантов."}
	}
	latest, err := s.Get(ctx, id)
	if err != nil {
		return out, err
	}
	if latest.Revision != run.Revision {
		return out, ErrConflict
	}
	out.ReferenceVersion = response.ReferenceVersion
	out.Candidates = response.Candidates
	return out, nil
}
func (s *Store) SelectCity(ctx context.Context, n *Normalizer, id string, position int, req api.CitySelectionRequest) (api.Row, error) {
	if req.ExpectedRevision < 1 {
		return api.Row{}, &cityAPIError{400, "Укажите положительную ревизию запуска."}
	}
	run, row, err := s.cityRow(ctx, id, position)
	if err != nil {
		return row, err
	}
	if n == nil || n.URL != run.ApiUrl {
		return row, ErrConflict
	}
	retry := false
	if req.ExpectedRevision != run.Revision {
		c := row.CitySelection
		retry = c != nil && c.EntityId == req.EntityId && c.ReferenceVersion == req.ReferenceVersion && c.AppliedRevision == run.Revision && req.ExpectedRevision == run.Revision-1
		if !retry {
			return row, ErrConflict
		}
	}
	if row.RawAddress == nil || req.EntityId == "" || req.ReferenceVersion == "" {
		return row, &cityAPIError{400, "Для выбора нужен исходный адрес, населённый пункт и версия справочника."}
	}
	var payload json.RawMessage
	err = n.cityCall(ctx, "selections", api.RecordCitySelectionRequest{Record: api.InputRecord{Id: strconv.Itoa(position), RawAddress: *row.RawAddress}, EntityId: req.EntityId, ReferenceVersion: req.ReferenceVersion}, &payload)
	if err != nil {
		return row, err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil {
		return row, &cityAPIError{502, "Некорректный результат выбора."}
	}
	for _, name := range requiredResultFields {
		if _, ok := fields[name]; !ok {
			return row, &cityAPIError{502, "В результате выбора отсутствует обязательное поле."}
		}
	}
	var result api.Result
	d := json.NewDecoder(bytes.NewReader(payload))
	d.UseNumber()
	if d.Decode(&result) != nil {
		return row, &cityAPIError{502, "Некорректный результат выбора."}
	}
	if result.Id != strconv.Itoa(position) || result.RawAddress != *row.RawAddress || result.Warnings == nil || result.Confidence < 0 || result.Confidence > 1 || (result.Status != api.Matched && result.Status != api.NeedsReview && result.Status != api.Unmatched) || result.Coordinates != nil || result.CatalogId != nil || result.CatalogRevision != nil {
		return row, &cityAPIError{502, "Нормализатор вернул некорректный результат выбора."}
	}
	if retry {
		latest, err := s.Get(ctx, id)
		if err != nil {
			return row, err
		}
		if latest.Revision != run.Revision {
			return row, ErrConflict
		}
		return row, nil // The current reference version was checked upstream; retain the receipt.
	}
	result.Id = row.Result.Id // Exact original file IDs, including duplicate int64s.
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return row, err
	}
	defer tx.Rollback()
	latest, err := readRun(tx, ctx, id)
	if err != nil {
		return row, err
	}
	if latest.Status != api.Review || latest.Revision != run.Revision {
		return row, ErrConflict
	}
	choice := api.CitySelection{RecordIndex: position, EntityId: req.EntityId, ReferenceVersion: req.ReferenceVersion, AppliedRevision: latest.Revision + 1}
	b, _ := json.Marshal(result)
	selection, _ := json.Marshal(choice)
	changed, err := tx.ExecContext(ctx, "UPDATE records SET result=?,reviewed=0,revision=revision+1,city_selection=? WHERE run_id=? AND position=? AND revision=?", string(b), string(selection), id, position, row.Revision)
	if err != nil {
		return row, err
	}
	count, err := changed.RowsAffected()
	if err != nil {
		return row, err
	}
	if count != 1 {
		return row, ErrConflict
	}
	if err = saveRun(ctx, tx, latest); err != nil {
		return row, err
	}
	if err = tx.Commit(); err != nil {
		return row, err
	}
	row.Result = result
	row.Reviewed = false
	row.Revision++
	row.CitySelection = &choice
	return row, nil
}
func (s Server) registerCitySelections(mux *http.ServeMux) {
	respondError := func(w http.ResponseWriter, err error) {
		var remote *cityAPIError
		if errors.As(err, &remote) {
			reply(w, remote.status, api.Error{Code: "city_selection_failed", Message: remote.message})
			return
		}
		failure(w, err)
	}
	for _, action := range []string{"candidates", "selections"} {
		method := "GET"
		if action == "selections" {
			method = "POST"
		}
		mux.HandleFunc(method+" /v1/runs/{id}/rows/{position}/"+action, func(w http.ResponseWriter, r *http.Request) {
			position, err := strconv.Atoi(r.PathValue("position"))
			if err != nil || position < 0 {
				failure(w, ErrNotFound)
				return
			}
			if action == "candidates" {
				out, err := s.Store.CityCandidates(r.Context(), s.Normalizer, r.PathValue("id"), position)
				if err != nil {
					respondError(w, err)
					return
				}
				reply(w, 200, out)
				return
			}
			var req api.CitySelectionRequest
			if !decode(w, r, &req) {
				return
			}
			out, err := s.Store.SelectCity(r.Context(), s.Normalizer, r.PathValue("id"), position, req)
			if err != nil {
				respondError(w, err)
				return
			}
			reply(w, 200, out)
		})
	}
}
