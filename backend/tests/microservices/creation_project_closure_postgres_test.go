package microservices_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// Explicit initial heads remain policy fixtures. These tests accept the real
// restricted-role gate, ledger, writer barriers and final atomic project change.
func projectClosurePGFixture(t *testing.T) (*sourcePolicyFixture, *pgsource.Store, source.ProjectClosureSpec) {
	t.Helper()
	f := newSourcePolicyFixture(t)
	for _, authority := range f.request.Command.Authorities {
		var control writefence.Control
		if authority.Issuer == writefence.Identity {
			authority.Epoch = 0
			control = completedAuthority(t, pgfence.NewAuthorityStore(f.identity), authority, false).Control
		} else {
			var row struct{ CoordinationID uuid.UUID }
			if err := f.admin.Raw(`SELECT coordination_id FROM infra.authority_head WHERE service_owner='creation' AND issuer=? AND authority_kind=? AND authority_id=?`, authority.Issuer, authority.Kind, authority.ID).Scan(&row).Error; err != nil {
				t.Fatal(err)
			}
			state, err := pgfence.NewAuthorityStore(f.creation).LoadCoordination(t.Context(), row.CoordinationID)
			if err != nil {
				t.Fatal(err)
			}
			control = state.Control
		}
		for _, db := range []*gorm.DB{f.identity, f.creation, f.execution, f.media} {
			if err := pgfence.New(db).Install(t.Context(), control); err != nil {
				t.Fatal(err)
			}
		}
	}
	var org struct{ OrgID uuid.UUID }
	if err := f.creation.Raw(`SELECT org_id FROM workspace.project WHERE id=?`, f.request.ProjectID).Scan(&org).Error; err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	spec := source.ProjectClosureSpec{ActorID: f.request.ActorID, SessionID: f.request.SessionID, ScopeID: f.request.ScopeID, OrganizationID: org.OrgID, ProjectID: f.request.ProjectID, RequestID: uuid.New(), IdempotencyKey: key, ExpectedRevision: 1, Action: "archive"}
	spec.ID = uuid.NewSHA1(spec.ActorID, []byte("creation.project.close/"+key.String()))
	spec.CoordinationID = uuid.NewSHA1(spec.ID, []byte("creation.project.close.authority"))
	spec.Command = writefence.Command{ID: uuid.NewSHA1(spec.ID, []byte("creation.project.close.command")), Action: "creation.project.archive"}
	for _, authority := range f.request.Command.Authorities {
		if authority.Kind == writefence.Project {
			spec.ProjectAuthority = authority
		} else {
			spec.Command.Authorities = append(spec.Command.Authorities, authority)
		}
	}
	var err error
	spec.Command.SHA256, err = spec.CommandSHA256()
	if err != nil || spec.Validate() != nil {
		t.Fatal("invalid closure spec", err)
	}
	store := pgsource.New(f.creation)
	identity := source.Identity{AccountID: spec.ActorID, SessionID: spec.SessionID, CredentialEpoch: 1}
	if _, err := store.RegisterSourceCommand(t.Context(), identity, source.AdmissionRequest{Command: spec.Command, Target: writefence.WriterCreation, ScopeID: spec.ScopeID}); err != nil {
		t.Fatal(err)
	}
	if _, err := pgfence.New(f.creation).Admit(t.Context(), spec.Command); err != nil {
		t.Fatal(err)
	}
	return f, store, spec
}
func closurePGUseCase(t *testing.T, f *sourcePolicyFixture, store *pgsource.Store, exec, media source.ProjectWorkReader) *source.ProjectClosures {
	t.Helper()
	writers := &scopeBootstrapWriters{stores: map[writefence.Writer]*pgfence.Store{writefence.WriterIdentity: pgfence.New(f.identity), writefence.WriterCreation: pgfence.New(f.creation), writefence.WriterExecution: pgfence.New(f.execution), writefence.WriterMedia: pgfence.New(f.media)}, calls: map[writefence.Writer]int{}}
	service, err := source.NewProjectClosures(store, map[writefence.Writer]source.ProjectWorkReader{writefence.WriterCreation: store, writefence.WriterExecution: exec, writefence.WriterMedia: media}, writefence.NewCoordinator(pgfence.NewAuthorityStore(f.creation), writers))
	if err != nil {
		t.Fatal(err)
	}
	return service
}
func assertClosureProjectEpoch(t *testing.T, f *sourcePolicyFixture, want int64) {
	t.Helper()
	var epoch int64
	if err := f.creation.Raw(`SELECT epoch FROM infra.authority_head WHERE service_owner='creation' AND issuer='creation' AND authority_kind='project' AND authority_id=?`, f.request.ProjectID).Scan(&epoch).Error; err != nil || epoch != want {
		t.Fatalf("project epoch=%d want=%d: %v", epoch, want, err)
	}
}

