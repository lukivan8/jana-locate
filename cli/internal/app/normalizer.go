package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.com/jana-locate-cli/internal/api"
)

var requiredResultFields = []string{"id", "raw_address", "province", "city", "district", "living_area", "street_type", "street_name", "house", "apartment", "entrance", "floor", "landmark", "confidence", "coordinates", "catalog_id", "catalog_revision", "status", "warnings"}

type Normalizer struct {
	URL, Token string
	HTTP       *http.Client
}

func NewNormalizer(base, token string) (*Normalizer, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("Адрес API должен использовать HTTP(S) и не содержать учётные данные, параметры запроса или фрагмент")
	}
	return &Normalizer{strings.TrimRight(base, "/"), token, &http.Client{Timeout: 300 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (n *Normalizer) Batch(ctx context.Context, rows []api.Row) ([]api.Row, error) {
	records := []api.InputRecord{}
	indexes := []int{}
	for i, r := range rows {
		if r.RawAddress == nil {
			rows[i].Result = emptyResult(r.Position, nil)
			rows[i].Result.Id = r.SourceId
			continue
		}
		records = append(records, api.InputRecord{Id: strconv.Itoa(r.Position), RawAddress: *r.RawAddress})
		indexes = append(indexes, i)
	}
	if len(records) == 0 {
		return rows, nil
	}
	body, _ := json.Marshal(map[string]any{"records": records})
	if len(body) > 2<<20 {
		return nil, errors.New("Пакет нормализации превышает 2 МиБ")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", n.URL+"/v1/normalize", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if n.Token != "" {
		req.Header.Set("Authorization", "Bearer "+n.Token)
	}
	res, err := n.HTTP.Do(req)
	if err != nil {
		return nil, errors.New("API нормализации недоступен; проверьте адрес и сеть, затем возобновите обработку")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("API нормализации вернул HTTP %d; проверьте доступ и доступность сервиса, затем возобновите обработку", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 128<<20+1))
	if err != nil {
		return nil, errors.New("Не удалось прочитать ответ нормализатора")
	}
	if len(b) > 128<<20 {
		return nil, errors.New("Ответ нормализатора превышает 128 МиБ")
	}
	var out struct {
		Results []api.Result `json:"results"`
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err = d.Decode(&out); err != nil {
		return nil, errors.New("API нормализации вернул некорректный JSON")
	}
	if len(out.Results) != len(records) {
		return nil, errors.New("API нормализации вернул неполный пакет результатов")
	}
	var wire struct {
		Results []map[string]json.RawMessage `json:"results"`
	}
	if err = json.Unmarshal(b, &wire); err != nil {
		return nil, errors.New("Некорректная структура результата нормализации")
	}
	for _, row := range wire.Results {
		for _, key := range requiredResultFields {
			if _, ok := row[key]; !ok {
				return nil, errors.New("В результате нормализации отсутствует обязательное поле")
			}
		}
	}
	for i, r := range out.Results {
		input := records[i]
		if r.Id != input.Id || r.RawAddress != input.RawAddress || r.Warnings == nil || r.Confidence < 0 || r.Confidence > 1 || (r.Status != api.Matched && r.Status != api.NeedsReview && r.Status != api.Unmatched) {
			return nil, errors.New("В ответе нормализатора нарушен порядок записей, изменены идентификаторы или исходные адреса либо некорректна диагностика")
		}
		r.Id = rows[indexes[i]].SourceId
		if r.Coordinates != nil {
			var coords map[string]json.RawMessage
			if err = json.Unmarshal(wire.Results[i]["coordinates"], &coords); err != nil {
				return nil, errors.New("Некорректные координаты в результате нормализации")
			}
			for _, key := range []string{"latitude", "longitude"} {
				if value, ok := coords[key]; !ok || string(value) == "null" {
					return nil, errors.New("В координатах нормализации отсутствует широта или долгота")
				}
			}
			if r.Coordinates.Latitude < -90 || r.Coordinates.Latitude > 90 || r.Coordinates.Longitude < -180 || r.Coordinates.Longitude > 180 {
				return nil, errors.New("Координаты нормализации выходят за допустимый диапазон")
			}
		}
		rows[indexes[i]].Result = r
	}
	return rows, nil
}
func Work(ctx context.Context, s *Store, n *Normalizer) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		runs, err := s.List(ctx)
		if err == nil {
			for _, run := range runs {
				if run.Status != api.Normalizing {
					continue
				}
				if run.ApiUrl != n.URL {
					_, _ = s.Status(ctx, run.Id, 0, api.Failed, "Адрес API отличается от сохранённого запуска; используйте прежний адрес или создайте новый запуск.")
					continue
				}
				rows, err := s.Rows(ctx, run.Id, 0, 100, false, true)
				if err != nil {
					continue
				}
				if len(rows) == 0 {
					continue
				}
				parsed, err := n.Batch(ctx, rows)
				if ctx.Err() != nil {
					return
				}
				if err != nil {
					_, _ = s.Status(ctx, run.Id, 0, api.Failed, UserMessage(err))
					continue
				}
				if err = s.SaveBatch(ctx, run.Id, parsed); err != nil {
					_, _ = s.Status(ctx, run.Id, 0, api.Failed, "Не удалось сохранить прогресс локально; проверьте свободное место и возобновите обработку.")
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
