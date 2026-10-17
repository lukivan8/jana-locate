package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"example.com/jana-locate-cli/internal/api"
)

func TestCLIProcess(t *testing.T) {
	if os.Getenv("JANA_TEST_CHILD") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			if err := run(args[i+1:]); err != nil {
				os.Stderr.WriteString(err.Error())
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}
func cliProcess(t *testing.T, input string, args ...string) ([]byte, []byte, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestCLIProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "JANA_TEST_CHILD=1", "JANA_DATABASE_URL=invalid-postgres-config", "JANA_API_URL=", "JANA_LISTEN=127.0.0.1:0")
	cmd.Stdin = strings.NewReader(input)
	var out, log bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &log
	err := cmd.Run()
	return out.Bytes(), log.Bytes(), err
}
func TestFileCommandsWithoutConnectAndConfirmation(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			Records []api.InputRecord `json:"records"`
		}
		json.NewDecoder(r.Body).Decode(&v)
		results := []api.Result{}
		for _, row := range v.Records {
			results = append(results, api.Result{Id: row.Id, RawAddress: row.RawAddress, Status: api.Unmatched, Warnings: []string{}})
		}
		json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	defer remote.Close()
	dir := t.TempDir()
	state := filepath.Join(dir, "journal.sqlite")
	file := filepath.Join(dir, "input.txt")
	output := filepath.Join(dir, "result.json")
	os.WriteFile(file, []byte("Алматы\nАстана\n"+strings.Repeat("Неизвестный адрес\n", 40)), 0600)
	args := []string{"--state", state, "--api-url", remote.URL, "--listen", "127.0.0.1:0"}
	out, log, err := cliProcess(t, "", append([]string{"normalize", "--file", file, "--no-interactive", "--quiet"}, args...)...)
	if err != nil {
		t.Fatalf("normalize: %v %s", err, log)
	}
	var r api.Run
	if err = json.Unmarshal(out, &r); err != nil || r.Status != api.Review || r.Processed != 42 {
		t.Fatalf("result %s %v", out, err)
	}
	// Browser opening is unavailable in Docker/SSH; the printed run URL must still be usable.
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "xdg-open"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, flag := range []string{"-w", "--web", "--open"} {
		_, log, err := cliProcess(t, "", append([]string{"publish", flag, "--run", r.Id, "--no-interactive"}, args...)...)
		if err != nil || !bytes.Contains(log, []byte("Веб-проверка: http://127.0.0.1:")) || !bytes.Contains(log, []byte("/?run="+r.Id)) {
			t.Fatalf("%s must print a run URL: %v %s", flag, err, log)
		}
	}
	assertInterruptsInput(t, append([]string{"publish", "--run", r.Id}, args...)...)
	landArgs := append([]string{"land", "--ready-only=false", "--run", r.Id, "--output", output}, args...)
	_, log, err = cliProcess(t, "n\np\nотмена\n", landArgs...)
	if err != nil {
		t.Fatalf("preview %v %s", err, log)
	}
	if !bytes.Contains(log, []byte("Веб-проверка: http://127.0.0.1:")) || !bytes.Contains(log, []byte("/?run="+r.Id)) || !bytes.Contains(log, []byte("Страница 2/3")) || !bytes.Contains(log, []byte("Требует проверки: 42")) || !bytes.Contains(log, []byte("ВНИМАНИЕ")) {
		t.Fatalf("missing pagination/summary/warning: %s", log)
	}
	if _, err = os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("preview wrote output")
	}
	_, log, err = cliProcess(t, "подтвердить\n", landArgs...)
	if err != nil {
		t.Fatalf("land %v %s", err, log)
	}
	b, err := os.ReadFile(output)
	if err != nil || !bytes.Contains(b, []byte("Алматы")) {
		t.Fatalf("export %s %v", b, err)
	}
}
func TestProfilePermissionsAndFlags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connection.json")
	p := profile{DatabaseURL: "postgresql://user:secret@localhost/db", VerifiedAt: "test"}
	if err := writeProfile(path, p); err != nil {
		t.Fatal(err)
	}
	loaded, err := readProfile(path)
	if err != nil || loaded.DatabaseURL != p.DatabaseURL {
		t.Fatal(err)
	}
	os.Chmod(path, 0644)
	if _, err = readProfile(path); err == nil {
		t.Fatal("read public credentials")
	}
	o, err := parseFlow("connect", []string{"--host", "localhost", "--table", "orders", "--no-interactive"})
	if err != nil || !o.visited["table"] || o.visited["password"] {
		t.Fatal("flags", err)
	}
}

