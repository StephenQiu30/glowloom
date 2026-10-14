package postgres

import (
	"context"
	"fmt"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type SessionDirectoryStore struct{ db *gorm.DB }

func NewSessionDirectoryStore(db *gorm.DB) *SessionDirectoryStore {
	return &SessionDirectoryStore{db: db}
}
func (s *SessionDirectoryStore) ListSelfSessions(ctx context.Context, actor application.Principal, now time.Time) ([]application.SessionSummary, error) {
	out := []application.SessionSummary{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockSelfActor(tx, actor, now); err != nil {
			return err
		}
		return tx.Raw(`SELECT id,id=? AS current,device_summary,created_at,last_active_at,absolute_expires_at FROM identity.user_session WHERE account_id=? AND credential_revision=? AND revoked_at IS NULL AND last_active_at<=? AND last_active_at>? AND absolute_expires_at>? ORDER BY last_active_at DESC,id DESC`, actor.SessionID, actor.ID, actor.CredentialEpoch, now, now.Add(-12*time.Hour), now).Scan(&out).Error
	})
	return out, err
}
func (s *SessionDirectoryStore) RevokeSelfSession(ctx context.Context, actor application.Principal, id uuid.UUID, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockSelfActor(tx, actor, now); err != nil {
			return err
		}
		var row struct {
			ID        uuid.UUID
			RevokedAt *time.Time
		}
		read := tx.Raw(`SELECT id,revoked_at FROM identity.user_session WHERE id=? AND account_id=? FOR UPDATE`, id, actor.ID).Scan(&row)
		if read.Error != nil {
			return fmt.Errorf("read own device: %w", read.Error)
		}
		if read.RowsAffected != 1 {
			return application.ErrSessionInvalid
		}
		if row.RevokedAt != nil {
			return nil
		}
		if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE id=? AND account_id=?`, now, id, actor.ID).Error; err != nil {
			return err
		}
		return appendSelfEvent(tx, actor.ID, "session.revoked", now)
	})
}
func (s *SessionDirectoryStore) RevokeOtherSessions(ctx context.Context, actor application.Principal, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockSelfActor(tx, actor, now); err != nil {
			return err
		}
		result := tx.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE account_id=? AND id<>? AND revoked_at IS NULL`, now, actor.ID, actor.SessionID)
		if result.Error != nil {
			return fmt.Errorf("revoke other devices: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return appendSelfEvent(tx, actor.ID, "session.others_revoked", now)
	})
}
