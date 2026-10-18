package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// SessionDirectoryStore checks current identity before listing or revoking sessions.
type SessionDirectoryStore struct{ db *gorm.DB }

// NewSessionDirectoryStore constructs SessionDirectoryStore with explicit caller-supplied dependencies.
func NewSessionDirectoryStore(db *gorm.DB) *SessionDirectoryStore {
	return &SessionDirectoryStore{db: db}
}

// ListSelfSessions returns only valid sessions belonging to the actor.
func (s *SessionDirectoryStore) ListSelfSessions(ctx context.Context, actor application.Principal, clock func() time.Time) ([]application.SessionSummary, error) {
	out := []application.SessionSummary{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockSelfActor(tx, actor, clock); err != nil {
			return err
		}
		now := clock().UTC()
		return tx.Raw(`SELECT id,id=? AS current,device_summary,created_at,last_active_at,absolute_expires_at FROM identity.user_session WHERE account_id=? AND credential_revision=? AND revoked_at IS NULL AND created_at<=? AND last_active_at<=? AND ((protocol_version=1 AND last_active_at + interval '12 hours'>?) OR (protocol_version=2 AND idle_timeout_seconds>0 AND last_active_at + idle_timeout_seconds * interval '1 second'>?)) AND absolute_expires_at>? ORDER BY last_active_at DESC,id DESC`, actor.SessionID, actor.ID, actor.CredentialEpoch, now, now, now, now, now).Scan(&out).Error
	})
	return out, err
}

// RevokeSelfSession revokes one owned session idempotently.
func (s *SessionDirectoryStore) RevokeSelfSession(ctx context.Context, actor application.Principal, id uuid.UUID, clock func() time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockSelfActor(tx, actor, clock); err != nil {
			return err
		}
		now := clock().UTC()
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

		if err := revokeSessions(tx, actor.ID, &id, nil, now); err != nil {
			return err
		}
		return appendSelfEvent(tx, actor.ID, "session.revoked", now)
	})
}

// RevokeOtherSessions preserves the requesting session while revoking the others.
func (s *SessionDirectoryStore) RevokeOtherSessions(ctx context.Context, actor application.Principal, clock func() time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockSelfActor(tx, actor, clock); err != nil {
			return err
		}
		now := clock().UTC()
		if err := revokeSessions(tx, actor.ID, nil, &actor.SessionID, now); err != nil {
			return err
		}
		return appendSelfEvent(tx, actor.ID, "session.others_revoked", now)
	})
}
