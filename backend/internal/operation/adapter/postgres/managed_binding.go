package postgres

import (
	"context"
	"fmt"

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
	return nil
}

// admitManagedDispatch runs after the operation is locked and before claiming
// the one paid send. Catalog lock ordering matches release activation. A
// completed disable wins over this claim; an already claimed call is never sent again.
func admitManagedDispatch(tx *gorm.DB, identity application.ProviderDispatchIdentity) error {
	var row struct {
		AdapterKey          string
		ProviderID, ModelID uuid.UUID
		ReleaseID, ConfigID *uuid.UUID
	}
	r := tx.Raw(`SELECT p.adapter_key,p.id AS provider_id,m.id AS model_id,r.id AS release_id,r.config_version_id AS config_id
 FROM catalog.model_profile_version v JOIN catalog.model_profile m ON m.id=v.model_profile_id
 JOIN catalog.provider p ON p.id=m.provider_id
 LEFT JOIN operation.provider_binding binding ON binding.operation_id=?
 LEFT JOIN catalog.model_release r ON r.id=binding.release_id AND r.model_id=m.id
 AND r.model_version_id=v.id AND r.price_version_id=?
 WHERE v.id=?`, identity.OperationID, identity.PriceRuleVersionID, identity.ModelProfileVersionID).Scan(&row)
	if r.Error != nil {
		return fmt.Errorf("read dispatch binding: %w", r.Error)
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
	r = tx.Raw(`SELECT concurrency_limit,rate_limit_per_min FROM catalog.provider WHERE id=? AND (status='active' OR ?) AND NOT is_delete FOR UPDATE`, row.ProviderID, isTrial).Scan(&connection)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrWorkflowModelUnavailable
	}
	var counts struct{ Active, Recent int64 }
	if err := tx.Raw(`SELECT count(DISTINCT op.id) FILTER(WHERE op.status NOT IN ('completed','failed','cancelled','expired')) AS active,
     count(DISTINCT op.id) FILTER(WHERE call.dispatch_started_at>now()-interval '1 minute') AS recent
     FROM operation.provider_credential_binding cb JOIN catalog.provider_credential key ON key.id=cb.credential_id
     JOIN operation.operation op ON op.id=cb.operation_id
     JOIN operation.provider_call call ON call.operation_id=op.id AND call.action='submit' AND call.dispatch_started_at IS NOT NULL AND NOT call.is_delete
     WHERE key.provider_id=?`, row.ProviderID).Scan(&counts).Error; err != nil {
		return err
	}
	if counts.Active >= connection.ConcurrencyLimit || counts.Recent >= connection.RateLimitPerMin {
		return application.ErrManagedCapacity
	}
	var key struct{ ID uuid.UUID }
	r = tx.Raw(`SELECT id FROM catalog.provider_credential WHERE provider_id=? AND status='active' AND NOT is_delete FOR SHARE`, row.ProviderID).Scan(&key)
	if r.Error != nil {
		return fmt.Errorf("lock dispatch credential: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrWorkflowModelUnavailable
	}
	r = tx.Raw(`SELECT 1 FROM catalog.model_profile m JOIN catalog.model_release r ON r.model_id=m.id
 WHERE m.id=? AND NOT m.is_delete AND r.id=? AND ((? AND r.status='draft') OR (NOT ? AND m.status='active' AND m.active_release_id=r.id AND r.status='active'))
 FOR SHARE OF m,r`, row.ModelID, *row.ReleaseID, isTrial, isTrial).Scan(&present)
	if r.Error != nil {
		return fmt.Errorf("lock dispatch release: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrWorkflowModelUnavailable
	}
	if err := checkWorkflowInputsForSubmission(tx, identity.OperationID, identity.ProjectID); err != nil {
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
 key.id AS credential_id,key.key_id,key.ciphertext
 FROM operation.provider_binding binding JOIN operation.operation op ON op.id=binding.operation_id AND NOT op.is_delete
 JOIN catalog.model_release release ON release.id=binding.release_id AND release.model_version_id=op.model_profile_version_id AND release.price_version_id=op.price_rule_version_id
 JOIN catalog.provider_config_version config ON config.id=release.config_version_id
 JOIN operation.provider_credential_binding cb ON cb.operation_id=op.id
 JOIN catalog.provider_credential key ON key.id=cb.credential_id AND key.provider_id=config.provider_id
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
