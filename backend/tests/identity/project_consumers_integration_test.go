package identity_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func projectConsumerFixture(t *testing.T) (*gorm.DB, identity.Principal, uuid.UUID, uuid.UUID, uuid.UUID, pgworkspace.RequestScopeFactory) {
	t.Helper()
	verifyProjectConsumerRollback(t)
	tx := existingIdentityTransaction(t)
	var installed bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='workspace' AND table_name='project' AND column_name='owner_scope_id')`).Scan(&installed).Error; err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Fatal("database is missing workspace.project.owner_scope_id; initialize it from db/schema.sql")
	}
	installSecondaryScope(t, tx)
	installUserRuntimeDependencies(t, tx)
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, actor := verifiedTestActor(t, tx, "project-consumer@example.invalid", now)
	var personal struct{ ID, PhysicalScopeID uuid.UUID }
	if err := tx.Raw(`SELECT id,physical_scope_id FROM workspace.creation_scope WHERE owner_user_id=?`, actor.ID).Scan(&personal).Error; err != nil {
		t.Fatal(err)
	}
	actor.ScopeID, actor.OrgID = personal.ID, personal.PhysicalScopeID
	_, creator := verifiedTestActor(t, tx, "former-project-author@example.invalid", now)
	team, scope, project := uuid.New(), uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'consumer scope',?)`, team, creator.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, scope, team, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'member','active',?,'created'),(?,?,'team_admin','removed',?,'created')`, team, actor.ID, now, team, creator.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'Retained team work','16:9','realistic')`, project, actor.OrgID, creator.ID, scope).Error; err != nil {
		t.Fatal(err)
	}
	factory := func(tx *gorm.DB) workspace.RequestScopeResolver {
		return workspace.NewRequestScopeAccess(pgworkspace.NewRequestScopeStore(tx, func(tx *gorm.DB) workspace.SessionActorAccess { return pgidentity.NewStore(tx) }), func() time.Time { return now })
	}
	return tx, actor, team, scope, project, factory
}

