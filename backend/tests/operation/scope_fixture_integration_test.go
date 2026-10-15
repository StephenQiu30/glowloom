package operation_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestOperationScopeFixturesInRollbackTransaction(t *testing.T) {
	database := operationStoreDB(t)
	tx := database.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("roll back operation scope fixtures: %v", err)
		}
	}()

	actor, project := operationStoreProject(t, tx)
	_, operationID, _, _ := operationStoreRows(t, tx, actor, project)
	store := operationStore(tx)
	operation, err := store.FindOperation(t.Context(), actor, project, operationID)
	if err != nil || operation.ID != operationID || operation.ProjectID != project {
		t.Fatalf("own explicit project operation = %+v: %v", operation, err)
	}

	peer := identityapp.Principal{ID: uuid.New(), OrgID: actor.OrgID}
	if err := tx.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password)
		VALUES(?,?,?,'Operation scope peer','producer','synthetic',false)`, peer.ID, peer.OrgID, "operation-scope-"+peer.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	peer = identityfixture.Session(t, tx, peer)
	if peer.ScopeID == actor.ScopeID || peer.OrgID != actor.OrgID {
		t.Fatal("peer fixture lacks a distinct personal scope in the same organization")
	}
	if _, err := store.FindOperation(t.Context(), peer, project, operationID); !errors.Is(err, workspaceapp.ErrProjectNotFound) {
		t.Fatalf("other personal scope operation error = %v", err)
	}
	withoutSession := actor
	withoutSession.SessionID = uuid.Nil
	if _, err := store.FindOperation(t.Context(), withoutSession, project, operationID); !errors.Is(err, workspaceapp.ErrScopeNotFound) {
		t.Fatalf("sessionless operation error = %v", err)
	}
}
