package identity_test

import (
	"context"
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

type changePasswordSessionsStub struct {
	logoutSessionsStub
	createErr error
	created   bool
	epoch     int64
}

func (s *changePasswordSessionsStub) Create(_ context.Context, _, _ uuid.UUID, epoch int64) (string, domain.Session, error) {
	s.created, s.epoch = true, epoch
	return "replacement-token", domain.Session{SessionEpoch: epoch}, s.createErr
}

type changePasswordStoreStub struct {
	saved  domain.User
	events []identityapp.OutboxEvent
	err    error
	called bool
}

func (s *changePasswordStoreStub) ChangePasswordWithEvents(_ context.Context, user domain.User, newHash string, events []identityapp.OutboxEvent) (domain.User, error) {
	s.called, s.events = true, events
	if !domain.VerifyPassword(newHash, "newPassword123") || user.PasswordHash == newHash {
		return domain.User{}, errors.New("new hash does not match the new password")
	}
	return s.saved, s.err
}

func changePasswordFixture(t *testing.T) (*identityapp.ChangePasswordCommand, *changePasswordSessionsStub, *changePasswordStoreStub) {
	t.Helper()
	orgID, userID := uuid.New(), uuid.New()
	hash, err := domain.HashPassword("oldPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	user := domain.User{
		ID: userID, OrgID: orgID, LoginName: "alice", Role: domain.RoleProducer,
		Status: domain.StatusActive, PasswordHash: hash, MustChangePassword: true,
		SessionEpoch: 4, Revision: 7,
	}
	sessions := &changePasswordSessionsStub{logoutSessionsStub: logoutSessionsStub{
		session: domain.Session{OrgID: orgID, UserID: userID, SessionEpoch: 4},
	}}
	accounts := accountReaderFunc(func(context.Context, uuid.UUID, uuid.UUID) (domain.User, error) {
		return user, nil
	})
	store := &changePasswordStoreStub{saved: domain.User{
		ID: userID, OrgID: orgID, Status: domain.StatusActive,
		MustChangePassword: false, SessionEpoch: 5, Revision: 8,
	}}
	command := identityapp.NewChangePasswordCommand(identityapp.NewAuthenticator(accounts, sessions), accounts, sessions, store, time.Now)
	return command, sessions, store
}

func TestChangePasswordReissuesOnlyCurrentSessionAndAudits(t *testing.T) {
	command, sessions, store := changePasswordFixture(t)
	result, err := command.Execute(t.Context(), identityapp.ChangePasswordInput{
		Token: "current-token", CurrentPassword: "oldPassword123", NewPassword: "newPassword123",
		RequestID: "change-request", ClientIP: "::ffff:127.0.0.1",
	})
	if err != nil || result.Token != "replacement-token" || result.MustChangePassword ||
		result.Revision != 8 || !sessions.created || sessions.epoch != 5 || sessions.destroyed ||
		!store.called || len(store.events) != 2 {
		t.Fatalf("result = %+v, sessions = %+v, store = %+v, error %v", result, sessions, store, err)
	}
	for _, event := range store.events {
		if strings.Contains(string(event.Payload), "oldPassword123") ||
			strings.Contains(string(event.Payload), "newPassword123") ||
			strings.Contains(string(event.Payload), "replacement-token") {
			t.Fatal("account event exposed password or token")
		}
		if event.Topic == "lanverse.audit.recorded.v1" {
			record, err := auditapp.NewIdentityActionParser().Parse(inbox.Record{
				Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
			})
			if err != nil || record.Action != "user.password_changed" || record.IP != "127.0.0.1" {
				t.Fatalf("password audit = %+v, error %v", record, err)
			}
		}
	}
}

func TestChangePasswordRejectsIncorrectOrReusedPasswordBeforeSessionCreation(t *testing.T) {
	for _, tc := range []struct {
		name, current, next string
		want                error
	}{
		{"incorrect current", "incorrect123", "newPassword123", identityapp.ErrCurrentPasswordInvalid},
		{"reused password", "oldPassword123", "oldPassword123", domain.ErrPasswordReused},
		{"weak password", "oldPassword123", "short", domain.ErrWeakPassword},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command, sessions, store := changePasswordFixture(t)
			result, err := command.Execute(t.Context(), identityapp.ChangePasswordInput{
				Token: "current-token", CurrentPassword: tc.current, NewPassword: tc.next,
				RequestID: "change-request", ClientIP: "127.0.0.1",
			})
			if !errors.Is(err, tc.want) || result.Token != "" || sessions.created || store.called {
				t.Fatalf("result = %+v, created %t, store called %t, error %v", result, sessions.created, store.called, err)
			}
		})
	}
}

func TestChangePasswordCompensatesFailedTransaction(t *testing.T) {
	command, sessions, store := changePasswordFixture(t)
	sentinel := errors.New("audit insert failed")
	store.err = sentinel
	result, err := command.Execute(t.Context(), identityapp.ChangePasswordInput{
		Token: "current-token", CurrentPassword: "oldPassword123", NewPassword: "newPassword123",
		RequestID: "change-request", ClientIP: "127.0.0.1",
	})
	if !errors.Is(err, sentinel) || result.Token != "" || !sessions.created || !sessions.destroyed {
		t.Fatalf("result = %+v, created %t, destroyed %t, error %v", result, sessions.created, sessions.destroyed, err)
	}
}

func TestChangePasswordStopsWhenSessionCreationFails(t *testing.T) {
	command, sessions, store := changePasswordFixture(t)
	sessions.createErr = errors.New("Redis unavailable")
	result, err := command.Execute(t.Context(), identityapp.ChangePasswordInput{
		Token: "current-token", CurrentPassword: "oldPassword123", NewPassword: "newPassword123",
		RequestID: "change-request", ClientIP: "127.0.0.1",
	})
	if !errors.Is(err, identityapp.ErrAuthenticationUnavailable) || result.Token != "" || store.called {
		t.Fatalf("result = %+v, store called %t, error %v", result, store.called, err)
	}
}
