package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"golang.org/x/term"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Cancellation must release the command even while stdin is idle. The reader
// owns stdin until the command exits; cancellation never starts another read.
func readAnswer(ctx context.Context, reader *bufio.Reader, password bool) (string, error) {
	type answer struct {
		value string
		err   error
	}
	out := make(chan answer, 1)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if password && term.IsTerminal(int(os.Stdin.Fd())) {
		original, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return "", err
		}
		defer term.Restore(int(os.Stdin.Fd()), original)
		defer fmt.Fprint(os.Stderr, "\r\n")
		var value []byte
		for {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if reader.Buffered() == 0 && !termInputReady(int(os.Stdin.Fd())) {
				continue
			}
			key, err := reader.ReadByte()
			if err != nil {
				return "", err
			}
			switch key {
			case 3:
				return "", context.Canceled
			case 4:
				if len(value) == 0 {
					return "", io.EOF
				}
			case '\r', '\n':
				return string(value), nil
			case 127, 8:
				if len(value) > 0 {
					_, size := utf8.DecodeLastRune(value)
					value = value[:len(value)-size]
				}
			default:
				if key >= 32 {
					value = append(value, key)
				}
			}
		}

	} else {
		go func() { v, e := reader.ReadString('\n'); out <- answer{v, e} }()
	}
	select {
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr)
		return "", ctx.Err()
	case a := <-out:
		if password {
			fmt.Fprintln(os.Stderr)
		}
		if a.err != nil && len(a.value) == 0 {
			return "", a.err
		}
		return strings.TrimSuffix(strings.TrimSuffix(a.value, "\n"), "\r"), nil
	}
}
func choose(ctx context.Context, reader *bufio.Reader, label string, values []string, current ...string) (int, error) {
	if len(values) == 0 {
		return 0, errors.New("нет доступных таблиц с одиночным поддерживаемым PK и текстовыми колонками")
	}
	defaultNumber := "1"
	if len(current) > 0 {
		for i, v := range values {
			if v == current[0] {
				defaultNumber = strconv.Itoa(i + 1)
			}
		}
	}
	if interactivePrompts() {
		selected, _ := strconv.Atoi(defaultNumber)
		return selectPrompt(ctx, reader, label, values, selected-1)
	}
	fmt.Fprintln(os.Stderr, label+":")
	for i, v := range values {
		fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, clean(v))
	}
	for {
		v, err := prompt(ctx, reader, "Номер", defaultNumber, false)
		if err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(v)
		if err == nil && n >= 1 && n <= len(values) {
			return n - 1, nil
		}
		fmt.Fprintln(os.Stderr, "Выберите номер из списка.")
	}
}
