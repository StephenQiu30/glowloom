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

func TestEnableUserRestoresAccessWithoutRevivingOldSessions(t *testing.T) {
	user := domain.User{Status: domain.StatusDisabled, SessionEpoch: 2, Revision: 3,
		FailedLoginCount: 5, LockedUntil: time.Now().Add(time.Hour)}
	if err := user.Enable(); err != nil || user.Status != domain.StatusActive ||
		user.SessionEpoch != 3 || user.Revision != 4 || user.FailedLoginCount != 0 ||
		!user.LockedUntil.IsZero() || user.SessionMatches(1) || user.SessionMatches(2) {
		t.Fatalf("enabled account state: status %s, epoch %d, revision %d, error %v",
			user.Status, user.SessionEpoch, user.Revision, err)
	}
	if err := user.Enable(); !errors.Is(err, domain.ErrAccountActive) || user.Revision != 4 {
		t.Fatalf("repeat enable = %v, revision %d", err, user.Revision)
	}
}

func TestResetPasswordRequiresNewSecretAndRevokesOldSessions(t *testing.T) {
	oldHash, err := domain.HashPassword("initialPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []domain.Status{domain.StatusActive, domain.StatusDisabled} {
		user := domain.User{Status: status, PasswordHash: oldHash, MustChangePassword: false,
			SessionEpoch: 4, Revision: 7, FailedLoginCount: 4, LockedUntil: time.Now().Add(time.Hour),
			PasswordChangedAt: time.Now()}
		if err := user.ResetPassword("replacementPassword456"); err != nil ||
			!domain.VerifyPassword(user.PasswordHash, "replacementPassword456") ||
			domain.VerifyPassword(user.PasswordHash, "initialPassword123") ||
			!user.MustChangePassword || user.SessionEpoch != 5 || user.Revision != 8 ||
			user.FailedLoginCount != 0 || !user.LockedUntil.IsZero() || !user.PasswordChangedAt.IsZero() ||
			user.Status != status {
			t.Fatalf("reset state: status %s, epoch %d, revision %d, error %v",
				user.Status, user.SessionEpoch, user.Revision, err)
		}
	}
	user := domain.User{Status: domain.StatusActive, PasswordHash: oldHash, SessionEpoch: 1, Revision: 1}
	if err := user.ResetPassword("initialPassword123"); !errors.Is(err, domain.ErrPasswordReused) ||
		user.PasswordHash != oldHash || user.SessionEpoch != 1 {
		t.Fatalf("reused password reset = %v, epoch %d", err, user.SessionEpoch)
	}
	if err := user.ResetPassword("short"); !errors.Is(err, domain.ErrWeakPassword) {
		t.Fatalf("weak password reset = %v", err)
	}
}

type lifecycleStoreStub struct {
	result  domain.User
	err     error
	events  []identityapp.OutboxEvent
	newHash string
	calls   int
}

func (s *lifecycleStoreStub) EnableWithEvents(_ context.Context, _, _, _ uuid.UUID, _ int64, events []identityapp.OutboxEvent) (domain.User, error) {
	s.calls++
	s.events = events
	return s.result, s.err
}

func (s *lifecycleStoreStub) FindByID(_ context.Context, _, _ uuid.UUID) (domain.User, error) {
	return s.result, s.err
}

func (s *lifecycleStoreStub) ResetPasswordWithEvents(_ context.Context, _, _ uuid.UUID, user domain.User, _ string, events []identityapp.OutboxEvent) (domain.User, error) {
	s.calls++
	s.events = events
	s.newHash = user.PasswordHash
	return user, s.err
}

func TestEnableUserCommandWritesSafeEvents(t *testing.T) {
	orgID, actorID, targetID := uuid.New(), uuid.New(), uuid.New()
	store := &lifecycleStoreStub{result: domain.User{ID: targetID, OrgID: orgID,
		Status: domain.StatusActive, Revision: 2, SessionEpoch: 3}}
	actor := identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleAdmin}
	result, err := identityapp.NewEnableUserCommand(store, time.Now).Execute(t.Context(), actor,
		identityapp.EnableUserInput{TargetID: targetID, ExpectedRevision: 1, RequestID: "enable-request"})
	if err != nil || result.ID != targetID || result.Status != domain.StatusActive || result.Revision != 2 || store.calls != 1 {
		t.Fatalf("enable result %+v, calls %d, error %v", result, store.calls, err)
	}
	checkLifecycleAudit(t, store.events, orgID, actorID, targetID, "enabled", "user.enabled", "enable-request")
	for _, invalid := range []identityapp.Principal{
		{ID: actorID, OrgID: orgID, Role: domain.RoleProducer},
		{ID: actorID, OrgID: orgID, Role: domain.RoleAdmin, MustChangePassword: true},
	} {
		if _, err := identityapp.NewEnableUserCommand(store, time.Now).Execute(t.Context(), invalid,
			identityapp.EnableUserInput{TargetID: targetID, ExpectedRevision: 1, RequestID: "enable-request"}); !errors.Is(err, identityapp.ErrForbidden) {
			t.Fatalf("untrusted enable = %v", err)
		}
	}
}

