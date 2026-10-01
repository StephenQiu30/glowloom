package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
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
		if err := requireCurrentCatalogAdmin(tx, actor.ID, actor.OrgID); err != nil {
			return err
		}
		query := adminModelSelect + ` WHERE NOT is_delete`
		args := []any{}
		if input.After != nil {
			query += ` AND (model_key,id)>(?,?::uuid)`
			args = append(args, input.After.Key, input.After.ID)
		}
		query += ` ORDER BY model_key,id LIMIT ?`
		args = append(args, input.Limit+1)
		var rows []modelProfileRow
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read admin models: %w", err)
		}
		more := len(rows) > input.Limit
		if more {
			rows = rows[:input.Limit]
		}
		for _, row := range rows {
			page.Items = append(page.Items, createdModel(row))
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
	ID                        uuid.UUID
	VersionNo                 int
	Unit, Rule, Currency      string
	FXRateToCNY               *string
	EffectiveFrom, CreateTime time.Time
}

// ReadAdminModel reads existing immutable histories without provider credentials.
func (s *Store) ReadAdminModel(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (application.AdminModelDetail, error) {
	result := application.AdminModelDetail{Versions: []application.AdminModelVersion{}, Prices: []application.AdminPrice{}}
	if s == nil || s.db == nil || id == uuid.Nil {
		return result, application.ErrInvalidAdminModels
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actor.ID, actor.OrgID); err != nil {
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
		var prices []adminPriceRow
		if err := tx.Raw(`SELECT id,version_no,unit,rule::text AS rule,currency::text AS currency,
fx_rate_to_cny::text AS fx_rate_to_cny,effective_from,create_time FROM catalog.price_rule_version
WHERE model_profile_id=?::uuid AND NOT is_delete ORDER BY version_no DESC LIMIT 1001`, id).Scan(&prices).Error; err != nil {
			return fmt.Errorf("read admin prices: %w", err)
		}
		if len(prices) > 1000 {
			return fmt.Errorf("price version history exceeds bound")
		}
		for _, p := range prices {
			result.Prices = append(result.Prices, application.AdminPrice{ID: p.ID, VersionNo: p.VersionNo, Unit: p.Unit, Rule: json.RawMessage(p.Rule), Currency: p.Currency, FXRateToCNY: p.FXRateToCNY, EffectiveFrom: p.EffectiveFrom, CreateTime: p.CreateTime})
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	return result, err
}

// ListAdminCapabilities returns the existing capability contracts for management.
func (s *Store) ListAdminCapabilities(ctx context.Context, actor identityapp.Principal) ([]application.CapabilitySummary, error) {
	items := []application.CapabilitySummary{}
	if s == nil || s.db == nil {
		return nil, application.ErrInvalidAdminModels
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentCatalogAdmin(tx, actor.ID, actor.OrgID); err != nil {
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
