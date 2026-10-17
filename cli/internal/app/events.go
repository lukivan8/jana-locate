package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"example.com/jana-locate-cli/internal/api"
)

// Watch the journal rather than process memory: foreground CLI commands and the
// worker can commit through different Store instances/processes.
func (s Server) events(w http.ResponseWriter, r *http.Request) {
	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "Поток обновлений недоступен", 500)
		return
	}
	runs, err := s.Store.List(r.Context())
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Time{})
	var previous []byte
	previousRevision := ""
	send := func(runs []api.Run) bool {
		data, err := json.Marshal(api.RunList{Runs: runs})
		if err != nil {
			return false
		}
		if bytes.Equal(data, previous) {
			return true
		}
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err = fmt.Fprintf(w, "event: runs\ndata: %s\n\n", data); err != nil {
			return false
		}
		if controller.Flush() != nil {
			return false
		}
		_ = controller.SetWriteDeadline(time.Time{})
		previous = data
		var revision strings.Builder
		for _, run := range runs {
			fmt.Fprintf(&revision, "%s:%d;", run.Id, run.Revision)
		}
		previousRevision = revision.String()
		return true
	}
	if !send(runs) {
		return
	}
	changes := time.NewTicker(500 * time.Millisecond)
	defer changes.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-changes.C:
			revision, readErr := s.Store.eventRevision(r.Context())
			if readErr != nil {
				return
			}
			if revision == previousRevision {
				continue
			}
			runs, err = s.Store.List(r.Context())
			if err != nil {
				return
			}
			if !send(runs) {
				return
			}
		case <-heartbeat.C:
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err = fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			if controller.Flush() != nil {
				return
			}
			_ = controller.SetWriteDeadline(time.Time{})
		}
	}
}

// Check cheap metadata revisions before recounting possibly large snapshots.
func (s *Store) eventRevision(ctx context.Context) (string, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,json_extract(metadata,'$.revision') FROM runs ORDER BY rowid DESC")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var result strings.Builder
	for rows.Next() {
		var id string
		var revision int
		if err = rows.Scan(&id, &revision); err != nil {
			return "", err
		}
		fmt.Fprintf(&result, "%s:%d;", id, revision)
	}
	return result.String(), rows.Err()
}
