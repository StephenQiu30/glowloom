package database_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	. "github.com/stephenqiu30/lanverse/backend/src/platform/database"
)

func TestWorkspaceIDContextRoundTrip(t *testing.T) {
	workspaceID := uuid.New()
	ctx := WithWorkspaceID(context.Background(), workspaceID)

	got, ok := WorkspaceID(ctx)
	if !ok {
		t.Fatal("WorkspaceID() did not find workspace context")
	}
	if got != workspaceID {
		t.Fatalf("workspace ID = %s, want %s", got, workspaceID)
	}
}

func TestWorkspaceIDContextRejectsNil(t *testing.T) {
	ctx := WithWorkspaceID(context.Background(), uuid.Nil)

	if _, ok := WorkspaceID(ctx); ok {
		t.Fatal("WorkspaceID() accepted uuid.Nil")
	}
}

func TestWithWorkspaceTransactionSetsAndClearsDatabaseSetting(t *testing.T) {
	if os.Getenv("LANVERSE_INTEGRATION") != "1" {
		t.Skip("set LANVERSE_INTEGRATION=1 to run PostgreSQL/GORM integration")
	}
	ctx := WithWorkspaceID(context.Background(), uuid.New())
	pool, err := Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	orm, err := OpenGORM(pool)
	if err != nil {
		t.Fatal(err)
	}

	var inside string
	if err := WithWorkspaceTransaction(ctx, orm, func(tx *gorm.DB) error {
		return tx.Raw("SELECT current_setting('app.workspace_id', true)").Scan(&inside).Error
	}); err != nil {
		t.Fatal(err)
	}
	workspaceID, _ := WorkspaceID(ctx)
	if inside != workspaceID.String() {
		t.Fatalf("transaction workspace setting = %q, want %q", inside, workspaceID)
	}

	var outside string
	if err := orm.Raw("SELECT current_setting('app.workspace_id', true)").Scan(&outside).Error; err != nil {
		t.Fatal(err)
	}
	if outside != "" {
		t.Fatalf("workspace setting leaked outside transaction: %q", outside)
	}
}
