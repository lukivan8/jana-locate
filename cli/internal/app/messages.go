package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"unicode"

	"github.com/jackc/pgx/v5/pgconn"
)

// UserMessage keeps driver/OS diagnostics out of the UI and never exposes a
// connection string. Known application errors already carry Russian guidance.
func UserMessage(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "Операция отменена"
	case errors.Is(err, context.DeadlineExceeded):
		return "Время ожидания истекло; проверьте доступность сервиса и повторите операцию"
	case errors.Is(err, io.EOF):
		return "Ввод завершён; передайте параметры флагами или запустите команду в терминале"
	case errors.Is(err, os.ErrNotExist):
		return "Файл или каталог не найден; проверьте указанный путь"
	case errors.Is(err, os.ErrPermission):
		return "Нет доступа к файлу или каталогу; проверьте права"
	case errors.Is(err, syscall.ENOSPC):
		return "Недостаточно свободного места; освободите место и повторите операцию"
	}
	var path *os.PathError
	if errors.As(err, &path) {
		return "Не удалось выполнить операцию с файлом или каталогом; проверьте путь и права доступа"
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return fmt.Sprintf("Ошибка PostgreSQL (код %s); проверьте права, структуру таблиц и параметры подключения", pg.Code)
	}
	var network net.Error
	if errors.As(err, &network) {
		return "Сетевой запрос не выполнен; проверьте адрес, порт и доступность сервиса"
	}
	var sqlite interface{ Code() int }
	if errors.As(err, &sqlite) {
		switch sqlite.Code() & 255 {
		case 5, 6:
			return "Локальный журнал занят другой операцией; повторите попытку"
		case 8:
			return "Локальный журнал доступен только для чтения; проверьте права"
		case 13:
			return "Недостаточно места для локального журнала; освободите место и повторите операцию"
		case 11, 26:
			return "Не удалось прочитать локальный журнал; проверьте файл журнала"
		default:
			return "Не удалось выполнить операцию с локальным журналом; проверьте права, свободное место и файл журнала"
		}
	}
	for _, r := range err.Error() {
		if unicode.In(r, unicode.Cyrillic) {
			return err.Error()
		}
	}
	return "Не удалось выполнить операцию; проверьте настройки, доступ к файлам и состояние локального журнала"
}
