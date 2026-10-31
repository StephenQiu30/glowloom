package providerhttp

import (
	"context"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// ProbeResult contains independent safe result categories and catalog candidates.
type ProbeResult struct {
	Auth, Adapter, Discovery string
	Candidates               []catalogapp.ModelCandidate
}

// Probe performs read-only authentication and discovery; it cannot generate output.
func (c *Client) Probe(ctx context.Context, config catalogdomain.ProviderConfig, key string) ProbeResult {
	driver, ok := c.drivers[config.AdapterKey]
	if !ok {
		return ProbeResult{Auth: "unknown", Adapter: "unsupported", Discovery: "unsupported", Candidates: []catalogapp.ModelCandidate{}}
	}
	return driver.Probe(ctx, c, config, key)
}
