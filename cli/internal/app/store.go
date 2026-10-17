// Package app owns local orchestration. It never parses addresses itself.
package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"example.com/jana-locate-cli/internal/api"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("Запуск или запись не найдены")
var ErrConflict = errors.New("Версия изменилась или действие сейчас недоступно")
var ErrSourceChanged = errors.New("Исходные строки изменились или удалены; создайте новый снимок")

const maxRecords = 100000
const maxInputBytes = 64 << 20

type Store struct{ DB *sql.DB }
type sourceRow struct {
	ID  string
	Raw *string
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	f.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON; PRAGMA journal_mode=DELETE; PRAGMA synchronous=FULL;
 CREATE TABLE IF NOT EXISTS runs(id TEXT PRIMARY KEY,metadata TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS records(run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,position INTEGER NOT NULL,source_id TEXT NOT NULL,raw_address TEXT,result TEXT,reviewed INTEGER NOT NULL DEFAULT 0,revision INTEGER NOT NULL DEFAULT 1,PRIMARY KEY(run_id,position),UNIQUE(run_id,source_id));`)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err = addInputIDColumn(db); err != nil {
		db.Close()
		return nil, err
	}
	if err = addRecordTextColumn(db, "city_selection"); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db}, nil
}
func id() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func stamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func (s *Store) Import(ctx context.Context, src api.Source, apiURL string, stream func(func(sourceRow) error) error) (api.Run, error) {
	return s.importSnapshot(ctx, src, apiURL, stream, nil)
}
func (s *Store) importSnapshot(ctx context.Context, src api.Source, apiURL string, stream func(func(sourceRow) error) error, ids []json.RawMessage) (api.Run, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return api.Run{}, err
	}
	defer tx.Rollback()
	r := api.Run{Id: id(), Source: src, ApiUrl: apiURL, Status: api.Normalizing, Revision: 1, CreatedAt: stamp(), UpdatedAt: stamp()}
	b, _ := json.Marshal(r)
	if _, err = tx.ExecContext(ctx, "INSERT INTO runs VALUES(?,?)", r.Id, string(b)); err != nil {
		return r, err
	}
	count, size := 0, 0
	err = stream(func(row sourceRow) error {
		count++
		if row.Raw != nil {
			size += len(*row.Raw)
			if len(*row.Raw) > 4096 {
				return errors.New("Исходный адрес превышает 4096 байт")
			}
		}
		size += len(row.ID)
		if count > maxRecords || size > maxInputBytes {
			return errors.New("Снимок превышает 100 000 строк или 64 МиБ; сократите выборку источника")
		}
		var inputID any
		if ids != nil {
			inputID = string(ids[count-1])
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO records(run_id,position,source_id,raw_address,input_id) VALUES(?,?,?,?,?)", r.Id, count-1, row.ID, row.Raw, inputID)
		return err
	})
	if err != nil {
		return r, err
	}
	if count == 0 {
		return r, errors.New("Исходная таблица пуста")
	}
	r.Total = count
	b, _ = json.Marshal(r)
	if _, err = tx.ExecContext(ctx, "UPDATE runs SET metadata=? WHERE id=?", string(b), r.Id); err != nil {
		return r, err
	}
	return r, tx.Commit()
}
func readRun(q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, ctx context.Context, id string) (api.Run, error) {
	var text string
	err := q.QueryRowContext(ctx, "SELECT metadata FROM runs WHERE id=?", id).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return api.Run{}, ErrNotFound
	}
	if err != nil {
		return api.Run{}, err
	}
	var r api.Run
	if err = json.Unmarshal([]byte(text), &r); err != nil {
		return r, err
	}
	err = q.QueryRowContext(ctx, "SELECT count(*),count(result),coalesce(sum(reviewed),0) FROM records WHERE run_id=?", id).Scan(&r.Total, &r.Processed, &r.Reviewed)
	return r, err
}
func (s *Store) Get(ctx context.Context, id string) (api.Run, error) { return readRun(s.DB, ctx, id) }
func (s *Store) List(ctx context.Context) ([]api.Run, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id FROM runs ORDER BY rowid DESC")
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []api.Run{}
	for _, id := range ids {
		r, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
func emptyResult(position int, raw *string) api.Result {
	r := api.Result{Id: fmt.Sprint(position), Status: api.Unmatched, Warnings: []string{}}
	if raw != nil {
		r.RawAddress = *raw
	} else {
		r.Warnings = []string{"source_address_null"}
	}
	return r
}
func (s *Store) Rows(ctx context.Context, id string, offset, limit int, reviewOnly, pending bool) ([]api.Row, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	where := ""
	if pending {
		where = " AND result IS NULL"
	}
	if reviewOnly {
		where = " AND reviewed=0 AND (result IS NULL OR json_extract(result,'$.status')<>'matched')"
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT position,source_id,raw_address,result,reviewed,revision,input_id,city_selection FROM records WHERE run_id=?"+where+" ORDER BY position LIMIT ? OFFSET ?", id, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func scanRows(rows *sql.Rows) ([]api.Row, error) {
	var err error
	out := []api.Row{}
	for rows.Next() {
		var r api.Row
		var result, inputID, citySelection sql.NullString
		if err = rows.Scan(&r.Position, &r.SourceId, &r.RawAddress, &result, &r.Reviewed, &r.Revision, &inputID, &citySelection); err != nil {
			return nil, err
		}
		if citySelection.Valid {
			if err = json.Unmarshal([]byte(citySelection.String), &r.CitySelection); err != nil {
				return nil, err
			}
		}
		normalized := result.Valid
		r.Normalized = &normalized
		if inputID.Valid {
			var id any
			d := json.NewDecoder(strings.NewReader(inputID.String))
			d.UseNumber()
			if err = d.Decode(&id); err != nil {
				return nil, err
			}
			v := fmt.Sprint(id)
			r.InputId = &v
		}
		r.Result = emptyResult(r.Position, r.RawAddress)
		if result.Valid {
			d := json.NewDecoder(strings.NewReader(result.String))
			d.UseNumber()
			if err = d.Decode(&r.Result); err != nil {
				return nil, err
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func saveRun(ctx context.Context, tx *sql.Tx, r api.Run) error {
	r.Revision++
	r.UpdatedAt = stamp()
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE runs SET metadata=? WHERE id=?", string(b), r.Id)
	return err
}
func (s *Store) Status(ctx context.Context, id string, revision int, status api.RunStatus, message string) (api.Run, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return api.Run{}, err
	}
	defer tx.Rollback()
	r, err := readRun(tx, ctx, id)
	if err != nil {
		return r, err
	}
	if revision > 0 && r.Revision != revision {
		return r, ErrConflict
	}
	if status == api.Normalizing && r.Status != api.Failed {
		return r, ErrConflict
	}
	r.Status = status
	r.Error = message
	if err = saveRun(ctx, tx, r); err != nil {
		return r, err
	}
	if err = tx.Commit(); err != nil {
		return r, err
	}
	return s.Get(ctx, id)
}
func (s *Store) SaveBatch(ctx context.Context, id string, rows []api.Row) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := readRun(tx, ctx, id)
	if err != nil {
		return err
	}
	if r.Status != api.Normalizing {
		return ErrConflict
	}
	for _, row := range rows {
		var original sql.NullString
		if err = tx.QueryRowContext(ctx, "SELECT input_id FROM records WHERE run_id=? AND position=?", id, row.Position).Scan(&original); err != nil {
			return err
		}
		if original.Valid {
			d := json.NewDecoder(strings.NewReader(original.String))
			d.UseNumber()
			if err = d.Decode(&row.Result.Id); err != nil {
				return err
			}
		}
		b, err := json.Marshal(row.Result)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, "UPDATE records SET result=? WHERE run_id=? AND position=? AND result IS NULL", string(b), id, row.Position)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrConflict
		}
	}
	if r.Processed+len(rows) == r.Total {
		r.Status = api.Review
		completedAt := stamp()
		r.NormalizationCompletedAt = &completedAt
	}
	if err = saveRun(ctx, tx, r); err != nil {
		return err
	}
	return tx.Commit()
}
func fields(r api.Result) api.AddressFields {
	return api.AddressFields{Province: r.Province, City: r.City, District: r.District, LivingArea: r.LivingArea, StreetType: r.StreetType, StreetName: r.StreetName, House: r.House, Apartment: r.Apartment, Entrance: r.Entrance, Floor: r.Floor, Landmark: r.Landmark}
}
func building(f api.AddressFields) api.AddressFields {
	f.Apartment = nil
	f.Entrance = nil
	f.Floor = nil
	f.Landmark = nil
	return f
}
func (s *Store) Review(ctx context.Context, id string, position, revision int, f api.AddressFields) (api.Row, error) {
	b, _ := json.Marshal(f)
	var vals map[string]*string
	_ = json.Unmarshal(b, &vals)
	for _, v := range vals {
		if v != nil && len(*v) > 4096 {
			return api.Row{}, errors.New("Значение поля превышает 4096 байт")
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return api.Row{}, err
	}
	defer tx.Rollback()
	run, err := readRun(tx, ctx, id)
	if err != nil {
		return api.Row{}, err
	}
	if run.Status != api.Review {
		return api.Row{}, ErrConflict
	}
	var row api.Row
	var result string
	err = tx.QueryRowContext(ctx, "SELECT position,source_id,raw_address,result,reviewed,revision FROM records WHERE run_id=? AND position=?", id, position).Scan(&row.Position, &row.SourceId, &row.RawAddress, &result, &row.Reviewed, &row.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, err
	}
	if row.Revision != revision {
		return row, ErrConflict
	}
	d := json.NewDecoder(strings.NewReader(result))
	d.UseNumber()
	if err = d.Decode(&row.Result); err != nil {
		return row, err
	}
	r := row.Result
	if !reflect.DeepEqual(building(fields(r)), building(f)) {
		r.Coordinates = nil
		r.CatalogId = nil
		r.CatalogRevision = nil
		r.Status = api.NeedsReview
	}
	r.Province = f.Province
	r.City = f.City
	r.District = f.District
	r.LivingArea = f.LivingArea
	r.StreetType = f.StreetType
	r.StreetName = f.StreetName
	r.House = f.House
	r.Apartment = f.Apartment
	r.Entrance = f.Entrance
	r.Floor = f.Floor
	r.Landmark = f.Landmark
	found := false
	for _, w := range r.Warnings {
		if w == "manually_reviewed_fields" {
			found = true
		}
	}
	if !found {
		r.Warnings = append(r.Warnings, "manually_reviewed_fields")
	}
	row.Result = r
	row.Revision++
	row.Reviewed = true
	b, _ = json.Marshal(r)
	if _, err = tx.ExecContext(ctx, "UPDATE records SET result=?,reviewed=1,revision=? WHERE run_id=? AND position=?", string(b), row.Revision, id, position); err != nil {
		return row, err
	}
	if err = saveRun(ctx, tx, run); err != nil {
		return row, err
	}
	return row, tx.Commit()
}
