package normalize

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GLiNER offsets are Unicode codepoints in the working text, not UTF-8 bytes.
type Entity struct {
	Start int     `json:"start"`
	End   int     `json:"end"`
	Text  string  `json:"text"`
	Label string  `json:"label"`
	Score float64 `json:"score"`
}
type Extractor interface {
	Extract(context.Context, []string) ([][]Entity, error)
}
type GLiNER struct {
	URL       string
	HTTP      *http.Client
	ChunkSize int
}

func (g *GLiNER) Ready(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(g.URL, "/")+"/readyz", nil)
	if err != nil {
		return err
	}
	c := g.HTTP
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Second}
	}
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("GLiNER readiness HTTP %d", res.StatusCode)
	}
	return nil
}
func (g *GLiNER) Extract(ctx context.Context, texts []string) ([][]Entity, error) {
	out := make([][]Entity, 0, len(texts))
	chunk := g.ChunkSize
	if chunk < 1 {
		chunk = 32
	}
	client := g.HTTP
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	for start := 0; start < len(texts); start += chunk {
		end := start + chunk
		if end > len(texts) {
			end = len(texts)
		}
		body, _ := json.Marshal(struct {
			Texts []string `json:"texts"`
		}{texts[start:end]})
		req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(g.URL, "/")+"/v1/entities", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			return nil, fmt.Errorf("GLiNER HTTP %d", res.StatusCode)
		}
		var response struct {
			Results [][]Entity `json:"results"`
		}
		err = json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&response)
		res.Body.Close()
		if err != nil {
			return nil, err
		}
		if len(response.Results) != end-start {
			return nil, fmt.Errorf("GLiNER result count mismatch")
		}
		for i, entities := range response.Results {
			runes := []rune(texts[start+i])
			for _, e := range entities {
				if e.Start < 0 || e.End <= e.Start || e.End > len(runes) || string(runes[e.Start:e.End]) != e.Text || e.Score < 0 || e.Score > 1 {
					return nil, fmt.Errorf("GLiNER invalid entity span or score")
				}
			}
		}
		out = append(out, response.Results...)
	}
	return out, nil
}
