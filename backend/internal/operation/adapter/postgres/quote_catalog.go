package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
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
		if err := requireCurrentActor(tx, actor); err != nil {
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
			return nil // agent_session has no single model or price snapshot.
		}
		var current struct {
			ModelVersionID *uuid.UUID
			PriceVersionID *uuid.UUID
		}
		result = tx.Raw(`
			SELECT m.current_version_id AS model_version_id,
			       price.id AS price_version_id
			FROM catalog.model_profile_version AS quoted_version
			JOIN catalog.model_profile AS m
			  ON m.id = quoted_version.model_profile_id AND NOT m.is_delete
			JOIN catalog.provider AS provider
			  ON provider.id = m.provider_id AND NOT provider.is_delete
			JOIN catalog.provider_credential AS credential
			  ON credential.provider_id = provider.id
			 AND credential.status = 'active' AND NOT credential.is_delete
			JOIN catalog.capability AS capability
			  ON capability.key = m.capability AND NOT capability.is_delete
			JOIN catalog.model_profile_version AS current_version
			  ON current_version.id = m.current_version_id
			 AND current_version.model_profile_id = m.id AND NOT current_version.is_delete
			LEFT JOIN LATERAL (
			  SELECT id FROM catalog.price_rule_version
			  WHERE model_profile_id = m.id AND effective_from <= ?
			    AND NOT is_delete
			  ORDER BY effective_from DESC, version_no DESC
			  LIMIT 1
			) AS price ON true
			WHERE quoted_version.id = ?::uuid AND NOT quoted_version.is_delete
			  AND m.capability = ? AND m.status = 'active'
			  AND provider.status = 'active'
			  AND (? OR provider.region = 'domestic')
			  AND ? = ANY(current_version.modes)
			FOR SHARE OF m, provider, credential
		`, at, quoted.ModelProfileVersionID.String(), quoted.Capability,
			project.AllowOverseasModels, quoted.Mode).Scan(&current)
		if result.Error != nil {
			return fmt.Errorf("read current quote model and price: %w", result.Error)
		}
		if result.RowsAffected == 1 {
			modelID, priceID = current.ModelVersionID, current.PriceVersionID
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read quote catalog: %w", err)
	}
	return modelID, priceID, nil
}
