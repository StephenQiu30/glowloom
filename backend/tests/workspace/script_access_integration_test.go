package workspace_test

import (
	"errors"
	"math"
	"testing"

	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func TestScriptProjectAccessPGRequiresOwningTransactionAndCurrentFacts(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	if _, err := workspacepg.NewProjectContentAccessStore(db).Authorize(ctx, actor, project, true); !errors.Is(err, workspaceapp.ErrProjectDependencyUnavailable) {
		t.Fatal("project lock escaped its owning transaction", err)
	}
	rollback := errors.New("owning script rollback")
	err := db.Transaction(func(tx *gorm.DB) error {
		access := workspacepg.NewProjectContentAccessStore(tx)
		facts, err := access.Authorize(ctx, actor, project, true)
		if err != nil || facts.ProjectID != project || facts.OrgID != actor.OrgID || facts.Revision != 1 {
			t.Fatal("current project facts", facts, err)
		}
		if revision, err := access.TouchContent(ctx, actor, project, 1); err != nil || revision != 2 {
			t.Fatal("content owner revision", revision, err)
		}
		if _, err := access.TouchContent(ctx, actor, project, 1); !errors.Is(err, domain.ErrProjectRevisionConflict) {
			t.Fatal("stale content CAS accepted", err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var revision int64
	if err := owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, project).Scan(&revision).Error; err != nil || revision != 1 {
		t.Fatal("script rollback leaked project revision", revision, err)
	}
	for _, status := range []string{"archived", "copying"} {
		if err := owner.Exec(`UPDATE workspace.project SET status=?,archived_at=CASE WHEN ?='archived' THEN now() ELSE NULL END WHERE id=?`, status, status, project).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			access := workspacepg.NewProjectContentAccessStore(tx)
			_, readErr := access.Authorize(ctx, actor, project, false)
			if status == "archived" && readErr != nil {
				t.Fatal("archived history unreadable", readErr)
			}
			if status == "copying" && !errors.Is(readErr, workspaceapp.ErrProjectNotFound) {
				t.Fatal("unpublished copy readable", readErr)
			}
			if _, writeErr := access.Authorize(ctx, actor, project, true); writeErr == nil {
				t.Fatal("non-active project writable", status)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := owner.Exec(`UPDATE workspace.project SET status='active',revision=? WHERE id=?`, math.MaxInt32, project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := workspacepg.NewProjectContentAccessStore(tx).TouchContent(ctx, actor, project, math.MaxInt32)
		if !errors.Is(err, domain.ErrProjectRevisionConflict) {
			t.Fatal("revision budget did not reject before overflow", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := workspacepg.NewProjectContentAccessStore(tx).Authorize(ctx, actor, project, false)
		if !errors.Is(err, identityapp.ErrForbidden) {
			t.Fatal("revoked actor reused frozen permission", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestScriptProjectAccessPGCommitsContentInvalidationAtomically(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := workspacepg.NewProjectContentAccessStore(tx).TouchContent(ctx, actor, project, 1)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var facts struct {
		Events, Revision int64
		Change, Topic    string
	}
	if err := owner.Raw(`SELECT (SELECT count(*) FROM infra.outbox WHERE partition_key=?) AS events,(SELECT revision FROM workspace.project WHERE id=?) AS revision,(SELECT payload->'data'->>'change' FROM infra.outbox WHERE partition_key=? LIMIT 1) AS change,(SELECT topic FROM infra.outbox WHERE partition_key=? LIMIT 1) AS topic`, project.String(), project, project.String(), project.String()).Scan(&facts).Error; err != nil {
		t.Fatal(err)
	}
	if facts.Events != 1 || facts.Revision != 2 || facts.Change != "updated" || facts.Topic != "lanverse.workspace.project_changed.v1" {
		t.Fatalf("content and actual invalidation diverged: %+v", facts)
	}
	for _, fault := range []string{"RAISE EXCEPTION 'synthetic script invalidation fault'", "RETURN NULL"} {
		t.Run(fault, func(t *testing.T) {
			rollback := errors.New("invalidation fault fixture rollback")
			err := owner.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec(`CREATE FUNCTION pg_temp.reject_script_invalidation() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN ` + fault + `;END$$`).Error; err != nil {
					return err
				}
				if err := tx.Exec(`CREATE TRIGGER script_invalidation_fault BEFORE INSERT ON infra.outbox FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_script_invalidation()`).Error; err != nil {
					return err
				}
				if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
					return err
				}
				if _, err := workspacepg.NewProjectContentAccessStore(tx).TouchContent(ctx, actor, project, 2); err == nil {
					t.Error("missing invalidation reported committed content")
				}
				return rollback
			})
			if !errors.Is(err, rollback) {
				t.Fatal(err)
			}
			var revision int64
			if err := owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, project).Scan(&revision).Error; err != nil || revision != 2 {
				t.Fatal("invalidation fault leaked project revision", revision, err)
			}
		})
	}
}
