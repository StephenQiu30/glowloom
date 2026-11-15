package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// ReferencePinReceiptKey binds a consumer's read to one original acquisition.
// Reading never registers a command, advances a gate or replaces an admission.
type ReferencePinReceiptKey struct {
	PinID, AcquireCommandID, ConsumerCommandID uuid.UUID
	ConsumerResourceID, ScopeID, ProjectID     uuid.UUID
	Consumer, ConsumerCommandSHA256            string
}

// Validate rejects partial identities and unsupported consumer domains.
func (k ReferencePinReceiptKey) Validate() error {
	if k.PinID == uuid.Nil || k.AcquireCommandID == uuid.Nil || k.ConsumerCommandID == uuid.Nil ||
		k.ConsumerResourceID == uuid.Nil || k.ScopeID == uuid.Nil || k.ProjectID == uuid.Nil || !domain.ReferencePinDigestValid(k.ConsumerCommandSHA256) {
		return domain.ErrInvalidReferencePin
	}
	switch k.Consumer {
	case "identity", "creation", "execution":
		return nil
	default:
		return domain.ErrInvalidReferencePin
	}
}

// ReferencePinReceiptReader resolves only a committed Acquire and its current
// local retention state. Unknown or missing evidence is an error, never a proof
// of non-adoption or permission to release bytes.
type ReferencePinReceiptReader interface {
	ReadReferencePinReceipt(context.Context, ReferencePinReceiptKey) (domain.ReferencePin, error)
}
