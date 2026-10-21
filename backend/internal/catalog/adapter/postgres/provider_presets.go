package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/presetvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func requirePlatformCatalogAdmin(tx *gorm.DB, actors ActorAccessFactory, actor identityapp.Principal) error {
	if actors == nil || actors(tx) == nil {
		return identityapp.ErrForbidden
	}
	return actors(tx).CheckPlatformSession(tx.Statement.Context, actor, time.Now)
}
func (s *Store) requirePlatformAdmin(tx *gorm.DB, actor identityapp.Principal) error {
	return requirePlatformCatalogAdmin(tx, s.actors, actor)
}

// catalogMutation stores only nonsecret request fingerprints and safe receipts.
// A transaction-level advisory lock closes the concurrent first-use race.
func catalogMutation[T any](ctx context.Context, db *gorm.DB, actors ActorAccessFactory, actor identityapp.Principal, action string, target, requestID uuid.UUID, input any, mutate func(*gorm.DB) (T, error)) (T, error) {
	return catalogRequest(ctx, db, actors, actor, action, target, requestID, input, true, mutate)
}

func catalogRequest[T any](ctx context.Context, db *gorm.DB, actors ActorAccessFactory, actor identityapp.Principal, action string, target, requestID uuid.UUID, input any, audit bool, mutate func(*gorm.DB) (T, error)) (T, error) {
	var output T
	encoded, err := json.Marshal(input)
	if err != nil {
		return output, err
	}
	digest := sha256.Sum256(encoded)
	fingerprint := hex.EncodeToString(digest[:])
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requirePlatformCatalogAdmin(tx, actors, actor); err != nil {
			return err
		}
		lockKey := actor.ID.String() + "/" + action + "/" + target.String() + "/" + requestID.String()
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, lockKey).Error; err != nil {
			return err
		}
		var receipt struct {
			Fingerprint string
			Result      []byte
		}
		r := tx.Raw(`SELECT fingerprint,result FROM catalog.config_request WHERE actor_id=? AND action=? AND target_id=? AND request_id=?`, actor.ID, action, target, requestID).Scan(&receipt)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 1 {
			if receipt.Fingerprint != fingerprint {
				return application.ErrPresetConflict
			}
			return json.Unmarshal(receipt.Result, &output)
		}
		output, err = mutate(tx)
		if err != nil {
			return err
		}
		data, err := json.Marshal(output)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO catalog.config_request(actor_id,action,target_id,request_id,fingerprint,result) VALUES(?,?,?,?,?,?::jsonb)`, actor.ID, action, target, requestID, fingerprint, string(data)).Error; err != nil {
			return err
		}
		if !audit {
			return nil
		}
		eventID := uuid.New()
		event, err := json.Marshal(map[string]any{"event_id": eventID, "event_type": "glowloom.audit.recorded.v1", "occurred_at": time.Now().UTC(), "org_id": actor.OrgID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": eventID}, "data": map[string]any{"action": action, "object": map[string]any{"type": "catalog", "id": target}, "request_id": requestID}})
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,'glowloom.audit.recorded.v1',?,?::jsonb)`, eventID, actor.OrgID.String(), string(event)).Error
	})
	if err != nil {
		return output, fmt.Errorf("catalog %s: %w", action, mapPresetConflict(err))
	}
	return output, nil
}

func mapPresetConflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return application.ErrPresetConflict
	}
	return err
}

type presetRow struct {
	ID, VersionID               uuid.UUID
	ParentID                    *uuid.UUID
	Slug, Origin                string
	Archived, EmergencyDisabled bool
	Revision                    int64
	Version                     int
	UpdatedAt                   time.Time
	Payload                     []byte
}

func (row presetRow) preset() (domain.Preset, error) {
	p := domain.Preset{ID: row.ID, VersionID: row.VersionID, ParentID: row.ParentID, Slug: row.Slug, Origin: row.Origin, Archived: row.Archived, EmergencyDisabled: row.EmergencyDisabled, Revision: row.Revision, Version: row.Version, UpdatedAt: row.UpdatedAt}
	err := json.Unmarshal(row.Payload, &p.Document)
	return p, err
}

