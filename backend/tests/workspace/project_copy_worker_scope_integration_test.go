package workspace_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestProjectCopyWorkerReadUsesDurableAuthorityWithoutRequestSession(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	tx := database.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer rollbackLifecycleTest(t, tx)

	actor, source := lifecycleActorProject(ctx, t, tx)
	store := projectCopyStore(tx)
	job, err := store.Create(ctx, actor, copyInput(source), time.Now())
	if err != nil {
		t.Fatal("admit synthetic copy", err)
	}
	worker, err := store.WorkerActor(ctx, actor.OrgID, job.ID)
	if err != nil {
		t.Fatal("restore durable worker actor", err)
	}
	if worker.ID != actor.ID || worker.OrgID != actor.OrgID || worker.ScopeID != actor.ScopeID || worker.SessionID != uuid.Nil || worker.CredentialEpoch != 0 {
		t.Fatal("worker authority synthesized or lost request-independent facts", worker)
	}
	if _, err := store.Find(ctx, worker, job.ID); !errors.Is(err, workspaceapp.ErrScopeNotFound) {
		t.Fatal("request read accepted a worker without a session", err)
	}
	if got, err := store.FindForWorker(ctx, worker, job.ID); err != nil || got.ID != job.ID {
		t.Fatal("durable worker could not read its bound job", got, err)
	}
	foreign := insertWorkspaceActor(ctx, t, tx, actor.OrgID.String())
	foreign.SessionID, foreign.CredentialEpoch = uuid.Nil, 0
	if _, err := store.FindForWorker(ctx, foreign, job.ID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("foreign actor read another execution's job", err)
	}
	lifecycleFixtureSQL(t, tx, `UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID)
	if _, err := store.FindForWorker(ctx, worker, job.ID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("disabled worker retained job access", err)
	}
}

func TestProjectCopyWorkerReadRechecksCurrentTeamMembership(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	tx := database.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer rollbackLifecycleTest(t, tx)
	actor := insertWorkspaceActor(ctx, t, tx, insertWorkspaceOrganization(ctx, t, tx))
	actor = workspaceTeamActor(t, tx, actor)
	source := uuid.MustParse(insertWorkspaceProject(ctx, t, tx, actor, "16:9", "realistic", nil))
	store := projectCopyStore(tx)
	job, err := store.Create(ctx, actor, copyInput(source), time.Now())
	if err != nil {
		t.Fatal("admit team-owned synthetic copy", err)
	}
	worker, err := store.WorkerActor(ctx, actor.OrgID, job.ID)
	if err != nil {
		t.Fatal("restore team worker", err)
	}
	if _, err := store.FindForWorker(ctx, worker, job.ID); err != nil {
		t.Fatal("active team worker read", err)
	}
	lifecycleFixtureSQL(t, tx, `UPDATE workspace.team_member SET status='removed' WHERE team_id=(SELECT team_id FROM workspace.creation_scope WHERE id=?) AND user_id=?`, actor.ScopeID, actor.ID)
	if _, err := store.FindForWorker(ctx, worker, job.ID); !errors.Is(err, workspaceapp.ErrScopeNotFound) {
		t.Fatal("removed team member retained worker read", err)
	}
}
