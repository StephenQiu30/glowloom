package domain

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrFenced           = errors.New("agent_runtime_fenced")
	ErrApprovalRequired = errors.New("agent_approval_required")
	ErrRunClosed        = errors.New("agent_run_closed")
	ErrPermission       = errors.New("agent_action_forbidden")
	ErrBudget           = errors.New("agent_budget_exceeded")
	ErrDependency       = errors.New("agent_dependency_unavailable")
)

// RuntimeIdentity binds each call to current business authority, not model arguments.
type RuntimeIdentity struct {
	RuntimeSessionID uuid.UUID       `json:"runtime_session_id"`
	RunID            uuid.UUID       `json:"run_id"`
	ProjectID        uuid.UUID       `json:"project_id"`
	ActorID          uuid.UUID       `json:"actor_id"`
	OwnerEpoch       int64           `json:"owner_epoch"`
	ControlRevision  int64           `json:"control_revision"`
	PolicyPin        string          `json:"policy_pin"`
	DefinitionPin    string          `json:"definition_pin"`
	ApprovalContext  ApprovalContext `json:"approval_context"`
}
type ApprovalContext struct {
	Purpose      string    `json:"purpose"`
	CandidateID  uuid.UUID `json:"candidate_id"`
	ManifestHash string    `json:"manifest_hash"`
	StepID       string    `json:"step_id,omitempty"`
}

// Run is the Go authorization aggregate; it holds no executable SDK checkpoints.
type Run struct {
	ID                      uuid.UUID         `json:"id"`
	SessionID               uuid.UUID         `json:"session_id"`
	RuntimeSessionID        uuid.UUID         `json:"runtime_session_id"`
	ProjectID               uuid.UUID         `json:"project_id"`
	ActorID                 uuid.UUID         `json:"-"`
	OrgID                   uuid.UUID         `json:"-"`
	ScopeID                 uuid.UUID         `json:"-"`
	RequestID               uuid.UUID         `json:"request_id"`
	Revision                int64             `json:"revision"`
	ControlRevision         int64             `json:"control_revision"`
	OwnerEpoch              int64             `json:"owner_epoch"`
	Status                  string            `json:"status"`
	ResumeStatus            string            `json:"-"`
	Goal                    GoalManifest      `json:"goal"`
	GoalHash                string            `json:"goal_hash"`
	Candidate               ApprovalCandidate `json:"candidate"`
	PlanRevision            int64             `json:"plan_revision"`
	Approved                bool              `json:"approved"`
	ApprovalExpiresAt       time.Time         `json:"approval_expires_at"`
	ApprovalMaxMicros       int64             `json:"approval_max_micros"`
	CandidateReservedMicros int64             `json:"candidate_reserved_micros"`
	ReservedMicros          int64             `json:"reserved_micros"`
	CreatedAt               time.Time         `json:"created_at"`
}

