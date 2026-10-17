package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"example.com/jana-locate-cli/internal/api"
	"example.com/jana-locate-cli/internal/app"
)

type flowOptions struct {
	command                                                                      string
	pageSize                                                                     int
	newRun, plain, verbose, noColor, readyOnly, readyOnlyView                    bool
	search, sort                                                                 string
	state, profilePath, apiURL, listen, runID, file, format, output              string
	host, database, user, password, sslmode, databaseURL                         string
	port                                                                         int
	schema, table, key, column, targetSchema, targetTable                        string
	noInteractive, quiet, web, open, confirm, force, passwordStdin, noForeignKey bool
	visited                                                                      map[string]bool
}

func parseFlow(command string, args []string) (flowOptions, error) {
	o := flowOptions{command: command}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	f.StringVar(&o.search, "search", "", "поиск по исходному адресу и региону")
	f.StringVar(&o.sort, "sort", "position", "сортировка: position, status, region, risk")
	f.BoolVar(&o.readyOnlyView, "review-only", false, "показать только требующие проверки")
	f.BoolVar(&o.newRun, "new", false, "создать новый снимок вместо продолжения сохранённого запуска")
	f.IntVar(&o.pageSize, "page-size", 20, "записей на странице (1–100)")
	f.BoolVar(&o.plain, "plain", false, "построчный режим вместо полноэкранного")
	f.BoolVar(&o.verbose, "verbose", false, "полные идентификаторы и диагностика")
	f.BoolVar(&o.noColor, "no-color", false, "отключить цвета")
	f.BoolVar(&o.readyOnly, "ready-only", true, "публиковать только автоматические и проверенные оператором записи")
	f.StringVar(&o.state, "state", env("JANA_STATE_PATH", "var/workspace.sqlite"), "локальный журнал")
	f.StringVar(&o.profilePath, "profile", "", "профиль подключения (по умолчанию рядом с журналом)")
	f.StringVar(&o.apiURL, "api-url", env("JANA_API_URL", "http://localhost:8080"), "URL нормализатора")
	f.StringVar(&o.listen, "listen", env("JANA_LISTEN", "127.0.0.1:8095"), "адрес локального веб-интерфейса")
	f.StringVar(&o.runID, "run", "", "сохранённый запуск; normalize продолжает его")
	f.StringVar(&o.file, "file", "", "JSON или построчный UTF-8 файл, без connect")
	f.StringVar(&o.format, "format", "auto", "auto, json или lines")
	f.StringVar(&o.output, "output", "", "выходной JSON для публикации")
	f.StringVar(&o.host, "host", "", "хост PostgreSQL")
	f.IntVar(&o.port, "port", 5432, "порт PostgreSQL")
	f.StringVar(&o.database, "database", "", "база PostgreSQL")
	f.StringVar(&o.user, "user", "", "пользователь PostgreSQL")
	f.StringVar(&o.password, "password", "", "пароль (также через переменные окружения или --password-stdin)")
	f.StringVar(&o.sslmode, "sslmode", "prefer", "режим SSL PostgreSQL")
	f.StringVar(&o.databaseURL, "database-url", "", "локальный DSN PostgreSQL")
	f.StringVar(&o.schema, "schema", "public", "схема источника")
	f.StringVar(&o.table, "table", "", "таблица источника")
	f.StringVar(&o.key, "key", "id", "одиночный первичный ключ")
	f.StringVar(&o.column, "column", "address", "колонка адреса")
	f.StringVar(&o.targetSchema, "target-schema", "public", "схема результата")
	f.StringVar(&o.targetTable, "target-table", "", "таблица результата")
	f.BoolVar(&o.noInteractive, "no-interactive", false, "не запрашивать ввод; normalize завершается после обработки")
	f.BoolVar(&o.quiet, "quiet", false, "только итоговый JSON в stdout")
	webHelp := "открыть сайт проверки"
	if command == "connect" {
		webHelp = "подключиться через браузер"
	}
	f.BoolVar(&o.web, "web", false, webHelp)
	f.BoolVar(&o.web, "w", false, webHelp)
	f.BoolVar(&o.open, "open", false, "открыть сайт проверки")
	f.BoolVar(&o.confirm, "confirm", false, "после обработки явно опубликовать готовые записи без stdin")
	f.BoolVar(&o.force, "force", false, "разрешить замену выходного файла")
	f.BoolVar(&o.passwordStdin, "password-stdin", false, "прочитать пароль одной строкой stdin")
	f.BoolVar(&o.noForeignKey, "no-foreign-key", false, "использовать JOIN вместо физического FK")
	if err := parseFlags(f, args); err != nil {
		return o, err
	}
	if f.NArg() != 0 {
		return o, errors.New("лишние аргументы; используйте флаги команды")
	}
	if o.sort != "position" && o.sort != "status" && o.sort != "region" && o.sort != "risk" {
		return o, errors.New("--sort: position, status, region или risk")
	}
	if o.pageSize < 1 || o.pageSize > 100 {
		return o, errors.New("--page-size должен быть от 1 до 100")
	}
	if o.profilePath == "" {
		o.profilePath = filepath.Join(filepath.Dir(o.state), "connection.json")
	}
	if command != "connect" && o.web {
		o.open = true
	}
	o.visited = map[string]bool{}
	f.Visit(func(v *flag.Flag) { o.visited[v.Name] = true })
	return o, nil
}
func flow(command string, args []string) error {
	o, err := parseFlow(command, args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	canonical, err := app.NewNormalizer(o.apiURL, "")
	if err != nil {
		return err
	}
	o.apiURL = canonical.URL
	reader := bufio.NewReader(os.Stdin)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	saved, profileErr := readProfile(o.profilePath)
	if profileErr != nil && !errors.Is(profileErr, os.ErrNotExist) && (command == "normalize" && o.file == "" && o.runID == "") {
		return profileErr
	}
	if profileErr != nil {
		saved = profile{}
	}
	if !o.visited["api-url"] && os.Getenv("JANA_API_URL") == "" && saved.APIURL != "" {
		o.apiURL = saved.APIURL
	}
	if command == "connect" {
		_, err = app.NewNormalizer(o.apiURL, "")
		if err != nil {
			return err
		}
		if o.web {
			return connectWeb(ctx, o, saved)
		}
		_, err = connect(ctx, &o, saved, reader)
		return err
	}
	if o.newRun && o.runID != "" {
		return errors.New("--new и --run нельзя использовать вместе")
	}
	if o.runID != "" && o.file != "" {
		return errors.New("--run и --file нельзя использовать вместе")
	}
	store, err := app.Open(o.state)
	if err != nil {
		return err
	}
	defer store.DB.Close()
	dsn := ""
	localDSN := func() (string, error) {
		if o.databaseURL != "" {
			return o.databaseURL, nil
		}
		if saved.DatabaseURL != "" {
			return saved.DatabaseURL, nil
		}
		return secret("JANA_DATABASE_URL")
	}
	var current api.Run
	if command == "normalize" && o.runID == "" && o.file == "" && !o.newRun && profileErr == nil {
		changed := false
		for _, name := range []string{"schema", "table", "key", "column", "target-schema", "target-table", "host", "port", "database", "user", "password", "sslmode", "database-url", "password-stdin", "no-foreign-key"} {
			changed = changed || o.visited[name]
		}
		if !changed {
			runs, listErr := store.List(ctx)
			if listErr != nil {
				return listErr
			}
			o.runID = partialRun(runs, saved.Source, o.apiURL)
			if o.runID != "" && !o.quiet {
				fmt.Fprintln(os.Stderr, "Продолжаем сохранённый запуск. Для нового снимка: normalize --new.")
			}
		}
	}
	if command == "normalize" {
		if o.runID != "" {
			current, err = store.Get(ctx, o.runID)
		} else if o.file != "" {
			current, err = store.ImportFile(ctx, o.file, o.format, o.apiURL)
		} else {
			changed := false
			for _, name := range []string{"schema", "table", "key", "column", "target-schema", "target-table", "host", "port", "database", "user", "password", "sslmode", "database-url", "password-stdin", "no-foreign-key"} {
				changed = changed || o.visited[name]
			}
			if profileErr != nil || changed {
				if o.noInteractive && o.table == "" && saved.Source.Table == "" {
					return errors.New("сначала выполните jana connect или укажите --file")
				}
				saved, err = connect(ctx, &o, saved, reader)
				if err != nil {
					return err
				}
				dsn = saved.DatabaseURL
			}
			dsn, err = localDSN()
			if err != nil {
				return err
			}
			current, err = (app.Postgres{DSN: dsn, APIURL: o.apiURL}).Import(ctx, store, saved.Source)
		}
		if err != nil {
			return err
		}
		if !o.visited["api-url"] && os.Getenv("JANA_API_URL") == "" {
			o.apiURL = current.ApiUrl
		}
		if o.apiURL != current.ApiUrl {
			return errors.New("URL API отличается от сохранённого запуска")
		}
		if current.Status == api.Failed {
			current, err = store.Status(ctx, current.Id, current.Revision, api.Normalizing, "")
			if err != nil {
				return err
			}
		}
	} else {
		if o.runID == "" {
			runs, err := store.List(ctx)
			if err != nil {
				return err
			}
			if len(runs) == 0 {
				return errors.New("нет запусков; сначала выполните normalize")
			}
			for _, run := range runs {
				if o.visited["target-table"] && run.Source.TargetTable != o.targetTable {
					continue
				}
				if o.visited["target-schema"] && run.Source.TargetSchema != o.targetSchema {
					continue
				}
				o.runID = run.Id
				break
			}
			if o.runID == "" {
				return errors.New("Нет сохранённого запуска для этой таблицы вывода; укажите --run")
			}

		}
		current, err = store.Get(ctx, o.runID)
		if err != nil {
			return err
		}
		if o.visited["target-table"] && current.Source.TargetTable != o.targetTable {
			return errors.New("Таблица вывода не совпадает с выбранным --run")
		}
		if o.visited["target-schema"] && current.Source.TargetSchema != o.targetSchema {
			return errors.New("Схема вывода не совпадает с выбранным --run")
		}
		o.apiURL = current.ApiUrl
	}
	token, err := secret("JANA_API_TOKEN")
	if err != nil {
		return err
	}
	n, err := app.NewNormalizer(o.apiURL, token)
	if err != nil {
		return err
	}
	if !app.IsFile(current.Source) {
		dsn, err = localDSN()
		if err != nil {
			return err
		}
	}
	p := app.Postgres{DSN: dsn, APIURL: o.apiURL}
	if command == "normalize" {
		web, err := startWeb(ctx, store, n, p, o.state, o.listen, o.profilePath)
		if err != nil {
			return err
		}
		defer web.Close()
		if o.open {
			openSite(web.URL + "?run=" + current.Id)
		}
		didPublish := false
		var writtenSummary api.RunSummary
		if !o.noInteractive && !o.plain && !o.confirm && interactivePrompts() {
			write := func(run api.Run, progress func(int, int)) (api.Run, error) {
				summary, err := store.Summary(ctx, run.Id)
				if err != nil {
					return run, err
				}
				options := app.WriteOptions{ReadyOnly: true, Progress: progress}
				var published api.Run
				if app.IsFile(run.Source) {
					published, err = store.ExportFileWithOptions(ctx, run.Id, publicationFile(run, o), run.Revision, o.force, options)
				} else {
					published, err = p.PublishWithOptions(ctx, store, run.Id, run.Revision, options)
				}
				if err == nil {
					didPublish = true
					writtenSummary = summary
				}
				return published, err
			}
			current, _, err = runWorkspaceUI(ctx, store, current, o, web.URL, func() error { openSite(web.URL + "?run=" + current.Id); return nil }, write, n)
		} else {
			plain := o
			plain.noInteractive = true
			err = watchNormalization(ctx, store, current.Id, plain)
			if err == nil && o.confirm {
				current, err = store.Get(ctx, current.Id)
				if err == nil && current.Status != api.Published {
					o.file = "" // publication uses the persisted snapshot
					o.readyOnly = true
					return land(ctx, store, p, n, current, o, reader)
				}
			}
		}
		if err != nil {
			return err
		}
		current, err = store.Get(context.Background(), current.Id)
		if err != nil {
			return err
		}
		if didPublish {
			publicationSummary(current, writtenSummary, o)
		}
		if !o.quiet {
			fmt.Fprintf(os.Stderr, "\nЗапуск %s сохранён. Продолжить: jana normalize --state %s --run %s\nСайт: %s?run=%s\n", current.Id, o.state, current.Id, web.URL, current.Id)
		}
		return json.NewEncoder(os.Stdout).Encode(current)
	}
	return land(ctx, store, p, n, current, o, reader)
}
func preview(ctx context.Context, s *app.Store, run api.Run, offset int, o flowOptions) error {
	id := run.Id
	if !o.verbose {
		id = id[:min(8, len(id))]
	}
	command := o.command
	if command == "" || command == "land" {
		command = "publish"
	}
	fmt.Fprintf(os.Stderr, "\njana %s · запуск %s · %s\n", command, id, runStatus(run.Status))
	fmt.Fprintf(os.Stderr, "Обработано %d/%d · проверено оператором %d\n", run.Processed, run.Total, run.Reviewed)
	if app.IsFile(run.Source) {
		fmt.Fprintf(os.Stderr, "Файл: %s\n", clean(run.Source.Table))
	} else {
		fmt.Fprintf(os.Stderr, "%s.%s → %s.%s\n", clean(run.Source.Schema), clean(run.Source.Table), clean(run.Source.TargetSchema), clean(run.Source.TargetTable))
	}
	summary, err := s.Summary(ctx, run.Id)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Автоматически проверено: %d · Проверено оператором: %d · Требует проверки: %d\n", summary.Automatic, summary.Reviewed, summary.NeedsReview)
	if summary.NeedsReview > 0 {
		if o.readyOnly {
			fmt.Fprintf(os.Stderr, "ВНИМАНИЕ: %d неподтверждённых записей будут пропущены. Правки — в normalize.\n", summary.NeedsReview)
		} else {
			fmt.Fprintf(os.Stderr, "ВНИМАНИЕ: %d записей будут записаны с текущими статусами и предупреждениями; --ready-only пропускает их.\n", summary.NeedsReview)
		}
	}
	if timing := timingText(run); timing != "" {
		fmt.Fprintln(os.Stderr, timing)
	}
	rows, total, err := s.Browse(ctx, run.Id, app.BrowseOptions{Offset: offset, Limit: o.pageSize, ReviewOnly: o.readyOnlyView, Query: o.search, Sort: o.sort})
	if err != nil {
		return err
	}
	for _, row := range rows {
		fmt.Fprintf(os.Stderr, "  %-8s %-22s %4s %s\n", fmt.Sprintf("#%d", row.Position+1), "["+rowStatus(row)+"]", rowConfidence(row), cellClip(rawAddress(row), 65))
		normalized := normalizedAddress(row.Result)
		delta := addressChanges(rawAddress(row), normalized)
		if normalized == "" {
			delta = "Компоненты адреса не определены"
		}
		fmt.Fprintln(os.Stderr, "           → "+cellClip(delta, 95))
		if !row.Reviewed && row.Result.Status != api.Matched && len(row.Result.Warnings) > 0 {
			fmt.Fprintln(os.Stderr, "             "+cellClip(reason(row.Result.Warnings[0]), 95))
		}
	}
	fmt.Fprintf(os.Stderr, "Страница %d/%d · N/P — следующая/предыдущая (с Enter)\n", offset/o.pageSize+1, max(1, (total+o.pageSize-1)/o.pageSize))
	return nil
}
func land(ctx context.Context, s *app.Store, p app.Postgres, n *app.Normalizer, current api.Run, o flowOptions, reader *bufio.Reader) error {
	if current.Status != api.Review && current.Status != api.Published {
		return errors.New("нормализация не завершена; выполните normalize --run " + current.Id)
	}
	var web *webSession
	defer func() { web.Close() }()
	ensureWeb := func() error {
		if web == nil {
			var err error
			web, err = startWeb(ctx, s, n, p, o.state, o.listen, o.profilePath)
			if err != nil {
				return err
			}
		}
		return nil
	}
	open := func() error {
		if err := ensureWeb(); err != nil {
			return err
		}
		openSite(web.URL + "?run=" + current.Id)
		return nil
	}
	if o.pageSize == 0 {
		o.pageSize = 20
	}
	if current.PublishedCount != nil && *current.PublishedCount < current.Total {
		o.readyOnlyView = true
	}
	offset := 0
	confirmed := o.confirm
	interactive := !o.noInteractive && !o.confirm && !o.plain && o.readyOnly && interactivePrompts()
	if interactive {
		if err := ensureWeb(); err != nil {
			return err
		}
		if o.open {
			if err := open(); err != nil {
				return err
			}
		}
		var err error
		current, confirmed, err = runPublishUI(ctx, s, current, o, web.URL, open)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Веб-проверка:", web.URL+"?run="+current.Id)
		if !confirmed {
			fmt.Fprintln(os.Stderr, "Публикация отменена. Подтверждение и правки: jana normalize --run "+current.Id)
			return nil
		}
		o.readyOnly = true
	} else {
		if err := preview(ctx, s, current, offset, o); err != nil {
			return err
		}
		if o.open {
			if err := open(); err != nil {
				return err
			}
		}
		if !o.open && !o.noInteractive {
			if err := ensureWeb(); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Веб-проверка:", web.URL+"?run="+current.Id)
		}
		if o.noInteractive && !o.confirm {
			return nil
		}
	}
	for !confirmed {
		fmt.Fprint(os.Stderr, "«подтвердить» — записать; N/P — страницы; W — браузер; Enter/«отмена» — выйти: ")
		value, err := readAnswer(ctx, reader, false)
		if err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "подтвердить", "confirm":
			latest, err := s.Get(ctx, current.Id)
			if err != nil {
				return err
			}
			if latest.Revision != current.Revision {
				current = latest
				if err = preview(ctx, s, current, offset, o); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "Правки изменились. Подтвердите обновлённый предпросмотр ещё раз.")
				continue
			}
			confirmed = true
		case "n", "p":
			_, total, browseErr := s.Browse(ctx, current.Id, app.BrowseOptions{Limit: 1, ReviewOnly: o.readyOnlyView, Query: o.search, Sort: o.sort})
			if browseErr != nil {
				return browseErr
			}
			if strings.EqualFold(strings.TrimSpace(value), "n") {
				offset = min(offset+o.pageSize, max(0, (total-1)/o.pageSize*o.pageSize))
			} else {
				offset = max(0, offset-o.pageSize)
			}
			current, err = s.Get(ctx, current.Id)
			if err != nil {
				return err
			}
			if err = preview(ctx, s, current, offset, o); err != nil {
				return err
			}
		case "w":
			if err := open(); err != nil {
				return err
			}
		case "", "отмена", "cancel", "q":
			return nil
		default:
			fmt.Fprintln(os.Stderr, "Введите «подтвердить», N, P, W или «отмена».")
		}
	}
	summary, err := s.Summary(ctx, current.Id)
	if err != nil {
		return err
	}
	progress := publicationProgress(o.quiet)
	writeOptions := app.WriteOptions{ReadyOnly: o.readyOnly, Progress: progress}
	if app.IsFile(current.Source) {
		if o.file != "" {
			return errors.New("publish использует сохранённый запуск, а не --file")
		}
		published, err := s.ExportFileWithOptions(ctx, current.Id, publicationFile(current, o), current.Revision, o.force, writeOptions)
		if err != nil {
			if progress != nil {
				fmt.Fprintln(os.Stderr)
			}
			return err
		}
		publicationSummary(published, summary, o)
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"run": published.Id, "output": publicationFile(current, o), "status": "exported", "published_count": published.PublishedCount})
	}
	published, err := p.PublishWithOptions(ctx, s, current.Id, current.Revision, writeOptions)
	if err != nil {
		if progress != nil {
			fmt.Fprintln(os.Stderr)
		}
		return err
	}
	publicationSummary(published, summary, o)
	return json.NewEncoder(os.Stdout).Encode(published)
}
