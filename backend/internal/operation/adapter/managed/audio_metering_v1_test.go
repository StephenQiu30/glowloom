package managed

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

func TestAudioMeteringSelectsVersionedProtocol(t *testing.T) {
	for _, tc := range []struct {
		strategy string
		strict   bool
	}{{"character_count_v1", true}, {"", false}} {
		loaded := application.WorkflowOperation{Operation: operationdomain.Operation{Capability: "audio.generate", Mode: "text2audio", Params: json.RawMessage(`{}`), QuoteDetail: json.RawMessage(`{"pricing_strategy":"` + tc.strategy + `"}`)}}
		request, err := (&Activities{}).request(t.Context(), loaded, catalogdomain.ProviderConfig{})
		require.NoError(t, err)
		require.Equal(t, tc.strict, request.StrictSpeechTerminal)
		require.Equal(t, tc.strict, request.RequireSpeechTerminalUsage)
	}
}

func TestNormalizeVersionedAudioRequiresConclusiveQuantity(t *testing.T) {
	for _, tc := range []struct {
		strategy, raw string
		code          int
		ok            bool
	}{
		{"original_duration_v1", `{"original_duration":60.0000000000000000001,"duration":30}`, 0, true},
		{"original_duration_v1", `{"duration":30}`, 0, false},
		{"original_duration_v1", `{"original_duration":120.0000000000000000001}`, 0, false},
		{"character_count_v1", `{"text_words":8}`, 20000000, true},
		{"character_count_v1", `{"text_words":8}`, 0, false},
		{"character_count_v1", `{}`, 20000000, false},
	} {
		usage := map[string]any{}
		err := normalizeVersionedAudioUsage(tc.strategy, application.GenerationResult{ProviderTerminalCode: tc.code, Usage: json.RawMessage(tc.raw)}, usage)
		if !tc.ok {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		if tc.strategy == "original_duration_v1" {
			require.Equal(t, json.Number("60.0000000000000000001"), usage["original_duration_seconds"])
			require.NotContains(t, usage, "billed_duration_ms")
		} else {
			require.Equal(t, int64(8), usage["character_count"])
		}
	}
}

func TestNormalizedTokenAliasesMustAgree(t *testing.T) {
	for _, raw := range []string{`{"prompt_tokens":8,"input_tokens":9}`, `{"completion_tokens":4,"output_tokens":5}`, `{"input_tokens":null}`, `{"cached_input_tokens":-1}`} {
		require.Error(t, normalizeTokenUsage(json.RawMessage(raw), map[string]any{}), raw)
	}
	usage := map[string]any{}
	require.NoError(t, normalizeTokenUsage(json.RawMessage(`{"prompt_tokens":8,"input_tokens":8,"completion_tokens":4,"output_tokens":4,"cached_input_tokens":2}`), usage))
	require.Equal(t, int64(8), usage["input_tokens"])
	require.Equal(t, int64(4), usage["output_tokens"])
	require.Equal(t, int64(2), usage["cached_input_tokens"])
}