func NewRun(session, project, actor, org, scope, request uuid.UUID, goal GoalManifest, now time.Time) Run {
	goal.Schema = "glowloom.goal/v1"
	hash, _ := CanonicalHash(goal)
	return Run{ID: uuid.New(), SessionID: session, RuntimeSessionID: uuid.New(), ProjectID: project, ActorID: actor, OrgID: org, ScopeID: scope, RequestID: request, Revision: 1, ControlRevision: 1, OwnerEpoch: 1, Status: "awaiting_planning_approval", Goal: goal, GoalHash: hash, Candidate: planningCandidate(goal), CreatedAt: now.UTC()}
}
func (r Run) RuntimeIdentity() RuntimeIdentity {
	return RuntimeIdentity{RuntimeSessionID: r.RuntimeSessionID, RunID: r.ID, ProjectID: r.ProjectID, ActorID: r.ActorID, OwnerEpoch: r.OwnerEpoch, ControlRevision: r.ControlRevision, PolicyPin: "creator/v1", DefinitionPin: "coordinator/v1", ApprovalContext: ApprovalContext{Purpose: r.Candidate.Purpose, CandidateID: r.Candidate.ID, ManifestHash: r.Candidate.Hash}}
}
func (r Run) Authorize(identity RuntimeIdentity, action string, now time.Time) error {
	expected := r.RuntimeIdentity()
	if identity.RuntimeSessionID != expected.RuntimeSessionID || identity.RunID != r.ID || identity.ProjectID != r.ProjectID || identity.ActorID != r.ActorID || identity.OwnerEpoch != r.OwnerEpoch || identity.PolicyPin != expected.PolicyPin || identity.DefinitionPin != expected.DefinitionPin {
		return ErrFenced
	}
	if action == "owner.drain" {
		// A signed cleanup may target an older runtime revision, but never a future
		// revision that could close a newer Host owner after a pause/resume race.
		if identity.ControlRevision < 1 || identity.ControlRevision > r.ControlRevision {
			return ErrFenced
		}
		return nil
	}
	if slices.Contains([]string{"session.snapshot", "receipt.read"}, action) {
		return nil
	}
	if identity.ControlRevision != r.ControlRevision || identity.ApprovalContext.Purpose != expected.ApprovalContext.Purpose || identity.ApprovalContext.CandidateID != r.Candidate.ID || identity.ApprovalContext.ManifestHash != r.Candidate.Hash {
		return ErrFenced
	}
	if r.Status == "paused" || r.Status == "stopped" || r.Status == "rejected" {
		return ErrRunClosed
	}
	if !r.Approved || !now.Before(r.ApprovalExpiresAt) {
		return ErrApprovalRequired
	}
	if !slices.Contains([]string{"planning", "running"}, r.Status) {
		return ErrRunClosed
	}
	switch action {
	case "session.open", "command.submit", "command.compact", "model.prepare", "model.generate", "model.generation", "model.compaction", "plan.register":
		return nil
	default:
		return ErrPermission
	}
}

