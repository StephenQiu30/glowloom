package gormdb

import (
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"gorm.io/gorm"
)

// FailCandidateSetInsert injects a storage failure after the row is written but
// before the Owner transaction can commit. It never affects other tables.
func FailCandidateSetInsert(db *Database) (func() error, error) {
	const callback = "test_candidate_set_commit_failure"
	err := db.Callback().Create().After("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == (model.GenerationReferenceCandidateSet{}).TableName() {
			tx.AddError(errors.New("injected candidate Set failure after insert"))
		}
	})
	return func() error { return db.Callback().Create().Remove(callback) }, err
}
