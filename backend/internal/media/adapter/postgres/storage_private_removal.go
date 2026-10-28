package postgres

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// privateWorkReference excludes only the current task's unpublished target
// reservation. Source history, published assets and another task's pins remain live.
func privateWorkReference(tx *gorm.DB, c legacyObjectColumn, l domain.ObjectLocation, w storageWorkRow) (bool, error) {
	var invalid bool
	var query string
	var args []any
	switch c.table {
	case "media.package_object":
		if w.Kind != "package" {
			return false, nil
		}
		query = `SELECT EXISTS(SELECT 1 FROM media.package_object t LEFT JOIN media.package_job j ON j.id=t.job_id WHERE t.object_location_id=? AND (j.id IS NULL OR NOT(j.id=? AND j.status='needs_reconciliation')))`
		args = []any{l.ID, w.WorkID}
	case "script.object_intent":
		if w.Kind != "script" {
			return false, nil
		}
		query = `SELECT EXISTS(SELECT 1 FROM script.object_intent t JOIN script.command c ON c.actor_id=t.actor_id AND c.request_id=t.request_id JOIN script.command_state s ON s.actor_id=c.actor_id AND s.request_id=c.request_id WHERE t.object_location_id=? AND NOT(COALESCE(c.plan->>'storage_intent_id','')=? AND c.project_id=? AND s.status='pending' AND s.cancellation_requested))`
		args = []any{l.ID, w.ID.String(), w.OwnerID}
	case "script.object_state":
		if w.Kind != "script" {
			return false, nil
		}
		query = `SELECT EXISTS(SELECT 1 FROM script.object_state t JOIN script.object_intent i ON i.object_key=t.object_key AND i.object_location_id=t.object_location_id JOIN script.command c ON c.actor_id=i.actor_id AND c.request_id=i.request_id JOIN script.command_state s ON s.actor_id=c.actor_id AND s.request_id=c.request_id WHERE t.object_location_id=? AND NOT(COALESCE(c.plan->>'storage_intent_id','')=? AND c.project_id=? AND s.status='pending' AND s.cancellation_requested))`
		args = []any{l.ID, w.ID.String(), w.OwnerID}
	case "script.copy_object_intent", "script.copy_object_state", "script.script_source", "script.script_version":
		if w.Kind != "script" {
			return false, nil
		}
		return privateCopyReference(tx, c, l, w)
	case "media.project_copy_object", "media.media_asset", "media.rendition":
		if w.Kind != "project-copy" {
			return false, nil
		}
		return privateCopyReference(tx, c, l, w)
	case "media.transfer_object":
		if w.Kind != "transfer" || c.location != "target_location_id" {
			return false, nil
		}
		var rows []struct {
			JobID           uuid.UUID
			ItemIndex       int
			TargetLibraryID uuid.UUID
			Status          string
		}
		if err := tx.Raw(`SELECT o.job_id,o.item_index,j.target_library_id,i.status FROM media.transfer_object o JOIN media.transfer_job j ON j.id=o.job_id JOIN media.transfer_item i ON i.job_id=o.job_id AND i.item_index=o.item_index WHERE o.target_location_id=?`, l.ID).Scan(&rows).Error; err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, nil
		}
		for _, r := range rows {
			if r.TargetLibraryID != w.OwnerID || uuid.NewSHA1(r.JobID, []byte("item/"+strconv.Itoa(r.ItemIndex))) != w.WorkID || r.Status == "succeeded" {
				return false, nil
			}
		}
		return true, nil
	case "mediatool.depth_object":
		if w.Kind != "depth" {
			return false, nil
		}
		var rows []struct {
			JobID, ProjectID                            uuid.UUID
			Attempt                                     int
			ProcessState                                string
			ExecutionUnconfirmed, CancellationRequested bool
			Status                                      string
		}
		if err := tx.Raw(`SELECT o.job_id,o.attempt,j.project_id,j.process_state,j.execution_unconfirmed,j.cancellation_requested,j.status FROM mediatool.depth_object o JOIN mediatool.depth_job j ON j.id=o.job_id AND j.attempt=o.attempt WHERE o.object_location_id=?`, l.ID).Scan(&rows).Error; err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, nil
		}
		for _, r := range rows {
			if r.ProjectID != w.OwnerID || uuid.NewSHA1(r.JobID, []byte("storage-attempt/"+strconv.Itoa(r.Attempt))) != w.WorkID || r.Status == "succeeded" || r.Status == "review_required" || !r.CancellationRequested || r.ExecutionUnconfirmed || (r.ProcessState != "ended" && r.ProcessState != "none") {
				return false, nil
			}
		}
		return true, nil
	default:
		return false, nil
	}
	if err := tx.Raw(query, args...).Scan(&invalid).Error; err != nil {
		return false, err
	}
	return !invalid, nil
}

