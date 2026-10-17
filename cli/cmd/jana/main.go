package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"example.com/jana-locate-cli/internal/api"
	"example.com/jana-locate-cli/internal/app"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func secret(key string) (string, error) {
	if file := os.Getenv(key + "_FILE"); file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("Не удалось прочитать файл, указанный в %s_FILE", key)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return os.Getenv(key), nil
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "jana:", app.UserMessage(err))
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Println("Jana CLI: connect → normalize (a — публикация) | serve | runs | export\nПодключение PostgreSQL: локальные JANA_DATABASE_URL[_FILE] или PGHOST/PGDATABASE/PGUSER/PGPASSWORD.\nСправка по команде: jana <команда> --help.")
		return nil
	}
	command := args[0]
	if command == "connect" || command == "normalize" || command == "land" || command == "publish" {
		return flow(command, args[1:])
	}
	if command == "healthcheck" {
		c := http.Client{Timeout: 2 * time.Second}
		_, port, err := net.SplitHostPort(env("JANA_LISTEN", "127.0.0.1:8095"))
		if err != nil {
			return errors.New("Некорректный адрес проверки готовности сервиса")
		}
		r, err := c.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz")
		if err != nil {
			return errors.New("Сервис не готов к работе")
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			return errors.New("Сервис не готов к работе")
		}
		return nil
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	state := fs.String("state", env("JANA_STATE_PATH", "var/workspace.sqlite"), "локальный журнал с сохранением прогресса")
	apiURL := fs.String("api-url", env("JANA_API_URL", "http://localhost:8080"), "адрес API нормализации")
	listen := fs.String("listen", env("JANA_LISTEN", "127.0.0.1:8095"), "адрес сайта проверки")
	openBrowser := fs.Bool("open", false, "открыть сайт проверки в браузере")
	runID := fs.String("run", "", "идентификатор сохранённого запуска")
	schema := fs.String("schema", "public", "схема источника")
	table := fs.String("table", "", "таблица источника")
	key := fs.String("key", "id", "одиночный первичный ключ")
	column := fs.String("column", "address", "колонка адреса")
	targetSchema := fs.String("target-schema", "public", "схема результата")
	targetTable := fs.String("target-table", "", "таблица результата (по умолчанию: <источник>_jana_addresses)")
	if err := parseFlags(fs, args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("Лишние позиционные аргументы; используйте флаги команды")
	}
	switch command {
	case "serve", "postgres", "import", "publish", "runs", "export":
	default:
		return errors.New("Неизвестная команда; справка: jana help")
	}
	saved, _ := readProfile(filepath.Join(filepath.Dir(*state), "connection.json"))
	explicitAPI := false
	fs.Visit(func(f *flag.Flag) { explicitAPI = explicitAPI || f.Name == "api-url" })
	if !explicitAPI && os.Getenv("JANA_API_URL") == "" && saved.APIURL != "" {
		*apiURL = saved.APIURL
	}
	token, err := secret("JANA_API_TOKEN")
	if err != nil {
		return err
	}
	normalizer, err := app.NewNormalizer(*apiURL, token)
	if err != nil {
		return err
	}
	dsn, err := secret("JANA_DATABASE_URL")
	if err != nil {
		return err
	}
	if saved.DatabaseURL != "" {
		dsn = saved.DatabaseURL
	}
	postgres := app.Postgres{DSN: dsn, APIURL: normalizer.URL}
	store, err := app.Open(*state)
	if err != nil {
		return errors.New("Не удалось открыть локальный журнал")
	}
	defer store.DB.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	printJSON := func(v any) error { return json.NewEncoder(os.Stdout).Encode(v) }
	if command == "runs" {
		runs, err := store.List(ctx)
		if err != nil {
			return err
		}
		return printJSON(api.RunList{Runs: runs})
	}
	if command == "export" {
		r, err := store.Get(ctx, *runID)
		if err != nil {
			return err
		}
		if r.Status != api.Review && r.Status != api.Published {
			return app.ErrConflict
		}
		rows, err := store.Rows(ctx, *runID, 0, 100000, false, false)
		if err != nil {
			return err
		}
		return printJSON(api.Rows{Rows: rows, Total: len(rows)})
	}
	if command == "import" || command == "postgres" {
		if *targetTable == "" {
			*targetTable = *table + "_jana_addresses"
		}
		src := api.Source{Schema: *schema, Table: *table, Key: *key, Column: *column, TargetSchema: *targetSchema, TargetTable: *targetTable}
		r, err := postgres.Import(ctx, store, src)
		if err != nil {
			return err
		}
		*runID = r.Id
		if command == "import" {
			return printJSON(r)
		}
	}
	web, err := startWeb(ctx, store, normalizer, postgres, *state, *listen)
	if err != nil {
		return err
	}
	defer web.Close()
	site := web.URL
	if *runID != "" {
		site += "?run=" + *runID
	}
	fmt.Fprintln(os.Stderr, "Веб-проверка Jana:", site)
	if *openBrowser {
		openSite(site)
	}
	if web.Owned {
		<-ctx.Done()
	}
	return nil
}
