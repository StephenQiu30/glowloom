package postgres

import (
	"context"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// CommitYjsCommands preserves atomicity for both independent callers and a
// transaction-bound Agent receipt adapter. No runtime RPC occurs under the lock.
func (s *Store) CommitYjsCommands(ctx context.Context, a identity.Principal, in application.PreparedYjsCommands) (application.YjsReceipt, error) {
	var receipt application.YjsReceipt
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bound := *s
		bound.db = tx
		if in.Initialization != nil {
			if _, err := bound.InitializeYjs(ctx, a, in.Base, *in.Initialization); err != nil {
				return err
			}
		}
		var err error
		receipt, err = bound.CommitYjs(ctx, a, in.Base, in.Update, &in.Candidate, "")
		return err
	})
	return receipt, err
}
