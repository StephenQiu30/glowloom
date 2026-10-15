package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
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
	r := s.db.WithContext(ctx).Raw(`SELECT * FROM media.media_asset WHERE id=? AND personal_actor_id IS NOT NULL AND project_id IS NULL AND kind='image' AND origin='upload' AND status='ready' AND moderation_status='passed' AND NOT is_delete AND mime_type='image/png' AND codec='png' AND byte_size BETWEEN 1 AND ? AND width BETWEEN 1 AND ? AND height BETWEEN 1 AND ? AND duration_ms IS NULL AND moderation_detail->'avatar'->>'method'='server_reencode_png' AND moderation_detail->'avatar'->>'version'='1' FOR SHARE`, id, application.AvatarOutputMaxBytes, application.AvatarOutputMaxDimension, application.AvatarOutputMaxDimension).Scan(&row)
	if r.Error != nil {
		return workspace.TeamAvatarObject{}, r.Error
	}
	if r.RowsAffected != 1 || row.domain().Validate() != nil {
		return workspace.TeamAvatarObject{}, identity.ErrAvatarUnavailable
	}
	return workspace.TeamAvatarObject{ID: row.ID, ObjectKey: row.ObjectKey}, nil
}
