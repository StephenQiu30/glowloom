package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// ProjectClosureTask is the existing task runtime responsibility for one gate cycle.
const ProjectClosureTask = "creation.project.close"

// ProjectClosureSpec binds an administrative source control to the original
// authenticated account/session/scope. ProjectAuthority is separately bound:
// closing that authority must not invalidate this control's own final Guard.
type ProjectClosureSpec struct {
	ID                    uuid.UUID            `json:"id"`
	Command               writefence.Command   `json:"command"`
	ActorID               uuid.UUID            `json:"actor_id"`
	SessionID             uuid.UUID            `json:"session_id"`
	ScopeID               uuid.UUID            `json:"scope_id"`
	OrganizationID        uuid.UUID            `json:"organization_id"`
	ProjectID             uuid.UUID            `json:"project_id"`
	RequestID             uuid.UUID            `json:"request_id"`
	IdempotencyKey        uuid.UUID            `json:"idempotency_key"`
	ExpectedRevision      int64                `json:"expected_revision"`
	ProjectAuthority      writefence.Authority `json:"project_authority"`
	CoordinationID        uuid.UUID            `json:"coordination_id"`
	Action                string               `json:"action"`
	ExpectedCoverAssetID  *uuid.UUID           `json:"expected_cover_asset_id,omitempty"`
	ExpectedProjectStatus string               `json:"expected_project_status,omitempty"`
}

// CommandSHA256 binds the original resource, generation and administrative intent.
func (s ProjectClosureSpec) CommandSHA256() (string, error) {
	s.Command.SHA256 = ""
	raw, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	encoded, err := writefence.CanonicalReceipt(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte("glowloom.creation.project_close\x00"), encoded...))
	return hex.EncodeToString(sum[:]), nil
}

// Validate permits only the explicitly bound archive/delete source controls.
func (s ProjectClosureSpec) Validate() error {
	if s.ActorID == uuid.Nil || s.SessionID == uuid.Nil || s.ScopeID == uuid.Nil || s.OrganizationID == uuid.Nil || s.ProjectID == uuid.Nil || s.RequestID == uuid.Nil || s.IdempotencyKey == uuid.Nil || s.ExpectedRevision < 1 || s.ExpectedRevision >= 2147483647 || s.ProjectAuthority.Validate() != nil || s.ProjectAuthority.Issuer != writefence.Creation || s.ProjectAuthority.Kind != writefence.Project || s.ProjectAuthority.ID != s.ProjectID || s.ProjectAuthority.Epoch >= 9223372036854775806 || (s.Action != "archive" && s.Action != "delete") {
		return writefence.ErrInvalid
	}
	if s.CoordinationID != uuid.NewSHA1(s.ID, []byte("creation.project.close.authority")) || s.ID != uuid.NewSHA1(s.ActorID, []byte("creation.project.close/"+s.IdempotencyKey.String())) || s.Command.ID != uuid.NewSHA1(s.ID, []byte("creation.project.close.command")) || s.Command.Action != "creation.project."+s.Action || len(s.Command.Authorities) != 3 {
		return writefence.ErrInvalid
	}
	if _, err := s.Command.AuthorityDigest(); err != nil {
		return err
	}
	account, session, scope := false, false, false
	for _, a := range s.Command.Authorities {
		switch {
		case a.Issuer == writefence.Identity && a.Kind == writefence.Account && a.ID == s.ActorID:
			account = true
		case a.Issuer == writefence.Identity && a.Kind == writefence.Session && a.ID == s.SessionID:
			session = true
		case a.Issuer == writefence.Creation && a.Kind == writefence.Scope && a.ID == s.ScopeID:
			scope = true
		default:
			return writefence.ErrInvalid
		}
	}
	if !account || !session || !scope {
		return writefence.ErrInvalid
	}
	sum, err := s.CommandSHA256()
	if err != nil || sum != s.Command.SHA256 {
		return writefence.ErrInvalid
	}
	return nil
}

// Control returns the immutable project authority transition for this intent.
func (s ProjectClosureSpec) Control() writefence.Control {
	a := s.ProjectAuthority
	a.Epoch++
	return writefence.Control{Authority: a, CoordinationID: s.CoordinationID, Blocked: true}
}

