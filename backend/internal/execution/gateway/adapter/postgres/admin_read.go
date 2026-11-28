package postgres

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

// Configuration returns reviewed metadata and unresolved identities, never keys or private prompts.
func (s *Store) Configuration(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		var models []struct {
			Document []byte
			Enabled  bool
			Draft    bool
		}
		if err := tx.Raw("SELECT document,enabled,draft FROM ai.model_current WHERE document->>'protocol'<>'local' ORDER BY name,id LIMIT 500").Scan(&models).Error; err != nil {
			return err
		}
		products := []map[string]any{}
		for _, row := range models {
			var model domain.Model
			if json.Unmarshal(row.Document, &model) != nil {
				return domain.ErrInvalid
			}
			products = append(products, map[string]any{"model": model, "enabled": row.Enabled, "draft": row.Draft})
		}
		out["models"] = products
		presets, err := modelPresets(tx)
		if err != nil {
			return err
		}
		out["model_presets"] = presets
		out["key_endpoints"] = application.KeyEndpoints()
		grants, err := listCreditGrants(tx)
		if err != nil {
			return err
		}
		out["credit_grants"] = grants
		var candidateRows []struct {
			OnboardingSummary
			RouteDocument []byte
		}
		if err := tx.Raw(`SELECT id,model_id,state,document->'route'->>'upstream_model' AS upstream,
 document->>'scope_id' AS scope_id,document->'route' AS route_document FROM ai.onboarding WHERE deleted_at IS NULL
 ORDER BY created_at DESC,id LIMIT 100`).Scan(&candidateRows).Error; err != nil {
			return err
		}
		type candidateListing struct {
			OnboardingSummary
			ProviderKey string    `json:"provider_key,omitempty"`
			SupplierID  uuid.UUID `json:"supplier_id"`
			Group       string    `json:"group"`
		}
		candidates := make([]candidateListing, 0, len(candidateRows))
		for _, row := range candidateRows {
			var route domain.Route
			if json.Unmarshal(row.RouteDocument, &route) != nil {
				return domain.ErrInvalid
			}
			candidates = append(candidates, candidateListing{row.OnboardingSummary, routeProviderKey(route), route.Supplier, route.Group})
		}
		out["onboarding"] = candidates
		var routes []struct {
			Document []byte
			Enabled  bool
		}
		if err := tx.Raw("SELECT document,enabled FROM ai.route_current WHERE deleted_at IS NULL AND document->>'protocol'<>'local' ORDER BY model_id,priority,id LIMIT 1000").Scan(&routes).Error; err != nil {
			return err
		}
		supplies := []map[string]any{}
		for _, row := range routes {
			var route domain.Route
			if json.Unmarshal(row.Document, &route) != nil {
				return domain.ErrInvalid
			}
			supplies = append(supplies, map[string]any{"route": route, "enabled": row.Enabled, "provider_key": routeProviderKey(route)})
		}
		out["routes"] = supplies
		var credentials []struct {
			ID         uuid.UUID `json:"id"`
			SupplierID uuid.UUID `json:"supplier_id"`
			State      string    `json:"state"`
		}
		if err := tx.Raw("SELECT id,supplier_id,state FROM ai.credential WHERE key_id<>'local-policy' ORDER BY id LIMIT 1000").Scan(&credentials).Error; err != nil {
			return err
		}
		out["credentials"] = credentials
		var quotas []struct {
			Name        string `json:"name"`
			Concurrency int    `json:"concurrency"`
			RPM         int    `json:"rpm"`
		}
		if err := tx.Raw("SELECT name,concurrency,requests_per_minute AS rpm FROM ai.quota WHERE name<>'local/whisper' ORDER BY name LIMIT 1000").Scan(&quotas).Error; err != nil {
			return err
		}
		out["quotas"] = quotas
		var unknown []struct {
			RequestID   uuid.UUID `json:"request_id"`
			AttemptID   uuid.UUID `json:"attempt_id"`
			ScopeID     uuid.UUID `json:"scope_id"`
			State       string    `json:"state"`
			UpstreamID  string    `json:"upstream_id,omitempty"`
			TariffMeter string    `json:"tariff_meter,omitempty"`
			TimedTariff bool      `json:"timed_tariff,omitempty"`
		}
		if err := tx.Raw(`SELECT r.id AS request_id,a.id AS attempt_id,r.scope_id,r.state,
 COALESCE(a.receipt->>'upstream_id',p.task_id,'') AS upstream_id,
 COALESCE(r.contract#>>'{price,media_tariff,meter}','') AS tariff_meter,
 COALESCE(r.contract#>>'{price,text_tariff,schedule}',a.contract#>>'{purchase,text_tariff,schedule}','')<>'' AS timed_tariff
 FROM ai.request r JOIN ai.attempt a ON a.request_id=r.id
 LEFT JOIN ai.pending p ON p.request_id=r.id AND p.attempt_id=a.id
 WHERE (r.state='unknown' AND (a.state='unknown' OR (a.state='received' AND a.receipt->>'purchase_pending'='true')))
 OR (r.state='settled' AND a.output_duration_evidence IS NOT NULL
     AND NOT EXISTS (SELECT 1 FROM ai.reconciliation e WHERE e.request_id=r.id))
 ORDER BY a.started_at LIMIT 100`).Scan(&unknown).Error; err != nil {
			return err
		}
		out["unknown"] = unknown
		return nil
	})
	return out, err
}

