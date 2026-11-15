package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// SessionAdmissionProof binds an already registered command to its original
// account, exact session and credential generation. It is current Identity policy
// evidence only, never a scope permission, business outcome or final-write fence.
type SessionAdmissionProof struct {
	AccountID       uuid.UUID
	SessionID       uuid.UUID
	CredentialEpoch int64
	Command         writefence.Command
	Target          writefence.Writer
}

// SessionAdmissionReader checks an existing immutable admission and current
// Identity policy without registering work or opening authority heads. The actor
// comes from durable Identity evidence, never an input Principal.
type SessionAdmissionReader interface {
	ReadSessionAdmission(context.Context, writefence.Command, writefence.Writer, func() time.Time) (SessionAdmissionProof, error)
}
