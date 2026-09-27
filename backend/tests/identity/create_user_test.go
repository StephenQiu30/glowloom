package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

type createUserStoreSpy struct {
	called bool
	actor  uuid.UUID
	user   domain.User
	events []identityapp.OutboxEvent
	err    error
}

func (s *createUserStoreSpy) CreateWithEvents(_ context.Context, actor uuid.UUID, user domain.User, events []identityapp.OutboxEvent) (domain.User, error) {
	s.called, s.actor, s.user, s.events = true, actor, user, events
	if s.err != nil {
		return domain.User{}, s.err
	}
	user.Status = domain.StatusActive
	user.MustChangePassword = true
	user.SessionEpoch = 1
	user.Revision = 1
	return user, nil
}

func TestCreateUserCommandWritesOnlySafeAccountEvents(t *testing.T) {
	store := &createUserStoreSpy{}
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	command := identityapp.NewCreateUserCommand(store, func() time.Time { return now })
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: domain.RoleAdmin}
	result, err := command.Execute(t.Context(), actor, identityapp.CreateUserInput{
		LoginName: "alice", DisplayName: "Alice", Role: domain.RoleProducer,
		InitialPassword: "initialPassword123", RequestID: "request-123",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if !store.called || store.actor != actor.ID || store.user.OrgID != actor.OrgID ||
		!domain.VerifyPassword(store.user.PasswordHash, "initialPassword123") ||
		result.ID != store.user.ID || result.OrgID != actor.OrgID || result.Revision != 1 ||
		!result.MustChangePassword {
		t.Fatalf("created account scope or defaults incorrect: %+v", result)
	}
	if len(store.events) != 2 {
		t.Fatalf("outbox events = %d, want two", len(store.events))
	}
	var auditCount, identityCount int
	for _, event := range store.events {
		if event.ID == uuid.Nil || event.PartitionKey != actor.OrgID.String() ||
			strings.Contains(string(event.Payload), "initialPassword123") ||
			strings.Contains(string(event.Payload), store.user.PasswordHash) {
			t.Fatal("outbox event has invalid routing or exposed credential")
		}
		var envelope struct {
			EventID   string `json:"event_id"`
			EventType string `json:"event_type"`
			OrgID     string `json:"org_id"`
		}
		if err := json.Unmarshal(event.Payload, &envelope); err != nil ||
			envelope.EventID != event.ID.String() || envelope.EventType != event.Topic ||
			envelope.OrgID != actor.OrgID.String() {
			t.Fatalf("invalid account event envelope: %v", err)
		}
		switch event.Topic {
		case "lanverse.audit.recorded.v1":
			auditCount++
			parsed, err := auditapp.NewIdentityActionParser().Parse(inbox.Record{
				Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
			})
			if err != nil || parsed.Action != "user.created" || parsed.ActorID == nil ||
				*parsed.ActorID != actor.ID || parsed.ObjectID != result.ID.String() ||
				parsed.RequestID != "request-123" {
				t.Fatalf("audit event = %+v, error %v", parsed, err)
			}
		case "lanverse.identity.user_changed.v1":
			identityCount++
		default:
			t.Fatalf("unexpected account topic %q", event.Topic)
		}
	}
	if auditCount != 1 || identityCount != 1 {
		t.Fatalf("audit/identity event counts = %d/%d", auditCount, identityCount)
	}
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), store.user.PasswordHash) {
		t.Fatalf("result exposed password hash: %v", err)
	}
}

func TestCreateUserCommandRejectsUnauthorizedAndWeakPassword(t *testing.T) {
	store := &createUserStoreSpy{}
	command := identityapp.NewCreateUserCommand(store, time.Now)
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: domain.RoleProducer}
	input := identityapp.CreateUserInput{
		LoginName: "alice", DisplayName: "Alice", Role: domain.RoleProducer,
		InitialPassword: "initialPassword123", RequestID: "request-123",
	}
	if _, err := command.Execute(t.Context(), actor, input); !errors.Is(err, identityapp.ErrForbidden) || store.called {
		t.Fatalf("producer create = %v, store called %t", err, store.called)
	}
	actor.Role = domain.RoleAdmin
	actor.MustChangePassword = true
	if _, err := command.Execute(t.Context(), actor, input); !errors.Is(err, identityapp.ErrForbidden) || store.called {
		t.Fatalf("first-login create = %v, store called %t", err, store.called)
	}
	actor.MustChangePassword = false
	input.InitialPassword = "weak"
	if _, err := command.Execute(t.Context(), actor, input); !errors.Is(err, domain.ErrWeakPassword) || store.called {
		t.Fatalf("weak-password create = %v, store called %t", err, store.called)
	}
}
