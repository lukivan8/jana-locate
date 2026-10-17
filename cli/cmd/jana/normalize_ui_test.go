package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/jana-locate-cli/internal/api"
	"example.com/jana-locate-cli/internal/app"
	"github.com/gdamore/tcell/v2"
)

func TestNormalizeWorkspaceReviewAndPartialPublication(t *testing.T) {
	u, s, run := uiFixture(t)
	u.normalize = true
	output := filepath.Join(t.TempDir(), "published.json")
	u.write = func(r api.Run, progress func(int, int)) (api.Run, error) {
		return s.ExportFileWithOptions(u.ctx, r.Id, output, r.Revision, true, app.WriteOptions{ReadyOnly: true, Progress: progress})
	}
	if err := u.refresh(); err != nil {
		t.Fatal(err)
	}
	key := func(r rune) {
		t.Helper()
		done, publish, err := u.handle(tcell.NewEventKey(tcell.KeyRune, r, 0))
		if err != nil || done || publish {
			t.Fatalf("key %q: done=%v publish=%v %v", r, done, publish, err)
		}
	}
	// Inspect and explicitly confirm the pending row, without mutating its fields.
	u.cursor = 1
	key('c')
	if u.mode != "review" || u.confirmRow.Position != 1 {
		t.Fatal("review did not capture selected record")
	}
	key('y')
	if err := u.refresh(); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Rows(u.ctx, run.Id, 1, 1, false, false)
	if !rows[0].Reviewed || rows[0].Result.StreetName == nil || *rows[0].Result.StreetName != "Рассветная" {
		t.Fatalf("review: %+v", rows[0])
	}
	key('a')
	key('y')
	if err := u.refresh(); err != nil {
		t.Fatal(err)
	}
	if u.run.Status != api.Review || u.run.PublishedCount == nil || *u.run.PublishedCount != 2 || !u.reviewOnly() || u.total != 33 {
		t.Fatalf("partial receipt: %+v pending=%d", u.run, u.total)
	}
	if !strings.Contains(u.message, "100%") {
		t.Fatal("no committed success")
	}
	// Correct another pending record through the same store used by the browser.
	row := u.rows[0]
	fields := api.AddressFields{City: stringValue("Караганда"), StreetName: stringValue("Примерная"), House: stringValue("8")}
	if _, err := s.Review(u.ctx, run.Id, row.Position, row.Revision, fields); err != nil {
		t.Fatal(err)
	}
	if err := u.refresh(); err != nil {
		t.Fatal(err)
	}
	key('a')
	key('y')
	if err := u.refresh(); err != nil {
		t.Fatal(err)
	}
	var data struct{ Results []api.Result }
	b, _ := os.ReadFile(output)
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Results) != 3 || *u.run.PublishedCount != 3 || u.total != 32 {
		t.Fatalf("republication duplicates/loss: %s", b)
	}
	// Leaving does not perform another publication.
	before := u.run.Revision
	done, publish, err := u.handle(tcell.NewEventKey(tcell.KeyRune, 'q', 0))
	latest, _ := s.Get(u.ctx, run.Id)
	if !done || publish || err != nil || latest.Revision != before {
		t.Fatal("exit modified journal")
	}
}

func TestNormalizeReviewRejectsStaleFields(t *testing.T) {
	u, s, run := uiFixture(t)
	u.normalize = true
	if err := u.refresh(); err != nil {
		t.Fatal(err)
	}
	u.cursor = 1
	u.handle(tcell.NewEventKey(tcell.KeyRune, 'c', 0))
	row := *u.confirmRow
	_, err := s.Review(u.ctx, run.Id, row.Position, row.Revision, api.AddressFields{House: stringValue("99")})
	if err != nil {
		t.Fatal(err)
	}
	// A background refresh must not replace the confirmation snapshot.
	if err = u.refresh(); err != nil {
		t.Fatal(err)
	}
	u.handle(tcell.NewEventKey(tcell.KeyRune, 'y', 0))
	rows, _ := s.Rows(u.ctx, run.Id, 1, 1, false, false)
	if !u.failed || rows[0].Result.House == nil || *rows[0].Result.House != "99" {
		t.Fatal("stale confirmation overwrote operator changes")
	}
}

func TestNormalizeWorkspaceWaitsForProcessingAndResumes(t *testing.T) {
	u, s, _ := uiFixture(t)
	u.normalize = true
	run, err := s.ImportRecords(u.ctx, []api.InputRecord{{Id: "new", RawAddress: "Синтетический адрес"}}, "http://normalizer.invalid")
	if err != nil {
		t.Fatal(err)
	}
	u.run = run
	u.dirty = true
	if err = u.refresh(); err != nil {
		t.Fatal(err)
	}
	u.handle(tcell.NewEventKey(tcell.KeyRune, 'a', 0))
	if u.mode == "publish" {
		t.Fatal("can publish unfinished run")
	}
	u.handle(tcell.NewEventKey(tcell.KeyRune, 'c', 0))
	if u.mode == "review" {
		t.Fatal("can review unfinished run")
	}
	_, err = s.Status(u.ctx, run.Id, run.Revision, api.Failed, "синтетический сбой")
	if err != nil {
		t.Fatal(err)
	}
	if err = u.refresh(); err != nil {
		t.Fatal(err)
	}
	u.handle(tcell.NewEventKey(tcell.KeyRune, 'r', 0))
	latest, _ := s.Get(context.Background(), run.Id)
	if latest.Status != api.Normalizing {
		t.Fatal("retry did not resume saved journal")
	}
}

func TestNormalizeScriptPublicationIsExplicitAndReadyOnly(t *testing.T) {
	u, s, run := uiFixture(t)
	output := filepath.Join(t.TempDir(), "published.json")
	args := []string{"normalize", "--run", run.Id, "--state", u.options.state, "--output", output, "--quiet", "--no-interactive"}
	out, log, err := cliProcess(t, "", args...)
	if err != nil {
		t.Fatalf("inspect: %v %s", err, log)
	}
	var saved api.Run
	if err = json.Unmarshal(out, &saved); err != nil || saved.Revision != run.Revision {
		t.Fatalf("script changed journal: %s %v", out, err)
	}
	if _, err = os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("normalization implicitly published")
	}
	args = append(args, "--confirm", "--ready-only=false")
	out, log, err = cliProcess(t, "", args...)
	if err != nil {
		t.Fatalf("publish: %v %s %s", err, out, log)
	}
	latest, _ := s.Get(u.ctx, run.Id)
	if latest.PublishedCount == nil || *latest.PublishedCount != 1 || latest.Status != api.Review {
		t.Fatalf("unsafe publication: %+v", latest)
	}
}
