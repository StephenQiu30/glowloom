package app_test

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

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func purgeScopeFixture(t *testing.T) (*gorm.DB, identity.Principal, uuid.UUID, uuid.UUID, media.PurgeWorkID) {
	t.Helper()
	dsn := os.Getenv("LV_TEST_EXISTING_DB_DSN")
	if dsn == "" {
		t.Skip("existing database connection absent; integration not verified")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 20*time.Second)
	t.Cleanup(cancel)
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal("connect existing database")
	}
	t.Cleanup(func() { _ = conn.Close() })
	var before int64
	var packageInstalled bool
	if err := conn.DB.Raw(`SELECT to_regclass('media.package_job') IS NOT NULL`).Scan(&packageInstalled).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.Raw(`SELECT count(*) FROM workspace.project WHERE name='scope-review-purge'`).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var after int64
		var packageAfter bool
		if err := conn.DB.Raw(`SELECT to_regclass('media.package_job') IS NOT NULL`).Scan(&packageAfter).Error; err != nil || packageInstalled != packageAfter {
			t.Error("package schema rollback verification failed")
		}
		if err := conn.DB.Raw(`SELECT count(*) FROM workspace.project WHERE name='scope-review-purge'`).Scan(&after).Error; err != nil || before != after {
			t.Error("purge fixture rollback verification failed")
		} else {
			t.Log("verified purge fixture rollback")
		}
	})
	tx := conn.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error("rollback purge scope test")
		}
	})
	if err := tx.Exec(`SET LOCAL lock_timeout='1s'; SET LOCAL statement_timeout='10s'`).Error; err != nil {
		t.Fatal(err)
	}
	for _, migration := range []string{"001_user_identity_expand.sql", "005_project_owner_scope.sql"} {
		var installed bool
		query := `SELECT to_regclass('identity.auth_challenge') IS NOT NULL`
		if strings.HasPrefix(migration, "005") {
			query = `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='workspace' AND table_name='project' AND column_name='owner_scope_id')`
		}
		if err := tx.Raw(query).Scan(&installed).Error; err != nil {
			t.Fatal(err)
		}
		if !installed {
			body, err := os.ReadFile("../../db/migrations/" + migration)
			if err != nil {
				t.Fatal(err)
			}
			if err := tx.Exec(string(body)).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	// This existing database predates package support. Install only the exact
	// canonical table declaration needed by the reference SELECT, inside the
	// rollback transaction; no package rows or persistent schema are created.
	if !packageInstalled {
		body, err := os.ReadFile("../../db/schema.sql")
		if err != nil {
			t.Fatal(err)
		}
		_, declaration, found := strings.Cut(string(body), "CREATE TABLE media.package_job (")
		if !found {
			t.Fatal("canonical package table declaration missing")
		}
		definition, _, found := strings.Cut(declaration, "\n);")
		if !found {
			t.Fatal("canonical package table declaration incomplete")
		}
		if err := tx.Exec("CREATE TABLE media.package_job (" + definition + "\n);").Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	hash, err := identitydomain.HashPassword("test only sufficiently long password", "")
	if err != nil {
		t.Fatal(err)
	}
	user := identitydomain.User{ID: uuid.New(), OrgID: uuid.New(), LoginName: uuid.NewString(), Email: uuid.NewString() + "@purge.invalid", EmailVersion: 1, EmailVerifiedAt: now, DisplayName: "purge scope test", PasswordHash: hash, PasswordChangedAt: now, TermsVersion: "test", TermsAcceptedAt: now}
	store := pgidentity.NewStore(tx)
	if err := store.EmailTransaction(t.Context(), func(t identity.EmailTx) error { return t.CreatePersonalAccount(user) }); err != nil {
		t.Fatal(err)
	}
	protector, err := credentialproof.New("test", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	auth := identity.NewEmailAuth(store, protector, identity.EmailPolicy{TermsVersion: "test"}, time.Now)
	login, err := auth.Login(t.Context(), user.Email, "test only sufficiently long password", false)
	if err != nil {
		t.Fatal(err)
	}
	actor := identity.Principal{ID: user.ID, OrgID: user.OrgID, Role: login.User.Role, SessionID: login.Session.ID, CredentialEpoch: login.User.SessionEpoch}
	team, scope, project := uuid.New(), uuid.New(), uuid.New()
	actor.ScopeID = scope
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'purge review',?)`, []any{team, actor.ID}},
		{`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, []any{scope, team, actor.OrgID}},
		{`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',now(),'created')`, []any{team, actor.ID}},
		{`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'scope-review-purge','16:9','realistic')`, []any{project, actor.OrgID, actor.ID, scope}},
	}
	for _, s := range statements {
		if err := tx.Exec(s.sql, s.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	lib, err := (mediadomain.LibraryScope{Kind: mediadomain.LibraryProject, ProjectID: &project}).Identity(actor.OrgID, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	job, item := uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO media.library(id,kind,org_id,project_id) VALUES(?,'project',?,?)`, lib, actor.OrgID, project).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO media.purge_job(id,org_id,actor_id,library_id,scope_kind,project_id,item_count,status,stage) VALUES(?,?,?,?,'project',?,1,'queued','frozen')`, job, actor.OrgID, actor.ID, lib, project).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO media.library_item(id,library_id,plain_text,title,category,position,catalog_state,trashed_at) VALUES(?,?,'test only','purge review','other',0,'trashed',now())`, item, lib).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO media.purge_item(job_id,item_index,item_id,frozen,status) VALUES(?,0,?,?,'queued')`, job, item, []byte(`{}`)).Error; err != nil {
		t.Fatal(err)
	}
	return tx, actor, team, project, media.PurgeWorkID{JobID: job, Attempt: 1, ExecutionID: uuid.NewString()}
}

func TestPurgeWorkerUsesCurrentMembershipWithoutRequestSession(t *testing.T) {
	tx, actor, team, project, work := purgeScopeFixture(t)
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewMediaLibraryProjectAccess(tx).Authorize(t.Context(), actor, project, true); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("revoked request session unexpectedly admitted: %v", err)
	}
	actor.SessionID = uuid.Nil
	actor.CredentialEpoch = 0
	actor.ScopeID = uuid.Nil
	_, workerStore := app.NewMediaPurgeWorker(tx, nil)
	lease, err := workerStore.ClaimPurge(t.Context(), work)
	if err != nil {
		t.Fatalf("purge worker required revoked request session: %v", err)
	}
	if err := workerStore.HeartbeatPurge(t.Context(), lease); err != nil {
		t.Fatalf("worker writeback: %v", err)
	}
	if _, err := app.NewMediaWorkerReferenceOwners(tx).HasMediaReferences(t.Context(), actor, project, uuid.New()); err != nil {
		t.Fatalf("worker reference factory required session: %v", err)
	}
	var leaseBefore time.Time
	if err := tx.Raw(`SELECT lease_until FROM media.purge_job WHERE id=?`, work.JobID).Scan(&leaseBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := workerStore.HeartbeatPurge(t.Context(), lease); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("removed member retained purge writeback: %v", err)
	}
	var leaseAfter time.Time
	if err := tx.Raw(`SELECT lease_until FROM media.purge_job WHERE id=?`, work.JobID).Scan(&leaseAfter).Error; err != nil {
		t.Fatal(err)
	}
	if !leaseAfter.Equal(leaseBefore) {
		t.Fatal("denied worker changed purge lease")
	}
	if _, err := app.NewMediaWorkerReferenceOwners(tx).HasMediaReferences(t.Context(), actor, project, uuid.New()); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("removed member retained worker reference access: %v", err)
	}
}
