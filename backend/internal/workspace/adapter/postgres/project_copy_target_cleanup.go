package postgres

import (
	"time"

	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// retireCopyTarget runs only after media byte absence and canvas cleanup are proven.
// The caller already holds both project locks. Personal classification stays in
// place as history; deleted targets are invisible and directory recycling clears it.
// Taking a library lock here would reverse admission's library-to-project order.
func retireCopyTarget(tx *gorm.DB, job domain.ProjectCopyJob, now time.Time) error {
	write := tx.Exec(`UPDATE workspace.project SET is_delete=true,delete_time=?,purge_after=?,revision=revision+1 WHERE id=? AND org_id=? AND status='copying' AND NOT is_delete AND revision<2147483647`, now, now.Add(30*24*time.Hour), job.TargetProjectID, job.OrgID)
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected != 1 {
		return domain.ErrProjectCopyStateConflict
	}
	return nil
}
