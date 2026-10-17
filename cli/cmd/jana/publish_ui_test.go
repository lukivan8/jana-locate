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

func uiFixture(t *testing.T) (*publishUI, *app.Store, api.Run) {
	t.Helper()
	ctx := context.Background()
	state := filepath.Join(t.TempDir(), "journal.sqlite")
	store, err := app.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })
	input := []api.InputRecord{}
	for i := 0; i < 35; i++ {
		raw := "Караганда, улица Пробная, дом 41/3"
		if i < 2 {
			raw = "Алматы, Рассветная 8, кв 21"
		}
		input = append(input, api.InputRecord{Id: json.Number("9223372036854775807"), RawAddress: raw})
	}
	run, err := store.ImportRecords(ctx, input, "http://normalizer.invalid")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := store.Rows(ctx, run.Id, 0, 100, false, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		rows[i].Result.Status = api.NeedsReview
		rows[i].Result.City = stringValue("Алматы")
		if i > 1 {
			rows[i].Result.City = stringValue("Караганда")
		}
		rows[i].Result.StreetName = stringValue("Пробная")
		rows[i].Result.House = stringValue("41/3")
		rows[i].Result.StreetType = stringValue("улица")
		if i < 2 {
			rows[i].Result.StreetName = stringValue("Рассветная")
			rows[i].Result.House = stringValue("8")
			rows[i].Result.Apartment = stringValue("21")
		}
		rows[i].Result.Confidence = .55
		rows[i].Result.Warnings = []string{"ambiguous_reference_street"}
	}
	rows[0].Result.Status = api.Matched
	rows[0].Result.Warnings = []string{}
	rows[0].Result.Confidence = .93
	if err = store.SaveBatch(ctx, run.Id, rows); err != nil {
		t.Fatal(err)
	}
	run, _ = store.Get(ctx, run.Id)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err = screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(120, 40)
	t.Cleanup(screen.Fini)
	u := newPublishUI(ctx, store, run, flowOptions{pageSize: 20, readyOnly: true, state: state}, screen, "http://localhost:8095/", func() error { return nil })
	if err = u.refresh(); err != nil {
		t.Fatal(err)
	}
	return u, store, run
}
func stringValue(v string) *string { return &v }
func keyUI(t *testing.T, u *publishUI, key tcell.Key, r rune) (bool, bool) {
	t.Helper()
	done, publish, err := u.handle(tcell.NewEventKey(key, r, tcell.ModNone))
	if err != nil {
		t.Fatal(err)
	}
	if err = u.refresh(); err != nil {
		t.Fatal(err)
	}
	u.render()
	return done, publish
}
func textUI(u *publishUI) string {
	screen := u.screen.(tcell.SimulationScreen)
	cells, w, h := screen.GetContents()
	lines := make([]string, h)
	for y := 0; y < h; y++ {
		var b strings.Builder
		for x := 0; x < w; x++ {
			r := cells[y*w+x].Runes
			if len(r) == 0 {
				b.WriteByte(' ')
			} else {
				b.WriteString(string(r))
			}
		}
		lines[y] = b.String()
	}
	return strings.Join(lines, "\n")
}
func TestPublishNavigationSearchDetailsAndReadOnlyActions(t *testing.T) {
	u, s, run := uiFixture(t)
	u.render()
	if text := textUI(u); !strings.Contains(text, "Страница 1/2") || strings.Contains(text, run.Id) {
		t.Fatalf("initial frame: %s", text)
	}
	keyUI(t, u, tcell.KeyDown, 0)
	if u.page != 1 || u.rows[0].Position != 20 {
		t.Fatal("arrow pagination failed")
	}
	keyUI(t, u, tcell.KeyRune, 'g')
	keyUI(t, u, tcell.KeyRune, '1')
	keyUI(t, u, tcell.KeyEnter, 0)
	if u.page != 0 {
		t.Fatal("goto failed")
	}
	keyUI(t, u, tcell.KeyRune, '/')
	for _, r := range "АЛМАТЫ" {
		keyUI(t, u, tcell.KeyRune, r)
	}
	keyUI(t, u, tcell.KeyEnter, 0)
	if u.total != 2 {
		t.Fatalf("Unicode search: %d", u.total)
	}
	keyUI(t, u, tcell.KeyRune, 'f')
	if u.total != 1 || u.rows[0].Position != 1 || u.summary.NeedsReview != 34 {
		t.Fatal("filter changed global totals or hid ambiguity")
	}
	keyUI(t, u, tcell.KeyEnter, 0)
	if !strings.Contains(textUI(u), "Неоднозначная улица") {
		t.Fatal("reason missing")
	}
	keyUI(t, u, tcell.KeyEscape, 0)
	for _, r := range "cer " {
		keyUI(t, u, tcell.KeyRune, r)
	}
	after, _ := s.Get(u.ctx, run.Id)
	if after.Revision != run.Revision {
		t.Fatal("publish review modified the journal")
	}
	keyUI(t, u, tcell.KeyRune, '?')
	if !strings.Contains(textUI(u), "Проверка перед публикацией") {
		t.Fatal("help missing")
	}
	keyUI(t, u, tcell.KeyRune, '?')
	done, publish := keyUI(t, u, tcell.KeyRune, 'q')
	if !done || publish {
		t.Fatal("q published data")
	}
}
func TestPublishConfirmationChecksFreshRevisionAndIgnoresPastedCommands(t *testing.T) {
	u, s, run := uiFixture(t)
	u.handle(tcell.NewEventPaste(true))
	keyUI(t, u, tcell.KeyRune, 'a')
	keyUI(t, u, tcell.KeyRune, 'y')
	u.handle(tcell.NewEventPaste(false))
	if u.mode != "list" {
		t.Fatal("paste invoked a command")
	}
	keyUI(t, u, tcell.KeyRune, 'a')
	if u.mode != "publish" {
		t.Fatal("publication confirmation missing")
	}
	rows, _, _ := s.Browse(u.ctx, run.Id, app.BrowseOptions{Limit: 20})
	if _, err := s.Review(u.ctx, run.Id, rows[1].Position, rows[1].Revision, api.AddressFields{Apartment: stringValue("77")}); err != nil {
		t.Fatal(err)
	}
	done, publish := keyUI(t, u, tcell.KeyRune, 'y')
	if done || publish || u.mode != "list" {
		t.Fatal("stale confirmation published")
	}
	keyUI(t, u, tcell.KeyRune, 'a')
	done, publish = keyUI(t, u, tcell.KeyRune, 'y')
	if !done || !publish {
		t.Fatal("fresh confirmation failed")
	}
	current, _ := s.Get(u.ctx, run.Id)
	if current.Status != api.Review {
		t.Fatal("screen itself wrote a publication")
	}
}
func TestPublishMouseResizeAndExportPreserveExactInput(t *testing.T) {
	u, _, run := uiFixture(t)
	screen := u.screen.(tcell.SimulationScreen)
	for _, size := range [][2]int{{120, 40}, {80, 24}, {40, 16}, {20, 12}} {
		screen.SetSize(size[0], size[1])
		u.render()
		cells, w, h := screen.GetContents()
		if len(cells) != w*h {
			t.Fatal("invalid render dimensions")
		}
	}
	screen.SetSize(120, 40)
	u.render()
	u.handle(tcell.NewEventMouse(10, 10, tcell.Button1, tcell.ModNone))
	u.render()
	if u.mode != "details" || u.cursor != 1 {
		t.Fatal("mouse did not open clicked row")
	}
	keyUI(t, u, tcell.KeyEscape, 0)
	path := filepath.Join(t.TempDir(), "page.json")
	if err := exportView(path, "json", run.Id, "page", u.rows); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "9223372036854775807") {
		t.Fatal("exact int64 ID lost")
	}
	if err = exportView(path, "json", run.Id, "page", u.rows); err == nil {
		t.Fatal("export overwrote a file")
	}
	if files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".jana-view-*")); len(files) > 0 {
		t.Fatal("temporary export leaked")
	}
}
func TestPartialJournalDefaultsAndSourceIsolation(t *testing.T) {
	u, _, run := uiFixture(t)
	count := 1
	run.PublishedCount = &count
	run.LastPublishedAt = stringValue("2026-10-17T00:00:00Z")
	model := newPublishUI(u.ctx, u.store, run, u.options, u.screen, u.site, u.open)
	if !model.reviewOnly() {
		t.Fatal("partial run did not start with pending records")
	}
	source := api.Source{ConnectionId: stringValue("database-one"), Schema: "public", Table: "orders", Key: "id", KeyType: "bigint", Column: "address", TargetSchema: "public", TargetTable: "output"}
	run.Source = source
	run.ApiUrl = "http://normalizer.invalid"
	newer := run
	newer.Id = "explicit-new-snapshot"
	newer.PublishedCount = nil
	newer.LastPublishedAt = nil
	if partialRun([]api.Run{newer, run}, source, run.ApiUrl) != newer.Id {
		t.Fatal("an explicit newer snapshot was superseded by old answers")
	}
	if partialRun([]api.Run{run}, source, run.ApiUrl) != run.Id {
		t.Fatal("same journal was not continued")
	}
	for _, changed := range []api.Source{{ConnectionId: stringValue("other-database"), Schema: "public", Table: "orders", Key: "id", KeyType: "bigint", Column: "address", TargetSchema: "public", TargetTable: "output"}, {ConnectionId: source.ConnectionId, Schema: "public", Table: "other", Key: "id", KeyType: "bigint", Column: "address", TargetSchema: "public", TargetTable: "output"}} {
		if partialRun([]api.Run{run}, changed, run.ApiUrl) != "" {
			t.Fatal("adopted another source's answers")
		}
	}
	if partialRun([]api.Run{run}, source, "http://other-api.invalid") != "" {
		t.Fatal("adopted another API's run")
	}
}
func TestNormalizeFrameEvidence(t *testing.T) {
	path := os.Getenv("JANA_TUI_ARTIFACT_JSON")
	if path == "" {
		t.Skip("optional synthetic terminal artifact")
	}
	u, _, _ := uiFixture(t)
	u.normalize = true
	if os.Getenv("JANA_TUI_URL") == "1" {
		u.mode = "url"
	}
	if os.Getenv("JANA_TUI_EDITOR") == "1" {
		u.cursor = 1
		u.editor = newAddressEditor(u.rows[1])
		u.mode = "edit"
		u.editor.field = 6
		u.editor.end()
	}
	u.render()
	screen := u.screen.(tcell.SimulationScreen)
	cells, w, h := screen.GetContents()
	type cell struct {
		Text      string
		R, G, B   int32
		Dim, Bold bool
	}
	out := struct {
		Width, Height int
		Cells         []cell
	}{Width: w, Height: h}
	for _, c := range cells {
		fg, _, attrs := c.Style.Decompose()
		r, g, b := fg.RGB()
		out.Cells = append(out.Cells, cell{Text: string(c.Runes), R: r, G: g, B: b, Dim: attrs&tcell.AttrDim != 0, Bold: attrs&tcell.AttrBold != 0})
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
