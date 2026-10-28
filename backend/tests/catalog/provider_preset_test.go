package catalog_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestBuiltinPresetsAreIsolatedAndContainNoCredentials(t *testing.T) {
	presets := domain.BuiltinPresets()
	require.Len(t, presets, 5)
	for _, p := range presets {
		require.NoError(t, p.Config.Validate())
	}
	presets[0].Config.Capabilities[0] = "tampered"
	require.Equal(t, "video", domain.BuiltinPresets()[0].Config.Capabilities[0])
}

func TestPresetImportRejectsUnknownFieldsAndUnsafeEndpoints(t *testing.T) {
	valid := `{"schema_version":1,"name":"OpenRouter","config":{"adapter_key":"openrouter","base_url":"https://openrouter.ai/api/v1","region":"overseas","capabilities":["text","image","video"],"timeout_seconds":120,"concurrency_limit":2,"rate_limit_per_min":30}}`
	p, err := domain.ParsePreset([]byte(valid))
	require.NoError(t, err)
	require.NotEmpty(t, p.Digest())
	for _, extra := range []string{`"api_key":"secret",`, `"headers":{"Authorization":"secret"},`, `"schema_version":2,`} {
		_, err := domain.ParsePreset([]byte("{" + extra + valid[1:]))
		require.Error(t, err)
	}
	for _, endpoint := range []string{"http://openrouter.ai/api/v1", "https://openrouter.ai.evil.test/api/v1", "https://user:password@openrouter.ai/api/v1", "https://openrouter.ai/api/v1?token=x", "https://127.0.0.1/api/v1"} {
		p.Config.BaseURL = endpoint
		require.Error(t, p.Config.Validate(), endpoint)
	}
}

func TestPresetUpgradePreservesOverridesAndReportsConflicts(t *testing.T) {
	old := domain.BuiltinPresets()[1].Config
	next := old
	next.TimeoutSeconds = 180
	next.RateLimitPerMin = 60
	current := old
	current.TimeoutSeconds = 90
	merged, conflicts := domain.MergePreset(old, next, current)
	require.Equal(t, 90, merged.TimeoutSeconds)
	require.Equal(t, 60, merged.RateLimitPerMin)
	require.Equal(t, []string{"timeout_seconds"}, conflicts)
	encoded, err := json.Marshal(merged)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "api_key")
}
