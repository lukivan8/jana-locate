package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"example.com/jana-locate-cli/internal/api"
)

func normalizedAddress(r api.Result) string {
	parts := []string{}
	for _, p := range []*string{r.Province, r.City, r.District, r.LivingArea, r.StreetType, r.StreetName, r.House} {
		if p != nil && *p != "" {
			parts = append(parts, *p)
		}
	}
	for _, f := range []struct {
		name  string
		value *string
	}{{"кв.", r.Apartment}, {"подъезд", r.Entrance}, {"этаж", r.Floor}, {"ориентир", r.Landmark}} {
		if f.value != nil && *f.value != "" {
			parts = append(parts, f.name+" "+*f.value)
		}
	}
	return strings.Join(parts, ", ")
}
func rawAddress(row api.Row) string {
	if row.RawAddress == nil {
		return "NULL"
	}
	if *row.RawAddress == "" {
		return "(пустая строка)"
	}
	return *row.RawAddress
}
func rowConfidence(row api.Row) string {
	if row.Normalized != nil && !*row.Normalized {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", row.Result.Confidence*100)
}

func rowStatus(row api.Row) string {
	if row.Reviewed {
		return "ПРОВЕРЕН ОПЕРАТОРОМ"
	}
	if row.Result.Status == api.Matched {
		return "НОРМАЛИЗОВАН"
	}
	if row.Normalized != nil && !*row.Normalized {
		return "ОЖИДАЕТ ОБРАБОТКИ"
	}
	return "НУЖНА ПРОВЕРКА"
}
func addressChanges(raw, normalized string) string {
	tokenize := func(v string) []string {
		return strings.FieldsFunc(v, func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune(",.;:()", r) })
	}
	a, b := tokenize(raw), tokenize(normalized)
	left, right := map[string]int{}, map[string]int{}
	for _, v := range a {
		left[strings.ToLower(v)]++
	}
	for _, v := range b {
		right[strings.ToLower(v)]++
	}
	added, removed := []string{}, []string{}
	for _, v := range b {
		k := strings.ToLower(v)
		if left[k] > 0 {
			left[k]--
		} else {
			added = append(added, v)
		}
	}
	for _, v := range a {
		k := strings.ToLower(v)
		if right[k] > 0 {
			right[k]--
		} else {
			removed = append(removed, v)
		}
	}
	if len(added) == 0 && len(removed) == 0 {
		return "Без изменений состава адреса"
	}
	parts := []string{}
	if len(removed) > 0 {
		parts = append(parts, "− "+strings.Join(removed, " "))
	}
	if len(added) > 0 {
		parts = append(parts, "+ "+strings.Join(added, " "))
	}
	return strings.Join(parts, " · ")
}
func reason(w string) string {
	labels := map[string]string{"libpostal_unavailable": "Резервный разбор: libpostal недоступен", "source_address_null": "В источнике нет адреса", "ambiguous_catalog_match": "Несколько подходящих зданий", "ambiguous_city_reference": "Неоднозначный населённый пункт", "ambiguous_reference_street": "Неоднозначная улица", "building_reference_conflict": "Здание противоречит справочнику", "manually_reviewed_fields": "Поля проверены оператором", "coordinates_unknown": "Координаты неизвестны", "synthetic_catalog_coordinates": "Синтетические координаты", "city_selected_by_operator": "Населённый пункт выбран оператором"}
	if label, ok := labels[w]; ok {
		return label
	}
	names := map[string]string{"province": "область", "city": "населённый пункт", "district": "район", "living_area": "жилой массив", "street": "улица", "street_name": "улица", "house": "дом"}
	for _, prefix := range []string{"ambiguous_reference_", "ambiguous_", "conflicting_", "missing_"} {
		if field, ok := strings.CutPrefix(w, prefix); ok {
			label := names[field]
			if label == "" {
				label = field
			}
			switch prefix {
			case "conflicting_":
				return "Конфликт: " + label
			case "missing_":
				return "Не определено: " + label
			default:
				return "Неоднозначность: " + label
			}
		}
	}
	return "Диагностика: " + w
}
func timingText(run api.Run) string {
	if run.NormalizationCompletedAt == nil || run.Total == 0 {
		return ""
	}
	start, e1 := time.Parse(time.RFC3339Nano, run.CreatedAt)
	end, e2 := time.Parse(time.RFC3339Nano, *run.NormalizationCompletedAt)
	if e1 != nil || e2 != nil || end.Before(start) {
		return ""
	}
	elapsed := end.Sub(start)
	return fmt.Sprintf("Разбор: %s · в среднем: %s на запись", terminalDuration(elapsed), terminalDuration(elapsed/time.Duration(run.Total)))
}

func terminalDuration(value time.Duration) string {
	number := func(v float64) string {
		return strings.ReplaceAll(strings.TrimRight(strings.TrimRight(strconv.FormatFloat(v, 'f', 2, 64), "0"), "."), ".", ",")
	}
	if value > 0 && value < 10*time.Microsecond {
		return "менее 0,01 мс"
	}
	if value < time.Second {
		return number(float64(value)/float64(time.Millisecond)) + " мс"
	}
	if value < time.Minute {
		return number(float64(value)/float64(time.Second)) + " с"
	}
	seconds := int(value.Round(time.Second) / time.Second)
	if seconds < 3600 {
		return fmt.Sprintf("%d мин %d с", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%d ч %d мин %d с", seconds/3600, (seconds/60)%60, seconds%60)
}

// Export is an exact snapshot view, not a publication and not an answer cache.
// The destination is never silently overwritten, and failed writes leave no partial file.
func exportView(path, format, id, scope string, rows []api.Row) error {
	if format != "json" && format != "csv" {
		return errors.New("Формат экспорта: json или csv")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".jana-view-*")
	if err != nil {
		return errors.New("Не удалось создать файл экспорта")
	}
	defer os.Remove(f.Name())
	if format == "json" {
		encoder := json.NewEncoder(f)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(struct {
			RunID string    `json:"run_id"`
			Scope string    `json:"scope"`
			Rows  []api.Row `json:"rows"`
		}{id, scope, rows})
	} else {
		writer := csv.NewWriter(f)
		err = writer.Write([]string{"position", "source_id", "raw_address", "raw_address_is_null", "status", "reviewed", "normalized_address", "warnings"})
		for _, row := range rows {
			if err != nil {
				break
			}
			raw := ""
			if row.RawAddress != nil {
				raw = *row.RawAddress
			}
			err = writer.Write([]string{fmt.Sprint(row.Position + 1), row.SourceId, raw, fmt.Sprint(row.RawAddress == nil), string(row.Result.Status), fmt.Sprint(row.Reviewed), normalizedAddress(row.Result), strings.Join(row.Result.Warnings, "; ")})
		}
		writer.Flush()
		if err == nil {
			err = writer.Error()
		}
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Link(f.Name(), path); errors.Is(err, os.ErrExist) {
		return errors.New("Файл уже существует; выберите другой путь")
	}
	return err
}
