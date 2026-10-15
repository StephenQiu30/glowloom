package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// MediaReferenceGuard reads operation-owned inputs and outputs, including retained history.
type MediaReferenceGuard struct {
	tx     *gorm.DB
	access ProjectAccessFactory
}

// NewMediaReferenceGuard shares the caller's authority and reference transaction.
func NewMediaReferenceGuard(tx *gorm.DB) *MediaReferenceGuard {
	return &MediaReferenceGuard{tx: tx}
}

// HasMediaReferences keeps historical task facts independent of their current visibility.
func (g *MediaReferenceGuard) HasMediaReferences(ctx context.Context, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	if g == nil || g.tx == nil || g.tx.Statement == nil || project == uuid.Nil || asset == uuid.Nil {
		return false, ErrUnavailable
	}
	if _, ok := g.tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return false, fmt.Errorf("operation reference guard requires caller transaction")
	}
	tx := g.tx.WithContext(ctx)
	if err := NewScopedStore(tx, g.access, nil).requireProject(tx, actor, project, false); err != nil {
		return false, err
	}
	var present bool
	if err := tx.Raw(`SELECT
 EXISTS(SELECT 1 FROM operation.operation_input i
   JOIN operation.operation o ON o.id=i.operation_id
   WHERE o.project_id=? AND (i.media_asset_id=? OR i.mask_asset_id=?
     OR (i.ref_type='media_asset' AND i.ref_id=?)))
 OR EXISTS(SELECT 1 FROM operation.operation_output x
   JOIN operation.operation o ON o.id=x.operation_id AND o.project_id=x.project_id
   WHERE o.project_id=? AND x.media_asset_id=?)`, project, asset, asset, asset, project, asset).Scan(&present).Error; err != nil {
		return false, fmt.Errorf("read retained operation media references: %w", err)
	}
	return present, nil
}

// NewScopedMediaReferenceGuard injects the caller-specific request or worker gate.
func NewScopedMediaReferenceGuard(tx *gorm.DB, access ProjectAccessFactory) *MediaReferenceGuard {
	return &MediaReferenceGuard{tx: tx, access: access}
}