func TestCreationProjectClosurePostgresGateBlocksNewAndKeepsOriginalRegisteredWork(t *testing.T) {
	f, store, spec := projectClosurePGFixture(t)
	id := source.Identity{AccountID: spec.ActorID, SessionID: spec.SessionID, CredentialEpoch: 1}
	original := source.AdmissionRequest{Command: f.request.Command, Target: writefence.WriterExecution, ScopeID: spec.ScopeID, ProjectID: spec.ProjectID}
	if _, err := store.RegisterSourceCommand(t.Context(), id, original); err != nil {
		t.Fatal(err)
	}
	if _, err := pgfence.New(f.execution).Admit(t.Context(), original.Command); err != nil {
		t.Fatal(err)
	}
	state, err := store.BeginProjectClosure(t.Context(), spec)
	if err != nil || !state.GateClosed || state.Cycle != 1 {
		t.Fatalf("actual gate not committed: %+v %v", state, err)
	}
	fresh := original
	fresh.Command.ID = uuid.New()
	fresh.Command.SHA256 = strings.Repeat("d", 64)
	if _, err := store.RegisterSourceCommand(t.Context(), id, fresh); !errors.Is(err, source.ErrProjectClosureBusy) {
		t.Fatalf("new source command crossed gate: %v", err)
	}
	if _, err := store.RegisterSourceCommand(t.Context(), id, original); err != nil {
		t.Fatalf("old responsibility was cancelled by gate: %v", err)
	}
	exec := &closureOrderReader{business: source.ProjectWorkBusy, commandState: writefence.Admitted}
	media := &closureOrderReader{business: source.ProjectWorkIdle, commandState: writefence.Committed}
	if _, err := closurePGUseCase(t, f, store, exec, media).Resolve(t.Context(), spec.ID); !errors.Is(err, source.ErrProjectClosureBusy) {
		t.Fatalf("actual busy lost original409: %v", err)
	}
	assertClosureProjectEpoch(t, f, 1)
	if err := f.execution.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error { return pgfence.New(f.execution).Guard(t.Context(), tx, original.Command) }); err != nil {
		t.Fatalf("busy close fenced old effect command: %v", err)
	}
	if _, err := store.RegisterSourceCommand(t.Context(), id, fresh); err != nil {
		t.Fatalf("known busy permanently closed admission: %v", err)
	}
	retry, err := store.BeginProjectClosure(t.Context(), spec)
	if err != nil || retry.Cycle != 2 || retry.ManifestFor(writefence.WriterExecution) == state.ManifestFor(writefence.WriterExecution) {
		t.Fatalf("busy retry reused old manifest/cycle: %+v %v", retry, err)
	}
	if err := store.ReopenBusyProjectGate(t.Context(), state); err == nil {
		t.Fatal("old close cycle reopened current gate")
	}
}
func TestCreationProjectClosurePostgresUnknownKeepsOriginalGateAndHead(t *testing.T) {
	f, store, spec := projectClosurePGFixture(t)
	exec := &closureOrderReader{business: source.ProjectWorkUnknown}
	media := &closureOrderReader{business: source.ProjectWorkIdle}
	service := closurePGUseCase(t, f, store, exec, media)
	if _, err := service.Close(t.Context(), spec); !errors.Is(err, source.ErrProjectClosureUnknown) {
		t.Fatal(err)
	}
	state, err := store.ReadProjectClosure(t.Context(), spec.ID)
	if err != nil || !state.GateClosed || state.Cycle != 1 || state.AuthorityStarted {
		t.Fatalf("unknown was cleared or advanced: %+v %v", state, err)
	}
	assertClosureProjectEpoch(t, f, 1)
	if _, err := closurePGUseCase(t, f, store, exec, media).Resolve(t.Context(), spec.ID); !errors.Is(err, source.ErrProjectClosureUnknown) {
		t.Fatal(err)
	}
	rebuilt, err := store.ReadProjectClosure(t.Context(), spec.ID)
	if err != nil || rebuilt.Cycle != state.Cycle || rebuilt.ManifestFor(writefence.WriterExecution) != state.ManifestFor(writefence.WriterExecution) {
		t.Fatal("unknown changed original manifest", err)
	}
}
func TestCreationProjectClosurePostgresIdleFourWriterDecisionIsAtomic(t *testing.T) {
	f, store, spec := projectClosurePGFixture(t)
	idleExec := &closureOrderReader{business: source.ProjectWorkIdle}
	idleMedia := &closureOrderReader{business: source.ProjectWorkIdle}
	project, err := closurePGUseCase(t, f, store, idleExec, idleMedia).Close(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if project.ID != spec.ProjectID || project.Status != "archived" || project.Revision != 2 || project.ArchivedAt == nil {
		t.Fatalf("original lifecycle not retained: %+v", project)
	}
	assertClosureProjectEpoch(t, f, 2)
	record, err := pgfence.New(f.creation).Load(t.Context(), spec.Command.ID)
	if err != nil || record.State != writefence.Committed {
		t.Fatalf("final source actor/scope receipt missing: %+v %v", record, err)
	}
	var count int64
	if err := f.creation.Raw(`SELECT count(*) FROM infra.outbox WHERE id IN (?,?)`, uuid.NewSHA1(spec.ID, []byte("project.changed")), uuid.NewSHA1(spec.ID, []byte("project.lifecycle.audit"))).Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("original lifecycle events missing", count, err)
	}
	original, err := store.ReadProjectClosure(t.Context(), spec.ID)
	if err != nil || original.Result == nil || original.Result.Status != "archived" {
		t.Fatal("unique original close result missing", err)
	}
	if _, err := closurePGUseCase(t, f, store, idleExec, idleMedia).Resolve(t.Context(), spec.ID); err != nil {
		t.Fatal("original decision did not replay", err)
	}
}
