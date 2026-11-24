package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strings"

	catalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

// PublishModel adds an immutable version and atomically replaces its live alias.
func (s *Store) PublishModel(ctx context.Context, m domain.Model) error {
	if m.Validate() != nil {
		return domain.ErrInvalid
	}
	raw, _ := json.Marshal(m)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		// Serializes initial publication and replacement for the same alias.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "ai-model/"+m.Name).Error; err != nil {
			return err
		}
		var prior struct{ Document []byte }
		r := tx.Raw("SELECT document FROM ai.model WHERE id=?", m.ID).Scan(&prior)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected > 0 {
			if domain.Digest(prior.Document) != domain.Digest(raw) {
				var original domain.Model
				if json.Unmarshal(prior.Document, &original) != nil || domain.Digest(mustJSON(original)) != domain.Digest(raw) {
					return domain.ErrConflict
				}
			}
			return nil
		}
		if err := tx.Exec("UPDATE ai.model SET enabled=false WHERE name=? AND document->>'protocol'=? AND enabled", m.Name, m.Protocol).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO ai.model(id,name,document,enabled) VALUES(?,?,?::jsonb,true)", m.ID, m.Name, string(raw)).Error
	})
}

// PublishRoute imports an already accepted route. Administrator onboarding uses the staged path.
func (s *Store) PublishRoute(ctx context.Context, route domain.Route, ciphertext []byte, keyID string, concurrency, rpm int) error {
	if keyID == "" {
		return domain.ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		var m struct{ Document []byte }
		r := tx.Raw("SELECT document FROM ai.model WHERE id=? AND enabled FOR SHARE", route.ModelID).Scan(&m)
		if r.Error != nil {
			return r.Error
		}
		var model domain.Model
		if r.RowsAffected != 1 || json.Unmarshal(m.Document, &model) != nil {
			return domain.ErrInvalid
		}
		if err := validateRoute(model, route, concurrency, rpm); err != nil {
			return err
		}
		return insertRoute(tx, route, ciphertext, keyID, concurrency, rpm, true)
	})
}

func validateRoute(model domain.Model, route domain.Route, concurrency, rpm int) error {
	if model.Protocol != route.Protocol || route.KeyID != "" || (route.Protocol != "media" && len(route.DriverConfig) != 0) {
		return domain.ErrInvalid
	}
	u, err := url.Parse(route.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(route.BaseURL, "\r\n\\") || route.ID == uuid.Nil || route.ModelID == uuid.Nil || route.Supplier == uuid.Nil || route.Credential == uuid.Nil || route.Group == "" || len(route.Group) > 160 || route.UpstreamModel == "" || !domain.Protocol(route.Protocol) || route.Purchase.Validate() != nil || route.Priority < 0 || concurrency < 1 || concurrency > 1000 || rpm < 1 {
		return domain.ErrInvalid
	}
	if route.Protocol == domain.EmbeddingProtocol && (route.BaseURL != domain.EmbeddingBaseURL || route.UpstreamModel != domain.EmbeddingModel || route.Purchase.Input <= 0 || route.Purchase.Request != 0 || route.Purchase.Output != 0 || route.Purchase.CacheRead != 0 || route.Purchase.CacheWrite != 0) {
		return domain.ErrInvalid
	}
	if route.Protocol != "local" && route.Purchase.ExplicitFree {
		return domain.ErrInvalid
	}
	if route.Protocol == "media" {
		var config catalog.ProviderConfig
		if decodeRouteDriver(route.DriverConfig, &config) != nil || config.Validate() != nil || config.BaseURL != route.BaseURL || !slices.Contains(config.Capabilities, strings.TrimSuffix(model.Capability, ".generate")) || !model.AcceptsMediaPrice(route.Purchase) ||
			model.VideoMetering != nil && config.AdapterKey != "volcengine_ark" {
			return domain.ErrInvalid
		}
		if config.AdapterKey == "volcengine_ark" && strings.HasPrefix(route.UpstreamModel, "doubao-seedance-2-5-") && slices.Contains(model.Modes, "image2video") {
			for _, name := range []string{"ratio", "aspect_ratio"} {
				if parameter, ok := model.Parameters[name]; ok {
					if len(parameter.Enum) == 0 {
						return domain.ErrInvalid
					}
					for _, value := range parameter.Enum {
						var ratio string
						if json.Unmarshal(value, &ratio) != nil || ratio != "adaptive" {
							return domain.ErrInvalid
						}
					}
					if len(parameter.Default) > 0 {
						var ratio string
						if json.Unmarshal(parameter.Default, &ratio) != nil || ratio != "adaptive" {
							return domain.ErrInvalid
						}
					}
				}
			}
		}
		for name := range model.Parameters {
			if !slices.Contains(config.ParameterFields(model.Capability), name) {
				return domain.ErrInvalid
			}
		}
	}
	return nil
}

func decodeRouteDriver(raw json.RawMessage, out *catalog.ProviderConfig) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func insertRoute(tx *gorm.DB, route domain.Route, ciphertext []byte, keyID string, concurrency, rpm int, enabled bool) error {
	if err := tx.Exec("INSERT INTO ai.quota(name,concurrency,requests_per_minute) VALUES(?,?,?) ON CONFLICT DO NOTHING", route.Group, concurrency, rpm).Error; err != nil {
		return err
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "ai-credential/"+route.Credential.String()).Error; err != nil {
		return err
	}
	var other int64
	if err := tx.Raw("SELECT count(*) FROM ai.route WHERE credential_id=? AND quota_group<>?", route.Credential, route.Group).Scan(&other).Error; err != nil {
		return err
	}
	if other > 0 {
		return domain.ErrConflict
	}
	var count int
	r := tx.Raw("SELECT 1 FROM ai.quota WHERE name=? AND concurrency=? AND requests_per_minute=? FOR UPDATE", route.Group, concurrency, rpm).Scan(&count)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return domain.ErrConflict
	}
	if len(ciphertext) == 0 {
		var prior struct {
			Ciphertext []byte
			KeyID      string
		}
		if err := tx.Raw("SELECT ciphertext,key_id FROM ai.credential WHERE id=? AND supplier_id=?", route.Credential, route.Supplier).Scan(&prior).Error; err != nil {
			return err
		}
		ciphertext = prior.Ciphertext
		keyID = prior.KeyID
		if len(ciphertext) == 0 {
			return domain.ErrInvalid
		}
	}
	if err := tx.Exec("INSERT INTO ai.credential(id,supplier_id,key_id,ciphertext,state) VALUES(?,?,?,?,'active') ON CONFLICT DO NOTHING", route.Credential, route.Supplier, keyID, ciphertext).Error; err != nil {
		return err
	}
	var existing struct {
		SupplierID uuid.UUID
		KeyID      string
		Ciphertext []byte
	}
	if err := tx.Raw("SELECT supplier_id,key_id,ciphertext FROM ai.credential WHERE id=? FOR SHARE", route.Credential).Scan(&existing).Error; err != nil {
		return err
	}
	if existing.SupplierID != route.Supplier || existing.KeyID != keyID || !bytes.Equal(existing.Ciphertext, ciphertext) {
		return domain.ErrConflict
	}
	if enabled {
		if err := tx.Exec("UPDATE ai.route SET enabled=false WHERE model_id=? AND credential_id=? AND enabled", route.ModelID, route.Credential).Error; err != nil {
			return err
		}
	}
	r = tx.Exec("INSERT INTO ai.route(id,model_id,credential_id,quota_group,priority,document,enabled) SELECT ?,?,?,?,?,?::jsonb,? WHERE NOT EXISTS (SELECT 1 FROM ai.onboarding WHERE id=? AND deleted_at IS NOT NULL) ON CONFLICT DO NOTHING", route.ID, route.ModelID, route.Credential, route.Group, route.Priority, string(mustJSON(route)), enabled, route.ID)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return domain.ErrConflict
	}
	return nil
}

