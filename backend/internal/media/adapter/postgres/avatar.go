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

// AvatarAccess validates the dedicated avatar output under profile transaction locks.
type AvatarAccess struct{ db *gorm.DB }

// NewAvatarAccess is injected by the app; identity never imports media adapters.
func NewAvatarAccess(db *gorm.DB) *AvatarAccess { return &AvatarAccess{db: db} }
func avatarEligible(tx *gorm.DB, actor, id uuid.UUID) (assetRow, error) {
	var row assetRow
	err := eligibleAvatarQuery(tx, id).Where("personal_actor_id = ?", actor).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, identityapp.ErrAvatarUnavailable
	}
	if err != nil {
		return row, err
	}
	if err := row.loadLocation(tx); err != nil {
		return row, err
	}
	if row.domain().Validate() != nil {
		return row, identityapp.ErrAvatarUnavailable
	}
	return row, nil
}

func eligibleAvatarQuery(tx *gorm.DB, id uuid.UUID) *gorm.DB {
	return tx.Model(&assetRow{}).Clauses(clause.Locking{Strength: "SHARE"}).
		Where("id = ? AND project_id IS NULL AND kind = ? AND origin = ? AND status = ? AND moderation_status = ? AND NOT is_delete", id,
			string(domain.KindImage), string(domain.OriginUpload), string(domain.StatusReady), string(domain.ModerationPassed)).
		Where("mime_type = ? AND codec = ? AND byte_size BETWEEN ? AND ? AND width BETWEEN ? AND ? AND height BETWEEN ? AND ? AND duration_ms IS NULL",
			"image/png", "png", 1, application.AvatarOutputMaxBytes, 1, application.AvatarOutputMaxDimension, 1, application.AvatarOutputMaxDimension).
		Where("moderation_detail->'avatar'->>'method' = ? AND moderation_detail->'avatar'->>'version' = ?", "server_reencode_png", "1")
}

type avatarUserRecord struct {
	ID            uuid.UUID
	OrgID         uuid.UUID
	AvatarAssetID *uuid.UUID
}

func (avatarUserRecord) TableName() string { return `identity."user"` }

type avatarAssetRecord struct {
	ID               uuid.UUID
	PersonalOrgID    uuid.UUID
	PersonalActorID  uuid.UUID
	Kind             string
	Origin           string
	Status           string
	ObjectKey        string
	ObjectLocationID uuid.UUID
	FileName         string
	MimeType         string
	ByteSize         int64
	SHA256           *string
	Width            *int32
	Height           *int32
	Codec            *string
	ModerationStatus string
	ModerationDetail jsonDocument
	Revision         int64
	CreateTime       time.Time
	UpdateTime       time.Time
}

func (avatarAssetRecord) TableName() string { return "media.media_asset" }

type avatarSecurityEventRecord struct {
	ID, ActorID, TargetUserID uuid.UUID
	ScopeKind, Action, Result string
	CreatedAt                 time.Time
}

func (avatarSecurityEventRecord) TableName() string { return "identity.security_event" }

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

func (s *AvatarStore) authorize(ctx context.Context, tx *gorm.DB, a identityapp.Principal, clock func() time.Time) error {
	if s.actors == nil {
		return identityapp.ErrSessionInvalid
	}
	access := s.actors(tx)
	if access == nil {
		return identityapp.ErrSessionInvalid
	}
	if err := access.CheckSession(ctx, a, clock); err != nil {
		return err
	}
	// Storage ownership is a media concern; never accept a client-selected org.
	var owner avatarUserRecord
	if err := tx.Model(&avatarUserRecord{}).Select("org_id").Where("id = ?", a.ID).Take(&owner).Error; err != nil {
		return err
	}
	if owner.OrgID == uuid.Nil || owner.OrgID != a.OrgID {
		return identityapp.ErrForbidden
	}
	return nil
}

// AuthorizeAvatar checks current identity and session before decoding.
func (s *AvatarStore) AuthorizeAvatar(ctx context.Context, a identityapp.Principal, clock func() time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.authorize(ctx, tx, a, clock) })
}

// ReserveAvatarLocation binds the private output to the current route in the
// same authorized transaction that creates its durable storage work intent.
func (s *AvatarStore) ReserveAvatarLocation(ctx context.Context, a identityapp.Principal, id uuid.UUID, key string, clock func() time.Time) (domain.ObjectLocation, error) {
	var location domain.ObjectLocation
	if s == nil || s.db == nil || id == uuid.Nil || clock == nil {
		return location, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, clock); err != nil {
			return err
		}
		routes := NewStorageRoutes(tx)
		intent, err := routes.PinStorageWork(ctx, application.StorageWorkOwner{Kind: application.StorageWorkKindAvatar, OwnerID: a.ID, WorkID: id})
		if err != nil {
			return err
		}
		location, err = routes.ReserveWorkLocation(ctx, intent.ID, key)
		return err
	})
	return location, err
}

func avatarStorageOwner(actor, asset uuid.UUID) application.StorageWorkOwner {
	return application.StorageWorkOwner{Kind: application.StorageWorkKindAvatar, OwnerID: actor, WorkID: asset}
}

