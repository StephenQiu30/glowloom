// Package domain defines Agent business authority, without execution state.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// Agent errors classify invalid input, unavailable authority, conflicts and budget failures.
var (
	ErrInvalid     = errors.New("invalid_agent_request")
	ErrConflict    = errors.New("agent_revision_conflict")
	ErrNotFound    = errors.New("agent_not_found")
	ErrBlocked     = errors.New("agent_authorization_blocked")
	ErrTerminal    = errors.New("agent_run_terminal")
	ErrGrant       = errors.New("agent_grant_invalid")
	ErrBudget      = errors.New("agent_budget_exceeded")
	ErrUnavailable = errors.New("agent_capability_unavailable")
)

// Scope is established by authenticated Go entry points, never model arguments.
type Scope struct {
	ActorID   uuid.UUID  `json:"actor_id"`
	OrgID     uuid.UUID  `json:"org_id"`
	ScopeID   uuid.UUID  `json:"scope_id"`
	ProjectID uuid.UUID  `json:"project_id"`
	CanvasID  *uuid.UUID `json:"canvas_id,omitempty"`
}

// Principal returns the authenticated identity carried by the established scope.
func (s Scope) Principal() identity.Principal {
	return identity.Principal{ID: s.ActorID, OrgID: s.OrgID, ScopeID: s.ScopeID}
}

// Valid reports whether all required scope identities are present.
func (s Scope) Valid() bool {
	return s.ActorID != uuid.Nil && s.OrgID != uuid.Nil && s.ScopeID != uuid.Nil && s.ProjectID != uuid.Nil && (s.CanvasID == nil || *s.CanvasID != uuid.Nil)
}

// Admission references an accepted run; it does not mirror its Pi lifecycle.
type Admission struct {
	RootRunID      uuid.UUID       `json:"root_run_id"`
	SessionID      uuid.UUID       `json:"session_id"`
	SubmissionID   uuid.UUID       `json:"submission_id"`
	RequestID      uuid.UUID       `json:"request_id"`
	ContentHash    string          `json:"content_hash"`
	Scope          Scope           `json:"scope"`
	BudgetMicros   int64           `json:"budget_micros"`
	SkillPins      []SkillPin      `json:"skill_pins"`
	ModelSelection json.RawMessage `json:"model_selection" swaggertype:"object"`
}

// Authority combines admitted intent with current authorization and control facts.
type Authority struct {
	Admission
	AuthorizationRevision int64     `json:"authorization_revision"`
	OwnerEpoch            int64     `json:"owner_epoch"`
	Barrier               bool      `json:"barrier"`
	Terminal              bool      `json:"terminal"`
	ReservedMicros        int64     `json:"reserved_micros"`
	Current               *Decision `json:"current,omitempty"`
}

// GrantRequest identifies the runtime ownership window requesting a capability.
type GrantRequest struct {
	RootRunID             uuid.UUID `json:"root_run_id"`
	SubmissionID          uuid.UUID `json:"submission_id"`
	OwnerEpoch            int64     `json:"owner_epoch"`
	AuthorizationRevision int64     `json:"authorization_revision"`
	LeaseUntil            time.Time `json:"lease_until"`
}

// Grant is an opaque cryptographically random capability, checked against Go facts.
type Grant struct {
	// AvailableMicros is a current ledger observation, not part of grant identity.
	AvailableMicros       int64     `json:"available_micros"`
	GrantID               uuid.UUID `json:"grant_id"`
	RootRunID             uuid.UUID `json:"root_run_id"`
	SubmissionID          uuid.UUID `json:"submission_id"`
	OwnerEpoch            int64     `json:"owner_epoch"`
	AuthorizationRevision int64     `json:"authorization_revision"`
	ExpiresAt             time.Time `json:"expires_at"`
	Scope                 Scope     `json:"scope"`
}

// DecisionInput contains an idempotent control request and its expected authorization revision.
type DecisionInput struct {
	RequestID                     uuid.UUID `json:"request_id"`
	Action                        string    `json:"action"`
	ExpectedAuthorizationRevision int64     `json:"expected_authorization_revision"`
	ProposalID                    uuid.UUID `json:"proposal_id,omitempty"`
	CandidateHash                 string    `json:"candidate_hash,omitempty"`
}

