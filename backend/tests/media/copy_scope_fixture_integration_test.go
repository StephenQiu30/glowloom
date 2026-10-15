package media_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// This pool-backed probe reads no project content and writes no fixture rows.
// A missing actor must reach the real authority check inside a transaction.
func TestMediaCopyFixtureOpensTransactionAndRejectsMissingActor(t *testing.T) {
	database := mediaStoreDB(t)
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New()}
	binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: uuid.New(), TargetProjectID: uuid.New()}
	snapshot := mediaapp.ProjectCopySnapshot{}
	if _, err := pgmedia.NewScopedProjectCopyStore(database, app.NewMediaWorkerScopeAccess).Objects(t.Context(), actor, binding, snapshot); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("owner repository accepted a pool without coordinator transaction", err)
	}
	if _, err := mediaCopyStore(database).Objects(t.Context(), actor, binding, snapshot); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("fixture failed to check the current actor in its transaction", err)
	}
}

func TestMediaCopyFixtureRechecksOwnerInRollbackTransaction(t *testing.T) {
	database := mediaStoreDB(t)
	tx := database.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error("roll back synthetic media copy", err)
		}
	}()
	actor, project := mediaStoreProject(t, tx)
	binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
	if err := tx.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type,status) VALUES(?,?,?,?,'Copy fixture','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID, actor.ID, actor.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	repo := mediaCopyStore(tx)
	snapshot, err := repo.Freeze(t.Context(), actor, binding, time.Now())
	if err != nil {
		t.Fatal("freeze the empty synthetic source", err)
	}
	actor.SessionID = uuid.Nil
	if objects, err := repo.Objects(t.Context(), actor, binding, snapshot); err != nil || len(objects) != 0 {
		t.Fatal("current durable owner lost its frozen object set", err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Objects(t.Context(), actor, binding, snapshot); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("disabled owner retained frozen object access", err)
	}
}
