package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestAdminDirectoryWithRealPostgres(t *testing.T) {
	dsn := os.Getenv("LV_TEST_ACCOUNT_DIRECTORY_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_ACCOUNT_DIRECTORY_DB_DSN to a disposable PostgreSQL database with identity, Outbox, and directory index migrations")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := pgidentity.NewStore(conn.DB)
	orgID, otherOrg := uuid.New(), uuid.New()
	adminA, adminB, producer, disabled, deleted, outsider := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	hash, err := domain.HashPassword("initialPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range []struct {
		id   uuid.UUID
		org  uuid.UUID
		name string
		role domain.Role
	}{
		{adminA, orgID, "admin-a", domain.RoleAdmin},
		{adminB, orgID, "admin-b", domain.RoleAdmin},
		{producer, orgID, "producer", domain.RoleProducer},
		{disabled, orgID, "disabled", domain.RoleProducer},
		{deleted, orgID, "deleted", domain.RoleProducer},
		{outsider, otherOrg, "outsider", domain.RoleAdmin},
	} {
		if err := store.Create(ctx, domain.User{ID: account.id, OrgID: account.org,
			LoginName: account.name, DisplayName: account.name, Role: account.role, PasswordHash: hash}); err != nil {
			t.Fatalf("create account %s: %v", account.name, err)
		}
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET must_change_password = false WHERE id IN (?::uuid, ?::uuid)
	`, adminA.String(), adminB.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET is_delete = true WHERE id = ?::uuid
	`, deleted.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.Disable(ctx, orgID, disabled, 1); err != nil {
		t.Fatalf("prepare disabled directory account: %v", err)
	}
	for _, position := range []struct {
		id      uuid.UUID
		created time.Time
	}{
		{producer, time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)},
		{adminA, time.Date(2026, 9, 27, 11, 0, 0, 0, time.UTC)},
		{adminB, time.Date(2026, 9, 27, 11, 0, 0, 0, time.UTC)},
		{disabled, time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)},
	} {
		if err := conn.DB.WithContext(ctx).Exec(`
			UPDATE identity."user" SET create_time = ? WHERE id = ?::uuid
		`, position.created, position.id.String()).Error; err != nil {
			t.Fatal(err)
		}
	}
	actorA := identityapp.Principal{ID: adminA, OrgID: orgID, Role: domain.RoleAdmin}
	query := identityapp.NewListUsersQuery(store)
	first, err := query.Execute(ctx, actorA, identityapp.ListUsersInput{Limit: 2})
	if err != nil || len(first.Users) != 2 || first.Next == nil {
		t.Fatalf("first directory page: %+v, %v", first, err)
	}
	second, err := query.Execute(ctx, actorA, identityapp.ListUsersInput{Limit: 2, After: first.Next})
	if err != nil || len(second.Users) != 2 || second.Next != nil {
		t.Fatalf("second directory page: %+v, %v", second, err)
	}
	ordered := append([]identityapp.UserListItem(nil), first.Users...)
	ordered = append(ordered, second.Users...)
	if ordered[0].ID != producer || ordered[3].ID != disabled || ordered[3].Status != domain.StatusDisabled {
		t.Fatalf("directory creation order or disabled account missing: %+v", ordered)
	}
	if ordered[1].CreateTime.Equal(ordered[2].CreateTime) && ordered[1].ID.String() < ordered[2].ID.String() {
		t.Fatalf("directory UUID tie break is ascending: %+v", ordered)
	}
	seen := map[uuid.UUID]bool{}
	for _, item := range ordered {
		seen[item.ID] = true
		body, err := json.Marshal(item)
		if err != nil || containsSecret(body, hash, "PasswordHash") {
			t.Fatalf("unsafe directory item: %v", err)
		}
	}
	if len(seen) != 4 || !seen[adminA] || !seen[adminB] || !seen[producer] || !seen[disabled] || seen[deleted] || seen[outsider] {
		t.Fatalf("directory scope: %+v", seen)
	}
	otherActor := identityapp.Principal{ID: outsider, OrgID: otherOrg, Role: domain.RoleAdmin}
	if _, err := query.Execute(ctx, otherActor, identityapp.ListUsersInput{}); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("first-login outsider directory: %v", err)
	}
	command := identityapp.NewUpdateUserCommand(store, time.Now)
	newName := "New producer"
	updated, err := command.Execute(ctx, actorA, identityapp.UpdateUserInput{
		TargetID: producer, ExpectedRevision: 1, DisplayName: &newName, RequestID: "rename-producer",
	})
	if err != nil || updated.DisplayName != newName || updated.Revision != 2 {
		t.Fatalf("rename producer: %+v, %v", updated, err)
	}
	current, err := store.FindByID(ctx, orgID, producer)
	if err != nil || current.DisplayName != newName || current.Revision != 2 || current.SessionEpoch != 1 || current.PasswordHash != hash {
		t.Fatalf("committed rename state: name %q, revision %d, epoch %d, error %v",
			current.DisplayName, current.Revision, current.SessionEpoch, err)
	}
	if _, err := command.Execute(ctx, actorA, identityapp.UpdateUserInput{
		TargetID: producer, ExpectedRevision: 1, DisplayName: &newName, RequestID: "stale",
	}); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatalf("stale update: %v", err)
	}
	installAccountAuditRejection(ctx, t, conn.DB)
	rejectedName := "Rejected"
	if _, err := command.Execute(ctx, actorA, identityapp.UpdateUserInput{
		TargetID: producer, ExpectedRevision: 2, DisplayName: &rejectedName, RequestID: "rollback",
	}); err == nil {
		t.Fatal("account changed despite audit insertion failure")
	}
	removeAccountAuditRejection(ctx, t, conn.DB)
	current, err = store.FindByID(ctx, orgID, producer)
	if err != nil || current.DisplayName != newName || current.Revision != 2 {
		t.Fatalf("profile after rollback: name %q, revision %d, error %v",
			current.DisplayName, current.Revision, err)
	}
	if _, err := command.Execute(ctx, actorA, identityapp.UpdateUserInput{
		TargetID: outsider, ExpectedRevision: 1, DisplayName: &newName, RequestID: "cross-org",
	}); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("cross-organization update: %v", err)
	}
	var eventCount int64
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, orgID.String()).Scan(&eventCount).Error; err != nil || eventCount != 2 {
		t.Fatalf("committed profile event count %d, %v", eventCount, err)
	}
	producerRole := domain.RoleProducer
	actors := []identityapp.Principal{actorA, {ID: adminB, OrgID: orgID, Role: domain.RoleAdmin}}
	var wg sync.WaitGroup
	results := make(chan error, len(actors))
	for _, actor := range actors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := command.Execute(ctx, actor, identityapp.UpdateUserInput{
				TargetID: actor.ID, ExpectedRevision: 1, Role: &producerRole, RequestID: "concurrent-demote",
			})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	var successes, lastAdminFailures int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrLastActiveAdmin):
			lastAdminFailures++
		default:
			t.Fatalf("concurrent administrator demotion: %v", err)
		}
	}
	if successes != 1 || lastAdminFailures != 1 {
		t.Fatalf("concurrent demotion successes %d, last-admin refusals %d", successes, lastAdminFailures)
	}
	var activeAdmins int64
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM identity."user" WHERE org_id = ?::uuid
		AND role = 'admin' AND status = 'active' AND NOT is_delete
	`, orgID.String()).Scan(&activeAdmins).Error; err != nil || activeAdmins != 1 {
		t.Fatalf("active administrators after race %d, %v", activeAdmins, err)
	}
	for _, actor := range actors {
		current, err := store.FindByID(ctx, orgID, actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = query.Execute(ctx, actor, identityapp.ListUsersInput{Limit: 1})
		if current.Role == domain.RoleProducer && !errors.Is(err, identityapp.ErrForbidden) {
			t.Fatalf("demoted administrator can list accounts: %v", err)
		}
	}
}
