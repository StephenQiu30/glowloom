package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

func depthPublic(tx *gorm.DB, r depthRow) (domain.DepthJob, error) {
	j := r.job()
	if r.Status == "review_required" || r.Status == "succeeded" {
		a, found, err := loadDepthArtifact(tx, r)
		if err != nil {
			return j, err
		}
		if !found {
			return j, application.ErrConflict
		}
		j.AssetID = &a.Asset.ID
		j.SHA256 = a.Asset.SHA256
	}
	return j, nil
}

// Get restores safe facts after current project and principal authorization.
func (s *DepthStore) Get(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (domain.DepthJob, error) {
	var j domain.DepthJob
	if s == nil || s.db == nil {
		return j, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, false); err != nil {
			return err
		}
		r, err := readDepth(tx, project, id, false)
		if err != nil {
			return err
		}
		if r.OrgID != actor.OrgID {
			return application.ErrNotFound
		}
		j, err = depthPublic(tx, r)
		return err
	})
	return j, err
}

// List returns bounded current facts for an explicitly scoped saved source.
func (s *DepthStore) List(ctx context.Context, actor identityapp.Principal, in application.ListInput) ([]domain.DepthJob, error) {
	if s == nil || s.db == nil {
		return nil, application.ErrUnavailable
	}
	if in.ProjectID == uuid.Nil || in.Limit < 1 || in.Limit > 201 {
		return nil, application.ErrInvalidDepthInput
	}
	jobs := []domain.DepthJob{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, in.ProjectID, false); err != nil {
			return err
		}
		q := `SELECT * FROM mediatool.depth_job WHERE project_id=? AND org_id=?`
		args := []any{in.ProjectID, actor.OrgID}
		for _, f := range []struct {
			column string
			id     uuid.UUID
		}{{"canvas_id", in.CanvasID}, {"node_id", in.NodeID}, {"id", in.After}} {
			if f.id != uuid.Nil {
				op := "="
				if f.column == "id" {
					op = "<"
				}
				q += " AND " + f.column + op + "?"
				args = append(args, f.id)
			}
		}
		q += ` ORDER BY id DESC LIMIT ?`
		args = append(args, in.Limit)
		var rows []depthRow
		if err := tx.Raw(q, args...).Scan(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			if _, err := depthFrozen(r); err != nil {
				return err
			}
			j, err := depthPublic(tx, r)
			if err != nil {
				return err
			}
			jobs = append(jobs, j)
		}
		return nil
	})
	return jobs, err
}

// PreviewAsset permits only the exact pending or approved result, never a source key.
func (s *DepthStore) PreviewAsset(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (domain.DepthJob, mediadomain.MediaAsset, error) {
	var j domain.DepthJob
	var asset mediadomain.MediaAsset
	if s == nil || s.db == nil {
		return j, asset, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, false); err != nil {
			return err
		}
		r, err := readDepth(tx, project, id, false)
		if err != nil {
			return err
		}
		if r.OrgID != actor.OrgID {
			return application.ErrNotFound
		}
		if r.Status != "review_required" && r.Status != "succeeded" || r.CancellationRequested {
			return application.ErrConflict
		}
		a, found, err := loadDepthArtifact(tx, r)
		if err != nil {
			return err
		}
		if !found {
			return application.ErrConflict
		}
		asset, err = s.media(tx).Asset(ctx, actor, project, a.Asset.ID)
		if err != nil {
			return normalize(err)
		}
		if asset.IsDelete || asset.ID != a.Asset.ID || asset.ProjectID != project || asset.ObjectKey != a.Asset.ObjectKey || asset.Kind != "video" || asset.SHA256 == nil || a.Asset.SHA256 == nil || *asset.SHA256 != *a.Asset.SHA256 || asset.ByteSize != a.Asset.ByteSize || asset.ContainsRealPerson || asset.ConsentRecordID != nil || (r.Status == "succeeded") != asset.CanReference() {
			return application.ErrConflict
		}
		if r.Status == "review_required" && (asset.Status != "processing" || asset.ModerationStatus != "pending") {
			return application.ErrConflict
		}
		j = r.job()
		j.AssetID = &asset.ID
		j.SHA256 = asset.SHA256
		return nil
	})
	return j, asset, err
}

// HasInflightWork retains pending review, unresolved objects and unknown native processes.
func (s *DepthStore) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if project == uuid.Nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil {
		return false, identityapp.ErrForbidden
	}
	var exists bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM mediatool.depth_job WHERE project_id=? AND org_id=? AND (status IN ('queued','running','review_required','cancel_requested') OR active_worker IS NOT NULL OR needs_reconciliation OR execution_unconfirmed))`, project, actor.OrgID).Scan(&exists).Error
	return exists, err
}
