package main

import (
	"fmt"
	"strings"

	"example.com/jana-locate-cli/internal/api"
	"github.com/gdamore/tcell/v2"
)

func (u *publishUI) openCities() {
	if u.run.Status != api.Review {
		u.message = "Выбор населённого пункта доступен после завершения разбора."
		return
	}
	row, ok := u.currentRow()
	if !ok {
		return
	}
	if u.normalizer == nil {
		u.message = "API справочника недоступен для этой команды."
		return
	}
	u.message = "Загружаем варианты населённого пункта…"
	u.render()
	data, err := u.store.CityCandidates(u.ctx, u.normalizer, u.run.Id, row.Position)
	if err != nil {
		u.notice(err)
		return
	}
	u.cityRow = &row
	u.cityData = &data
	u.cityCursor = 0
	u.detailScroll = 0
	u.mode = "city"
	u.failed = false
	u.message = ""
	if row.CitySelection != nil && row.CitySelection.ReferenceVersion == data.ReferenceVersion {
		for i, c := range data.Candidates {
			if c.EntityId == row.CitySelection.EntityId {
				u.cityCursor = i
			}
		}
	}
}
func (u *publishUI) cityBody() string {
	if u.cityData == nil || u.cityRow == nil {
		return "Варианты недоступны."
	}
	lines := []string{fmt.Sprintf("Населённый пункт по справочнику · запись #%d", u.cityRow.Position+1), "Исходный: " + rawAddress(*u.cityRow), ""}
	if len(u.cityData.Candidates) == 0 {
		lines = append(lines, "Совместимых вариантов в справочнике нет.")
	}
	for i, c := range u.cityData.Candidates {
		marker := "  "
		if i == u.cityCursor {
			marker = "> "
		}
		kind := "тип не указан"
		if c.SettlementType != nil {
			kind = *c.SettlementType
		}
		province := "область не указана"
		if c.Province != nil {
			province = *c.Province
		}
		lines = append(lines, fmt.Sprintf("%s%d. %s · %s · %s", marker, i+1, c.Name, kind, province), "   "+c.Source+" · "+c.EntityId, "")
	}
	lines = append(lines, "Выбор пересчитает адрес из исходной строки и заменит ручные исправления.", "Остальные неоднозначности останутся видимыми. Полная проверка записи выполняется отдельно.")
	if len(u.cityData.Candidates) == 1 {
		lines = append(lines, "Единственный совместимый вариант; дополнительных альтернатив нет.")
	}
	return strings.Join(lines, "\n")
}
func (u *publishUI) cityKey(ev *tcell.EventKey) {
	if ev.Key() == tcell.KeyEscape || ev.Rune() == 'q' {
		u.mode = "list"
		return
	}
	switch ev.Key() {
	case tcell.KeyDown:
		u.cityCursor = min(max(0, len(u.cityData.Candidates)-1), u.cityCursor+1)
	case tcell.KeyUp:
		u.cityCursor = max(0, u.cityCursor-1)
	case tcell.KeyPgDn:
		u.detailScroll += 10
	case tcell.KeyPgUp:
		u.detailScroll = max(0, u.detailScroll-10)
	case tcell.KeyEnter:
		if len(u.cityData.Candidates) < 2 {
			u.mode = "list"
			return
		}
		c := u.cityData.Candidates[u.cityCursor]
		_, err := u.store.SelectCity(u.ctx, u.normalizer, u.run.Id, u.cityRow.Position, api.CitySelectionRequest{ExpectedRevision: u.cityData.Revision, ReferenceVersion: u.cityData.ReferenceVersion, EntityId: c.EntityId})
		if err != nil {
			u.notice(err)
			return
		}
		u.mode = "list"
		u.dirty = true
		u.failed = false
		u.message = "Населённый пункт выбран. Адрес пересчитан; проверьте оставшиеся поля."
	case tcell.KeyRune:
		if ev.Rune() == 'j' {
			u.cityCursor = min(max(0, len(u.cityData.Candidates)-1), u.cityCursor+1)
		}
		if ev.Rune() == 'k' {
			u.cityCursor = max(0, u.cityCursor-1)
		}
	}
}
