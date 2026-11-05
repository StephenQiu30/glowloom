package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
		var old voiceProfileRecord
		r := tx.Where(map[string]any{"project_id": pid, "actor_id": actor.ID, "request_id": in.RequestID}).Take(&old)
		if r.Error != nil && !errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return r.Error
		}
		if r.Error == nil {
			if old.RequestName != in.Name || old.AssetID != in.AssetID || old.IsDelete || old.Gender != in.Gender || old.Age != in.Age || old.Language != in.Language || old.Style != in.Style {
				return application.ErrVoiceConflict
			}
			out = old.profile()
			return nil
		}
		q := application.NewAssetQuery(NewScopedStore(tx, s.scopes))
		asset, err := q.Reference(ctx, actor, pid, in.AssetID)
		if err != nil {
			return err
		}
		if err = application.ValidateReferenceVoice(in.Name, in.Consent, asset); err != nil {
			return err
		}
		profile := voiceProfileRecord{ID: uuid.New(), ProjectID: pid, ActorID: actor.ID, RequestID: in.RequestID, Name: in.Name, RequestName: in.Name, AssetID: in.AssetID, AssetRevision: asset.Revision, ConsentAt: time.Now().UTC(), Revision: 1, Gender: in.Gender, Age: in.Age, Language: in.Language, Style: in.Style}
		if err = tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "project_id"}, {Name: "actor_id"}, {Name: "request_id"}}, DoNothing: true}).Create(&profile).Error; err != nil {
			return err
		}
		r = tx.Where(map[string]any{"project_id": pid, "actor_id": actor.ID, "request_id": in.RequestID, "is_delete": false, "request_name": in.Name, "asset_id": in.AssetID}).Take(&profile)
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return application.ErrVoiceConflict
		}
		if r.Error != nil {
			return r.Error
		}
		if profile.Gender != in.Gender || profile.Age != in.Age || profile.Language != in.Language || profile.Style != in.Style {
			return application.ErrVoiceConflict
		}
		out = profile.profile()
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
		var rows []voiceProfileRecord
		if err := tx.Where("project_id = ? AND actor_id = ? AND NOT is_delete AND id > ?", pid, actor.ID, after).Order("id").Limit(51).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 50 {
			cursor := rows[49].ID
			out.NextCursor = &cursor
			rows = rows[:50]
		}
		out.Items = make([]application.VoiceProfile, 0, len(rows))
		for _, row := range rows {
			out.Items = append(out.Items, row.profile())
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
		var row voiceProfileRecord
		r := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(map[string]any{"id": id, "project_id": pid, "actor_id": actor.ID, "is_delete": false}).Take(&row)
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return application.ErrNotFound
		}
		if r.Error != nil {
			return r.Error
		}
		if row.Revision != in.Revision {
			return application.ErrVoiceConflict
		}
		now := time.Now().UTC()
		if err := requireOne(tx.Model(&voiceProfileRecord{}).Where(map[string]any{"id": id, "project_id": pid, "actor_id": actor.ID, "revision": row.Revision, "is_delete": false}).Updates(map[string]any{"name": in.Name, "favorite": in.Favorite, "is_delete": in.Delete, "revision": gorm.Expr("revision + ?", 1), "updated_at": now}), application.ErrVoiceConflict); err != nil {
			return err
		}
		row.Name, row.Favorite, row.IsDelete, row.Revision, row.UpdatedAt = in.Name, in.Favorite, in.Delete, row.Revision+1, now
		out = row.profile()
		return nil
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
		if err := tx.Model(&voiceFavoriteRecord{}).Where(map[string]any{"project_id": pid, "actor_id": actor.ID, "favorite": true}).Pluck("speaker", &keys).Error; err != nil {
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
		record := voiceFavoriteRecord{ProjectID: pid, ActorID: actor.ID, Speaker: key, Favorite: favorite}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "project_id"}, {Name: "actor_id"}, {Name: "speaker"}}, DoUpdates: clause.Assignments(map[string]any{"favorite": favorite})}).Create(&record).Error
	})
}
