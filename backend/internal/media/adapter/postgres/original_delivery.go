package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// ForOriginalDelivery binds each read and write transaction to the authorized
// request for the original terminal result.
func (r *IngestRepository) ForOriginalDelivery(authorize func(*gorm.DB, uuid.UUID) error) *IngestRepository {
	bound := *r
	bound.original = authorize
	return &bound
}

func (r *IngestRepository) requireIngestState(status string) error {
	if status == string(operationdomain.StatusIngesting) && r.original == nil || r.original != nil && (status == string(operationdomain.StatusFailed) || status == string(operationdomain.StatusCompleted) || status == string(operationdomain.StatusCancelled)) {
		return nil
	}
	return ErrOperationNotIngesting
}
