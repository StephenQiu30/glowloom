package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

type loginStoreStub struct {
	user       domain.User
	findErr    error
	findCalls  int
	saveErr    error
	saved      domain.User
	events     []identityapp.OutboxEvent
	standalone []identityapp.OutboxEvent
	recordErr  error
}

func (s *loginStoreStub) FindByLogin(context.Context, uuid.UUID, string) (domain.User, error) {
	s.findCalls++
	return s.user, s.findErr
}

func (s *loginStoreStub) SaveLoginAttempt(_ context.Context, user domain.User, _ int64, events []identityapp.OutboxEvent) error {
	s.saved, s.events = user, events
	return s.saveErr
}

func (s *loginStoreStub) RecordLoginAudit(_ context.Context, event identityapp.OutboxEvent) error {
	s.standalone = append(s.standalone, event)
	return s.recordErr
}

type loginSessionsStub struct {
	created      bool
	destroyed    bool
	createCalls  int
	destroyCalls int
	createErr    error
}

func (s *loginSessionsStub) Create(context.Context, uuid.UUID, uuid.UUID, int64) (string, domain.Session, error) {
	s.created = true
	s.createCalls++
	if s.createErr != nil {
		return "", domain.Session{}, s.createErr
	}
	return "opaque-token", domain.Session{}, nil
}

func (s *loginSessionsStub) Destroy(context.Context, string) error {
	s.destroyed = true
	s.destroyCalls++
	return nil
}

func TestLoginCommandFailsClosedForRedisErrorsAndIPLimit(t *testing.T) {
	store := &loginStoreStub{}
	sessions := &loginSessionsStub{}
	dependencyErr := errors.New("Redis unavailable")
	input := identityapp.LoginInput{
		OrgID: uuid.New(), LoginName: "alice", Password: "initialPassword123",
		ClientIP: "127.0.0.1", RequestID: "login-limit-test",
	}
	command := identityapp.NewLoginCommand(store, sessions, loginLimiterStub{err: dependencyErr}, time.Now)
	if _, err := command.Execute(t.Context(), input); !errors.Is(err, identityapp.ErrLoginUnavailable) ||
		!errors.Is(err, dependencyErr) || store.findCalls != 0 || sessions.created {
		t.Fatalf("Redis failure = %v, account reads %d, session created %t", err, store.findCalls, sessions.created)
	}
	command = identityapp.NewLoginCommand(store, sessions, loginLimiterStub{allowed: false}, time.Now)
	limited, err := command.Execute(t.Context(), input)
	if !errors.Is(err, identityapp.ErrLoginRateLimited) || limited.RetryAfter != time.Minute ||
		store.findCalls != 0 || len(store.standalone) != 1 {
		t.Fatalf("IP limit = %v, retry %v, account reads %d, audits %d", err, limited.RetryAfter, store.findCalls, len(store.standalone))
	}
	assertLoginAudit(t, store.standalone[0], "auth.login_failed", "ip_rate_limited")
	store.recordErr = dependencyErr
	if _, err := command.Execute(t.Context(), input); !errors.Is(err, identityapp.ErrLoginUnavailable) ||
		!errors.Is(err, dependencyErr) {
		t.Fatalf("audit failure after IP limit = %v", err)
	}
}

