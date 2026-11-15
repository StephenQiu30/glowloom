package microservices_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func TestCreationProjectClosurePostgresOldLeaseAndBusyCycleCannotRecoverNewGate(t *testing.T) {
	f, store, spec := projectClosurePGFixture(t)
	state, err := store.BeginProjectClosure(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	business := fmt.Sprintf("%s/%d", spec.ID, state.Cycle)
	tasks := taskruntime.NewStore(f.creation).ForBusiness(business)
	old, err := tasks.Claim(t.Context(), []string{source.ProjectClosureTask}, "old-close-worker", time.Minute)
	if err != nil || old == nil {
		t.Fatal("original close task missing", err)
	}
	if _, err := store.ReadProjectClosureForTask(t.Context(), *old); err != nil {
		t.Fatal(err)
	}
	if err := f.admin.Exec(`UPDATE infra.task_work SET lease_until=statement_timestamp()-interval '1 second' WHERE service_owner='creation' AND kind=? AND business_id=?`, source.ProjectClosureTask, business).Error; err != nil {
		t.Fatal(err)
	}
	current, err := tasks.Claim(t.Context(), []string{source.ProjectClosureTask}, "replacement-close-worker", time.Minute)
	if err != nil || current == nil || current.Epoch <= old.Epoch {
		t.Fatal("replacement lease not persisted", err)
	}
	if _, err := store.ReadProjectClosureForTask(t.Context(), *old); !errors.Is(err, taskruntime.ErrLeaseLost) {
		t.Fatalf("old close lease was accepted: %v", err)
	}
	if _, err := store.ReadProjectClosureForTask(t.Context(), *current); err != nil {
		t.Fatal(err)
	}
	if err := store.ReopenBusyProjectGate(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	next, err := store.BeginProjectClosure(t.Context(), spec)
	if err != nil || next.Cycle != 2 {
		t.Fatal("same public ID did not create next known-busy cycle", err)
	}
	if _, err := store.ReadProjectClosureForTask(t.Context(), *current); !errors.Is(err, taskruntime.ErrLeaseLost) {
		t.Fatalf("old cycle's valid lease crossed new gate: %v", err)
	}
	if err := store.ReopenBusyProjectGate(t.Context(), state); !errors.Is(err, taskruntime.ErrLeaseLost) {
		t.Fatalf("old cycle reopened new admission: %v", err)
	}
	assertClosureProjectEpoch(t, f, 1)
}

func TestCreationProjectClosurePostgresAdmissionFirstIsIncludedBeforeGate(t *testing.T) {
	f, store, spec := projectClosurePGFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	blocker := f.creation.WithContext(ctx).Begin()
	if blocker.Error != nil {
		t.Fatal(blocker.Error)
	}
	defer func() { _ = blocker.Rollback().Error }()
	if err := blocker.Exec(`SELECT epoch FROM infra.authority_head WHERE service_owner='creation' AND issuer='creation' AND authority_kind='scope' AND authority_id=? FOR UPDATE`, spec.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	admitted := make(chan error, 1)
	go func() {
		_, err := store.RegisterSourceCommand(ctx, source.Identity{AccountID: spec.ActorID, SessionID: spec.SessionID, CredentialEpoch: 1}, source.AdmissionRequest{Command: f.request.Command, Target: writefence.WriterExecution, ScopeID: spec.ScopeID, ProjectID: spec.ProjectID})
		admitted <- err
	}()
	waitSourcePolicyBlock(ctx, t, f.admin, "owner_b_creation_test", "authority_head")
	closed := make(chan error, 1)
	go func() { _, err := store.BeginProjectClosure(ctx, spec); closed <- err }()
	waitSourcePolicyBlock(ctx, t, f.admin, "owner_b_creation_test", "SELECT owner_scope_id FROM workspace.project")
	if err := blocker.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-admitted; err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	state, err := store.ReadProjectClosure(t.Context(), spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := store.ReadProjectClosureCommands(t.Context(), state, writefence.WriterExecution, uuid.Nil, source.ProjectWorkPageSize)
	if err != nil || len(commands) != 1 || commands[0].ID != f.request.Command.ID {
		t.Fatalf("pre-gate registered responsibility omitted: %+v %v", commands, err)
	}
}

func TestCreationProjectClosurePostgresGateFirstRejectsWaitingNewAdmission(t *testing.T) {
	f, store, spec := projectClosurePGFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	blocker := f.creation.WithContext(ctx).Begin()
	if blocker.Error != nil {
		t.Fatal(blocker.Error)
	}
	defer func() { _ = blocker.Rollback().Error }()
	if err := blocker.Exec(`SELECT id FROM workspace.project WHERE id=? FOR UPDATE`, spec.ProjectID).Error; err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { _, err := store.BeginProjectClosure(ctx, spec); closed <- err }()
	waitSourcePolicyBlock(ctx, t, f.admin, "owner_b_creation_test", "SELECT owner_scope_id FROM workspace.project")
	admitted := make(chan error, 1)
	go func() {
		_, err := store.RegisterSourceCommand(ctx, source.Identity{AccountID: spec.ActorID, SessionID: spec.SessionID, CredentialEpoch: 1}, source.AdmissionRequest{Command: f.request.Command, Target: writefence.WriterExecution, ScopeID: spec.ScopeID, ProjectID: spec.ProjectID})
		admitted <- err
	}()
	waitSourcePolicyBlock(ctx, t, f.admin, "owner_b_creation_test", "SELECT status,is_delete FROM workspace.project")
	if err := blocker.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if err := <-admitted; !errors.Is(err, source.ErrProjectClosureBusy) {
		t.Fatalf("waiting source permit crossed committed close gate: %v", err)
	}
	assertClosureProjectEpoch(t, f, 1)
}
