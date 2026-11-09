package postgres

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

const maxPriceCandidateBatch = 1000

// ListOpenRouterModelIDs returns configured text models whose flat token pricing can be normalized.
func (s *Store) ListOpenRouterModelIDs(ctx context.Context) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("read OpenRouter price sync models: unavailable catalog store")
	}
	var modelIDs []string
	result := s.db.WithContext(ctx).Raw(`
		SELECT provider_model_id FROM (
   SELECT model.payload->>'provider_model_id' AS provider_model_id
   FROM catalog.model_preset model
   JOIN catalog.provider_preset_version version ON version.id=model.preset_version_id
   JOIN catalog.provider_preset preset ON preset.id=version.preset_id AND preset.origin='builtin'
   WHERE version.payload->'config'->>'adapter_key'='openrouter' AND model.payload->>'capability'='text.generate'
   AND version.version_no=(SELECT max(version_no) FROM catalog.provider_preset_version WHERE preset_id=preset.id)
   UNION
			SELECT version.provider_model_id
			FROM catalog.provider provider
			JOIN catalog.model_profile model ON model.provider_id=provider.id AND model.capability='text.generate' AND NOT model.is_delete
			JOIN catalog.model_profile_version version ON version.id=model.current_version_id AND NOT version.is_delete
			WHERE provider.adapter_key='openrouter' AND NOT provider.is_delete
			UNION
			SELECT version.provider_model_id
			FROM catalog.model_price_preset preset
			JOIN catalog.provider provider ON provider.adapter_key=preset.adapter_key AND NOT provider.is_delete
			JOIN catalog.model_profile model ON model.provider_id=provider.id AND model.capability='text.generate' AND NOT model.is_delete
			JOIN catalog.model_profile_version version ON version.id=model.current_version_id AND version.provider_model_id=preset.provider_model_id AND NOT version.is_delete
			WHERE preset.adapter_key='openrouter'
		) configured
		ORDER BY provider_model_id
		LIMIT ?`, maxPriceCandidateBatch+1).Scan(&modelIDs)
	if result.Error != nil {
		return nil, fmt.Errorf("read OpenRouter price sync models: %w", result.Error)
	}
	if len(modelIDs) > maxPriceCandidateBatch {
		return nil, fmt.Errorf("OpenRouter price sync model allowlist exceeds limit")
	}
	seen := make(map[string]struct{}, len(modelIDs))
	allowlist := make([]string, 0, len(modelIDs)+domain.MaxPresetModels)
	for _, modelID := range modelIDs {
		seen[modelID] = struct{}{}
		allowlist = append(allowlist, modelID)
	}
	if len(allowlist) > maxPriceCandidateBatch {
		return nil, fmt.Errorf("OpenRouter price sync model allowlist exceeds limit")
	}
	slices.Sort(allowlist)
	return allowlist, nil
}

