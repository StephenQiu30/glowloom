package workspace_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application/source"

	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/adapter/postgres"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/application"

	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/adapter/postgres"

	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application"

	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/execution/billing/adapter/postgres"

	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/billing/application"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/adapter/postgres"

	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/adapter/postgres"

	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
)

func workspaceRequestScope(tx *gorm.DB) workspaceapp.RequestScopeResolver {
	return workspaceapp.NewRequestScopeAccess(pgworkspace.NewRequestScopeStore(tx, func(db *gorm.DB) workspaceapp.SessionActorAccess {
		return pgidentity.NewStore(db)
	}), time.Now)
}

func workspaceWorkerScope(tx *gorm.DB) workspaceapp.WorkerScopeResolver {
	return workspaceapp.NewScopeAccess(pgworkspace.NewScopeStore(tx, func(db *gorm.DB) workspaceapp.ActorAccess {
		return pgidentity.NewStore(db)
	}))
}

func workspaceStore(db *gorm.DB) *pgworkspace.Store {
	return pgworkspace.NewScopedStore(db, workspaceRequestScope, nil, nil)
}

func workspaceStoreWithWork(db *gorm.DB, work pgworkspace.ProjectWorkFactory) *pgworkspace.Store {
	return pgworkspace.NewScopedStore(db, workspaceRequestScope, work, nil)
}

func workspaceStoreWithCover(db *gorm.DB, work pgworkspace.ProjectWorkFactory, cover pgworkspace.ProjectCoverFactory) *pgworkspace.Store {
	return pgworkspace.NewScopedStore(db, workspaceRequestScope, work, cover)
}

func workspaceProjectAccess(tx *gorm.DB) *pgworkspace.ProjectContentAccessStore {
	return pgworkspace.NewScopedProjectContentAccessStore(tx, workspaceRequestScope(tx))
}

func workspaceCopyAccess(tx *gorm.DB, authority workspaceapp.ProjectCopyAuthority) *pgworkspace.ProjectCopyAccessStore {
	return pgworkspace.NewScopedProjectCopyAccessStore(tx, authority, workspaceRequestScope, workspaceWorkerScope)
}

func workspaceCanvasWorkerScope(tx *gorm.DB) canvasapp.ProjectAccess {
	return app.NewWorkerCanvasProjectAccess(tx)
}

func workspaceMediaStore(db *gorm.DB) *pgmedia.Store {
	return pgmedia.NewScopedStore(db, app.NewMediaScopeAccess)
}

func workspaceCanvasStore(db *gorm.DB, media pgcanvas.MediaReaderFactory) *pgcanvas.Store {
	return pgcanvas.NewScopedStore(db, media, app.NewCanvasProjectAccess)
}

func workspaceMediaCopyStore(tx *gorm.DB) *pgmedia.ProjectCopyStore {
	return pgmedia.NewScopedProjectCopyStore(tx, app.NewMediaWorkerScopeAccess).WithRoutedStorage()
}

func workspaceCanvasCopyStore(tx *gorm.DB, source pgcanvas.MediaReaderFactory, target pgcanvas.CopyMediaReaderFactory) *pgcanvas.ProjectCopyStore {
	return pgcanvas.NewScopedProjectCopyStore(tx, source, target, workspaceCanvasWorkerScope)
}

func workspaceFolderStore(db *gorm.DB, work pgworkspace.ProjectWorkFactory, media pgworkspace.FolderMediaFactory) *pgworkspace.FolderStore {
	return pgworkspace.NewScopedFolderStore(db, workspaceRequestScope, work, media)
}

func workspaceCopyStore(db *gorm.DB, work pgworkspace.ProjectWorkFactory, owners pgworkspace.ProjectCopyOwnersFactory) *pgworkspace.ProjectCopyStore {
	return pgworkspace.NewScopedProjectCopyStore(db, workspaceRequestScope, workspaceWorkerScope, work, owners)
}

type workspaceBillingCopyAccess struct{ tx *gorm.DB }

func (a workspaceBillingCopyAccess) AuthorizeBilling(ctx context.Context, actor identityapp.Principal, project uuid.UUID, _ bool) error {
	return app.NewMediaWorkerScopeAccess(a.tx).AuthorizeCopyTarget(ctx, mediaBoundaryActor(actor), project)
}

func workspaceCopyBudgetStore(tx *gorm.DB) *pgbilling.Store {
	return pgbilling.NewScopedStore(tx, func(db *gorm.DB) billingapp.ProjectAccess {
		return workspaceBillingCopyAccess{tx: db}
	})
}

type workspaceOperationAccess struct {
	tx     *gorm.DB
	worker bool
}

func (a workspaceOperationAccess) AuthorizeOperation(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	access := workspaceProjectAccess(a.tx)
	if a.worker {
		access = pgworkspace.NewWorkerProjectContentAccessStore(a.tx, workspaceWorkerScope(a.tx))
	}
	_, err := access.Authorize(ctx, actor, project, write)
	if a.worker && (errors.Is(err, identityapp.ErrForbidden) || errors.Is(err, workspacesource.ErrScopeNotFound) || errors.Is(err, workspacesource.ErrProjectNotFound)) {
		return operationapp.ErrExecutionAuthorityRevoked
	}
	return err
}

