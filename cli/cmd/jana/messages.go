package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"example.com/jana-locate-cli/internal/api"
)

func parseFlags(f *flag.FlagSet, args []string) error {
	f.SetOutput(io.Discard)
	f.Usage = func() {
		fmt.Fprintf(os.Stderr, "Использование: jana %s [флаги]\n\nФлаги:\n", clean(f.Name()))
		f.VisitAll(func(option *flag.Flag) {
			fmt.Fprintf(os.Stderr, "  --%s\n      %s", option.Name, option.Usage)
			if option.DefValue != "" && option.DefValue != "false" && option.DefValue != "0" && option.Name != "password" && option.Name != "database-url" {
				fmt.Fprintf(os.Stderr, " (по умолчанию: %s)", clean(option.DefValue))
			}
			fmt.Fprintln(os.Stderr)
		})
	}
	err := f.Parse(args)
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return err
	}
	if name, ok := strings.CutPrefix(err.Error(), "flag provided but not defined: "); ok {
		return fmt.Errorf("Неизвестный флаг %s; справка: jana %s --help", clean(name), clean(f.Name()))
	}
	if name, ok := strings.CutPrefix(err.Error(), "flag needs an argument: "); ok {
		return fmt.Errorf("Для флага %s нужно значение; справка: jana %s --help", clean(name), clean(f.Name()))
	}
	return fmt.Errorf("Некорректное значение флага; справка: jana %s --help", clean(f.Name()))
}
func resultStatus(status api.ResultStatus) string {
	switch status {
	case api.Matched:
		return "Нормализован"
	case api.NeedsReview:
		return "Нужна проверка"
	case api.Unmatched:
		return "Не распознан"
	default:
		return "Ожидает обработки"
	}
}
func runStatus(status api.RunStatus) string {
	switch status {
	case api.Normalizing:
		return "Обрабатывается"
	case api.Review:
		return "Готово к проверке"
	case api.Failed:
		return "Ошибка обработки"
	case api.Published:
		return "Опубликован"
	default:
		return "Ожидает обработки"
	}
}
func sslChoices() ([]string, []string) {
	return []string{"prefer", "require", "verify-full", "verify-ca", "disable", "allow"}, []string{
		"Предпочитать SSL (prefer)", "Требовать SSL (require)", "Проверять сертификат и хост (verify-full)", "Проверять сертификат (verify-ca)", "Без SSL (disable)", "Допускать SSL (allow)",
	}
}