// ReopenControl binds compensation to the original stored close coordination.
// It never resumes old fenced work; all original business was drained before close.
func (s ProjectClosureSpec) ReopenControl() writefence.Control {
	a := s.ProjectAuthority
	a.Epoch += 2
	return writefence.Control{Authority: a, CoordinationID: uuid.NewSHA1(s.CoordinationID, []byte("creation.project.close.reopen")), Blocked: s.ExpectedProjectStatus == domain.ProjectStatusArchived}
}

// ProjectClosureState is the durable source decision. A private gate does not
// change the public project state or writer epoch while responsibility is busy.
type ProjectClosureState struct {
	Spec             ProjectClosureSpec
	LedgerSHA256     string
	Cycle            int64
	GateClosed       bool
	AuthorityStarted bool
	Aborting         bool
	Aborted          bool
	Result           *domain.Project
	Details          *ProjectCloseDetails
}

// ManifestFor binds one target to the complete original ledger and gate cycle.
// A previous busy cycle cannot lend its IDLE observation to a new attempt.
func (s ProjectClosureState) ManifestFor(target writefence.Writer) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("glowloom.creation.project_closure.manifest\x00%s/%d/%s/%s/%s", s.Spec.ID, s.Cycle, target, s.LedgerSHA256, s.Spec.Command.SHA256)))
	return hex.EncodeToString(sum[:])
}

// ProjectClosureStore persists only Creation-owned gates, original registrations,
// authority control and the final same-transaction project/receipt decision.
type ProjectClosureStore interface {
	BeginProjectClosure(context.Context, ProjectClosureSpec) (ProjectClosureState, error)
	ReadProjectClosure(context.Context, uuid.UUID) (ProjectClosureState, error)
	ReadProjectClosureForTask(context.Context, taskruntime.Job) (ProjectClosureState, error)
	ReadProjectClosureCommands(context.Context, ProjectClosureState, writefence.Writer, uuid.UUID, int) ([]writefence.Command, error)
	ReopenBusyProjectGate(context.Context, ProjectClosureState) error
	BeginClosedProjectAuthority(context.Context, ProjectClosureState) (writefence.Control, error)
	CommitClosedProject(context.Context, ProjectClosureState, writefence.Control) (domain.Project, error)
	BeginProjectClosureAbort(context.Context, ProjectClosureState) (writefence.Control, bool, error)
	CompleteProjectClosureAbort(context.Context, ProjectClosureState, writefence.Control) error
}

// ProjectClosures coordinates source admission and the original owner responsibilities.
type ProjectClosures struct {
	store        ProjectClosureStore
	participants map[writefence.Writer]ProjectWorkReader
	writers      ScopeAuthorityInstaller
	covers       ProjectCoverReader
}

// NewProjectClosures requires every actual participant and the four-writer coordinator.
func NewProjectClosures(store ProjectClosureStore, participants map[writefence.Writer]ProjectWorkReader, writers ScopeAuthorityInstaller, covers ...ProjectCoverReader) (*ProjectClosures, error) {
	if store == nil || writers == nil || len(participants) != 3 {
		return nil, writefence.ErrInvalid
	}
	peers := make(map[writefence.Writer]ProjectWorkReader, 3)
	for _, target := range []writefence.Writer{writefence.WriterCreation, writefence.WriterExecution, writefence.WriterMedia} {
		if participants[target] == nil {
			return nil, writefence.ErrInvalid
		}
		peers[target] = participants[target]
	}
	var cover ProjectCoverReader
	if len(covers) > 1 {
		return nil, writefence.ErrInvalid
	}
	if len(covers) == 1 {
		if covers[0] == nil {
			return nil, writefence.ErrInvalid
		}
		cover = covers[0]
	}
	return &ProjectClosures{store: store, participants: peers, writers: writers, covers: cover}, nil
}

// Close accepts the source gate before performing any participant network read.
func (s *ProjectClosures) Close(ctx context.Context, spec ProjectClosureSpec) (domain.Project, error) {
	if s == nil || spec.Validate() != nil {
		return domain.Project{}, writefence.ErrInvalid
	}
	state, err := s.store.BeginProjectClosure(ctx, spec)
	if err != nil {
		if closureAccessFailed(err) {
			original, readErr := s.store.ReadProjectClosure(ctx, spec.ID)
			if readErr == nil && original.GateClosed {
				return s.abort(ctx, original)
			}
		}
		return domain.Project{}, err
	}
	return s.resume(ctx, state)
}

