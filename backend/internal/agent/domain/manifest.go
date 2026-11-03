package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/gowebpki/jcs"
)

const HarnessEngine = "pi-durable-v1"

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var stepPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)

// CanonicalHash uses the same ECMAScript JSON representation as the runtime.
func CanonicalHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", ErrInvalid
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return "", ErrInvalid
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

// DialogueModel is a release selected by trusted, project-scoped catalog preflight.
type DialogueModel struct {
	ReleaseID     uuid.UUID       `json:"release_id"`
	ContextWindow int             `json:"context_window"`
	MaxTokens     int             `json:"max_tokens"`
	Input         []string        `json:"input"`
	Params        json.RawMessage `json:"params" swaggertype:"object"`
}

// InputRef freezes an authorized business source, not a transient URL.
type InputRef struct {
	Kind      string    `json:"kind"`
	ID        uuid.UUID `json:"id"`
	VersionID uuid.UUID `json:"version_id"`
	Hash      string    `json:"hash"`
}

// SkillPin is an immutable, authorized package version.
type SkillPin struct {
	ID        uuid.UUID `json:"id"`
	VersionID uuid.UUID `json:"version_id"`
	Hash      string    `json:"hash"`
}

// GoalManifest is the immutable authorization snapshot for paid planning.
type GoalManifest struct {
	Schema               string        `json:"schema"`
	Goal                 string        `json:"goal"`
	Model                DialogueModel `json:"model"`
	Inputs               []InputRef    `json:"inputs"`
	Skills               []SkillPin    `json:"skills"`
	PlanningBudgetMicros int64         `json:"planning_budget_micros"`
	RootBudgetMicros     int64         `json:"root_budget_micros"`
}

func (g GoalManifest) Validate() error {
	if g.Schema != "glowloom.goal/v1" || strings.TrimSpace(g.Goal) == "" || len(g.Goal) > 256*1024 || g.Model.ReleaseID == uuid.Nil || g.Model.ContextWindow < 1024 || g.Model.MaxTokens < 1 || g.Model.MaxTokens >= g.Model.ContextWindow || !slices.Contains(g.Model.Input, "text") || !json.Valid(g.Model.Params) || len(g.Model.Params) > 8192 || g.PlanningBudgetMicros <= 0 || g.RootBudgetMicros < g.PlanningBudgetMicros || g.RootBudgetMicros > 9007199254740991 || len(g.Inputs) > 128 || len(g.Skills) > 32 {
		return ErrInvalid
	}
	for _, ref := range g.Inputs {
		if ref.ID == uuid.Nil || ref.VersionID == uuid.Nil || !digestPattern.MatchString(ref.Hash) || !slices.Contains([]string{"script", "subject", "artifact"}, ref.Kind) {
			return ErrInvalid
		}
	}
	for _, pin := range g.Skills {
		if pin.ID == uuid.Nil || pin.VersionID == uuid.Nil || !digestPattern.MatchString(pin.Hash) {
			return ErrInvalid
		}
	}
	return nil
}

// ActionGrant constrains one action in an execution step.
type ActionGrant struct {
	Name       string `json:"name"`
	ParamsHash string `json:"params_hash,omitempty"`
}

// PlanStep describes intent and dependencies; Go never schedules its model loop.
type PlanStep struct {
	ID        string        `json:"id"`
	Agent     string        `json:"agent"`
	DependsOn []string      `json:"depends_on"`
	Required  bool          `json:"required"`
	Actions   []ActionGrant `json:"actions"`
}

// PlanManifest is frozen in the SDK before it is registered for user approval.
type PlanManifest struct {
	Schema         string     `json:"schema"`
	Revision       int64      `json:"revision"`
	GoalHash       string     `json:"goal_hash"`
	ModelReleaseID uuid.UUID  `json:"model_release_id"`
	Inputs         []InputRef `json:"inputs"`
	Skills         []SkillPin `json:"skills"`
	Steps          []PlanStep `json:"steps"`
}

func (p PlanManifest) Validate(goal GoalManifest, goalHash string) error {
	if p.Schema != "glowloom.plan/v1" || p.Revision < 1 || p.GoalHash != goalHash || p.ModelReleaseID != goal.Model.ReleaseID || len(p.Steps) == 0 || len(p.Steps) > 128 {
		return ErrInvalid
	}
	for _, ref := range p.Inputs {
		if !slices.Contains(goal.Inputs, ref) {
			return ErrPermission
		}
	}
	for _, pin := range p.Skills {
		if !slices.Contains(goal.Skills, pin) {
			return ErrPermission
		}
	}
	steps := map[string]PlanStep{}
	for _, step := range p.Steps {
		if !stepPattern.MatchString(step.ID) || step.Agent == "" || len(step.Agent) > 160 || len(step.Actions) == 0 || len(step.Actions) > 32 {
			return ErrInvalid
		}
		if _, ok := steps[step.ID]; ok {
			return ErrInvalid
		}
		steps[step.ID] = step
		for _, action := range step.Actions {
			if action.Name == "" || len(action.Name) > 128 || (action.ParamsHash != "" && !digestPattern.MatchString(action.ParamsHash)) {
				return ErrInvalid
			}
		}
	}
	visited := map[string]bool{}
	visiting := map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visited[id] {
			return true
		}
		if visiting[id] {
			return false
		}
		step, ok := steps[id]
		if !ok {
			return false
		}
		visiting[id] = true
		for _, dep := range step.DependsOn {
			if !visit(dep) {
				return false
			}
		}
		visiting[id] = false
		visited[id] = true
		return true
	}
	for id := range steps {
		if !visit(id) {
			return ErrInvalid
		}
	}
	return nil
}

// ApprovalCandidate is an immutable manifest awaiting an explicit user decision.
type ApprovalCandidate struct {
	ID           uuid.UUID       `json:"id"`
	Purpose      string          `json:"purpose"`
	Hash         string          `json:"hash"`
	Manifest     json.RawMessage `json:"manifest" swaggertype:"object"`
	PlanRevision int64           `json:"plan_revision"`
}

func planningCandidate(goal GoalManifest) ApprovalCandidate {
	hash, _ := CanonicalHash(goal)
	raw, _ := json.Marshal(goal)
	return ApprovalCandidate{ID: uuid.New(), Purpose: "planning", Hash: hash, Manifest: raw}
}