// ControlInput carries the exact user decision; request/revision deduplication belongs to the ledger.
type ControlInput struct {
	Action       string    `json:"action"`
	CandidateID  uuid.UUID `json:"candidate_id,omitempty"`
	ManifestHash string    `json:"manifest_hash,omitempty"`
	MaxMicros    int64     `json:"max_micros,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Message      string    `json:"message,omitempty"`
}

func (r *Run) Apply(input ControlInput, now time.Time) error {
	switch input.Action {
	case "approve":
		if r.Status != "awaiting_planning_approval" && r.Status != "awaiting_execution_approval" {
			return ErrConflict
		}
		if input.CandidateID != r.Candidate.ID || input.ManifestHash != r.Candidate.Hash {
			return ErrConflict
		}
		if input.MaxMicros <= 0 || input.MaxMicros > r.Goal.RootBudgetMicros-r.ReservedMicros || !now.Before(input.ExpiresAt) || input.ExpiresAt.After(now.Add(24*time.Hour)) {
			return ErrInvalid
		}
		if r.Candidate.Purpose == "planning" && input.MaxMicros > r.Goal.PlanningBudgetMicros {
			return ErrBudget
		}
		r.Approved = true
		r.ApprovalMaxMicros = input.MaxMicros
		r.CandidateReservedMicros = 0
		r.ApprovalExpiresAt = input.ExpiresAt.UTC()
		r.Status = "planning"
		if r.Candidate.Purpose == "execution" {
			r.Status = "running"
		}
	case "pause":
		if r.Status != "planning" && r.Status != "running" {
			return ErrConflict
		}
		r.ResumeStatus = r.Status
		r.Status = "paused"
		r.ControlRevision++
	case "resume":
		if r.Status != "paused" {
			return ErrConflict
		}
		if !r.Approved || !now.Before(r.ApprovalExpiresAt) {
			return ErrApprovalRequired
		}
		r.Status = r.ResumeStatus
		r.ResumeStatus = ""
		r.ControlRevision++
	case "stop":
		if r.Status == "stopped" {
			return ErrConflict
		}
		r.Status = "stopped"
		r.ControlRevision++
		r.Approved = false
	case "reject":
		if r.Status != "awaiting_planning_approval" && r.Status != "awaiting_execution_approval" {
			return ErrConflict
		}
		r.Status = "rejected"
		r.ControlRevision++
		r.Approved = false
	case "plan.revise", "steer":
		if r.Status == "stopped" || strings.TrimSpace(input.Message) == "" || len(input.Message) > 256*1024 {
			return ErrConflict
		}
		r.Goal.Goal = input.Message
		r.GoalHash, _ = CanonicalHash(r.Goal)
		r.Candidate = planningCandidate(r.Goal)
		r.Status = "awaiting_planning_approval"
		r.Approved = false
		r.ControlRevision++
	default:
		return ErrInvalid
	}
	r.Revision++
	return nil
}
func (r *Run) RegisterPlan(candidateID uuid.UUID, plan PlanManifest, now time.Time) (ApprovalCandidate, error) {
	if candidateID == uuid.Nil || !r.Approved || !now.Before(r.ApprovalExpiresAt) || (r.Status != "planning" && r.Status != "running") {
		return ApprovalCandidate{}, ErrApprovalRequired
	}
	if plan.Revision != r.PlanRevision+1 {
		return ApprovalCandidate{}, ErrConflict
	}
	if err := plan.Validate(r.Goal, r.GoalHash); err != nil {
		return ApprovalCandidate{}, err
	}
	hash, err := CanonicalHash(plan)
	if err != nil {
		return ApprovalCandidate{}, err
	}
	raw, _ := json.Marshal(plan)
	candidate := ApprovalCandidate{ID: candidateID, Purpose: "execution", Hash: hash, Manifest: raw, PlanRevision: plan.Revision}
	r.Candidate = candidate
	r.PlanRevision = plan.Revision
	r.Status = "awaiting_execution_approval"
	r.Approved = false
	r.ControlRevision++
	r.Revision++
	return candidate, nil
}

// ActionRequest is derived by a trusted Go tool/model adapter, never a public price quote.
type ActionRequest struct {
	Action     string   `json:"action"`
	StepID     string   `json:"step_id,omitempty"`
	ParamsHash string   `json:"params_hash,omitempty"`
	Input      InputRef `json:"input"`
	Skill      SkillPin `json:"skill"`
	MaxMicros  int64    `json:"max_micros"`
}

func (r Run) CheckGrant(request ActionRequest, now time.Time) error {
	if err := r.Authorize(r.RuntimeIdentity(), "model.generate", now); err != nil {
		return err
	}
	if request.Action == "model.generate" || request.Action == "model.compaction" {
		return nil
	}
	if r.Candidate.Purpose == "planning" {
		switch request.Action {
		case "project.read":
			return nil
		case "script.read", "subject.read", "artifact.read":
			if request.Input.Kind+".read" == request.Action && slices.Contains(r.Goal.Inputs, request.Input) {
				return nil
			}
		case "skill.catalog":
			return nil
		case "skill.load", "skill.file":
			if slices.Contains(r.Goal.Skills, request.Skill) {
				return nil
			}
		}
		return ErrPermission
	}
	var plan PlanManifest
	if json.Unmarshal(r.Candidate.Manifest, &plan) != nil {
		return ErrInvalid
	}
	for _, step := range plan.Steps {
		if step.ID != request.StepID {
			continue
		}
		for _, action := range step.Actions {
			if action.Name == request.Action && (action.ParamsHash == "" || action.ParamsHash == request.ParamsHash) {
				return nil
			}
		}
	}
	return ErrPermission
}
func (r *Run) Reserve(request ActionRequest, now time.Time) error {
	if err := r.CheckGrant(request, now); err != nil {
		return err
	}
	if request.MaxMicros < 0 {
		return ErrInvalid
	}
	if request.MaxMicros > r.Goal.RootBudgetMicros-r.ReservedMicros || request.MaxMicros > r.ApprovalMaxMicros-r.CandidateReservedMicros {
		return ErrBudget
	}
	r.ReservedMicros += request.MaxMicros
	r.CandidateReservedMicros += request.MaxMicros
	return nil
}
