package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

var (
	// ErrProjectPreparationUnknown retains the original unresolved responsibility.
	ErrProjectPreparationUnknown = errors.New("project preparation unresolved")
	// ErrProjectPreparationConflict forbids changing an accepted resource or input.
	ErrProjectPreparationConflict = errors.New("project preparation conflict")
)

// ProjectPreparationTask recovers only accepted Creation project preparations.
const ProjectPreparationTask = "creation.project.prepare"

// ProjectPreparationSpec is the original admitted local create command. Its three
// authorities are account, session and scope; a new project's head does not exist
// until the separate zero-epoch initialization has completed at every writer.
type ProjectPreparationSpec struct {
	PreparationID  uuid.UUID          `json:"preparation_id"`
	Command        writefence.Command `json:"command"`
	ActorID        uuid.UUID          `json:"actor_id"`
	SessionID      uuid.UUID          `json:"session_id"`
	IdempotencyKey uuid.UUID          `json:"idempotency_key"`
	RequestID      uuid.UUID          `json:"request_id"`
	ScopeID        uuid.UUID          `json:"scope_id"`
	ScopeRevision  int64              `json:"scope_revision"`
	InputSHA256    string             `json:"input_sha256"`
	Project        domain.Project     `json:"project"`
}

func validPreparationHash(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && hex.EncodeToString(raw) == value
}

// Validate rejects public caller identity, unbound resources and extra authorities.
func (s ProjectPreparationSpec) Validate() error {
	if s.PreparationID == uuid.Nil || s.ActorID == uuid.Nil || s.SessionID == uuid.Nil || s.IdempotencyKey == uuid.Nil || s.RequestID == uuid.Nil || s.ScopeID == uuid.Nil || s.ScopeRevision < 1 || !validPreparationHash(s.InputSHA256) || s.Project.Validate() != nil || s.Project.OwnerID != s.ActorID || s.Project.CanvasID == uuid.Nil || s.Project.Status != domain.ProjectStatusCopying || s.Project.Revision != 1 || s.Project.IndependentMedia || s.Project.AllowOverseasModels || s.Project.IsDelete || s.Project.ArchivedAt != nil || s.Project.DeleteTime != nil || s.Project.PurgeAfter != nil || !s.Project.CreateTime.IsZero() || !s.Project.UpdateTime.IsZero() || s.Project.CoverAssetID != nil {
		return ErrProjectPreparationConflict
	}
	if s.PreparationID != uuid.NewSHA1(s.ActorID, []byte("creation.project.preparation/"+s.IdempotencyKey.String())) || s.Project.ID != uuid.NewSHA1(s.PreparationID, []byte("creation.project")) || s.Project.CanvasID != uuid.NewSHA1(s.PreparationID, []byte("creation.canvas")) || s.Command.ID != uuid.NewSHA1(s.PreparationID, []byte("creation.project.create")) {
		return ErrProjectPreparationConflict
	}
	action := "creation.project.create"
	if s.Project.IndependentCanvas {
		action = "creation.independent_canvas.create"
	}
	if s.Command.Action != action || len(s.Command.Authorities) != 3 {
		return ErrProjectPreparationConflict
	}
	if _, err := s.Command.AuthorityDigest(); err != nil {
		return err
	}
	expected := map[string]uuid.UUID{"identity/account": s.ActorID, "identity/session": s.SessionID, "creation/scope": s.ScopeID}
	for _, authority := range s.Command.Authorities {
		if expected[string(authority.Issuer)+"/"+string(authority.Kind)] != authority.ID {
			return ErrProjectPreparationConflict
		}
	}
	return nil
}

// CommandSHA256 binds the original fixed resources, raw request fingerprint,
// revisions and complete authority generations without recursively hashing itself.
func (s ProjectPreparationSpec) CommandSHA256() (string, error) {
	s.Command.SHA256 = ""
	raw, err := json.Marshal(s)
	if err != nil {
		return "", ErrProjectPreparationConflict
	}
	hash := sha256.Sum256(append([]byte("glowloom.creation.project.prepare\x00"), raw...))
	return hex.EncodeToString(hash[:]), nil
}

