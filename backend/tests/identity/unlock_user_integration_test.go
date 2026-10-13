package identity_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func TestUnlockActualLoginGuardAndAtomicAudit(t *testing.T) {
	dsn := os.Getenv("LV_TEST_UNLOCK_DB_DSN")
	if dsn == "" {
		t.Skip("requires a disposable schema-initialized PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := pgidentity.NewStore(conn.DB)
	org, admin, member := uuid.New(), uuid.New(), uuid.New()
	login := "unlock-" + member.String()
	hash, err := domain.HashPassword("unlockTestPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []domain.User{{ID: admin, OrgID: org, LoginName: "admin-" + admin.String(), DisplayName: "Admin", Role: domain.RoleAdmin, PasswordHash: hash}, {ID: member, OrgID: org, LoginName: login, DisplayName: "Member", Role: domain.RoleProducer, PasswordHash: hash}} {
		if err := store.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if err := conn.DB.WithContext(ctx).Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE identity."user" SET must_change_password=false WHERE id=?`, admin)
	exec(`UPDATE identity."user" SET status='disabled',failed_login_count=5,locked_until=now()+interval '1 hour' WHERE id=?`, member)
	exec(`INSERT INTO identity.login_guard(login_key,failure_times,locked_until) VALUES(?,ARRAY[now()],now()+interval '1 hour')`, login)
	actor := application.Principal{ID: admin, OrgID: org, Role: domain.RoleAdmin}
	cmd := application.NewUnlockUserCommand(store, time.Now)
	input := application.UnlockUserInput{TargetID: member, ExpectedRevision: 1, RequestID: "unlock-audit"}
	page, err := application.NewListUsersQuery(store).Execute(ctx, actor, application.ListUsersInput{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	locked := false
	for _, u := range page.Users {
		if u.ID == member {
			locked = u.LoginLocked
		}
	}
	if !locked {
		t.Fatal("directory omitted actual login lock")
	}
	installAccountAuditRejection(ctx, t, conn.DB)
	if _, err := cmd.Execute(ctx, actor, input); err == nil {
		t.Fatal("unlock ignored failed audit")
	}
	removeAccountAuditRejection(ctx, t, conn.DB)
	var stillLocked bool
	if err := conn.DB.WithContext(ctx).Raw(`SELECT locked_until>now() FROM identity.login_guard WHERE login_key=?`, login).Scan(&stillLocked).Error; err != nil || !stillLocked {
		t.Fatalf("guard rollback failed: %v", err)
	}
	assertAccountState(ctx, t, store, org, member, domain.StatusDisabled, 1, 1, hash, true)
	result, err := cmd.Execute(ctx, actor, input)
	if err != nil || result.Revision != 2 || result.Status != domain.StatusDisabled {
		t.Fatalf("unlock: %+v %v", result, err)
	}
	assertAccountState(ctx, t, store, org, member, domain.StatusDisabled, 1, 2, hash, true)
	var cleared bool
	if err := conn.DB.WithContext(ctx).Raw(`SELECT locked_until IS NULL AND cardinality(failure_times)=0 FROM identity.login_guard WHERE login_key=?`, login).Scan(&cleared).Error; err != nil || !cleared {
		t.Fatalf("actual auth lock not cleared: %v", err)
	}
	if _, err := cmd.Execute(ctx, actor, input); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	input.ExpectedRevision = 2
	if _, err := cmd.Execute(ctx, actor, input); !errors.Is(err, domain.ErrAccountUnchanged) {
		t.Fatalf("no lock: %v", err)
	}
	// A new lock after an earlier unlock must not be mistaken for the earlier result.
	exec(`UPDATE identity.login_guard SET locked_until=now()+interval '1 hour' WHERE login_key=?`, login)
	actor.OrgID = uuid.New()
	if _, err := cmd.Execute(ctx, actor, input); err == nil {
		t.Fatal("cross-organization unlock allowed")
	}
	actor.OrgID = org
	exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, admin)
	if _, err := cmd.Execute(ctx, actor, input); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("stale admin allowed: %v", err)
	}
	var count int64
	if err := conn.DB.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key=?`, org.String()).Scan(&count).Error; err != nil || count != 2 {
		t.Fatalf("audit count %d: %v", count, err)
	}
}
