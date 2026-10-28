package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

// MediaGuard retains every immutable subject binding, including deleted heads
// and former versions. Its caller has already authorized the source's purge.
type MediaGuard struct{ db *gorm.DB }

// NewMediaGuard binds retained subject-reference checks to the media transaction.
func NewMediaGuard(tx *gorm.DB) *MediaGuard { return &MediaGuard{db: tx} }

// HasRetainedMediaReference is global because personal media has no project ID.
func (g *MediaGuard) HasRetainedMediaReference(ctx context.Context, asset uuid.UUID) (bool, error) {
	if g == nil || g.db == nil || g.db.Statement == nil || asset == uuid.Nil {
		return false, application.ErrUnavailable
	}
	if _, ok := g.db.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return false, application.ErrUnavailable
	}
	var found bool
	err := g.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM subject.media_binding WHERE asset_id=?)`, asset).Scan(&found).Error
	return found, err
}