func TestResetPasswordCommandNeverEmitsPassword(t *testing.T) {
	orgID, actorID, targetID := uuid.New(), uuid.New(), uuid.New()
	oldHash, err := domain.HashPassword("initialPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	store := &lifecycleStoreStub{result: domain.User{ID: targetID, OrgID: orgID,
		Status: domain.StatusActive, PasswordHash: oldHash, Revision: 1, SessionEpoch: 1}}
	actor := identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleAdmin}
	result, err := identityapp.NewResetPasswordCommand(store, time.Now).Execute(t.Context(), actor,
		identityapp.ResetPasswordInput{TargetID: targetID, ExpectedRevision: 1,
			NewPassword: "replacementPassword456", RequestID: "reset-request"})
	if err != nil || result.ID != targetID || result.Revision != 2 || !result.MustChangePassword || store.calls != 1 {
		t.Fatalf("reset result %+v, calls %d, error %v", result, store.calls, err)
	}
	checkLifecycleAudit(t, store.events, orgID, actorID, targetID, "password_reset", "user.password_reset", "reset-request")
	for _, event := range store.events {
		if containsSecret(event.Payload, "replacementPassword456", oldHash, store.newHash) {
			t.Fatal("reset event exposes password material")
		}
	}
	if _, err := identityapp.NewResetPasswordCommand(store, time.Now).Execute(t.Context(), actor,
		identityapp.ResetPasswordInput{TargetID: targetID, ExpectedRevision: 1,
			NewPassword: "initialPassword123", RequestID: "reset-request"}); !errors.Is(err, domain.ErrPasswordReused) {
		t.Fatalf("reused reset password = %v", err)
	}
}

func TestLifecycleCommandsRejectUntrustedInputAndPreserveConflicts(t *testing.T) {
	orgID, actorID, targetID := uuid.New(), uuid.New(), uuid.New()
	actor := identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleAdmin}
	store := &lifecycleStoreStub{result: domain.User{ID: targetID, OrgID: orgID,
		Status: domain.StatusDisabled, Revision: 2, SessionEpoch: 2}}
	if _, err := identityapp.NewEnableUserCommand(store, time.Now).Execute(t.Context(), actor,
		identityapp.EnableUserInput{TargetID: targetID, RequestID: "enable"}); !errors.Is(err, identityapp.ErrInvalidEnableUser) || store.calls != 0 {
		t.Fatalf("missing enable revision: error %v, calls %d", err, store.calls)
	}
	if _, err := identityapp.NewResetPasswordCommand(store, time.Now).Execute(t.Context(),
		identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleProducer},
		identityapp.ResetPasswordInput{TargetID: targetID, ExpectedRevision: 2,
			NewPassword: "replacementPassword456", RequestID: "reset"}); !errors.Is(err, identityapp.ErrForbidden) || store.calls != 0 {
		t.Fatalf("producer reset: error %v, calls %d", err, store.calls)
	}
	store.err = domain.ErrRevisionConflict
	if _, err := identityapp.NewEnableUserCommand(store, time.Now).Execute(t.Context(), actor,
		identityapp.EnableUserInput{TargetID: targetID, ExpectedRevision: 2, RequestID: "enable"}); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatalf("enable revision conflict: %v", err)
	}
}

func checkLifecycleAudit(t *testing.T, events []identityapp.OutboxEvent, orgID, actorID, targetID uuid.UUID, change, action, requestID string) {
	t.Helper()
	if len(events) != 2 || events[0].Topic != "lanverse.identity.user_changed.v1" ||
		events[0].PartitionKey != orgID.String() || events[1].Topic != "lanverse.audit.recorded.v1" {
		t.Fatalf("lifecycle event count or routing invalid: %d", len(events))
	}
	if !containsSecret(events[0].Payload, change) {
		t.Fatalf("identity event lacks change %s", change)
	}
	record, err := auditapp.NewIdentityActionParser().Parse(inbox.Record{
		Topic: events[1].Topic, Key: []byte(events[1].PartitionKey), Value: events[1].Payload,
	})
	if err != nil || record.Action != action || record.ActorID == nil || *record.ActorID != actorID ||
		record.ObjectID != targetID.String() || record.RequestID != requestID {
		t.Fatalf("lifecycle audit action %s, actor %v, object %s, error %v",
			record.Action, record.ActorID, record.ObjectID, err)
	}
}

func containsSecret(payload []byte, values ...string) bool {
	for _, value := range values {
		if value != "" && strings.Contains(string(payload), value) {
			return true
		}
	}
	return false
}
