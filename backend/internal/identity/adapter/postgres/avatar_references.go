package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// AvatarReferences protects retained profile bindings, including soft-deleted accounts.
// The media owner holds the original asset lock before asking this read-only port.
type AvatarReferences struct{ tx *gorm.DB }

// NewAvatarReferences binds reference inspection to the media owner's transaction.
func NewAvatarReferences(tx *gorm.DB) *AvatarReferences { return &AvatarReferences{tx: tx} }

// HasAvatarReference returns no account fields and grants no content access.
func (r *AvatarReferences) HasAvatarReference(ctx context.Context, asset uuid.UUID) (bool, error) {
	if r == nil || r.tx == nil || r.tx.Statement == nil || asset == uuid.Nil {
		return false, application.ErrAvatarUnavailable
	}
	if _, ok := r.tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return false, application.ErrAvatarUnavailable
	}
	var row struct{ ID uuid.UUID }
	read := r.tx.WithContext(ctx).Table(`identity."user"`).Select("id").Where("avatar_asset_id = ?", asset).Limit(1).Find(&row)
	return read.RowsAffected > 0, read.Error
}
