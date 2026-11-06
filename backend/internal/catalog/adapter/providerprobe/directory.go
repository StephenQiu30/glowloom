package providerprobe

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	catalogflow "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/task"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func (a *Activities) readDirectory(ctx context.Context, config catalogdomain.ProviderConfig, key string, started time.Time) catalogflow.AgentTestOutput {
	ids, err := a.client.ReadModelIDs(ctx, config, key)
	result := catalogflow.AgentTestOutput{ElapsedMS: time.Since(started).Milliseconds(), AuthResult: "unknown", AdapterResult: "ok", DiscoveryResult: "unreachable", Result: catalogdomain.TestUnreachable}
	switch {
	case err == nil:
		result.AuthResult, result.DiscoveryResult, result.Result = "ok", "ok", catalogdomain.TestOK
		if len(ids) > 600 {
			result.DiscoveryResult = "partial"
			ids = ids[:600]
		}
		for _, id := range ids {
			result.Candidates = append(result.Candidates, catalogapp.ModelCandidate{ProviderModelID: id, DisplayName: id, Metadata: json.RawMessage("{}")})
		}
	case errors.Is(err, catalogapp.ErrModelDirectoryCredential):
		result.AuthResult, result.DiscoveryResult, result.Result = "auth_failed", "auth_failed", catalogdomain.TestAuthFailed
	case errors.Is(err, catalogapp.ErrModelDirectoryUnsupported):
		result.AuthResult, result.AdapterResult, result.DiscoveryResult, result.Result = "unsupported", "unsupported", "unsupported", catalogdomain.TestUnsupported
	case errors.Is(err, catalogapp.ErrModelDirectoryRateLimited):
		result.DiscoveryResult = "rate_limited"
	}
	return result
}