// ProjectBudgetPreparation is Creation's minimum participant request. Its exact
// shape is encoded through the generated Execution contract by the outer adapter.
type ProjectBudgetPreparation struct {
	ExecutionCommandID uuid.UUID `json:"execution_command_id"`
	PreparationID      uuid.UUID `json:"preparation_id"`
	SourceCommandID    uuid.UUID `json:"source_command_id"`
	SourceSHA256       string    `json:"source_sha256"`
	InputSHA256        string    `json:"input_sha256"`
	ProjectID          uuid.UUID `json:"project_id"`
	OrganizationID     uuid.UUID `json:"organization_id"`
	ScopeID            uuid.UUID `json:"scope_id"`
}

// BudgetRequest retains the exact original resource/preparation identities.
func (s ProjectPreparationSpec) BudgetRequest() ProjectBudgetPreparation {
	return ProjectBudgetPreparation{ExecutionCommandID: uuid.NewSHA1(s.PreparationID, []byte("execution.project_budget.prepare")), PreparationID: s.PreparationID, SourceCommandID: s.Command.ID, SourceSHA256: s.Command.SHA256, InputSHA256: s.InputSHA256, ProjectID: s.Project.ID, OrganizationID: s.Project.OrgID, ScopeID: s.ScopeID}
}

// Validate checks the immutable responsibility key without granting any rights.
func (p ProjectBudgetPreparation) Validate() error {
	if p.PreparationID == uuid.Nil || p.ExecutionCommandID != uuid.NewSHA1(p.PreparationID, []byte("execution.project_budget.prepare")) || p.SourceCommandID == uuid.Nil || p.SourceCommandID == p.ExecutionCommandID || p.ProjectID == uuid.Nil || p.OrganizationID == uuid.Nil || p.ScopeID == uuid.Nil || !validPreparationHash(p.SourceSHA256) || !validPreparationHash(p.InputSHA256) {
		return ErrProjectPreparationConflict
	}
	return nil
}

// PreparedProjectBudget is Execution's immutable zero-credit opening ACK.
type PreparedProjectBudget struct {
	Request        ProjectBudgetPreparation `json:"request"`
	BudgetID       uuid.UUID                `json:"budget_id"`
	BudgetRevision int64                    `json:"budget_revision"`
	LimitMicros    int64                    `json:"limit_micros"`
	SpentMicros    int64                    `json:"spent_micros"`
	ReservedMicros int64                    `json:"reserved_micros"`
	WalletPrepared bool                     `json:"wallet_prepared"`
}

// Validate accepts the exact initial uncapped sentinel and no spending grant.
func (r PreparedProjectBudget) Validate() error {
	if r.Request.Validate() != nil || r.BudgetID != uuid.NewSHA1(r.Request.PreparationID, []byte("billing.project_budget")) || r.BudgetRevision != 1 || r.LimitMicros != 9007199254740991 || r.SpentMicros != 0 || r.ReservedMicros != 0 || !r.WalletPrepared {
		return ErrProjectPreparationConflict
	}
	return nil
}

// ProjectPreparation retains accepted facts and monotonic decisions. A prepared
// object is private; only Activation is a successful public create response.
type ProjectPreparation struct {
	Spec       ProjectPreparationSpec `json:"spec"`
	Budget     *PreparedProjectBudget `json:"budget,omitempty"`
	Activation *CreatedProject        `json:"activation,omitempty"`
}

// InitialProjectControl cannot reopen an old head or choose a replacement ID.
func InitialProjectControl(spec ProjectPreparationSpec) writefence.Control {
	return writefence.Control{Authority: writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Project, ID: spec.Project.ID, Epoch: 1}, CoordinationID: uuid.NewSHA1(spec.PreparationID, []byte("creation.project.initial")), Blocked: false}
}

// ProjectPreparationRepository owns the local policy/fence/task/receipt transactions.
// A task lease, when present, is checked in every mutating recovery transaction.
type ProjectPreparationRepository interface {
	PrepareProject(context.Context, ProjectPreparationSpec) (ProjectPreparation, error)
	ReadProjectPreparation(context.Context, ProjectBudgetPreparation) (ProjectPreparation, error)
	ReadProjectPreparationForTask(context.Context, taskruntime.Job) (ProjectPreparation, error)
	SaveProjectBudget(context.Context, ProjectBudgetPreparation, PreparedProjectBudget, *taskruntime.Job) (ProjectPreparation, error)
	BeginProjectAuthority(context.Context, ProjectBudgetPreparation, *taskruntime.Job) (writefence.Control, error)
	ActivatePreparedProject(context.Context, ProjectBudgetPreparation, writefence.Control, *taskruntime.Job) (CreatedProject, error)
}

