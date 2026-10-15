package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type testScopeActor struct{ store *pgidentity.Store }

func (a testScopeActor) CheckSession(ctx context.Context, actor identity.Principal, now time.Time) error {
	return a.store.CheckSession(ctx, actor, now)
}

func TestScopeRejectsRevokedSession(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, actor := verifiedTestActor(t, tx, "scope-session@example.invalid", now)
	var row struct{ ID uuid.UUID }
	if err := tx.Raw(`SELECT id FROM workspace.creation_scope WHERE owner_user_id=?`, actor.ID).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	scope := row.ID
	access := workspace.NewRequestScopeAccess(pgworkspace.NewRequestScopeStore(tx, func(tx *gorm.DB) workspace.SessionActorAccess { return testScopeActor{store: pgidentity.NewStore(tx)} }), func() time.Time { return now })
	if _, err := access.Resolve(t.Context(), actor, scope, true); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE id=?`, now, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := access.Resolve(t.Context(), actor, scope, false); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("revoked session retained scope: %v", err)
	}
}

func (a testScopeActor) CheckActive(ctx context.Context, id uuid.UUID) error {
	err := a.store.CheckActive(ctx, id)
	if errors.Is(err, identity.ErrForbidden) {
		return workspace.ErrScopeNotFound
	}
	return err
}

func TestScopeRechecksDurableMembership(t *testing.T) {
	tx := existingIdentityTransaction(t)
	actor, platform, org := uuid.New(), uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'scope fixture')`, org).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{actor, platform} {
		if err := tx.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,email_key,email_verified_at) VALUES(?,?,?,'fixture','producer','fixture-hash',false,?,now())`, id, org, id.String(), id.String()+"@scope.invalid").Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, platform).Error; err != nil {
		t.Fatal(err)
	}
	teamA, teamB, scopeA, scopeB, personal := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for i, team := range []uuid.UUID{teamA, teamB} {
		if err := tx.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'fixture team',?)`, team, actor).Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',now(),'created')`, team, actor).Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, []uuid.UUID{scopeA, scopeB}[i], team, org).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Exec(`INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id) VALUES(?,'personal',?,?)`, personal, actor, org).Error; err != nil {
		t.Fatal(err)
	}
	access := workspace.NewScopeAccess(pgworkspace.NewScopeStore(tx, func(tx *gorm.DB) workspace.ActorAccess { return testScopeActor{store: pgidentity.NewStore(tx)} }))
	if _, err := access.Resolve(t.Context(), actor, scopeA, true); err != nil {
		t.Fatal(err)
	}
	if _, err := access.Resolve(t.Context(), platform, scopeA, false); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatal("platform authority exposed private content")
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed',auth_epoch=auth_epoch+1 WHERE team_id=? AND user_id=?`, teamA, actor).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := access.Resolve(t.Context(), actor, scopeA, false); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatal("cached membership survived removal")
	}
	for _, scope := range []uuid.UUID{scopeB, personal} {
		if _, err := access.Resolve(t.Context(), actor, scope, true); err != nil {
			t.Fatal("removal affected another space")
		}
	}
	if err := tx.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor).Error; err != nil {
		t.Fatal(err)
	}
	for _, scope := range []uuid.UUID{scopeB, personal} {
		if _, err := access.Resolve(t.Context(), actor, scope, false); !errors.Is(err, workspace.ErrScopeNotFound) {
			t.Fatal("global disable failed to revoke scope")
		}
	}
}
