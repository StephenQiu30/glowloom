package script_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	composition "github.com/StephenQiu30/glowloom-server/backend/internal/app"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	canvaspg "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	identitypg "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	scriptpg "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/postgres"
	scriptapp "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func scriptFixtureRequestScope(tx *gorm.DB) workspaceapp.RequestScopeResolver {
	return workspaceapp.NewRequestScopeAccess(workspacepg.NewRequestScopeStore(tx, func(db *gorm.DB) workspaceapp.SessionActorAccess { return identitypg.NewStore(db) }), time.Now)
}
func scriptFixtureWorkerScope(tx *gorm.DB) workspaceapp.WorkerScopeResolver {
	return workspaceapp.NewScopeAccess(workspacepg.NewScopeStore(tx, func(db *gorm.DB) workspaceapp.ActorAccess { return identitypg.NewStore(db) }))
}
func scriptFixtureProjectAccess(tx *gorm.DB) *workspacepg.ProjectContentAccessStore {
	return workspacepg.NewScopedProjectContentAccessStore(tx, scriptFixtureRequestScope(tx))
}
func scriptFixtureWorkerAccess(tx *gorm.DB) *workspacepg.ProjectContentAccessStore {
	return workspacepg.NewWorkerProjectContentAccessStore(tx, scriptFixtureWorkerScope(tx))
}
func scriptFixtureCopyAccess(tx *gorm.DB, authority workspaceapp.ProjectCopyAuthority) *workspacepg.ProjectCopyAccessStore {
	return workspacepg.NewScopedProjectCopyAccessStore(tx, authority, scriptFixtureRequestScope, scriptFixtureWorkerScope)
}
func scriptFixtureMediaStore(db *gorm.DB) *mediapg.Store {
	return mediapg.NewScopedStore(db, composition.NewMediaScopeAccess)
}
func scriptFixtureDocumentStore(db *gorm.DB) *mediapg.DocumentSourceStore {
	return mediapg.NewScopedDocumentSourceStore(db, composition.NewMediaScopeAccess)
}
func scriptFixtureWorkerDocumentStore(db *gorm.DB) *mediapg.DocumentSourceStore {
	return mediapg.NewScopedDocumentSourceStore(db, composition.NewMediaWorkerScopeAccess)
}
func scriptFixtureMediaCopyStore(db *gorm.DB) *mediapg.ProjectCopyStore {
	return mediapg.NewScopedProjectCopyStore(db, composition.NewMediaWorkerScopeAccess)
}

func scriptFixtureCanvasCopyStore(db *gorm.DB, media canvaspg.MediaReaderFactory, copied canvaspg.CopyMediaReaderFactory) *canvaspg.ProjectCopyStore {
	return canvaspg.NewScopedProjectCopyStore(db, media, copied, composition.NewWorkerCanvasProjectAccess)
}

func scriptFixtureWorkspaceCopyWithScript(db *gorm.DB, work workspacepg.ProjectWorkFactory, owners workspacepg.ProjectCopyOwnersFactory, script workspacepg.ProjectCopyScriptOwnerFactory) *workspacepg.ProjectCopyStore {
	return workspacepg.NewScopedProjectCopyStoreWithScript(db, scriptFixtureRequestScope, scriptFixtureWorkerScope, work, owners, script)
}

type scriptFixtureCopyBudgetAccess struct{ tx *gorm.DB }

func (a scriptFixtureCopyBudgetAccess) AuthorizeBilling(ctx context.Context, actor identityapp.Principal, project uuid.UUID, _ bool) error {
	return composition.NewMediaWorkerScopeAccess(a.tx).AuthorizeCopyTarget(ctx, actor, project)
}
func scriptFixtureBudgetStore(db *gorm.DB) *billingpg.Store {
	return billingpg.NewScopedStore(db, func(tx *gorm.DB) billingapp.ProjectAccess { return scriptFixtureCopyBudgetAccess{tx} })
}

// scriptFixtureTeamActor gives shared-content recovery tests an explicit team owner.
func scriptFixtureTeamActor(t *testing.T, db *gorm.DB, actor identityapp.Principal) identityapp.Principal {
	t.Helper()
	team, scope := uuid.New(), uuid.New()
	for _, row := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Content fixture team',?)`, []any{team, actor.ID}},
		{`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',now(),'created')`, []any{team, actor.ID}},
		{`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, []any{scope, team, actor.OrgID}},
	} {
		if err := db.Exec(row.sql, row.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	actor.ScopeID = scope
	return actor
}
func scriptFixtureTeamAdmin(t *testing.T, db *gorm.DB, actor identityapp.Principal) identityapp.Principal {
	t.Helper()
	actor = identityfixture.Session(t, db, actor)
	var binding struct{ ID, TeamID uuid.UUID }
	if err := db.Raw(`SELECT id,team_id FROM workspace.creation_scope WHERE kind='team' AND physical_scope_id=?`, actor.OrgID).Scan(&binding).Error; err != nil || binding.ID == uuid.Nil {
		t.Fatal("missing explicit fixture team", err)
	}
	if err := db.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',now(),'created')`, binding.TeamID, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	actor.ScopeID = binding.ID
	return actor
}

func scriptFixtureImportStore(db *gorm.DB, request scriptpg.ProjectAccessFactory, sources scriptpg.ImportSourcesFactory) *scriptpg.ImportStore {
	return scriptpg.NewImportStoreWithWorkerAccess(db, request, func(tx *gorm.DB) scriptapp.ProjectAccess { return scriptFixtureWorkerAccess(tx) }, sources)
}

// scriptFixtureNonmember has a valid session but no membership in the selected team.
func scriptFixtureNonmember(t *testing.T, db *gorm.DB, owner identityapp.Principal) identityapp.Principal {
	t.Helper()
	actor := identityapp.Principal{ID: uuid.New(), OrgID: owner.OrgID, Role: "producer"}
	if err := db.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,password_hash,role,status,must_change_password) VALUES(?,?,?,?,?,'producer','active',false)`, actor.ID, actor.OrgID, "script-outsider-"+actor.ID.String(), "No team membership", "synthetic-test-not-a-credential").Error; err != nil {
		t.Fatal(err)
	}
	actor = identityfixture.Session(t, db, actor)
	actor.ScopeID = owner.ScopeID
	return actor
}

func scriptFixtureWorkerMediaStore(db *gorm.DB) *mediapg.Store {
	return mediapg.NewScopedStore(db, composition.NewMediaWorkerScopeAccess)
}
