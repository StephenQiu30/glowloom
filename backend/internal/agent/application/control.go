package application

import (
	"context"
	"strings"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/google/uuid"
)

// StartRunInput contains choices, not model capabilities or runtime authority.
type StartRunInput struct {
	RequestID            uuid.UUID          `json:"request_id"`
	ExpectedRevision     int64              `json:"expected_revision"`
	Goal                 string             `json:"goal"`
	Model                domain.ModelChoice `json:"model"`
	Inputs               []domain.InputRef  `json:"inputs"`
	Skills               []domain.SkillPin  `json:"skills"`
	PlanningBudgetMicros int64              `json:"planning_budget_micros"`
	RootBudgetMicros     int64              `json:"root_budget_micros"`
}

// Preflight resolves current project authority and immutable source/model pins without model calls.
type Preflight interface {
	Freeze(context.Context, identity.Principal, uuid.UUID, StartRunInput) (domain.GoalManifest, error)
}

// ControlRequest identifies one user command and its optimistic Run revision.
type ControlRequest struct {
	RequestID        uuid.UUID `json:"request_id"`
	ExpectedRevision int64     `json:"expected_revision"`
	domain.ControlInput
}

// RunReceipt is an admission fact; status is never a model's completion claim.
type RunReceipt struct {
	RunID           uuid.UUID `json:"run_id"`
	CommandID       uuid.UUID `json:"command_id"`
	Revision        int64     `json:"revision"`
	ControlRevision int64     `json:"control_revision"`
	Status          string    `json:"status"`
	CandidateID     uuid.UUID `json:"candidate_id"`
}

// RuntimeDelivery is persisted before transport, and carries both sides of a fenced handoff.
type RuntimeDelivery struct {
	Kind     string                  `json:"kind"`
	Previous *domain.RuntimeIdentity `json:"previous,omitempty"`
	Identity domain.RuntimeIdentity  `json:"identity"`
	Model    domain.DialogueModel    `json:"model"`
	Command  RuntimeInput            `json:"command"`
}
type RuntimeInput struct {
	RequestID uuid.UUID `json:"request_id"`
	Message   string    `json:"message"`
}

// Change is executed on a locked Run, without waiting for a Host or model.
type Change func(*domain.Run) (RuntimeDelivery, error)

