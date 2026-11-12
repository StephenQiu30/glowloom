package postgres

import (
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

// Worker authorization has already established the project's scope. Verify the
// source belongs to that project instead of accepting an unrelated canvas ID.
func validateGoalSource(tx *gorm.DB, in domain.Admission) error {
	source := in.GoalContract.Source
	if source == nil {
		return nil
	}
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM canvas.document d JOIN canvas.node n ON n.document_id=d.id WHERE d.id=? AND d.project_id=? AND NOT d.is_delete AND n.id=? AND NOT n.is_delete AND d.revision>=?`, source.CanvasID, in.Scope.ProjectID, source.NodeID, source.Revision).Scan(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return domain.ErrNotFound
	}
	return nil
}
