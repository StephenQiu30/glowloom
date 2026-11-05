package postgres

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
)

func checkRequestScope(tx *gorm.DB, actor identityapp.Principal, project, key uuid.UUID, action, hash string) error {
	var row scriptRequestRow
	if err := tx.Where("actor_id = ? AND request_id = ?", actor.ID, key).Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if row.OrgID != actor.OrgID || row.ProjectID != project || row.RequestHash != hash || action != "" && row.Action != action {
		return application.ErrIdempotencyConflict
	}
	return nil
}
func registerRequest(tx *gorm.DB, actor identityapp.Principal, project, key uuid.UUID, action, hash string, now time.Time) error {
	if err := checkRequestScope(tx, actor, project, key, action, hash); err != nil {
		return err
	}
	row := scriptRequestRow{ActorID: actor.ID, RequestID: key, OrgID: actor.OrgID, ProjectID: project, Action: action, RequestHash: hash, CreatedAt: now}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "actor_id"}, {Name: "request_id"}}, DoNothing: true}).Create(&row).Error; err != nil {
		return err
	}
	var stored scriptRequestRow
	if err := tx.Where("actor_id = ? AND request_id = ?", actor.ID, key).Take(&stored).Error; err != nil {
		return err
	}
	if stored.OrgID != actor.OrgID || stored.ProjectID != project || stored.Action != action || stored.RequestHash != hash {
		return application.ErrConflict
	}
	return nil
}
