package app

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRussianUserMessages(t *testing.T) {
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, io.EOF, os.ErrPermission, os.ErrNotExist, errors.New("driver failure: password=private-secret")} {
		message := UserMessage(err)
		if !strings.ContainsAny(message, "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЭЮЯ") || strings.Contains(message, "private-secret") {
			t.Fatalf("untranslated or unsafe message: %s", message)
		}
	}
	message := "Таблица результата уже существует и не принадлежит этому источнику Jana. Укажите другое имя таблицы результата"
	if UserMessage(errors.New(message)) != message {
		t.Fatal("lost actionable error guidance")
	}
}
