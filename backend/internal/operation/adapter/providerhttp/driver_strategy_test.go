package providerhttp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestDriverRegistryCompleteness(t *testing.T) {
	expectedAdapters := []struct {
		adapter  string
		expected string
	}{
		{"openai", "*providerhttp.OpenAICompatibleDriver"},
		{"deepseek", "*providerhttp.OpenAICompatibleDriver"},
		{"codex", "*providerhttp.OpenAICompatibleDriver"},
		{"openrouter", "*providerhttp.OpenAICompatibleDriver"},
		{"volcengine_ark", "*providerhttp.VolcengineArkDriver"},
		{"byteplus", "*providerhttp.VolcengineArkDriver"},
		{"volcengine_speech", "*providerhttp.VolcengineSpeechDriver"},
		{"volcengine_music", "*providerhttp.VolcengineMusicDriver"},
		{"volcengine_avatar", "*providerhttp.VolcengineVisualDriver"},
		{"volcengine_motion", "*providerhttp.VolcengineVisualDriver"},
	}

	for _, tc := range expectedAdapters {
		t.Run(tc.adapter, func(t *testing.T) {
			driver, ok := GetDriver(tc.adapter)
			require.True(t, ok, "adapter %s must be registered", tc.adapter)
			require.NotNil(t, driver)
		})
	}
}

func TestClientUsesStrategyDriverAndSupportsCustomOverride(t *testing.T) {
	client := NewClient(nil)

	// An unregistered adapter key should be rejected safely without crashing.
	unknownCfg := catalogdomain.ProviderConfig{
		AdapterKey:     "unknown_future_provider",
		BaseURL:        "https://api.future.ai/v1",
		TimeoutSeconds: 30,
		Capabilities:   []string{"text"},
	}
	res, err := client.Submit(context.Background(), application.GenerationRequest{
		Config:      unknownCfg,
		Capability:  "text.generate",
		ModelID:     "future-1",
		Prompt:      "hi",
		OutputCount: 1,
	}, "test-key")
	require.NoError(t, err)
	require.Equal(t, "not_submitted", res.Outcome)

	// Inject a custom driver dynamically for this client instance.
	mockCalled := false
	mockDriver := &mockTestDriver{
		onSubmit: func(ctx context.Context, c *Client, input application.GenerationRequest, key string) (application.GenerationResult, error) {
			mockCalled = true
			return application.GenerationResult{Outcome: "completed", State: "succeeded", Text: "mock success"}, nil
		},
	}
	client.SetDriver("unknown_future_provider", mockDriver)

	res, err = client.Submit(context.Background(), application.GenerationRequest{
		Config:      unknownCfg,
		Capability:  "text.generate",
		ModelID:     "future-1",
		Prompt:      "hi",
		OutputCount: 1,
	}, "test-key")
	require.NoError(t, err)
	require.True(t, mockCalled)
	require.Equal(t, "completed", res.Outcome)
	require.Equal(t, "mock success", res.Text)
}

func TestProbeModelDelegatesDefaultParamsToStrategyDriver(t *testing.T) {
	// Verify that Ark provides 2K image size, OpenAI provides 1024x1024, and Speech provides speaker
	arkDriver, ok := GetDriver("volcengine_ark")
	require.True(t, ok)
	require.Equal(t, "2K", arkDriver.ProbeModelDefaultParams("image.generate")["size"])

	openaiDriver, ok := GetDriver("openai")
	require.True(t, ok)
	require.Equal(t, "1024x1024", openaiDriver.ProbeModelDefaultParams("image.generate")["size"])

	speechDriver, ok := GetDriver("volcengine_speech")
	require.True(t, ok)
	require.Equal(t, "zh_female_vv_uranus_bigtts", speechDriver.ProbeModelDefaultParams("audio.generate")["speaker"])
}

type mockTestDriver struct {
	onSubmit func(ctx context.Context, c *Client, input application.GenerationRequest, key string) (application.GenerationResult, error)
}

func (m *mockTestDriver) Submit(ctx context.Context, c *Client, input application.GenerationRequest, key string) (application.GenerationResult, error) {
	if m.onSubmit != nil {
		return m.onSubmit(ctx, c, input, key)
	}
	return application.GenerationResult{Outcome: "not_submitted"}, nil
}

func (m *mockTestDriver) Query(ctx context.Context, c *Client, config catalogdomain.ProviderConfig, taskID, key string) (application.GenerationResult, error) {
	return unknownResult(), nil
}

func (m *mockTestDriver) Probe(ctx context.Context, c *Client, config catalogdomain.ProviderConfig, key string) ProbeResult {
	return ProbeResult{Auth: "ok", Adapter: "ok", Discovery: "ok"}
}

func (m *mockTestDriver) ProbeModelDefaultParams(capability string) map[string]any {
	return nil
}

var _ ProtocolDriver = (*mockTestDriver)(nil)
