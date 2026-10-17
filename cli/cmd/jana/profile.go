package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"example.com/jana-locate-cli/internal/api"
	"example.com/jana-locate-cli/internal/app"
	"github.com/jackc/pgx/v5"
)

type profile struct {
	DatabaseURL string     `json:"database_url"`
	Source      api.Source `json:"source"`
	APIURL      string     `json:"api_url"`
	VerifiedAt  string     `json:"verified_at"`
}

func readProfile(path string) (profile, error) {
	var p profile
	info, err := os.Stat(path)
	if err != nil {
		return p, err
	}
	if info.Mode().Perm()&0077 != 0 {
		return p, errors.New("профиль подключения должен иметь права 0600")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(b, &p); err != nil || p.VerifiedAt == "" {
		return p, errors.New("повреждён профиль; выполните connect заново")
	}
	return p, nil
}
func writeProfile(path string, p profile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".connection-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(p)
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
	return os.Rename(f.Name(), path)
}
func prompt(ctx context.Context, reader *bufio.Reader, label, current string, password bool) (string, error) {
	if interactivePrompts() {
		return textPrompt(ctx, reader, label, current, password)
	}
	if password {
		fmt.Fprint(os.Stderr, label, " [Enter — оставить]: ")
	} else if current != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, current)
	} else {
		fmt.Fprint(os.Stderr, label, ": ")
	}
	value, err := readAnswer(ctx, reader, password)
	if err != nil {
		return "", err
	}
	if !password {
		value = strings.TrimSpace(value)
	}
	if value == "" {
		return current, nil
	}
	return value, nil
}
func connectionURL(host string, port int, database, user, password, ssl string) string {
	u := url.URL{Scheme: "postgresql", User: url.UserPassword(user, password), Path: "/" + database}
	q := url.Values{"sslmode": {ssl}}
	if strings.HasPrefix(host, "/") {
		q.Set("host", host)
		q.Set("port", strconv.Itoa(port))
	} else {
		u.Host = net.JoinHostPort(host, strconv.Itoa(port))
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func connect(ctx context.Context, o *flowOptions, existing profile, reader *bufio.Reader) (profile, error) {
	showConnection(existing)
	dsn, err := secret("JANA_DATABASE_URL")
	if err != nil {
		return profile{}, err
	}
	if existing.DatabaseURL != "" {
		dsn = existing.DatabaseURL
	}
	if o.databaseURL != "" {
		dsn = o.databaseURL
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return profile{}, errors.New("некорректные параметры PostgreSQL")
	}
	host, port, db, user, password := cfg.Host, int(cfg.Port), cfg.Database, cfg.User, cfg.Password
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
	originalSSL := ssl
	for key, ref := range map[string]*string{"host": &host, "database": &db, "user": &user, "password": &password, "sslmode": &ssl} {
		if o.visited[key] {
			*ref = map[string]string{"host": o.host, "database": o.database, "user": o.user, "password": o.password, "sslmode": o.sslmode}[key]
		}
	}
	if o.visited["port"] {
		port = o.port
	}
	if o.passwordStdin {
		v, err := readAnswer(ctx, reader, false)
		if err != nil && len(v) == 0 {
			return profile{}, errors.New("нет пароля в stdin")
		}
		password = strings.TrimSuffix(strings.TrimSuffix(v, "\n"), "\r")
	}
	src := existing.Source
	if o.visited["table"] && o.table != src.Table && !o.visited["target-table"] {
		src.TargetTable = ""
	}
	for name, ref := range map[string]*string{"schema": &src.Schema, "table": &src.Table, "key": &src.Key, "column": &src.Column, "target-schema": &src.TargetSchema, "target-table": &src.TargetTable} {
		value := map[string]string{"schema": o.schema, "table": o.table, "key": o.key, "column": o.column, "target-schema": o.targetSchema, "target-table": o.targetTable}[name]
		if o.visited[name] || *ref == "" {
			*ref = value
		}
	}
	if src.Schema == "" {
		src.Schema = "public"
	}
	if src.Key == "" {
		src.Key = "id"
	}
	if src.Column == "" {
		src.Column = "address"
	}
	if src.TargetSchema == "" {
		src.TargetSchema = "public"
	}
	wizard := !o.noInteractive && !o.visited["table"] && !o.passwordStdin
	if wizard {
		fmt.Fprintln(os.Stderr, "Подключение PostgreSQL · параметры и пароль сохраняются локально с правами 0600")
		for _, question := range []struct {
			label    string
			value    *string
			password bool
		}{{"Хост", &host, false}, {"База данных", &db, false}, {"Пользователь", &user, false}, {"Пароль", &password, true}, {"Режим SSL", &ssl, false}} {
			// Port belongs immediately after host in the wizard.
			var v string
			var err error
			if question.label == "Режим SSL" && interactivePrompts() {
				modes, labels := sslChoices()
				selected := labels[0]
				for i, mode := range modes {
					if mode == *question.value {
						selected = labels[i]
					}
				}
				index, e := choose(ctx, reader, question.label, labels, selected)
				err = e
				if e == nil {
					v = modes[index]
				}
			} else {
				v, err = prompt(ctx, reader, question.label, *question.value, question.password)
			}
			if err != nil {
				return profile{}, err
			}
			*question.value = v
			if question.label == "Хост" {
				v, err = prompt(ctx, reader, "Порт", strconv.Itoa(port), false)
				if err != nil {
					return profile{}, err
				}
				port, err = strconv.Atoi(v)
				if err != nil {
					return profile{}, errors.New("порт должен быть числом")
				}
			}
		}
	}
	if port < 1 || port > 65535 || host == "" || db == "" || user == "" {
		return profile{}, errors.New("Укажите корректные хост, порт, базу данных и пользователя")
	}
	switch ssl {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return profile{}, errors.New("Некорректный режим SSL")
	}
	if host != cfg.Host || port != int(cfg.Port) || db != cfg.Database || user != cfg.User || password != cfg.Password || ssl != originalSSL || dsn == "" || o.passwordStdin || o.visited["host"] || o.visited["port"] || o.visited["database"] || o.visited["user"] || o.visited["password"] || o.visited["sslmode"] {
		dsn = connectionURL(host, port, db, user, password, ssl)
	}
	if wizard {
		catalog, err := (app.Postgres{DSN: dsn}).Catalog(ctx)
		if err != nil {
			return profile{}, err
		}
		fmt.Fprintln(os.Stderr, "✓ Подключение PostgreSQL установлено.")
		names := make([]string, len(catalog.Tables))
		for i, table := range catalog.Tables {
			names[i] = table.Schema + "." + table.Name
		}
		index, err := choose(ctx, reader, "Таблица", names, src.Schema+"."+src.Table)
		if err != nil {
			return profile{}, err
		}
		table := catalog.Tables[index]
		src.Schema, src.Table, src.Key = table.Schema, table.Name, table.Key
		index, err = choose(ctx, reader, "Колонка адреса", table.Columns, src.Column)
		if err != nil {
			return profile{}, err
		}
		src.Column = table.Columns[index]
		fmt.Fprintf(os.Stderr, "Первичный ключ: %s\n", clean(src.Key))
		if src.TargetSchema, err = prompt(ctx, reader, "Схема результата", src.TargetSchema, false); err != nil {
			return profile{}, err
		}
	}
	if src.Table != existing.Source.Table && !o.visited["target-table"] {
		src.TargetTable = ""
	}
	if src.TargetTable == "" {
		src.TargetTable = src.Table + "_jana_addresses"
	}
	if wizard {
		if src.TargetTable, err = prompt(ctx, reader, "Таблица результата", src.TargetTable, false); err != nil {
			return profile{}, err
		}
	}
	kind := "postgres"
	src.Kind = &kind
	fk := !o.noForeignKey
	src.ForeignKey = &fk
	p := app.Postgres{DSN: dsn, APIURL: o.apiURL}
	src, err = p.Check(ctx, src)
	if err != nil {
		return profile{}, err
	}
	result := profile{DatabaseURL: dsn, Source: src, APIURL: o.apiURL, VerifiedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err = writeProfile(o.profilePath, result); err != nil {
		return profile{}, errors.New("не удалось сохранить локальный профиль")
	}
	fmt.Fprintf(os.Stderr, "Подключение проверено: %s:%d / %s · %s.%s.%s\nПрофиль: %s\n", host, port, db, src.Schema, src.Table, src.Column, o.profilePath)
	return result, nil
}

func showConnection(saved profile) {
	if saved.VerifiedAt == "" {
		return
	}
	defaults, err := app.DescribeConnection(saved.DatabaseURL, &saved.Source, saved.VerifiedAt)
	if err != nil {
		return
	}
	fmt.Fprintf(os.Stderr, "\nСохранённое подключение: %s@%s:%d / %s\nИсточник: %s.%s.%s → %s.%s\nПоследняя проверка: %s. Параметры ниже — сохранённые значения по умолчанию.\n", clean(defaults.User), clean(defaults.Host), defaults.Port, clean(defaults.Database), clean(saved.Source.Schema), clean(saved.Source.Table), clean(saved.Source.Column), clean(saved.Source.TargetSchema), clean(saved.Source.TargetTable), clean(saved.VerifiedAt))
}
