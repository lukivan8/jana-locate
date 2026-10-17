package app

import (
	"context"
	"errors"
	"example.com/jana-locate-cli/internal/api"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// No credential is returned to the browser. Saving is owned by the local CLI.
type Connections struct {
	Save func(context.Context, string, api.Source) error
	DSN  func() string
	Load func() (string, api.ConnectionProfile, error)
}

// DescribeConnection returns only non-secret defaults. Parsing preserves the
// driver's TLS semantics, including non-URL DSNs and custom certificate options.
func DescribeConnection(dsn string, source *api.Source, verified string) (api.ConnectionProfile, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return api.ConnectionProfile{}, errors.New("некорректный локальный профиль PostgreSQL")
	}
	ssl := "disable"
	if cfg.TLSConfig != nil {
		ssl = "verify-full"
		if cfg.TLSConfig.InsecureSkipVerify {
			ssl = "require"
			if cfg.TLSConfig.VerifyPeerCertificate != nil || cfg.TLSConfig.VerifyConnection != nil {
				ssl = "verify-ca"
			}
		}
	}
	for _, fallback := range cfg.Fallbacks {
		if fallback.TLSConfig == nil && cfg.TLSConfig != nil {
			ssl = "prefer"
		}
		if fallback.TLSConfig != nil && cfg.TLSConfig == nil {
			ssl = "allow"
		}
	}
	if u, e := url.Parse(dsn); e == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		if value := u.Query().Get("sslmode"); value != "" {
			ssl = value
		}
	}
	return api.ConnectionProfile{Configured: verified != "", Host: cfg.Host, Port: int(cfg.Port), Database: cfg.Database, User: cfg.User, Sslmode: ssl, HasPassword: cfg.Password != "", VerifiedAt: verified, Source: source}, nil
}
func (s Server) connectionProfile(w http.ResponseWriter, r *http.Request) {
	out := api.ConnectionProfile{}
	if s.Connections != nil && s.Connections.Load != nil {
		_, saved, err := s.Connections.Load()
		if err != nil {
			failure(w, err)
			return
		}
		out = saved
	}
	reply(w, 200, out)
}
func (s Server) requestConnectionDSN(req api.ConnectionRequest) (string, error) {
	if req.UseSavedPassword != nil && *req.UseSavedPassword && (req.Password == nil || *req.Password == "") {
		if s.Connections.Load == nil {
			return "", errors.New("нет сохранённого подключения")
		}
		dsn, saved, err := s.Connections.Load()
		if err != nil {
			return "", err
		}
		if !saved.Configured || saved.Host != req.Host || saved.Port != req.Port || saved.Database != req.Database || saved.User != req.User {
			return "", errors.New("сохранённый пароль доступен только для прежнего подключения; введите новый пароль")
		}
		if saved.Sslmode == req.Sslmode {
			return dsn, nil
		}
		cfg, err := pgx.ParseConfig(dsn)
		if err != nil {
			return "", errors.New("некорректный локальный профиль")
		}
		req.Password = &cfg.Password
	}
	return connectionDSN(req)
}

