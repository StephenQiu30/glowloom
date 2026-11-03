package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ForOriginalDelivery binds each read and write transaction to the authorized
// request for the original terminal result.
func (r *IngestRepository) ForOriginalDelivery(authorize func(*gorm.DB, uuid.UUID) error) *IngestRepository {
	bound := *r
	bound.original = authorize
	return &bound
}

func (r *IngestRepository) requireIngestState(status string) error {
	if status == "ingesting" && r.original == nil || r.original != nil && (status == "failed" || status == "completed" || status == "cancelled") {
		return nil
	}
	return ErrOperationNotIngesting
}
