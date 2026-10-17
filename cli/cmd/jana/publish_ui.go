package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"example.com/jana-locate-cli/internal/api"
	"example.com/jana-locate-cli/internal/app"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

type publishUI struct {
	screen                                  tcell.Screen
	store                                   *app.Store
	ctx                                     context.Context
	options                                 flowOptions
	run                                     api.Run
	summary                                 api.RunSummary
	rows                                    []api.Row
	total, page, cursor, scroll             int
	query, sort, mode, input, message, site string
	detailScroll, confirmRevision           int
	revision                                int
	dirty, failed, pasting                  bool
	open                                    func() error
	normalize                               bool
	nativeMouseOff                          bool
	confirmRow                              *api.Row
	editor                                  *addressEditor
	normalizer                              *app.Normalizer
	cityData                                *api.CityCandidates
	cityRow                                 *api.Row
	cityCursor                              int
	write                                   func(api.Run, func(int, int)) (api.Run, error)
}

func newPublishUI(ctx context.Context, s *app.Store, run api.Run, o flowOptions, screen tcell.Screen, site string, open func() error) *publishUI {
	if o.pageSize == 0 {
		o.pageSize = 20
	}
	if o.sort == "" {
		o.sort = "position"
	}
	if run.PublishedCount != nil && *run.PublishedCount < run.Total {
		o.readyOnlyView = true
	}
	return &publishUI{ctx: ctx, store: s, run: run, options: o, screen: screen, site: site, open: open, sort: o.sort, query: o.search, dirty: true, mode: "list"}
}
func (u *publishUI) browseOptions(limit, offset int) app.BrowseOptions {
	return app.BrowseOptions{Limit: limit, Offset: offset, Query: u.query, ReviewOnly: u.reviewOnly(), Sort: u.sort}
}
func (u *publishUI) reviewOnly() bool { return u.options.readyOnlyView }
func (u *publishUI) refresh() error {
	run, err := u.store.Get(u.ctx, u.run.Id)
	if err != nil {
		return err
	}
	if !u.dirty && run.Revision == u.revision {
		return nil
	}
	u.run = run
	u.summary, err = u.store.Summary(u.ctx, run.Id)
	if err != nil {
		return err
	}
	u.rows, u.total, err = u.store.Browse(u.ctx, run.Id, u.browseOptions(u.options.pageSize, u.page*u.options.pageSize))
	if err != nil {
		return err
	}
	last := max(0, (u.total-1)/u.options.pageSize)
	if u.page > last {
		u.page = last
		u.rows, u.total, err = u.store.Browse(u.ctx, run.Id, u.browseOptions(u.options.pageSize, u.page*u.options.pageSize))
		if err != nil {
			return err
		}
	}
	u.cursor = max(0, min(u.cursor, len(u.rows)-1))
	u.scroll = min(u.scroll, u.cursor)
	u.revision = run.Revision
	u.dirty = false
	return nil
}
func cellClip(text string, width int) string {
	text = clean(text)
	if width < 1 {
		return ""
	}
	if uniseg.StringWidth(text) <= width {
		return text
	}
	var b strings.Builder
	n := 0
	g := uniseg.NewGraphemes(text)
	for g.Next() {
		if n+g.Width() > width-1 {
			break
		}
		b.WriteString(g.Str())
		n += g.Width()
	}
	return b.String() + "…"
}
func wrapLines(text string, width int) []string {
	width = max(1, width)
	out := []string{}
	for _, line := range strings.Split(text, "\n") {
		var b strings.Builder
		n := 0
		g := uniseg.NewGraphemes(clean(line))
		for g.Next() {
			w := g.Width()
			if n+w > width && n > 0 {
				out = append(out, b.String())
				b.Reset()
				n = 0
			}
			b.WriteString(g.Str())
			n += w
		}
		out = append(out, b.String())
	}
	return out
}
func (u *publishUI) style(color tcell.Color, bold bool) tcell.Style {
	style := tcell.StyleDefault.Bold(bold)
	if !u.options.noColor && os.Getenv("NO_COLOR") == "" {
		style = style.Foreground(color)
	}
	return style
}
func (u *publishUI) put(x, y int, text string, style tcell.Style) {
	w, h := u.screen.Size()
	if y < 0 || y >= h || x >= w {
		return
	}
	g := uniseg.NewGraphemes(cellClip(text, w-x))
	for g.Next() {
		runes := []rune(g.Str())
		if len(runes) > 0 {
			u.screen.SetContent(x, y, runes[0], runes[1:], style)
		}
		x += g.Width()
	}
}
func (u *publishUI) pages() int { return max(1, (u.total+u.options.pageSize-1)/u.options.pageSize) }
func (u *publishUI) currentRow() (api.Row, bool) {
	if u.cursor < 0 || u.cursor >= len(u.rows) {
		return api.Row{}, false
	}
	return u.rows[u.cursor], true
}
func (u *publishUI) detail() string {
	row, ok := u.currentRow()
	if !ok {
		return "В этой очереди записей нет."
	}
	id := row.SourceId
	if row.InputId != nil {
		id = *row.InputId
	}
	lines := []string{fmt.Sprintf("Запись #%d · %s", row.Position+1, rowStatus(row)), "ID источника: " + id, "", "Исходный адрес:", rawAddress(row), "", "Нормализованный адрес:", normalizedAddress(row.Result), "", "Уверенность: " + rowConfidence(row), "Причины и предупреждения:"}
	for _, warning := range row.Result.Warnings {
		lines = append(lines, "• "+reason(warning))
	}
	if len(row.Result.Warnings) == 0 {
		lines = append(lines, "Диагностических предупреждений нет.")
	}
	if u.normalize {
		lines = append(lines, "", "c — подтвердить · e — текстовая форма · l — справочник · w — браузер")
	} else {
		lines = append(lines, "", "Подтверждение и исправления: jana normalize --run "+u.run.Id)
	}
	return strings.Join(lines, "\n")
}
func (u *publishUI) help() string {
	intro := "Проверка перед публикацией — без изменения адресов."
	editing := "Подтверждение и редактирование: jana normalize --run " + u.run.Id
	if u.normalize {
		intro = "Нормализация, проверка и публикация в одном журнале."
		editing = "c — подтвердить выбранную запись после просмотра; e — текстовая форма; w — браузер.\nПубликация доступна после завершения обработки. q сохраняет журнал."
	}
	return strings.Join([]string{
		intro,
		"",
		"j / ↓ / N / PgDn — следующая страница",
		"k / ↑ / P / PgUp — предыдущая страница",
		"g — перейти на страницу по номеру",
		"/ — поиск по исходному адресу, области или населённому пункту",
		"f — только требующие проверки / все записи",
		"s — сортировка: номер → статус → регион → уверенность",
		"Tab / → — следующая запись; Shift+Tab / ← — предыдущая",
		"Enter / v / щелчок мышью — полный адрес и причины проверки",
		"В деталях: j/k, стрелки и колесо мыши — прокрутка",
		"x — экспорт текущей страницы; X — всего отфильтрованного списка",
		"Экспорт: json или csv и путь к новому файлу; это не публикация",
		"w — браузер и полная ссылка",
		"l — выбрать населённый пункт из справочника и пересчитать адрес",
		"a — опубликовать все готовые записи запуска после подтверждения",
		"q — сохранить журнал и выйти; ? — открыть/закрыть справку",
		"",
		"Процент рядом со статусом — уверенность нормализатора; до разбора — прочерк.",
		"Готовые: автоматически нормализованные и подтверждённые оператором.",
		"Поиск, сортировка и страница не ограничивают публикацию.",
		"Неподтверждённые остаются в журнале для следующего normalize.",
		"Повторный normalize после частичной записи начинает с этой очереди.",
		"",
		editing,
	}, "\n")
}
func (u *publishUI) render() {
	// Let the terminal handle link clicks and selection on the URL screen.
	// Restore application mouse navigation when returning to the workspace.
	if u.mode == "url" && !u.nativeMouseOff {
		u.screen.DisableMouse()
		u.nativeMouseOff = true
	}
	if u.mode != "url" && u.nativeMouseOff {
		u.screen.EnableMouse(tcell.MouseButtonEvents)
		u.nativeMouseOff = false
	}
	u.screen.Clear()
	u.screen.HideCursor()
	w, h := u.screen.Size()
	normal := tcell.StyleDefault
	dim := normal.Dim(true)
	if w < 20 || h < 14 {
		u.put(0, 0, "Увеличьте окно терминала", normal)
		u.put(0, 1, "q — выйти", normal)
		u.screen.Show()
		return
	}
	id := cellClip(u.run.Id, 8)
	if u.options.verbose {
		id = u.run.Id
	}
	command := "publish"
	if u.normalize {
		command = "normalize"
	}
	u.put(1, 0, "jana "+command+" · запуск "+id+" · "+runStatus(u.run.Status), u.style(tcell.ColorWhite, true))
	u.put(1, 1, fmt.Sprintf("Обработано: %d/%d · Авто: %d · Проверено: %d · Требуют проверки: %d", u.run.Processed, u.run.Total, u.summary.Automatic, u.summary.Reviewed, u.summary.NeedsReview), normal)
	source := u.run.Source.Schema + "." + u.run.Source.Table + " → " + u.run.Source.TargetSchema + "." + u.run.Source.TargetTable
	if app.IsFile(u.run.Source) {
		source = "Файл: " + u.run.Source.Table
	}
	u.put(1, 2, source, dim)
	filter := "все"
	if u.reviewOnly() {
		filter = "нужна проверка"
	}
	labels := map[string]string{"position": "по номеру", "status": "по статусу", "region": "по региону", "risk": "по уверенности"}
	u.put(1, 3, fmt.Sprintf("Страница %d/%d · Записей: %d из %d · Фильтр: %s", u.page+1, u.pages(), len(u.rows), u.total, filter), normal)
	u.put(1, 4, "Сортировка: "+labels[u.sort]+" · Поиск: "+u.query, dim)
	timing := timingText(u.run)
	if u.normalize && u.run.Status == api.Normalizing {
		width := max(1, min(24, w-35))
		filled := 0
		if u.run.Total > 0 {
			filled = width * u.run.Processed / u.run.Total
		}
		timing = fmt.Sprintf("Разбор [%s%s] %d/%d", strings.Repeat("█", min(width, filled)), strings.Repeat("░", max(0, width-filled)), u.run.Processed, u.run.Total)
	}
	if u.normalize && u.run.Status == api.Failed {
		timing = "Разбор прерван: " + clean(u.run.Error) + " · r — повторить"
	}
	if u.run.PublishedCount != nil {
		timing = fmt.Sprintf("В таблице: %d · ", *u.run.PublishedCount) + timing
	}
	u.put(1, 5, timing, dim)
	u.put(0, 6, strings.Repeat("─", w), dim)
	if u.mode == "edit" {
		u.renderEditor()
		return
	}
	if u.mode == "list" {
		capacity := max(1, (h-10)/3)
		if u.cursor < u.scroll {
			u.scroll = u.cursor
		}
		if u.cursor >= u.scroll+capacity {
			u.scroll = u.cursor - capacity + 1
		}
		for i := u.scroll; i < len(u.rows) && i < u.scroll+capacity; i++ {
			row := u.rows[i]
			y := 7 + (i-u.scroll)*3
			status := rowStatus(row)
			color := tcell.ColorYellow
			if row.Reviewed || row.Result.Status == api.Matched {
				color = tcell.ColorLightGreen
			}
			if w < 70 {
				if row.Reviewed {
					status = "ПРОВЕРЕН"
				} else if row.Result.Status == api.Matched {
					status = "ГОТОВО"
				} else if row.Normalized != nil && !*row.Normalized {
					status = "ОЖИДАЕТ"
				} else {
					status = "ПРОВЕРКА"
				}
			}
			prefix := fmt.Sprintf(" #%d [%s] %4s ", row.Position+1, status, rowConfidence(row))
			if w >= 70 {
				prefix = fmt.Sprintf(" %-8s %-22s %4s ", fmt.Sprintf("#%d", row.Position+1), "["+status+"]", rowConfidence(row))
			}
			style := u.style(color, i == u.cursor)
			if i == u.cursor {
				prefix = ">" + prefix[1:]
			}
			u.put(0, y, prefix, style)
			u.put(uniseg.StringWidth(prefix), y, rawAddress(row), normal)
			delta := addressChanges(rawAddress(row), normalizedAddress(row.Result))
			if normalizedAddress(row.Result) == "" {
				delta = "Компоненты адреса не определены"
				if row.Normalized != nil && !*row.Normalized {
					delta = "Ожидает разбора"
				}
			}
			u.put(3, y+1, "→ "+delta, dim)
			if !row.Reviewed && row.Result.Status != api.Matched && len(row.Result.Warnings) > 0 {
				u.put(3, y+2, reason(row.Result.Warnings[0]), u.style(tcell.ColorYellow, false))
			}
		}
		if len(u.rows) == 0 {
			u.put(2, 8, "В этой очереди записей нет.", normal)
			u.put(2, 9, "f — все записи · a — публикация готовых", dim)
		}
	} else {
		body := ""
		switch u.mode {
		case "city":
			body = u.cityBody()
		case "details":
			body = u.detail()
		case "help":
			body = u.help()
		case "url":
			body = "Веб-проверка:\n" + u.site + "?run=" + u.run.Id + "\n\nCmd+Click / Ctrl+Click — открыть в браузере.\nСсылку также можно выделить и скопировать.\nEsc — назад"
		case "review":
			row := u.confirmRow
			if row != nil {
				body = fmt.Sprintf("Подтверждение записи #%d\n\nИсходный адрес:\n%s\n\nНормализованный адрес:\n%s\n\nУверенность: %s\n\nПричины проверки:\n", row.Position+1, rawAddress(*row), normalizedAddress(row.Result), rowConfidence(*row))
				for _, warning := range row.Result.Warnings {
					body += "• " + reason(warning) + "\n"
				}
				body += "\ny — подтвердить этот результат; Esc — отмена"
			}
		case "publish":
			body = fmt.Sprintf("Публикация всех готовых записей запуска\n\nНормализовано автоматически: %d\nПодтверждено оператором: %d\nБудет пропущено: %d\n\nРезультат: %s\n\nИсходная таблица не изменится.\nПоиск, фильтр и страница не ограничивают публикацию.\n\ny — записать; Esc — отмена", u.summary.Automatic, u.summary.Reviewed, u.summary.NeedsReview, u.run.Source.TargetSchema+"."+u.run.Source.TargetTable)
			if app.IsFile(u.run.Source) {
				body += "\nФайл: " + publicationFile(u.run, u.options)
			}
		case "search":
			body = "Поиск по адресу / региону (пустой запрос сбрасывает поиск)\n\n" + u.input + "\n\nEnter — применить; Ctrl+U — очистить; Esc — отмена"
		case "goto":
			body = fmt.Sprintf("Номер страницы (1–%d):\n\n%s\n\nEnter — перейти; Esc — отмена", u.pages(), u.input)
		case "export", "export-all":
			body = "Экспорт " + map[bool]string{true: "всего отфильтрованного списка", false: "текущей страницы"}[u.mode == "export-all"] + "\n\n" + u.input + "\n\nФормат и путь: json /data/page.json\nEnter — сохранить; Ctrl+U — очистить; Esc — отмена\nСуществующий файл не перезаписывается."
		}
		lines := wrapLines(body, w-4)
		u.detailScroll = max(0, min(u.detailScroll, max(0, len(lines)-(h-10))))
		target := u.site + "?run=" + u.run.Id
		urlLines := len(wrapLines(target, w-4))
		for y := 7; y < h-3 && y-7+u.detailScroll < len(lines); y++ {
			index := y - 7 + u.detailScroll
			style := normal
			if u.mode == "url" && index >= 1 && index <= urlLines {
				style = u.style(tcell.ColorLightBlue, false).Underline(true).Url(target).UrlId("jana-run-" + u.run.Id)
			}
			u.put(2, y, lines[index], style)
		}
	}
	style := dim
	if u.failed {
		style = u.style(tcell.ColorRed, true)
	}
	message := u.message
	if message == "" && u.mode == "list" && len(u.rows) > 0 {
		message = fmt.Sprintf("На экране: %d–%d из %d записей страницы · Tab / ← / → — выбор", u.scroll+1, min(u.scroll+max(1, (h-10)/3), len(u.rows)), len(u.rows))
	}
	u.put(1, h-3, message, style)
	u.put(1, h-2, "j/↓ k/↑ — страницы · Tab — запись · / поиск · f фильтр · g номер", dim)
	footer := "Enter детали · w браузер · a публикация · q выход · ? справка"
	if u.normalize {
		footer = "w браузер · e правки · l справочник · c подтвердить · a публикация · q выход · ?"
	}
	u.put(1, h-1, footer, dim)
	if u.mode != "list" {
		u.put(1, h-2, strings.Repeat(" ", w-1), normal)
		u.put(1, h-1, strings.Repeat(" ", w-1), normal)
		u.put(1, h-2, "↑/↓ — прокрутка · Esc — назад", dim)
		copy := "Подтверждение и правки адресов — в normalize"
		if u.normalize {
			copy = "c — подтвердить · e — текстовая форма · l — справочник · w — браузер"
		}
		if u.mode == "url" {
			copy = "Мышь — ссылка и выделение текста · Esc — назад"
		}
		u.put(1, h-1, copy, dim)
	}
	if u.mode == "city" {
		u.put(1, h-2, strings.Repeat(" ", w-1), normal)
		u.put(1, h-1, strings.Repeat(" ", w-1), normal)
		u.put(1, h-2, "↑↓ / j k — вариант · PgUp/PgDn — прокрутка", dim)
		hint := "Enter — выбрать и пересчитать · Esc — отменить"
		if u.cityData == nil || len(u.cityData.Candidates) < 2 {
			hint = "Enter / Esc — назад"
		}
		u.put(1, h-1, hint, normal)
	}
	if u.mode == "publish" || u.mode == "review" {
		u.put(1, h-1, strings.Repeat(" ", w-1), normal)
		label := "y — опубликовать готовые · Esc — отмена"
		if u.mode == "review" {
			label = "y — подтвердить выбранный результат · Esc — отмена"
		}
		u.put(1, h-1, label, normal)
	}
	u.screen.Show()
}
func publicationFile(run api.Run, o flowOptions) string {
	if o.output != "" {
		return o.output
	}
	name := run.Source.Table
	return strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)) + ".normalized.json"
}
func (u *publishUI) notice(err error) {
	u.failed = err != nil
	if err != nil {
		u.message = app.UserMessage(err)
	}
}
func (u *publishUI) movePage(step int) {
	u.page = max(0, min(u.page+step, u.pages()-1))
	u.cursor = 0
	u.scroll = 0
	u.dirty = true
}
func (u *publishUI) handle(event tcell.Event) (bool, bool, error) {
	switch ev := event.(type) {
	case *tcell.EventResize:
		u.screen.Sync()
	case *tcell.EventMouse:
		if u.mode == "edit" {
			u.editMouse(ev)
			return false, false, nil
		}
		_, y := ev.Position()
		_, height := u.screen.Size()
		if ev.Buttons()&tcell.WheelDown != 0 {
			if u.mode == "list" {
				u.movePage(1)
			} else {
				u.detailScroll += 3
			}
		}
		if ev.Buttons()&tcell.WheelUp != 0 {
			if u.mode == "list" {
				u.movePage(-1)
			} else {
				u.detailScroll = max(0, u.detailScroll-3)
			}
		}
		if ev.Buttons()&tcell.Button1 != 0 && u.mode == "list" && y >= 7 && y < 7+max(1, (height-10)/3)*3 {
			index := u.scroll + (y-7)/3
			if index < len(u.rows) {
				u.cursor = index
				u.mode = "details"
				u.detailScroll = 0
			}
		}
	case *tcell.EventPaste:
		u.pasting = ev.Start()
	case *tcell.EventKey:
		if u.pasting {
			if u.mode == "edit" && ev.Key() == tcell.KeyRune && ev.Rune() >= 32 {
				u.insertEdit(ev.Rune())
				return false, false, nil
			}
			if (u.mode == "search" || u.mode == "goto" || u.mode == "export" || u.mode == "export-all") && ev.Key() == tcell.KeyRune && ev.Rune() >= 32 && len(u.input) < 8192 {
				u.input += string(ev.Rune())
			}
			return false, false, nil
		}
		if ev.Key() == tcell.KeyCtrlC {
			return true, false, nil
		}
		if u.mode == "city" {
			u.cityKey(ev)
			return false, false, nil
		}
		if u.mode == "edit" {
			u.editKey(ev)
			return false, false, nil
		}
		r := ev.Rune()
		if u.mode == "publish" || u.mode == "review" {
			if ev.Key() == tcell.KeyDown {
				u.detailScroll++
			}
			if ev.Key() == tcell.KeyUp {
				u.detailScroll = max(0, u.detailScroll-1)
			}
			if ev.Key() == tcell.KeyPgDn {
				u.detailScroll += 10
			}
			if ev.Key() == tcell.KeyPgUp {
				u.detailScroll = max(0, u.detailScroll-10)
			}
			if ev.Key() == tcell.KeyEscape || r == 'q' || r == 'n' {
				u.mode = "list"
				return false, false, nil
			}
			if r == 'y' || r == 'Y' || r == 'д' || r == 'Д' {
				if u.mode == "review" {
					row := u.confirmRow
					if row == nil {
						u.mode = "list"
						return false, false, nil
					}
					r := row.Result
					fields := api.AddressFields{Province: r.Province, City: r.City, District: r.District, LivingArea: r.LivingArea, StreetType: r.StreetType, StreetName: r.StreetName, House: r.House, Apartment: r.Apartment, Entrance: r.Entrance, Floor: r.Floor, Landmark: r.Landmark}
					_, err := u.store.Review(u.ctx, u.run.Id, row.Position, row.Revision, fields)
					u.mode = "list"
					u.dirty = true
					u.message = fmt.Sprintf("Запись #%d подтверждена. a — опубликовать готовые.", row.Position+1)
					u.notice(err)
					return false, false, nil
				}
				latest, err := u.store.Get(u.ctx, u.run.Id)
				if err != nil {
					return false, false, err
				}
				if latest.Revision != u.confirmRevision {
					u.mode = "list"
					u.dirty = true
					u.message = "Данные изменились. Проверьте обновлённый список и подтвердите публикацию заново."
					return false, false, nil
				}
				if u.write != nil {
					u.mode = "list"
					before := u.summary
					published, err := u.write(latest, func(done, total int) {
						percent := 0
						if total > 0 {
							percent = min(99, 100*done/total)
						}
						filled := percent * 20 / 100
						u.message = fmt.Sprintf("Запись [%s%s] %d%% (%d/%d) · транзакция", strings.Repeat("█", filled), strings.Repeat("░", 20-filled), percent, done, total)
						u.render()
					})
					u.dirty = true
					if err != nil {
						u.notice(err)
						return false, false, nil
					}
					u.run = published
					u.options.readyOnlyView = published.Status == api.Review
					u.page = 0
					u.cursor = 0
					u.scroll = 0
					u.failed = false
					u.message = fmt.Sprintf("✓ Записано · 100%% · Авто: %d · Вручную: %d · Пропущено: %d", before.Automatic, before.Reviewed, before.NeedsReview)
					return false, false, nil
				}
				return true, true, nil
			}
			return false, false, nil
		}
		if u.mode == "search" || u.mode == "goto" || u.mode == "export" || u.mode == "export-all" {
			switch ev.Key() {
			case tcell.KeyEscape:
				u.mode = "list"
			case tcell.KeyCtrlU:
				u.input = ""
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				rr := []rune(u.input)
				if len(rr) > 0 {
					u.input = string(rr[:len(rr)-1])
				}
			case tcell.KeyRune:
				if r >= 32 && len(u.input) < 8192 {
					u.input += string(r)
				}
			case tcell.KeyEnter:
				switch u.mode {
				case "search":
					u.query = strings.TrimSpace(u.input)
					u.page = 0
					u.cursor = 0
					u.scroll = 0
					u.dirty = true
					u.mode = "list"
				case "goto":
					n, err := strconv.Atoi(strings.TrimSpace(u.input))
					if err != nil || n < 1 || n > u.pages() {
						u.message = "Введите номер существующей страницы"
						break
					}
					u.page = n - 1
					u.cursor = 0
					u.scroll = 0
					u.dirty = true
					u.mode = "list"
				default:
					args := strings.SplitN(strings.TrimSpace(u.input), " ", 2)
					if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
						u.message = "Укажите формат и путь к файлу"
						break
					}
					rows := u.rows
					scope := "page"
					if u.mode == "export-all" {
						var err error
						rows, _, err = u.store.Browse(u.ctx, u.run.Id, u.browseOptions(100000, 0))
						if err != nil {
							u.notice(err)
							break
						}
						scope = "filtered"
					}
					if err := exportView(strings.TrimSpace(args[1]), args[0], u.run.Id, scope, rows); err != nil {
						u.notice(err)
						break
					}
					u.message = fmt.Sprintf("Экспортировано записей: %d · %s", len(rows), clean(args[1]))
					u.failed = false
					u.mode = "list"
				}
			}
			return false, false, nil
		}
		if u.normalize && (u.mode == "list" || u.mode == "details") && (r == 'l' || r == 'L') {
			u.openCities()
			return false, false, nil
		}
		if u.normalize && (u.mode == "list" || u.mode == "details") && (r == 'c' || r == 'e' || r == 'w' || r == 'W') {
			if r == 'e' {
				if u.run.Status != api.Review {
					u.message = "Редактирование доступно после завершения разбора."
					return false, false, nil
				}
				if row, ok := u.currentRow(); ok {
					u.editor = newAddressEditor(row)
					u.mode = "edit"
					u.message = ""
					u.failed = false
				}
				return false, false, nil
			}
			if r == 'c' {
				if u.run.Status != api.Review {
					u.message = "Подтверждение доступно после завершения разбора."
					return false, false, nil
				}
				if row, ok := u.currentRow(); ok {
					u.confirmRow = &row
					u.mode = "review"
					u.detailScroll = 0
				}
				return false, false, nil
			}
			if err := u.screen.Suspend(); err != nil {
				u.notice(err)
				return false, false, nil
			}
			err := u.open()
			resumed := u.screen.Resume()
			if err == nil {
				err = resumed
			}
			u.notice(err)
			u.mode = "url"
			u.detailScroll = 0
			return false, false, nil
		}
		if u.mode != "list" {
			if ev.Key() == tcell.KeyEscape || r == 'q' || r == '?' || ev.Key() == tcell.KeyEnter {
				u.mode = "list"
				u.detailScroll = 0
				return false, false, nil
			}
			if ev.Key() == tcell.KeyDown || r == 'j' {
				u.detailScroll++
			}
			if ev.Key() == tcell.KeyUp || r == 'k' {
				u.detailScroll = max(0, u.detailScroll-1)
			}
			if ev.Key() == tcell.KeyPgDn {
				u.detailScroll += 10
			}
			if ev.Key() == tcell.KeyPgUp {
				u.detailScroll = max(0, u.detailScroll-10)
			}
			return false, false, nil
		}
		switch ev.Key() {
		case tcell.KeyDown, tcell.KeyPgDn:
			u.movePage(1)
		case tcell.KeyUp, tcell.KeyPgUp:
			u.movePage(-1)
		case tcell.KeyTab, tcell.KeyRight:
			u.cursor = min(u.cursor+1, len(u.rows)-1)
		case tcell.KeyBacktab, tcell.KeyLeft:
			u.cursor = max(0, u.cursor-1)
		case tcell.KeyHome:
			u.cursor = 0
		case tcell.KeyEnd:
			u.cursor = max(0, len(u.rows)-1)
		case tcell.KeyEnter:
			if len(u.rows) > 0 {
				u.mode = "details"
				u.detailScroll = 0
			}
		case tcell.KeyRune:
			switch r {
			case 'q', 'Q':
				return true, false, nil
			case 'j', 'n', 'N':
				u.movePage(1)
			case 'k', 'p', 'P':
				u.movePage(-1)
			case 'g':
				u.mode = "goto"
				u.input = ""
			case '/':
				u.mode = "search"
				u.input = u.query
			case 'f':
				u.options.readyOnlyView = !u.options.readyOnlyView
				u.page = 0
				u.cursor = 0
				u.scroll = 0
				u.dirty = true
			case 's':
				sorts := []string{"position", "status", "region", "risk", "position"}
				for i := 0; i < len(sorts)-1; i++ {
					if u.sort == sorts[i] {
						u.sort = sorts[i+1]
						break
					}
				}
				u.page = 0
				u.cursor = 0
				u.scroll = 0
				u.dirty = true
			case '?':
				u.mode = "help"
				u.detailScroll = 0
			case 'v':
				if len(u.rows) > 0 {
					u.mode = "details"
					u.detailScroll = 0
				}
			case 'w', 'W':
				if err := u.screen.Suspend(); err != nil {
					u.notice(err)
					break
				}
				err := u.open()
				resumeErr := u.screen.Resume()
				if err == nil {
					err = resumeErr
				}
				u.notice(err)
				u.mode = "url"
				u.detailScroll = 0
			case 'x', 'X':
				u.mode = "export"
				scope := "page"
				if r == 'X' {
					u.mode = "export-all"
					scope = "filtered"
				}
				u.input = "json " + filepath.Join(filepath.Dir(u.options.state), fmt.Sprintf("jana-%s-%s.json", u.run.Id[:min(8, len(u.run.Id))], scope))
			case 'a', 'A':
				if u.run.Status != api.Review && u.run.Status != api.Published {
					u.message = "Публикация доступна после завершения разбора."
					break
				}
				if u.run.Status == api.Published {
					u.message = "Этот запуск уже опубликован."
					break
				}
				if u.summary.Automatic+u.summary.Reviewed == 0 {
					u.message = "Нет готовых записей. Для подтверждения вернитесь в jana normalize."
					if u.normalize {
						u.message = "Нет готовых записей. c — проверить результат; e — текстовая форма; w — браузер."
					}
					break
				}
				u.mode = "publish"
				u.confirmRevision = u.run.Revision
				u.detailScroll = 0
			case 'r':
				if u.normalize && u.run.Status == api.Failed {
					_, err := u.store.Status(u.ctx, u.run.Id, u.run.Revision, api.Normalizing, "")
					u.dirty = true
					u.notice(err)
					if err == nil {
						u.message = "Разбор продолжен с сохранённой позиции."
					}
					break
				}
				if u.normalize {
					break
				}
				u.message = "publish не изменяет записи. Подтверждение и правки — в jana normalize."
			case 'c', 'e', ' ':
				u.message = "publish не изменяет записи. Подтверждение и правки — в jana normalize."
			}
		}
	}
	return false, false, nil
}