// routeProviderKey identifies a display logo from the route's actual adapter or direct endpoint.
func routeProviderKey(route domain.Route) string {
	endpoint, err := url.Parse(route.BaseURL)
	if err != nil || endpoint.Scheme != "https" || (endpoint.Port() != "" && endpoint.Port() != "443") {
		return ""
	}
	host := strings.ToLower(endpoint.Hostname())
	if len(route.DriverConfig) > 0 {
		var driver struct {
			AdapterKey string `json:"adapter_key"`
		}
		if json.Unmarshal(route.DriverConfig, &driver) != nil {
			return ""
		}
		// The OpenAI adapter also accepts self-hosted compatible endpoints.
		if driver.AdapterKey == "openai" && host != "api.openai.com" {
			return ""
		}
		return driver.AdapterKey
	}
	switch host {
	case "api.openai.com":
		return "openai"
	case "openrouter.ai":
		return "openrouter"
	case "ark.cn-beijing.volces.com":
		return "volcengine_ark"
	case "ark.ap-southeast.bytepluses.com":
		return "byteplus"
	case "api.deepseek.com":
		return "deepseek"
	case "ai-gateway.vercel.sh":
		return "vercel"
	default:
		return ""
	}
}

// SetBudget changes a project spending limit only when it still covers authoritative commitments.
func (s *Store) SetBudget(ctx context.Context, scope, project uuid.UUID, credits int64) error {
	if credits < 0 || scope == uuid.Nil || project == uuid.Nil {
		return domain.ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, scope, uuid.Nil, true); err != nil {
			return err
		}
		var n int
		if err := tx.Raw("SELECT 1 FROM workspace.project WHERE id=? AND owner_scope_id=? AND NOT is_delete FOR SHARE", project, scope).Scan(&n).Error; err != nil {
			return err
		}
		if n != 1 {
			return domain.ErrForbidden
		}
		if err := tx.Exec("INSERT INTO ai.account(scope_id) VALUES(?) ON CONFLICT DO NOTHING", scope).Error; err != nil {
			return err
		}
		if err := tx.Exec("SELECT scope_id FROM ai.account WHERE scope_id=? FOR UPDATE", scope).Error; err != nil {
			return err
		}
		q := tx.Exec("INSERT INTO ai.budget(scope_id,id,limit_credits) VALUES(?,?,?) ON CONFLICT(scope_id,id) DO UPDATE SET limit_credits=excluded.limit_credits WHERE ai.budget.held+ai.budget.spent<=excluded.limit_credits", scope, project, credits)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrBudget
		}
		return nil
	})
}
