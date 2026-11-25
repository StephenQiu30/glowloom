package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/billing/domain"
)

var (
	// ErrProjectBudgetPreparation rejects retargeted original bootstrap responsibility.
	ErrProjectBudgetPreparation = errors.New("project budget preparation conflict")
	// ErrProjectBudgetUnknown requires original Resolve before any replacement attempt.
	ErrProjectBudgetUnknown = errors.New("project budget preparation unresolved")
	// ErrProjectBudgetOwner rejects non-Execution or nested-transaction execution.
	ErrProjectBudgetOwner = errors.New("project budget preparation requires Execution owner")
)

// ProjectBudgetPreparation binds an independent Execution command to the exact
// immutable Creation project-preparation receipt. It contains no spending grant,
// arbitrary budget amount, user credential or caller-supplied Principal.
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

// Validate requires the original deterministic Execution effect and closed source hashes.
func (p ProjectBudgetPreparation) Validate() error {
	if p.PreparationID == uuid.Nil || p.ExecutionCommandID != uuid.NewSHA1(p.PreparationID, []byte("execution.project_budget.prepare")) || p.SourceCommandID == uuid.Nil || p.SourceCommandID == p.ExecutionCommandID || p.ProjectID == uuid.Nil || p.OrganizationID == uuid.Nil || p.ScopeID == uuid.Nil {
		return ErrProjectBudgetPreparation
	}
	for _, value := range []string{p.SourceSHA256, p.InputSHA256} {
		b, err := hex.DecodeString(value)
		if err != nil || len(b) != 32 || hex.EncodeToString(b) != value {
			return ErrProjectBudgetPreparation
		}
	}
	return nil
}

// Digest preserves all original resource identities without float conversions.
func (p ProjectBudgetPreparation) Digest() string {
	raw, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ProjectPreparedProof is a read-only attestation of already persisted Creation
// project/source facts. Prepared never means public activation or spending permission.
type ProjectPreparedProof struct {
	Request ProjectBudgetPreparation
	Phase   string
}

// ProjectPreparationSource is the consumer-owned minimum private source proof port.
// It executes outside the Execution SQL transaction and must not create source facts.
type ProjectPreparationSource interface {
	ResolveProjectPreparation(context.Context, ProjectBudgetPreparation) (ProjectPreparedProof, error)
}

// ProjectBudgetReceipt retains initial preparation facts even if the active budget
// changes later. No credit lot or wallet grant is part of this responsibility.
type ProjectBudgetReceipt struct {
	Request        ProjectBudgetPreparation `json:"request"`
	BudgetID       uuid.UUID                `json:"budget_id"`
	BudgetRevision int64                    `json:"budget_revision"`
	LimitMicros    int64                    `json:"limit_micros"`
	SpentMicros    int64                    `json:"spent_micros"`
	ReservedMicros int64                    `json:"reserved_micros"`
	WalletPrepared bool                     `json:"wallet_prepared"`
}

// NewProjectBudgetReceipt fixes the existing uncapped ceiling and zero opening usage.
func NewProjectBudgetReceipt(p ProjectBudgetPreparation) ProjectBudgetReceipt {
	return ProjectBudgetReceipt{Request: p, BudgetID: uuid.NewSHA1(p.PreparationID, []byte("billing.project_budget")), BudgetRevision: 1, LimitMicros: domain.UncappedBudgetMicros, WalletPrepared: true}
}

// Validate accepts only the exact immutable initial receipt, never mutable current balance.
func (r ProjectBudgetReceipt) Validate() error {
	if r.Request.Validate() != nil || r != NewProjectBudgetReceipt(r.Request) {
		return ErrProjectBudgetPreparation
	}
	return nil
}

// ProjectBudgetRepository commits the wallet account, budget and original receipt
// together. Resolve is read-only and cannot adopt a different existing budget.
type ProjectBudgetRepository interface {
	ResolveProjectBudget(context.Context, ProjectBudgetPreparation) (ProjectBudgetReceipt, error)
	CommitProjectBudget(context.Context, ProjectBudgetPreparation, ProjectPreparedProof) (ProjectBudgetReceipt, error)
}

// ProjectBudgets coordinates exact source proof and local preparation without
// SQL locks over RPC, permissions to spend, or a new remote Billing service.
type ProjectBudgets struct {
	store  ProjectBudgetRepository
	source ProjectPreparationSource
}

// NewProjectBudgets refuses a missing actual source participant; no fallback is provided.
func NewProjectBudgets(store ProjectBudgetRepository, source ProjectPreparationSource) (*ProjectBudgets, error) {
	if store == nil || source == nil {
		return nil, ErrProjectBudgetPreparation
	}
	return &ProjectBudgets{store: store, source: source}, nil
}

// Prepare replays an original ACK before network I/O and otherwise requires the
// exact durable Creation source proof before the single local effect transaction.
func (s *ProjectBudgets) Prepare(ctx context.Context, p ProjectBudgetPreparation) (ProjectBudgetReceipt, error) {
	if s == nil || s.store == nil || s.source == nil || p.Validate() != nil {
		return ProjectBudgetReceipt{}, ErrProjectBudgetPreparation
	}
	receipt, err := s.store.ResolveProjectBudget(ctx, p)
	if err == nil {
		return s.exact(p, receipt)
	}
	if !errors.Is(err, ErrProjectBudgetUnknown) {
		return ProjectBudgetReceipt{}, err
	}
	proof, err := s.source.ResolveProjectPreparation(ctx, p)
	if err != nil {
		return ProjectBudgetReceipt{}, err
	}
	if proof.Request != p || proof.Phase != "prepared" {
		return ProjectBudgetReceipt{}, ErrProjectBudgetPreparation
	}
	receipt, err = s.store.CommitProjectBudget(ctx, p, proof)
	if err != nil {
		return ProjectBudgetReceipt{}, err
	}
	return s.exact(p, receipt)
}

// Resolve returns only the original immutable ACK and never creates or reopens work.
func (s *ProjectBudgets) Resolve(ctx context.Context, p ProjectBudgetPreparation) (ProjectBudgetReceipt, error) {
	if s == nil || s.store == nil || p.Validate() != nil {
		return ProjectBudgetReceipt{}, ErrProjectBudgetPreparation
	}
	receipt, err := s.store.ResolveProjectBudget(ctx, p)
	if err != nil {
		return ProjectBudgetReceipt{}, err
	}
	return s.exact(p, receipt)
}
func (s *ProjectBudgets) exact(p ProjectBudgetPreparation, r ProjectBudgetReceipt) (ProjectBudgetReceipt, error) {
	if r.Request != p || r.Validate() != nil {
		return ProjectBudgetReceipt{}, ErrProjectBudgetPreparation
	}
	return r, nil
}