// ControlStore owns atomic ledger/replay/authority transitions, not SDK scheduling.
type ControlStore interface {
	CreateSession(context.Context, identity.Principal, uuid.UUID, uuid.UUID, string) (domain.Session, error)
	FindStart(context.Context, identity.Principal, uuid.UUID, uuid.UUID, uuid.UUID, string) (RunReceipt, bool, error)
	CreateRun(context.Context, identity.Principal, domain.Run, int64, string) (RunReceipt, error)
	ReadRun(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (domain.Run, error)
	ChangeRun(context.Context, identity.Principal, uuid.UUID, uuid.UUID, ControlRequest, string, Change) (RunReceipt, error)
	RegisterPlan(context.Context, domain.RuntimeIdentity, uuid.UUID, domain.PlanManifest, time.Time) (domain.ApprovalCandidate, error)
	Authorize(context.Context, domain.RuntimeIdentity, string, time.Time) error
	Admit(context.Context, domain.RuntimeIdentity, uuid.UUID, domain.ActionRequest, time.Time) (AdmissionReceipt, error)
	ClaimDelivery(context.Context, time.Time) (DeliveryClaim, bool, error)
	CompleteDelivery(context.Context, uuid.UUID, uuid.UUID) error
	RetryDelivery(context.Context, uuid.UUID, uuid.UUID, time.Time) error
}

// DeliveryClaim is a short database lease; Host I/O always happens after the lease transaction commits.
type DeliveryClaim struct {
	ID     uuid.UUID
	Token  uuid.UUID
	RunID  uuid.UUID
	Attempt int
	Value  RuntimeDelivery
}

// AdmissionReceipt distinguishes a new Go reservation from an existing in-flight operation.
type AdmissionReceipt struct {
	ID              uuid.UUID `json:"id"`
	RunID           uuid.UUID `json:"run_id"`
	Status          string    `json:"status"`
	ControlRevision int64     `json:"control_revision"`
	MaxMicros       int64     `json:"max_micros"`
	Replayed        bool      `json:"replayed"`
}

// Controller validates user commands and selects deterministic transitions.
type Controller struct {
	store     ControlStore
	preflight Preflight
	now       func() time.Time
}

func NewController(store ControlStore, preflight Preflight, now func() time.Time) *Controller {
	return &Controller{store: store, preflight: preflight, now: now}
}
func (c *Controller) CreateSession(ctx context.Context, actor identity.Principal, pid, request uuid.UUID, title string) (domain.Session, error) {
	if request == uuid.Nil || !ValidTitle(title) {
		return domain.Session{}, domain.ErrInvalid
	}
	return c.store.CreateSession(ctx, actor, pid, request, title)
}
func (c *Controller) StartRun(ctx context.Context, actor identity.Principal, pid, sid uuid.UUID, input StartRunInput) (RunReceipt, error) {
	if input.RequestID == uuid.Nil || input.ExpectedRevision < 1 || strings.TrimSpace(input.Goal) == "" || len(input.Goal) > 256*1024 || input.Model.Key == "" || input.Model.Mode != "text_to_text" {
		return RunReceipt{}, domain.ErrInvalid
	}
	canonical := input
	canonical.ExpectedRevision = 0
	hash, err := domain.CanonicalHash(canonical)
	if err != nil {
		return RunReceipt{}, err
	}
	if receipt, found, err := c.store.FindStart(ctx, actor, pid, sid, input.RequestID, hash); err != nil || found {
		return receipt, err
	}
	if c.preflight == nil {
		return RunReceipt{}, domain.ErrDependency
	}
	goal, err := c.preflight.Freeze(ctx, actor, pid, input)
	if err != nil {
		return RunReceipt{}, err
	}
	goal.Schema = "glowloom.goal/v1"
	if err := goal.Validate(); err != nil {
		return RunReceipt{}, err
	}
	run := domain.NewRun(sid, pid, actor.ID, actor.OrgID, actor.ScopeID, input.RequestID, goal, c.now())
	return c.store.CreateRun(ctx, actor, run, input.ExpectedRevision, hash)
}
func (c *Controller) ReadRun(ctx context.Context, actor identity.Principal, pid, rid uuid.UUID) (domain.Run, error) {
	return c.store.ReadRun(ctx, actor, pid, rid)
}
func (c *Controller) ApplyCommand(ctx context.Context, actor identity.Principal, pid, rid uuid.UUID, input ControlRequest) (RunReceipt, error) {
	if input.RequestID == uuid.Nil || input.ExpectedRevision < 1 {
		return RunReceipt{}, domain.ErrInvalid
	}
	normalized := input
	normalized.ExpectedRevision = 0
	hash, err := domain.CanonicalHash(normalized)
	if err != nil {
		return RunReceipt{}, err
	}
	return c.store.ChangeRun(ctx, actor, pid, rid, input, hash, func(run *domain.Run) (RuntimeDelivery, error) {
		old := run.RuntimeIdentity()
		if err := run.Apply(input.ControlInput, c.now()); err != nil {
			return RuntimeDelivery{}, err
		}
		delivery := RuntimeDelivery{Kind: "none", Identity: run.RuntimeIdentity()}
		switch input.Action {
		case "approve", "resume":
			delivery.Kind = "submit"
			delivery.Previous = &old
			delivery.Model = run.Goal.Model
			delivery.Command = RuntimeInput{RequestID: uuid.NewSHA1(input.RequestID, []byte("input/v1")), Message: run.Goal.Goal}
			if run.Candidate.Purpose == "execution" {
				delivery.Command.Message = "按已批准的执行计划推进。计划：" + string(run.Candidate.Manifest)
			}
		case "pause", "stop", "reject", "plan.revise", "steer":
			delivery.Kind = "drain"
			delivery.Previous = &old
		}
		return delivery, nil
	})
}
func (c *Controller) RegisterPlanCandidate(ctx context.Context, id domain.RuntimeIdentity, request uuid.UUID, plan domain.PlanManifest) (domain.ApprovalCandidate, error) {
	if request == uuid.Nil {
		return domain.ApprovalCandidate{}, domain.ErrInvalid
	}
	return c.store.RegisterPlan(ctx, id, request, plan, c.now())
}
func (c *Controller) AuthorizeRuntime(ctx context.Context, id domain.RuntimeIdentity, action string) error {
	return c.store.Authorize(ctx, id, action, c.now())
}

// AdmitRuntimeAction is called by trusted Go business adapters after pricing; it is not a Host price API.
func (c *Controller) AdmitRuntimeAction(ctx context.Context, id domain.RuntimeIdentity, request uuid.UUID, action domain.ActionRequest) (AdmissionReceipt, error) {
	if request == uuid.Nil {
		return AdmissionReceipt{}, domain.ErrInvalid
	}
	return c.store.Admit(ctx, id, request, action, c.now())
}

// ControlAccess resolves request and durable worker permissions inside the same transaction.
type ControlAccess interface {
	AuthorizeRequest(context.Context, identity.Principal, uuid.UUID, bool) error
	AuthorizeWorker(context.Context, identity.Principal, uuid.UUID) error
}
