package billing_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestBillingScopeFixturesInRollbackTransaction(t *testing.T) {
	ctx, database := billingDB(t)
	tx := database.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("roll back billing scope fixtures: %v", err)
		}
	}()

	actor, project := billingProject(t, tx)
	seedReserveBudget(t, tx, project, 100)
	store := billingStore(tx)
	budget, err := store.FindBudget(ctx, actor, project)
	if err != nil || budget.ProjectID != project || budget.LimitMicros != 100 {
		t.Fatalf("own explicit project budget = %+v: %v", budget, err)
	}

	peer := identityapp.Principal{ID: uuid.New(), OrgID: actor.OrgID}
	if err := tx.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password)
		VALUES(?,?,?,'Billing scope peer','producer','synthetic',false)`, peer.ID, peer.OrgID, "billing-scope-"+peer.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	peer = identityfixture.Session(t, tx, peer)
	if peer.ScopeID == actor.ScopeID || peer.OrgID != actor.OrgID {
		t.Fatal("peer fixture lacks a distinct personal scope in the same organization")
	}
	if _, err := store.FindBudget(ctx, peer, project); !errors.Is(err, workspaceapp.ErrProjectNotFound) {
		t.Fatalf("other personal scope budget error = %v", err)
	}
	withoutSession := actor
	withoutSession.SessionID = uuid.Nil
	if _, err := store.FindBudget(ctx, withoutSession, project); !errors.Is(err, workspaceapp.ErrScopeNotFound) {
		t.Fatalf("sessionless budget error = %v", err)
	}
}
