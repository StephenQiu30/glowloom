package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// ReferencePinSourceProof identifies the current Creation policy transaction for
// the exact original acquisition. It cannot prove adoption or authorize a release.
type ReferencePinSourceProof struct {
	AccountID, SessionID, PhysicalScopeID uuid.UUID
}

// ReferencePinSourceAdmission resolves the original target-Media Identity proof
// before Creation checks scope/member/project policy and records source admission.
// It runs outside Media's final transaction, whose Guard still fences the effect.
type ReferencePinSourceAdmission interface {
	AdmitReferencePinSource(context.Context, ReferencePinRequest) (ReferencePinSourceProof, error)
}

// Validate requires the exact acquisition resource identity before a private RPC.
func (p ReferencePinSourceProof) Validate(request ReferencePinRequest) error {
	if request.Validate() != nil || request.Command.Action != "media.reference_pin.acquire" || len(request.Command.Authorities) != 4 ||
		p.AccountID == uuid.Nil || p.SessionID == uuid.Nil || p.PhysicalScopeID == uuid.Nil {
		return domain.ErrReferencePinProof
	}
	if _, err := request.Command.AuthorityDigest(); err != nil {
		return err
	}
	for _, a := range request.Command.Authorities {
		switch {
		case a.Issuer == writefence.Identity && a.Kind == writefence.Account && a.ID == p.AccountID:
		case a.Issuer == writefence.Identity && a.Kind == writefence.Session && a.ID == p.SessionID:
		case a.Issuer == writefence.Creation && a.Kind == writefence.Scope && a.ID == request.Identity.ScopeID:
		case a.Issuer == writefence.Creation && a.Kind == writefence.Project && a.ID == request.Identity.ProjectID:
		default:
			return domain.ErrReferencePinProof
		}
	}
	return nil
}
