package client

import (
	"context"
	"encoding/json"
	"github.com/lukivan8/jana-address/pkg/api"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestContextualEvidenceBeyondLegacyResponseLimit(t *testing.T) {
	evidence := strings.Repeat("x", 17<<20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"id": "a", "raw_address": "Астана", "normalization_details": map[string]any{"evidence": evidence}}}})
	}))
	defer server.Close()
	c := New(server.URL)
	if c.HTTP.Timeout != 300*time.Second {
		t.Fatal("timeout shorter than batch contract")
	}
	out, err := c.Normalize(context.Background(), api.NormalizeRequest{Records: []api.InputRecord{{Id: "a", RawAddress: "Астана"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 1 || out.Results[0].NormalizationDetails == nil || len((*out.Results[0].NormalizationDetails)["evidence"].(string)) != len(evidence) {
		t.Fatal("truncated evidence")
	}
}
