package main

import (
	"fmt"

	"example.com/jana-locate-cli/internal/api"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

type addressInput struct {
	label string
	value *string
}
type addressEditor struct {
	row                   api.Row
	fields                []addressInput
	field, cursor, scroll int
}

func newAddressEditor(row api.Row) *addressEditor {
	r := row.Result
	fields := []addressInput{{"Область", r.Province}, {"Населённый пункт", r.City}, {"Район", r.District}, {"Жилой массив", r.LivingArea}, {"Тип улицы", r.StreetType}, {"Улица", r.StreetName}, {"Дом", r.House}, {"Квартира", r.Apartment}, {"Подъезд", r.Entrance}, {"Этаж", r.Floor}, {"Ориентир", r.Landmark}}
	// A draft owns its values independently of background journal refreshes.
	for i := range fields {
		if fields[i].value != nil {
			value := *fields[i].value
			fields[i].value = &value
		}
	}
	e := &addressEditor{row: row, fields: fields}
	e.end()
	return e
}
func (e *addressEditor) text() string {
	if value := e.fields[e.field].value; value != nil {
		return *value
	}
	return ""
}
func (e *addressEditor) end()          { e.cursor = len([]rune(e.text())) }
func (e *addressEditor) move(step int) { e.field = max(0, min(len(e.fields)-1, e.field+step)); e.end() }
func (e *addressEditor) set(value string) {
	e.fields[e.field].value = &value
	e.cursor = min(e.cursor, len([]rune(value)))
}
func (u *publishUI) insertEdit(r rune) {
	e := u.editor
	text := []rune(e.text())
	e.cursor = max(0, min(e.cursor, len(text)))
	value := string(text[:e.cursor]) + string(r) + string(text[e.cursor:])
	if len(value) > 4096 {
		u.failed = true
		u.message = "Поле не может превышать 4096 байт."
		return
	}
	e.set(value)
	e.cursor++
}
func (e *addressEditor) values() api.AddressFields {
	f := e.fields
	return api.AddressFields{Province: f[0].value, City: f[1].value, District: f[2].value, LivingArea: f[3].value, StreetType: f[4].value, StreetName: f[5].value, House: f[6].value, Apartment: f[7].value, Entrance: f[8].value, Floor: f[9].value, Landmark: f[10].value}
}
func (u *publishUI) editKey(ev *tcell.EventKey) {
	e := u.editor
	switch ev.Key() {
	case tcell.KeyEscape:
		u.editor = nil
		u.mode = "list"
		u.message = "Изменения отменены."
		u.failed = false
	case tcell.KeyCtrlS:
		_, err := u.store.Review(u.ctx, u.run.Id, e.row.Position, e.row.Revision, e.values())
		if err != nil {
			u.notice(err)
			return
		}
		u.mode = "list"
		u.editor = nil
		u.dirty = true
		u.failed = false
		u.message = fmt.Sprintf("Запись #%d сохранена и подтверждена. a — опубликовать готовые.", e.row.Position+1)
	case tcell.KeyTab, tcell.KeyDown, tcell.KeyEnter:
		e.move(1)
	case tcell.KeyBacktab, tcell.KeyUp:
		e.move(-1)
	case tcell.KeyLeft:
		e.cursor = max(0, e.cursor-1)
	case tcell.KeyRight:
		e.cursor = min(len([]rune(e.text())), e.cursor+1)
	case tcell.KeyHome, tcell.KeyCtrlA:
		e.cursor = 0
	case tcell.KeyEnd, tcell.KeyCtrlE:
		e.end()
	case tcell.KeyCtrlU:
		e.set("")
		e.cursor = 0
	case tcell.KeyCtrlN:
		e.fields[e.field].value = nil
		e.cursor = 0
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		text := []rune(e.text())
		if e.cursor > 0 {
			position := e.cursor - 1
			e.set(string(text[:position]) + string(text[e.cursor:]))
			e.cursor = position
		}
	case tcell.KeyDelete:
		text := []rune(e.text())
		if e.cursor < len(text) {
			e.set(string(text[:e.cursor]) + string(text[e.cursor+1:]))
		}
	case tcell.KeyRune:
		if ev.Rune() >= 32 {
			u.insertEdit(ev.Rune())
		}
	}
}

func (u *publishUI) renderEditor() {
	e := u.editor
	w, h := u.screen.Size()
	normal := tcell.StyleDefault
	dim := normal.Dim(true)
	u.put(1, 7, fmt.Sprintf("Редактирование записи #%d · сохранение подтверждает результат", e.row.Position+1), u.style(tcell.ColorWhite, true))
	u.put(1, 8, "Уверенность: "+rowConfidence(e.row)+" · Исходный: "+rawAddress(e.row), dim)
	capacity := max(1, h-13)
	if e.field < e.scroll {
		e.scroll = e.field
	}
	if e.field >= e.scroll+capacity {
		e.scroll = e.field - capacity + 1
	}
	labelWidth := min(19, max(8, w/3))
	valueX := labelWidth + 3
	valueWidth := max(1, w-valueX-2)
	for i := e.scroll; i < len(e.fields) && i < e.scroll+capacity; i++ {
		y := 9 + i - e.scroll
		field := e.fields[i]
		style := normal
		if i == e.field {
			style = u.style(tcell.ColorLightGreen, true)
		}
		u.put(1, y, cellClip(field.label, labelWidth), style)
		value := "NULL · не задано"
		vstyle := dim
		if field.value != nil {
			value = *field.value
			vstyle = normal
			if value == "" {
				value = "(пустая строка)"
				vstyle = dim
			}
		}
		if i == e.field {
			// Keep the insertion point visible for long Unicode values.
			text := []rune(e.text())
			start := 0
			for start < e.cursor && uniseg.StringWidth(string(text[start:e.cursor])) >= valueWidth {
				start++
			}
			if field.value != nil && len(text) > 0 {
				value = string(text[start:])
				vstyle = normal
			}
			u.screen.ShowCursor(valueX+uniseg.StringWidth(string(text[start:e.cursor])), y)
		}
		u.put(valueX, y, cellClip(value, valueWidth), vstyle)
	}
	msg := u.message
	style := dim
	if u.failed {
		style = u.style(tcell.ColorRed, true)
	}
	u.put(1, h-4, msg, style)
	u.put(1, h-3, fmt.Sprintf("Поле %d/%d · Tab / ↑↓ — поля · ←→ Home End — курсор", e.field+1, len(e.fields)), dim)
	u.put(1, h-2, "Ctrl+U — пустая строка · Ctrl+N — NULL (не задано)", dim)
	u.put(1, h-1, "Ctrl+S — сохранить и подтвердить · Esc — отменить", normal)
	u.screen.Show()
}

// Mouse navigation changes focus, never saves a draft.
func (u *publishUI) editMouse(ev *tcell.EventMouse) {
	e := u.editor
	if ev.Buttons()&tcell.WheelDown != 0 {
		e.move(1)
	}
	if ev.Buttons()&tcell.WheelUp != 0 {
		e.move(-1)
	}
	_, y := ev.Position()
	_, h := u.screen.Size()
	if ev.Buttons()&tcell.Button1 != 0 && y >= 9 && y < 9+max(1, h-13) {
		e.field = min(len(e.fields)-1, e.scroll+y-9)
		e.end()
	}
}