func runPublishUI(ctx context.Context, s *app.Store, run api.Run, o flowOptions, site string, open func() error) (api.Run, bool, error) {
	return runWorkspaceUI(ctx, s, run, o, site, open, nil)
}

func runWorkspaceUI(ctx context.Context, s *app.Store, run api.Run, o flowOptions, site string, open func() error, write func(api.Run, func(int, int)) (api.Run, error), normalizers ...*app.Normalizer) (api.Run, bool, error) {
	screen, err := tcell.NewScreen()
	if err != nil {
		return run, false, fmt.Errorf("Не удалось открыть терминал; используйте --plain")
	}
	if err = screen.Init(); err != nil {
		return run, false, fmt.Errorf("Не удалось настроить терминал; используйте --plain")
	}
	defer screen.Fini()
	screen.EnableMouse(tcell.MouseButtonEvents)
	screen.EnablePaste()
	screen.HideCursor()
	u := newPublishUI(ctx, s, run, o, screen, site, open)
	u.normalize = write != nil
	u.write = write
	if len(normalizers) > 0 {
		u.normalizer = normalizers[0]
	}
	if run.PublishedCount != nil && *run.PublishedCount < run.Total {
		u.options.readyOnlyView = true
	}
	if err = u.refresh(); err != nil {
		return run, false, err
	}
	stop, done := make(chan struct{}), make(chan struct{})
	defer func() { close(stop); <-done }()
	go func() {
		defer close(done)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_ = screen.PostEvent(tcell.NewEventInterrupt(nil))
			}
		}
	}()
	for {
		if err = ctx.Err(); err != nil {
			return u.run, false, err
		}
		if err = u.refresh(); err != nil {
			return u.run, false, err
		}
		u.render()
		event := screen.PollEvent()
		if event == nil {
			return u.run, false, nil
		}
		finished, publish, err := u.handle(event)
		if err != nil {
			return u.run, false, err
		}
		if finished {
			return u.run, publish, nil
		}
	}
}
