// Пример HTTP-интеграции без SDK. Запуск из корня: go run examples/http/go/main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type record struct {
	ID         string `json:"id"`
	RawAddress string `json:"raw_address"`
}

func run() error {
	base := os.Getenv("JANA_API_URL")
	if base == "" {
		base = "http://localhost:8080"
	}
	input := struct {
		Records []record `json:"records"`
	}{
		Records: []record{{ID: "order-1", RawAddress: "Астана, проспект Туран 55/6 кв 99"}},
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/v1/normalize", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 300 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, data)
	}
	var output struct {
		Results []record `json:"results"`
	}
	if err := json.Unmarshal(data, &output); err != nil {
		return err
	}
	if len(output.Results) != len(input.Records) {
		return fmt.Errorf("неожиданное количество результатов")
	}
	for i, row := range output.Results {
		if row != input.Records[i] {
			return fmt.Errorf("API не сохранил исходный ID или адрес")
		}
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err != nil {
		return err
	}
	fmt.Println(pretty.String())
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка нормализации:", err)
		os.Exit(1)
	}
}
