package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/httpapi"
	"github.com/lukivan8/jana-address/internal/normalize"
	"github.com/lukivan8/jana-address/internal/postal"
	"github.com/lukivan8/jana-address/internal/reference"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func main() {
	// A self-contained readiness probe keeps the final image free of curl and shells.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		c := http.Client{Timeout: 2 * time.Second}
		r, e := c.Get("http://127.0.0.1" + env("HTTP_ADDR", ":8080") + "/readyz")
		if e != nil {
			os.Exit(1)
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	store, err := catalog.Open(env("CATALOG_SEED_PATH", "data/catalog.seed.json"), env("CATALOG_OVERLAY_PATH", "var/catalog.confirmed.json"))
	if err != nil {
		slog.Error("load catalog", "error", err)
		os.Exit(1)
	}
	engine := normalize.Engine{Catalog: store, AllowDemo: env("ALLOW_DEMO_CATALOG", "false") == "true"}
	engine.Postal, err = postal.Open(env("LIBPOSTAL_DATA_DIR", ""))
	if err != nil {
		if env("MODELS_REQUIRED", "false") == "true" {
			slog.Error("required libpostal unavailable", "error", err)
			os.Exit(1)
		}
		slog.Warn("libpostal disabled", "error", err)
	}
	if path := os.Getenv("REFERENCE_PATH"); path != "" {
		engine.Reference, err = reference.Open(path)
		if err != nil {
			slog.Error("load immutable reference", "error", err)
			os.Exit(1)
		}
	}
	if endpoint := os.Getenv("GLINER_URL"); endpoint != "" {
		model := &normalize.GLiNER{URL: endpoint}
		probe, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = model.Ready(probe)
		cancel()
		if err != nil && env("MODELS_REQUIRED", "false") == "true" {
			slog.Error("required GLiNER unavailable", "error", err)
			os.Exit(1)
		}
		if err != nil {
			slog.Warn("GLiNER not ready; requests will expose degraded warnings", "error", err)
		}
		engine.GLiNER = model
	} else if env("MODELS_REQUIRED", "false") == "true" {
		slog.Error("GLINER_URL is required in model-enabled mode")
		os.Exit(1)
	}
	slog.Info("normalization configuration", "reference_entities", engine.Reference.Count(), "allow_demo_catalog", engine.AllowDemo)
	server := &http.Server{Addr: env("HTTP_ADDR", ":8080"), Handler: httpapi.New(store, strings.Split(env("CORS_ORIGINS", "http://localhost:3000"), ","), engine), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 300 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 16}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			slog.Error("shutdown", "error", err)
		}
	}()
	slog.Info("jana-address listening", "address", server.Addr, "catalog_entries", len(store.List()))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("serve", "error", err)
		os.Exit(1)
	}
}