// ProjectBudgetParticipant resolves/creates only the original zero-credit preparation.
type ProjectBudgetParticipant interface {
	ResolveProjectBudget(context.Context, ProjectBudgetPreparation) (PreparedProjectBudget, error)
	PrepareProjectBudget(context.Context, ProjectBudgetPreparation) (PreparedProjectBudget, error)
}

// ProjectPreparations drives the existing task worker and foreground retry with
// the same original IDs. Network calls never run under a local SQL transaction.
type ProjectPreparations struct {
	store   ProjectPreparationRepository
	budgets ProjectBudgetParticipant
	writers ScopeAuthorityInstaller
}

// NewProjectPreparations refuses missing durable source or real participants.
func NewProjectPreparations(store ProjectPreparationRepository, budgets ProjectBudgetParticipant, writers ScopeAuthorityInstaller) (*ProjectPreparations, error) {
	if store == nil || budgets == nil || writers == nil {
		return nil, ErrProjectPreparationConflict
	}
	return &ProjectPreparations{store, budgets, writers}, nil
}

// Prepare persists hidden local resources and recovery work before any remote call.
// Its caller must first admit this exact command through Identity and Creation.
func (p *ProjectPreparations) Prepare(ctx context.Context, spec ProjectPreparationSpec) (CreatedProject, error) {
	if p == nil || spec.Validate() != nil {
		return CreatedProject{}, ErrProjectPreparationConflict
	}
	state, err := p.store.PrepareProject(ctx, spec)
	if err != nil {
		return CreatedProject{}, err
	}
	return p.resume(ctx, state, nil)
}

// Handle uses the original accepted task's Owner/Epoch and no invented session.
func (p *ProjectPreparations) Handle(ctx context.Context, job taskruntime.Job) (time.Time, error) {
	if p == nil {
		return time.Time{}, ErrProjectPreparationConflict
	}
	state, err := p.store.ReadProjectPreparationForTask(ctx, job)
	if err != nil {
		return time.Time{}, err
	}
	_, err = p.resume(ctx, state, &job)
	return time.Time{}, err
}

func (p *ProjectPreparations) resume(ctx context.Context, state ProjectPreparation, job *taskruntime.Job) (CreatedProject, error) {
	if state.Spec.Validate() != nil {
		return CreatedProject{}, ErrProjectPreparationConflict
	}
	if state.Activation != nil {
		return *state.Activation, nil
	}
	key := state.Spec.BudgetRequest()
	if state.Budget == nil {
		budget, err := p.budgets.ResolveProjectBudget(ctx, key)
		if errors.Is(err, ErrProjectPreparationUnknown) {
			budget, err = p.budgets.PrepareProjectBudget(ctx, key)
		}
		if err != nil {
			return CreatedProject{}, err
		}
		if budget.Request != key || budget.Validate() != nil {
			return CreatedProject{}, ErrProjectPreparationConflict
		}
		state, err = p.store.SaveProjectBudget(ctx, key, budget, job)
		if err != nil {
			return CreatedProject{}, err
		}
	}
	control, err := p.store.BeginProjectAuthority(ctx, key, job)
	if err != nil {
		return CreatedProject{}, err
	}
	if control != InitialProjectControl(state.Spec) {
		return CreatedProject{}, ErrProjectPreparationConflict
	}
	expected := control.Authority
	expected.Epoch = 0
	installed, err := p.writers.Set(ctx, expected, control.CoordinationID, false)
	if err != nil {
		return CreatedProject{}, err
	}
	if installed.Control != control || !installed.Complete || len(installed.Acknowledged) != len(writefence.Writers()) {
		return CreatedProject{}, ErrProjectPreparationConflict
	}
	seen := map[writefence.Writer]bool{}
	for _, writer := range installed.Acknowledged {
		if writer.Validate() != nil || seen[writer] {
			return CreatedProject{}, ErrProjectPreparationConflict
		}
		seen[writer] = true
	}
	return p.store.ActivatePreparedProject(ctx, key, control, job)
}
