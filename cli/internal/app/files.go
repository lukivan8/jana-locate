package app

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"example.com/jana-locate-cli/internal/api"
)

func stringPtr(v string) *string { return &v }
func IsFile(src api.Source) bool { return src.Kind != nil && *src.Kind == "file" }

// ImportFile fixes the input locally before the first normalization request.
// Positional source keys allow duplicate user IDs, which are stored separately.
func (s *Store) ImportFile(ctx context.Context, path, format, apiURL string) (api.Run, error) {
	f, err := os.Open(path)
	if err != nil {
		return api.Run{}, errors.New("не удалось открыть входной файл")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxInputBytes+1))
	if err != nil {
		return api.Run{}, err
	}
	if len(b) > maxInputBytes {
		return api.Run{}, errors.New("входной файл превышает 64 МиБ")
	}
	if !utf8.Valid(b) {
		return api.Run{}, errors.New("входной файл должен быть в UTF-8")
	}
	if format == "auto" {
		format = "lines"
		if strings.EqualFold(filepath.Ext(path), ".json") {
			format = "json"
		}
	}
	input := []sourceRow{}
	ids := []json.RawMessage{}
	add := func(raw *string, id json.RawMessage) {
		input = append(input, sourceRow{ID: fmt.Sprint(len(input) + 1), Raw: raw})
		ids = append(ids, id)
	}
	switch format {
	case "json":
		var value json.RawMessage
		d := json.NewDecoder(bytes.NewReader(b))
		if err = d.Decode(&value); err != nil {
			return api.Run{}, errors.New("некорректный JSON-файл")
		}
		if d.Decode(new(any)) != io.EOF {
			return api.Run{}, errors.New("в файле должен быть один JSON-документ")
		}
		value = bytes.TrimSpace(value)
		if len(value) > 0 && value[0] == '{' {
			var envelope struct {
				Records json.RawMessage `json:"records"`
			}
			if err = json.Unmarshal(value, &envelope); err != nil || envelope.Records == nil {
				return api.Run{}, errors.New("JSON должен быть массивом или объектом {records:[...]}")
			}
			value = envelope.Records
		}
		array := json.NewDecoder(bytes.NewReader(value))
		token, err := array.Token()
		if err != nil || token != json.Delim('[') {
			return api.Run{}, errors.New("JSON должен содержать массив адресов")
		}
		for i := 0; array.More(); i++ {
			if i >= maxRecords {
				return api.Run{}, errors.New("в файле более 100000 записей")
			}
			var record json.RawMessage
			if err = array.Decode(&record); err != nil {
				return api.Run{}, errors.New("некорректная запись JSON")
			}
			record = bytes.TrimSpace(record)
			var raw string
			if len(record) > 0 && record[0] == '"' {
				if err = json.Unmarshal(record, &raw); err != nil {
					return api.Run{}, err
				}
				id, _ := json.Marshal(fmt.Sprint(i + 1))
				add(&raw, id)
				continue
			}
			var r struct {
				Id  json.RawMessage `json:"id"`
				Raw *string         `json:"raw_address"`
			}
			if err = json.Unmarshal(record, &r); err != nil || r.Raw == nil || len(r.Id) == 0 {
				return api.Run{}, fmt.Errorf("запись %d: нужны id и строка raw_address", i+1)
			}
			var id any
			d := json.NewDecoder(bytes.NewReader(r.Id))
			d.UseNumber()
			if err = d.Decode(&id); err != nil {
				return api.Run{}, err
			}
			valid := false
			switch v := id.(type) {
			case string:
				valid = true
			case json.Number:
				_, err = v.Int64()
				valid = err == nil
			}
			if !valid {
				return api.Run{}, fmt.Errorf("запись %d: id должен быть строкой или int64", i+1)
			}
			add(r.Raw, r.Id)
		}
	case "lines":
		reader := bufio.NewReader(bytes.NewReader(b))
		for {
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				if len(input) >= maxRecords {
					return api.Run{}, errors.New("в файле более 100000 строк")
				}
				if strings.HasSuffix(line, "\n") {
					line = strings.TrimSuffix(line, "\n")
					line = strings.TrimSuffix(line, "\r")
				}
				id, _ := json.Marshal(fmt.Sprint(len(input) + 1))
				add(&line, id)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return api.Run{}, err
			}
		}
	default:
		return api.Run{}, errors.New("формат должен быть auto, json или lines")
	}
	src := api.Source{Kind: stringPtr("file"), FileName: stringPtr(filepath.Base(path)), Table: filepath.Base(path), Key: "position", Column: "raw_address", KeyType: "text"}
	return s.importSnapshot(ctx, src, apiURL, func(accept func(sourceRow) error) error {
		for _, r := range input {
			if err := accept(r); err != nil {
				return err
			}
		}
		return nil
	}, ids)
}

