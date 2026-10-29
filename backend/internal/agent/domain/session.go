// Package domain defines the bounded image-creation Agent's durable state.
package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Policy identifies the server-enforced single-image planning protocol.
const Policy = "image-agent-json-v1"

// ErrInvalid denotes malformed input or an unauthorized plan action.
var ErrInvalid = errors.New("invalid_agent_request")

// ErrConflict denotes an outdated revision or conflicting replay.
var ErrConflict = errors.New("agent_revision_conflict")

// ErrNotFound hides missing and inaccessible conversations.
var ErrNotFound = errors.New("agent_session_not_found")

// ModelChoice freezes a selected model, mode, and parameter values.
type ModelChoice struct {
	Key    string          `json:"key"`
	Mode   string          `json:"mode"`
	Params json.RawMessage `json:"params" swaggertype:"object"`
}

// TurnInput carries one idempotent goal and its selected models.
type TurnInput struct {
	RequestID        uuid.UUID   `json:"request_id"`
	ExpectedRevision int64       `json:"expected_revision"`
	Message          string      `json:"message"`
	Planner          ModelChoice `json:"planner"`
	Image            ModelChoice `json:"image"`
}

// Valid checks the bounded inputs allowed by this planning policy.
func (in TurnInput) Valid() bool {
	return in.RequestID != uuid.Nil && in.ExpectedRevision > 0 && strings.TrimSpace(in.Message) != "" && len(in.Message) <= 8000 && in.Planner.Key != "" && len(in.Planner.Key) <= 200 && in.Planner.Mode == "text_to_text" && in.Image.Key != "" && len(in.Image.Key) <= 200 && (in.Image.Mode == "text2image" || in.Image.Mode == "text_to_image") && len(in.Planner.Params) <= 8192 && json.Valid(in.Planner.Params) && len(in.Image.Params) <= 8192 && json.Valid(in.Image.Params)
}

// Plan contains the only two actions the planner may request.
type Plan struct {
	Action  string `json:"action"`
	Message string `json:"message"`
	Prompt  string `json:"prompt"`
}

// ParsePlan accepts one explicit action and never treats supplier output as authority.
func ParsePlan(value string) (Plan, error) {
	var plan Plan
	if len(value) > 16000 {
		return plan, ErrInvalid
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return Plan{}, ErrInvalid
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return Plan{}, ErrInvalid
		}
		name, ok := token.(string)
		if !ok || seen[name] {
			return Plan{}, ErrInvalid
		}
		seen[name] = true
		var text string
		if decoder.Decode(&text) != nil {
			return Plan{}, ErrInvalid
		}
		switch name {
		case "action":
			plan.Action = text
		case "message":
			plan.Message = text
		case "prompt":
			plan.Prompt = text
		default:
			return Plan{}, ErrInvalid
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') || len(seen) != 3 {
		return Plan{}, ErrInvalid
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || strings.TrimSpace(plan.Message) == "" || len(plan.Message) > 4000 || len(plan.Prompt) > 8000 {
		return Plan{}, ErrInvalid
	}
	if plan.Action == "reply" && plan.Prompt == "" || plan.Action == "generate_image" && strings.TrimSpace(plan.Prompt) != "" {
		return plan, nil
	}
	return Plan{}, ErrInvalid
}

// Stage retains the original operation and its displayed quote.
type Stage struct {
	Phase       string    `json:"phase"`
	OperationID uuid.UUID `json:"operation_id"`
	QuoteMicros int64     `json:"quote_micros"`
	ExpiresAt   time.Time `json:"expires_at"`
	Status      string    `json:"status"`
}

// Turn records the durable progress of one bounded creation request.
type Turn struct {
	ID           uuid.UUID  `json:"id"`
	Input        TurnInput  `json:"input"`
	Policy       string     `json:"policy"`
	Phase        string     `json:"phase"`
	Status       string     `json:"status"`
	Stages       []Stage    `json:"stages"`
	Plan         *Plan      `json:"plan,omitempty"`
	Reply        string     `json:"reply,omitempty"`
	OutputID     *uuid.UUID `json:"output_id,omitempty"`
	MediaAssetID *uuid.UUID `json:"media_asset_id,omitempty"`
	FailureCode  string     `json:"failure_code,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Terminal reports whether automatic advancement must stop.
func (t Turn) Terminal() bool {
	return t.Status == "completed" || t.Status == "failed" || t.Status == "stopped" || t.Status == "expired"
}

// Session is an actor-private conversation within a project.
type Session struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	ActorID   uuid.UUID `json:"-"`
	Title     string    `json:"title"`
	Revision  int64     `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
	Turns     []Turn    `json:"turns"`
}

// SameInput excludes the optimistic revision, which changes after a successful replay.
func SameInput(a, b TurnInput) bool {
	a.ExpectedRevision = 0
	b.ExpectedRevision = 0
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return bytes.Equal(left, right)
}
