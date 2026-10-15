package identity_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestProjectScopeDoesNotFollowPhysicalOrganization(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	var installed bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='workspace' AND table_name='project' AND column_name='owner_scope_id')`).Scan(&installed).Error; err != nil {
		t.Fatal(err)
	}
	if !installed {
		migration, err := os.ReadFile("../../db/migrations/005_project_owner_scope.sql")
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(string(migration)).Error; err != nil {
			t.Fatal(err)
		}
	}
	_, actor := verifiedTestActor(t, tx, "project-scope@example.invalid", now)
	var personal struct{ ID, PhysicalScopeID uuid.UUID }
	if err := tx.Raw(`SELECT id,physical_scope_id FROM workspace.creation_scope WHERE owner_user_id=?`, actor.ID).Scan(&personal).Error; err != nil {
		t.Fatal(err)
	}
	actor.ScopeID = personal.ID
	actor.OrgID = personal.PhysicalScopeID
	team, teamScope, project := uuid.New(), uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Shared physical range',?)`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, teamScope, team, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',?,'created')`, team, actor.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'Team owned project','16:9','realistic')`, project, actor.OrgID, actor.ID, teamScope).Error; err != nil {
		t.Fatal(err)
	}
	scopes := workspace.NewRequestScopeAccess(pgworkspace.NewRequestScopeStore(tx, func(tx *gorm.DB) workspace.SessionActorAccess { return pgidentity.NewStore(tx) }), func() time.Time { return now })
	access := pgworkspace.NewScopedProjectContentAccessStore(tx, scopes)
	if _, err := pgworkspace.NewScopedProjectContentAccessStore(tx, nil).Authorize(t.Context(), actor, project, false); !errors.Is(err, workspace.ErrProjectDependencyUnavailable) {
		t.Fatalf("missing scope authority used legacy access: %v", err)
	}
	if _, err := access.Authorize(t.Context(), actor, project, false); !errors.Is(err, workspace.ErrProjectNotFound) {
		t.Fatalf("personal scope reached a team project through shared physical storage: %v", err)
	}
	actor.ScopeID = teamScope
	if _, err := access.Authorize(t.Context(), actor, project, true); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed',auth_epoch=auth_epoch+1 WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := access.Authorize(t.Context(), actor, project, false); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("removed membership retained project access: %v", err)
	}
}
