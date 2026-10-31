package canvasfixture

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// RestoreVersion uses the real Yjs runtime and owner transaction, just like an accepted edit.
func (s *Service) RestoreVersion(ctx context.Context, actor identity.Principal, id, version uuid.UUID) (application.YjsRestoreResult, error) {
	base, err := s.shared.Load(ctx, actor, id)
	if err != nil {
		return application.YjsRestoreResult{}, err
	}
	return s.shared.RestoreVersion(ctx, actor, id, application.YjsRestore{ID: uuid.New(), SessionID: ClientID, VersionID: version, Epoch: base.Epoch, CommitSeq: base.CommitSeq})
}