// CredentialState stops new dispatches immediately and preserves original identity.
func (s *Store) CredentialState(ctx context.Context, id uuid.UUID, state string) error {
	if state != "active" && state != "draining" && state != "revoked" {
		return domain.ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		r := tx.Exec("UPDATE ai.credential SET state=? WHERE id=? AND state<>'revoked'", state, id)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return domain.ErrConflict
		}
		return nil
	})
}

// Wallet reports the authoritative projection and its immutable event proof.
func (s *Store) Wallet(ctx context.Context, scope uuid.UUID) (map[string]any, error) {
	result := map[string]any{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, scope, uuid.Nil, false); err != nil {
			return err
		}
		var a struct{ Available, Held, Version int64 }
		if err := tx.Raw("SELECT available,held,version FROM ai.account WHERE scope_id=? FOR SHARE", scope).Scan(&a).Error; err != nil {
			return err
		}
		var events []struct {
			ID                                    uuid.UUID
			RequestID                             *uuid.UUID
			Kind                                  string
			AvailableDelta, HeldDelta, SpentDelta int64
		}
		allowed, err := CanReadLedger(tx, scope, s.actor.ID)
		if err != nil {
			return err
		}
		if allowed {
			if err := tx.Raw("SELECT id,request_id,kind,available_delta,held_delta,spent_delta FROM ai.money_event WHERE scope_id=? ORDER BY created_at,id LIMIT 100", scope).Scan(&events).Error; err != nil {
				return err
			}
		}
		result["history_available"] = allowed
		result["available"] = a.Available
		result["held"] = a.Held
		result["version"] = a.Version
		result["events"] = events
		return nil
	})
	return result, err
}

// Bills joins every new request to its single hold, independently of submission origin.
func (s *Store) Bills(ctx context.Context, scope uuid.UUID) ([]domain.Record, error) {
	var result []domain.Record
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, scope, uuid.Nil, false); err != nil {
			return err
		}
		var ids []uuid.UUID
		allowed, err := CanReadLedger(tx, scope, s.actor.ID)
		if err != nil {
			return err
		}
		query := "SELECT id FROM ai.request WHERE scope_id=?"
		args := []any{scope}
		if !allowed {
			query += " AND actor_id=?"
			args = append(args, s.actor.ID)
		}
		query += " ORDER BY created_at DESC,id DESC LIMIT 100"
		if err := tx.Raw(query, args...).Scan(&ids).Error; err != nil {
			return err
		}
		for _, id := range ids {
			record, err := scanRecord(tx, id)
			if err != nil {
				return err
			}
			result = append(result, record)
		}
		return nil
	})
	return result, err
}
