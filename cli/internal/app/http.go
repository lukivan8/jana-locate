package app

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	contract "example.com/jana-locate-cli/api"
	"example.com/jana-locate-cli/internal/api"
	"example.com/jana-locate-cli/web"
)

type Server struct {
	Store               *Store
	Normalizer          *Normalizer
	Connections         *Connections
	Postgres            Postgres
	Token, PublicOrigin string
	Network             bool
	AuthDisabled        bool
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, err error) {
	code, status := "operation_failed", 500
	message := UserMessage(err)
	if errors.Is(err, ErrNotFound) {
		code, status, message = "not_found", 404, "Запуск или запись не найдены."
	}
	if errors.Is(err, ErrConflict) {
		code, status, message = "revision_conflict", 409, "Версия изменилась или действие сейчас недоступно. Загрузите сохранённую версию."
	}
	if errors.Is(err, ErrSourceChanged) {
		code, status, message = "source_changed", 409, "Исходные адреса изменились или удалены. Публикация отменена; создайте новый снимок."
	}
	reply(w, status, api.Error{Code: code, Message: message})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		reply(w, 400, api.Error{Code: "invalid_request", Message: "Требуется application/json."})
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	d.UseNumber()
	err = d.Decode(v)
	if err == nil {
		if d.Decode(new(any)) != io.EOF {
			err = errors.New("Ожидается один JSON-документ")
		}
	}
	if err != nil {
		reply(w, 400, api.Error{Code: "invalid_request", Message: "Некорректный запрос или превышен размер 2 MiB."})
		return false
	}
	return true
}
func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerCitySelections(mux)
	mux.HandleFunc("GET /v1/connection", s.connectionProfile)
	mux.HandleFunc("POST /v1/connection/catalog", func(w http.ResponseWriter, r *http.Request) { s.connection(w, r, false) })
	mux.HandleFunc("POST /v1/connection/save", func(w http.ResponseWriter, r *http.Request) { s.connection(w, r, true) })
	mux.HandleFunc("GET /v1/runs/{id}/summary", func(w http.ResponseWriter, r *http.Request) {
		summary, err := s.Store.Summary(r.Context(), r.PathValue("id"))
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, 200, summary)
	})
	mux.HandleFunc("GET /v1/runs/{id}/table", func(w http.ResponseWriter, r *http.Request) {
		offset := 0
		var err error
		if v := r.URL.Query().Get("offset"); v != "" {
			offset, err = strconv.Atoi(v)
		}
		if err != nil || offset < 0 {
			reply(w, 400, api.Error{Code: "invalid_offset", Message: "Некорректная страница"})
			return
		}
		table, err := s.postgres().PublishedTable(r.Context(), s.Store, r.PathValue("id"), offset)
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, 200, table)
	})
	mux.HandleFunc("GET /v1/events", s.events)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(contract.OpenAPI)
	})
	mux.HandleFunc("GET /v1/runs", func(w http.ResponseWriter, r *http.Request) {
		runs, err := s.Store.List(r.Context())
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, 200, api.RunList{Runs: runs})
	})
	mux.HandleFunc("POST /v1/runs/input", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Records []struct {
				Id         any     `json:"id"`
				RawAddress *string `json:"raw_address"`
			} `json:"records"`
		}
		if !decode(w, r, &req) {
			return
		}
		if len(req.Records) < 1 || len(req.Records) > 1000 {
			reply(w, 400, api.Error{Code: "invalid_input", Message: "Введите от 1 до 1 000 адресов."})
			return
		}
		records := make([]api.InputRecord, len(req.Records))
		for i, record := range req.Records {
			validID := false
			switch id := record.Id.(type) {
			case string:
				validID = true
			case json.Number:
				_, err := id.Int64()
				validID = err == nil
			}
			if !validID || record.RawAddress == nil || len(*record.RawAddress) > 4096 {
				reply(w, 400, api.Error{Code: "invalid_input", Message: "Каждой записи нужны id (строка или int64) и raw_address (строка до 4096 байт)."})
				return
			}
			records[i] = api.InputRecord{Id: record.Id, RawAddress: *record.RawAddress}
		}
		run, err := s.Store.ImportRecords(r.Context(), records, s.Postgres.APIURL)
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, 200, run)
	})
	mux.HandleFunc("POST /v1/runs", func(w http.ResponseWriter, r *http.Request) {
		var req api.ImportRequest
		if !decode(w, r, &req) {
			return
		}
		src := api.Source{Schema: req.Schema, Table: req.Table, Key: req.Key, Column: req.Column, TargetSchema: req.TargetSchema, TargetTable: req.TargetTable}
		if !validNames(src) {
			reply(w, 400, api.Error{Code: "invalid_source", Message: "Укажите схему, таблицу, ключ, колонку и отдельную таблицу результата (имена до 63 байт)."})
			return
		}
		run, err := s.postgres().Import(r.Context(), s.Store, src)
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, 200, run)
	})
	mux.HandleFunc("GET /v1/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		run, err := s.Store.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, 200, run)
	})
	mux.HandleFunc("GET /v1/runs/{id}/rows", func(w http.ResponseWriter, r *http.Request) {
		offset := 0
		if v := r.URL.Query().Get("offset"); v != "" {
			var err error
			offset, err = strconv.Atoi(v)
			if err != nil || offset < 0 {
				reply(w, 400, api.Error{Code: "invalid_offset", Message: "Некорректная позиция страницы."})
				return
			}
		}
		rows, err := s.Store.Rows(r.Context(), r.PathValue("id"), offset, 50, r.URL.Query().Get("review_only") == "true", false)
		if err != nil {
			failure(w, err)
			return
		}
		run, err := s.Store.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, 200, api.Rows{Rows: rows, Total: run.Total})
	})
	mux.HandleFunc("POST /v1/runs/{id}/rows/{position}/review", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ExpectedRevision *int               `json:"expected_revision"`
			Fields           *api.AddressFields `json:"fields"`
		}
		if !decode(w, r, &req) {
			return
		}
		position, err := strconv.Atoi(r.PathValue("position"))
		if err != nil || position < 0 || req.ExpectedRevision == nil || *req.ExpectedRevision < 1 || req.Fields == nil {
			reply(w, 400, api.Error{Code: "invalid_review", Message: "Нужны поля и актуальная версия записи."})
			return
		}
		row, err := s.Store.Review(r.Context(), r.PathValue("id"), position, *req.ExpectedRevision, *req.Fields)
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, 200, row)
	})
	for _, action := range []string{"resume", "publish"} {
		action := action
		mux.HandleFunc("POST /v1/runs/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			var req api.RevisionRequest
			if !decode(w, r, &req) {
				return
			}
			if req.ExpectedRevision < 1 {
				reply(w, 400, api.Error{Code: "invalid_revision", Message: "Нужна актуальная версия запуска."})
				return
			}
			var run api.Run
			var err error
			if action == "resume" {
				run, err = s.Store.Status(r.Context(), r.PathValue("id"), req.ExpectedRevision, api.Normalizing, "")
			} else {
				run, err = s.postgres().Publish(r.Context(), s.Store, r.PathValue("id"), req.ExpectedRevision)
			}
			if err != nil {
				failure(w, err)
				return
			}
			reply(w, 200, run)
		})
	}
	mux.HandleFunc("GET /v1/runs/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		run, err := s.Store.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			failure(w, err)
			return
		}
		if run.Status != api.Review && run.Status != api.Published {
			failure(w, ErrConflict)
			return
		}
		rows, err := s.Store.Rows(r.Context(), run.Id, 0, maxRecords, false, false)
		if err != nil {
			failure(w, err)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="jana-cli-results.json"`)
		reply(w, 200, api.Rows{Rows: rows, Total: run.Total})
	})
	mux.HandleFunc("GET /config.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = io.WriteString(w, `window.JANA_CONFIG = {apiBaseUrl: "", workspace: "local"};`)
	})
	assets, err := fs.Sub(web.Files, "static")
	if err != nil {
		panic(err)
	}
	shell, err := fs.ReadFile(assets, "_shell.html")
	if err != nil {
		panic(err)
	}
	scriptPolicy := "script-src 'self'"
	for _, match := range regexp.MustCompile(`(?s)<script[^>]*>(.*?)</script>`).FindAllSubmatch(shell, -1) {
		if len(match[1]) == 0 {
			continue
		}
		hash := sha256.Sum256(match[1])
		scriptPolicy += " 'sha256-" + base64.StdEncoding.EncodeToString(hash[:]) + "'"
	}
	static := http.FileServer(http.FS(assets))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/api" || r.URL.Path == "/geocoder" {
			// The same pre-rendered SPA shell serves each client-side destination.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(shell)
			return
		}
		static.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; "+scriptPolicy+"; style-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if r.URL.Path == "/healthz" && r.Method == "GET" {
			mux.ServeHTTP(w, r)
			return
		}
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		ip := net.ParseIP(host)
		publicHost := false
		if (len(s.Token) >= 16 || s.AuthDisabled) && s.PublicOrigin != "" {
			u, e := url.Parse(s.PublicOrigin)
			publicHost = e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && s.PublicOrigin == u.Scheme+"://"+u.Host && u.Host == r.Host
		}
		if !s.Network && !publicHost && host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			reply(w, 403, api.Error{Code: "host_forbidden", Message: "Недопустимый адрес локального интерфейса."})
			return
		}
		if !s.AuthDisabled && s.Token != "" {
			user, password, ok := r.BasicAuth()
			if !ok || user != "jana" || subtle.ConstantTimeCompare([]byte(password), []byte(s.Token)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="Локальный интерфейс Jana", charset="UTF-8"`)
				reply(w, 401, api.Error{Code: "authentication_required", Message: "Требуется вход в локальный интерфейс."})
				return
			}
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				same := err == nil && u.Host == r.Host && (u.Scheme == "http" || u.Scheme == "https")
				if !same && origin != s.PublicOrigin {
					reply(w, 403, api.Error{Code: "origin_forbidden", Message: "Запрос должен исходить из интерфейса этого сервиса."})
					return
				}
			}
		}
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		mux.ServeHTTP(w, r)
	})
}