// UpsertPriceCandidates stores public-source proposals without changing billable prices.
func (s *Store) UpsertPriceCandidates(ctx context.Context, candidates []domain.PriceCandidate) (int, error) {
	if s == nil || s.db == nil || len(candidates) > maxPriceCandidateBatch {
		return 0, fmt.Errorf("invalid price candidate batch")
	}
	changed := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, candidate := range candidates {
			if candidate.Validate() != nil || !supportedPriceCandidateAdapter(candidate.AdapterKey) ||
				candidate.Status != domain.PriceCandidatePendingReview && candidate.Status != domain.PriceCandidateNeedsReview {
				return fmt.Errorf("invalid public price candidate")
			}
			var unit any
			if candidate.Unit != nil {
				unit = string(*candidate.Unit)
			}
			var rule any
			if len(candidate.Rule) > 0 {
				rule = string(candidate.Rule)
			}
			result := tx.Exec(`
				INSERT INTO catalog.model_price_candidate
					(id,adapter_key,provider_model_id,source_url,source_hash,parser_version,source_fetched_at,currency,unit,rule,source_pricing,status)
				VALUES (?::uuid,?,?,?,?,?,?,?, ?,?::jsonb,?::jsonb,?)
				ON CONFLICT (adapter_key,provider_model_id,source_hash,parser_version) DO UPDATE
				SET source_fetched_at=GREATEST(catalog.model_price_candidate.source_fetched_at,EXCLUDED.source_fetched_at)
				`,
				candidate.ID.String(), candidate.AdapterKey, candidate.ProviderModelID, candidate.SourceURL,
				candidate.SourceHash, candidate.ParserVersion, candidate.SourceFetchedAt,
				candidate.Currency, unit, rule, string(candidate.SourcePricing), string(candidate.Status))
			if result.Error != nil {
				return fmt.Errorf("upsert public price candidate: %w", result.Error)
			}
			changed += int(result.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("persist OpenRouter price candidates: %w", err)
	}
	return changed, nil
}

func supportedPriceCandidateAdapter(adapter string) bool {
	switch adapter {
	case "openrouter", "openai", "deepseek", "volcengine_ark":
		return true
	default:
		return false
	}
}

func lockPriceCandidateForPublication(tx *gorm.DB, candidateID, modelID uuid.UUID, price domain.PriceRuleVersion) error {
	if tx == nil || candidateID == uuid.Nil || modelID == uuid.Nil || price.ModelID != modelID {
		return application.ErrInvalidPublishPriceRule
	}
	var row struct {
		Status, AdapterKey, ProviderModelID string
		SourceMatches, CurrencyMatches      bool
		UnitMatches, RuleMatches            bool
	}
	result := tx.Raw(`
		SELECT candidate.status,provider.adapter_key,version.provider_model_id,
		       candidate.source_url=? AS source_matches,candidate.currency::text=? AS currency_matches,
		       (candidate.status='needs_review' OR candidate.unit IS NULL OR candidate.unit=?) AS unit_matches,
		       (candidate.status='needs_review' OR candidate.rule=?::jsonb) AS rule_matches
		FROM catalog.model_price_candidate candidate
		JOIN catalog.model_profile model ON model.id=?::uuid AND NOT model.is_delete
		JOIN catalog.provider provider ON provider.id=model.provider_id AND NOT provider.is_delete
		JOIN catalog.model_profile_version version ON version.id=model.current_version_id AND NOT version.is_delete
		WHERE candidate.id=?::uuid AND candidate.adapter_key=provider.adapter_key
		  AND candidate.provider_model_id=version.provider_model_id
		FOR UPDATE OF candidate`,
		price.SourceURL, price.Currency, string(price.Unit), string(price.Rule), modelID.String(), candidateID.String()).Scan(&row)
	if result.Error != nil {
		return fmt.Errorf("lock price candidate: %w", result.Error)
	}
	if result.RowsAffected != 1 ||
		row.Status != string(domain.PriceCandidatePendingReview) && row.Status != string(domain.PriceCandidateNeedsReview) ||
		!row.SourceMatches || !row.CurrencyMatches || !row.UnitMatches || !row.RuleMatches || !supportedPriceCandidateAdapter(row.AdapterKey) || row.ProviderModelID == "" {
		return application.ErrInvalidPublishPriceRule
	}
	return nil
}

func markPriceCandidatePublished(tx *gorm.DB, candidateID uuid.UUID, procurement domain.PriceRuleVersion, priceVersionID, actorID uuid.UUID, reviewedAt time.Time) error {
	if tx == nil || candidateID == uuid.Nil || procurement.ModelID == uuid.Nil || priceVersionID == uuid.Nil || actorID == uuid.Nil || reviewedAt.IsZero() {
		return application.ErrInvalidPublishPriceRule
	}
	result := tx.Exec(`
		UPDATE catalog.model_price_candidate
		SET status='published',unit=?,rule=?::jsonb,published_price_version_id=?::uuid,reviewed_by=?::uuid,reviewed_at=?
		WHERE id=?::uuid AND status IN ('pending_review','needs_review') AND published_price_version_id IS NULL`,
		string(procurement.Unit), string(procurement.Rule), priceVersionID.String(), actorID.String(), reviewedAt.UTC(), candidateID.String())
	if result.Error != nil {
		return fmt.Errorf("mark price candidate published: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.ErrInvalidPublishPriceRule
	}
	return nil
}
