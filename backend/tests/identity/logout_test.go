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

type logoutSessionsStub struct {
	session      domain.Session
	loadErr      error
	destroyErr   error
	afterDestroy func()
	touched      bool
	destroyed    bool
}

func (s *logoutSessionsStub) Load(context.Context, string) (domain.Session, error) {
	return s.session, s.loadErr
}

func (s *logoutSessionsStub) Touch(context.Context, string, domain.Session) error {
	s.touched = true
	return nil
}

func (s *logoutSessionsStub) Destroy(context.Context, string) error {
	s.destroyed = true
	if s.afterDestroy != nil {
		s.afterDestroy()
	}
	return s.destroyErr
}

type logoutAuditStub struct {
	event  identityapp.OutboxEvent
	err    error
	ctxErr error
}

func (s *logoutAuditStub) RecordLogoutAudit(ctx context.Context, event identityapp.OutboxEvent) error {
	s.event = event
	s.ctxErr = ctx.Err()
	return s.err
}

func TestLogoutCommandRevokesCurrentSessionAndAuditsUser(t *testing.T) {
	orgID, userID := uuid.New(), uuid.New()
	account := domain.User{ID: userID, OrgID: orgID, Status: domain.StatusActive, SessionEpoch: 2}
	sessions := &logoutSessionsStub{session: domain.Session{OrgID: orgID, UserID: userID, SessionEpoch: 2}}
	accounts := accountReaderFunc(func(context.Context, uuid.UUID, uuid.UUID) (domain.User, error) {
		return account, nil
	})
	audits := &logoutAuditStub{}
	command := identityapp.NewLogoutCommand(identityapp.NewAuthenticator(accounts, sessions), sessions, audits, time.Now)
	result, err := command.Execute(t.Context(), identityapp.LogoutInput{
		Token: "opaque-token", RequestID: "logout-request", ClientIP: "::ffff:127.0.0.1",
	})
	if err != nil || !result.Revoked || !sessions.touched || !sessions.destroyed {
		t.Fatalf("logout result = %+v, touched %t, destroyed %t, error %v", result, sessions.touched, sessions.destroyed, err)
	}
	if audits.event.Topic != "lanverse.audit.recorded.v1" || strings.Contains(string(audits.event.Payload), "opaque-token") {
		t.Fatal("logout audit missing or exposed token")
	}
	record, err := auditapp.NewIdentityActionParser().Parse(inbox.Record{
		Topic: audits.event.Topic, Key: []byte(audits.event.PartitionKey), Value: audits.event.Payload,
	})
	if err != nil || record.Action != "auth.logout" || record.ActorID == nil || *record.ActorID != userID ||
		record.ObjectType != "user" || record.ObjectID != userID.String() || record.IP != "127.0.0.1" {
		t.Fatalf("logout audit = %+v, error %v", record, err)
	}
}

func TestLogoutCommandDoesNotAuditFailedRevocation(t *testing.T) {
	orgID, userID := uuid.New(), uuid.New()
	accounts := accountReaderFunc(func(context.Context, uuid.UUID, uuid.UUID) (domain.User, error) {
		return domain.User{ID: userID, OrgID: orgID, Status: domain.StatusActive, SessionEpoch: 1}, nil
	})
	sessions := &logoutSessionsStub{
		session:    domain.Session{OrgID: orgID, UserID: userID, SessionEpoch: 1},
		destroyErr: errors.New("Redis unavailable"),
	}
	audits := &logoutAuditStub{}
	command := identityapp.NewLogoutCommand(identityapp.NewAuthenticator(accounts, sessions), sessions, audits, time.Now)
	result, err := command.Execute(t.Context(), identityapp.LogoutInput{
		Token: "opaque-token", RequestID: "logout-request", ClientIP: "127.0.0.1",
	})
	if !errors.Is(err, identityapp.ErrLogoutUnavailable) || result.Revoked || audits.event.ID != uuid.Nil {
		t.Fatalf("failed revocation = %+v, error %v, audit ID %s", result, err, audits.event.ID)
	}
}

func TestLogoutCommandReportsAuditFailureAfterRevocation(t *testing.T) {
	orgID, userID := uuid.New(), uuid.New()
	accounts := accountReaderFunc(func(context.Context, uuid.UUID, uuid.UUID) (domain.User, error) {
		return domain.User{ID: userID, OrgID: orgID, Status: domain.StatusActive, SessionEpoch: 1}, nil
	})
	sessions := &logoutSessionsStub{session: domain.Session{OrgID: orgID, UserID: userID, SessionEpoch: 1}}
	audits := &logoutAuditStub{err: errors.New("PostgreSQL unavailable")}
	command := identityapp.NewLogoutCommand(identityapp.NewAuthenticator(accounts, sessions), sessions, audits, time.Now)
	result, err := command.Execute(t.Context(), identityapp.LogoutInput{
		Token: "opaque-token", RequestID: "logout-request", ClientIP: "127.0.0.1",
	})
	if !errors.Is(err, identityapp.ErrLogoutUnavailable) || !result.Revoked || !sessions.destroyed {
		t.Fatalf("audit failure = %+v, destroyed %t, error %v", result, sessions.destroyed, err)
	}
}

func TestLogoutCommandRejectsMissingSessionWithoutAudit(t *testing.T) {
	sessions := &logoutSessionsStub{loadErr: domain.ErrSessionNotFound}
	accounts := accountReaderFunc(func(context.Context, uuid.UUID, uuid.UUID) (domain.User, error) {
		t.Fatal("account was queried after missing session")
		return domain.User{}, nil
	})
	audits := &logoutAuditStub{}
	command := identityapp.NewLogoutCommand(identityapp.NewAuthenticator(accounts, sessions), sessions, audits, time.Now)
	result, err := command.Execute(t.Context(), identityapp.LogoutInput{
		Token: "opaque-token", RequestID: "logout-request", ClientIP: "127.0.0.1",
	})
	if !errors.Is(err, identityapp.ErrUnauthenticated) || result.Revoked || sessions.destroyed || audits.event.ID != uuid.Nil {
		t.Fatalf("missing session = %+v, destroyed %t, audit ID %s, error %v", result, sessions.destroyed, audits.event.ID, err)
	}
}

func TestLogoutCommandAuditsAfterRequestCancellationDuringRevocation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	orgID, userID := uuid.New(), uuid.New()
	accounts := accountReaderFunc(func(context.Context, uuid.UUID, uuid.UUID) (domain.User, error) {
		return domain.User{ID: userID, OrgID: orgID, Status: domain.StatusActive, SessionEpoch: 1}, nil
	})
	sessions := &logoutSessionsStub{
		session:      domain.Session{OrgID: orgID, UserID: userID, SessionEpoch: 1},
		afterDestroy: cancel,
	}
	audits := &logoutAuditStub{}
	command := identityapp.NewLogoutCommand(identityapp.NewAuthenticator(accounts, sessions), sessions, audits, time.Now)
	result, err := command.Execute(ctx, identityapp.LogoutInput{
		Token: "opaque-token", RequestID: "logout-cancelled", ClientIP: "127.0.0.1",
	})
	if err != nil || !result.Revoked || audits.event.ID == uuid.Nil || audits.ctxErr != nil {
		t.Fatalf("logout after cancellation = %+v, audit ID %s, audit context %v, error %v", result, audits.event.ID, audits.ctxErr, err)
	}
}
