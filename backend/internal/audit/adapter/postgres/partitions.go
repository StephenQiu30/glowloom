package postgres

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// PartitionStore prepares writable audit months using the table owner account.
type PartitionStore struct {
	db *gorm.DB
}

// NewPartitionStore injects the privileged database handle for partition work.
func NewPartitionStore(db *gorm.DB) *PartitionStore {
	return &PartitionStore{db: db}
}

// EnsurePartitions attaches the current UTC month and the next three months.
// All moves and attachments share one transaction, so a failed month leaves
// default rows and the append-only trigger unchanged.
func (s *PartitionStore) EnsurePartitions(ctx context.Context, now time.Time) error {
	current := now.UTC()
	first := time.Date(current.Year(), current.Month(), 1, 0, 0, 0, 0, time.UTC)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("LOCK TABLE audit.audit_log IN ACCESS EXCLUSIVE MODE").Error; err != nil {
			return fmt.Errorf("lock audit log for partition maintenance: %w", err)
		}
		for offset := range 4 {
			if err := ensureAuditMonth(tx, first.AddDate(0, offset, 0)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("ensure audit partitions: %w", err)
	}
	return nil
}

func ensureAuditMonth(tx *gorm.DB, month time.Time) error {
	name := "audit_log_" + month.Format("200601")
	end := month.AddDate(0, 1, 0)
	var attached bool
	if err := tx.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM pg_inherits AS i
			JOIN pg_class AS c ON c.oid = i.inhrelid
			JOIN pg_namespace AS n ON n.oid = c.relnamespace
			WHERE i.inhparent = 'audit.audit_log'::regclass
			  AND n.nspname = 'audit' AND c.relname = ?
		)
	`, name).Scan(&attached).Error; err != nil {
		return fmt.Errorf("check audit partition %s: %w", name, err)
	}
	if attached {
		return nil
	}
	// The table name comes only from a UTC month, never from request input.
	if err := tx.Exec("CREATE TABLE audit." + name + " (LIKE audit.audit_log INCLUDING ALL)").Error; err != nil {
		return fmt.Errorf("create audit partition %s: %w", name, err)
	}
	if err := tx.Exec("ALTER TABLE audit.audit_log_default DISABLE TRIGGER audit_log_append_only").Error; err != nil {
		return fmt.Errorf("pause default audit partition trigger: %w", err)
	}
	move := `
		WITH moved AS (
			DELETE FROM audit.audit_log_default
			WHERE create_time >= ? AND create_time < ?
			RETURNING id, org_id, project_id, actor_id, actor_kind, action,
			          object_type, object_id, before, after, request_id, trace_id,
			          ip, create_time, update_time, is_delete
		)
		INSERT INTO audit.` + name + ` (
			id, org_id, project_id, actor_id, actor_kind, action,
			object_type, object_id, before, after, request_id, trace_id,
			ip, create_time, update_time, is_delete
		)
		SELECT id, org_id, project_id, actor_id, actor_kind, action,
		       object_type, object_id, before, after, request_id, trace_id,
		       ip, create_time, update_time, is_delete FROM moved
	`
	if err := tx.Exec(move, month, end).Error; err != nil {
		return fmt.Errorf("move default audit rows into %s: %w", name, err)
	}
	attach := fmt.Sprintf(
		"ALTER TABLE audit.audit_log ATTACH PARTITION audit.%s FOR VALUES FROM ('%s') TO ('%s')",
		name, month.Format(time.RFC3339), end.Format(time.RFC3339),
	)
	if err := tx.Exec(attach).Error; err != nil {
		return fmt.Errorf("attach audit partition %s: %w", name, err)
	}
	if err := tx.Exec("ALTER TABLE audit.audit_log_default ENABLE TRIGGER audit_log_append_only").Error; err != nil {
		return fmt.Errorf("resume default audit partition trigger: %w", err)
	}
	return nil
}
