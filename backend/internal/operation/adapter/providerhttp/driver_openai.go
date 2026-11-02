package providerhttp

import (
	"context"
	"strings"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// OpenAICompatibleDriver coordinates protocol operations by delegating
// to a concrete ProtocolStrategy (Strategy Pattern).
type OpenAICompatibleDriver struct {
	strategy ProtocolStrategy
}

// NewOpenAICompatibleDriver constructs an OpenAICompatibleDriver backed by a protocol strategy.
func NewOpenAICompatibleDriver(strategy ProtocolStrategy) *OpenAICompatibleDriver {
	return &OpenAICompatibleDriver{strategy: strategy}
}

func init() {
	standardStrategy := &OpenAIStandardStrategy{}
	openrouterStrategy := &OpenRouterStrategy{}
	vercelStrategy := &VercelStrategy{}

	RegisterDriver(NewOpenAICompatibleDriver(standardStrategy), "openai", "deepseek", "codex")
	RegisterDriver(NewOpenAICompatibleDriver(openrouterStrategy), "openrouter")
	RegisterDriver(NewOpenAICompatibleDriver(vercelStrategy), "vercel")
}

func (d *OpenAICompatibleDriver) effectiveStrategy() ProtocolStrategy {
	if d != nil && d.strategy != nil {
		return d.strategy
	}
	return &OpenAIStandardStrategy{}
}

// Submit delegates execution to the concrete strategy.
func (d *OpenAICompatibleDriver) Submit(ctx context.Context, c *Client, input application.GenerationRequest, key string) (application.GenerationResult, error) {
	return d.effectiveStrategy().Submit(ctx, c, input, key)
}

// Query delegates asynchronous task status polling to the concrete strategy.
func (d *OpenAICompatibleDriver) Query(ctx context.Context, c *Client, config catalogdomain.ProviderConfig, taskID, key string) (application.GenerationResult, error) {
	return d.effectiveStrategy().Query(ctx, c, config, taskID, key)
}

// Probe reads credential validity and account-visible model metadata via the concrete strategy.
func (d *OpenAICompatibleDriver) Probe(ctx context.Context, c *Client, config catalogdomain.ProviderConfig, key string) ProbeResult {
	return d.effectiveStrategy().Probe(ctx, c, config, key)
}

// ProbeModelDefaultParams supplies conservative validation parameters via the concrete strategy.
func (d *OpenAICompatibleDriver) ProbeModelDefaultParams(capability string) map[string]any {
	return d.effectiveStrategy().ProbeModelDefaultParams(capability)
}

// GPT Image aliases share the same request contract as named GPT Image models.
func isGPTImage(model string) bool {
	return strings.HasPrefix(model, "gpt-image-") || model == "chatgpt-image-latest"
}
