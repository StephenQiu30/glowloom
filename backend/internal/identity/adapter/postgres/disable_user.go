package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// DisableWithEvents atomically revokes the target account's session epoch and
// writes both required events after rechecking the administrator in PostgreSQL.
func (s *Store) DisableWithEvents(ctx context.Context, actorID, orgID, targetID uuid.UUID, expectedRevision int64, events []application.OutboxEvent) (domain.User, error) {
	if actorID == uuid.Nil || orgID == uuid.Nil || targetID == uuid.Nil || expectedRevision < 1 ||
		!validDisableEvents(actorID, orgID, targetID, expectedRevision, events) {
		return domain.User{}, ErrInvalidUser
	}
	var disabled domain.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAccountChangeOrg(tx, orgID); err != nil {
			return err
		}
		if err := requireCurrentAdmin(tx, orgID, actorID); err != nil {
			return err
		}
		var err error
		disabled, err = disableUserInTx(tx, orgID, targetID, expectedRevision)
		if err != nil {
			return err
		}
		return insertOutboxEvents(tx, events)
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("disable account transaction: %w", err)
	}
	return disabled, nil
}

func validDisableEvents(actorID, orgID, targetID uuid.UUID, expectedRevision int64, events []application.OutboxEvent) bool {
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
		changed.Aggregate.Revision != expectedRevision+1 || changed.Data.Change != "disabled" {
		return false
	}
	audit, err := parseIdentityAudit(events[1])
	return err == nil && audit.OrgID == orgID && audit.ActorKind == "user" &&
		audit.ActorID != nil && *audit.ActorID == actorID && audit.Action == "user.disabled" &&
		audit.ObjectType == "user" && audit.ObjectID == targetID.String() &&
		string(audit.Before) == `{"status":"active"}` && string(audit.After) == `{"status":"disabled"}`
}

var _ application.DisableUserStore = (*Store)(nil)
