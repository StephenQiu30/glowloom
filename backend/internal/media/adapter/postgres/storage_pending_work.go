package postgres

import (
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// requireFrozenPendingWork refuses cutover of an old unfinished producer whose
// output target is still unknown. Recovery must retain its original profile;
// neither a new route nor the existence of unrelated old work proves that target.
func requireFrozenPendingWork(tx *gorm.DB) error {
	var unknown bool
	if err := tx.Raw(`SELECT
 EXISTS(SELECT 1 FROM operation.operation o WHERE NOT EXISTS(SELECT 1 FROM media.storage_work_intent w WHERE w.kind='operation' AND w.owner_id=o.project_id AND w.work_id=o.id)) OR
 EXISTS(SELECT 1 FROM subject.voice_job j WHERE j.status NOT IN ('succeeded','failed') AND NOT EXISTS(SELECT 1 FROM media.storage_work_intent w WHERE w.id=j.storage_intent_id AND w.kind='subject-voice' AND w.owner_id=j.owner_scope_id AND w.work_id=j.id)) OR
 EXISTS(SELECT 1 FROM media.package_job j WHERE j.status='needs_reconciliation' AND NOT EXISTS(SELECT 1 FROM media.storage_work_intent w WHERE w.kind='package' AND w.owner_id=COALESCE(j.project_id,j.actor_id) AND w.work_id=j.id)) OR
 EXISTS(SELECT 1 FROM media.project_copy_snapshot s JOIN workspace.project_copy_job j ON j.id=s.job_id WHERE j.status<>'succeeded' AND EXISTS(SELECT 1 FROM media.project_copy_object o WHERE o.snapshot_id=s.id AND o.status<>'removed') AND NOT EXISTS(SELECT 1 FROM media.storage_work_intent w WHERE w.kind='project-copy' AND w.owner_id=s.target_project_id AND w.work_id=s.id)) OR
 EXISTS(SELECT 1 FROM script.command c JOIN script.command_state s ON s.actor_id=c.actor_id AND s.request_id=c.request_id WHERE s.status='pending' AND EXISTS(SELECT 1 FROM script.object_intent i WHERE i.actor_id=c.actor_id AND i.request_id=c.request_id) AND NOT EXISTS(SELECT 1 FROM media.storage_work_intent w WHERE w.kind='script' AND w.owner_id=c.project_id AND w.id::text=c.plan->>'storage_intent_id')) OR
 EXISTS(SELECT 1 FROM script.copy_snapshot s WHERE EXISTS(SELECT 1 FROM script.copy_object_intent i JOIN script.copy_object_state o ON o.target_key=i.target_key AND o.target_location_id IS NOT DISTINCT FROM i.target_location_id WHERE i.snapshot_id=s.id AND NOT o.removed) AND NOT EXISTS(SELECT 1 FROM script.copy_receipt r WHERE r.snapshot_id=s.id) AND NOT EXISTS(SELECT 1 FROM media.storage_work_intent w WHERE w.id=s.storage_intent_id AND w.kind='script' AND w.owner_id=s.target_project_id AND w.work_id=s.id))`).Scan(&unknown).Error; err != nil {
		return err
	}
	if unknown {
		return application.ErrStorageNotReady
	}
	var transfers []struct {
		JobID, TargetLibraryID uuid.UUID
		ItemIndex              int
	}
	if err := tx.Raw(`SELECT DISTINCT i.job_id,i.item_index,j.target_library_id FROM media.transfer_item i JOIN media.transfer_job j ON j.id=i.job_id WHERE i.status<>'succeeded' AND i.status<>'cancelled' AND EXISTS(SELECT 1 FROM media.transfer_object o WHERE o.job_id=i.job_id AND o.item_index=i.item_index AND o.status<>'removed')`).Scan(&transfers).Error; err != nil {
		return err
	}
	for _, r := range transfers {
		var frozen bool
		id := uuid.NewSHA1(r.JobID, []byte("item/"+strconv.Itoa(r.ItemIndex)))
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.storage_work_intent WHERE kind='transfer' AND owner_id=? AND work_id=?)`, r.TargetLibraryID, id).Scan(&frozen).Error; err != nil {
			return err
		}
		if !frozen {
			return application.ErrStorageNotReady
		}
	}
	for _, kind := range []string{"export", "depth", "transcription"} {
		var rows []struct {
			ID, ProjectID uuid.UUID
			Attempt       int
		}
		if err := tx.Raw(`SELECT id,project_id,attempt FROM mediatool.` + kind + `_job WHERE status IN ('queued','running','cancel_requested') OR active_worker IS NOT NULL`).Scan(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			var frozen bool
			work := uuid.NewSHA1(r.ID, []byte("storage-attempt/"+strconv.Itoa(r.Attempt)))
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.storage_work_intent WHERE kind=? AND owner_id=? AND work_id=?)`, kind, r.ProjectID, work).Scan(&frozen).Error; err != nil {
				return err
			}
			if !frozen {
				return application.ErrStorageNotReady
			}
		}
	}
	return nil
}
