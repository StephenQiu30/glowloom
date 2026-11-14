package application

import (
	"context"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// SessionAdmissionStore records exact account/session liveness under the original
// Identity policy locks. It does not grant scope, platform or effect-owner business
// permission. Those independent decisions remain mandatory at their own owners.
type SessionAdmissionStore interface {
	RegisterSessionCommand(context.Context, Principal, writefence.Command, writefence.Writer, func() time.Time) error
}
