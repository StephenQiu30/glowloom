package postgres

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// revokeSessions locks every matching parent before its refresh records. Account
// and settings locks belong to the caller. The nested transaction also protects
// callers that catch an error without aborting their surrounding transaction.
func revokeSessions(db *gorm.DB, account uuid.UUID, only, except *uuid.UUID, now time.Time) error {
	return db.Transaction(func(tx *gorm.DB) error {
		query := `SELECT id FROM identity.user_session WHERE account_id=?`
		args := []any{account}
		if only != nil {
			query += ` AND id=?`
			args = append(args, *only)
		}
		if except != nil {
			query += ` AND id<>?`
			args = append(args, *except)
		}
		query += ` ORDER BY id FOR UPDATE`
		var sessions []struct{ ID uuid.UUID }
		if e := tx.Raw(query, args...).Scan(&sessions).Error; e != nil {
			return e
		}
		for _, session := range sessions {
			id := session.ID
			if e := tx.Exec(`UPDATE identity.user_session SET revoked_at=COALESCE(revoked_at,?) WHERE id=? AND account_id=?`, now, id, account).Error; e != nil {
				return e
			}
		}
		for _, session := range sessions {
			id := session.ID
			if e := (&authTx{db: tx}).RevokeRefreshChain(id, now); e != nil {
				return e
			}
		}
		return nil
	})
}
