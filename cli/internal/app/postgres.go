package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"example.com/jana-locate-cli/internal/api"
	"github.com/jackc/pgx/v5"
)

type Postgres struct{ DSN, APIURL string }

func (p Postgres) connect(ctx context.Context) (*pgx.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c, err := pgx.Connect(ctx, p.DSN)
	if err != nil {
		return nil, errors.New("Не удалось подключиться к PostgreSQL; проверьте хост, порт, базу, пользователя, пароль и доступность сети")
	}
	return c, nil
}
func (p Postgres) identity() (string, error) {
	c, err := pgx.ParseConfig(p.DSN)
	if err != nil {
		return "", errors.New("Некорректные локальные параметры PostgreSQL")
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d/%s", c.Host, c.Port, c.Database)))), nil
}
func qi(names ...string) string { return pgx.Identifier(names).Sanitize() }
func validNames(src api.Source) bool {
	for _, v := range []string{src.Schema, src.Table, src.Key, src.Column, src.TargetSchema, src.TargetTable} {
		if v == "" || len(v) > 63 || strings.ContainsRune(v, 0) {
			return false
		}
	}
	return src.Schema != src.TargetSchema || src.Table != src.TargetTable
}
func sourceType(ctx context.Context, tx pgx.Tx, src api.Source) (string, error) {
	var relkind string
	if err := tx.QueryRow(ctx, `SELECT c.relkind::text FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=$2`, src.Schema, src.Table).Scan(&relkind); err != nil || relkind != "r" {
		return "", errors.New("Источник должен быть обычной таблицей PostgreSQL")
	}
	rows, err := tx.Query(ctx, `SELECT a.attname,a.atttypid::int FROM pg_index i JOIN pg_class c ON c.oid=i.indrelid JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum=ANY(i.indkey) WHERE i.indisprimary AND n.nspname=$1 AND c.relname=$2`, src.Schema, src.Table)
	if err != nil {
		return "", errors.New("Не удалось проверить первичный ключ исходной таблицы")
	}
	keys := 0
	name := ""
	oid := 0
	for rows.Next() {
		keys++
		if err = rows.Scan(&name, &oid); err != nil {
			rows.Close()
			return "", errors.New("Некорректные сведения о первичном ключе")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", errors.New("Не удалось прочитать сведения о первичном ключе")
	}
	types := map[int]string{20: "bigint", 21: "smallint", 23: "integer", 25: "text", 1043: "text", 1042: "text", 2950: "uuid"}
	typ := types[oid]
	if keys != 1 || name != src.Key || typ == "" || oid == 1042 {
		return "", errors.New("Выберите одиночный первичный ключ типа integer, bigint, smallint, text, varchar или uuid")
	}
	var addrOID int
	if err = tx.QueryRow(ctx, `SELECT a.atttypid::int FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=$2 AND a.attname=$3 AND NOT a.attisdropped`, src.Schema, src.Table, src.Column).Scan(&addrOID); err != nil || (addrOID != 25 && addrOID != 1043) {
		return "", errors.New("Колонка адреса должна иметь тип text или varchar")
	}
	return typ, nil
}
func inspectTarget(ctx context.Context, tx pgx.Tx, src api.Source) (bool, error) {
	target := qi(src.TargetSchema, src.TargetTable)
	marker := "jana-locate-cli/v1:" + qi(src.Schema, src.Table) + ":" + qi(src.Key, src.Column) + ":" + src.KeyType
	var exists bool
	var existing *string
	if err := tx.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL,obj_description(to_regclass($1),'pg_class')`, target).Scan(&exists, &existing); err != nil {
		return false, errors.New("Не удалось проверить таблицу результата")
	}
	if exists && (existing == nil || *existing != marker) {
		return false, errors.New("Таблица результата уже существует и не принадлежит этому источнику Jana. Укажите другое имя таблицы результата")
	}
	return exists, nil
}
func (p Postgres) Import(ctx context.Context, s *Store, src api.Source) (api.Run, error) {
	if !validNames(src) {
		return api.Run{}, errors.New("Укажите схему, таблицу, первичный ключ, колонку адреса и отдельную таблицу результата; имена — до 63 байт")
	}
	conn, err := p.connect(ctx)
	if err != nil {
		return api.Run{}, err
	}
	defer conn.Close(context.Background())
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return api.Run{}, errors.New("Не удалось создать снимок исходной таблицы")
	}
	defer tx.Rollback(context.Background())
	typ, err := sourceType(ctx, tx, src)
	if err != nil {
		return api.Run{}, err
	}
	src.KeyType = typ
	identity, e := p.identity()
	if e != nil {
		return api.Run{}, e
	}
	src.ConnectionId = &identity
	if err = checkReferences(ctx, tx, src); err != nil {
		return api.Run{}, err
	}
	var writable bool
	if err = tx.QueryRow(ctx, `SELECT has_schema_privilege(current_user,$1,'USAGE') AND has_schema_privilege(current_user,$1,'CREATE')`, src.TargetSchema).Scan(&writable); err != nil || !writable {
		return api.Run{}, errors.New("Схема результата должна существовать; нужны права USAGE и CREATE")
	}
	if _, err = inspectTarget(ctx, tx, src); err != nil {
		return api.Run{}, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf("SELECT %s::text,%s FROM %s ORDER BY %s", qi(src.Key), qi(src.Column), qi(src.Schema, src.Table), qi(src.Key)))
	if err != nil {
		return api.Run{}, errors.New("Не удалось прочитать исходную таблицу; проверьте права SELECT")
	}
	defer rows.Close()
	run, err := s.Import(ctx, src, p.APIURL, func(accept func(sourceRow) error) error {
		for rows.Next() {
			var row sourceRow
			if err := rows.Scan(&row.ID, &row.Raw); err != nil {
				return errors.New("Не удалось прочитать значения исходной строки")
			}
			if err := accept(row); err != nil {
				return err
			}
		}
		if rows.Err() != nil {
			return errors.New("Создание снимка источника прервано")
		}
		return nil
	})
	// The read-only PostgreSQL transaction can be rolled back after all rows have
	// been copied. SQLite already owns the complete, consistent input snapshot.
	return run, err
}
func (p Postgres) Publish(ctx context.Context, s *Store, id string, revision int) (api.Run, error) {
	return p.PublishWithOptions(ctx, s, id, revision, WriteOptions{})
}

func (p Postgres) PublishWithOptions(ctx context.Context, s *Store, id string, revision int, options WriteOptions) (api.Run, error) {
	// Hold a local read/write transaction throughout publication. Other edits
	// cannot commit while the selected local revision is being published.
	local, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return api.Run{}, err
	}
	defer local.Rollback()
	run, err := readRun(local, ctx, id)
	if err != nil {
		return run, err
	}
	if run.Revision != revision {
		return run, ErrConflict
	}
	if run.Status == api.Published {
		return run, nil
	}
	if run.Status != api.Review || run.Processed != run.Total {
		return run, ErrConflict
	}
	total := run.Total
	if options.ReadyOnly {
		if err = local.QueryRowContext(ctx, "SELECT count(*) FROM records WHERE run_id=?"+writeFilter(true), id).Scan(&total); err != nil {
			return run, err
		}
		if total == 0 {
			return run, errors.New("Нет готовых записей для публикации")
		}
	}
	if options.Progress != nil {
		options.Progress(0, total)
	}
	done := 0
	src := run.Source
	identity, e := p.identity()
	if e != nil {
		return run, e
	}
	if src.ConnectionId != nil && *src.ConnectionId != identity {
		return run, errors.New("подключение отличается от источника запуска; выберите исходный профиль")
	}
	if IsFile(src) {
		return run, errors.New("Для файлового запуска используйте publish или экспорт JSON")
	}
	if !validNames(src) {
		return run, errors.New("Некорректные сохранённые параметры источника")
	}
	conn, err := p.connect(ctx)
	if err != nil {
		return run, err
	}
	defer conn.Close(context.Background())
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return run, errors.New("Не удалось начать транзакцию записи результата")
	}
	defer tx.Rollback(context.Background())
	typ, err := sourceType(ctx, tx, src)
	if err != nil || typ != src.KeyType {
		return run, errors.New("Структура исходной таблицы изменилась; создайте новый снимок")
	}
	if err = checkReferences(ctx, tx, src); err != nil {
		return run, err
	}
	target := qi(src.TargetSchema, src.TargetTable)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, target); err != nil {
		return run, errors.New("Не удалось заблокировать таблицу для записи результата")
	}
	marker := "jana-locate-cli/v1:" + qi(src.Schema, src.Table) + ":" + qi(src.Key, src.Column) + ":" + typ
	exists, err := inspectTarget(ctx, tx, src)
	if err != nil {
		return run, err
	}
	fieldNames := []string{"province", "city", "district", "living_area", "street_type", "street_name", "house", "apartment", "entrance", "floor", "landmark"}
	definitions := []string{}
	for _, f := range fieldNames {
		definitions = append(definitions, qi(f)+" text")
	}
	create := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (run_id text NOT NULL,source_id %s NOT NULL,raw_address text,%s,confidence double precision NOT NULL,coordinates jsonb,status text NOT NULL,warnings jsonb NOT NULL,reviewed boolean NOT NULL,revision bigint NOT NULL,PRIMARY KEY(run_id,source_id))", target, typ, strings.Join(definitions, ","))
	if _, err = tx.Exec(ctx, create); err != nil {
		return run, errors.New("Не удалось создать таблицу результата; проверьте права в схеме результата")
	}
	// Quote the marker as a SQL literal; identifiers above are separately quoted.
	if !exists {
		if _, err = tx.Exec(ctx, "COMMENT ON TABLE "+target+" IS '"+strings.ReplaceAll(marker, "'", "''")+"'"); err != nil {
			return run, errors.New("Не удалось сохранить отметку Jana для таблицы результата")
		}
	}
	if src.ForeignKey != nil && *src.ForeignKey {
		var present bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid=to_regclass($1) AND conname='jana_source_fk' AND contype='f' AND confrelid=to_regclass($2))`, target, qi(src.Schema, src.Table)).Scan(&present); err != nil {
			return run, errors.New("не удалось проверить внешний ключ")
		}
		if !present {
			if _, err = tx.Exec(ctx, "ALTER TABLE "+target+" ADD CONSTRAINT jana_source_fk FOREIGN KEY (source_id) REFERENCES "+qi(src.Schema, src.Table)+" ("+qi(src.Key)+") ON DELETE NO ACTION"); err != nil {
				return run, errors.New("не удалось создать внешний ключ; проверьте REFERENCES и существующие результаты")
			}
		}
	}
	if _, err = tx.Exec(ctx, "DELETE FROM "+target+" WHERE run_id=$1", id); err != nil {
		return run, errors.New("Не удалось заменить предыдущие результаты этого запуска")
	}
	rows, err := local.QueryContext(ctx, "SELECT position,source_id,raw_address,result,reviewed,revision FROM records WHERE run_id=?"+writeFilter(options.ReadyOnly)+" ORDER BY position", id)
	if err != nil {
		return run, err
	}
	columns := []string{"run_id", "source_id", "raw_address"}
	columns = append(columns, fieldNames...)
	columns = append(columns, "confidence", "coordinates", "status", "warnings", "reviewed", "revision")
	quoted := []string{}
	placeholders := []string{}
	for i, c := range columns {
		quoted = append(quoted, qi(c))
		v := fmt.Sprintf("$%d", i+1)
		if c == "source_id" {
			v = "(" + v + "::text)::" + typ
		}
		placeholders = append(placeholders, v)
	}
	insert := "INSERT INTO " + target + " (" + strings.Join(quoted, ",") + ") VALUES (" + strings.Join(placeholders, ",") + ")"
	batch := []api.Row{}
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		ids := []string{}
		for _, r := range batch {
			ids = append(ids, r.SourceId)
		}
		query := fmt.Sprintf("SELECT %s::text,%s FROM %s WHERE %s=ANY(ARRAY(SELECT unnest($1::text[])::%s))", qi(src.Key), qi(src.Column), qi(src.Schema, src.Table), qi(src.Key), typ)
		current, err := tx.Query(ctx, query, ids)
		if err != nil {
			return errors.New("Не удалось проверить текущее состояние исходных строк")
		}
		actual := map[string]*string{}
		for current.Next() {
			var key string
			var raw *string
			if err = current.Scan(&key, &raw); err != nil {
				current.Close()
				return errors.New("Не удалось прочитать значения текущих исходных строк")
			}
			actual[key] = raw
		}
		err = current.Err()
		current.Close()
		if err != nil {
			return errors.New("Проверка исходных строк прервана")
		}
		requests := &pgx.Batch{}
		for _, r := range batch {
			v, ok := actual[r.SourceId]
			if !ok || !reflect.DeepEqual(v, r.RawAddress) {
				return ErrSourceChanged
			}
			f := fields(r.Result)
			var coords any
			if r.Result.Coordinates != nil {
				b, _ := json.Marshal(r.Result.Coordinates)
				coords = string(b)
			}
			warnings, _ := json.Marshal(r.Result.Warnings)
			values := []any{id, r.SourceId, r.RawAddress, f.Province, f.City, f.District, f.LivingArea, f.StreetType, f.StreetName, f.House, f.Apartment, f.Entrance, f.Floor, f.Landmark, r.Result.Confidence, coords, string(r.Result.Status), string(warnings), r.Reviewed, r.Revision}
			requests.Queue(insert, values...)
		}
		replies := tx.SendBatch(ctx, requests)
		for range batch {
			if _, err = replies.Exec(); err != nil {
				replies.Close()
				return errors.New("Не удалось записать строки результата; транзакция отменена")
			}
		}
		if err = replies.Close(); err != nil {
			return errors.New("Не удалось записать пакет результатов")
		}
		done += len(batch)
		if options.Progress != nil {
			options.Progress(done, total)
		}
		batch = nil
		return nil
	}
	for rows.Next() {
		var row api.Row
		var result string
		if err = rows.Scan(&row.Position, &row.SourceId, &row.RawAddress, &result, &row.Reviewed, &row.Revision); err != nil {
			rows.Close()
			return run, err
		}
		if err = json.Unmarshal([]byte(result), &row.Result); err != nil {
			rows.Close()
			return run, err
		}
		batch = append(batch, row)
		if len(batch) == 100 {
			if err = flush(); err != nil {
				rows.Close()
				return run, err
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return run, err
	}
	if err = flush(); err != nil {
		return run, err
	}
	if err = tx.Commit(ctx); err != nil {
		return run, errors.New("Не удалось подтвердить запись результата или получить подтверждение; повторите публикацию того же запуска")
	}
	run.Status = api.Published
	if options.ReadyOnly && total < run.Total {
		run.Status = api.Review
	}
	run.PublishedCount = &total
	publishedAt := stamp()
	run.LastPublishedAt = &publishedAt
	run.Error = ""
	if err = saveRun(ctx, local, run); err != nil {
		return run, errors.New("Результаты записаны в PostgreSQL, но локальный прогресс не сохранён; повторите публикацию того же запуска")
	}
	if err = local.Commit(); err != nil {
		return run, errors.New("Результаты записаны в PostgreSQL, но локальный прогресс не сохранён; повторите публикацию того же запуска")
	}
	return s.Get(ctx, id)
}

// Check validates connect without importing addresses or writing source/result tables.
func (p Postgres) Check(ctx context.Context, src api.Source) (api.Source, error) {
	if !validNames(src) {
		return src, errors.New("укажите таблицу, колонку, первичный ключ и отдельную таблицу результата")
	}
	c, err := p.connect(ctx)
	if err != nil {
		return src, err
	}
	defer c.Close(context.Background())
	tx, err := c.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return src, err
	}
	defer tx.Rollback(context.Background())
	typ, err := sourceType(ctx, tx, src)
	if err != nil {
		return src, err
	}
	src.KeyType = typ
	identity, e := p.identity()
	if e != nil {
		return src, e
	}
	src.ConnectionId = &identity
	if _, err = tx.Exec(ctx, "SELECT "+qi(src.Key)+","+qi(src.Column)+" FROM "+qi(src.Schema, src.Table)+" LIMIT 0"); err != nil {
		return src, errors.New("нет доступа SELECT к источнику")
	}
	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT has_schema_privilege(current_user,$1,'USAGE') AND has_schema_privilege(current_user,$1,'CREATE')`, src.TargetSchema).Scan(&allowed); err != nil || !allowed {
		return src, errors.New("нужны USAGE/CREATE в схеме результата")
	}
	if err = checkReferences(ctx, tx, src); err != nil {
		return src, err
	}
	if _, err = inspectTarget(ctx, tx, src); err != nil {
		return src, err
	}
	return src, nil
}
func checkReferences(ctx context.Context, tx pgx.Tx, src api.Source) error {
	if src.ForeignKey == nil || !*src.ForeignKey {
		return nil
	}
	var allowed bool
	if err := tx.QueryRow(ctx, `SELECT has_column_privilege(current_user,$1::text,$2::text,'REFERENCES')`, qi(src.Schema, src.Table), src.Key).Scan(&allowed); err != nil || !allowed {
		return errors.New("для внешнего ключа нужно REFERENCES на исходный PK; используйте --no-foreign-key для связи через JOIN")
	}
	return nil
}
