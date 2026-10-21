package app_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestPreparedAgreementCannotEnableRegistration(t *testing.T) {
	auth, err := identityruntime.NewAuth(&gorm.DB{}, config.Config{IdentityKey: publicTestKey(t), IdentityTermsVersion: "2026-10-15.1", IdentityMailSourceHourlyLimit: 10, IdentityMailGlobalHourlyLimit: 100}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.StartRegistration(t.Context(), "alice@example.com", ""); !errors.Is(err, identity.ErrRegistrationUnavailable) {
		t.Fatalf("prepared package registration: %v", err)
	}
	if _, err := auth.CompleteRegistration(t.Context(), identity.CompleteRegistrationInput{TermsVersion: "2026-10-15.1", TermsAccepted: true}); !errors.Is(err, identity.ErrRegistrationUnavailable) {
		t.Fatalf("prepared package completion: %v", err)
	}
}

type mailLoopFixture struct{ entered, ended chan struct{} }

func (f mailLoopFixture) RunOnce(ctx context.Context, _ int) (int, error) {
	close(f.entered)
	<-ctx.Done()
	close(f.ended)
	return 0, ctx.Err()
}
func TestMailLoopCleanupCancelsAndJoinsActiveBatch(t *testing.T) {
	f := mailLoopFixture{make(chan struct{}), make(chan struct{})}
	stop := identityruntime.StartMailRunner(t.Context(), f, zap.NewNop(), time.Millisecond)
	select {
	case <-f.entered:
	case <-time.After(time.Second):
		t.Fatal("batch not started")
	}
	stop()
	select {
	case <-f.ended:
	default:
		t.Fatal("cleanup returned before batch exited")
	}
}

func TestExportLoopCleanupCancelsAndJoinsActiveBatch(t *testing.T) {
	f := mailLoopFixture{make(chan struct{}), make(chan struct{})}
	stop := identityruntime.StartExportRunner(t.Context(), f, zap.NewNop(), time.Millisecond)
	select {
	case <-f.entered:
	case <-time.After(time.Second):
		t.Fatal("export batch not started")
	}
	stop()
	select {
	case <-f.ended:
	default:
		t.Fatal("cleanup did not join export batch")
	}
}
func TestExportLoopDisabledStorageDoesNotRequireSMTPOrKeys(t *testing.T) {
	stop, err := identityruntime.StartDirectoryExportLoop(t.Context(), nil, config.Config{}, nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	stop()
}

func TestSessionV2VerifierOnly(t *testing.T) {
	codec := &wiringTokens{signingDisabled: true}
	coordinator := &wiringCoordinator{}
	auth := identity.NewEmailAuth(wiringStore{}, proofFixture{}, identity.EmailPolicy{}, time.Now)
	if err := identityruntime.ConfigureSessionSecurity(auth, config.Config{}, identityruntime.SessionDependencies{Tokens: codec, Coordinator: coordinator}); err != nil {
		t.Fatal(err)
	}
	_, err := auth.CurrentAccess(t.Context(), "access", "csrf", identity.NoActivity)
	if !errors.Is(err, identity.ErrSessionInvalid) || !codec.verified || !coordinator.checked {
		t.Fatalf("verifier-only access not consumed: %v", err)
	}
	if err := auth.LogoutCurrentWithRefresh(t.Context(), "invalid", "csrf", uuid.New()); err != nil {
		t.Fatalf("verifier-only logout: %v", err)
	}
	storeErr := errors.New("observable transaction boundary")
	logoutAuth := identity.NewEmailAuth(sessionWiringErrorStore{err: storeErr}, proofFixture{}, identity.EmailPolicy{}, time.Now)
	if err := identityruntime.ConfigureSessionSecurity(logoutAuth, config.Config{}, identityruntime.SessionDependencies{Tokens: codec, Coordinator: coordinator}); err != nil {
		t.Fatal(err)
	}
	if err := logoutAuth.LogoutCurrentWithRefresh(t.Context(), base64.RawURLEncoding.EncodeToString(make([]byte, 32)), "csrf", uuid.New()); !errors.Is(err, storeErr) {
		t.Fatalf("verifier-only logout did not reach authority store: %v", err)
	}
	if _, err := auth.Refresh(t.Context(), "refresh", "csrf", "peer", uuid.New()); !errors.Is(err, identity.ErrRefreshDependency) {
		t.Fatalf("verifier-only refresh: %v", err)
	}
}

type sessionWiringErrorStore struct {
	identity.EmailStore
	err error
}

func (f sessionWiringErrorStore) EmailTransaction(context.Context, func(identity.EmailTx) error) error {
	return f.err
}
