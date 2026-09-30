package identity_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestLocalWorkspaceIsStableAndDoesNotRestoreDisabledIdentityWithPostgres(t *testing.T) {
	dsn := os.Getenv("LV_TEST_LOCAL_WORKSPACE_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_LOCAL_WORKSPACE_DB_DSN to an empty disposable lanverse_local_workspace_test_* database with identity and organization migrations")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal("open local workspace test database")
	}
	t.Cleanup(func() { _ = conn.Close() })
	var name string
	if err := conn.DB.WithContext(ctx).Raw("SELECT current_database()").Scan(&name).Error; err != nil ||
		!strings.HasPrefix(name, "lanverse_local_workspace_test_") {
		t.Fatal("local workspace test requires its dedicated disposable database")
	}
	var count int
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM identity."user"`).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("test identity table must be empty")
	}
	store := pgidentity.NewStore(conn.DB)
	first, err := store.EnsureWorkspace(ctx)
	if err != nil || first.ID == uuid.Nil || first.OrgID == uuid.Nil || first.MustChangePassword ||
		first.Role != domain.RoleProducer || !domain.ValidPasswordHash(first.PasswordHash) {
		t.Fatalf("local workspace creation error %v", err)
	}
	const concurrent = 4
	users := make([]domain.User, concurrent)
	errorsFound := make([]error, concurrent)
	var wait sync.WaitGroup
	for i := range concurrent {
		wait.Go(func() { users[i], errorsFound[i] = store.EnsureWorkspace(ctx) })
	}
	wait.Wait()
	for i, user := range users {
		if errorsFound[i] != nil || user.ID != first.ID || user.OrgID != first.OrgID ||
			user.Revision != first.Revision || user.PasswordHash != first.PasswordHash {
			t.Fatal("concurrent access changed the workspace identity")
		}
	}
	for _, table := range []string{`identity."user"`, `workspace.organization`} {
		if err := conn.DB.WithContext(ctx).Raw("SELECT count(*) FROM " + table).Scan(&count).Error; err != nil || count != 1 {
			t.Fatalf("expected one row in %s", table)
		}
	}
	if err := conn.DB.WithContext(ctx).Exec(`UPDATE identity."user" SET status = 'disabled' WHERE id = ?`, first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureWorkspace(ctx); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("disabled workspace error %v, want forbidden", err)
	}
	var status string
	if err := conn.DB.WithContext(ctx).Raw(`SELECT status FROM identity."user" WHERE id = ?`, first.ID).Scan(&status).Error; err != nil || status != "disabled" {
		t.Fatal("disabled account was changed")
	}
}
