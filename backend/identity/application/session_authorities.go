package application

import (
	"context"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// SessionAuthorities is a current source snapshot, never a write admission.
// Credential epoch and these independently advancing generations may differ.
type SessionAuthorities struct {
	Account, Session writefence.Authority
}

// SessionAuthorityReader rechecks the exact live principal and both complete/open
// source heads under account/session locks. It neither touches activity nor creates
// missing source state. Every subsequent effect still requires its own admission.
type SessionAuthorityReader interface {
	ReadSessionAuthorities(context.Context, Principal, func() time.Time) (SessionAuthorities, error)
}
