package storyboard_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	command "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	database "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	adapter "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	"github.com/google/uuid"
)

func TestIntentReceiptQueryReturnsOnlyExactOwnerResult(t *testing.T) {
	dsn := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to verify accepted text receipts")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err = schema.Sync(ctx, db); err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	userID, workspaceID := uuid.New(), uuid.New()
	now := time.Now().UTC()
	for _, record := range []any{
		&model.UserAccount{ID: userID, EmailNormalized: userID.String() + "@example.invalid", PasswordHash: "synthetic", TokenVersion: 1, DisplayName: "MVP receipt test", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.Workspace{ID: workspaceID, Name: "MVP receipt test", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
	} {
		if err = tx.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	store := adapter.New(tx)
	setID := uuid.NewString()
	err = store.WithinTransaction(ctx, func(repo app.Repository) error {
		if _, queryErr := repo.GetIntentReceipt(ctx, setID); !errors.Is(queryErr, app.ErrNotFound) {
			t.Fatalf("missing receipt: %v", queryErr)
		}
		base := command.Receipt{ID: uuid.NewString(), WorkspaceID: workspaceID.String(), Operation: "storyboard.freeze_intent_set", IdempotencyKey: uuid.NewString(), InputHash: strings.Repeat("1", 64), ResourceID: setID, Result: []byte(`{"immutable":"text"}`), CreatedBy: userID.String(), CreatedAt: now}
		if createErr := repo.CreateReceipt(ctx, base); createErr != nil {
			return createErr
		}
		unrelated := base
		unrelated.ID = uuid.NewString()
		unrelated.Operation = "storyboard.other"
		if createErr := repo.CreateReceipt(ctx, unrelated); createErr != nil {
			return createErr
		}
		read, queryErr := repo.GetIntentReceipt(ctx, setID)
		if queryErr != nil {
			return queryErr
		}
		if read.ID != base.ID || read.ResourceID != base.ResourceID || read.Operation != base.Operation {
			t.Fatalf("wrong owner receipt: %+v", read)
		}
		if _, queryErr = repo.GetIntentReceipt(ctx, uuid.NewString()); !errors.Is(queryErr, app.ErrNotFound) {
			t.Fatalf("cross-resource receipt: %v", queryErr)
		}
		duplicate := base
		duplicate.ID = uuid.NewString()
		duplicate.IdempotencyKey = uuid.NewString()
		if createErr := repo.CreateReceipt(ctx, duplicate); createErr != nil {
			return createErr
		}
		if _, queryErr = repo.GetIntentReceipt(ctx, setID); queryErr == nil {
			t.Fatal("ambiguous receipts returned as accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
