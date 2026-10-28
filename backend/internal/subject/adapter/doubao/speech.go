// Package doubao adapts the shared speech protocol to Subject-owned voice jobs.
package doubao

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

// Client converts private job evidence using the one shared HTTP implementation.
type Client struct{ provider operation.GenerationProvider }

// NewClient retains the shared fixed-origin, no-redirect transport.
func NewClient(client *http.Client) *Client { return NewSpeech(providerhttp.NewClient(client)) }

// NewSpeech injects the existing provider protocol without another HTTP stack.
func NewSpeech(provider operation.GenerationProvider) *Client { return &Client{provider: provider} }

// Synthesize converts one completed shared response to an immutable audio receipt.
func (c *Client) Synthesize(ctx context.Context, in application.VoiceSynthesisInput, key string) (application.VoiceSynthesisResult, error) {
	var config catalog.ProviderConfig
	if c == nil || c.provider == nil || json.Unmarshal(in.Config, &config) != nil || config.Validate() != nil || config.AdapterKey != "volcengine_speech" || in.RequestID == uuid.Nil || in.ResourceID != "seed-tts-2.0" {
		return application.VoiceSynthesisResult{}, application.ErrInvalidVoiceJob
	}
	result, err := c.provider.Submit(ctx, operation.GenerationRequest{StrictSpeechTerminal: true, Config: config, ModelID: in.ResourceID, Capability: "audio.generate", Mode: "text2audio", Prompt: in.Text, Params: map[string]any{"speaker": in.VoiceKey, "speech_rate": in.SpeechRate}, OutputCount: 1, ProviderRequestID: in.RequestID.String()}, key)
	if err != nil {
		return application.VoiceSynthesisResult{}, application.ErrVoiceExecutionUnknown
	}
	if result.Outcome == "rejected" || result.Outcome == "not_submitted" {
		return application.VoiceSynthesisResult{}, application.ErrVoiceProviderRejected
	}
	if result.ProviderTerminalCode != 20000000 || result.Outcome != "completed" || len(result.Audio) != 1 || result.Audio[0].MediaType != "audio/mpeg" || len(result.Audio[0].Bytes) == 0 || len(result.Audio[0].Bytes) > 8<<20 {
		return application.VoiceSynthesisResult{}, application.ErrVoiceExecutionUnknown
	}
	return application.VoiceSynthesisResult{Audio: result.Audio[0].Bytes, MIMEType: result.Audio[0].MediaType, TerminalCode: result.ProviderTerminalCode, Usage: result.Usage, ProviderLogID: result.ProviderLogID}, nil
}
