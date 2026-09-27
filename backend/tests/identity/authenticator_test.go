package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

type accountReaderFunc func(context.Context, uuid.UUID, uuid.UUID) (domain.User, error)

func (f accountReaderFunc) FindByID(ctx context.Context, orgID, userID uuid.UUID) (domain.User, error) {
	return f(ctx, orgID, userID)
}

type sessionReaderStub struct {
	session  domain.Session
	loadErr  error
	touchErr error
	touched  bool
}

func (s *sessionReaderStub) Load(context.Context, string) (domain.Session, error) {
	return s.session, s.loadErr
}

func (s *sessionReaderStub) Touch(context.Context, string, domain.Session) error {
	s.touched = true
	return s.touchErr
}

func TestAuthenticateChecksDurableAccountBeforeRenewal(t *testing.T) {
	orgID, userID := uuid.New(), uuid.New()
	session := domain.Session{
		OrgID: orgID, UserID: userID, SessionEpoch: 3,
		CreateTime: time.Now().UTC(), LastSeen: time.Now().UTC(),
	}
	sessions := &sessionReaderStub{session: session}
	users := accountReaderFunc(func(_ context.Context, gotOrg, gotUser uuid.UUID) (domain.User, error) {
		if gotOrg != orgID || gotUser != userID {
			t.Fatalf("account lookup scope = %s/%s, want %s/%s", gotOrg, gotUser, orgID, userID)
		}
		return domain.User{
			ID: userID, OrgID: orgID, LoginName: "alice", DisplayName: "Alice",
			Role: domain.RoleProducer, Status: domain.StatusActive, SessionEpoch: 3,
			MustChangePassword: true, PasswordHash: "canary-password-hash",
		}, nil
	})
	authenticator := identityapp.NewAuthenticator(users, sessions)
	principal, err := authenticator.Authenticate(t.Context(), "opaque-token")
	if err != nil || !sessions.touched {
		t.Fatalf("authenticated principal = %+v, touch %t, error %v", principal, sessions.touched, err)
	}
	if principal.ID != userID || principal.OrgID != orgID || principal.LoginName != "alice" ||
		principal.Role != domain.RoleProducer || !principal.MustChangePassword {
		t.Fatalf("principal = %+v", principal)
	}
	encoded, err := json.Marshal(principal)
	if err != nil || strings.Contains(string(encoded), "canary-password-hash") {
		t.Fatalf("principal exposed password hash, error %v", err)
	}
}

func TestAuthenticateRejectsStaleDisabledOrMissingIdentity(t *testing.T) {
	orgID, userID := uuid.New(), uuid.New()
	baseSession := domain.Session{OrgID: orgID, UserID: userID, SessionEpoch: 3}
	baseUser := domain.User{ID: userID, OrgID: orgID, Status: domain.StatusActive, SessionEpoch: 3}
	for _, tc := range []struct {
		name       string
		sessionErr error
		user       domain.User
		userErr    error
	}{
		{name: "missing session", sessionErr: domain.ErrSessionNotFound, user: baseUser},
		{name: "stale epoch", user: func() domain.User { u := baseUser; u.SessionEpoch = 4; return u }()},
		{name: "disabled user", user: func() domain.User { u := baseUser; u.Status = domain.StatusDisabled; return u }()},
		{name: "wrong organization", user: func() domain.User { u := baseUser; u.OrgID = uuid.New(); return u }()},
		{name: "wrong account", user: func() domain.User { u := baseUser; u.ID = uuid.New(); return u }()},
		{name: "missing user", userErr: domain.ErrUserNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sessions := &sessionReaderStub{session: baseSession, loadErr: tc.sessionErr}
			users := accountReaderFunc(func(context.Context, uuid.UUID, uuid.UUID) (domain.User, error) {
				return tc.user, tc.userErr
			})
			authenticator := identityapp.NewAuthenticator(users, sessions)
			if _, err := authenticator.Authenticate(t.Context(), "opaque-token"); !errors.Is(err, identityapp.ErrUnauthenticated) {
				t.Fatalf("Authenticate() = %v, want ErrUnauthenticated", err)
			}
			if sessions.touched {
				t.Fatal("rejected session was renewed")
			}
		})
	}
}

func TestAuthenticateFailsClosedOnDependencyErrors(t *testing.T) {
	orgID, userID := uuid.New(), uuid.New()
	serviceErr := errors.New("temporary dependency failure")
	session := domain.Session{OrgID: orgID, UserID: userID, SessionEpoch: 1}
	user := domain.User{ID: userID, OrgID: orgID, Status: domain.StatusActive, SessionEpoch: 1}
	for _, tc := range []struct {
		name       string
		loadErr    error
		userErr    error
		touchErr   error
		wantUnauth bool
	}{
		{name: "Redis read", loadErr: serviceErr},
		{name: "PostgreSQL read", userErr: serviceErr},
		{name: "Redis renewal", touchErr: serviceErr},
		{name: "session deleted during renewal", touchErr: domain.ErrSessionNotFound, wantUnauth: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sessions := &sessionReaderStub{session: session, loadErr: tc.loadErr, touchErr: tc.touchErr}
			users := accountReaderFunc(func(context.Context, uuid.UUID, uuid.UUID) (domain.User, error) {
				return user, tc.userErr
			})
			authenticator := identityapp.NewAuthenticator(users, sessions)
			_, err := authenticator.Authenticate(t.Context(), "opaque-token")
			if tc.wantUnauth {
				if !errors.Is(err, identityapp.ErrUnauthenticated) {
					t.Fatalf("Authenticate() = %v, want ErrUnauthenticated", err)
				}
				return
			}
			if !errors.Is(err, identityapp.ErrAuthenticationUnavailable) || !errors.Is(err, serviceErr) {
				t.Fatalf("Authenticate() = %v, want unavailable with cause", err)
			}
		})
	}
}
