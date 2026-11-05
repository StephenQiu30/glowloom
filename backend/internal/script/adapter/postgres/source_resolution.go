package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// CancelledPublication proves that one original command was never published
// and its actual I/O plus every original private object has been retired. It is
// read-only and never resumes publication or invents an absence proof.
func (s *SourceStore) CancelledPublication(ctx context.Context, actor identity.Principal, project, key uuid.UUID, hash string) (bool, error) {
	cancelled := false
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspace.ProjectContentAccess) error {
		prior, err := findCommand(tx, actor, project, key, hash)
		if err != nil {
			return err
		}
		if prior == nil || prior.Receipt != nil {
			return nil
		}
		record, err := s.sourceWriteRecord(tx, actor, project, sourceIntentID(prior.Plan), true)
		if err != nil {
			return err
		}
		if record.Plan.RequestHash != hash || record.Plan.Command.Key != key || record.Plan.ActorID != actor.ID {
			return application.ErrIdempotencyConflict
		}
		if record.View.Status != domain.CommandStatusCancelled || !record.View.CancellationRequested || record.View.NeedsReconciliation || record.OwnerID != nil || record.IOState != domain.CommandIOStateIdle && record.IOState != domain.CommandIOStateEnded {
			return nil
		}
		for _, object := range record.Objects {
			if !object.Removed {
				return nil
			}
		}
		ids := make([]uuid.UUID, 0, len(record.Plan.NewSources))
		for _, source := range record.Plan.NewSources {
			ids = append(ids, source.ID)
		}
		var n int64
		if len(ids) > 0 {
			if err := tx.Table("script.script_source").Where("id IN ?", ids).Count(&n).Error; err != nil {
				return err
			}
			if n != 0 {
				return application.ErrNeedsReconciliation
			}
		}
		if !record.Plan.Reuse {
			if err := tx.Table("script.script_version").Where("id = ?", record.Plan.Version.ID).Count(&n).Error; err != nil {
				return err
			}
			if n != 0 {
				return application.ErrNeedsReconciliation
			}
		}
		cancelled = true
		return nil
	})
	return cancelled, err
}
