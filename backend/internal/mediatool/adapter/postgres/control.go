package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// Control records cancellation requests or queues a fenced retry of frozen inputs.
func (s *Store) Control(ctx context.Context, actor identityapp.Principal, project, id, key uuid.UUID, revision int64, action string) (domain.ExportJob, error) {
	var job domain.ExportJob
	if s == nil || s.db == nil {
		return job, application.ErrUnavailable
	}
	if revision < 1 || (action != "cancel" && action != "retry") {
		return job, application.ErrInvalidExport
	}
	hash, err := commandHash(action, project, id, revision)
	if err != nil {
		return job, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, true); err != nil {
			return err
		}
		if err := commandLock(tx, actor.ID, key); err != nil {
			return err
		}
		replay, found, err := commandReplay(tx, actor.ID, key, hash)
		if err != nil {
			return err
		}
		if found {
			job = replay
			return nil
		}
		if action == "retry" {
			if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "media-export-project/"+project.String()).Error; err != nil {
				return err
			}
		}
		row, err := readJob(tx, project, id, true)
		if err != nil {
			return err
		}
		if row.Revision != revision {
			return application.ErrConflict
		}
		emit := "cancel"
		if action == "cancel" {
			if row.Status != "queued" && row.Status != "running" && row.Status != "review_required" {
				return application.ErrConflict
			}
			if row.Status == "review_required" && row.AssetID != nil {
				if err := s.media(tx).Reject(ctx, actor, project, *row.AssetID, "export_cancelled", time.Now().UTC()); err != nil {
					return normalize(err)
				}
			}
			if row.Status == "review_required" {
				row.Status = "cancelled"
				row.Stage = "cancelled"
				emit = ""
			} else {
				row.Status = "cancel_requested"
				row.Stage = "cancelling"
			}
		} else {
			if (row.Status != "failed" && row.Status != "cancelled") || row.Attempt >= 100 {
				return application.ErrConflict
			}
			var active int64
			if err := tx.Raw(`SELECT count(*) FROM mediatool.export_job WHERE project_id=? AND status IN ('queued','running','cancel_requested')`, project).Scan(&active).Error; err != nil {
				return err
			}
			if active >= 2 {
				return application.ErrConflict
			}
			if err := s.validateSources(ctx, tx, row); err != nil {
				return err
			}
			row.Status = "queued"
			row.Stage = "queued"
			row.Progress = 0
			row.Attempt++
			row.ActiveWorker = nil
			row.AssetID = nil
			row.SHA256 = nil
			row.FailureCode = nil
			row.ActorID = actor.ID
			row.ActorRole = string(actor.Role)
			emit = "start"
		}
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		if err := updateJob(tx, row); err != nil {
			return err
		}
		job = row.public()
		return recordCommand(tx, actor, key, hash, action, job, emit)
	})
	return job, err
}

// Review accepts only inspected output bytes and publishes media in the same transaction.
func (s *Store) Review(ctx context.Context, actor identityapp.Principal, id, key uuid.UUID, input application.ReviewInput) (domain.ExportJob, error) {
	var job domain.ExportJob
	if s == nil || s.db == nil {
		return job, application.ErrUnavailable
	}
	if input.ProjectID == uuid.Nil || input.Revision < 1 || len(input.SHA256) != 64 || !input.LocalReviewConfirmed {
		return job, application.ErrInvalidExport
	}
	hash, err := commandHash("review", input.ProjectID, id, input)
	if err != nil {
		return job, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, input.ProjectID, true); err != nil {
			return err
		}
		if err := commandLock(tx, actor.ID, key); err != nil {
			return err
		}
		replay, found, err := commandReplay(tx, actor.ID, key, hash)
		if err != nil {
			return err
		}
		if found {
			job = replay
			return nil
		}
		row, err := readJob(tx, input.ProjectID, id, true)
		if err != nil {
			return err
		}
		if row.Revision != input.Revision || row.Status != "review_required" || row.AssetID == nil || row.SHA256 == nil || *row.SHA256 != input.SHA256 {
			return application.ErrConflict
		}
		if err := s.media(tx).Review(ctx, actor, row.ProjectID, *row.AssetID, input.SHA256, time.Now().UTC()); err != nil {
			return normalize(err)
		}
		row.Status = "succeeded"
		row.Stage = "complete"
		row.Progress = 100
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		if err := updateJob(tx, row); err != nil {
			return err
		}
		job = row.public()
		return recordCommand(tx, actor, key, hash, "reviewed", job, "")
	})
	return job, err
}

// PreviewAsset exposes an exact produced result only through the owning media reader.
func (s *Store) PreviewAsset(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (domain.ExportJob, mediadomain.MediaAsset, error) {
	var job domain.ExportJob
	var asset mediadomain.MediaAsset
	if s == nil || s.db == nil {
		return job, asset, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, false); err != nil {
			return err
		}
		row, err := readJob(tx, project, id, false)
		if err != nil {
			return err
		}
		if (row.Status != "review_required" && row.Status != "succeeded") || row.AssetID == nil || row.SHA256 == nil {
			return application.ErrConflict
		}
		asset, err = s.media(tx).Asset(ctx, actor, project, *row.AssetID)
		if err != nil {
			return normalize(err)
		}
		if asset.IsDelete || asset.SHA256 == nil || *asset.SHA256 != *row.SHA256 || ((row.Status == "succeeded") != asset.CanReference()) {
			return application.ErrConflict
		}
		job = row.public()
		return nil
	})
	return job, asset, err
}

// Frozen returns saved edit facts for an authorized subtitle download.
func (s *Store) Frozen(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (domain.FrozenExport, error) {
	var frozen domain.FrozenExport
	if s == nil || s.db == nil {
		return frozen, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, false); err != nil {
			return err
		}
		row, err := readJob(tx, project, id, false)
		if err != nil {
			return err
		}
		return json.Unmarshal(row.Frozen, &frozen)
	})
	return frozen, err
}
func updateJob(tx *gorm.DB, row jobRow) error {
	return tx.Exec(`UPDATE mediatool.export_job SET status=?,stage=?,progress=?,attempt=?,revision=?,asset_id=?,sha256=?,failure_code=?,actor_id=?,actor_role=?,updated_at=?,active_worker=? WHERE id=?`, row.Status, row.Stage, row.Progress, row.Attempt, row.Revision, row.AssetID, row.SHA256, row.FailureCode, row.ActorID, row.ActorRole, row.UpdatedAt, row.ActiveWorker, row.ID).Error
}