// Resolve resumes only the original durable close and never creates another ID.
func (s *ProjectClosures) Resolve(ctx context.Context, id uuid.UUID) (domain.Project, error) {
	if s == nil || id == uuid.Nil {
		return domain.Project{}, writefence.ErrInvalid
	}
	state, err := s.store.ReadProjectClosure(ctx, id)
	if err != nil {
		return domain.Project{}, err
	}
	return s.resume(ctx, state)
}
func (s *ProjectClosures) resume(ctx context.Context, state ProjectClosureState) (domain.Project, error) {
	if state.Aborted {
		return domain.Project{}, writefence.ErrFenced
	}
	if state.Aborting {
		return s.abort(ctx, state)
	}
	project, err := s.finish(ctx, state)
	if closureAccessFailed(err) {
		current, readErr := s.store.ReadProjectClosure(ctx, state.Spec.ID)
		if readErr == nil && current.GateClosed && current.Cycle == state.Cycle {
			return s.abort(ctx, current)
		}
	}
	return project, err
}
func closureAccessFailed(err error) bool {
	return errors.Is(err, writefence.ErrFenced) || errors.Is(err, writefence.ErrFrozen) || errors.Is(err, ErrScopeNotFound) || errors.Is(err, ErrProjectNotFound) || errors.Is(err, domain.ErrProjectRevisionConflict) || errors.Is(err, domain.ErrProjectStateConflict)
}
func (s *ProjectClosures) abort(ctx context.Context, state ProjectClosureState) (domain.Project, error) {
	control, reopen, err := s.store.BeginProjectClosureAbort(ctx, state)
	if err != nil {
		return domain.Project{}, err
	}
	if reopen {
		installed, err := s.writers.Set(ctx, state.Spec.Control().Authority, control.CoordinationID, control.Blocked)
		if err != nil {
			return domain.Project{}, err
		}
		if installed.Control != control || !installed.Complete || len(installed.Acknowledged) != len(writefence.Writers()) {
			return domain.Project{}, writefence.ErrConflict
		}
		seen := map[writefence.Writer]bool{}
		for _, writer := range installed.Acknowledged {
			if writer.Validate() != nil || seen[writer] {
				return domain.Project{}, writefence.ErrConflict
			}
			seen[writer] = true
		}
	}
	if err := s.store.CompleteProjectClosureAbort(ctx, state, control); err != nil {
		return domain.Project{}, err
	}
	return domain.Project{}, writefence.ErrFenced
}
func (s *ProjectClosures) finish(ctx context.Context, state ProjectClosureState) (domain.Project, error) {
	if state.Spec.Validate() != nil {
		return domain.Project{}, writefence.ErrConflict
	}
	if state.Result != nil {
		if state.Result.ID != state.Spec.ProjectID || state.Result.OrgID != state.Spec.OrganizationID {
			return domain.Project{}, writefence.ErrConflict
		}
		return *state.Result, nil
	}
	if !state.GateClosed || state.Cycle < 1 || !validPreparationHash(state.LedgerSHA256) {
		return domain.Project{}, writefence.ErrConflict
	}
	if !state.AuthorityStarted {
		busy, unknown, err := s.inspect(ctx, state)
		if busy {
			if reopenErr := s.store.ReopenBusyProjectGate(ctx, state); reopenErr != nil {
				return domain.Project{}, reopenErr
			}
			return domain.Project{}, ErrProjectClosureBusy
		}
		if err != nil {
			return domain.Project{}, err
		}
		if unknown {
			return domain.Project{}, ErrProjectClosureUnknown
		}
		control, err := s.store.BeginClosedProjectAuthority(ctx, state)
		if err != nil {
			return domain.Project{}, err
		}
		if control != state.Spec.Control() {
			return domain.Project{}, writefence.ErrConflict
		}
	}
	control := state.Spec.Control()
	installed, err := s.writers.Set(ctx, state.Spec.ProjectAuthority, control.CoordinationID, true)
	if err != nil {
		return domain.Project{}, err
	}
	if installed.Control != control || !installed.Complete || len(installed.Acknowledged) != len(writefence.Writers()) {
		return domain.Project{}, writefence.ErrConflict
	}
	seen := map[writefence.Writer]bool{}
	for _, writer := range installed.Acknowledged {
		if writer.Validate() != nil || seen[writer] {
			return domain.Project{}, writefence.ErrConflict
		}
		seen[writer] = true
	}
	if state.Spec.ExpectedCoverAssetID != nil {
		if s.covers == nil {
			return domain.Project{}, ErrProjectClosureUnknown
		}
		request := state.Spec.CoverRequest()
		observation, err := s.covers.ReadProjectCover(ctx, request)
		if err != nil {
			return domain.Project{}, err
		}
		if observation.Request != request {
			return domain.Project{}, writefence.ErrConflict
		}
		ctx = context.WithValue(ctx, projectClosureCoverContext{}, observation)
	}
	return s.store.CommitClosedProject(ctx, state, control)
}