func TestLoginCommandRevokesProvisionalSessionsOnRevisionConflict(t *testing.T) {
	hash, err := domain.HashPassword("initialPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	orgID := uuid.New()
	store := &loginStoreStub{
		user: domain.User{
			ID: uuid.New(), OrgID: orgID, LoginName: "alice", Status: domain.StatusActive,
			PasswordHash: hash, SessionEpoch: 1, Revision: 1,
		},
		saveErr: domain.ErrRevisionConflict,
	}
	sessions := &loginSessionsStub{}
	command := identityapp.NewLoginCommand(store, sessions, loginLimiterStub{allowed: true}, time.Now)
	result, err := command.Execute(t.Context(), identityapp.LoginInput{
		OrgID: orgID, LoginName: "alice", Password: "initialPassword123",
		ClientIP: "127.0.0.1", RequestID: "login-conflict-test",
	})
	if !errors.Is(err, identityapp.ErrLoginUnavailable) || result.Token != "" ||
		store.findCalls != 3 || sessions.createCalls != 3 || sessions.destroyCalls != 3 {
		t.Fatalf("revision conflicts = %v, token present %t, reads %d, sessions %d/%d", err,
			result.Token != "", store.findCalls, sessions.createCalls, sessions.destroyCalls)
	}
}

type loginLimiterStub struct {
	allowed bool
	err     error
}

func (l loginLimiterStub) AllowAttempt(context.Context, string) (bool, time.Duration, error) {
	return l.allowed, time.Minute, l.err
}

func TestLoginCommandCreatesSessionOnlyForAuditedSuccess(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	orgID, userID := uuid.New(), uuid.New()
	hash, err := domain.HashPassword("initialPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	store := &loginStoreStub{user: domain.User{
		ID: userID, OrgID: orgID, LoginName: "alice", DisplayName: "Alice",
		Role: domain.RoleProducer, Status: domain.StatusActive, PasswordHash: hash,
		MustChangePassword: true, SessionEpoch: 1, Revision: 1,
	}}
	sessions := &loginSessionsStub{}
	command := identityapp.NewLoginCommand(store, sessions, loginLimiterStub{allowed: true}, func() time.Time { return now })
	result, err := command.Execute(t.Context(), identityapp.LoginInput{
		OrgID: orgID, LoginName: "alice", Password: "initialPassword123",
		ClientIP: "127.0.0.1", RequestID: "login-success-test",
	})
	if err != nil || !sessions.created || sessions.destroyed || result.Token != "opaque-token" ||
		result.User.ID != userID || !result.User.MustChangePassword ||
		store.saved.FailedLoginCount != 0 || store.saved.Revision != 2 || len(store.events) != 1 {
		t.Fatalf("successful login = %+v, session %+v, saved %+v, events %d, error %v", result, sessions, store.saved, len(store.events), err)
	}
	assertLoginAudit(t, store.events[0], "auth.login_succeeded", "")
	store.saveErr = errors.New("database commit failed")
	sessions = &loginSessionsStub{}
	command = identityapp.NewLoginCommand(store, sessions, loginLimiterStub{allowed: true}, func() time.Time { return now })
	if _, err := command.Execute(t.Context(), identityapp.LoginInput{
		OrgID: orgID, LoginName: "alice", Password: "initialPassword123",
		ClientIP: "127.0.0.1", RequestID: "login-rollback-test",
	}); !errors.Is(err, identityapp.ErrLoginUnavailable) || !sessions.destroyed {
		t.Fatalf("failed transaction = %v, session destroyed %t", err, sessions.destroyed)
	}
}

func TestLoginCommandLocksOnFifthFailureAndHidesUnknownUser(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	orgID, userID := uuid.New(), uuid.New()
	hash, err := domain.HashPassword("initialPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	store := &loginStoreStub{user: domain.User{
		ID: userID, OrgID: orgID, LoginName: "alice", Role: domain.RoleProducer,
		Status: domain.StatusActive, PasswordHash: hash, SessionEpoch: 1,
		FailedLoginCount: 4, Revision: 5,
	}}
	sessions := &loginSessionsStub{}
	command := identityapp.NewLoginCommand(store, sessions, loginLimiterStub{allowed: true}, func() time.Time { return now })
	input := identityapp.LoginInput{
		OrgID: orgID, LoginName: "alice", Password: "wrongPassword123",
		ClientIP: "127.0.0.1", RequestID: "login-failure-test",
	}
	if _, err := command.Execute(t.Context(), input); !errors.Is(err, identityapp.ErrInvalidCredentials) ||
		sessions.created || store.saved.FailedLoginCount != 5 ||
		!store.saved.LockedUntil.Equal(now.Add(15*time.Minute)) || len(store.events) != 2 {
		t.Fatalf("fifth failure = %v, session %+v, saved %+v, events %d", err, sessions, store.saved, len(store.events))
	}
	assertLoginAudit(t, store.events[0], "auth.login_failed", "password_mismatch")
	assertLoginAudit(t, store.events[1], "auth.locked", "")
	store.findErr = domain.ErrUserNotFound
	store.events = nil
	if _, err := command.Execute(t.Context(), input); !errors.Is(err, identityapp.ErrInvalidCredentials) ||
		len(store.standalone) != 1 || sessions.created {
		t.Fatalf("unknown login = %v, standalone audits %d", err, len(store.standalone))
	}
	assertLoginAudit(t, store.standalone[0], "auth.login_failed", "login_not_found")
}

func assertLoginAudit(t *testing.T, event identityapp.OutboxEvent, wantAction, wantReason string) {
	t.Helper()
	if event.ID == uuid.Nil || event.Topic != "lanverse.audit.recorded.v1" {
		t.Fatalf("audit routing = %+v", event)
	}
	var envelope struct {
		Data struct {
			Action string         `json:"action"`
			After  map[string]any `json:"after"`
		} `json:"data"`
	}
	if err := json.Unmarshal(event.Payload, &envelope); err != nil || envelope.Data.Action != wantAction {
		t.Fatalf("audit action = %q, error %v", envelope.Data.Action, err)
	}
	if wantReason != "" && envelope.Data.After["reason"] != wantReason {
		t.Fatalf("audit reason = %v, want %q", envelope.Data.After["reason"], wantReason)
	}
}
