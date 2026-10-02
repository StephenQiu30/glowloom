package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func verifyCopyProjectCover(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, job domain.ProjectCopyJob, owner application.ProjectCopyCoverOwner) error {
	frozen, err := readFrozenCopyWorkspace(tx, job)
	if err != nil {
		return err
	}
	var actual struct{ CoverAssetID *uuid.UUID }
	read := tx.Raw(`SELECT cover_asset_id FROM workspace.project WHERE id=? AND org_id=? AND status='copying' AND NOT is_delete FOR SHARE`, job.TargetProjectID, job.OrgID).Scan(&actual)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 || !application.SameProjectCover(frozen.Target.CoverAssetID, actual.CoverAssetID) {
		return application.ErrProjectCoverUnavailable
	}
	if actual.CoverAssetID == nil {
		return nil
	}
	if owner == nil {
		return application.ErrProjectDependencyUnavailable
	}
	return owner.Verify(ctx, actor, copyMediaBinding(job), frozen.Target.CoverAssetID, actual.CoverAssetID)
}

// The mapped asset has an actual foreign-key identity only after media registration.
// Bind it in that checkpoint's transaction while the target remains unpublished.
func bindCopyProjectCover(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, job domain.ProjectCopyJob, owner application.ProjectCopyCoverOwner) error {
	frozen, err := readFrozenCopyWorkspace(tx, job)
	if err != nil {
		return err
	}
	if frozen.Target.CoverAssetID == nil {
		return nil
	}
	if owner == nil {
		return application.ErrProjectDependencyUnavailable
	}
	if err := owner.Verify(ctx, actor, copyMediaBinding(job), frozen.Target.CoverAssetID, frozen.Target.CoverAssetID); err != nil {
		return err
	}
	written := tx.Exec(`UPDATE workspace.project SET cover_asset_id=? WHERE id=? AND org_id=? AND status='copying' AND NOT is_delete AND cover_asset_id IS NULL`, frozen.Target.CoverAssetID, job.TargetProjectID, job.OrgID)
	if written.Error != nil {
		return written.Error
	}
	if written.RowsAffected != 1 {
		return domain.ErrProjectCopyStateConflict
	}
	return nil
}