func TestProjectConsumersUseScopeAndExactSession(t *testing.T) {
	tx, actor, team, scope, project, factory := projectConsumerFixture(t)
	store := pgworkspace.NewScopedStore(tx, factory, nil, nil)
	personalScope := actor.ScopeID
	actor.ScopeID = scope
	page, err := store.ListProjectsForActor(t.Context(), actor, workspace.ListProjectsInput{Limit: 20})
	if err != nil || len(page.Projects) != 1 || page.Projects[0].ID != project {
		t.Fatalf("team member cannot list former creator's team project: count=%d err=%v", len(page.Projects), err)
	}
	if _, err := store.FindProject(t.Context(), actor, project); err != nil {
		t.Fatalf("team project read: %v", err)
	}
	if err := store.RequireOwnedProject(t.Context(), actor, project); err != nil {
		t.Fatalf("route scope: %v", err)
	}
	if _, err := pgworkspace.NewScopedStore(tx, nil, nil, nil).FindProject(t.Context(), actor, project); !errors.Is(err, workspace.ErrProjectDependencyUnavailable) {
		t.Fatalf("missing authority did not fail closed: %v", err)
	}
	other := actor
	other.ScopeID = personalScope
	if _, err := factory(tx).Resolve(t.Context(), other, personalScope, false); err != nil {
		t.Fatalf("personal comparison scope is not valid: %v", err)
	}
	if _, err := store.FindProject(t.Context(), other, project); !errors.Is(err, workspace.ErrProjectNotFound) {
		t.Fatalf("valid personal scope read team project through physical org: %v", err)
	}
	if _, err := pgworkspace.NewScopedProjectContentAccessStore(tx, factory(tx)).TouchContent(t.Context(), other, project, 1); !errors.Is(err, workspace.ErrProjectNotFound) {
		t.Fatalf("valid personal scope wrote team project: %v", err)
	}
	other.ScopeID = uuid.New()
	if _, err := store.FindProject(t.Context(), other, project); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("other scope read: %v", err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	actor.PlatformRole = "platform_admin"
	if _, err := store.ListProjectsForActor(t.Context(), actor, workspace.ListProjectsInput{Limit: 20}); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("platform administrator bypassed membership: %v", err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='active' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.FindProject(t.Context(), actor, project); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("revoked exact session read: %v", err)
	}
}

func TestProjectContentWriteDoesNotRequireLegacyRole(t *testing.T) {
	tx, actor, _, scope, project, factory := projectConsumerFixture(t)
	actor.ScopeID = scope
	access := pgworkspace.NewScopedProjectContentAccessStore(tx, factory(tx))
	revision, err := access.TouchContent(t.Context(), actor, project, 1)
	if err != nil || revision != 2 {
		t.Fatalf("authorized team member content write: revision=%d err=%v", revision, err)
	}
	var author struct{ OwnerID uuid.UUID }
	if err := tx.Raw(`SELECT owner_id FROM workspace.project WHERE id=?`, project).Scan(&author).Error; err != nil {
		t.Fatal(err)
	}
	if author.OwnerID == actor.ID {
		t.Fatal("team member replaced original author")
	}
}

func TestQueuedProjectWorkRechecksMembershipAtDispatchAndWriteback(t *testing.T) {
	tx, actor, team, scope, project, factory := projectConsumerFixture(t)
	actor.ScopeID = scope
	requestID := uuid.New()
	if err := tx.Exec(`INSERT INTO script.request(actor_id,request_id,org_id,project_id,action,request_hash,created_at) VALUES(?,?,?,?,'file_import_create',repeat('a',64),now())`, actor.ID, requestID, actor.OrgID, project).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO script.import_job(id,org_id,project_id,actor_id,actor_role,request_id,request_hash,expected_script_revision,rights_confirmed_at,created_at) VALUES(?,?,?,?,NULL,?,repeat('a',64),0,now(),now())`, uuid.New(), actor.OrgID, project, actor.ID, requestID).Error; err != nil {
		t.Fatalf("worker admission required synthetic legacy role: %v", err)
	}
	workers := workspace.NewScopeAccess(pgworkspace.NewScopeStore(tx, func(tx *gorm.DB) workspace.ActorAccess { return pgidentity.NewStore(tx) }))
	access := pgworkspace.NewWorkerProjectContentAccessStore(tx, workers)
	// A worker must not require the initiating HTTP session to remain alive.
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	actor.SessionID = uuid.Nil
	actor.CredentialEpoch = 0
	actor.ScopeID = uuid.Nil
	if _, err := access.Authorize(t.Context(), actor, project, true); err != nil {
		t.Fatalf("active team worker cannot dispatch: %v", err)
	}
	if _, err := pgworkspace.NewScopedProjectContentAccessStore(tx, factory(tx)).Authorize(t.Context(), actor, project, true); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("worker was admitted as request: %v", err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := access.TouchContent(t.Context(), actor, project, 1); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("removed member wrote worker result: %v", err)
	}
	var revision int64
	if err := tx.Raw(`SELECT revision FROM workspace.project WHERE id=?`, project).Scan(&revision).Error; err != nil {
		t.Fatal(err)
	}
	if revision != 1 {
		t.Fatal("denied worker changed project")
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='active' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := access.Authorize(t.Context(), actor, project, true); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("disabled worker dispatched: %v", err)
	}
}

func TestProjectScopeRejectsUnmappedAndAllowsArchivedReadOnly(t *testing.T) {
	tx, actor, team, scope, project, factory := projectConsumerFixture(t)
	actor.ScopeID = scope
	access := pgworkspace.NewScopedProjectContentAccessStore(tx, factory(tx))
	if err := tx.Exec(`UPDATE workspace.project SET owner_scope_id=NULL WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := access.Authorize(t.Context(), actor, project, false); !errors.Is(err, workspace.ErrProjectNotFound) {
		t.Fatalf("unmapped project readable: %v", err)
	}
	if err := tx.Exec(`UPDATE workspace.project SET owner_scope_id=? WHERE id=?`, scope, project).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE workspace.team SET status='archived' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := access.Authorize(t.Context(), actor, project, false); err != nil {
		t.Fatalf("archived scope read denied: %v", err)
	}
	if _, err := access.TouchContent(t.Context(), actor, project, 1); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("archived scope writable: %v", err)
	}
	actor.OrgID = uuid.New()
	if _, err := access.Authorize(t.Context(), actor, project, false); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("forged physical scope accepted: %v", err)
	}
}

