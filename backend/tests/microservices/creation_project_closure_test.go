package microservices_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// These are business-order tests. Restricted role/row-lock/RPC acceptance must
// independently exercise the actual gate store and participants.
type closureOrderStore struct {
	state                         source.ProjectClosureState
	commands                      map[writefence.Writer][]writefence.Command
	reopened, advanced, committed int
}

func (s *closureOrderStore) BeginProjectClosure(context.Context, source.ProjectClosureSpec) (source.ProjectClosureState, error) {
	return s.state, nil
}
func (s *closureOrderStore) ReadProjectClosure(context.Context, uuid.UUID) (source.ProjectClosureState, error) {
	return s.state, nil
}
func (s *closureOrderStore) ReadProjectClosureForTask(context.Context, taskruntime.Job) (source.ProjectClosureState, error) {
	return s.state, nil
}
func (s *closureOrderStore) ReadProjectClosureCommands(_ context.Context, _ source.ProjectClosureState, target writefence.Writer, after uuid.UUID, limit int) ([]writefence.Command, error) {
	var out []writefence.Command
	for _, command := range s.commands[target] {
		if after == uuid.Nil || command.ID.String() > after.String() {
			out = append(out, command)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}
func (s *closureOrderStore) ReopenBusyProjectGate(context.Context, source.ProjectClosureState) error {
	s.reopened++
	s.state.GateClosed = false
	return nil
}
func (s *closureOrderStore) BeginClosedProjectAuthority(context.Context, source.ProjectClosureState) (writefence.Control, error) {
	s.advanced++
	s.state.AuthorityStarted = true
	return s.state.Spec.Control(), nil
}
func (s *closureOrderStore) CommitClosedProject(context.Context, source.ProjectClosureState, writefence.Control) (domain.Project, error) {
	s.committed++
	project := domain.Project{ID: s.state.Spec.ProjectID, OrgID: s.state.Spec.OrganizationID, Status: "archived"}
	s.state.Result = &project
	return project, nil
}

func (s *closureOrderStore) BeginProjectClosureAbort(context.Context, source.ProjectClosureState) (writefence.Control, bool, error) {
	s.state.Aborting = true
	return writefence.Control{}, false, nil
}
func (s *closureOrderStore) CompleteProjectClosureAbort(context.Context, source.ProjectClosureState, writefence.Control) error {
	s.state.Aborted = true
	s.state.Aborting = false
	s.state.GateClosed = false
	return nil
}

type closureOrderWriters struct {
	calls int
	id    uuid.UUID
}

func (w *closureOrderWriters) Set(_ context.Context, expected writefence.Authority, id uuid.UUID, blocked bool) (writefence.Coordination, error) {
	w.calls++
	w.id = id
	expected.Epoch++
	return writefence.Coordination{Control: writefence.Control{Authority: expected, CoordinationID: id, Blocked: blocked}, Complete: true, Acknowledged: writefence.Writers()}, nil
}

type closureOrderReader struct {
	business     source.ProjectWorkState
	commandState writefence.State
	err          error
	tamper       bool
	calls        int
}

func (r *closureOrderReader) ReadProjectWork(_ context.Context, request source.ProjectWorkRequest) (source.ProjectWorkSnapshot, error) {
	r.calls++
	if r.err != nil {
		return source.ProjectWorkSnapshot{}, r.err
	}
	response := source.ProjectWorkSnapshot{Request: request, BusinessState: r.business}
	for _, command := range request.Commands {
		observation := source.ProjectCommandObservation{Command: command, State: r.commandState}
		if r.commandState == writefence.Committed {
			observation.ReceiptSHA256 = strings.Repeat("b", 64)
		}
		response.Commands = append(response.Commands, observation)
	}
	if r.tamper {
		response.Request.ProjectID = uuid.New()
	}
	return response, nil
}
func closureOrderFixture(t *testing.T) (*closureOrderStore, *closureOrderWriters, map[writefence.Writer]*closureOrderReader) {
	t.Helper()
	actor, session, scope, project, key := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	spec := source.ProjectClosureSpec{ActorID: actor, SessionID: session, ScopeID: scope, OrganizationID: uuid.New(), ProjectID: project, RequestID: uuid.New(), IdempotencyKey: key, ExpectedRevision: 17, ProjectAuthority: writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Project, ID: project, Epoch: 4}, Action: "archive"}
	spec.ID = uuid.NewSHA1(actor, []byte("creation.project.close/"+key.String()))
	spec.CoordinationID = uuid.NewSHA1(spec.ID, []byte("creation.project.close.authority"))
	spec.Command = writefence.Command{ID: uuid.NewSHA1(spec.ID, []byte("creation.project.close.command")), Action: "creation.project.archive", Authorities: []writefence.Authority{{Issuer: writefence.Identity, Kind: writefence.Account, ID: actor, Epoch: 2}, {Issuer: writefence.Identity, Kind: writefence.Session, ID: session, Epoch: 3}, {Issuer: writefence.Creation, Kind: writefence.Scope, ID: scope, Epoch: 7}}}
	var err error
	spec.Command.SHA256, err = spec.CommandSHA256()
	if err != nil || spec.Validate() != nil {
		t.Fatalf("invalid closure fixture: %v", err)
	}
	store := &closureOrderStore{state: source.ProjectClosureState{Spec: spec, LedgerSHA256: strings.Repeat("a", 64), Cycle: 1, GateClosed: true}, commands: map[writefence.Writer][]writefence.Command{}}
	readers := map[writefence.Writer]*closureOrderReader{}
	for _, target := range []writefence.Writer{writefence.WriterCreation, writefence.WriterExecution, writefence.WriterMedia} {
		readers[target] = &closureOrderReader{business: source.ProjectWorkIdle, commandState: writefence.Committed}
	}
	return store, &closureOrderWriters{}, readers
}
func closureOrderUseCase(t *testing.T, store *closureOrderStore, writers *closureOrderWriters, readers map[writefence.Writer]*closureOrderReader) *source.ProjectClosures {
	t.Helper()
	ports := map[writefence.Writer]source.ProjectWorkReader{}
	for target, reader := range readers {
		ports[target] = reader
	}
	useCase, err := source.NewProjectClosures(store, ports, writers)
	if err != nil {
		t.Fatal(err)
	}
	return useCase
}
func closureRegisteredCommand(store *closureOrderStore) writefence.Command {
	authorities := append([]writefence.Authority(nil), store.state.Spec.Command.Authorities...)
	authorities = append(authorities, store.state.Spec.ProjectAuthority)
	return writefence.Command{ID: uuid.New(), SHA256: strings.Repeat("c", 64), Action: "media.reference_pin.acquire", Authorities: authorities}
}

func TestCreationProjectClosureRegisteredAdmittedWorkIsBusyWithoutAdvancingEpoch(t *testing.T) {
	store, writers, readers := closureOrderFixture(t)
	store.commands[writefence.WriterMedia] = []writefence.Command{closureRegisteredCommand(store)}
	readers[writefence.WriterMedia].commandState = writefence.Admitted
	_, err := closureOrderUseCase(t, store, writers, readers).Close(t.Context(), store.state.Spec)
	if !errors.Is(err, source.ErrProjectClosureBusy) || store.reopened != 1 || store.state.GateClosed || store.advanced != 0 || writers.calls != 0 || store.committed != 0 {
		t.Fatalf("busy responsibility changed original writer generation: %v %+v", err, store)
	}
}
func TestCreationProjectClosureUnknownKeepsOriginalGateAndRebuildUsesOriginalControl(t *testing.T) {
	store, writers, readers := closureOrderFixture(t)
	readers[writefence.WriterExecution].business = source.ProjectWorkUnknown
	_, err := closureOrderUseCase(t, store, writers, readers).Close(t.Context(), store.state.Spec)
	if !errors.Is(err, source.ErrProjectClosureUnknown) || !store.state.GateClosed || store.reopened != 0 || writers.calls != 0 || store.advanced != 0 {
		t.Fatalf("unknown became idle or cancelled old work: %v", err)
	}
	original := store.state.Spec.ID
	readers[writefence.WriterExecution].business = source.ProjectWorkIdle
	project, err := closureOrderUseCase(t, store, writers, readers).Resolve(t.Context(), original)
	if err != nil || project.ID != store.state.Spec.ProjectID || store.advanced != 1 || writers.calls != 1 || writers.id != store.state.Spec.Control().CoordinationID || store.committed != 1 {
		t.Fatalf("rebuild did not preserve original closure/control: %v", err)
	}
}
func TestCreationProjectClosureMissingRegisteredCommandCannotBeIdle(t *testing.T) {
	store, writers, readers := closureOrderFixture(t)
	command := closureRegisteredCommand(store)
	command.Action = "execution.frozen_input.accept"
	store.commands[writefence.WriterExecution] = []writefence.Command{command}
	readers[writefence.WriterExecution].commandState = writefence.State("unknown")
	_, err := closureOrderUseCase(t, store, writers, readers).Close(t.Context(), store.state.Spec)
	if !errors.Is(err, source.ErrProjectClosureUnknown) || writers.calls != 0 || store.advanced != 0 || !store.state.GateClosed {
		t.Fatalf("not-found responsibility became idle: %v", err)
	}
}
func TestCreationProjectClosureKnownBusyReopensGateEvenWhenAnotherParticipantUnknown(t *testing.T) {
	store, writers, readers := closureOrderFixture(t)
	readers[writefence.WriterExecution].err = errors.New("synthetic unavailable")
	readers[writefence.WriterMedia].business = source.ProjectWorkBusy
	_, err := closureOrderUseCase(t, store, writers, readers).Close(t.Context(), store.state.Spec)
	if !errors.Is(err, source.ErrProjectClosureBusy) || store.reopened != 1 || writers.calls != 0 || store.advanced != 0 {
		t.Fatalf("known busy permanently closed the source gate: %v", err)
	}
}
func TestCreationProjectClosureRetargetedResponseCannotAuthorizeClose(t *testing.T) {
	store, writers, readers := closureOrderFixture(t)
	readers[writefence.WriterMedia].tamper = true
	_, err := closureOrderUseCase(t, store, writers, readers).Close(t.Context(), store.state.Spec)
	if !errors.Is(err, source.ErrProjectClosureUnknown) || writers.calls != 0 || store.advanced != 0 || !store.state.GateClosed {
		t.Fatalf("retargeted response authorized close: %v", err)
	}
}
