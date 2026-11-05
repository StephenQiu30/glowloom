package postgres

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	mediatooldomain "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	scriptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// hasLiveStorageReference checks every registered owner before granting deletion.
func hasLiveStorageReference(tx *gorm.DB, column storageOwnerColumn, locationID uuid.UUID) (bool, error) {
	var records []map[string]any
	// Callers hold the canonical location UPDATE lock before scanning. Published
	// owners can be immutable: row locking them would require forbidden UPDATE
	// privileges and cannot protect a reference which does not exist yet.
	query := tx.Table(column.table).Where(column.location+" = ?", locationID)
	if err := query.Find(&records).Error; err != nil {
		return false, err
	}
	for _, record := range records {
		retired, err := storageOwnerRetired(tx, column, record)
		if err != nil {
			return false, err
		}
		if !retired {
			return true, nil
		}
	}
	return false, nil
}

// privateWorkReference excludes only the current task's unpublished target
// reservation. Source history, published assets and another task's pins remain live.
func privateWorkReference(tx *gorm.DB, column storageOwnerColumn, location domain.ObjectLocation, work storageWorkRow) (bool, error) {
	var count int64
	switch column.table {
	case "media.package_object":
		if work.Kind != application.StorageWorkKindPackage {
			return false, nil
		}
		err := tx.Table("media.package_object AS o").
			Joins("LEFT JOIN media.package_job AS j ON j.id = o.job_id").
			Where("o.object_location_id = ? AND (j.id IS NULL OR NOT (j.id = ? AND j.status = ?))", location.ID, work.WorkID, application.PackageStatusNeedsReconciliation).
			Count(&count).Error
		return count == 0, err
	case "script.object_intent":
		if work.Kind != application.StorageWorkKindScript {
			return false, nil
		}
		err := tx.Table("script.object_intent AS o").
			Joins("JOIN script.command AS c ON c.actor_id = o.actor_id AND c.request_id = o.request_id").
			Joins("JOIN script.command_state AS s ON s.actor_id = c.actor_id AND s.request_id = c.request_id").
			Where("o.object_location_id = ? AND NOT (COALESCE(c.plan->>'storage_intent_id', '') = ? AND c.project_id = ? AND s.status = ? AND s.cancellation_requested)", location.ID, work.ID.String(), work.OwnerID, scriptdomain.CommandStatusPending).
			Count(&count).Error
		return count == 0, err
	case "script.object_state":
		if work.Kind != application.StorageWorkKindScript {
			return false, nil
		}
		err := tx.Table("script.object_state AS o").
			Joins("JOIN script.object_intent AS i ON i.object_key = o.object_key AND i.object_location_id = o.object_location_id").
			Joins("JOIN script.command AS c ON c.actor_id = i.actor_id AND c.request_id = i.request_id").
			Joins("JOIN script.command_state AS s ON s.actor_id = c.actor_id AND s.request_id = c.request_id").
			Where("o.object_location_id = ? AND NOT (COALESCE(c.plan->>'storage_intent_id', '') = ? AND c.project_id = ? AND s.status = ? AND s.cancellation_requested)", location.ID, work.ID.String(), work.OwnerID, scriptdomain.CommandStatusPending).
			Count(&count).Error
		return count == 0, err
	case "script.copy_object_intent", "script.copy_object_state", "script.script_source", "script.script_version",
		"media.project_copy_object", "media.media_asset", "media.rendition":
		if work.Kind != application.StorageWorkKindScript && work.Kind != application.StorageWorkKindProjectCopy {
			return false, nil
		}
		return privateCopyReference(tx, column, location, work)
	case "media.transfer_object":
		if work.Kind != application.StorageWorkKindTransfer || column.location != "target_location_id" {
			return false, nil
		}
		var rows []struct {
			JobID                    uuid.UUID
			ItemIndex, OutputAttempt int
			TargetLibraryID          uuid.UUID
			WorkIntentID             *uuid.UUID
			Status                   string
		}
		err := tx.Table("media.transfer_object AS o").
			Select("o.job_id, o.item_index, o.output_attempt, j.target_library_id, a.work_intent_id, i.status").
			Joins("JOIN media.transfer_job AS j ON j.id = o.job_id").
			Joins("JOIN media.transfer_item AS i ON i.job_id = o.job_id AND i.item_index = o.item_index").
			Joins("JOIN media.transfer_output_attempt AS a ON a.job_id = o.job_id AND a.item_index = o.item_index AND a.output_attempt = o.output_attempt").
			Where("o.target_location_id = ?", location.ID).
			Find(&rows).Error
		if err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, nil
		}
		for _, row := range rows {
			if row.WorkIntentID == nil || *row.WorkIntentID != work.ID || row.TargetLibraryID != work.OwnerID || transferWorkOwner(row.JobID, row.TargetLibraryID, row.ItemIndex, row.OutputAttempt).WorkID != work.WorkID || row.Status == string(domain.TransferItemStatusSucceeded) {
				return false, nil
			}
		}
		return true, nil
	case "mediatool.depth_object":
		if work.Kind != application.StorageWorkKindDepth {
			return false, nil
		}
		var rows []struct {
			JobID, ProjectID                            uuid.UUID
			Attempt                                     int
			ProcessState                                string
			ExecutionUnconfirmed, CancellationRequested bool
			Status                                      string
		}
		err := tx.Table("mediatool.depth_object AS o").
			Select("o.job_id, o.attempt, j.project_id, j.process_state, j.execution_unconfirmed, j.cancellation_requested, j.status").
			Joins("JOIN mediatool.depth_job AS j ON j.id = o.job_id AND j.attempt = o.attempt").
			Where("o.object_location_id = ?", location.ID).
			Find(&rows).Error
		if err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, nil
		}
		for _, row := range rows {
			if row.ProjectID != work.OwnerID || uuid.NewSHA1(row.JobID, []byte("storage-attempt/"+strconv.Itoa(row.Attempt))) != work.WorkID || row.Status == string(mediatooldomain.DepthSucceeded) || row.Status == string(mediatooldomain.DepthReviewRequired) || !row.CancellationRequested || row.ExecutionUnconfirmed || (row.ProcessState != string(mediatooldomain.DepthProcessEnded) && row.ProcessState != string(mediatooldomain.DepthProcessNone)) {
				return false, nil
			}
		}
		return true, nil
	default:
		return false, nil
	}
}

