package providerhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestArkMapsFrozenDisabledThinkingWithoutMutatingParameters(t *testing.T) {
	calls := 0
	client := NewClient(&http.Client{Transport: creationTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.JSONEq(t, `{"type":"disabled"}`, string(body["thinking"]))
		return nil, errors.New("fixture transport outcome unknown")
	})})
	params := map[string]any{"thinking": "disabled"}
	_, err := client.Submit(t.Context(), application.GenerationRequest{Config: creationConfig("volcengine_ark"), ModelID: "doubao-seed-2-1-lite-260915", Capability: "text.generate", Mode: "text_to_text", Prompt: "hello", OutputCount: 1, Params: params}, "fixture-key")
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "disabled", params["thinking"])
}

func TestArkRejectsUnbudgetedThinkingBeforeDispatch(t *testing.T) {
	calls := 0
	client := NewClient(&http.Client{Transport: creationTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("must not dispatch") })})
	for _, value := range []any{"enabled", "auto", map[string]any{"type": "disabled"}, nil} {
		result, err := client.Submit(t.Context(), application.GenerationRequest{Config: creationConfig("volcengine_ark"), ModelID: "doubao-seed-2-1-lite-260915", Capability: "text.generate", Mode: "text_to_text", Prompt: "hello", OutputCount: 1, Params: map[string]any{"thinking": value}}, "fixture-key")
		require.NoError(t, err)
		require.Equal(t, application.ProviderOutcomeNotSubmitted, result.Outcome)
	}
	require.Zero(t, calls)
}
