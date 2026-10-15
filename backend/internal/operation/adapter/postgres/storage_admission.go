package postgres

import (
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// OperationStorageFactory binds output admission to the quote transaction.
type OperationStorageFactory func(*gorm.DB) application.OperationStorageAdmission

// NewStorageAwareStore retains existing authority/compiler dependencies while
// explicitly adding transactional storage admission. It does not mutate base.
func NewStorageAwareStore(base *Store, factory OperationStorageFactory) *Store {
	if base == nil || factory == nil {
		return nil
	}
	bound := *base
	bound.storageAdmission = factory
	return &bound
}