func privateCopyReference(tx *gorm.DB, c legacyObjectColumn, l domain.ObjectLocation, w storageWorkRow) (bool, error) {
	var project, job uuid.UUID
	query := `SELECT target_project_id,job_id FROM media.project_copy_snapshot WHERE id=? AND target_project_id=?`
	if w.Kind == "script" {
		query = `SELECT target_project_id,job_id FROM script.copy_snapshot WHERE id=? AND target_project_id=? AND storage_intent_id=?`
	}
	var binding struct{ TargetProjectID, JobID uuid.UUID }
	args := []any{w.WorkID, w.OwnerID}
	if w.Kind == "script" {
		args = append(args, w.ID)
	}
	r := tx.Raw(query, args...).Scan(&binding)
	if r.Error != nil {
		return false, r.Error
	}
	if r.RowsAffected != 1 {
		return false, nil
	}
	project, job = binding.TargetProjectID, binding.JobID
	var private bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project p JOIN workspace.project_copy_job j ON j.id=? AND j.target_project_id=p.id WHERE p.id=? AND p.status='copying' AND NOT p.is_delete AND j.stage='cleanup' AND j.status<>'succeeded')`, job, project).Scan(&private).Error; err != nil {
		return false, err
	}
	if !private {
		return false, nil
	}
	var invalid bool
	var predicate string
	args = []any{l.ID}
	switch c.table {
	case "media.project_copy_object":
		if c.location != "target_location_id" {
			return false, nil
		}
		predicate = "t.snapshot_id<>?"
		args = append(args, w.WorkID)
	case "media.media_asset":
		predicate = "t.project_id<>? OR t.personal_actor_id IS NOT NULL"
		args = append(args, project)
	case "media.rendition":
		predicate = "NOT EXISTS(SELECT 1 FROM media.media_asset a WHERE a.id=t.media_asset_id AND a.project_id=? AND a.personal_actor_id IS NULL)"
		args = append(args, project)
	case "script.copy_object_intent":
		if c.location != "target_location_id" {
			return false, nil
		}
		predicate = "t.snapshot_id<>?"
		args = append(args, w.WorkID)
	case "script.copy_object_state":
		predicate = "NOT EXISTS(SELECT 1 FROM script.copy_object_intent i WHERE i.target_location_id=t.target_location_id AND i.target_key=t.target_key AND i.snapshot_id=?)"
		args = append(args, w.WorkID)
	case "script.script_source", "script.script_version":
		predicate = "t.project_id<>?"
		args = append(args, project)
	default:
		return false, nil
	}
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM `+c.table+` t WHERE `+c.location+`=? AND (`+predicate+`))`, args...).Scan(&invalid).Error; err != nil {
		return false, err
	}
	return !invalid, nil
}

// BeginPrivateCopyRemoval retires a partially registered result only while its
// entire workspace target is private and the current coordinator is cleaning it.
// The caller retains that coordinator's current worker fence transaction.
func (s *StorageRoutes) BeginPrivateCopyRemoval(ctx context.Context, id uuid.UUID, l domain.ObjectLocation) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	removed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var w storageWorkRow
		r := tx.Raw(`SELECT * FROM media.storage_work_intent WHERE id=? AND kind IN ('project-copy','script') AND state='accepted' FOR UPDATE`, id).Scan(&w)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return nil
		}
		// Match snapshot/job even when this result has not produced any visible row.
		probe := legacyObjectColumn{table: "media.media_asset", location: "object_location_id"}
		if w.Kind == "script" {
			probe = legacyObjectColumn{table: "script.script_source", location: "original_location_id"}
		}
		private, err := privateCopyReference(tx, probe, l, w)
		if err != nil || !private {
			return err
		}
		for _, c := range legacyObjectColumns {
			var live bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM `+c.table+` t WHERE `+c.location+`=? AND NOT(`+legacyRetired(c)+`))`, l.ID).Scan(&live).Error; err != nil {
				return err
			}
			if live {
				private, err := privateWorkReference(tx, c, l, w)
				if err != nil {
					return err
				}
				if !private {
					return nil
				}
			}
		}
		u := tx.Exec(`UPDATE media.object_location SET state='delete_pending' WHERE id=? AND owner_intent_id=? AND profile_id=? AND bucket=? AND object_key=? AND state IN ('accepted','delete_pending') AND (write_until IS NULL OR write_until<clock_timestamp())`, l.ID, id, l.Ref.ProfileID, l.Ref.Bucket, l.Ref.Key)
		removed = u.RowsAffected == 1
		return u.Error
	})
	return removed, err
}
