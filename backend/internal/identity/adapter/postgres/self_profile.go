package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// AvatarAccessFactory binds avatar ownership checks to the profile transaction.
type AvatarAccessFactory func(*gorm.DB) application.AvatarAccess

// SelfProfileStore coordinates current-identity checks and profile updates.
type SelfProfileStore struct {
	db      *gorm.DB
	avatars AvatarAccessFactory
}

// NewSelfProfileStore constructs SelfProfileStore with explicit caller-supplied dependencies.
func NewSelfProfileStore(db *gorm.DB, avatars AvatarAccessFactory) *SelfProfileStore {
	return &SelfProfileStore{db: db, avatars: avatars}
}

type selfProfileTx struct {
	db      *gorm.DB
	user    domain.User
	avatars application.AvatarAccess
}

func (t *selfProfileTx) Account() domain.User { return t.user }

// lockSelfActor locks user before session and rejects stale principals.
// Sample the injected clock only after the locks: a waiting request may observe
// activity committed by a later request, or expire while it waits.
func lockSelfActor(tx *gorm.DB, actor application.Principal, clock func() time.Time) (domain.User, error) {
	if actor.ID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1 {
		return domain.User{}, application.ErrSessionInvalid
	}
	var settings bool
	if err := tx.Raw(`SELECT singleton FROM identity.platform_settings WHERE singleton FOR SHARE`).Scan(&settings).Error; err != nil {
		return domain.User{}, err
	}
	if !settings {
		return domain.User{}, application.ErrSessionInvalid
	}
	user, err := (&authTx{db: tx}).AccountByID(actor.ID)
	if errors.Is(err, domain.ErrUserNotFound) {
		return domain.User{}, application.ErrSessionInvalid
	}
	if err != nil {
		return domain.User{}, err
	}
	var session domain.AuthSession
	r := tx.Raw(`SELECT id,account_id,credential_revision,created_at,last_active_at,absolute_expires_at,persistent,protocol_version,idle_timeout_seconds,refresh_generation,revoked_at,reauth_at,reauth_epoch FROM identity.user_session WHERE id=? AND account_id=? FOR UPDATE`, actor.SessionID, actor.ID).Scan(&session)
	if r.Error != nil {
		return domain.User{}, fmt.Errorf("lock self session: %w", r.Error)
	}
	now := clock().UTC()
	if r.RowsAffected != 1 || !session.Valid(user, now) || user.EmailVerifiedAt.IsZero() || user.SessionEpoch != actor.CredentialEpoch {
		return domain.User{}, application.ErrSessionInvalid
	}
	return user, nil
}

// ProfileTransaction keeps identity checks and profile updates under the same locks.
func (s *SelfProfileStore) ProfileTransaction(ctx context.Context, actor application.Principal, clock func() time.Time, run func(application.SelfProfileTx) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, err := lockSelfActor(tx, actor, clock)
		if err != nil {
			return err
		}
		state := &selfProfileTx{db: tx, user: u}
		if s.avatars != nil {
			state.avatars = s.avatars(tx)
		}
		return run(state)
	})
}
func (t *selfProfileTx) ValidateAvatar(ctx context.Context, id uuid.UUID) error {
	if t.avatars == nil {
		return application.ErrAvatarUnavailable
	}
	return t.avatars.Validate(ctx, t.user.ID, id)
}
func (t *selfProfileTx) SaveProfile(u domain.User, expected int64, now time.Time) error {
	result := t.db.Exec(`UPDATE identity."user" SET display_name=?,avatar_asset_id=?,revision=?,update_time=? WHERE id=? AND revision=?`, u.DisplayName, u.AvatarAssetID, u.Revision, now, u.ID, expected)
	if result.Error != nil {
		return fmt.Errorf("save self profile: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return domain.ErrRevisionConflict
	}
	return appendSelfEvent(t.db, u.ID, "profile.updated", now)
}
func (t *selfProfileTx) SavePreferences(u domain.User, expected int64, now time.Time) error {
	result := t.db.Exec(`UPDATE identity."user" SET theme=?,prompt_suffix=?,revision=?,update_time=? WHERE id=? AND revision=?`, u.Theme, u.PromptSuffix, u.Revision, now, u.ID, expected)
	if result.Error != nil {
		return fmt.Errorf("save preferences: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return domain.ErrRevisionConflict
	}
	return appendSelfEvent(t.db, u.ID, "preferences.updated", now)
}
func appendSelfEvent(tx *gorm.DB, user uuid.UUID, action string, now time.Time) error {
	return tx.Exec(`INSERT INTO identity.security_event(id,actor_id,target_user_id,scope_kind,action,result,created_at) VALUES(?,?,?,'self',?,'success',?)`, uuid.New(), user, user, action, now).Error
}
