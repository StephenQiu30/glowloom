package operation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
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

	if _, _, err := pgmedia.NewIngestRepository(tx).FindOutput(t.Context(), operationID, 1); !errors.Is(err, pgmedia.ErrUnavailable) {
		t.Fatal("missing ingest authority did not fail closed", err)
	}
	ingest := pgmedia.NewScopedIngestRepository(tx, app.NewMediaWorkerScopeAccess)
	if _, found, err := ingest.FindOutput(t.Context(), operationID, 1); err != nil || found {
		t.Fatal("durable worker ingest authority rejected an empty output", found, err)
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
	if err := tx.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := ingest.FindOutput(t.Context(), operationID, 1); !errors.Is(err, pgmedia.ErrNotFound) || !errors.Is(err, workspaceapp.ErrScopeNotFound) {
		t.Fatal("disabled ingest worker retained output access", err)
	}
}

// batchAuthorityProbe stops before any business rows are read or changed.
type batchAuthorityProbe struct {
	seenProject uuid.UUID
	seenActor   identityapp.Principal
	write       bool
	calls       int
	err         error
}

func (p *batchAuthorityProbe) AuthorizeOperation(_ context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	p.seenProject, p.seenActor, p.write = project, actor, write
	p.calls++
	return p.err
}

func TestConfirmBatchQuotePoolPreservesRequestedProjectAuthority(t *testing.T) {
	database := operationStoreDB(t)
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 1}
	input := operationapp.ConfirmBatchQuoteInput{ProjectID: uuid.New(), BatchID: uuid.New(), RequestID: uuid.NewString()}
	stop := errors.New("stop after transaction-bound authority")
	probe := &batchAuthorityProbe{err: stop}
	store := pgoperation.NewScopedStore(database, func(*gorm.DB) operationapp.ProjectAccess { return probe }, nil)
	if _, err := store.ConfirmBatchQuote(t.Context(), actor, input); !errors.Is(err, stop) {
		t.Fatal("authority rejection not preserved", err)
	}
	if probe.calls != 1 || probe.seenProject != input.ProjectID || probe.seenActor.ID != actor.ID || probe.seenActor.ScopeID != actor.ScopeID || probe.seenActor.SessionID != actor.SessionID || !probe.write {
		t.Fatal("confirmation lost the requested project or principal before authorizing", probe.seenProject, probe.calls)
	}
}
