package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"example.com/jana-locate-cli/internal/api"
	"example.com/jana-locate-cli/internal/app"
)

type webSession struct {
	URL   string
	Owned bool
	close func()
}

func (w *webSession) Close() {
	if w != nil && w.close != nil {
		w.close()
		w.close = nil
	}
}
func openSite(site string) {
	fmt.Fprintln(os.Stderr, "Веб-проверка:", site)
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", site)
	} else {
		cmd = exec.Command("xdg-open", site)
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if cmd.Start() == nil {
		go cmd.Wait()
	} else {
		fmt.Fprintln(os.Stderr, "Откройте ссылку в браузере на своём компьютере.")
	}
}

// A shared workspace has one worker; foreground CLI commands can reuse serve.
func startWeb(ctx context.Context, store *app.Store, n *app.Normalizer, p app.Postgres, state, listen string, profiles ...string) (*webSession, error) {
	state, _ = filepath.Abs(state)
	profilePath := filepath.Join(filepath.Dir(state), "connection.json")
	if len(profiles) > 0 {
		profilePath, _ = filepath.Abs(profiles[0])
	}
	lock, err := os.OpenFile(state+".serve.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	metadata := state + ".web.json"
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		for attempt := 0; attempt < 20; attempt++ {
			b, err := os.ReadFile(metadata)
			var v struct{ URL, ProfilePath string }
			if err == nil && json.Unmarshal(b, &v) == nil && v.URL != "" {
				if v.ProfilePath != "" && v.ProfilePath != profilePath {
					return nil, errors.New("Локальный сервис использует другой --profile; укажите его путь или отдельные --state и --listen")
				}
				return &webSession{URL: v.URL}, nil
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
		return nil, errors.New("Локальный сервис уже запущен, но адрес сайта недоступен")
	}
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		lock.Close()
		return nil, errors.New("Адрес --listen должен иметь вид хост:порт")
	}
	ip := net.ParseIP(host)
	network := host != "localhost" && (ip == nil || !ip.IsLoopback())
	token, err := secret("JANA_WEB_TOKEN")
	if err != nil {
		lock.Close()
		return nil, err
	}
	authMode := env("JANA_WEB_AUTH", "password")
	if authMode != "password" && authMode != "none" {
		lock.Close()
		return nil, errors.New("JANA_WEB_AUTH должен быть password или none")
	}
	authDisabled := authMode == "none"
	if authDisabled {
		token = ""
	}
	if network && !authDisabled && len(token) < 16 {
		lock.Close()
		return nil, errors.New("сетевой интерфейс требует JANA_WEB_TOKEN минимум 16 символов")
	}
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		lock.Close()
		return nil, errors.New("порт веб-проверки занят")
	}
	workerCtx, cancel := context.WithCancel(ctx)
	connections := &app.Connections{
		Load: func() (string, api.ConnectionProfile, error) {
			saved, err := readProfile(profilePath)
			if errors.Is(err, os.ErrNotExist) {
				return "", api.ConnectionProfile{}, nil
			}
			if err != nil {
				return "", api.ConnectionProfile{}, err
			}
			defaults, err := app.DescribeConnection(saved.DatabaseURL, &saved.Source, saved.VerifiedAt)
			return saved.DatabaseURL, defaults, err
		},
		DSN: func() string {
			saved, err := readProfile(profilePath)
			if err == nil {
				return saved.DatabaseURL
			}
			return p.DSN
		},
		Save: func(ctx context.Context, dsn string, src api.Source) error {
			return writeProfile(profilePath, profile{DatabaseURL: dsn, Source: src, APIURL: n.URL, VerifiedAt: time.Now().UTC().Format(time.RFC3339Nano)})
		},
	}
	server := &http.Server{Handler: app.Server{Normalizer: n, Store: store, Postgres: p, Connections: connections, Token: token, PublicOrigin: os.Getenv("JANA_PUBLIC_ORIGIN"), Network: network, AuthDisabled: authDisabled}.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 300 * time.Second, IdleTimeout: 60 * time.Second}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	if network {
		host = "localhost"
	}
	site := "http://" + net.JoinHostPort(host, port) + "/"
	if external := os.Getenv("JANA_BROWSER_URL"); external != "" {
		u, e := url.Parse(external)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			listener.Close()
			lock.Close()
			cancel()
			return nil, errors.New("JANA_BROWSER_URL должен быть адресом HTTP(S) без учётных данных, параметров запроса и фрагмента")
		}
		site = strings.TrimRight(external, "/") + "/"
	}
	b, _ := json.Marshal(struct{ URL, ProfilePath string }{site, profilePath})
	if err = os.WriteFile(metadata, b, 0600); err != nil {
		listener.Close()
		lock.Close()
		cancel()
		return nil, err
	}
	workDone := make(chan struct{})
	serveDone := make(chan struct{})
	go func() { defer close(workDone); app.Work(workerCtx, store, n) }()
	go func() { defer close(serveDone); _ = server.Serve(listener) }()
	closed := false
	closeFn := func() {
		if closed {
			return
		}
		closed = true
		cancel()
		stop, release := context.WithTimeout(context.Background(), 10*time.Second)
		defer release()
		_ = server.Shutdown(stop)
		<-workDone
		<-serveDone
		_ = os.Remove(metadata)
		lock.Close()
	}
	return &webSession{URL: site, Owned: true, close: closeFn}, nil
}
