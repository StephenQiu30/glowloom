package prompt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	promptapp "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// SessionAccess binds legacy exact-session checks to the caller's content Tx.
// It belongs only to the retained monolith; an independent Creation root uses its
// authenticated Identity contract and local final-write fence instead.
func SessionAccess(tx *gorm.DB) promptapp.ActorAccess {
	return promptSessionAccess{store: pgidentity.NewStore(tx)}
}

type promptSessionAccess struct {
	store interface {
		CheckSession(context.Context, identity.Principal, func() time.Time) error
	}
}

func (a promptSessionAccess) CheckSession(ctx context.Context, actor promptapp.Actor, clock func() time.Time) error {
	return AccessError(a.store.CheckSession(ctx, legacyActor(actor), clock))
}
func legacyActor(actor promptapp.Actor) identity.Principal {
	return identity.Principal{ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
}

// AccessError translates policy classifications at the legacy assembly boundary.
// Prompt implementation packages never import Identity's error or Principal types.
func AccessError(err error) error {
	switch {
	case errors.Is(err, identity.ErrSessionInvalid):
		return fmt.Errorf("%w: %w", promptapp.ErrSessionInvalid, err)
	case errors.Is(err, identity.ErrForbidden):
		return fmt.Errorf("%w: %w", promptapp.ErrForbidden, err)
	default:
		return err
	}
}
