package postgres

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// ChangePasswordWithEvents rechecks the durable account and commits the hash,
// session epoch, identity event, and audit event in one transaction.
func (s *Store) ChangePasswordWithEvents(ctx context.Context, user domain.User, newHash string, events []application.OutboxEvent) (domain.User, error) {
	if s == nil || s.db == nil || user.ID == uuid.Nil || user.OrgID == uuid.Nil ||
		!domain.ValidPasswordHash(user.PasswordHash) || !domain.ValidPasswordHash(newHash) ||
		user.PasswordHash == newHash || user.SessionEpoch < 1 || user.SessionEpoch >= math.MaxInt32 ||
		user.Revision < 1 || user.Revision >= math.MaxInt32 ||
		!validCreateEvents(user.OrgID, events) {
		return domain.User{}, ErrInvalidUser
	}
	var changed domain.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct {
			PasswordHash string
			Status       string
			SessionEpoch int64
			Revision     int64
		}
		result := tx.Raw(`
			SELECT password_hash, status, session_epoch, revision
			FROM identity."user"
			WHERE org_id = ?::uuid AND id = ?::uuid AND NOT is_delete
			FOR UPDATE
		`, user.OrgID.String(), user.ID.String()).Scan(&current)
		if result.Error != nil {
			return fmt.Errorf("read account for password change: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		if current.Status != string(domain.StatusActive) {
			return domain.ErrAccountDisabled
		}
		if current.Revision != user.Revision || current.SessionEpoch != user.SessionEpoch ||
			current.PasswordHash != user.PasswordHash {
			return ErrRevisionConflict
		}
		updated := tx.Exec(`
			UPDATE identity."user"
			SET password_hash = ?, must_change_password = false,
			    password_changed_at = now(), failed_login_count = 0, locked_until = NULL,
			    session_epoch = session_epoch + 1, revision = revision + 1, update_time = now()
			WHERE org_id = ?::uuid AND id = ?::uuid AND revision = ? AND NOT is_delete
		`, newHash, user.OrgID.String(), user.ID.String(), user.Revision)
		if updated.Error != nil {
			return fmt.Errorf("change account password: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		if err := insertOutboxEvents(tx, events); err != nil {
			return err
		}
		var err error
		changed, err = NewStore(tx).FindByID(ctx, user.OrgID, user.ID)
		if err != nil {
			return fmt.Errorf("read changed account: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("change password transaction: %w", err)
	}
	return changed, nil
}

var _ application.ChangePasswordStore = (*Store)(nil)
