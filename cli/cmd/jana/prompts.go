package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/term"
	"golang.org/x/text/width"
)

func interactivePrompts() bool {
	return os.Getenv("TERM") != "dumb" && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}
func tint(code, text string) string {
	if os.Getenv("NO_COLOR") != "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func promptCells(value []rune) int {
	cells := 0
	for _, r := range value {
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			cells += 2
		default:
			cells++
		}
	}
	return cells
}
func promptClip(value string, limit int) string {
	if limit < 1 {
		return ""
	}
	runes := []rune(clean(value))
	if promptCells(runes) <= limit {
		return string(runes)
	}
	end := 0
	for end < len(runes) && promptCells(runes[:end+1]) <= limit-1 {
		end++
	}
	return string(runes[:end]) + "…"
}

// Each line stays within the terminal width, so cursor movement never depends
// on wrapping. Completed steps remain in scrollback; only the active step redraws.
type promptFrame struct{ lines int }

func promptSize() (int, int) {
	w, h, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || w < 12 {
		w = 80
	}
	if h < 6 {
		h = 24
	}
	return w, h
}
func (f *promptFrame) draw(lines []string) {
	if f.lines > 0 {
		fmt.Fprint(os.Stderr, "\r")
		if f.lines > 1 {
			fmt.Fprintf(os.Stderr, "\x1b[%dA", f.lines-1)
		}
		fmt.Fprint(os.Stderr, "\x1b[J")
	}
	fmt.Fprint(os.Stderr, strings.Join(lines, "\r\n"))
	f.lines = len(lines)
}
func (f *promptFrame) finish(label, value string, canceled bool) {
	w, _ := promptSize()
	marker, color := "◆", "36"
	if canceled {
		marker, color = "■", "33"
	}
	f.draw([]string{tint(color, marker) + " " + promptClip(label, w-4), tint("2", "│  "+promptClip(value, w-4))})
	fmt.Fprint(os.Stderr, "\r\n")
	f.lines = 0
}

