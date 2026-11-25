package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CanReadLedger checks personal ownership or current team administration after caller authorization.
// The caller retains Identity and Creation authority locks in this same transaction.
func CanReadLedger(tx *gorm.DB, scope, actor uuid.UUID) (bool, error) {
	var allowed bool
	err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.creation_scope s WHERE s.id=? AND
 ((s.kind='personal' AND s.owner_user_id=?) OR (s.kind='team' AND EXISTS(
 SELECT 1 FROM workspace.team_member m WHERE m.team_id=s.team_id AND m.user_id=? AND m.role='team_admin' AND m.status='active'))))`, scope, actor, actor).Scan(&allowed).Error
	return allowed, err
}
