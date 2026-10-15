package app_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestPreparedAgreementCannotEnableRegistration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.key")
	if err := credentialproof.CreateFile(path); err != nil {
		t.Fatal(err)
	}
	auth, err := identityruntime.NewAuth(&gorm.DB{}, config.Config{IdentityKeyFile: path, IdentityTermsVersion: "2026-10-15.1", IdentityMailSourceHourlyLimit: 10, IdentityMailGlobalHourlyLimit: 100}, time.Now)
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
