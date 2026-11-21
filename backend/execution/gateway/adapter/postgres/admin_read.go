package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
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
		if err := tx.Raw("SELECT document,enabled,draft FROM ai.model ORDER BY name,id LIMIT 500").Scan(&models).Error; err != nil {
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
		candidates := []OnboardingSummary{}
		if err := tx.Raw(`SELECT id,model_id,state,document->'route'->>'upstream_model' AS upstream FROM ai.onboarding ORDER BY created_at DESC,id LIMIT 100`).Scan(&candidates).Error; err != nil {
			return err
		}
		out["onboarding"] = candidates
		var routes []struct {
			Document []byte
			Enabled  bool
		}
		if err := tx.Raw("SELECT document,enabled FROM ai.route ORDER BY model_id,priority,id LIMIT 1000").Scan(&routes).Error; err != nil {
			return err
		}
		supplies := []map[string]any{}
		for _, row := range routes {
			var route domain.Route
			if json.Unmarshal(row.Document, &route) != nil {
				return domain.ErrInvalid
			}
			supplies = append(supplies, map[string]any{"route": route, "enabled": row.Enabled})
		}
		out["routes"] = supplies
		var credentials []struct {
			ID         uuid.UUID `json:"id"`
			SupplierID uuid.UUID `json:"supplier_id"`
			State      string    `json:"state"`
		}
		if err := tx.Raw("SELECT id,supplier_id,state FROM ai.credential ORDER BY id LIMIT 1000").Scan(&credentials).Error; err != nil {
			return err
		}
		out["credentials"] = credentials
		var quotas []struct {
			Name        string `json:"name"`
			Concurrency int    `json:"concurrency"`
			RPM         int    `json:"rpm"`
		}
		if err := tx.Raw("SELECT name,concurrency,requests_per_minute AS rpm FROM ai.quota ORDER BY name LIMIT 1000").Scan(&quotas).Error; err != nil {
			return err
		}
		out["quotas"] = quotas
		var unknown []struct {
			RequestID  uuid.UUID `json:"request_id"`
			AttemptID  uuid.UUID `json:"attempt_id"`
			ScopeID    uuid.UUID `json:"scope_id"`
			State      string    `json:"state"`
			UpstreamID string    `json:"upstream_id,omitempty"`
		}
		if err := tx.Raw(`SELECT r.id AS request_id,a.id AS attempt_id,r.scope_id,r.state,
 COALESCE(a.receipt->>'upstream_id',p.task_id,'') AS upstream_id
 FROM ai.request r JOIN ai.attempt a ON a.request_id=r.id
 LEFT JOIN ai.pending p ON p.request_id=r.id AND p.attempt_id=a.id
 WHERE r.state='unknown' AND (a.state='unknown' OR (a.state='received' AND a.receipt->>'purchase_pending'='true'))
 ORDER BY a.started_at LIMIT 100`).Scan(&unknown).Error; err != nil {
			return err
		}
		out["unknown"] = unknown
		return nil
	})
	return out, err
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
