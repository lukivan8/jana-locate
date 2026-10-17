// Package jobs processes durable work independently of browser connections.
package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/normalize"
	"github.com/lukivan8/jana-address/pkg/api"
)

func Run(ctx context.Context, store *catalog.JobStore, engine normalize.Engine) {
	pause := func() bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(time.Second):
			return true
		}
	}
	for ctx.Err() == nil {
		j, err := store.Claim()
		if err != nil {
			if !errors.Is(err, catalog.ErrJobNotFound) {
				slog.Error("claim normalization job", "error", err)
			}
			if !pause() {
				return
			}
			continue
		}
		results, message := process(ctx, engine, j.Records)
		if ctx.Err() != nil {
			return
		} // Remain running on disk; startup requeues interrupted work.
		for {
			err = store.Finish(j.Id, results, message)
			if errors.Is(err, catalog.ErrJobLimit) {
				results = nil
				message = "Job results exceed storage capacity. Delete old jobs and create a new job."
				continue
			}
			if err == nil || errors.Is(err, catalog.ErrJobNotFound) {
				break
			}
			slog.Error("persist normalization job", "error", err)
			if !pause() {
				return
			}
		}
	}
}
func process(ctx context.Context, engine normalize.Engine, records []api.InputRecord) (results []api.Result, message string) {
	defer func() {
		if recover() != nil {
			results = nil
			message = "Normalization failed; create a new job to retry."
		}
	}()
	results = engine.NormalizeBatchContext(ctx, records)
	return
}
