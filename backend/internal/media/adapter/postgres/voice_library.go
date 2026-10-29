package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func (s *Store) voiceAuthority(tx *gorm.DB, actor identity.Principal, pid uuid.UUID, write bool) error {
	if actor.ID == uuid.Nil || pid == uuid.Nil {
		return identity.ErrForbidden
	}
	if err := s.requireCurrentActor(tx, actor); err != nil {
		return err
	}
	return s.requireProject(tx, actor, pid, write)
}

// CreateVoice atomically binds a currently usable project audio asset to its owner.
func (s *Store) CreateVoice(ctx context.Context, actor identity.Principal, pid uuid.UUID, in application.VoiceCreate) (application.VoiceProfile, error) {
	var out application.VoiceProfile
	if err := in.Normalize(); err != nil {
		return out, err
	}
	if in.RequestID == uuid.Nil || !application.ValidVoiceName(in.Name) || !in.Consent {
		return out, application.ErrInvalidQuery
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.voiceAuthority(tx, actor, pid, true); err != nil {
			return err
		}
		// Read the original immutable request before validating a new source. A lost
		// receipt remains recoverable even if the source has since become unavailable.
		var old struct {
			application.VoiceProfile
			RequestName string
			Deleted     bool
		}
		r := tx.Raw(`SELECT *,is_delete AS deleted FROM media.voice_profile WHERE project_id=? AND actor_id=? AND request_id=?`, pid, actor.ID, in.RequestID).Scan(&old)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 1 {
			if old.RequestName != in.Name || old.AssetID != in.AssetID || old.Deleted || old.VoiceTags != in.VoiceTags {
				return application.ErrVoiceConflict
			}
			out = old.VoiceProfile
			return nil
		}
		q := application.NewAssetQuery(NewScopedStore(tx, s.scopes), nil)
		asset, err := q.Reference(ctx, actor, pid, in.AssetID)
		if err != nil {
			return err
		}
		if err = application.ValidateReferenceVoice(in.Name, in.Consent, asset); err != nil {
			return err
		}
		if err = tx.Exec(`INSERT INTO media.voice_profile(id,project_id,actor_id,request_id,name,request_name,asset_id,asset_revision,consent_at,gender,age,language,style) VALUES(?,?,?,?,?,?,?,?,now(),?,?,?,?) ON CONFLICT(project_id,actor_id,request_id) DO NOTHING`, uuid.New(), pid, actor.ID, in.RequestID, in.Name, in.Name, in.AssetID, asset.Revision, in.Gender, in.Age, in.Language, in.Style).Error; err != nil {
			return err
		}
		r = tx.Raw(`SELECT * FROM media.voice_profile WHERE project_id=? AND actor_id=? AND request_id=? AND NOT is_delete AND request_name=? AND asset_id=?`, pid, actor.ID, in.RequestID, in.Name, in.AssetID).Scan(&out)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 || out.VoiceTags != in.VoiceTags {
			return application.ErrVoiceConflict
		}
		return nil
	})
	return out, err
}

// ListVoices paginates private saved references independently of model availability.
func (s *Store) ListVoices(ctx context.Context, actor identity.Principal, pid, after uuid.UUID) (application.VoicePage, error) {
	out := application.VoicePage{Items: []application.VoiceProfile{}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.voiceAuthority(tx, actor, pid, false); err != nil {
			return err
		}
		query := `SELECT * FROM media.voice_profile WHERE project_id=? AND actor_id=? AND NOT is_delete AND id>? ORDER BY id LIMIT 51`
		if err := tx.Raw(query, pid, actor.ID, after).Scan(&out.Items).Error; err != nil {
			return err
		}
		if len(out.Items) > 50 {
			cursor := out.Items[49].ID
			out.NextCursor = &cursor
			out.Items = out.Items[:50]
		}
		return nil
	})
	return out, err
}

// UpdateVoice serializes edits; deletion retains the idempotency receipt and source.
func (s *Store) UpdateVoice(ctx context.Context, actor identity.Principal, pid, id uuid.UUID, in application.VoiceUpdate) (application.VoiceProfile, error) {
	var out application.VoiceProfile
	if !application.ValidVoiceName(in.Name) || in.Revision < 1 {
		return out, application.ErrInvalidQuery
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.voiceAuthority(tx, actor, pid, true); err != nil {
			return err
		}
		r := tx.Raw(`SELECT * FROM media.voice_profile WHERE id=? AND project_id=? AND actor_id=? AND NOT is_delete FOR UPDATE`, id, pid, actor.ID).Scan(&out)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrNotFound
		}
		if out.Revision != in.Revision {
			return application.ErrVoiceConflict
		}
		return tx.Raw(`UPDATE media.voice_profile SET name=?,favorite=?,is_delete=?,revision=revision+1,updated_at=now() WHERE id=? RETURNING *`, in.Name, in.Favorite, in.Delete, id).Scan(&out).Error
	})
	return out, err
}

// ListSystemVoices combines the public catalog with private persisted favorites.
func (s *Store) ListSystemVoices(ctx context.Context, actor identity.Principal, pid uuid.UUID) ([]application.SystemVoice, error) {
	out := application.SystemVoices()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.voiceAuthority(tx, actor, pid, false); err != nil {
			return err
		}
		var keys []string
		if err := tx.Raw(`SELECT speaker FROM media.voice_favorite WHERE project_id=? AND actor_id=? AND favorite`, pid, actor.ID).Scan(&keys).Error; err != nil {
			return err
		}
		for i := range out {
			for _, key := range keys {
				if out[i].ID == key {
					out[i].Favorite = true
				}
			}
		}
		return nil
	})
	return out, err
}

// FavoriteSystemVoice uses an absolute value so unknown receipts can be replayed.
func (s *Store) FavoriteSystemVoice(ctx context.Context, actor identity.Principal, pid uuid.UUID, key string, favorite bool) error {
	valid := false
	for _, v := range application.SystemVoices() {
		if v.ID == key {
			valid = true
		}
	}
	if !valid {
		return application.ErrInvalidQuery
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.voiceAuthority(tx, actor, pid, true); err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO media.voice_favorite(project_id,actor_id,speaker,favorite) VALUES(?,?,?,?) ON CONFLICT(project_id,actor_id,speaker) DO UPDATE SET favorite=excluded.favorite`, pid, actor.ID, key, favorite).Error
	})
}
