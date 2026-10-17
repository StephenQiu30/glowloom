package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// BackfillLegacyOperationWork freezes pre-cutover operation targets without
// rewriting JSON/Temporal receipts. This privileged migration requires reviewed
// drain evidence and is forbidden after the first guarded activation. A missing
// target during ordinary recovery remains an error; execution never guesses.
func (s *StorageRoutes) BackfillLegacyOperationWork(ctx context.Context, configSHA string, limit int) (int, error) {
	if s == nil || s.db == nil {
		return 0, application.ErrUnavailable
	}
	if !storageDigest(configSHA) || limit < 1 || limit > 1000 {
		return 0, application.ErrStorageNotReady
	}
	bound := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended('media.storage_route',0))`).Error; err != nil {
			return err
		}
		binding, err := readLegacyBinding(tx)
		if err != nil {
			return err
		}
		var permitted bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.storage_deployment_evidence WHERE config_sha256=?) AND NOT EXISTS(SELECT 1 FROM media.storage_activation_guard)`, configSHA).Scan(&permitted).Error; err != nil {
			return err
		}
		if !permitted {
			return application.ErrStorageNotReady
		}
		var rows []struct {
			ID, ProjectID uuid.UUID
			Status        string
		}
		if err := tx.Raw(`SELECT o.id,o.project_id,o.status FROM operation.operation o JOIN media.storage_deployment_evidence e ON e.config_sha256=? WHERE o.create_time<=e.recorded_at AND NOT EXISTS(SELECT 1 FROM media.storage_work_intent w WHERE w.kind='operation' AND w.owner_id=o.project_id AND w.work_id=o.id) ORDER BY o.create_time,o.id LIMIT ? FOR UPDATE OF o SKIP LOCKED`, configSHA, limit).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			state := "open"
			switch row.Status {
			case "completed", "failed", "cancelled", "expired":
				state = "accepted"
			}
			update := tx.Exec(`INSERT INTO media.storage_work_intent(id,kind,owner_id,work_id,profile_id,bucket,config_revision,state,closed_at) VALUES(?,'operation',?,?,?,?,?,?,CASE WHEN ?='open' THEN NULL ELSE clock_timestamp() END) ON CONFLICT(kind,owner_id,work_id) DO NOTHING`, uuid.New(), row.ProjectID, row.ID, binding.ProfileID, binding.Bucket, "legacy-"+binding.EvidenceSHA256[:16], state, state)
			if update.Error != nil {
				return update.Error
			}
			bound += int(update.RowsAffected)
		}
		return nil
	})
	return bound, err
}
