package postgres

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

func checkRequestScope(tx *gorm.DB, actor identityapp.Principal, project, key uuid.UUID, action, hash string) error {
	var row struct {
		OrgID, ProjectID    uuid.UUID
		Action, RequestHash string
	}
	read := tx.Raw(`SELECT org_id,project_id,action,request_hash FROM script.request WHERE actor_id=? AND request_id=?`, actor.ID, key).Scan(&row)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected == 0 {
		return nil
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
	write := tx.Exec(`INSERT INTO script.request(actor_id,request_id,org_id,project_id,action,request_hash,created_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(actor_id,request_id) DO NOTHING`, actor.ID, key, actor.OrgID, project, action, hash, now)
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected == 1 {
		return nil
	}
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM script.request WHERE actor_id=? AND request_id=? AND org_id=? AND project_id=? AND action=? AND request_hash=?`, actor.ID, key, actor.OrgID, project, action, hash).Scan(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return application.ErrConflict
	}
	return nil
}
