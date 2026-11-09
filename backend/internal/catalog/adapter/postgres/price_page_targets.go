package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// ListOfficialPricePageTargets reads only first-party page presets for review snapshots.
func (s *Store) ListOfficialPricePageTargets(ctx context.Context) ([]application.OfficialPricePageTarget, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("read official price page targets: unavailable catalog store")
	}
	var rows []struct {
		AdapterKey, ProviderModelID, SourceURL, Currency string
		Unit                                             *string
		ReferenceLines                                   string
	}
	result := s.db.WithContext(ctx).Raw(`
		SELECT adapter_key,provider_model_id,source_url,currency,unit,reference_lines::text AS reference_lines
		FROM catalog.model_price_preset
		WHERE adapter_key IN ('volcengine_ark','deepseek','openai')
		ORDER BY adapter_key,provider_model_id
		LIMIT ?`, maxPriceCandidateBatch+1).Scan(&rows)
	if result.Error != nil {
		return nil, fmt.Errorf("read official price page targets: %w", result.Error)
	}
	if len(rows) > maxPriceCandidateBatch {
		return nil, fmt.Errorf("official price page targets exceed limit")
	}
	targets := make([]application.OfficialPricePageTarget, 0, len(rows))
	for _, row := range rows {
		var referenceLines []string
		if err := json.Unmarshal([]byte(row.ReferenceLines), &referenceLines); err != nil || len(referenceLines) == 0 {
			return nil, domain.ErrInvalidPriceRule
		}
		target := application.OfficialPricePageTarget{
			AdapterKey: row.AdapterKey, ProviderModelID: row.ProviderModelID,
			SourceURL: row.SourceURL, Currency: row.Currency, ReferenceLines: referenceLines,
		}
		if row.Unit != nil {
			unit := domain.PriceUnit(*row.Unit)
			target.Unit = &unit
		}
		targets = append(targets, target)
	}
	return targets, nil
}

var _ application.PriceCandidateSyncStore = (*Store)(nil)