const presetSelect = `SELECT p.id,p.slug,p.origin,p.parent_id,p.archived,p.emergency_disabled,p.revision,p.update_time AS updated_at,
 v.id AS version_id,v.version_no AS version,v.payload FROM catalog.provider_preset p
 JOIN LATERAL(SELECT * FROM catalog.provider_preset_version WHERE preset_id=p.id ORDER BY version_no DESC LIMIT 1) v ON true`

func readPreset(tx *gorm.DB, id uuid.UUID) (domain.Preset, error) {
	var row presetRow
	r := tx.Raw(presetSelect+` WHERE p.id=?`, id).Scan(&row)
	if r.Error != nil {
		return domain.Preset{}, r.Error
	}
	if r.RowsAffected != 1 {
		return domain.Preset{}, application.ErrPresetNotFound
	}
	return row.preset()
}

// ListPresets reads an authorized bounded registry, including archived entries.
func (s *Store) ListPresets(ctx context.Context, actor identityapp.Principal) ([]domain.Preset, error) {
	items := []domain.Preset{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requirePlatformCatalogAdmin(tx, s.actors, actor); err != nil {
			return err
		}
		var rows []presetRow
		if err := tx.Raw(presetSelect + ` ORDER BY p.origin,p.slug LIMIT 200`).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			preset, err := row.preset()
			if err != nil {
				return err
			}
			items = append(items, preset)
		}
		return nil
	})
	return items, err
}

// SavePreset appends a custom version; builtins can only be copied.
func (s *Store) SavePreset(ctx context.Context, actor identityapp.Principal, input application.SavePresetInput) (domain.Preset, error) {
	return catalogMutation(ctx, s.db, s.actors, actor, "preset.save", input.ID, input.RequestID, input, func(tx *gorm.DB) (domain.Preset, error) {
		if err := presetvalidation.Validate(input.Document); err != nil {
			return domain.Preset{}, err
		}
		id := input.ID
		version := 1
		if id == uuid.Nil {
			id = uuid.New()
			if input.ParentID != nil {
				var present int
				r := tx.Raw(`SELECT 1 FROM catalog.provider_preset WHERE id=? AND NOT archived AND NOT emergency_disabled FOR SHARE`, *input.ParentID).Scan(&present)
				if r.Error != nil {
					return domain.Preset{}, r.Error
				}
				if r.RowsAffected != 1 {
					return domain.Preset{}, application.ErrPresetUnavailable
				}
			}
			if err := tx.Exec(`INSERT INTO catalog.provider_preset(id,slug,origin,parent_id) VALUES(?,?,'custom',?)`, id, input.Slug, input.ParentID).Error; err != nil {
				return domain.Preset{}, err
			}
		} else {
			r := tx.Exec(`UPDATE catalog.provider_preset SET revision=revision+1,update_time=now() WHERE id=? AND slug=? AND origin='custom' AND revision=? AND NOT archived AND NOT emergency_disabled`, id, input.Slug, input.ExpectedRevision)
			if r.Error != nil {
				return domain.Preset{}, r.Error
			}
			if r.RowsAffected != 1 {
				return domain.Preset{}, application.ErrPresetConflict
			}
			if err := tx.Raw(`SELECT COALESCE(max(version_no),0)+1 FROM catalog.provider_preset_version WHERE preset_id=?`, id).Scan(&version).Error; err != nil {
				return domain.Preset{}, err
			}
		}
		payload, err := json.Marshal(input.Document)
		if err != nil {
			return domain.Preset{}, err
		}
		if err := tx.Exec(`INSERT INTO catalog.provider_preset_version(id,preset_id,version_no,payload,content_hash) VALUES(?,?,?,?::jsonb,?)`, uuid.New(), id, version, string(payload), input.Document.Digest()).Error; err != nil {
			return domain.Preset{}, err
		}
		return readPreset(tx, id)
	})
}

