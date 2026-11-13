package postgres

import (
	"context"
	"fmt"
	"time"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// freezeManagedRelease runs inside quote insertion. A release includes immutable
// configuration, model and price; saving a draft never changes this identity.
func freezeManagedRelease(tx *gorm.DB, op domain.Operation) error {
	if op.ModelProfileVersionID == nil {
		return nil
	}
	var row struct {
		AdapterKey string
		ReleaseID  *uuid.UUID
	}
	r := tx.Raw(`SELECT p.adapter_key, r.id AS release_id
 FROM catalog.model_profile_version v JOIN catalog.model_profile m ON m.id=v.model_profile_id
 JOIN catalog.provider p ON p.id=m.provider_id
 LEFT JOIN catalog.model_release r ON r.id=m.active_release_id AND r.status='active'
 AND r.model_version_id=v.id AND r.price_version_id=?
 WHERE v.id=? FOR SHARE OF m,p`, op.PriceRuleVersionID, op.ModelProfileVersionID).Scan(&row)
	if r.Error != nil {
		return fmt.Errorf("read quote release: %w", r.Error)
	}
	if !catalogdomain.IsManagedCloud(row.AdapterKey) {
		return nil
	}
	if row.ReleaseID == nil {
		return application.ErrWorkflowModelUnavailable
	}
	if err := tx.Exec(`INSERT INTO operation.provider_binding(operation_id,release_id) VALUES(?,?)`, op.ID, *row.ReleaseID).Error; err != nil {
		return fmt.Errorf("freeze quote release: %w", err)
	}
	// A fixed Agent authorization and explicit upstream endpoint remain pinned.
	if err := tx.Exec(`INSERT INTO operation.route_pool_binding(operation_id,version_id)
 SELECT ?,pv.id FROM catalog.route_pool_version pv
 JOIN catalog.model_profile_version mv ON mv.id=?
 JOIN operation.operation op ON op.id=? AND op.origin<>'agent'
 WHERE pv.primary_release_id=? AND COALESCE(mv.limits->>'provider_endpoint','')=''
 ORDER BY pv.revision DESC LIMIT 1`, op.ID, op.ModelProfileVersionID, op.ID, *row.ReleaseID).Error; err != nil {
		return err
	}
	return nil
}

// admitManagedDispatch runs after the operation is locked and before claiming
// the one paid send. Catalog lock ordering matches release activation. A
// completed disable wins over this claim; an already claimed call is never sent again.
func (s *Store) admitManagedDispatch(tx *gorm.DB, identity application.ProviderDispatchIdentity) error {
	var row struct {
		AdapterKey          string
		TimePricing         string
		CheckedAt           time.Time
		ProviderID, ModelID uuid.UUID
		ReleaseID, ConfigID *uuid.UUID
	}
	r := tx.Raw(`SELECT p.adapter_key,p.id AS provider_id,m.id AS model_id,r.id AS release_id,r.config_version_id AS config_id,price.rule->>'time_pricing' AS time_pricing,clock_timestamp() AS checked_at
 FROM catalog.model_profile_version v JOIN catalog.model_profile m ON m.id=v.model_profile_id
 JOIN catalog.provider p ON p.id=m.provider_id
 LEFT JOIN operation.provider_binding binding ON binding.operation_id=?
 LEFT JOIN catalog.model_release r ON r.id=binding.release_id AND r.model_id=m.id
 AND r.model_version_id=v.id AND r.price_version_id=?
 LEFT JOIN catalog.price_rule_version price ON price.id=r.price_version_id
 WHERE v.id=?`, identity.OperationID, identity.PriceRuleVersionID, identity.ModelProfileVersionID).Scan(&row)
	if r.Error != nil {
		return fmt.Errorf("read dispatch binding: %w", r.Error)
	}
	if row.TimePricing != "" {
		if _, err := catalogdomain.DeepSeekOffPeak2026(row.CheckedAt); err != nil || row.AdapterKey != "deepseek" || row.TimePricing != catalogdomain.DeepSeekCN2026 {
			return application.ErrWorkflowModelUnavailable
		}
	}
	if !catalogdomain.IsManagedCloud(row.AdapterKey) {
		return nil
	}
	if row.ReleaseID == nil || row.ConfigID == nil {
		return application.ErrWorkflowModelUnavailable
	}
	var trial struct{ ActorID uuid.UUID }
	if err := tx.Raw(`SELECT actor_id FROM operation.provider_trial WHERE operation_id=?`, identity.OperationID).Scan(&trial).Error; err != nil {
		return err
	}
	isTrial := trial.ActorID != uuid.Nil
	if isTrial {
		var authorized int
		r := tx.Raw(`SELECT 1 FROM identity."user" WHERE id=? AND platform_role='platform_admin' AND status='active' AND NOT is_delete FOR SHARE`, trial.ActorID).Scan(&authorized)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrWorkflowModelUnavailable
		}
	}
	if !isTrial {
		route, err := s.selectPoolMember(tx, identity)
		if err != nil {
			return err
		}
		if route != nil {
			row.ProviderID, row.ModelID = route.ProviderID, route.ModelID
			row.ReleaseID, row.ConfigID = &route.ReleaseID, &route.ConfigVersionID
		}
	}
	var present int
	r = tx.Raw(`SELECT 1 FROM catalog.provider_preset preset
 JOIN catalog.provider_preset_version pv ON pv.preset_id=preset.id
 JOIN catalog.provider_config_version config ON config.preset_version_id=pv.id
 WHERE config.id=? AND config.provider_id=? AND NOT preset.emergency_disabled FOR SHARE OF preset`, *row.ConfigID, row.ProviderID).Scan(&present)
	if r.Error != nil {
		return fmt.Errorf("lock dispatch preset: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrWorkflowModelUnavailable
	}
	var connection struct{ ConcurrencyLimit, RateLimitPerMin int64 }
	r = tx.Raw(`SELECT l.concurrency_limit,l.rate_limit_per_min FROM catalog.provider p JOIN catalog.effective_provider_limit l ON l.id=p.id WHERE p.id=? AND (status='active' OR (? AND status='disabled')) AND NOT is_delete FOR UPDATE OF p`, row.ProviderID, isTrial).Scan(&connection)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrWorkflowModelUnavailable
	}
	available, err := modelHasCapacity(tx, row.ProviderID, *row.ReleaseID, identity.OperationID, connection.ConcurrencyLimit, connection.RateLimitPerMin)
	if err != nil {
		return err
	}
	if !available {
		return application.ErrManagedCapacity
	}
	var key struct{ ID uuid.UUID }
	r = tx.Raw(`SELECT c.id FROM catalog.provider_credential c
 WHERE c.provider_id=? AND NOT EXISTS(SELECT 1 FROM catalog.route_connection_cooldown down WHERE down.provider_id=c.provider_id AND down.until_at>statement_timestamp()) AND (c.status='active' OR (NOT ? AND c.status='draining' AND c.pool_enabled)) AND NOT c.is_delete AND (? OR (
 c.last_test_result IS DISTINCT FROM 'auth_failed' AND EXISTS (
 SELECT 1 FROM catalog.model_validation x
 JOIN operation.operation completed ON completed.id=x.operation_id AND completed.status='completed' AND NOT completed.is_delete
 JOIN operation.provider_binding binding ON binding.operation_id=completed.id AND binding.release_id=x.release_id
 JOIN operation.provider_credential_binding used ON used.operation_id=completed.id AND used.credential_id=c.id
 WHERE x.release_id=? AND x.credential_id=c.id AND x.mode=(SELECT mode FROM operation.operation WHERE id=?)
 AND x.status='succeeded' AND x.adapter_version='ai-provider-v1' AND x.evidence->>'kind'='real_provider'
 AND completed.mode=x.mode AND EXISTS(SELECT 1 FROM catalog.model_release verified WHERE verified.id=x.release_id AND verified.model_version_id=completed.model_profile_version_id AND verified.price_version_id=completed.price_rule_version_id))))
 ORDER BY (SELECT max(call.dispatch_started_at) FROM operation.provider_credential_binding used JOIN operation.provider_call call ON call.operation_id=used.operation_id AND call.action='submit' WHERE used.credential_id=c.id) ASC NULLS FIRST,c.create_time,c.id LIMIT 1 FOR SHARE OF c`, row.ProviderID, isTrial, isTrial, *row.ReleaseID, identity.OperationID).Scan(&key)
	if r.Error != nil {
		return fmt.Errorf("lock dispatch credential: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrWorkflowModelUnavailable
	}
	r = tx.Raw(`SELECT 1 FROM catalog.model_profile m JOIN catalog.model_release r ON r.model_id=m.id
 WHERE m.id=? AND m.status<>'archived' AND NOT m.is_delete AND r.id=? AND ((? AND (r.status='draft' OR (r.status='active' AND m.status='active' AND m.active_release_id=r.id))) OR (NOT ? AND m.status='active' AND EXISTS(SELECT 1 FROM catalog.executable_release e WHERE e.id=r.id)))
 FOR SHARE OF m,r`, row.ModelID, *row.ReleaseID, isTrial, isTrial).Scan(&present)
	if r.Error != nil {
		return fmt.Errorf("lock dispatch release: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrWorkflowModelUnavailable
	}
	if err := s.checkWorkflowInputsForSubmission(tx, identity.OperationID, identity.ProjectID); err != nil {
		return err
	}
	// This table is insert-only to the application role. No key rotation can rewrite it.
	if err := tx.Exec(`INSERT INTO operation.provider_credential_binding(operation_id,credential_id) VALUES(?,?) ON CONFLICT(operation_id) DO NOTHING`, identity.OperationID, key.ID).Error; err != nil {
		return fmt.Errorf("bind dispatch credential: %w", err)
	}
	r = tx.Raw(`SELECT 1 FROM operation.provider_credential_binding WHERE operation_id=? AND credential_id=?`, identity.OperationID, key.ID).Scan(&present)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrWorkflowModelUnavailable
	}
	return nil
}

// LoadManagedCredential accepts draining only for the exact previously bound
// operation. Revoked keys stop every subsequent query and content download.
func (s *Store) loadManagedCredential(ctx context.Context, operationID uuid.UUID) (application.ManagedCredential, error) {
	if s == nil || s.db == nil {
		return application.ManagedCredential{}, ErrUnavailable
	}
	var row application.ManagedCredential
	r := s.db.WithContext(ctx).Raw(`SELECT config.provider_id,config.id AS config_version_id,config.config,
 mv.provider_model_id AS model_id,COALESCE(mv.limits->>'provider_endpoint','') AS endpoint_tag,key.id AS credential_id,key.key_id,key.ciphertext
 FROM operation.provider_binding binding JOIN operation.operation op ON op.id=binding.operation_id AND NOT op.is_delete
 JOIN catalog.model_release release ON release.id=binding.release_id AND release.model_version_id=op.model_profile_version_id AND release.price_version_id=op.price_rule_version_id
 LEFT JOIN operation.route_delivery routed ON routed.operation_id=op.id
 LEFT JOIN operation.route_retry retry ON retry.operation_id=op.id
 JOIN catalog.model_release actual ON actual.id=COALESCE(retry.release_id,routed.release_id,release.id)
 JOIN catalog.model_profile_version mv ON mv.id=actual.model_version_id
 JOIN catalog.provider_config_version config ON config.id=actual.config_version_id
 JOIN operation.provider_credential_binding cb ON cb.operation_id=op.id
 JOIN catalog.provider_credential key ON key.id=COALESCE(retry.credential_id,cb.credential_id) AND key.provider_id=config.provider_id
 WHERE op.id=? AND key.status IN ('active','draining') AND NOT key.is_delete`, operationID).Scan(&row)
	if r.Error != nil {
		return application.ManagedCredential{}, fmt.Errorf("load bound credential: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ManagedCredential{}, application.ErrWorkflowModelUnavailable
	}
	return row, nil
}

// ManagedMediaKey checks project ownership and review state before URL signing.
func (s *Store) managedMediaKey(ctx context.Context, projectID, assetID uuid.UUID) (string, error) {
	var row struct{ ObjectKey string }
	r := s.db.WithContext(ctx).Raw(`SELECT object_key FROM media.media_asset WHERE id=? AND project_id=? AND status='ready' AND moderation_status IN ('passed','skipped') AND NOT is_delete`, assetID, projectID).Scan(&row)
	if r.Error != nil {
		return "", r.Error
	}
	if r.RowsAffected != 1 {
		return "", application.ErrWorkflowInputNotReady
	}
	return row.ObjectKey, nil
}

// LoadManagedCredential rechecks current execution authority before exposing a bound key.
func (s *Store) LoadManagedCredential(ctx context.Context, id uuid.UUID) (application.ManagedCredential, error) {
	if s == nil || s.db == nil {
		return application.ManagedCredential{}, ErrUnavailable
	}
	var result application.ManagedCredential
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireWorker(tx, id, true); err != nil {
			return err
		}
		var err error
		result, err = s.withDB(tx).loadManagedCredential(ctx, id)
		return err
	})
	return result, err
}

// ManagedMediaKey validates the operation's frozen project and current actor before signing inputs.
func (s *Store) ManagedMediaKey(ctx context.Context, operationID, projectID, assetID uuid.UUID) (string, error) {
	if s == nil || s.db == nil {
		return "", ErrUnavailable
	}
	var key string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireWorker(tx, operationID, true); err != nil {
			return err
		}
		var present int
		r := tx.Raw(`SELECT 1 FROM operation.operation o JOIN operation.operation_input i ON i.operation_id=o.id WHERE o.id=? AND o.project_id=? AND (i.media_asset_id=? OR i.mask_asset_id=?) AND NOT o.is_delete AND NOT i.is_delete LIMIT 1`, operationID, projectID, assetID, assetID).Scan(&present)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrNotFound
		}
		var err error
		key, err = s.withDB(tx).managedMediaKey(ctx, projectID, assetID)
		return err
	})
	return key, err
}
