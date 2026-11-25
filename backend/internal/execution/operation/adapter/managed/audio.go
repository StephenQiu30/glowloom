package managed

import (
	"context"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/domain"
)

// AudioReader is the legacy music consumer port; Media owns canonical encoding.
type AudioReader interface {
	ReadAudio(context.Context, catalog.ProviderConfig, string, string) ([]byte, int64, error)
}
