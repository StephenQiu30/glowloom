package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// UpdateProfileWithEvents rechecks administrator rights and the account patch
// under a single transaction before persisting both required events.
func (s *Store) UpdateProfileWithEvents(ctx context.Context, actorID uuid.UUID, before, after domain.User, events []application.OutboxEvent) (domain.User, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || before.ID == uuid.Nil || before.OrgID == uuid.Nil ||
		before.Revision < 1 || before.Revision >= math.MaxInt32 || after.ID != before.ID || after.OrgID != before.OrgID ||
		after.Revision != before.Revision+1 || after.SessionEpoch != before.SessionEpoch ||
		!validLifecycleEvents(actorID, before.OrgID, before.ID, before.Revision, "updated", "user.updated", events) {
		return domain.User{}, ErrInvalidUser
	}
	var updated domain.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAccountChangeOrg(tx, before.OrgID); err != nil {
			return err
		}
		if err := requireCurrentAdmin(tx, before.OrgID, actorID); err != nil {
			return err
		}
		var state struct {
			DisplayName  string
			Role         string
			Status       string
			SessionEpoch int64
			Revision     int64
		}
		result := tx.Raw(`
			SELECT display_name, role, status, session_epoch, revision FROM identity."user"
			WHERE org_id = ?::uuid AND id = ?::uuid AND NOT is_delete FOR UPDATE
		`, before.OrgID.String(), before.ID.String()).Scan(&state)
		if result.Error != nil {
			return fmt.Errorf("read account for profile update: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		if state.Revision != before.Revision {
			return ErrRevisionConflict
		}
		if state.DisplayName != before.DisplayName || domain.Role(state.Role) != before.Role ||
			domain.Status(state.Status) != before.Status || state.SessionEpoch != before.SessionEpoch {
			return ErrRevisionConflict
		}
		var activeAdmins int64
		if err := tx.Raw(`
			SELECT count(*) FROM identity."user"
			WHERE org_id = ?::uuid AND role = 'admin' AND status = 'active' AND NOT is_delete
		`, before.OrgID.String()).Scan(&activeAdmins).Error; err != nil {
			return fmt.Errorf("count active administrators: %w", err)
		}
		updated = domain.User{ID: before.ID, OrgID: before.OrgID, DisplayName: state.DisplayName,
			Role: domain.Role(state.Role), Status: domain.Status(state.Status),
			SessionEpoch: state.SessionEpoch, Revision: state.Revision}
		if err := updated.UpdateProfile(&after.DisplayName, &after.Role, int(activeAdmins)); err != nil {
			return err
		}
		if updated.DisplayName != after.DisplayName || updated.Role != after.Role || updated.Revision != after.Revision {
			return ErrInvalidUser
		}
		if !validProfileAudit(events[1], before, updated) {
			return ErrInvalidUser
		}
		result = tx.Exec(`
			UPDATE identity."user" SET display_name = ?, role = ?, revision = revision + 1, update_time = now()
			WHERE org_id = ?::uuid AND id = ?::uuid AND revision = ? AND NOT is_delete
		`, updated.DisplayName, string(updated.Role), before.OrgID.String(), before.ID.String(), before.Revision)
		if result.Error != nil {
			return fmt.Errorf("update account profile: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		return insertOutboxEvents(tx, events)
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("update account profile transaction: %w", err)
	}
	return updated, nil
}

func validProfileAudit(event application.OutboxEvent, before, after domain.User) bool {
	audit, err := parseIdentityAudit(event)
	if err != nil {
		return false
	}
	oldSummary, err := json.Marshal(map[string]any{"display_name": before.DisplayName, "role": before.Role})
	if err != nil {
		return false
	}
	newSummary, err := json.Marshal(map[string]any{"display_name": after.DisplayName, "role": after.Role})
	return err == nil && string(audit.Before) == string(oldSummary) && string(audit.After) == string(newSummary)
}

var _ application.UpdateUserStore = (*Store)(nil)
