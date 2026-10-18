package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lukivan8/jana-address/internal/normalize"
)

func TestUnavailableContextualIs503(t *testing.T) {
	inference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "busy", 503) }))
	defer inference.Close()
	handler := New(nil, nil, normalize.Engine{Contextual: &normalize.ContextualClient{URL: inference.URL}})
	request := httptest.NewRequest(http.MethodPost, "/v1/normalize", strings.NewReader(`{"records":[{"id":"a","raw_address":"Астана"}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 503 || !strings.Contains(response.Body.String(), "normalizer_unavailable") {
		t.Fatalf("failure treated as address result: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/v1/normalize/candidates", strings.NewReader(`{"record":{"id":"a","raw_address":"Астана"}}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 503 {
		t.Fatalf("outage treated as empty candidates: %d %s", response.Code, response.Body.String())
	}

}
