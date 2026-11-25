// Package application defines the consumed business and runtime boundaries.
package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ControlAccess checks current request and worker access to an Agent project.
type ControlAccess interface {
	AuthorizeRequest(context.Context, identity.Principal, uuid.UUID, bool) error
	AuthorizeWorker(context.Context, identity.Principal, uuid.UUID) error
}

// Executor runs a business call under the established scope and receipt identity.
type Executor func(context.Context, domain.Scope, uuid.UUID) (domain.CallResult, error)

// DecisionValidator validates a proposed business change within its owning scope.
type DecisionValidator interface {
	ValidateProposal(context.Context, domain.Scope, domain.ProposalInput) error
}

// BusinessStore persists business facts only. Runtime state is never available here.
type BusinessStore interface {
	AdmitRun(context.Context, domain.Admission) error
	ReadAuthority(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (domain.Authority, error)
	RegisterGrant(context.Context, domain.GrantRequest) (domain.Grant, error)
	ValidateGrant(context.Context, domain.Grant) (domain.Grant, error)
	Decide(context.Context, identity.Principal, uuid.UUID, uuid.UUID, domain.DecisionInput) (domain.Decision, error)
	CurrentAuthority(context.Context, uuid.UUID) (domain.Authority, error)
	Quiesce(context.Context, uuid.UUID, domain.QuiescedReceipt) (domain.Grant, error)
	Acknowledge(context.Context, uuid.UUID, domain.AppliedReceipt) error
	Propose(context.Context, domain.ProposalInput) (domain.Proposal, error)
	ExecuteCall(context.Context, string, string, domain.CallInput, Executor) (domain.CallReceipt, error)
	ReadCall(context.Context, uuid.UUID, uuid.UUID, domain.CallIdentity) (domain.CallReceipt, error)
	ClaimDecision(context.Context, time.Time) (domain.Decision, bool, error)
	RetryDecision(context.Context, uuid.UUID, time.Time) error
}

// RecoverCapability recovers the result belonging to an existing call receipt.
type RecoverCapability func(context.Context, uuid.UUID) (domain.CallResult, error)

// ModelCapability executes a bounded model call using Go-owned authority and billing.
type ModelCapability func(context.Context, domain.Scope, uuid.UUID, json.RawMessage, int64) (domain.CallResult, error)

// ToolDescriptor declares a tool's schema, roles, effects and replay contract.
type ToolDescriptor struct {
	Name            string          `json:"name"`
	Roles           []string        `json:"roles"`
	Description     string          `json:"description"`
	Parameters      json.RawMessage `json:"parameters" swaggertype:"object"`
	Effect          string          `json:"effect"`
	ContractVersion string          `json:"contract_version"`
	Replay          string          `json:"replay"`
}

// Tool pairs a declared business capability with its execution function.
type Tool struct {
	Descriptor ToolDescriptor
	Execute    func(context.Context, domain.Scope, uuid.UUID, json.RawMessage) (domain.CallResult, error)
}

// Capabilities contains explicitly installed formal business adapters.
type Capabilities struct {
	Tools        []Tool
	Model        ModelCapability
	ModelRecover RecoverCapability
	ToolRecover  RecoverCapability
	Snapshot     func(context.Context, domain.Scope) (any, error)
	RunSnapshot  func(context.Context, domain.Grant) (any, error)
}