// ImportRecords snapshots the browser's reviewed input without a temporary file.
func (s *Store) ImportRecords(ctx context.Context, records []api.InputRecord, apiURL string) (api.Run, error) {
	ids := make([]json.RawMessage, len(records))
	for i, record := range records {
		switch v := record.Id.(type) {
		case string:
		case json.Number:
			if _, err := v.Int64(); err != nil {
				return api.Run{}, errors.New("id должен быть строкой или int64")
			}
		default:
			return api.Run{}, errors.New("id должен быть строкой или int64")
		}
		encoded, err := json.Marshal(record.Id)
		if err != nil {
			return api.Run{}, err
		}
		ids[i] = encoded
	}
	src := api.Source{Kind: stringPtr("file"), FileName: stringPtr("Ввод адресов"), Table: "Ввод адресов", Key: "position", Column: "raw_address", KeyType: "text"}
	return s.importSnapshot(ctx, src, apiURL, func(accept func(sourceRow) error) error {
		for i, record := range records {
			raw := record.RawAddress
			if err := accept(sourceRow{ID: fmt.Sprint(i + 1), Raw: &raw}); err != nil {
				return err
			}
		}
		return nil
	}, ids)
}

// ExportFile uses an atomic replacement, and requires explicit overwrite intent.
func (s *Store) ExportFile(ctx context.Context, id, path string, revision int, overwrite bool) (api.Run, error) {
	return s.ExportFileWithOptions(ctx, id, path, revision, overwrite, WriteOptions{})
}

func (s *Store) ExportFileWithOptions(ctx context.Context, id, path string, revision int, overwrite bool, options WriteOptions) (api.Run, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return api.Run{}, err
	}
	defer tx.Rollback()
	run, err := readRun(tx, ctx, id)
	if err != nil {
		return run, err
	}
	if run.Revision != revision || !IsFile(run.Source) || (run.Status != api.Review && run.Status != api.Published) {
		return run, ErrConflict
	}
	rows, err := tx.QueryContext(ctx, "SELECT result FROM records WHERE run_id=?"+writeFilter(options.ReadyOnly)+" ORDER BY position", id)
	if err != nil {
		return run, err
	}
	results := []json.RawMessage{}
	for rows.Next() {
		var b string
		if err = rows.Scan(&b); err != nil {
			rows.Close()
			return run, err
		}
		results = append(results, json.RawMessage(b))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return run, err
	}
	if options.ReadyOnly && len(results) == 0 {
		return run, errors.New("Нет готовых записей для публикации")
	}
	if options.Progress != nil {
		options.Progress(0, len(results))
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".jana-results-*.tmp")
	if err != nil {
		return run, errors.New("не удалось создать выходной файл")
	}
	defer os.Remove(f.Name())
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(struct {
		Results []json.RawMessage `json:"results"`
	}{results})
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return run, err
	}
	if overwrite {
		err = os.Rename(f.Name(), path)
	} else {
		err = os.Link(f.Name(), path)
	}
	if errors.Is(err, os.ErrExist) {
		return run, errors.New("выходной файл уже существует; выберите другой путь или задайте --force")
	}
	if err != nil {
		return run, err
	}
	if options.Progress != nil {
		options.Progress(len(results), len(results))
	}
	if run.Status != api.Published {
		run.Status = api.Published
		count := len(results)
		if options.ReadyOnly && count < run.Total {
			run.Status = api.Review
		}
		run.PublishedCount = &count
		publishedAt := stamp()
		run.LastPublishedAt = &publishedAt
		if err = saveRun(ctx, tx, run); err != nil {
			return run, err
		}
		if err = tx.Commit(); err != nil {
			return run, err
		}
		return s.Get(ctx, id)
	}
	return run, nil
}

// AddInputIDColumn upgrades journals from the first CLI version in place.
func addInputIDColumn(db *sql.DB) error { return addRecordTextColumn(db, "input_id") }
func addRecordTextColumn(db *sql.DB, column string) error {
	rows, err := db.Query("PRAGMA table_info(records)")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var def any
		if err = rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !found {
		_, err = db.Exec("ALTER TABLE records ADD COLUMN " + column + " TEXT")
	}
	return err
}