// BeginAvatarWrite persists the bounded provider-write deadline before SDK I/O.
func (s *AvatarStore) BeginAvatarWrite(ctx context.Context, a identityapp.Principal, id uuid.UUID, location domain.ObjectLocation) error {
	routes := NewStorageRoutes(s.db)
	intent, err := routes.ReadStorageWork(ctx, avatarStorageOwner(a.ID, id))
	if err != nil {
		return err
	}
	return routes.BeginWorkWrite(ctx, intent.ID, location)
}

// ConfirmAvatarLocation records provider metadata only after the exact bytes verify.
func (s *AvatarStore) ConfirmAvatarLocation(ctx context.Context, a identityapp.Principal, id uuid.UUID, location domain.ObjectLocation, info application.ObjectInfo) error {
	routes := NewStorageRoutes(s.db)
	intent, err := routes.ReadStorageWork(ctx, avatarStorageOwner(a.ID, id))
	if err != nil {
		return err
	}
	return routes.ConfirmWorkLocation(ctx, intent.ID, location, info)
}

// CancelAvatarObject retires an uncommitted intent and grants deletion only
// after the storage ledger's in-flight write drain has completed.
func (s *AvatarStore) CancelAvatarObject(ctx context.Context, a identityapp.Principal, id uuid.UUID, location domain.ObjectLocation) (bool, error) {
	var removable bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		routes := NewStorageRoutes(tx)
		intent, err := routes.ReadStorageWork(ctx, avatarStorageOwner(a.ID, id))
		if err != nil {
			return err
		}
		if intent.State == application.StorageWorkAccepted {
			return application.ErrStorageRouteConflict
		}
		if err := routes.CancelStorageWork(ctx, intent.ID); err != nil {
			return err
		}
		removable, err = routes.BeginWorkRemoval(ctx, intent.ID, location)
		return err
	})
	return removable, err
}

// ConfirmAvatarRemoval retires the exact location after provider deletion.
func (s *AvatarStore) ConfirmAvatarRemoval(ctx context.Context, a identityapp.Principal, id uuid.UUID, location domain.ObjectLocation) error {
	routes := NewStorageRoutes(s.db)
	intent, err := routes.ReadStorageWork(ctx, avatarStorageOwner(a.ID, id))
	if err != nil {
		return err
	}
	return routes.ConfirmWorkRemoval(ctx, intent.ID, location)
}

// CommitAvatar repeats current authorization when publishing immutable metadata.
func (s *AvatarStore) CommitAvatar(ctx context.Context, a identityapp.Principal, m domain.MediaAsset, clock func() time.Time) error {
	if m.Validate() != nil || m.Personal == nil || m.Personal.ActorID != a.ID || m.Personal.OrgID != a.OrgID || m.ProjectID != uuid.Nil || m.Kind != domain.KindImage || m.MimeType != "image/png" {
		return application.ErrInvalidUpload
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, clock); err != nil {
			return err
		}
		if m.ObjectLocation != nil {
			intent, err := NewStorageRoutes(tx).ReadStorageWork(ctx, avatarStorageOwner(a.ID, m.ID))
			if err != nil || intent.State != application.StorageWorkOpen || intent.ProfileID != m.ObjectLocation.Ref.ProfileID || intent.Bucket != m.ObjectLocation.Ref.Bucket {
				return application.ErrStorageRouteConflict
			}
			if err := NewStorageRoutes(tx).AcceptStorageWork(ctx, intent.ID, []uuid.UUID{m.ObjectLocation.ID}); err != nil {
				return err
			}
		} else {
			return application.ErrStorageNotReady
		}
		asset := avatarAssetRecord{ID: m.ID, PersonalOrgID: a.OrgID, PersonalActorID: a.ID, Kind: string(m.Kind), Origin: string(m.Origin), Status: string(m.Status), ObjectKey: m.ObjectKey, ObjectLocationID: m.ObjectLocation.ID, FileName: m.FileName, MimeType: m.MimeType, ByteSize: m.ByteSize, SHA256: m.SHA256, Width: m.Width, Height: m.Height, Codec: m.Codec, ModerationStatus: string(m.ModerationStatus), ModerationDetail: jsonDocument(m.ModerationDetail), Revision: m.Revision, CreateTime: m.CreateTime, UpdateTime: m.UpdateTime}
		if err := tx.Create(&asset).Error; err != nil {
			return err
		}
		event := avatarSecurityEventRecord{ID: uuid.New(), ActorID: a.ID, TargetUserID: a.ID, ScopeKind: "self", Action: "avatar.uploaded", Result: "success", CreatedAt: clock().UTC()}
		return tx.Create(&event).Error
	})
}

// AvatarExists preserves any durable row when object cleanup follows an uncertain commit.
func (s *AvatarStore) AvatarExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&assetRow{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

// AvatarTransaction keeps identity and avatar locks until preview signing finishes.
func (s *AvatarStore) AvatarTransaction(ctx context.Context, a identityapp.Principal, clock func() time.Time, run func(domain.MediaAsset) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, clock); err != nil {
			return err
		}
		var binding avatarUserRecord
		if err := tx.Model(&avatarUserRecord{}).Select("avatar_asset_id").Where("id = ?", a.ID).Take(&binding).Error; err != nil {
			return err
		}
		if binding.AvatarAssetID == nil || *binding.AvatarAssetID == uuid.Nil {
			return application.ErrNotFound
		}
		row, err := avatarEligible(tx, a.ID, *binding.AvatarAssetID)
		if err != nil {
			return err
		}
		if err := row.loadLocation(tx); err != nil {
			return err
		}
		return run(row.domain())
	})
}
