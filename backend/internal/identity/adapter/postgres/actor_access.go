package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// CheckSession retains identity/session locks in the caller's content transaction.
// A connection-pool call is rejected because releasing the locks before a write
// would allow a concurrent revocation to race the authorization decision.
func (s *Store) CheckSession(ctx context.Context, actor application.Principal, clock func() time.Time) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return application.ErrSessionInvalid
	}
	if _, ok := s.db.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return application.ErrSessionInvalid
	}
	_, err := lockSelfActor(s.db.WithContext(ctx), actor, clock)
	return err
}

// CheckRecentSession requires a password proof bound to this live session and epoch.
func (s *Store) CheckRecentSession(ctx context.Context, actor application.Principal, clock func() time.Time) error {
	if err := s.CheckSession(ctx, actor, clock); err != nil {
		return err
	}
	u, err := (&authTx{db: s.db.WithContext(ctx)}).AccountByID(actor.ID)
	if err != nil {
		return err
	}
	var session domain.AuthSession
	if err := s.db.WithContext(ctx).Raw(`SELECT * FROM identity.user_session WHERE id=? AND account_id=?`, actor.SessionID, actor.ID).Scan(&session).Error; err != nil {
		return err
	}
	if !session.RecentlyAuthenticated(u, clock().UTC()) {
		return application.ErrReauthenticationRequired
	}
	return nil
}

// CheckActive rechecks availability without granting any content or team role.
func (s *Store) CheckActive(ctx context.Context, id uuid.UUID) error {
	var present int
	r := s.db.WithContext(ctx).Raw(`SELECT 1 FROM identity."user" WHERE id=? AND status='active' AND NOT is_delete AND email_key IS NOT NULL AND email_verified_at IS NOT NULL FOR SHARE`, id).Scan(&present)
	if r.Error != nil {
		return fmt.Errorf("read actor availability: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrForbidden
	}
	return nil
}

// CheckPlatformSession reuses current session checks and adds exact platform epoch authority.
func (s *Store) CheckPlatformSession(ctx context.Context, a application.Principal, clock func() time.Time) error {
	if err := s.CheckSession(ctx, a, clock); err != nil {
		return err
	}
	u, err := (&authTx{db: s.db.WithContext(ctx)}).AccountByID(a.ID)
	if err != nil {
		return err
	}
	if u.PlatformRole != "platform_admin" || u.PlatformEpoch != a.PlatformEpoch {
		return application.ErrForbidden
	}
	return nil
}