func workspaceOperationStore(db *gorm.DB) *pgoperation.Store {
	return pgoperation.NewScopedStore(db, func(tx *gorm.DB) operationapp.ProjectAccess {
		return workspaceOperationAccess{tx: tx}
	}, func(tx *gorm.DB) operationapp.ProjectAccess {
		return workspaceOperationAccess{tx: tx, worker: true}
	}, func(tx *gorm.DB) operationapp.PlatformAccess { return pgidentity.NewStore(tx) }).WithExecutionAvailability(func(tx *gorm.DB) operationapp.ExecutionAvailability {
		return workspaceExecutionAvailability{tx: tx}
	})
}

// Lifecycle fixtures exercise admission locks with an uninstalled synthetic
// adapter; actual published models retain the production availability reader.
type workspaceExecutionAvailability struct{ tx *gorm.DB }

func (r workspaceExecutionAvailability) AvailableForNewExecution(ctx context.Context, version, release uuid.UUID, mode string) (bool, error) {
	var synthetic bool
	err := r.tx.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM catalog.model_profile_version v
 JOIN catalog.model_profile m ON m.id=v.model_profile_id JOIN catalog.provider p ON p.id=m.provider_id
 WHERE v.id=? AND ?=ANY(v.modes) AND NOT v.is_delete AND NOT m.is_delete AND m.status='active'
 AND p.adapter_key='test' AND p.key LIKE 'lifecycle-%' AND p.status='active' AND NOT p.is_delete)`, version, mode).Row().Scan(&synthetic)
	if err != nil || synthetic {
		return synthetic, err
	}
	return pgcatalog.NewStore(r.tx).AvailableForNewExecution(ctx, version, release, mode)
}

func workspaceMediaDocumentStore(db *gorm.DB) *pgmedia.DocumentSourceStore {
	return pgmedia.NewScopedDocumentSourceStore(db, app.NewMediaScopeAccess)
}

func workspaceReferenceGuard(tx *gorm.DB) *pgworkspace.MediaReferenceGuard {
	return pgworkspace.NewScopedMediaReferenceGuard(tx, workspaceProjectAccess(tx))
}

// workspaceTeamActor selects an explicitly owned team scope for fixtures that
// exercise revocation of shared content independently of account sessions.
func workspaceTeamActor(t *testing.T, db *gorm.DB, actor identityapp.Principal) identityapp.Principal {
	t.Helper()
	team, scope := uuid.New(), uuid.New()
	lifecycleFixtureSQL(t, db, `INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Fixture team',?)`, team, actor.ID)
	lifecycleFixtureSQL(t, db, `INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',now(),'created')`, team, actor.ID)
	lifecycleFixtureSQL(t, db, `INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, scope, team, actor.OrgID)
	actor.ScopeID = scope
	return actor
}

func TestWorkspaceFixtureBindsLiveSessionAndExplicitOwnerScope(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	tx := database.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer rollbackLifecycleTest(t, tx)
	actor := insertWorkspaceActor(ctx, t, tx, insertWorkspaceOrganization(ctx, t, tx))
	other := insertWorkspaceActor(ctx, t, tx, actor.OrgID.String())
	project := uuid.MustParse(insertWorkspaceProject(ctx, t, tx, actor, "16:9", "realistic", nil))
	if actor.SessionID == uuid.Nil || actor.ScopeID == uuid.Nil || actor.CredentialEpoch < 1 {
		t.Fatal("fixture did not acquire a real session and personal scope")
	}
	var ownership struct{ OwnerID, OwnerScopeID uuid.UUID }
	if err := tx.Raw(`SELECT owner_id,owner_scope_id FROM workspace.project WHERE id=?`, project).Scan(&ownership).Error; err != nil || ownership.OwnerID != actor.ID || ownership.OwnerScopeID != actor.ScopeID {
		t.Fatal("fixture ownership differs from its explicit actor", ownership, err)
	}
	store := workspaceStore(tx)
	if _, err := store.FindProject(ctx, actor, project); err != nil {
		t.Fatal("current session cannot read its explicit project", err)
	}
	if _, err := pgworkspace.NewStore(tx).FindProject(ctx, actor, project); !errors.Is(err, workspacesource.ErrProjectDependencyUnavailable) {
		t.Fatal("missing scope dependency did not fail closed", err)
	}
	if _, err := store.FindProject(ctx, other, project); !errors.Is(err, workspacesource.ErrProjectNotFound) {
		t.Fatal("another personal scope read the project", err)
	}
	forged := actor
	forged.ScopeID = other.ScopeID
	if _, err := store.FindProject(ctx, forged, project); !errors.Is(err, workspacesource.ErrScopeNotFound) {
		t.Fatal("forged selected scope was trusted", err)
	}
	stale := actor
	stale.CredentialEpoch++
	if _, err := store.FindProject(ctx, stale, project); !errors.Is(err, identityapp.ErrSessionInvalid) {
		t.Fatal("stale session epoch was trusted", err)
	}
}
