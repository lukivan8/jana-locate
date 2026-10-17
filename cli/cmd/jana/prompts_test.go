package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestPromptPTYChild(t *testing.T) {
	if os.Getenv("JANA_PROMPT_TEST_CHILD") != "1" {
		return
	}
	reader := bufio.NewReader(os.Stdin)
	ctx := context.Background()
	out := map[string]any{}
	var err error
	switch os.Getenv("JANA_PROMPT_SCENARIO") {
	case "wizard":
		out["host"], err = prompt(ctx, reader, "Хост", "localhost", false)
		if err == nil {
			out["table"], err = choose(ctx, reader, "Таблица", []string{"public.address_book", "public.orders", "public.other"}, "public.orders")
		}
		if err == nil {
			out["column"], err = choose(ctx, reader, "Колонка адреса", []string{"address", "delivery_address"}, "delivery_address")
		}
		if err == nil {
			out["password"], err = prompt(ctx, reader, "Пароль", "saved-secret", true)
		}
	case "long":
		values := make([]string, 40)
		for i := range values {
			values[i] = fmt.Sprintf("table_%02d", i)
		}
		out["selected"], err = choose(ctx, reader, "Таблица", values, "table_25")
	case "cancel-select":
		out["selected"], err = choose(ctx, reader, "Таблица", []string{"one", "two"})
	default:
		out["password"], err = prompt(ctx, reader, "Пароль", "saved-secret", true)
	}
	if err != nil {
		out["error"] = err.Error()
	}
	json.NewEncoder(os.Stdout).Encode(out)
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
func TestInteractivePromptsThroughPTY(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("PTY acceptance requires Python 3")
	}
	cmd := exec.Command(python, "../../tests/terminal.py", os.Args[0])
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PTY workflow failed: %v\n%s", err, b)
	}
	t.Log(string(b))
}
