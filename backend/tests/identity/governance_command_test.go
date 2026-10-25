package identity_test

import (
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func TestGovernanceCommandReceiptAndReplay(t *testing.T) {
	if os.Getenv("GLOWLOOM_TEST_EXISTING_DB_DSN") == "" && os.Getenv("GLOWLOOM_TEST_CANVAS_DB_DSN") != "" {
		t.Setenv("GLOWLOOM_TEST_EXISTING_DB_DSN", os.Getenv("GLOWLOOM_TEST_CANVAS_DB_DSN"))
	}
	tx, svc, actor, now := governanceFixture(t)
	_, target := verifiedTestActor(t, tx, "batch-target@example.invalid", now)
	if err := tx.Exec(`UPDATE identity."user" SET revision=1 WHERE id=?`, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	for range 2 {
		if err := svc.ExecuteUserCommand(t.Context(), actor, target.ID, 1, "Batch moderation reason", "disable", key); err != nil {
			t.Fatal(err)
		}
	}
	receipt, err := svc.UserCommandReceipt(t.Context(), actor, target.ID, key)
	if err != nil || receipt.Action != "disable" || receipt.RequestID != key {
		t.Fatal("missing bound receipt", receipt, err)
	}
	if err := svc.ExecuteUserCommand(t.Context(), actor, target.ID, 1, "Different moderation reason", "disable", key); !errors.Is(err, identity.ErrGovernanceConflict) {
		t.Fatal("key reused with another payload", err)
	}
	if _, err := svc.UserCommandReceipt(t.Context(), actor, uuid.New(), key); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatal("receipt leaked to another target", err)
	}
	var revision int64
	if err := tx.Raw(`SELECT revision FROM identity."user" WHERE id=?`, target.ID).Scan(&revision).Error; err != nil || revision != 2 {
		t.Fatal("replay mutated account again", revision, err)
	}
	if err := svc.ExecuteUserCommand(t.Context(), actor, actor.ID, 1, "Batch moderation reason", "disable", uuid.New()); !errors.Is(err, identity.ErrInvalidGovernance) {
		t.Fatal("batch can revoke its own actor", err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='creator' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UserCommandReceipt(t.Context(), actor, target.ID, key); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("receipt authorization not refreshed", err)
	}
}