// Decision is an accepted business control decision with its stable identity.
type Decision struct {
	DecisionID            uuid.UUID  `json:"decision_id"`
	RootRunID             uuid.UUID  `json:"root_run_id"`
	SessionID             uuid.UUID  `json:"session_id"`
	AuthorizationRevision int64      `json:"authorization_revision"`
	Action                string     `json:"action"`
	ContentHash           string     `json:"content_hash"`
	ProposalID            *uuid.UUID `json:"proposal_id,omitempty"`
	CandidateHash         string     `json:"candidate_hash,omitempty"`
	Scope
}

// QuiescedReceipt proves stopped execution for a specific decision and owner epoch.
type QuiescedReceipt struct {
	ReceiptID             uuid.UUID `json:"receipt_id"`
	DecisionID            uuid.UUID `json:"decision_id"`
	AuthorizationRevision int64     `json:"authorization_revision"`
	OwnerEpoch            int64     `json:"owner_epoch"`
	ContinuationID        uuid.UUID `json:"continuation_id"`
	LeaseUntil            time.Time `json:"lease_until"`
}

// AppliedReceipt records the runtime outcome of applying a control decision.
type AppliedReceipt struct {
	ReceiptID             uuid.UUID `json:"receipt_id"`
	DecisionID            uuid.UUID `json:"decision_id"`
	AuthorizationRevision int64     `json:"authorization_revision"`
	Status                string    `json:"status"`
	ExecutionRevision     int64     `json:"execution_revision"`
}

// CallIdentity pins the task, call, checkpoint, attempt and input hash.
type CallIdentity struct {
	TaskID          string `json:"task_id"`
	CallID          string `json:"call_id"`
	CheckpointID    string `json:"checkpoint_id"`
	Attempt         int64  `json:"attempt"`
	ContractVersion string `json:"contract_version"`
	InputHash       string `json:"input_hash"`
}

// CallInput combines a current grant, invocation identity and bounded business input.
type CallInput struct {
	Grant     Grant           `json:"grant"`
	Call      CallIdentity    `json:"call"`
	Input     json.RawMessage `json:"input" swaggertype:"object"`
	MaxMicros int64           `json:"max_micros"`
}

// CallResult records a classified business outcome, result and cost.
type CallResult struct {
	Status     string          `json:"status"`
	Result     json.RawMessage `json:"result" swaggertype:"object"`
	CostMicros int64           `json:"cost_micros"`
}

// CallReceipt binds an invocation and its result to a durable receipt identity.
type CallReceipt struct {
	MaxMicros    int64        `json:"-"`
	Kind         string       `json:"kind"`
	Name         string       `json:"name"`
	ReceiptID    uuid.UUID    `json:"receipt_id"`
	RootRunID    uuid.UUID    `json:"root_run_id"`
	SubmissionID uuid.UUID    `json:"submission_id"`
	Call         CallIdentity `json:"call"`
	CallResult
}

// ProposalInput pins a candidate and the grant, revision, price and budget used to propose it.
type ProposalInput struct {
	Grant          Grant           `json:"grant"`
	RequestID      uuid.UUID       `json:"request_id"`
	CandidateHash  string          `json:"candidate_hash"`
	CanvasRevision int64           `json:"canvas_revision"`
	PriceVersion   string          `json:"price_version"`
	BudgetMicros   int64           `json:"budget_micros"`
	Candidate      json.RawMessage `json:"candidate" swaggertype:"object"`
}

// Proposal records the accepted candidate and its authorization revision.
type Proposal struct {
	ProposalID            uuid.UUID `json:"proposal_id"`
	RootRunID             uuid.UUID `json:"root_run_id"`
	AuthorizationRevision int64     `json:"authorization_revision"`
	ProposalInput
	Control struct {
		Terminate bool `json:"terminate"`
	} `json:"control"`
}

// Hash hashes canonical JSON when possible and otherwise hashes the original bytes.
func Hash(b []byte) string {
	var v any
	if json.Unmarshal(b, &v) == nil {
		if canonical, err := json.Marshal(v); err == nil {
			b = canonical
		}
	}
	return RawHash(b)
}

// ContentHash serializes and hashes a value, returning an empty hash on encoding failure.
func ContentHash(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return Hash(b)
}

// ContinuationID derives a stable continuation identity from its run and decision.
func ContinuationID(run, decision uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(run, []byte("continuation:"+decision.String()))
}

// RootRunID derives a stable run identity from its session and request.
func RootRunID(session, request uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(session, []byte("run:"+request.String()))
}

// SubmissionID derives the initial submission identity for a run.
func SubmissionID(run uuid.UUID) uuid.UUID { return uuid.NewSHA1(run, []byte("submission:initial")) }

// RawHash returns the SHA-256 digest of the exact input bytes.
func RawHash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
