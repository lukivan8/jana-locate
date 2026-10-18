package app

import (
	"context"
	"encoding/json"
	"example.com/jana-locate-cli/internal/api"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContextualResponseBeyondLegacyLimit(t *testing.T) {
	raw := "Астана"
	evidence := strings.Repeat("x", 17<<20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := emptyResult(0, &raw)
		result.Id = "0"
		details := map[string]interface{}{"evidence": evidence}
		result.NormalizationDetails = &details
		json.NewEncoder(w).Encode(map[string]any{"results": []api.Result{result}})
	}))
	defer server.Close()
	normalizer, err := NewNormalizer(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := normalizer.Batch(context.Background(), []api.Row{{Position: 0, SourceId: "source", RawAddress: &raw}})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Result.Id != "source" || rows[0].Result.NormalizationDetails == nil {
		t.Fatal("lost result or original identity")
	}
}