// Key decoding polls stdin so idle prompts remain cancellable without blocked
// reader goroutines. Escape sequences have a bounded wait (including lone Esc).
func promptKey(ctx context.Context, r *bufio.Reader) (string, error) {
	for r.Buffered() == 0 {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if termInputReady(int(os.Stdin.Fd())) {
			break
		}
	}
	b, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	if b == 3 {
		return "", context.Canceled
	}
	if b == 4 {
		return "eof", nil
	}
	if b == 27 {
		sequence := []byte{b}
		deadline := time.Now().Add(150 * time.Millisecond)
		for len(sequence) < 16 && time.Now().Before(deadline) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if r.Buffered() == 0 && !termInputReady(int(os.Stdin.Fd())) {
				continue
			}
			next, e := r.ReadByte()
			if e != nil {
				return "", e
			}
			sequence = append(sequence, next)
			if len(sequence) == 2 && next != '[' && next != 'O' {
				break
			}
			if len(sequence) > 2 && ((next >= 'A' && next <= 'Z') || next == '~') {
				break
			}
		}
		switch string(sequence) {
		case "\x1b[A", "\x1bOA":
			return "up", nil
		case "\x1b[B", "\x1bOB":
			return "down", nil
		case "\x1b[C", "\x1bOC":
			return "right", nil
		case "\x1b[D", "\x1bOD":
			return "left", nil
		case "\x1b[H", "\x1bOH", "\x1b[1~", "\x1b[7~":
			return "home", nil
		case "\x1b[F", "\x1bOF", "\x1b[4~", "\x1b[8~":
			return "end", nil
		case "\x1b[3~":
			return "delete", nil
		case "\x1b[5~":
			return "pageup", nil
		case "\x1b[6~":
			return "pagedown", nil
		case "\x1b":
			return "", context.Canceled
		default:
			return "ignore", nil
		}
	}
	switch b {
	case '\r', '\n':
		return "enter", nil
	case 127, 8:
		return "backspace", nil
	case 1:
		return "home", nil
	case 5:
		return "end", nil
	case 21:
		return "clear", nil
	case 11:
		return "kill", nil
	}
	if b < 32 {
		return "ignore", nil
	}
	bytes := []byte{b}
	for !utf8.FullRune(bytes) {
		for r.Buffered() == 0 && !termInputReady(int(os.Stdin.Fd())) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		next, e := r.ReadByte()
		if e != nil {
			return "", e
		}
		bytes = append(bytes, next)
	}
	value, _ := utf8.DecodeRune(bytes)
	if unicode.IsControl(value) {
		return "ignore", nil
	}
	// Prefix distinguishes literal text from named keys (e.g. the letter 'k').
	return "text:" + string(value), nil
}
func selectPrompt(ctx context.Context, r *bufio.Reader, label string, values []string, selected int) (result int, err error) {
	original, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return 0, err
	}
	defer term.Restore(int(os.Stdin.Fd()), original)
	fmt.Fprint(os.Stderr, "\x1b[?25l")
	defer fmt.Fprint(os.Stderr, "\x1b[?25h")
	frame := promptFrame{}
	defer func() {
		if err != nil {
			frame.finish(label, "Отменено", true)
		}
	}()
	for {
		w, h := promptSize()
		count := min(len(values), max(1, min(8, h-5)))
		start := max(0, min(selected-count/2, len(values)-count))
		lines := []string{tint("36", "◆") + " " + promptClip(label, w-4), tint("2", "│")}
		for i := start; i < start+count; i++ {
			prefix := "│  ○ "
			if i == selected {
				prefix = "│  ● "
				lines = append(lines, tint("36", prefix+promptClip(values[i], w-6)))
			} else {
				lines = append(lines, tint("2", prefix+promptClip(values[i], w-6)))
			}
		}
		help := "↑/↓ — выбор · Enter — подтвердить"
		if len(values) > count {
			help = fmt.Sprintf("%d/%d · %s", selected+1, len(values), help)
		}
		lines = append(lines, tint("2", "│  "+promptClip(help, w-4)))
		frame.draw(lines)
		key, e := promptKey(ctx, r)
		if e != nil {
			return 0, e
		}
		switch key {
		case "up", "text:k":
			selected = (selected + len(values) - 1) % len(values)
		case "down", "text:j":
			selected = (selected + 1) % len(values)
		case "pageup":
			selected = max(0, selected-count)
		case "pagedown":
			selected = min(len(values)-1, selected+count)
		case "home":
			selected = 0
		case "end":
			selected = len(values) - 1
		case "eof":
			return 0, io.EOF
		case "enter":
			frame.finish(label, values[selected], false)
			return selected, nil
		}
	}
}
func textPrompt(ctx context.Context, r *bufio.Reader, label, current string, password bool) (result string, err error) {
	original, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	defer term.Restore(int(os.Stdin.Fd()), original)
	frame := promptFrame{}
	defer func() {
		if err != nil {
			frame.finish(label, "Отменено", true)
		}
	}()
	value := []rune{}
	cursor := 0
	for {
		w, _ := promptSize()
		hint := "Введите значение"
		if current != "" {
			hint = current
		}
		if password {
			hint = "Введите пароль"
			if current != "" {
				hint = "Enter — оставить сохранённый пароль"
			}
		}
		shown := value
		if password {
			shown = []rune(strings.Repeat("•", len(value)))
		}
		room := max(1, w-5)
		start := 0
		for start < cursor && promptCells(shown[start:cursor]) > room-1 {
			start++
		}
		end := start
		for end < len(shown) && promptCells(shown[start:end+1]) <= room {
			end++
		}
		line := string(shown[start:end])
		if len(value) == 0 {
			line = tint("2", promptClip(hint, room))
		}
		frame.draw([]string{tint("36", "◇") + " " + promptClip(label, w-4), tint("36", "│") + "  " + line})
		fmt.Fprintf(os.Stderr, "\r\x1b[%dC", 3+promptCells(shown[start:cursor]))
		key, e := promptKey(ctx, r)
		if e != nil {
			return "", e
		}
		switch key {
		case "enter":
			result = string(value)
			if !password {
				result = strings.TrimSpace(result)
			}
			if result == "" {
				result = current
			}
			summary := result
			if password {
				summary = "Не задан"
				if result != "" {
					summary = "Пароль сохранён"
				}
			}
			frame.finish(label, summary, false)
			return result, nil
		case "eof":
			if len(value) == 0 {
				return "", io.EOF
			}
		case "left":
			cursor = max(0, cursor-1)
		case "right":
			cursor = min(len(value), cursor+1)
		case "home":
			cursor = 0
		case "end":
			cursor = len(value)
		case "clear":
			value = nil
			cursor = 0
		case "kill":
			value = value[:cursor]
		case "backspace":
			if cursor > 0 {
				value = append(value[:cursor-1], value[cursor:]...)
				cursor--
			}
		case "delete":
			if cursor < len(value) {
				value = append(value[:cursor], value[cursor+1:]...)
			}
		default:
			if strings.HasPrefix(key, "text:") {
				character := []rune(strings.TrimPrefix(key, "text:"))[0]
				value = append(value, 0)
				copy(value[cursor+1:], value[cursor:])
				value[cursor] = character
				cursor++
			}
		}
	}
}
