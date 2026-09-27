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

// ResetPasswordWithEvents commits a replacement hash, new epoch, first-login
// requirement, and both events under the account administrator lock.
func (s *Store) ResetPasswordWithEvents(ctx context.Context, actorID, orgID uuid.UUID, next domain.User, previousHash string, events []application.OutboxEvent) (domain.User, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil || next.ID == uuid.Nil ||
		next.OrgID != orgID || next.Revision < 2 || next.Revision > math.MaxInt32 ||
		next.SessionEpoch < 2 || next.SessionEpoch > math.MaxInt32 || !next.MustChangePassword ||
		!domain.ValidPasswordHash(previousHash) || !domain.ValidPasswordHash(next.PasswordHash) ||
		previousHash == next.PasswordHash ||
		!validLifecycleEvents(actorID, orgID, next.ID, next.Revision-1, "password_reset", "user.password_reset", events) {
		return domain.User{}, ErrInvalidUser
	}
	var reset domain.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAccountChangeOrg(tx, orgID); err != nil {
			return err
		}
		if err := requireCurrentAdmin(tx, orgID, actorID); err != nil {
			return err
		}
		var state struct {
			PasswordHash       string
			Status             string
			MustChangePassword bool
			SessionEpoch       int64
			Revision           int64
		}
		result := tx.Raw(`
			SELECT password_hash, status, must_change_password, session_epoch, revision
			FROM identity."user" WHERE org_id = ?::uuid AND id = ?::uuid AND NOT is_delete
			FOR UPDATE
		`, orgID.String(), next.ID.String()).Scan(&state)
		if result.Error != nil {
			return fmt.Errorf("read account for password reset: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		if state.Revision != next.Revision-1 || state.SessionEpoch != next.SessionEpoch-1 ||
			state.PasswordHash != previousHash || state.Status != string(next.Status) {
			return ErrRevisionConflict
		}
		audit, err := parseIdentityAudit(events[1])
		if err != nil || string(audit.Before) != fmt.Sprintf(`{"must_change_password":%t}`, state.MustChangePassword) ||
			string(audit.After) != `{"must_change_password":true}` {
			return ErrInvalidUser
		}
		updated := tx.Exec(`
			UPDATE identity."user"
			SET password_hash = ?, must_change_password = true,
			    password_changed_at = NULL, failed_login_count = 0, locked_until = NULL,
			    session_epoch = session_epoch + 1, revision = revision + 1, update_time = now()
			WHERE org_id = ?::uuid AND id = ?::uuid AND revision = ? AND NOT is_delete
		`, next.PasswordHash, orgID.String(), next.ID.String(), state.Revision)
		if updated.Error != nil {
			return fmt.Errorf("reset account password: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		if err := insertOutboxEvents(tx, events); err != nil {
			return err
		}
		reset, err = NewStore(tx).FindByID(ctx, orgID, next.ID)
		if err != nil {
			return fmt.Errorf("read reset account: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("reset account password transaction: %w", err)
	}
	return reset, nil
}

var _ application.ResetPasswordStore = (*Store)(nil)
