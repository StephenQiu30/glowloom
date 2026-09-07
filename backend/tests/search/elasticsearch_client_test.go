package search_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	searches "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/elasticsearch"
)

func TestElasticsearchClientUsesBasicAuthAndDoesNotRetry(t *testing.T) {
	var attempts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attempts.Add(1)
		username, password, ok := request.BasicAuth()
		if !ok || username != "search-reader" || password != "test-secret" {
			t.Errorf("elasticsearch request Basic Auth = %q/%q present=%t", username, password, ok)
		}
		writer.Header().Set("X-Elastic-Product", "Elasticsearch")
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	index, err := searches.New(searches.Config{
		Addresses: []string{server.URL}, Username: "search-reader", Password: "test-secret",
		ScriptAlias: formalScriptSearchAlias, StoryGraphAlias: formalStoryGraphSearchAlias,
	})
	if err != nil {
		t.Fatalf("open elasticsearch client: %v", err)
	}
	if err = index.Ping(context.Background()); err == nil {
		t.Fatal("elasticsearch service-unavailable response passed Ping")
	}
	if attempts.Load() != 1 {
		t.Fatalf("elasticsearch client retried a fenced request: attempts=%d", attempts.Load())
	}
}
