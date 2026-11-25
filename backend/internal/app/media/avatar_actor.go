package media

import (
	"context"
	"time"

	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// NewAvatarActorAccess retains the original transaction-bound Identity authority.
// PlatformEpoch is forwarded exactly; no role or grant is reconstructed locally.
func NewAvatarActorAccess(tx *gorm.DB) mediaapp.AvatarActorAccess { return avatarActorAccess{tx: tx} }

type avatarActorAccess struct{ tx *gorm.DB }

func (a avatarActorAccess) CheckSession(ctx context.Context, actor mediaapp.Actor, clock func() time.Time) error {
	return OwningError(pgidentity.NewStore(a.tx).CheckSession(ctx, IdentityActor(actor), clock))
}
func (a avatarActorAccess) CheckPlatformSession(ctx context.Context, actor mediaapp.Actor, clock func() time.Time) error {
	return OwningError(pgidentity.NewStore(a.tx).CheckPlatformSession(ctx, IdentityActor(actor), clock))
}
