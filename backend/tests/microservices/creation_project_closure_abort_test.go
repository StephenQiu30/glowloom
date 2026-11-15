package microservices_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func closureAbortAccount(t *testing.T, f *sourcePolicyFixture, spec source.ProjectClosureSpec) {
	t.Helper()
	var account writefence.Authority
	for _, authority := range spec.Command.Authorities {
		if authority.Kind == writefence.Account {
			account = authority
		}
	}
	control := completedAuthority(t, pgfence.NewAuthorityStore(f.identity), account, true).Control
	for _, db := range []*gorm.DB{f.identity, f.creation, f.execution, f.media} {
		if err := pgfence.New(db).Install(t.Context(), control); err != nil {
			t.Fatal(err)
		}
	}
}
func closureAbortWriters(f *sourcePolicyFixture, loseMedia bool) *scopeBootstrapWriters {
	return &scopeBootstrapWriters{stores: map[writefence.Writer]*pgfence.Store{writefence.WriterIdentity: pgfence.New(f.identity), writefence.WriterCreation: pgfence.New(f.creation), writefence.WriterExecution: pgfence.New(f.execution), writefence.WriterMedia: pgfence.New(f.media)}, calls: map[writefence.Writer]int{}, lostMediaACK: loseMedia}
}
func closureAbortUseCase(t *testing.T, f *sourcePolicyFixture, store *pgsource.Store, writers *scopeBootstrapWriters) *source.ProjectClosures {
	t.Helper()
	exec, media := closureActualParticipants(t, f)
	service, err := source.NewProjectClosures(store, map[writefence.Writer]source.ProjectWorkReader{writefence.WriterCreation: store, writefence.WriterExecution: exec, writefence.WriterMedia: media}, writefence.NewCoordinator(pgfence.NewAuthorityStore(f.creation), writers))
	if err != nil {
		t.Fatal(err)
	}
	return service
}
func assertAbortedClose(t *testing.T, f *sourcePolicyFixture, store *pgsource.Store, spec source.ProjectClosureSpec, epoch int64) {
	t.Helper()
	state, err := store.ReadProjectClosure(t.Context(), spec.ID)
	if err != nil || !state.Aborted || state.GateClosed || state.Result != nil {
		t.Fatalf("rejected control held project permanently: %+v %v", state, err)
	}
	assertClosureProjectEpoch(t, f, epoch)
	var project struct {
		Status   string
		Revision int64
	}
	if err := f.creation.Raw(`SELECT status,revision FROM workspace.project WHERE id=?`, spec.ProjectID).Scan(&project).Error; err != nil || project.Status != "active" || project.Revision != 1 {
		t.Fatalf("denied close changed public project: %+v %v", project, err)
	}
	record, err := pgfence.New(f.creation).Load(t.Context(), spec.Command.ID)
	if err != nil || record.State != writefence.Frozen {
		t.Fatalf("denied close retained a write permit: %+v %v", record, err)
	}
}
func TestCreationProjectClosureRevokedBeforeAuthorityReopensOnlyAdmissionGate(t *testing.T) {
	f, store, spec := projectClosurePGFixture(t)
	if _, err := store.BeginProjectClosure(t.Context(), spec); err != nil {
		t.Fatal(err)
	}
	closureAbortAccount(t, f, spec)
	writers := closureAbortWriters(f, false)
	_, err := closureAbortUseCase(t, f, store, writers).Resolve(t.Context(), spec.ID)
	if !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("revoked control returned success: %v", err)
	}
	assertAbortedClose(t, f, store, spec, 1)
	for _, count := range writers.calls {
		if count != 0 {
			t.Fatal("pre-authority denial changed a project writer generation")
		}
	}
}
func TestCreationProjectClosureRevokedAfterCloseRecoversOriginalReopenACK(t *testing.T) {
	f, store, spec := projectClosurePGFixture(t)
	state, err := store.BeginProjectClosure(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	// The real idle drain is accepted independently in the full remote test. Here
	// pause its already accepted source phase just before the final actor Guard.
	if _, err := store.BeginClosedProjectAuthority(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	initial := closureAbortWriters(f, false)
	if _, err := writefence.NewCoordinator(pgfence.NewAuthorityStore(f.creation), initial).Set(t.Context(), spec.ProjectAuthority, spec.CoordinationID, true); err != nil {
		t.Fatal(err)
	}
	closureAbortAccount(t, f, spec)
	losing := closureAbortWriters(f, true)
	if _, err := closureAbortUseCase(t, f, store, losing).Resolve(t.Context(), spec.ID); err == nil {
		t.Fatal("missing original reopen ACK returned success")
	}
	pending, err := store.ReadProjectClosure(t.Context(), spec.ID)
	if err != nil || !pending.Aborting || !pending.GateClosed || pending.Cycle != 1 {
		t.Fatalf("unknown reopen discarded original gate: %+v %v", pending, err)
	}
	opening, err := pgfence.NewAuthorityStore(f.creation).LoadCoordination(t.Context(), spec.ReopenControl().CoordinationID)
	if err != nil {
		t.Fatal(err)
	}
	acknowledged := map[writefence.Writer]bool{}
	for _, writer := range opening.Acknowledged {
		acknowledged[writer] = true
	}
	rebuilt := closureAbortWriters(f, false)
	_, err = closureAbortUseCase(t, f, store, rebuilt).Resolve(t.Context(), spec.ID)
	if !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("original rejected control changed outcome: %v", err)
	}
	assertAbortedClose(t, f, store, spec, 3)
	for _, writer := range writefence.Writers() {
		want := 0
		if !acknowledged[writer] {
			want = 1
		}
		if rebuilt.calls[writer] != want {
			t.Fatalf("reopen replay installed %s %d times want=%d", writer, rebuilt.calls[writer], want)
		}
	}
	var head struct {
		CoordinationID    uuid.UUID
		Blocked, Complete bool
	}
	if err := f.creation.Raw(`SELECT coordination_id,blocked,complete FROM infra.authority_head WHERE service_owner='creation' AND issuer='creation' AND authority_kind='project' AND authority_id=?`, spec.ProjectID).Scan(&head).Error; err != nil || head.CoordinationID != spec.ReopenControl().CoordinationID || head.Blocked || !head.Complete {
		t.Fatalf("original reopen identity lost: %+v %v", head, err)
	}
}
