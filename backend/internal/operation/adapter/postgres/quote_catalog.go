package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// ReadCurrentQuoteCatalog reads the currently usable model and effective price
// for a quoted operation. Nil IDs mean unavailable, so callers can calculate
// quote freshness reasons. Confirmation must repeat this read in its write
// transaction; this standalone read is only for preflight and testing.
func (s *Store) ReadCurrentQuoteCatalog(ctx context.Context, actor identityapp.Principal, quoted domain.Operation, at time.Time) (*uuid.UUID, *uuid.UUID, error) {
	if s == nil || s.db == nil {
		return nil, nil, ErrUnavailable
	}
	if err := quoted.Validate(); err != nil {
		return nil, nil, err
	}
	if at.IsZero() {
		return nil, nil, application.ErrInvalidQuoteCurrentFacts
	}
	var modelID, priceID *uuid.UUID
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireProject(tx, actor, quoted.ProjectID, false); err != nil {
			return err
		}
		var project struct{ AllowOverseasModels bool }
		result := tx.Raw(`
			SELECT allow_overseas_models
			FROM workspace.project
			WHERE id = ?::uuid AND org_id = ?::uuid
			  AND status = 'active' AND NOT is_delete
			FOR SHARE
		`, quoted.ProjectID.String(), actor.OrgID.String()).Scan(&project)
		if result.Error != nil {
			return fmt.Errorf("read quote project policy: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		if quoted.ModelProfileVersionID == nil {
			return domain.ErrInvalidOperation
		}
		trial, trialModel, trialPrice, err := currentTrialCatalog(tx, actor, quoted, project.AllowOverseasModels)
		if err != nil {
			return err
		}
		if trial {
			modelID, priceID = trialModel, trialPrice
			return nil
		}
		var frozen struct{ ReleaseID uuid.UUID }
		if err := tx.Raw(`SELECT release_id FROM operation.provider_binding WHERE operation_id=?`, quoted.ID).Scan(&frozen).Error; err != nil {
			return err
		}
		// Existing quotes can use only their frozen pool, even after a later
		// publication adds or removes a backup. Route admission rechecks at send.
		var pool struct {
			ID    uuid.UUID
			Ready bool
		}
		result = tx.Raw(`SELECT b.version_id AS id, EXISTS(SELECT 1 FROM catalog.route_pool_ready ready
 JOIN catalog.model_release r ON r.id=ready.primary_release_id
 JOIN catalog.model_profile m ON m.id=r.model_id AND m.capability=?
 JOIN catalog.provider p ON p.id=m.provider_id AND NOT p.is_delete AND p.status<>'archived'
 JOIN catalog.provider_config_version cfg ON cfg.id=r.config_version_id
 JOIN catalog.provider_preset_version pv ON pv.id=cfg.preset_version_id
 JOIN catalog.provider_preset ps ON ps.id=pv.preset_id AND NOT ps.emergency_disabled AND NOT ps.archived
 WHERE ready.version_id=b.version_id AND ready.mode=? AND r.id=? AND r.model_version_id=?
 AND r.price_version_id=? AND (p.region='domestic' OR ?)) AS ready
 FROM operation.route_pool_binding b WHERE b.operation_id=?`, quoted.Capability, quoted.Mode, frozen.ReleaseID, quoted.ModelProfileVersionID, quoted.PriceRuleVersionID, project.AllowOverseasModels, quoted.ID).Scan(&pool)
		if result.Error != nil {
			return result.Error
		}
		if pool.ID != uuid.Nil {
			if pool.Ready {
				modelID, priceID = quoted.ModelProfileVersionID, quoted.PriceRuleVersionID
			}
			return nil
		}
		// A retail-only publication can retire the original release. Keep its
		// sale, provided that the current connection and exact key remain usable.
		if frozen.ReleaseID != uuid.Nil {
			var permitted bool
			err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM catalog.executable_release r
 JOIN catalog.model_profile m ON m.id=r.model_id AND m.capability=?
 JOIN catalog.provider p ON p.id=m.provider_id AND (p.region='domestic' OR ?)
 JOIN catalog.verified_route_key key ON key.release_id=r.id AND key.mode=?
 WHERE r.id=? AND r.status='retired' AND r.model_version_id=? AND r.price_version_id=?)`, quoted.Capability, project.AllowOverseasModels, quoted.Mode, frozen.ReleaseID, quoted.ModelProfileVersionID, quoted.PriceRuleVersionID).Row().Scan(&permitted)
			if err != nil {
				return err
			}
			if permitted {
				modelID, priceID = quoted.ModelProfileVersionID, quoted.PriceRuleVersionID
				return nil
			}
		}
		var current struct {
			ReleaseID      *uuid.UUID
			ModelVersionID *uuid.UUID
			PriceVersionID *uuid.UUID
		}
		result = tx.Raw(fmt.Sprintf(`
			SELECT published.id AS release_id, CASE WHEN provider.adapter_key IN (%[1]s) THEN published.model_version_id ELSE m.current_version_id END AS model_version_id,
			       price.id AS price_version_id
			FROM catalog.model_profile_version AS quoted_version
			JOIN catalog.model_profile AS m
			  ON m.id = quoted_version.model_profile_id AND NOT m.is_delete
			JOIN catalog.provider AS provider
			  ON provider.id = m.provider_id AND NOT provider.is_delete
			JOIN LATERAL (SELECT k.id FROM catalog.provider_credential k WHERE k.provider_id=provider.id
 AND (k.status='active' OR (provider.adapter_key IN (%[1]s) AND k.status='draining' AND k.pool_enabled)) AND NOT k.is_delete
 ORDER BY k.id LIMIT 1 FOR SHARE OF k) credential ON true
			LEFT JOIN catalog.model_release AS published ON published.id=m.active_release_id AND published.status='active'
			JOIN catalog.capability AS capability
			  ON capability.key = m.capability AND NOT capability.is_delete
			JOIN catalog.model_profile_version AS current_version
			  ON current_version.id = CASE WHEN provider.adapter_key IN (%[1]s) THEN published.model_version_id ELSE m.current_version_id END
			 AND current_version.model_profile_id = m.id AND NOT current_version.is_delete
			LEFT JOIN LATERAL (
			  SELECT id FROM catalog.price_rule_version
			  WHERE (provider.adapter_key NOT IN (%[1]s) OR id=published.price_version_id) AND model_profile_id = m.id AND effective_from <= ?
			    AND NOT is_delete
			  ORDER BY effective_from DESC, version_no DESC
			  LIMIT 1
			) AS price ON true
			WHERE quoted_version.id = ?::uuid AND (provider.adapter_key NOT IN (%[1]s) OR EXISTS (SELECT 1 FROM catalog.provider_config_version pc JOIN catalog.provider_preset_version pv ON pv.id=pc.preset_version_id JOIN catalog.provider_preset preset ON preset.id=pv.preset_id WHERE pc.id=published.config_version_id AND NOT preset.emergency_disabled)) AND NOT quoted_version.is_delete
			  AND m.capability = ? AND m.status = 'active'
			  AND provider.status = 'active'
			  AND (? OR provider.region = 'domestic')
			  AND ? = ANY(current_version.modes)
			FOR SHARE OF m, provider
		`, catalogdomain.ManagedCloudSQLInList()), at, quoted.ModelProfileVersionID.String(), quoted.Capability,
			project.AllowOverseasModels, quoted.Mode).Scan(&current)
		if result.Error != nil {
			return fmt.Errorf("read current quote model and price: %w", result.Error)
		}
		if result.RowsAffected == 1 && (current.ReleaseID == nil || (frozen.ReleaseID != uuid.Nil && *current.ReleaseID == frozen.ReleaseID)) {
			if current.ModelVersionID == nil {
				return nil
			}
			releaseID := uuid.Nil
			if current.ReleaseID != nil {
				releaseID = *current.ReleaseID
			}
			available, err := s.availableForExecution(tx, *current.ModelVersionID, releaseID, quoted.Mode)
			if err != nil {
				return err
			}
			if !available {
				return nil
			}
			modelID, priceID = current.ModelVersionID, current.PriceVersionID
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read quote catalog: %w", err)
	}
	return modelID, priceID, nil
}

// commercialPriceReady checks the frozen version used by the active release.
// Release creation has already checked the linked procurement tariff.
func commercialPriceReady(tx *gorm.DB, priceID *uuid.UUID) (bool, error) {
	var ready bool
	err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM catalog.model_release r
		JOIN catalog.price_rule_version p ON p.id=r.price_version_id
		WHERE r.price_version_id=? AND (r.status='active' OR EXISTS(SELECT 1 FROM catalog.executable_release e WHERE e.id=r.id)) AND NOT p.is_delete
		  AND jsonb_exists(p.rule,'procurement_price_version_id'))`, priceID).Row().Scan(&ready)
	return ready, err
}
