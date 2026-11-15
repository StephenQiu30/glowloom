package source

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// AdmissionProof reads the original Identity admission for an effect owner.
// Resolve must authenticate Creation, check the full command/target and current
// account/session/head policy, and never create an admission or reopen a head.
type AdmissionProof interface {
	ResolveSourceAdmission(context.Context, writefence.Command, writefence.Writer) (Identity, error)
}

// ProofAdmission checks an effect owner's existing Identity admission before
// Creation records current resource policy. It never accepts a caller Principal,
// forwards another owner's credentials, or registers Identity on its behalf.
type ProofAdmission struct {
	proof AdmissionProof
	store CommandStore
}

// NewProofAdmission injects the private Identity proof reader and local store.
func NewProofAdmission(proof AdmissionProof, store CommandStore) *ProofAdmission {
	return &ProofAdmission{proof: proof, store: store}
}

// Admit preserves the same command and target across both source issuers.
// A missing proof fails closed; no historical projection grants fresh membership.
func (s *ProofAdmission) Admit(ctx context.Context, request AdmissionRequest) (Permit, error) {
	if s == nil || s.proof == nil || s.store == nil {
		return Permit{}, writefence.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return Permit{}, err
	}
	if request.Target != writefence.WriterExecution && request.Target != writefence.WriterMedia {
		return Permit{}, writefence.ErrInvalid
	}
	identity, err := s.proof.ResolveSourceAdmission(ctx, request.Command, request.Target)
	if err != nil {
		return Permit{}, fmt.Errorf("resolve original source identity admission: %w", err)
	}
	if err := request.Validate(identity); err != nil {
		return Permit{}, err
	}
	permit, err := s.store.RegisterSourceCommand(ctx, identity, request)
	if err != nil {
		return Permit{}, fmt.Errorf("register original source resource admission: %w", err)
	}
	if !sourcePermitMatches(permit, identity, request) {
		return Permit{}, writefence.ErrConflict
	}
	return permit, nil
}

func sourcePermitMatches(permit Permit, identity Identity, request AdmissionRequest) bool {
	got, want := permit.Command, request.Command
	gd, ge := got.AuthorityDigest()
	wd, we := want.AuthorityDigest()
	return ge == nil && we == nil && gd == wd && got.ID == want.ID && got.SHA256 == want.SHA256 && got.Action == want.Action && permit.Target == request.Target && permit.AccountID == identity.AccountID && permit.SessionID == identity.SessionID && permit.ScopeID == request.ScopeID && permit.ProjectID == request.ProjectID && permit.PhysicalScopeID != uuid.Nil
}
