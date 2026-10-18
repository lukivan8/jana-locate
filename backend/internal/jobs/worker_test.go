package jobs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lukivan8/jana-address/internal/catalog"
	"github.com/lukivan8/jana-address/internal/normalize"
	"github.com/lukivan8/jana-address/internal/postal"
	"github.com/lukivan8/jana-address/pkg/api"
)

type interruptedParser struct{ entered, release chan struct{} }

func (p interruptedParser) Parse(string) ([]postal.Component, error) {
	close(p.entered)
	<-p.release
	return nil, nil
}

type brokenParser struct{}

func (brokenParser) Parse(string) ([]postal.Component, error) { panic("test parser failure") }
func waitJob(t *testing.T, s *catalog.JobStore, id, status string) api.NormalizationJob {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		j, err := s.Get(id)
		if err == nil && string(j.Status) == status {
			return j
		}
		select {
		case <-timeout.C:
			t.Fatalf("job did not become %s: %+v %v", status, j, err)
		case <-tick.C:
		}
	}
}
func TestInterruptedWorkerResumesFromDurableInput(t *testing.T) {
	dir := t.TempDir()
	buildings, err := catalog.Open("../../data/catalog.seed.json", filepath.Join(dir, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "jobs.json")
	store, err := catalog.OpenJobs(path)
	if err != nil {
		t.Fatal(err)
	}
	j, err := store.Create([]api.InputRecord{{Id: "original", RawAddress: "  Астана, Туран 55/6 кв 77  "}})
	if err != nil {
		t.Fatal(err)
	}
	parser := interruptedParser{make(chan struct{}), make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); Run(ctx, store, normalize.Engine{Catalog: buildings, Postal: parser}) }()
	select {
	case <-parser.entered:
	case <-time.After(3 * time.Second):
		cancel()
		close(parser.release)
		t.Fatal("worker did not start")
	}
	cancel()
	close(parser.release)
	<-done
	if persisted, _ := store.Get(j.Id); persisted.Status != "running" {
		t.Fatal("shutdown incorrectly committed results")
	}
	restored, err := catalog.OpenJobs(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	done = make(chan struct{})
	go func() { defer close(done); Run(ctx, restored, normalize.Engine{Catalog: buildings}) }()
	completed := waitJob(t, restored, j.Id, "completed")
	cancel()
	<-done
	if len(completed.Results) != 1 || completed.Results[0].RawAddress != j.Records[0].RawAddress || completed.Results[0].Id != "original" {
		t.Fatal("recovered output lost source linkage")
	}
}
func TestWorkerRecordsParserFailure(t *testing.T) {
	dir := t.TempDir()
	buildings, err := catalog.Open("../../data/catalog.seed.json", filepath.Join(dir, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := catalog.OpenJobs(filepath.Join(dir, "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	j, err := store.Create([]api.InputRecord{{Id: "failure", RawAddress: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(ctx, store, normalize.Engine{Catalog: buildings, Postal: brokenParser{}})
	}()
	failed := waitJob(t, store, j.Id, "failed")
	cancel()
	<-done
	if failed.Error == "" {
		t.Fatal("failure hidden")
	}
}

func TestUnavailableContextualFailsJob(t *testing.T) {
	results, message := process(context.Background(), normalize.Engine{Contextual: &normalize.ContextualClient{URL: "http://127.0.0.1:1"}}, []api.InputRecord{{Id: "a", RawAddress: "Астана"}})
	if results != nil || message == "" {
		t.Fatal("inference outage treated as completed job")
	}
}
