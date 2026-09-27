package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	auditdomain "github.com/StephenQiu30/lanverse/backend/internal/audit/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

// SaveLoginAttempt commits the updated account state and its audit events as
// one transaction. A failed audit insert rolls back the lock or success state.
func (s *Store) SaveLoginAttempt(ctx context.Context, user domain.User, expectedRevision int64, events []application.OutboxEvent) error {
	if user.ID == uuid.Nil || user.OrgID == uuid.Nil || len(events) < 1 || len(events) > 2 {
		return ErrInvalidUser
	}
	for index, event := range events {
		audit, err := parseIdentityAudit(event)
		if err != nil || audit.OrgID != user.OrgID || audit.ObjectType != "user" || audit.ObjectID != user.ID.String() {
			return ErrInvalidUser
		}
		if index == 0 && audit.Action != "auth.login_succeeded" && audit.Action != "auth.login_failed" {
			return ErrInvalidUser
		}
		if index == 1 && (events[0].ID == event.ID || audit.Action != "auth.locked") {
			return ErrInvalidUser
		}
	}
	if len(events) == 2 {
		first, _ := parseIdentityAudit(events[0])
		if first.Action != "auth.login_failed" {
			return ErrInvalidUser
		}
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := NewStore(tx).SaveLoginState(ctx, user, expectedRevision); err != nil {
			return err
		}
		return insertOutboxEvents(tx, events)
	}); err != nil {
		return fmt.Errorf("save login attempt transaction: %w", err)
	}
	return nil
}

// RecordLoginAudit persists a login denial without an account-state update.
func (s *Store) RecordLoginAudit(ctx context.Context, event application.OutboxEvent) error {
	audit, err := parseIdentityAudit(event)
	if err != nil || (audit.Action != "auth.login_failed" && audit.Action != "auth.locked") {
		return ErrInvalidUser
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return insertOutboxEvents(tx, []application.OutboxEvent{event})
	}); err != nil {
		return fmt.Errorf("record login audit transaction: %w", err)
	}
	return nil
}

// RecordLogoutAudit writes the audit event only after Redis confirms revocation.
func (s *Store) RecordLogoutAudit(ctx context.Context, event application.OutboxEvent) error {
	audit, err := parseIdentityAudit(event)
	if err != nil || audit.Action != "auth.logout" || audit.ActorKind != "user" ||
		audit.ActorID == nil || audit.ObjectType != "user" || audit.ObjectID != audit.ActorID.String() {
		return ErrInvalidUser
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return insertOutboxEvents(tx, []application.OutboxEvent{event})
	}); err != nil {
		return fmt.Errorf("record logout audit transaction: %w", err)
	}
	return nil
}

func parseIdentityAudit(event application.OutboxEvent) (auditdomain.Record, error) {
	if event.ID == uuid.Nil {
		return auditdomain.Record{}, ErrInvalidUser
	}
	parsed, err := auditapp.NewIdentityActionParser().Parse(inbox.Record{
		Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
	})
	if err != nil || parsed.ID != event.ID {
		return auditdomain.Record{}, ErrInvalidUser
	}
	return parsed, nil
}

var _ application.LoginAccountStore = (*Store)(nil)
