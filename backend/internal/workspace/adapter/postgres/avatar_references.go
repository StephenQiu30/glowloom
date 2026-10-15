package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// TeamAvatarReferences protects retained team bindings independently of the uploader.
// Media retains the original asset lock, serializing this check with avatar validation.
type TeamAvatarReferences struct{ tx *gorm.DB }

// NewTeamAvatarReferences binds reference inspection to the media owner's transaction.
func NewTeamAvatarReferences(tx *gorm.DB) *TeamAvatarReferences { return &TeamAvatarReferences{tx: tx} }

// HasAvatarReference includes archived and deleted teams without revealing team data.
func (r *TeamAvatarReferences) HasAvatarReference(ctx context.Context, asset uuid.UUID) (bool, error) {
	if r == nil || r.tx == nil || r.tx.Statement == nil || asset == uuid.Nil {
		return false, application.ErrProjectDependencyUnavailable
	}
	if _, ok := r.tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return false, application.ErrProjectDependencyUnavailable
	}
	var used bool
	err := r.tx.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM workspace.team WHERE avatar_asset_id=?)`, asset).Scan(&used).Error
	return used, err
}
