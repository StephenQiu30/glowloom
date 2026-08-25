package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func TestPresentBibleIncludesGenerationError(t *testing.T) {
	result := presentBible(domain.Bible{
		ID:              "bible-1",
		Error:           json.RawMessage(`{"code":"codex_unavailable","summary":"model request failed","retryable":true}`),
		ReviewDecisions: map[string]string{},
	})

	errorValue, ok := result["generation_error"].(map[string]any)
	if !ok {
		t.Fatal("generation_error must be an object")
	}
	if errorValue["code"] != "codex_unavailable" || errorValue["summary"] != "model request failed" {
		t.Fatalf("unexpected generation_error: %#v", errorValue)
	}
}