func TestPostgresConnectNormalizeLand(t *testing.T) {
	dsn := os.Getenv("JANA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := context.Background()
	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	schema := fmt.Sprintf("flow_%d", time.Now().UnixNano())
	q := pgx.Identifier{schema}.Sanitize()
	if _, err = db.Exec(ctx, "CREATE SCHEMA "+q+"; CREATE TABLE "+q+".source(id bigint PRIMARY KEY,address text); INSERT INTO "+q+".source VALUES(9223372036854775807,'synthetic')"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, "DROP SCHEMA "+q+" CASCADE")
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			Records []api.InputRecord `json:"records"`
		}
		json.NewDecoder(r.Body).Decode(&v)
		out := []api.Result{}
		for _, row := range v.Records {
			out = append(out, api.Result{Id: row.Id, RawAddress: row.RawAddress, Status: api.Unmatched, Warnings: []string{}})
		}
		json.NewEncoder(w).Encode(map[string]any{"results": out})
	}))
	defer remote.Close()
	state := filepath.Join(t.TempDir(), "workspace.sqlite")
	common := []string{"--state", state, "--api-url", remote.URL, "--listen", "127.0.0.1:0"}
	_, log, err := cliProcess(t, "", append([]string{"connect", "--database-url", dsn, "--schema", schema, "--table", "source", "--target-schema", schema, "--target-table", "output", "--no-interactive"}, common...)...)
	if err != nil {
		t.Fatalf("connect %v %s", err, log)
	}
	cfg, _ := pgx.ParseConfig(dsn)
	if cfg.Password != "" && bytes.Contains(log, []byte(cfg.Password)) {
		t.Fatal("password printed")
	}
	// Reconnecting with Enter at every prompt must retain the source even when
	// address-prioritized tables from other schemas precede it in the catalog.
	_, log, err = cliProcess(t, strings.Repeat("\n", 10), append([]string{"connect"}, common...)...)
	if err != nil || !bytes.Contains(log, []byte("Сохранённое подключение")) {
		t.Fatalf("saved connect defaults: %v %s", err, log)
	}
	saved, err := readProfile(filepath.Join(filepath.Dir(state), "connection.json"))
	if err != nil || saved.Source.Schema != schema || saved.Source.Table != "source" || saved.Source.Column != "address" || saved.Source.TargetTable != "output" {
		t.Fatalf("changed defaults: %+v %v", saved.Source, err)
	}
	out, log, err := cliProcess(t, "", append([]string{"normalize", "--no-interactive", "--quiet"}, common...)...)
	if err != nil {
		t.Fatalf("normalize %v %s", err, log)
	}
	var r api.Run
	json.Unmarshal(out, &r)
	if r.Total != 1 || r.Status != api.Review {
		t.Fatalf("bad run %s", out)
	}
	args := append([]string{"land", "--ready-only=false", "--run", r.Id}, common...)
	_, log, err = cliProcess(t, "cancel\n", args...)
	if err != nil {
		t.Fatalf("preview %v %s", err, log)
	}
	var target *string
	db.QueryRow(ctx, "SELECT to_regclass($1)::text", schema+".output").Scan(&target)
	if target != nil {
		t.Fatal("preview created table")
	}
	_, log, err = cliProcess(t, "confirm\n", args...)
	if err != nil {
		t.Fatalf("land %v %s", err, log)
	}
	var fk bool
	err = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid=to_regclass($1) AND contype='f' AND confrelid=to_regclass($2))", schema+".output", schema+".source").Scan(&fk)
	if err != nil || !fk {
		t.Fatalf("missing relation %v", err)
	}
	var key string
	err = db.QueryRow(ctx, "SELECT source_id::text FROM "+q+".output").Scan(&key)
	if err != nil || key != "9223372036854775807" {
		t.Fatalf("bad PK %s %v", key, err)
	}
}

func TestHealthcheckUsesConfiguredListenPort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("unexpected health path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv("JANA_LISTEN", strings.TrimPrefix(server.URL, "http://"))
	if err := run([]string{"healthcheck"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JANA_LISTEN", "invalid-address")
	if err := run([]string{"healthcheck"}); err == nil {
		t.Fatal("invalid listener accepted")
	}
}

func assertInterruptsInput(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestCLIProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "JANA_TEST_CHILD=1", "JANA_DATABASE_URL=", "JANA_WEB_AUTH=none")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	log, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	go func() { b := make([]byte, 1); _, _ = log.Read(b); close(ready); io.Copy(io.Discard, log) }()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("input prompt missing")
	}
	time.Sleep(100 * time.Millisecond)
	if err = cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("interrupt ignored")
		}
	case <-time.After(2 * time.Second):
		cmd.Process.Kill()
		t.Fatal("Ctrl+C blocked on stdin")
	}
}
func TestConnectInterruptsIdleInput(t *testing.T) {
	assertInterruptsInput(t, "connect", "--state", filepath.Join(t.TempDir(), "state.sqlite"))
}

func TestRussianHelpAndFlagErrors(t *testing.T) {
	for _, command := range []string{"connect", "normalize", "publish", "serve", "runs"} {
		_, log, err := cliProcess(t, "", command, "--help")
		if err != nil || !bytes.Contains(log, []byte("Использование:")) || bytes.Contains(log, []byte("Usage of")) || bytes.Contains(log, []byte("default")) {
			t.Fatalf("help %s: %v %s", command, err, log)
		}
	}
	for _, args := range [][]string{{"connect", "--unknown-flag"}, {"connect", "--port"}, {"connect", "--port", "bad-port"}} {
		_, log, err := cliProcess(t, "", args...)
		if err == nil || !bytes.Contains(log, []byte("флаг")) || bytes.Contains(log, []byte("invalid value")) || bytes.Contains(log, []byte("flag provided")) {
			t.Fatalf("flags: %v %s", err, log)
		}
	}
}

func TestWebFlagsSelectCommandWorkflow(t *testing.T) {
	for _, command := range []string{"connect", "normalize", "publish", "land"} {
		for _, flag := range []string{"-w", "--web"} {
			o, err := parseFlow(command, []string{flag})
			if err != nil || !o.web || o.open != (command != "connect") {
				t.Fatalf("%s %s: %+v %v", command, flag, o, err)
			}
		}
	}
}
