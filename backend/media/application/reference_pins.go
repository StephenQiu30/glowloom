package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// ReferencePinRequest uses a separately admitted command for each local action.
// Command.SHA256 remains the original business digest, never a replacement hash.
type ReferencePinRequest struct {
	Command  writefence.Command
	Identity domain.ReferencePinIdentity
}

// Validate binds the local command's fences to the exact scope and project.
// A valid fence for another resource cannot authorize this pin's effect.
func (r ReferencePinRequest) Validate() error {
	if err := r.Identity.Validate(); err != nil {
		return err
	}
	var scope, project bool
	for _, authority := range r.Command.Authorities {
		if authority.Issuer != writefence.Creation {
			continue
		}
		scope = scope || authority.Kind == writefence.Scope && authority.ID == r.Identity.ScopeID
		project = project || authority.Kind == writefence.Project && authority.ID == r.Identity.ProjectID
	}
	if !scope || !project {
		return domain.ErrInvalidReferencePin
	}
	return nil
}

// CloseReferenceAcquisitionRequest closes exactly one open gate generation.
type CloseReferenceAcquisitionRequest struct {
	Command writefence.Command
	AssetID uuid.UUID
	Epoch   int64
}

// ReferencePinConsumerVerifier belongs to the consuming service boundary. It
// must authenticate the fixed consumer, original command, resource and receipt.
// VerifyRelease proves either permanent non-adoption (the original command is
// frozen against late commit), or durable removal of this exact reference.
// Calls run outside the Media transaction; unknown evidence must return an error.
type ReferencePinConsumerVerifier interface {
	VerifyAdoption(context.Context, domain.ReferencePin) (domain.ReferencePinProof, error)
	VerifyRelease(context.Context, domain.ReferencePin) (domain.ReferencePinProof, error)
}

// ReferencePins is the durable Media responsibility port. Resolve is a separate
// fenced recovery command; Acquire replay always returns its original held ACK.
type ReferencePins interface {
	Acquire(context.Context, ReferencePinRequest) (domain.ReferencePin, error)
	Resolve(context.Context, ReferencePinRequest) (domain.ReferencePin, error)
	Bind(context.Context, ReferencePinRequest) (domain.ReferencePin, error)
	Release(context.Context, ReferencePinRequest) (domain.ReferencePin, error)
	CloseAcquisition(context.Context, CloseReferenceAcquisitionRequest) (domain.ReferenceAcquisition, error)
}