// Register before existingIdentityTransaction so its explicit rollback runs first.
func verifyProjectConsumerRollback(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("LV_TEST_EXISTING_DB_DSN")
	if dsn == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal("open rollback verification connection")
	}
	t.Cleanup(func() { _ = conn.Close() })
	read := func() (int64, int64) {
		var columns, fixtures int64
		if err := conn.DB.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema='workspace' AND table_name IN ('project','style_preset') AND column_name='owner_scope_id'`).Scan(&columns).Error; err != nil {
			t.Error("read rollback schema evidence")
		}
		if err := conn.DB.Raw(`SELECT count(*) FROM workspace.project WHERE name='Retained team work'`).Scan(&fixtures).Error; err != nil {
			t.Error("read rollback data evidence")
		}
		return columns, fixtures
	}
	columns, fixtures := read()
	t.Cleanup(func() {
		afterColumns, afterFixtures := read()
		if afterColumns != columns || afterFixtures != fixtures {
			t.Errorf("rollback leaked schema/data: columns %d->%d fixtures %d->%d", columns, afterColumns, fixtures, afterFixtures)
		} else {
			t.Log("verified rollback: owner-scope schema and fixture counts unchanged")
		}
	})
}

func TestCreateProjectPersistsSelectedScopeAndProtectsReplay(t *testing.T) {
	tx, actor, _, scope, _, factory := projectConsumerFixture(t)
	var personal struct{ ID uuid.UUID }
	if err := tx.Raw(`SELECT id FROM workspace.creation_scope WHERE owner_user_id=?`, actor.ID).Scan(&personal).Error; err != nil {
		t.Fatal(err)
	}
	actor.ScopeID = scope
	store := pgworkspace.NewScopedStore(tx, factory, nil, nil)
	command := workspace.NewCreateProjectCommand(store, time.Now)
	key := uuid.New()
	input := workspace.CreateProjectInput{Name: "Retained team work", AspectRatio: "16:9", StyleType: "realistic", RequestID: uuid.NewString()}
	created, err := command.ExecuteIdempotent(t.Context(), actor, key, input)
	if err != nil {
		t.Fatalf("scope-bound creation: %v", err)
	}
	var row struct{ OwnerID, OwnerScopeID uuid.UUID }
	if err := tx.Raw(`SELECT owner_id,owner_scope_id FROM workspace.project WHERE id=?`, created.ID).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.OwnerID != actor.ID || row.OwnerScopeID != scope {
		t.Fatal("creation did not preserve separate author and scope")
	}
	actor.ScopeID = personal.ID
	if _, err := command.ExecuteIdempotent(t.Context(), actor, key, input); !errors.Is(err, workspace.ErrIdempotencyConflict) {
		t.Fatalf("cross-scope replay leaked response: %v", err)
	}
}

type projectLockTrace struct {
	logger.Interface
	queries []string
}

func (l *projectLockTrace) Trace(_ context.Context, _ time.Time, query func() (string, int64), _ error) {
	sql, _ := query()
	if strings.HasPrefix(sql, "SELECT 1 FROM workspace.project WHERE") && strings.Contains(sql, "owner_scope_id=") {
		l.queries = append(l.queries, sql)
	}
}
func TestProjectAuthorizationTakesWriteLockOnFirstRead(t *testing.T) {
	tx, actor, _, scope, project, factory := projectConsumerFixture(t)
	actor.ScopeID = scope
	trace := &projectLockTrace{Interface: logger.Default.LogMode(logger.Silent)}
	store := pgworkspace.NewScopedStore(tx.Session(&gorm.Session{Logger: trace}), factory, nil, nil)
	if _, err := store.ReadModelDefaults(t.Context(), actor, project); err != nil {
		t.Fatal(err)
	}
	if len(trace.queries) != 1 || !strings.HasSuffix(trace.queries[0], "FOR SHARE") {
		t.Fatalf("read authorization lock mode=%v", trace.queries)
	}
	trace.queries = nil
	_, err := workspace.NewModelDefaultsService(store, time.Now).Save(t.Context(), actor, workspace.ModelDefaultsChange{ProjectID: project, ExpectedRevision: 1, DefaultModels: map[string]string{}, IdempotencyKey: uuid.New(), RequestID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.queries) != 1 || !strings.HasSuffix(trace.queries[0], "FOR UPDATE") {
		t.Fatalf("write authorization starts with shared lock: %v", trace.queries)
	}
}
