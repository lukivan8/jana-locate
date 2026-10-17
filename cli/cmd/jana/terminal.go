package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"example.com/jana-locate-cli/internal/api"
	"example.com/jana-locate-cli/internal/app"
)

func clean(v string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, v)
}
func clip(v string, n int) string {
	r := []rune(clean(v))
	if n < 1 {
		return ""
	}
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return string(r)
}

// The fullscreen workspace owns interactive input. This watcher is used only
// for scripts and plain output; completing normalization never publishes data.
func watchNormalization(ctx context.Context, s *app.Store, id string, o flowOptions) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	last := -1
	for {
		run, err := s.Get(ctx, id)
		if err != nil {
			return err
		}
		if !o.quiet && last != run.Processed {
			fmt.Fprintf(os.Stderr, "Обработано %d/%d · %s\n", run.Processed, run.Total, runStatus(run.Status))
			last = run.Processed
		}
		if run.Status == api.Failed {
			return errors.New(run.Error + "; продолжить: jana normalize --run " + id)
		}
		if run.Status == api.Review || run.Status == api.Published {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
