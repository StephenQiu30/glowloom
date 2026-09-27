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

// EnableWithEvents restores a disabled account and writes both events atomically.
func (s *Store) EnableWithEvents(ctx context.Context, actorID, orgID, targetID uuid.UUID, expectedRevision int64, events []application.OutboxEvent) (domain.User, error) {
	if s == nil || s.db == nil || actorID == uuid.Nil || orgID == uuid.Nil || targetID == uuid.Nil ||
		expectedRevision < 1 || expectedRevision >= math.MaxInt32 ||
		!validLifecycleEvents(actorID, orgID, targetID, expectedRevision, "enabled", "user.enabled", events) {
		return domain.User{}, ErrInvalidUser
	}
	var enabled domain.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAccountChangeOrg(tx, orgID); err != nil {
			return err
		}
		if err := requireCurrentAdmin(tx, orgID, actorID); err != nil {
			return err
		}
		var state struct {
			Status       string
			SessionEpoch int64
			Revision     int64
		}
		result := tx.Raw(`
			SELECT status, session_epoch, revision FROM identity."user"
			WHERE org_id = ?::uuid AND id = ?::uuid AND NOT is_delete FOR UPDATE
		`, orgID.String(), targetID.String()).Scan(&state)
		if result.Error != nil {
			return fmt.Errorf("read account for enable: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		if state.Revision != expectedRevision {
			return ErrRevisionConflict
		}
		enabled = domain.User{ID: targetID, OrgID: orgID, Status: domain.Status(state.Status),
			SessionEpoch: state.SessionEpoch, Revision: state.Revision}
		if err := enabled.Enable(); err != nil {
			return err
		}
		audit, err := parseIdentityAudit(events[1])
		if err != nil || string(audit.Before) != `{"status":"disabled"}` ||
			string(audit.After) != `{"status":"active"}` {
			return ErrInvalidUser
		}
		updated := tx.Exec(`
			UPDATE identity."user"
			SET status = 'active', session_epoch = ?, failed_login_count = 0,
			    locked_until = NULL, revision = revision + 1, update_time = now()
			WHERE org_id = ?::uuid AND id = ?::uuid AND revision = ? AND NOT is_delete
		`, enabled.SessionEpoch, orgID.String(), targetID.String(), expectedRevision)
		if updated.Error != nil {
			return fmt.Errorf("enable account: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		return insertOutboxEvents(tx, events)
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("enable account transaction: %w", err)
	}
	return enabled, nil
}

func requireCurrentAdmin(tx *gorm.DB, orgID, actorID uuid.UUID) error {
	var actor struct {
		Role               string
		Status             string
		MustChangePassword bool
	}
	result := tx.Raw(`
		SELECT role, status, must_change_password FROM identity."user"
		WHERE org_id = ?::uuid AND id = ?::uuid AND NOT is_delete FOR SHARE
	`, orgID.String(), actorID.String()).Scan(&actor)
	if result.Error != nil {
		return fmt.Errorf("read account administrator: %w", result.Error)
	}
	if result.RowsAffected != 1 || actor.Role != string(domain.RoleAdmin) ||
		actor.Status != string(domain.StatusActive) || actor.MustChangePassword {
		return application.ErrForbidden
	}
	return nil
}

func validLifecycleEvents(actorID, orgID, targetID uuid.UUID, revision int64, change, action string, events []application.OutboxEvent) bool {
	if !validCreateEvents(orgID, events) || events[0].Topic != "lanverse.identity.user_changed.v1" ||
		events[1].Topic != "lanverse.audit.recorded.v1" || events[0].ID == events[1].ID {
		return false
	}
	var changed struct {
		Actor struct {
			Kind string    `json:"kind"`
			ID   uuid.UUID `json:"id"`
		} `json:"actor"`
		Aggregate struct {
			Type     string    `json:"type"`
			ID       uuid.UUID `json:"id"`
			Revision int64     `json:"revision"`
		} `json:"aggregate"`
		Data struct {
			Change string `json:"change"`
		} `json:"data"`
	}
	if err := json.Unmarshal(events[0].Payload, &changed); err != nil ||
		changed.Actor.Kind != "user" || changed.Actor.ID != actorID ||
		changed.Aggregate.Type != "user" || changed.Aggregate.ID != targetID ||
		changed.Aggregate.Revision != revision+1 || changed.Data.Change != change {
		return false
	}
	audit, err := parseIdentityAudit(events[1])
	return err == nil && audit.OrgID == orgID && audit.ActorKind == "user" &&
		audit.ActorID != nil && *audit.ActorID == actorID && audit.Action == action &&
		audit.ObjectType == "user" && audit.ObjectID == targetID.String()
}

var _ application.EnableUserStore = (*Store)(nil)
