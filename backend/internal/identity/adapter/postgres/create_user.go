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

// CreateWithEvents revalidates the administrator and commits the account plus
// identity and audit Outbox events in one PostgreSQL transaction.
func (s *Store) CreateWithEvents(ctx context.Context, actorID uuid.UUID, user domain.User, events []application.OutboxEvent) (domain.User, error) {
	if actorID == uuid.Nil || user.ID == uuid.Nil || user.OrgID == uuid.Nil || !validCreateEvents(user.OrgID, events) {
		return domain.User{}, ErrInvalidUser
	}
	var created domain.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var actor struct {
			Role               string
			Status             string
			MustChangePassword bool
		}
		result := tx.Raw(`
			SELECT role, status, must_change_password
			FROM identity."user"
			WHERE org_id = ?::uuid AND id = ?::uuid AND NOT is_delete
			FOR SHARE
		`, user.OrgID.String(), actorID.String()).Scan(&actor)
		if result.Error != nil {
			return fmt.Errorf("read account administrator: %w", result.Error)
		}
		if result.RowsAffected != 1 || actor.Role != string(domain.RoleAdmin) ||
			actor.Status != string(domain.StatusActive) || actor.MustChangePassword {
			return application.ErrForbidden
		}
		transactionStore := NewStore(tx)
		if err := transactionStore.Create(ctx, user); err != nil {
			return err
		}
		if err := insertOutboxEvents(tx, events); err != nil {
			return err
		}
		var err error
		created, err = transactionStore.FindByID(ctx, user.OrgID, user.ID)
		if err != nil {
			return fmt.Errorf("read created account: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("create account transaction: %w", err)
	}
	return created, nil
}

func insertOutboxEvents(tx *gorm.DB, events []application.OutboxEvent) error {
	for _, event := range events {
		inserted := tx.Exec(`
			INSERT INTO infra.outbox (id, topic, partition_key, payload)
			VALUES (?::uuid, ?, ?, ?::jsonb)
		`, event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
		if inserted.Error != nil {
			return fmt.Errorf("write account outbox event %s: %w", event.Topic, inserted.Error)
		}
		if inserted.RowsAffected != 1 {
			return fmt.Errorf("write account outbox event %s: inserted %d rows", event.Topic, inserted.RowsAffected)
		}
	}
	return nil
}

func validCreateEvents(orgID uuid.UUID, events []application.OutboxEvent) bool {
	if len(events) != 2 {
		return false
	}
	seen := make(map[string]bool, len(events))
	for _, event := range events {
		if event.ID == uuid.Nil || event.PartitionKey != orgID.String() || seen[event.Topic] || !json.Valid(event.Payload) {
			return false
		}
		seen[event.Topic] = true
		var envelope struct {
			EventID   string `json:"event_id"`
			EventType string `json:"event_type"`
			OrgID     string `json:"org_id"`
		}
		if err := json.Unmarshal(event.Payload, &envelope); err != nil ||
			envelope.EventID != event.ID.String() || envelope.EventType != event.Topic ||
			envelope.OrgID != orgID.String() {
			return false
		}
	}
	return seen["lanverse.identity.user_changed.v1"] && seen["lanverse.audit.recorded.v1"]
}

var _ application.CreateUserStore = (*Store)(nil)
