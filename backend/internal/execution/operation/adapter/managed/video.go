package managed

import (
	"context"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/domain"
)

// VideoReader reads approved legacy content; processing belongs to the injected owner.
// Owned activities transfer raw bytes through GeneratedFiles and ArtifactDelivery.
type VideoReader interface {
	Read(context.Context, catalog.ProviderConfig, string, string, string) ([]byte, int64, error)
}
