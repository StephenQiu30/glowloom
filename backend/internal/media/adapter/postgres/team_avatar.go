package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// TeamAvatarAccess exposes only normalized avatar media to the workspace consumer.
type TeamAvatarAccess struct{ db *gorm.DB }

// NewTeamAvatarAccess is injected into the caller's authorized team transaction.
func NewTeamAvatarAccess(db *gorm.DB) *TeamAvatarAccess { return &TeamAvatarAccess{db: db} }

// Validate reuses the approved current-uploader binding validator.
func (s *TeamAvatarAccess) Validate(ctx context.Context, actor, id uuid.UUID) error {
	return NewAvatarAccess(s.db).Validate(ctx, actor, id)
}

// ReadBound is called only after workspace selects the current team/member avatar binding.
func (s *TeamAvatarAccess) ReadBound(ctx context.Context, id uuid.UUID) (workspace.TeamAvatarObject, error) {
	var row assetRow
	err := eligibleAvatarQuery(s.db.WithContext(ctx), id).Where("personal_actor_id IS NOT NULL").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return workspace.TeamAvatarObject{}, identity.ErrAvatarUnavailable
	}
	if err != nil {
		return workspace.TeamAvatarObject{}, err
	}
	if err := row.loadLocation(s.db.WithContext(ctx)); err != nil {
		return workspace.TeamAvatarObject{}, err
	}
	asset := row.domain()
	if asset.Validate() != nil || asset.ObjectLocation == nil || !asset.ObjectLocation.Readable(asset.ObjectKey) {
		return workspace.TeamAvatarObject{}, identity.ErrAvatarUnavailable
	}
	return workspace.TeamAvatarObject{ID: row.ID, ProfileID: asset.ObjectLocation.Ref.ProfileID, Bucket: asset.ObjectLocation.Ref.Bucket, ObjectKey: row.ObjectKey}, nil
}