func connectionDSN(v api.ConnectionRequest) (string, error) {
	if v.Host == "" || v.Database == "" || v.User == "" || v.Port < 1 || v.Port > 65535 {
		return "", errors.New("Укажите хост, порт, базу данных и пользователя")
	}
	switch v.Sslmode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return "", errors.New("Некорректный режим SSL")
	}
	password := ""
	if v.Password != nil {
		password = *v.Password
	}
	u := url.URL{Scheme: "postgresql", User: url.UserPassword(v.User, password), Host: net.JoinHostPort(v.Host, strconv.Itoa(v.Port)), Path: "/" + v.Database}
	q := url.Values{"sslmode": {v.Sslmode}}
	if strings.HasPrefix(v.Host, "/") {
		u.Host = ""
		q.Set("host", v.Host)
		q.Set("port", strconv.Itoa(v.Port))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func (p Postgres) Catalog(ctx context.Context) (api.DatabaseCatalog, error) {
	result := api.DatabaseCatalog{Tables: []api.DatabaseTable{}}
	c, err := p.connect(ctx)
	if err != nil {
		return result, err
	}
	defer c.Close(context.Background())
	rows, err := c.Query(ctx, `SELECT n.nspname,c.relname,pk.attname,a.attname FROM pg_class c
 JOIN pg_namespace n ON n.oid=c.relnamespace
 JOIN pg_index i ON i.indrelid=c.oid AND i.indisprimary AND i.indnkeyatts=1
 JOIN pg_attribute pk ON pk.attrelid=c.oid AND pk.attnum=i.indkey[0] AND pk.atttypid IN (20,21,23,25,1043,2950)
 JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped AND a.atttypid IN (25,1043)
 WHERE c.relkind='r' AND n.nspname NOT IN ('pg_catalog','information_schema')
 AND has_table_privilege(c.oid,'SELECT')
 ORDER BY (lower(c.relname) LIKE '%address%') DESC,n.nspname,c.relname,(lower(a.attname) LIKE '%address%') DESC,a.attname`)
	if err != nil {
		return result, errors.New("не удалось прочитать каталог PostgreSQL")
	}
	defer rows.Close()
	for rows.Next() {
		var schema, name, key, column string
		if err = rows.Scan(&schema, &name, &key, &column); err != nil {
			return result, err
		}
		if len(result.Tables) == 0 || result.Tables[len(result.Tables)-1].Schema != schema || result.Tables[len(result.Tables)-1].Name != name {
			result.Tables = append(result.Tables, api.DatabaseTable{Schema: schema, Name: name, Key: key, Columns: []string{}})
		}
		last := &result.Tables[len(result.Tables)-1]
		last.Columns = append(last.Columns, column)
	}
	return result, rows.Err()
}
func (s Server) postgres() Postgres {
	p := s.Postgres
	if s.Connections != nil && s.Connections.DSN != nil {
		p.DSN = s.Connections.DSN()
	}
	return p
}
func (s Server) connection(w http.ResponseWriter, r *http.Request, save bool) {
	if s.Connections == nil {
		failure(w, errors.New("веб-подключение недоступно в этом сервисе"))
		return
	}
	var req api.ConnectionRequest
	if !decode(w, r, &req) {
		return
	}
	dsn, err := s.requestConnectionDSN(req)
	if err != nil {
		failure(w, err)
		return
	}
	p := Postgres{DSN: dsn, APIURL: s.Postgres.APIURL}
	if !save {
		catalog, err := p.Catalog(r.Context())
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, 200, catalog)
		return
	}
	if req.Source == nil {
		failure(w, errors.New("выберите таблицу и колонку"))
		return
	}
	v := req.Source
	kind := "postgres"
	fk := true
	src, err := p.Check(r.Context(), api.Source{Schema: v.Schema, Table: v.Table, Key: v.Key, Column: v.Column, TargetSchema: v.TargetSchema, TargetTable: v.TargetTable, Kind: &kind, ForeignKey: &fk})
	if err == nil {
		err = s.Connections.Save(r.Context(), dsn, src)
	}
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, src)
}

// Read the committed database table, not the editable journal. Cast every value
// to text before JSON encoding to preserve bigint keys/revisions exactly.
func (p Postgres) PublishedTable(ctx context.Context, s *Store, id string, offset int) (api.PublishedTable, error) {
	out := api.PublishedTable{Columns: []string{"run_id", "source_id", "raw_address", "province", "city", "district", "living_area", "street_type", "street_name", "house", "apartment", "entrance", "floor", "landmark", "confidence", "coordinates", "status", "warnings", "reviewed", "revision"}, Rows: [][]string{}}
	run, err := s.Get(ctx, id)
	if err != nil {
		return out, err
	}
	if run.Status != api.Published || IsFile(run.Source) {
		return out, ErrConflict
	}
	identity, err := p.identity()
	if err != nil {
		return out, err
	}
	if run.Source.ConnectionId != nil && *run.Source.ConnectionId != identity {
		return out, errors.New("подключение отличается от источника запуска")
	}
	if !validNames(run.Source) {
		return out, ErrConflict
	}
	c, err := p.connect(ctx)
	if err != nil {
		return out, err
	}
	defer c.Close(context.Background())
	target := qi(run.Source.TargetSchema, run.Source.TargetTable)
	tx, err := c.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.Background())
	if _, err = inspectTarget(ctx, tx, run.Source); err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM "+target+" WHERE run_id=$1", id).Scan(&out.Total); err != nil {
		return out, errors.New("не удалось прочитать итоговую таблицу")
	}
	columns := make([]string, len(out.Columns))
	for i, name := range out.Columns {
		columns[i] = qi(name) + "::text"
	}
	rows, err := tx.Query(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE run_id=$1 ORDER BY source_id LIMIT 50 OFFSET $2", strings.Join(columns, ","), target), id, offset)
	if err != nil {
		return out, errors.New("не удалось прочитать итоговую таблицу")
	}
	defer rows.Close()
	for rows.Next() {
		values := make([]*string, len(columns))
		dest := make([]any, len(columns))
		for i := range dest {
			dest[i] = &values[i]
		}
		if err = rows.Scan(dest...); err != nil {
			return out, err
		}
		row := make([]string, len(columns))
		for i, value := range values {
			row[i] = "NULL"
			if value != nil {
				row[i] = *value
			}
		}
		out.Rows = append(out.Rows, row)
	}
	return out, rows.Err()
}
