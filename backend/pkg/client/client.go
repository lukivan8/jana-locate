// Package client is the public Go SDK. It depends only on the standard library
// and pkg/api; it does not import backend internals.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lukivan8/jana-address/pkg/api"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}
type HTTPError struct {
	Status int
	Detail api.Error
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("jana-address: HTTP %d: %s: %s", e.Status, e.Detail.Code, e.Detail.Message)
}
func New(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 60 * time.Second}}
}
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	transport := c.HTTP
	if transport == nil {
		transport = &http.Client{Timeout: 60 * time.Second}
	}
	res, err := transport.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	d := json.NewDecoder(io.LimitReader(res.Body, 16<<20))
	d.UseNumber()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		e := &HTTPError{Status: res.StatusCode}
		_ = d.Decode(&e.Detail)
		return e
	}
	return d.Decode(out)
}
func (c *Client) Normalize(ctx context.Context, in api.NormalizeRequest) (api.NormalizeResponse, error) {
	var out api.NormalizeResponse
	err := c.do(ctx, "POST", "/v1/normalize", in, &out)
	return out, err
}
func (c *Client) Catalog(ctx context.Context) (api.CatalogResponse, error) {
	var out api.CatalogResponse
	err := c.do(ctx, "GET", "/v1/catalog", nil, &out)
	return out, err
}
func (c *Client) Confirm(ctx context.Context, in api.ConfirmationRequest) (api.CatalogEntry, error) {
	var out api.CatalogEntry
	err := c.do(ctx, "POST", "/v1/catalog/confirmations", in, &out)
	return out, err
}
