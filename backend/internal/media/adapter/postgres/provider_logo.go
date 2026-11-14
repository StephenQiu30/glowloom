package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// NewProviderLogoStore reuses routed image writes with platform-only upload authority.
func NewProviderLogoStore(db *gorm.DB, actors AvatarActorAccessFactory) *AvatarStore {
	s := NewAvatarStore(db, actors)
	s.providerLogo = true
	return s
}

func uploadEventAction(logo bool) string {
	if logo {
		return "provider_logo.uploaded"
	}
	return "avatar.uploaded"
}

// ProviderLogoTransaction permits owner previews and authenticated reads of bound logos.
// It rechecks live sessions, purpose and bindings on both sides of URL signing.
func (s *AvatarStore) ProviderLogoTransaction(ctx context.Context, actor identityapp.Principal, id uuid.UUID, clock func() time.Time, run func(domain.MediaAsset) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := NewAvatarStore(tx, s.actors).authorize(ctx, tx, actor, clock); err != nil {
			return err
		}
		var row assetRow
		err := tx.Model(&assetRow{}).Clauses(clause.Locking{Strength: "SHARE"}).
			Where("id=? AND project_id IS NULL AND NOT is_delete AND kind='image' AND origin='upload' AND status='ready' AND moderation_status='passed' AND mime_type='image/png' AND codec='png'", id).
			Where("byte_size BETWEEN 1 AND ? AND width BETWEEN 1 AND ? AND height BETWEEN 1 AND ? AND duration_ms IS NULL", application.AvatarOutputMaxBytes, application.AvatarOutputMaxDimension, application.AvatarOutputMaxDimension).
			Where("moderation_detail->'provider_logo'->>'method'='server_reencode_png' AND moderation_detail->'provider_logo'->>'version'='1'").
			Where("(personal_actor_id=? OR id IN (SELECT logo_asset_id FROM catalog.provider WHERE logo_asset_id IS NOT NULL AND NOT is_delete))", actor.ID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return application.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := row.loadLocation(tx); err != nil {
			return err
		}
		asset := row.domain()
		if asset.Validate() != nil {
			return application.ErrNotFound
		}
		return run(asset)
	})
}
