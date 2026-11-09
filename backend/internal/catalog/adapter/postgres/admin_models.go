package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

const adminModelSelect = `SELECT id,model_key,provider_id,capability,display_name,status,
current_version_id,revision,create_time,update_time FROM catalog.model_profile`

func createdModel(row modelProfileRow) application.CreatedModel {
	m := row.model()
	return application.CreatedModel{ID: m.ID, Key: m.Key, ProviderID: m.ProviderID, Capability: m.Capability, DisplayName: m.DisplayName, Status: m.Status, CurrentVersionID: m.CurrentVersionID, Revision: m.Revision, CreateTime: m.CreateTime, UpdateTime: m.UpdateTime}
}

// ListAdminModels includes all real models independently of any project's region.
func (s *Store) ListAdminModels(ctx context.Context, actor identityapp.Principal, input application.AdminModelListInput) (application.AdminModelPage, error) {
	page := application.AdminModelPage{Items: []application.CreatedModel{}}
	if s == nil || s.db == nil || input.Limit < 1 || input.Limit > 200 {
		return page, application.ErrInvalidAdminModels
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requirePlatformAdmin(tx, actor); err != nil {
			return err
		}
		query := `WITH models AS (SELECT m.id,m.model_key,m.provider_id,m.capability,m.display_name,m.status,m.current_version_id,m.revision,m.create_time,m.update_time,
			EXISTS(SELECT 1 FROM catalog.model_release r WHERE r.model_id=m.id AND r.status IN ('active','retired')) AS has_publication,
			EXISTS(SELECT 1 FROM catalog.price_rule_version p WHERE p.model_profile_id=m.id AND NOT p.is_delete) AS has_price
			FROM catalog.model_profile m WHERE NOT m.is_delete) SELECT * FROM models AS m WHERE TRUE`
		args := []any{}
		if input.PricingReview {
			query = adminPresetPricingQuery
			args = append(args, domain.ManagedCloudAdapters())
		}
		if input.BillingStatus != "" {
			query += ` AND m.billing_status=?`
			args = append(args, input.BillingStatus)
		}
		if input.ProviderID != uuid.Nil {
			query += ` AND m.provider_id=?`
			args = append(args, input.ProviderID)
		}
		if input.Name != "" {
			query += ` AND (strpos(lower(m.display_name), lower(?)) > 0 OR strpos(lower(m.model_key), lower(?)) > 0 OR m.id::text=lower(?) OR EXISTS(SELECT 1 FROM catalog.model_profile_version search_version WHERE search_version.id=m.current_version_id AND search_version.provider_model_id=? AND NOT search_version.is_delete))`
			args = append(args, input.Name, input.Name, input.Name, input.Name)
		}
		if input.Capability != "" {
			query += ` AND m.capability = ?`
			args = append(args, input.Capability)
		}
		if input.After != nil {
			query += ` AND (model_key,id)>(?,?::uuid)`
			args = append(args, input.After.Key, input.After.ID)
		}
		// Scan stable bounded candidate batches until the filtered page is full.
		// Filtering is before page truncation; no unavailable row consumes a slot.
		baseQuery, baseArgs := query, append([]any{}, args...)
		var scanAfter *application.ModelCatalogCursor
		more := false
		for {
			batchQuery, batchArgs := baseQuery, append([]any{}, baseArgs...)
			if scanAfter != nil {
				batchQuery += ` AND (model_key,id)>(?,?::uuid)`
				batchArgs = append(batchArgs, scanAfter.Key, scanAfter.ID)
			}
			batchQuery += ` ORDER BY model_key,id LIMIT 200`
			var rows []struct {
				Model                                      modelProfileRow `gorm:"embedded"`
				HasPublication, HasPrice                   bool
				BillingUnit                                string
				ProcurementReady, SaleReady, SalePublished bool
				BillingStatus                              string
			}
			if err := tx.Raw(batchQuery, batchArgs...).Scan(&rows).Error; err != nil {
				return fmt.Errorf("read admin models: %w", err)
			}
			ids := make([]uuid.UUID, 0, len(rows))
			for _, row := range rows {
				ids = append(ids, row.Model.ID)
			}
			availability, err := readModelAvailability(tx, ids)
			if err != nil {
				return err
			}
			for _, row := range rows {
				item := createdModel(row.Model)
				item.ModelAvailability = availability[item.ID]
				if input.Status != "" && item.Available != (input.Status == "available") {
					continue
				}
				item.HasPublication = row.HasPublication
				item.HasPrice = row.HasPrice
				if input.PricingReview {
					item.Billing = &application.AdminModelBilling{Unit: row.BillingUnit, ProcurementReady: row.ProcurementReady, SaleReady: row.SaleReady, SalePublished: row.SalePublished, Status: row.BillingStatus}
				}
				page.Items = append(page.Items, item)
				if len(page.Items) > input.Limit {
					more = true
					break
				}
			}
			if more || len(rows) < 200 {
				break
			}
			last := rows[len(rows)-1].Model
			scanAfter = &application.ModelCatalogCursor{Key: last.ModelKey, ID: last.ID}
		}
		if more {
			page.Items = page.Items[:input.Limit]
		}

		if more {
			last := page.Items[len(page.Items)-1]
			page.Next = &application.ModelCatalogCursor{Key: last.Key, ID: last.ID}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	return page, err
}

type adminVersionRow struct {
	ID                                              uuid.UUID
	VersionNo                                       int
	ProviderModelID, Modes, Limits, ParamSchema     string
	SupportsQuery, SupportsCancel, SupportsCallback bool
	ExpectedMaxMS                                   int
	Moderation, Queue                               string
	CreateTime                                      time.Time
}

type adminPriceRow struct {
	SourceURL                 *string
	ReviewedAt                *time.Time
	ID                        uuid.UUID
	VersionNo                 int
	Unit, Rule, Currency      string
	FXRateToCNY               *string
	EffectiveFrom, CreateTime time.Time
}

type adminPriceCandidateRow struct {
	SourceURL, SourceHash, ParserVersion, ProviderModelID, Currency, Status string
	ID                                                                      uuid.UUID
	Unit                                                                    *string
	Rule, SourcePricing                                                     string
	SourceFetchedAt                                                         time.Time
}

// ReadAdminModel reads existing immutable histories without provider credentials.
func (s *Store) ReadAdminModel(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (application.AdminModelDetail, error) {
	result := application.AdminModelDetail{Versions: []application.AdminModelVersion{}, Prices: []application.AdminPrice{}, PriceCandidates: []application.ModelPriceCandidate{}}
	if s == nil || s.db == nil || id == uuid.Nil {
		return result, application.ErrInvalidAdminModels
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requirePlatformAdmin(tx, actor); err != nil {
			return err
		}
		var model modelProfileRow
		row := tx.Raw(adminModelSelect+` WHERE id=?::uuid AND NOT is_delete`, id).Scan(&model)
		if row.Error != nil {
			return fmt.Errorf("read admin model: %w", row.Error)
		}
		if row.RowsAffected != 1 {
			return application.ErrModelNotFound
		}
		result.Model = createdModel(model)
		availability, err := readModelAvailability(tx, []uuid.UUID{id})
		if err != nil {
			return err
		}
		result.Model.ModelAvailability = availability[id]
		var candidate struct{ Snapshot []byte }
		if err := tx.Raw(`SELECT candidate_snapshot AS snapshot FROM catalog.model_profile WHERE id=?`, id).Scan(&candidate).Error; err != nil {
			return fmt.Errorf("read model candidate: %w", err)
		}
		if len(candidate.Snapshot) > 0 {
			if err := json.Unmarshal(candidate.Snapshot, &result.Candidate); err != nil {
				return fmt.Errorf("decode model candidate: %w", err)
			}
		}
		var versions []adminVersionRow
		if err := tx.Raw(`SELECT id,version_no,provider_model_id,to_jsonb(modes)::text AS modes,
limits::text AS limits,param_schema::text AS param_schema,supports_query,supports_cancel,supports_callback,
expected_max_ms,moderation,queue,create_time FROM catalog.model_profile_version
WHERE model_profile_id=?::uuid AND NOT is_delete ORDER BY version_no DESC LIMIT 1001`, id).Scan(&versions).Error; err != nil {
			return fmt.Errorf("read admin model versions: %w", err)
		}
		if len(versions) > 1000 {
			return fmt.Errorf("model version history exceeds bound")
		}
		for _, v := range versions {
			var modes []string
			if err := json.Unmarshal([]byte(v.Modes), &modes); err != nil {
				return fmt.Errorf("decode model version modes: %w", err)
			}
			result.Versions = append(result.Versions, application.AdminModelVersion{ID: v.ID, VersionNo: v.VersionNo, ProviderModelID: v.ProviderModelID, Modes: modes, Limits: json.RawMessage(v.Limits), ParamSchema: json.RawMessage(v.ParamSchema), SupportsQuery: v.SupportsQuery, SupportsCancel: v.SupportsCancel, SupportsCallback: v.SupportsCallback, ExpectedMaxMS: v.ExpectedMaxMS, Moderation: v.Moderation, Queue: v.Queue, CreateTime: v.CreateTime})
		}
		if selection := result.Candidate; selection != nil && !strings.HasPrefix(selection.Model.Key, "custom-") {
			providerModelID := selection.Model.ProviderModelID
			if len(versions) > 0 {
				providerModelID = versions[0].ProviderModelID
			}
			if providerModelID == selection.Model.ProviderModelID {
				var preset struct {
					Currency, SourceURL, ReferenceLines string
					Unit                                *string
					Rule                                []byte
				}
				read := tx.Raw(`SELECT currency, source_url, reference_lines::text AS reference_lines,
  CASE WHEN available THEN rule END AS rule, CASE WHEN available THEN unit END AS unit
FROM (SELECT *, executable_until IS NULL OR executable_until > (now() AT TIME ZONE 'Asia/Shanghai')::date AS available
      FROM catalog.model_price_preset) AS pricing WHERE adapter_key=? AND provider_model_id=?`,
					selection.Model.AdapterKey, providerModelID).Scan(&preset)
				if read.Error != nil {
					return fmt.Errorf("read model price preset: %w", read.Error)
				}
				if read.RowsAffected == 1 {
					item := &application.ModelPricePreset{
						Currency: preset.Currency, SourceURL: preset.SourceURL,
						Unit: preset.Unit, Rule: json.RawMessage(preset.Rule),
					}
					if err := json.Unmarshal([]byte(preset.ReferenceLines), &item.ReferenceLines); err != nil {
						return fmt.Errorf("decode model price preset: %w", err)
					}
					result.PricePreset = item
				}
			}
		}
		var prices []adminPriceRow
		if err := tx.Raw(`SELECT id,version_no,unit,rule::text AS rule,currency::text AS currency,
source_url,reviewed_at,fx_rate_to_cny::text AS fx_rate_to_cny,effective_from,create_time FROM catalog.price_rule_version
WHERE model_profile_id=?::uuid AND NOT is_delete ORDER BY version_no DESC LIMIT 1001`, id).Scan(&prices).Error; err != nil {
			return fmt.Errorf("read admin prices: %w", err)
		}
		if len(prices) > 1000 {
			return fmt.Errorf("price version history exceeds bound")
		}
		for _, p := range prices {
			result.Prices = append(result.Prices, application.AdminPrice{SourceURL: p.SourceURL, ReviewedAt: p.ReviewedAt, ID: p.ID, VersionNo: p.VersionNo, Unit: p.Unit, Rule: json.RawMessage(p.Rule), Currency: p.Currency, FXRateToCNY: p.FXRateToCNY, EffectiveFrom: p.EffectiveFrom, CreateTime: p.CreateTime})
		}
		if selection := result.Candidate; selection != nil {
			providerModelID := selection.Model.ProviderModelID
			if len(versions) > 0 {
				providerModelID = versions[0].ProviderModelID
			}
			if selection.Model.AdapterKey != "" && providerModelID != "" {
				var candidates []adminPriceCandidateRow
				if err := tx.Raw(`SELECT id,provider_model_id,source_url,source_hash,parser_version,source_fetched_at,
					currency::text AS currency,unit,rule::text AS rule,source_pricing::text AS source_pricing,status
					FROM catalog.model_price_candidate WHERE adapter_key=? AND provider_model_id=?
					ORDER BY source_fetched_at DESC,id DESC LIMIT 21`, selection.Model.AdapterKey, providerModelID).Scan(&candidates).Error; err != nil {
					return fmt.Errorf("read model price candidates: %w", err)
				}
				if len(candidates) > 20 {
					candidates = candidates[:20]
				}
				for _, candidate := range candidates {
					result.PriceCandidates = append(result.PriceCandidates, application.ModelPriceCandidate{
						ID: candidate.ID, ProviderModelID: candidate.ProviderModelID, SourceURL: candidate.SourceURL,
						SourceHash: candidate.SourceHash, ParserVersion: candidate.ParserVersion, SourceFetchedAt: candidate.SourceFetchedAt,
						Currency: candidate.Currency, Unit: candidate.Unit, Rule: json.RawMessage(candidate.Rule),
						SourcePricing: json.RawMessage(candidate.SourcePricing), Status: candidate.Status,
					})
				}
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err == nil && result.Candidate != nil && result.Candidate.Model.AdapterKey != "" {
		var candidates []adminPriceCandidateRow
		queryErr := s.db.WithContext(ctx).Raw(`SELECT id,provider_model_id,source_url,source_hash,parser_version,source_fetched_at,
			currency::text AS currency,unit,rule::text AS rule,source_pricing::text AS source_pricing,status
		FROM catalog.model_price_candidate
		WHERE adapter_key=? AND provider_model_id=(
			SELECT provider_model_id FROM catalog.model_profile_version
			WHERE model_profile_id=?::uuid AND NOT is_delete ORDER BY version_no DESC LIMIT 1)
		ORDER BY source_fetched_at DESC,id DESC LIMIT 21`,
			result.Candidate.Model.AdapterKey, id.String()).Scan(&candidates).Error
		if queryErr != nil {
			return result, fmt.Errorf("read model price candidates: %w", queryErr)
		}
		if len(candidates) > 20 {
			candidates = candidates[:20]
		}
		for _, candidate := range candidates {
			result.PriceCandidates = append(result.PriceCandidates, application.ModelPriceCandidate{
				ID: candidate.ID, ProviderModelID: candidate.ProviderModelID, SourceURL: candidate.SourceURL,
				SourceHash: candidate.SourceHash, ParserVersion: candidate.ParserVersion, SourceFetchedAt: candidate.SourceFetchedAt,
				Currency: candidate.Currency, Unit: candidate.Unit, Rule: json.RawMessage(candidate.Rule),
				SourcePricing: json.RawMessage(candidate.SourcePricing), Status: candidate.Status,
			})
		}
	}
	return result, err
}

// ListAdminCapabilities returns the existing capability contracts for management.
func (s *Store) ListAdminCapabilities(ctx context.Context, actor identityapp.Principal) ([]application.CapabilitySummary, error) {
	items := []application.CapabilitySummary{}
	if s == nil || s.db == nil {
		return nil, application.ErrInvalidAdminModels
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requirePlatformAdmin(tx, actor); err != nil {
			return err
		}
		var rows []struct {
			ID                                 uuid.UUID
			Key, OutputType, Modes, InputRoles string
		}
		if err := tx.Raw(`SELECT id,key,output_type,to_jsonb(modes)::text AS modes,to_jsonb(input_roles)::text AS input_roles FROM catalog.capability WHERE NOT is_delete ORDER BY key LIMIT 1001`).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read capabilities: %w", err)
		}
		if len(rows) > 1000 {
			return fmt.Errorf("capability catalog exceeds bound")
		}
		for _, row := range rows {
			item := application.CapabilitySummary{ID: row.ID, Key: row.Key, OutputType: row.OutputType}
			if err := json.Unmarshal([]byte(row.Modes), &item.Modes); err != nil {
				return fmt.Errorf("decode capability modes: %w", err)
			}
			if err := json.Unmarshal([]byte(row.InputRoles), &item.InputRoles); err != nil {
				return fmt.Errorf("decode capability roles: %w", err)
			}
			items = append(items, item)
		}
		return nil
	})
	return items, err
}

var _ application.AdminModelsStore = (*Store)(nil)
