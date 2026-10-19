package bible_test

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

func bibleFixtureRequestScope(tx *gorm.DB) workspaceapp.RequestScopeResolver {
	return workspaceapp.NewRequestScopeAccess(workspacepg.NewRequestScopeStore(tx, func(db *gorm.DB) workspaceapp.SessionActorAccess { return identitypg.NewStore(db) }), time.Now)
}
func bibleFixtureWorkerScope(tx *gorm.DB) workspaceapp.WorkerScopeResolver {
	return workspaceapp.NewScopeAccess(workspacepg.NewScopeStore(tx, func(db *gorm.DB) workspaceapp.ActorAccess { return identitypg.NewStore(db) }))
}
func bibleFixtureProjectAccess(tx *gorm.DB) *workspacepg.ProjectContentAccessStore {
	return workspacepg.NewScopedProjectContentAccessStore(tx, bibleFixtureRequestScope(tx))
}
func bibleFixtureWorkerAccess(tx *gorm.DB) *workspacepg.ProjectContentAccessStore {
	return workspacepg.NewWorkerProjectContentAccessStore(tx, bibleFixtureWorkerScope(tx))
}
func bibleFixtureCopyAccess(tx *gorm.DB, authority workspaceapp.ProjectCopyAuthority) *workspacepg.ProjectCopyAccessStore {
	return workspacepg.NewScopedProjectCopyAccessStore(tx, authority, bibleFixtureRequestScope, bibleFixtureWorkerScope)
}
func bibleFixtureMediaStore(db *gorm.DB) *mediapg.Store {
	return mediapg.NewScopedStore(db, composition.NewMediaScopeAccess)
}
func bibleFixtureDocumentStore(db *gorm.DB) *mediapg.DocumentSourceStore {
	return mediapg.NewScopedDocumentSourceStore(db, composition.NewMediaScopeAccess)
}
func bibleFixtureWorkerDocumentStore(db *gorm.DB) *mediapg.DocumentSourceStore {
	return mediapg.NewScopedDocumentSourceStore(db, composition.NewMediaWorkerScopeAccess)
}
func bibleFixtureMediaCopyStore(db *gorm.DB) *mediapg.ProjectCopyStore {
	return mediapg.NewScopedProjectCopyStore(db, composition.NewMediaWorkerScopeAccess)
}
func bibleFixtureLibraryStore(db *gorm.DB, access mediapg.LibraryProjectAccessFactory, clock func() time.Time) *mediapg.LibraryStore {
	return mediapg.NewScopedLibraryStore(db, access, clock, composition.NewMediaScopeAccess)
}
func bibleFixtureCanvasStore(db *gorm.DB, media canvaspg.MediaReaderFactory) *canvaspg.Store {
	return canvaspg.NewScopedStore(db, media, composition.NewCanvasProjectAccess)
}
func bibleFixtureCanvasCopyStore(db *gorm.DB, media canvaspg.MediaReaderFactory, copied canvaspg.CopyMediaReaderFactory) *canvaspg.ProjectCopyStore {
	return canvaspg.NewScopedProjectCopyStore(db, media, copied, composition.NewWorkerCanvasProjectAccess)
}
func bibleFixtureWorkspaceStore(db *gorm.DB) *workspacepg.Store {
	return workspacepg.NewScopedStore(db, bibleFixtureRequestScope, nil, nil)
}
func bibleFixtureWorkspaceCopyStore(db *gorm.DB, work workspacepg.ProjectWorkFactory, owners workspacepg.ProjectCopyOwnersFactory) *workspacepg.ProjectCopyStore {
	return workspacepg.NewScopedProjectCopyStore(db, bibleFixtureRequestScope, bibleFixtureWorkerScope, work, owners)
}
func bibleFixtureWorkspaceCopyWithScript(db *gorm.DB, work workspacepg.ProjectWorkFactory, owners workspacepg.ProjectCopyOwnersFactory, script workspacepg.ProjectCopyScriptOwnerFactory) *workspacepg.ProjectCopyStore {
	return workspacepg.NewScopedProjectCopyStoreWithScript(db, bibleFixtureRequestScope, bibleFixtureWorkerScope, work, owners, script)
}
func bibleFixtureWorkspaceCopyWithBible(db *gorm.DB, work workspacepg.ProjectWorkFactory, owners workspacepg.ProjectCopyOwnersFactory, script workspacepg.ProjectCopyScriptOwnerFactory, bible workspacepg.ProjectCopyBibleOwnerFactory) *workspacepg.ProjectCopyStore {
	return workspacepg.NewScopedProjectCopyStoreWithBible(db, bibleFixtureRequestScope, bibleFixtureWorkerScope, work, owners, script, bible)
}

type bibleFixtureCopyBudgetAccess struct{ tx *gorm.DB }

func (a bibleFixtureCopyBudgetAccess) AuthorizeBilling(ctx context.Context, actor identityapp.Principal, project uuid.UUID, _ bool) error {
	return composition.NewMediaWorkerScopeAccess(a.tx).AuthorizeCopyTarget(ctx, actor, project)
}
func bibleFixtureBudgetStore(db *gorm.DB) *billingpg.Store {
	return billingpg.NewScopedStore(db, func(tx *gorm.DB) billingapp.ProjectAccess { return bibleFixtureCopyBudgetAccess{tx} })
}

// bibleFixtureTeamActor gives shared-content recovery tests an explicit team owner.
func bibleFixtureTeamActor(t *testing.T, db *gorm.DB, actor identityapp.Principal) identityapp.Principal {
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
func bibleFixtureTeamAdmin(t *testing.T, db *gorm.DB, actor identityapp.Principal) identityapp.Principal {
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

func bibleFixtureImportStore(db *gorm.DB, request scriptpg.ProjectAccessFactory, sources scriptpg.ImportSourcesFactory) *scriptpg.ImportStore {
	return scriptpg.NewImportStoreWithWorkerAccess(db, request, func(tx *gorm.DB) scriptapp.ProjectAccess { return bibleFixtureWorkerAccess(tx) }, sources)
}

func bibleFixtureWorkerMediaStore(db *gorm.DB) *mediapg.Store {
	return mediapg.NewScopedStore(db, composition.NewMediaWorkerScopeAccess)
}
