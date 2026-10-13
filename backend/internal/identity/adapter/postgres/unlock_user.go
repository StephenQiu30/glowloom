package postgres

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// UnlockWithEvents clears both lock representations and commits the audit atomically.
// The guard is locked before the target account, matching the authentication lock order.
func (s *Store) UnlockWithEvents(ctx context.Context, actorID, orgID, targetID uuid.UUID, revision int64, now time.Time, events []application.OutboxEvent) (domain.User, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil || targetID == uuid.Nil || actorID == targetID || revision < 1 || revision >= math.MaxInt32 || now.IsZero() || !validLifecycleEvents(actorID, orgID, targetID, revision, "unlocked", "user.unlocked", events) {
		return domain.User{}, ErrInvalidUser
	}
	var saved domain.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAccountChangeOrg(tx, orgID); err != nil {
			return err
		}
		// Login names are immutable. Read scope before touching the global login guard.
		var login string
		found := tx.Raw(`SELECT login_name FROM identity."user" WHERE org_id=?::uuid AND id=?::uuid AND NOT is_delete`, orgID.String(), targetID.String()).Scan(&login)
		if found.Error != nil {
			return fmt.Errorf("read unlock login: %w", found.Error)
		}
		if found.RowsAffected != 1 {
			return ErrNotFound
		}
		key, err := domain.NormalizeLogin(login)
		if err != nil {
			return ErrInvalidUser
		}
		auth := &authTx{db: tx}
		guard, err := auth.Guard(key)
		if err != nil {
			return err
		}
		if err := requireCurrentAdmin(tx, orgID, actorID); err != nil {
			return err
		}
		found = tx.Raw(`SELECT id,org_id,status,revision,locked_until,failed_login_count FROM identity."user" WHERE org_id=?::uuid AND id=?::uuid AND NOT is_delete FOR UPDATE`, orgID.String(), targetID.String()).Scan(&saved)
		if found.Error != nil {
			return fmt.Errorf("lock account for unlock: %w", found.Error)
		}
		if found.RowsAffected != 1 {
			return ErrNotFound
		}
		if saved.Revision != revision {
			return ErrRevisionConflict
		}
		if !guard.Locked(now) && !now.Before(saved.LockedUntil) {
			return domain.ErrAccountUnchanged
		}
		audit, err := parseIdentityAudit(events[1])
		if err != nil || string(audit.Before) != `{"login_locked":true}` || string(audit.After) != `{"login_locked":false}` {
			return ErrInvalidUser
		}
		if err := auth.SaveGuard(key, domain.LoginGuard{}); err != nil {
			return err
		}
		updated := tx.Exec(`UPDATE identity."user" SET failed_login_count=0, locked_until=NULL, revision=revision+1,update_time=now() WHERE org_id=?::uuid AND id=?::uuid AND revision=? AND NOT is_delete`, orgID.String(), targetID.String(), revision)
		if updated.Error != nil {
			return fmt.Errorf("unlock account: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		if err := insertOutboxEvents(tx, events); err != nil {
			return err
		}
		saved.Revision++
		saved.FailedLoginCount = 0
		saved.LockedUntil = time.Time{}
		return nil
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("unlock account transaction: %w", err)
	}
	return saved, nil
}

var _ application.UnlockUserStore = (*Store)(nil)
