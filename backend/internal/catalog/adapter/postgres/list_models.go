package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

// ErrModelCatalogProjectNotFound also hides projects outside the actor's organization.
var ErrModelCatalogProjectNotFound = application.ErrModelCatalogProjectNotFound

// ListModelsForProject reads a bounded page after locking the live actor,
// organization, and project in the same transaction as the catalog query.
func (s *Store) ListModelsForProject(ctx context.Context, actor identityapp.Principal, input application.ListModelsInput) (application.ModelCatalogPage, error) {
	if s == nil || s.db == nil || input.ProjectID == uuid.Nil || input.Limit < 0 || input.Limit > 200 ||
		(input.After != nil && (strings.TrimSpace(input.After.Key) == "" || input.After.ID == uuid.Nil)) {
		return application.ModelCatalogPage{}, application.ErrInvalidModelCatalog
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil {
		return application.ModelCatalogPage{}, identityapp.ErrForbidden
	}
	if input.Limit == 0 {
		input.Limit = 50
	}
	page := application.ModelCatalogPage{Models: []application.ModelCatalogItem{}}
	err := platformdb.ReadSnapshot(ctx, s.db, func(tx *gorm.DB) error {
		page = application.ModelCatalogPage{Models: []application.ModelCatalogItem{}}
		if err := s.requireProject(tx, actor, input.ProjectID); err != nil {
			return err
		}
		var project struct{ AllowOverseasModels bool }
		result := tx.Raw(`
			SELECT allow_overseas_models
			FROM workspace.project
			WHERE id = ?::uuid AND org_id = ?::uuid
			  AND status = 'active' AND NOT is_delete
			FOR SHARE
		`, input.ProjectID.String(), actor.OrgID.String()).Scan(&project)
		if result.Error != nil {
			return fmt.Errorf("read model catalog project: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrModelCatalogProjectNotFound
		}
		query := fmt.Sprintf(`
			SELECT m.id, published.id AS release_id, m.model_key, m.display_name, m.capability, m.status,
			       to_jsonb(cap.input_roles)::text AS input_roles,
			       p.id AS provider_id, p.name AS provider_name, p.adapter_key AS provider_adapter_key,
			       p.region AS provider_region, p.status AS provider_status,
			       (c.id IS NOT NULL) AS credential_present,
			       c.last_test_result AS credential_test_result,
			       v.id AS version_id, v.version_no AS version_no,
			       to_jsonb(v.modes)::text AS version_modes,
			       v.limits::text AS version_limits,
			       v.param_schema::text AS version_param_schema,
			       pr.source_url AS price_source_url,pr.reviewed_at AS price_reviewed_at,pr.id AS price_id, pr.version_no AS price_version_no,
			       pr.unit AS price_unit,
			       pr.currency::text AS price_currency,
			       pr.fx_rate_to_cny::text AS price_fx_rate_to_cny,
			       pr.effective_from AS price_effective_from
			FROM catalog.model_profile AS m
			JOIN catalog.provider AS p ON p.id = m.provider_id AND NOT p.is_delete
			LEFT JOIN catalog.model_release AS published ON published.id=m.active_release_id AND published.status='active'
			JOIN catalog.capability AS cap ON cap.key = m.capability AND NOT cap.is_delete
			LEFT JOIN catalog.model_profile_version AS v
			  ON v.id = CASE WHEN p.adapter_key IN (%[1]s) THEN published.model_version_id ELSE m.current_version_id END AND v.model_profile_id = m.id AND NOT v.is_delete
			LEFT JOIN catalog.provider_credential AS c
			  ON c.provider_id = p.id AND c.status = 'active' AND NOT c.is_delete
			LEFT JOIN LATERAL (
			  SELECT id, version_no, unit, rule, currency, fx_rate_to_cny, source_url, reviewed_at, effective_from
			  FROM catalog.price_rule_version
			  WHERE (p.adapter_key NOT IN (%[1]s) OR id=published.price_version_id) AND model_profile_id = m.id AND effective_from <= transaction_timestamp()
			    AND NOT is_delete
			  ORDER BY effective_from DESC, version_no DESC
			  LIMIT 1
			) AS pr ON true
			WHERE NOT m.is_delete AND m.status <> 'archived' AND p.status <> 'archived' AND (p.adapter_key NOT IN (%[1]s) OR EXISTS (SELECT 1 FROM catalog.provider_config_version pc JOIN catalog.provider_preset_version pv ON pv.id=pc.preset_version_id JOIN catalog.provider_preset preset ON preset.id=pv.preset_id WHERE pc.id=published.config_version_id AND NOT preset.emergency_disabled)) AND (? OR p.region = 'domestic')
			  AND (? = '' OR m.model_key = ?)
			  AND (? = '' OR m.capability = ?)
			  AND (? = '' OR ? = ANY(v.modes))
		`, domain.ManagedCloudSQLInList())
		args := []any{project.AllowOverseasModels, input.ModelKey, input.ModelKey, input.Capability, input.Capability, input.Mode, input.Mode}
		if input.After != nil {
			query += ` AND (m.model_key, m.id) > (?, ?::uuid)`
			args = append(args, input.After.Key, input.After.ID.String())
		}
		baseQuery, baseArgs := query, append([]any{}, args...)
		var scanAfter *application.ModelCatalogCursor
		more := false
		for {
			batchQuery, batchArgs := baseQuery, append([]any{}, baseArgs...)
			if scanAfter != nil {
				batchQuery += ` AND (m.model_key,m.id)>(?,?::uuid)`
				batchArgs = append(batchArgs, scanAfter.Key, scanAfter.ID)
			}
			batchQuery += ` ORDER BY m.model_key,m.id LIMIT 200`
			var rows []modelCatalogRow
			if err := tx.Raw(batchQuery, batchArgs...).Scan(&rows).Error; err != nil {
				return fmt.Errorf("read project model catalog: %w", err)
			}
			ids := make([]uuid.UUID, 0, len(rows))
			for _, row := range rows {
				ids = append(ids, row.ID)
			}
			availability, err := readModelAvailability(tx, ids)
			if err != nil {
				return err
			}
			availableRows := make([]modelCatalogRow, 0, len(rows))
			for _, row := range rows {
				if availability[row.ID].AvailableForMode(input.Mode) {
					availableRows = append(availableRows, row)
				}
			}
			if err := s.loadModelCatalogPriceRules(ctx, tx, availableRows); err != nil {
				return err
			}
			for _, row := range availableRows {
				item, err := row.item()
				if err != nil {
					return err
				}
				item.ModelAvailability = availability[row.ID]
				page.Models = append(page.Models, item)
				if len(page.Models) > input.Limit {
					more = true
					break
				}
			}
			if more || len(rows) < 200 {
				break
			}
			last := rows[len(rows)-1]
			scanAfter = &application.ModelCatalogCursor{Key: last.ModelKey, ID: last.ID}
		}
		if more {
			page.Models = page.Models[:input.Limit]
		}

		if more {
			last := page.Models[len(page.Models)-1]
			page.Next = &application.ModelCatalogCursor{Key: last.Key, ID: last.ID}
		}
		return nil
	})
	if err != nil {
		return application.ModelCatalogPage{}, fmt.Errorf("list models for project: %w", err)
	}
	return page, nil
}

func (s *Store) requireProject(tx *gorm.DB, actor identityapp.Principal, project uuid.UUID) error {
	if s.projects == nil || s.projects(tx) == nil {
		return identityapp.ErrForbidden
	}
	return s.projects(tx).AuthorizeCatalog(tx.Statement.Context, actor, project)
}

type modelCatalogRow struct {
	ID                   uuid.UUID
	ReleaseID            *uuid.UUID
	ModelKey             string
	DisplayName          string
	Capability           string
	InputRoles           string
	Status               string
	ProviderID           uuid.UUID
	ProviderName         string
	ProviderAdapterKey   string
	ProviderRegion       string
	ProviderStatus       string
	CredentialPresent    bool
	CredentialTestResult *string
	VersionID            *uuid.UUID
	VersionNo            *int
	VersionModes         *string
	VersionLimits        *string
	VersionParamSchema   *string
	PriceSourceURL       *string
	PriceReviewedAt      *time.Time
	PriceID              *uuid.UUID
	PriceVersionNo       *int
	PriceUnit            *string
	PriceRule            *string
	PriceCurrency        *string
	PriceFXRateToCNY     *string
	PriceEffectiveFrom   *time.Time
}

func (row modelCatalogRow) item() (application.ModelCatalogItem, error) {
	var roles []string
	if err := json.Unmarshal([]byte(row.InputRoles), &roles); err != nil {
		return application.ModelCatalogItem{}, fmt.Errorf("decode model input roles: %w", err)
	}
	item := application.ModelCatalogItem{
		ID: row.ID, Key: row.ModelKey, DisplayName: row.DisplayName,
		Capability: row.Capability, Status: domain.ModelStatus(row.Status),
		InputRoles: roles,
		ProviderID: row.ProviderID, ProviderName: row.ProviderName, ProviderAdapterKey: row.ProviderAdapterKey,
		ProviderRegion:    domain.Region(row.ProviderRegion),
		ProviderStatus:    domain.ProviderStatus(row.ProviderStatus),
		CredentialPresent: row.CredentialPresent,
	}
	if row.ReleaseID != nil {
		item.ReleaseID = *row.ReleaseID
	}
	if row.CredentialTestResult != nil {
		result := domain.TestResult(*row.CredentialTestResult)
		item.CredentialTestResult = &result
	}
	if row.VersionID != nil {
		if row.VersionNo == nil || row.VersionModes == nil || row.VersionLimits == nil || row.VersionParamSchema == nil {
			return application.ModelCatalogItem{}, application.ErrInvalidModelCatalog
		}
		var modes []string
		if err := json.Unmarshal([]byte(*row.VersionModes), &modes); err != nil {
			return application.ModelCatalogItem{}, fmt.Errorf("decode current model modes: %w", err)
		}
		var limits struct {
			ContextWindow int `json:"context_window"`
		}
		if err := json.Unmarshal([]byte(*row.VersionLimits), &limits); err != nil {
			return application.ModelCatalogItem{}, fmt.Errorf("decode current model limits: %w", err)
		}
		item.CurrentVersion = &application.ModelCatalogVersion{
			ID: *row.VersionID, VersionNo: *row.VersionNo, ContextWindow: limits.ContextWindow, Modes: modes,
			Limits:      json.RawMessage(*row.VersionLimits),
			ParamSchema: json.RawMessage(*row.VersionParamSchema),
		}
	}
	if row.PriceID != nil {
		if row.PriceVersionNo == nil || row.PriceUnit == nil || row.PriceRule == nil ||
			row.PriceCurrency == nil || row.PriceEffectiveFrom == nil {
			return application.ModelCatalogItem{}, application.ErrInvalidModelCatalog
		}
		item.CurrentPrice = &application.ModelCatalogPrice{
			SourceURL: row.PriceSourceURL, ReviewedAt: row.PriceReviewedAt, ID: *row.PriceID, VersionNo: *row.PriceVersionNo, Unit: *row.PriceUnit,
			Rule: json.RawMessage(*row.PriceRule), Currency: *row.PriceCurrency,
			FXRateToCNY: row.PriceFXRateToCNY, EffectiveFrom: *row.PriceEffectiveFrom,
		}
	}
	return item, nil
}

var _ application.ListModelsStore = (*Store)(nil)
