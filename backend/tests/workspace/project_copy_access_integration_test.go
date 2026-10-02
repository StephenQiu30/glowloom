package workspace_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func TestProjectCopyAccessPGAdmissionRetainsTrustedTransactionScope(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	target := uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))
	lifecycleFixtureSQL(t, owner, `UPDATE workspace.project SET status='copying' WHERE id=?`, target)
	binding := workspaceapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: source, TargetProjectID: target}
	authority := workspaceapp.ProjectCopyAuthority{Binding: binding, ActorID: actor.ID, SourceRevision: 1, Phase: "freeze"}
	if err := workspacepg.NewProjectCopyAccessStore(db, authority).Authorize(ctx, actor, binding, false); !errors.Is(err, workspaceapp.ErrProjectDependencyUnavailable) {
		t.Fatal("pool cannot retain owning copy locks", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		access := workspacepg.NewProjectCopyAccessStore(tx, authority)
		for _, targetRead := range []bool{false, true} {
			if err := access.Authorize(ctx, actor, binding, targetRead); err != nil {
				t.Fatal("trusted admission missing before job insertion", targetRead, err)
			}
		}
		wrong := binding
		wrong.JobID = uuid.New()
		if err := access.Authorize(ctx, actor, wrong, true); !errors.Is(err, domain.ErrInvalidProjectCopy) {
			t.Fatal("another client-selected job accepted", err)
		}
		stale := authority
		stale.SourceRevision++
		if err := workspacepg.NewProjectCopyAccessStore(tx, stale).Authorize(ctx, actor, binding, false); !errors.Is(err, domain.ErrProjectRevisionConflict) {
			t.Fatal("unfrozen source revision accepted", err)
		}
		if _, err := workspacepg.NewProjectContentAccessStore(tx).Authorize(ctx, actor, target, false); !errors.Is(err, workspaceapp.ErrProjectNotFound) {
			t.Fatal("copy authority changed ordinary target visibility", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	lifecycleFixtureSQL(t, owner, `UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID)
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := workspacepg.NewProjectCopyAccessStore(tx, authority).Authorize(ctx, actor, binding, true); !errors.Is(err, identityapp.ErrForbidden) {
			t.Fatal("revoked current role reused trusted admission", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
