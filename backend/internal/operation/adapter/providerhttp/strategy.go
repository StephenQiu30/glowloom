package providerhttp

import (
	"context"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// ProtocolStrategy encapsulates protocol-specific submission, polling,
// and discovery behaviors for a provider family (Strategy Pattern).
type ProtocolStrategy interface {
	Name() string
	Submit(ctx context.Context, c *Client, input application.GenerationRequest, key string) (application.GenerationResult, error)
	Query(ctx context.Context, c *Client, config catalogdomain.ProviderConfig, taskID, key string) (application.GenerationResult, error)
	Probe(ctx context.Context, c *Client, config catalogdomain.ProviderConfig, key string) ProbeResult
	ProbeModelDefaultParams(capability string) map[string]any
}
