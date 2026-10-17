package main

import (
	"fmt"
	"os"
	"strings"

	"example.com/jana-locate-cli/internal/api"
	"golang.org/x/term"
)

func publicationProgress(quiet bool) func(int, int) {
	if quiet || !term.IsTerminal(int(os.Stderr.Fd())) {
		return nil
	}
	last := -1
	return func(done, total int) {
		percent := 0
		if total > 0 {
			percent = 100 * done / total
		}
		percent = min(99, percent)
		if percent == last {
			return
		}
		last = percent
		filled := percent * 24 / 100
		line := fmt.Sprintf("Запись адресов… [%s%s] %d%% (%d/%d) · транзакция", strings.Repeat("█", filled), strings.Repeat("░", 24-filled), percent, done, total)
		width, _, err := term.GetSize(int(os.Stderr.Fd()))
		if err != nil {
			width = 80
		}
		fmt.Fprint(os.Stderr, "\r\x1b[2K", cellClip(line, width))
	}
}
func publicationSummary(run api.Run, summary api.RunSummary, o flowOptions) {
	if o.quiet {
		return
	}
	if term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprint(os.Stderr, "\r\x1b[2K")
	}
	fmt.Fprintln(os.Stderr, "✓ Запись завершена · 100%")
	fmt.Fprintf(os.Stderr, "✓ Записано: %d автоматически нормализованных\n✓ Подтверждено вручную: %d\n", summary.Automatic, summary.Reviewed)
	written := summary.Automatic + summary.Reviewed
	if run.PublishedCount != nil {
		written = *run.PublishedCount
	} else if run.Status == api.Published {
		written = run.Total
	}
	skipped := max(0, run.Total-written)
	if extra := written - summary.Automatic - summary.Reviewed; extra > 0 {
		fmt.Fprintf(os.Stderr, "⚠ Дополнительно записано требующих проверки: %d\n", extra)
	}
	fmt.Fprintf(os.Stderr, "⚠ Пропущено: %d (требуют проверки)\n", skipped)
	if run.Status == api.Review {
		fmt.Fprintln(os.Stderr, "Запуск остаётся открытым. Правки: jana normalize --run "+run.Id+"\nПовторный jana normalize покажет неподтверждённые адреса.")
	}
}