func privateCopyReference(tx *gorm.DB, column storageOwnerColumn, location domain.ObjectLocation, work storageWorkRow) (bool, error) {
	table := "media.project_copy_snapshot"
	query := tx.Table(table).Select("target_project_id, job_id").Where("id = ? AND target_project_id = ?", work.WorkID, work.OwnerID)
	if work.Kind == application.StorageWorkKindScript {
		table = "script.copy_snapshot"
		query = tx.Table(table).Select("target_project_id, job_id").Where("id = ? AND target_project_id = ? AND storage_intent_id = ?", work.WorkID, work.OwnerID, work.ID)
	} else if work.Kind != application.StorageWorkKindProjectCopy {
		return false, nil
	}
	var binding struct {
		TargetProjectID uuid.UUID
		JobID           uuid.UUID
	}
	if err := query.Take(&binding).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	// The copy coordinator publishes by changing these mutable owner rows. Keep
	// that transition outside the private-cleanup decision until this transaction ends.
	for _, owner := range []struct {
		table string
		id    uuid.UUID
	}{
		{"workspace.project", binding.TargetProjectID},
		{"workspace.project_copy_job", binding.JobID},
	} {
		var row struct{ ID uuid.UUID }
		if err := tx.Table(owner.table).Select("id").Where("id = ?", owner.id).
			Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
	}
	var privateCount int64
	if err := tx.Table("workspace.project AS p").
		Joins("JOIN workspace.project_copy_job AS j ON j.id = ? AND j.target_project_id = p.id", binding.JobID).
		Where("p.id = ? AND p.status = ? AND NOT p.is_delete AND j.stage = ? AND j.status <> ?", binding.TargetProjectID, workspacedomain.ProjectStatusCopying, workspacedomain.ProjectCopyStageCleanup, workspacedomain.ProjectCopyStatusSucceeded).
		Count(&privateCount).Error; err != nil {
		return false, err
	}
	if privateCount == 0 {
		return false, nil
	}
	query = tx.Table(column.table+" AS t").Where("t."+column.location+" = ?", location.ID)
	switch column.table {
	case "media.project_copy_object", "script.copy_object_intent":
		if column.location != "target_location_id" {
			return false, nil
		}
		query = query.Where("t.snapshot_id <> ?", work.WorkID)
	case "media.media_asset":
		query = query.Where("t.project_id <> ? OR t.personal_actor_id IS NOT NULL", binding.TargetProjectID)
	case "media.rendition":
		query = query.Joins("LEFT JOIN media.media_asset AS a ON a.id = t.media_asset_id").Where("a.id IS NULL OR a.project_id <> ? OR a.personal_actor_id IS NOT NULL", binding.TargetProjectID)
	case "script.copy_object_state":
		intent := tx.Table("script.copy_object_intent AS i").Select("1").
			Where("i.target_location_id = t.target_location_id AND i.target_key = t.target_key AND i.snapshot_id = ?", work.WorkID)
		query = query.Where("NOT EXISTS (?)", intent)
	case "script.script_source", "script.script_version":
		query = query.Where("t.project_id <> ?", binding.TargetProjectID)
	default:
		return false, nil
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count == 0, nil
}

// BeginPrivateCopyRemoval grants deletion only while the target project is
// private and the owning copy coordinator is cleaning it.
func (s *StorageRoutes) BeginPrivateCopyRemoval(ctx context.Context, id uuid.UUID, location domain.ObjectLocation) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	removable := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var work storageWorkRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND kind IN ? AND state = ?", id, []application.StorageWorkKind{application.StorageWorkKindProjectCopy, application.StorageWorkKindScript}, application.StorageWorkAccepted).
			Take(&work).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var row objectLocationRow
		read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND owner_intent_id = ? AND profile_id = ? AND bucket = ? AND object_key = ?", location.ID, id, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key).
			Where("state IN ? AND (write_until IS NULL OR write_until < clock_timestamp())", []domain.ObjectLocationState{domain.ObjectLocationAccepted, domain.ObjectLocationDeletePending}).Take(&row)
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return nil
		}
		if read.Error != nil {
			return read.Error
		}
		probe := storageOwnerColumn{table: "media.media_asset", key: "object_key", location: "object_location_id"}
		if work.Kind == application.StorageWorkKindScript {
			probe = storageOwnerColumn{table: "script.script_source", key: "original_key", location: "original_location_id"}
		}
		private, err := privateCopyReference(tx, probe, location, work)
		if err != nil || !private {
			return err
		}
		for _, column := range storageOwnerColumns {
			live, err := hasLiveStorageReference(tx, column, location.ID)
			if err != nil {
				return err
			}
			if live {
				private, err := privateWorkReference(tx, column, location, work)
				if err != nil {
					return err
				}
				if !private {
					return nil
				}
			}
		}
		result := tx.Model(&objectLocationRow{}).
			Where("id = ? AND owner_intent_id = ? AND profile_id = ? AND bucket = ? AND object_key = ?", location.ID, id, location.Ref.ProfileID, location.Ref.Bucket, location.Ref.Key).
			Where("state IN ? AND (write_until IS NULL OR write_until < clock_timestamp())", []domain.ObjectLocationState{domain.ObjectLocationAccepted, domain.ObjectLocationDeletePending}).
			Update("state", domain.ObjectLocationDeletePending)
		if result.Error != nil {
			return result.Error
		}
		removable = result.RowsAffected == 1
		return nil
	})
	return removable, err
}
