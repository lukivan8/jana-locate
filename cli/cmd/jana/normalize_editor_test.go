package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/jana-locate-cli/internal/api"
	"example.com/jana-locate-cli/internal/app"
	"github.com/gdamore/tcell/v2"
)

func editorKey(t *testing.T, u *publishUI, key tcell.Key, r rune) {
	t.Helper()
	done, publish, err := u.handle(tcell.NewEventKey(key, r, 0))
	if done || publish || err != nil {
		t.Fatalf("editor key %v %q: done=%v publish=%v %v", key, r, done, publish, err)
	}
	u.render()
}
func editorText(t *testing.T, u *publishUI, text string) {
	t.Helper()
	for _, r := range text {
		editorKey(t, u, tcell.KeyRune, r)
	}
}

func TestTerminalEditorSaveCancelNullAndPublication(t *testing.T) {
	u, s, run := uiFixture(t)
	u.normalize = true
	u.cursor = 1
	before, _ := s.Get(u.ctx, run.Id)
	editorKey(t, u, tcell.KeyRune, 'e')
	if u.mode != "edit" || u.editor == nil {
		t.Fatal("no native editor")
	}
	editorText(t, u, "Не сохранять")
	editorKey(t, u, tcell.KeyEscape, 0)
	latest, _ := s.Get(u.ctx, run.Id)
	if latest.Revision != before.Revision {
		t.Fatal("cancel saved draft")
	}
	editorKey(t, u, tcell.KeyRune, 'e')
	// Null province remains null; city becomes explicitly empty.
	editorKey(t, u, tcell.KeyTab, 0)
	editorKey(t, u, tcell.KeyCtrlU, 0)
	for i := 0; i < 4; i++ {
		editorKey(t, u, tcell.KeyTab, 0)
	}
	editorKey(t, u, tcell.KeyCtrlU, 0)
	editorText(t, u, "Тестовая")
	// Home/End and Unicode insertion/delete leave the intended value.
	editorKey(t, u, tcell.KeyHome, 0)
	editorText(t, u, "я")
	editorKey(t, u, tcell.KeyHome, 0)
	editorKey(t, u, tcell.KeyDelete, 0)
	editorKey(t, u, tcell.KeyEnd, 0)
	editorText(t, u, "я")
	editorKey(t, u, tcell.KeyBackspace2, 0)
	editorKey(t, u, tcell.KeyTab, 0)
	editorKey(t, u, tcell.KeyCtrlN, 0) // house -> null
	if err := u.refresh(); err != nil {
		t.Fatal(err)
	} // draft survives live refresh
	editorKey(t, u, tcell.KeyCtrlS, 0)
	if u.mode != "list" || u.editor != nil || u.failed {
		t.Fatalf("save failed: %s", u.message)
	}
	rows, _ := s.Rows(u.ctx, run.Id, 1, 1, false, false)
	r := rows[0].Result
	if !rows[0].Reviewed || r.Province != nil || r.City == nil || *r.City != "" || r.House != nil || r.StreetName == nil || *r.StreetName != "Тестовая" || r.Apartment == nil || *r.Apartment != "21" {
		t.Fatalf("field semantics lost: %+v", r)
	}
	if err := u.refresh(); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "ready.json")
	u.write = func(run api.Run, p func(int, int)) (api.Run, error) {
		return s.ExportFileWithOptions(u.ctx, run.Id, output, run.Revision, false, app.WriteOptions{ReadyOnly: true, Progress: p})
	}
	editorKey(t, u, tcell.KeyRune, 'a')
	editorKey(t, u, tcell.KeyRune, 'y')
	data, _ := os.ReadFile(output)
	var result struct{ Results []api.Result }
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 2 || !strings.Contains(string(data), "9223372036854775807") || !strings.Contains(string(data), "Тестовая") {
		t.Fatalf("publication lost edit or exact ID: %s", data)
	}
}

func TestTerminalEditorConflictKeepsDraft(t *testing.T) {
	u, s, run := uiFixture(t)
	u.normalize = true
	u.cursor = 1
	editorKey(t, u, tcell.KeyRune, 'e')
	editorText(t, u, "Мой черновик")
	row := u.editor.row
	if _, err := s.Review(u.ctx, run.Id, row.Position, row.Revision, api.AddressFields{Province: stringValue("Новая правка")}); err != nil {
		t.Fatal(err)
	}
	if err := u.refresh(); err != nil {
		t.Fatal(err)
	}
	editorKey(t, u, tcell.KeyCtrlS, 0)
	if !u.failed || u.mode != "edit" || u.editor.text() != "Мой черновик" {
		t.Fatal("conflict lost the draft")
	}
	rows, _ := s.Rows(u.ctx, run.Id, 1, 1, false, false)
	if rows[0].Result.Province == nil || *rows[0].Result.Province != "Новая правка" {
		t.Fatal("overwrote newer revision")
	}
	editorKey(t, u, tcell.KeyEscape, 0)
	if err := u.refresh(); err != nil {
		t.Fatal(err)
	}
	editorKey(t, u, tcell.KeyRune, 'e')
	if u.editor.text() != "Новая правка" {
		t.Fatal("reopen did not load latest revision")
	}
}

func TestTerminalEditorPasteLimitsAndSmallWindow(t *testing.T) {
	u, s, run := uiFixture(t)
	u.normalize = true
	editorKey(t, u, tcell.KeyRune, 'e')
	u.handle(tcell.NewEventPaste(true))
	editorText(t, u, "ayq? / Алматы")
	editorKey(t, u, tcell.KeyCtrlS, 0)
	editorKey(t, u, tcell.KeyEnter, 0)
	u.handle(tcell.NewEventPaste(false))
	if u.mode != "edit" || u.editor.field != 0 || u.editor.text() != "ayq? / Алматы" {
		t.Fatal("paste triggered a command")
	}
	before, _ := s.Get(u.ctx, run.Id)
	if before.Reviewed != 0 {
		t.Fatal("paste saved a row")
	}
	e := u.editor
	e.set(strings.Repeat("я", 2048))
	e.end()
	editorText(t, u, "я")
	if len(e.text()) != 4096 || !u.failed {
		t.Fatal("field byte limit not enforced")
	}
	screen := u.screen.(tcell.SimulationScreen)
	for _, size := range [][2]int{{39, 14}, {80, 24}, {120, 40}} {
		screen.SetSize(size[0], size[1])
		u.render()
		if !strings.Contains(textUI(u), "Ctrl+S") {
			t.Fatalf("save hint hidden at %v", size)
		}
	}
	// Deleting the only rune leaves a valid cursor at zero.
	e.set("я")
	e.end()
	editorKey(t, u, tcell.KeyBackspace2, 0)
	if e.text() != "" || e.cursor != 0 {
		t.Fatal("backspace cursor corrupted")
	}
	editorKey(t, u, tcell.KeyCtrlN, 0)
	if e.fields[0].value != nil {
		t.Fatal("NULL action failed")
	}
	editorKey(t, u, tcell.KeyCtrlU, 0)
	if e.fields[0].value == nil || *e.fields[0].value != "" {
		t.Fatal("empty string conflated with NULL")
	}
}
