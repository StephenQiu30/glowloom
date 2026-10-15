package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// AvatarAccess validates the dedicated avatar output under profile transaction locks.
type AvatarAccess struct{ db *gorm.DB }

// NewAvatarAccess is injected by the app; identity never imports media adapters.
func NewAvatarAccess(db *gorm.DB) *AvatarAccess { return &AvatarAccess{db: db} }
func avatarEligible(tx *gorm.DB, actor, id uuid.UUID) (assetRow, error) {
	var row assetRow
	r := tx.Raw(`SELECT * FROM media.media_asset WHERE id=? AND personal_actor_id=? AND project_id IS NULL AND kind='image' AND origin='upload' AND status='ready' AND moderation_status='passed' AND NOT is_delete AND mime_type='image/png' AND codec='png' AND byte_size BETWEEN 1 AND ? AND width BETWEEN 1 AND ? AND height BETWEEN 1 AND ? AND duration_ms IS NULL AND moderation_detail->'avatar'->>'method'='server_reencode_png' AND moderation_detail->'avatar'->>'version'='1' FOR SHARE`, id, actor, application.AvatarOutputMaxBytes, application.AvatarOutputMaxDimension, application.AvatarOutputMaxDimension).Scan(&row)
	if r.Error != nil {
		return row, r.Error
	}
	if r.RowsAffected != 1 || row.domain().Validate() != nil {
		return row, identityapp.ErrAvatarUnavailable
	}
	return row, nil
}

// Validate hides arbitrary, deleted, unprocessed and other-account assets.
func (s *AvatarAccess) Validate(ctx context.Context, actor, id uuid.UUID) error {
	_, err := avatarEligible(s.db.WithContext(ctx), actor, id)
	return err
}

// AvatarActorAccessFactory retains identity locks within the avatar transaction.
type AvatarActorAccessFactory func(*gorm.DB) application.AvatarActorAccess

// AvatarStore publishes avatar metadata using explicitly injected identity authority.
type AvatarStore struct {
	db     *gorm.DB
	actors AvatarActorAccessFactory
}

// NewAvatarStore binds the live identity checker to each store transaction.
func NewAvatarStore(db *gorm.DB, actors AvatarActorAccessFactory) *AvatarStore {
	return &AvatarStore{db: db, actors: actors}
}

func (s *AvatarStore) authorize(ctx context.Context, tx *gorm.DB, a identityapp.Principal, now time.Time) error {
	if s.actors == nil {
		return identityapp.ErrSessionInvalid
	}
	access := s.actors(tx)
	if access == nil {
		return identityapp.ErrSessionInvalid
	}
	if err := access.CheckSession(ctx, a, now); err != nil {
		return err
	}
	// Storage ownership is a media concern; never accept a client-selected org.
	var owner struct{ OrgID uuid.UUID }
	if err := tx.Raw(`SELECT org_id FROM identity."user" WHERE id=?`, a.ID).Scan(&owner).Error; err != nil {
		return err
	}
	if owner.OrgID == uuid.Nil || owner.OrgID != a.OrgID {
		return identityapp.ErrForbidden
	}
	return nil
}

// AuthorizeAvatar checks current identity and session before decoding.
func (s *AvatarStore) AuthorizeAvatar(ctx context.Context, a identityapp.Principal, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.authorize(ctx, tx, a, now) })
}

// CommitAvatar repeats current authorization when publishing immutable metadata.
func (s *AvatarStore) CommitAvatar(ctx context.Context, a identityapp.Principal, m domain.MediaAsset, now time.Time) error {
	if m.Validate() != nil || m.Personal == nil || m.Personal.ActorID != a.ID || m.Personal.OrgID != a.OrgID || m.ProjectID != uuid.Nil || m.Kind != domain.KindImage || m.MimeType != "image/png" {
		return application.ErrInvalidUpload
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, now); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO media.media_asset(id,personal_org_id,personal_actor_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,width,height,codec,moderation_status,moderation_detail,revision,create_time,update_time) VALUES(?,?,?,'image','upload','ready',?,'avatar.png','image/png',?,?,?,?,'png','passed',?::jsonb,1,?,?)`, m.ID, a.OrgID, a.ID, m.ObjectKey, m.ByteSize, m.SHA256, m.Width, m.Height, string(m.ModerationDetail), m.CreateTime, m.UpdateTime).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO identity.security_event(id,actor_id,target_user_id,scope_kind,action,result,created_at) VALUES(?,?,?,'self','avatar.uploaded','success',?)`, uuid.New(), a.ID, a.ID, now).Error
	})
}

// AvatarExists preserves any durable row when object cleanup follows an uncertain commit.
func (s *AvatarStore) AvatarExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM media.media_asset WHERE id=?)`, id).Scan(&exists).Error
	return exists, err
}

// AvatarTransaction keeps identity and avatar locks until preview signing finishes.
func (s *AvatarStore) AvatarTransaction(ctx context.Context, a identityapp.Principal, now time.Time, run func(domain.MediaAsset) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, now); err != nil {
			return err
		}
		var binding struct{ ID *uuid.UUID }
		if err := tx.Raw(`SELECT avatar_asset_id AS id FROM identity."user" WHERE id=?`, a.ID).Scan(&binding).Error; err != nil {
			return err
		}
		if binding.ID == nil || *binding.ID == uuid.Nil {
			return application.ErrNotFound
		}
		row, err := avatarEligible(tx, a.ID, *binding.ID)
		if err != nil {
			return err
		}
		return run(row.domain())
	})
}