func (s *ProjectClosures) inspect(ctx context.Context, state ProjectClosureState) (bool, bool, error) {
	unknown := false
	var observedError error
	for _, target := range []writefence.Writer{writefence.WriterCreation, writefence.WriterExecution, writefence.WriterMedia} {
		after := uuid.Nil
		for {
			commands, err := s.store.ReadProjectClosureCommands(ctx, state, target, after, ProjectWorkPageSize)
			if err != nil {
				return false, true, err
			}
			if len(commands) > ProjectWorkPageSize {
				return false, true, writefence.ErrConflict
			}
			for i, command := range commands {
				if (i == 0 && after != uuid.Nil && command.ID.String() <= after.String()) || (i > 0 && command.ID.String() <= commands[i-1].ID.String()) {
					return false, true, writefence.ErrConflict
				}
			}
			request := ProjectWorkRequest{CloseID: state.Spec.ID, ProjectID: state.Spec.ProjectID, OrganizationID: state.Spec.OrganizationID, ScopeID: state.Spec.ScopeID, ProjectEpoch: state.Spec.ProjectAuthority.Epoch, Commands: commands, ManifestSHA256: state.ManifestFor(target)}
			if err := request.Validate(); err != nil {
				return false, true, err
			}
			response, err := s.participants[target].ReadProjectWork(ctx, request)
			if err != nil {
				unknown = true
				observedError = errors.Join(observedError, projectWorkFailure(target, err))
			} else {
				work, err := response.Inspect(request)
				if err != nil {
					unknown = true
					observedError = errors.Join(observedError, projectWorkFailure(target, err))
				} else {
					switch work {
					case ProjectWorkBusy:
						return true, false, nil
					case ProjectWorkUnknown:
						unknown = true
					case ProjectWorkIdle:
					default:
						return false, true, writefence.ErrUnavailable
					}
				}
			}
			if len(commands) < ProjectWorkPageSize {
				break
			}
			after = commands[len(commands)-1].ID
		}
	}
	return false, unknown, observedError
}

type projectClosureJobContext struct{}

// ProjectClosureJob exposes the original lease to the local transaction adapter.
func ProjectClosureJob(ctx context.Context) (taskruntime.Job, bool) {
	job, ok := ctx.Value(projectClosureJobContext{}).(taskruntime.Job)
	return job, ok
}

// Handle recovers one original leased cycle through the existing task worker.
func (s *ProjectClosures) Handle(ctx context.Context, job taskruntime.Job) (timeNext time.Time, err error) {
	if s == nil || job.Kind != ProjectClosureTask {
		return time.Time{}, writefence.ErrInvalid
	}
	state, err := s.store.ReadProjectClosureForTask(ctx, job)
	if err != nil {
		return time.Time{}, err
	}
	_, err = s.resume(context.WithValue(ctx, projectClosureJobContext{}, job), state)
	if errors.Is(err, ErrProjectClosureBusy) {
		return time.Time{}, nil
	}
	if errors.Is(err, writefence.ErrFenced) {
		current, readErr := s.store.ReadProjectClosure(ctx, state.Spec.ID)
		if readErr == nil && current.Aborted && current.Cycle == state.Cycle {
			return time.Time{}, nil
		}
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("recover original project closure: %w", err)
	}
	return time.Time{}, nil
}
