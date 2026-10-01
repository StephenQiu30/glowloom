package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
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
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword ||
		(actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer) {
		return application.ModelCatalogPage{}, identityapp.ErrForbidden
	}
	if input.Limit == 0 {
		input.Limit = 50
	}
	page := application.ModelCatalogPage{Models: []application.ModelCatalogItem{}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentModelCatalogActor(tx, actor); err != nil {
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
		query := `
			SELECT m.id, m.model_key, m.display_name, m.capability, m.status,
			       to_jsonb(cap.input_roles)::text AS input_roles,
			       p.id AS provider_id, p.name AS provider_name,
			       p.region AS provider_region, p.status AS provider_status,
			       (c.id IS NOT NULL) AS credential_present,
			       c.last_test_result AS credential_test_result,
			       v.id AS version_id, v.version_no AS version_no,
			       to_jsonb(v.modes)::text AS version_modes,
			       v.limits::text AS version_limits,
			       v.param_schema::text AS version_param_schema,
			       pr.id AS price_id, pr.version_no AS price_version_no,
			       pr.unit AS price_unit, pr.rule::text AS price_rule,
			       pr.currency::text AS price_currency,
			       pr.fx_rate_to_cny::text AS price_fx_rate_to_cny,
			       pr.effective_from AS price_effective_from
			FROM catalog.model_profile AS m
			JOIN catalog.provider AS p ON p.id = m.provider_id AND NOT p.is_delete
			JOIN catalog.capability AS cap ON cap.key = m.capability AND NOT cap.is_delete
			LEFT JOIN catalog.model_profile_version AS v
			  ON v.id = m.current_version_id AND v.model_profile_id = m.id AND NOT v.is_delete
			LEFT JOIN catalog.provider_credential AS c
			  ON c.provider_id = p.id AND c.status = 'active' AND NOT c.is_delete
			LEFT JOIN LATERAL (
			  SELECT id, version_no, unit, rule, currency, fx_rate_to_cny, effective_from
			  FROM catalog.price_rule_version
			  WHERE model_profile_id = m.id AND effective_from <= transaction_timestamp()
			    AND NOT is_delete
			  ORDER BY effective_from DESC, version_no DESC
			  LIMIT 1
			) AS pr ON true
			WHERE NOT m.is_delete AND (? OR p.region = 'domestic')
			  AND (? = '' OR m.capability = ?)
			  AND (? = '' OR ? = ANY(v.modes))
		`
		args := []any{project.AllowOverseasModels, input.Capability, input.Capability, input.Mode, input.Mode}
		if input.After != nil {
			query += ` AND (m.model_key, m.id) > (?, ?::uuid)`
			args = append(args, input.After.Key, input.After.ID.String())
		}
		query += ` ORDER BY m.model_key, m.id LIMIT ?`
		args = append(args, input.Limit+1)
		var rows []modelCatalogRow
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read project model catalog: %w", err)
		}
		more := len(rows) > input.Limit
		if more {
			rows = rows[:input.Limit]
		}
		for _, row := range rows {
			item, err := row.item()
			if err != nil {
				return err
			}
			page.Models = append(page.Models, item)
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

func requireCurrentModelCatalogActor(tx *gorm.DB, actor identityapp.Principal) error {
	var present int
	result := tx.Raw(`
		SELECT 1
		FROM identity."user" AS u
		JOIN workspace.organization AS o ON o.id = u.org_id
		WHERE u.id = ?::uuid AND u.org_id = ?::uuid
		  AND u.role IN ('admin', 'producer')
		  AND u.status = 'active' AND NOT u.is_delete AND NOT u.must_change_password
		  AND o.status = 'active' AND NOT o.is_delete
		FOR SHARE OF u, o
	`, actor.ID.String(), actor.OrgID.String()).Scan(&present)
	if result.Error != nil {
		return fmt.Errorf("check current model catalog actor: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return identityapp.ErrForbidden
	}
	return nil
}

type modelCatalogRow struct {
	ID                   uuid.UUID
	ModelKey             string
	DisplayName          string
	Capability           string
	InputRoles           string
	Status               string
	ProviderID           uuid.UUID
	ProviderName         string
	ProviderRegion       string
	ProviderStatus       string
	CredentialPresent    bool
	CredentialTestResult *string
	VersionID            *uuid.UUID
	VersionNo            *int
	VersionModes         *string
	VersionLimits        *string
	VersionParamSchema   *string
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
		ProviderID: row.ProviderID, ProviderName: row.ProviderName,
		ProviderRegion:    domain.Region(row.ProviderRegion),
		ProviderStatus:    domain.ProviderStatus(row.ProviderStatus),
		CredentialPresent: row.CredentialPresent,
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
		item.CurrentVersion = &application.ModelCatalogVersion{
			ID: *row.VersionID, VersionNo: *row.VersionNo, Modes: modes,
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
			ID: *row.PriceID, VersionNo: *row.PriceVersionNo, Unit: *row.PriceUnit,
			Rule: json.RawMessage(*row.PriceRule), Currency: *row.PriceCurrency,
			FXRateToCNY: row.PriceFXRateToCNY, EffectiveFrom: *row.PriceEffectiveFrom,
		}
	}
	return item, nil
}

var _ application.ListModelsStore = (*Store)(nil)