// SetPresetState does not alter connections already pinned to immutable versions.
func (s *Store) SetPresetState(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input application.PresetStateInput) (domain.Preset, error) {
	return catalogMutation(ctx, s.db, s.actors, actor, "preset.state", id, input.RequestID, input, func(tx *gorm.DB) (domain.Preset, error) {
		r := tx.Exec(`UPDATE catalog.provider_preset SET archived=?,emergency_disabled=?,revision=revision+1,update_time=now() WHERE id=? AND revision=?`, input.Archived, input.EmergencyDisabled, id, input.ExpectedRevision)
		if r.Error != nil {
			return domain.Preset{}, r.Error
		}
		if r.RowsAffected != 1 {
			return domain.Preset{}, application.ErrPresetConflict
		}
		return readPreset(tx, id)
	})
}

// ConnectPreset atomically creates the disabled provider and its initial snapshot.
func (s *Store) ConnectPreset(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input application.ConnectPresetInput) (application.PresetConnection, error) {
	return catalogMutation(ctx, s.db, s.actors, actor, "preset.connect", id, input.RequestID, input, func(tx *gorm.DB) (application.PresetConnection, error) {
		var row struct{ Payload []byte }
		r := tx.Raw(`SELECT v.payload FROM catalog.provider_preset p JOIN catalog.provider_preset_version v ON v.preset_id=p.id WHERE p.id=? AND v.id=? AND NOT p.archived AND NOT p.emergency_disabled FOR SHARE OF p`, id, input.VersionID).Scan(&row)
		if r.Error != nil {
			return application.PresetConnection{}, r.Error
		}
		if r.RowsAffected != 1 {
			return application.PresetConnection{}, application.ErrPresetUnavailable
		}
		var document domain.PresetDocument
		if err := json.Unmarshal(row.Payload, &document); err != nil {
			return application.PresetConnection{}, err
		}
		if err := document.Validate(); err != nil {
			return application.PresetConnection{}, err
		}
		if err := presetvalidation.Validate(document); err != nil {
			return application.PresetConnection{}, err
		}
		if input.Name == "" {
			input.Name = document.Name + " · " + input.RequestID.String()[:8]
		}
		providerID, configID := uuid.New(), uuid.New()
		config := document.Config
		if err := tx.Exec(`INSERT INTO catalog.provider(id,key,name,adapter_key,region,status,concurrency_limit,rate_limit_per_min) VALUES(?,?,?,?,?,'disabled',?,?)`, providerID, input.Key, input.Name, config.AdapterKey, config.Region, config.ConcurrencyLimit, config.RateLimitPerMin).Error; err != nil {
			return application.PresetConnection{}, err
		}
		payload, err := json.Marshal(config)
		if err != nil {
			return application.PresetConnection{}, err
		}
		hash := sha256.Sum256(payload)
		if err := tx.Exec(`INSERT INTO catalog.provider_config_version(id,provider_id,preset_version_id,version_no,config,content_hash) VALUES(?,?,?,1,?::jsonb,?)`, configID, providerID, input.VersionID, string(payload), hex.EncodeToString(hash[:])).Error; err != nil {
			return application.PresetConnection{}, err
		}
		if err := tx.Exec(`UPDATE catalog.provider SET current_config_version_id=? WHERE id=?`, configID, providerID).Error; err != nil {
			return application.PresetConnection{}, err
		}
		modelIDs, err := s.instantiatePresetModels(ctx, tx, actor, providerID, configID, document)
		if err != nil {
			return application.PresetConnection{}, err
		}
		return application.PresetConnection{ProviderID: providerID, ModelIDs: modelIDs, Config: application.ConnectionConfig{ID: configID, ProviderID: providerID, PresetVersionID: input.VersionID, Version: 1, Config: config}}, nil
	})
}
