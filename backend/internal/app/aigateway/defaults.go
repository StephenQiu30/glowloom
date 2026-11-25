package aigateway

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application"
	billingdomain "github.com/StephenQiu30/glowloom-server/backend/internal/execution/billing/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// Defaults keeps the owning project map and resolves every choice against the new catalog.
type Defaults struct{ Runtime *Runtime }

// ReadModelDefaults resolves current project choices from the rebuilt catalog.
func (d Defaults) ReadModelDefaults(ctx context.Context, a identity.Principal, id uuid.UUID) (workspace.ModelDefaults, error) {
	var out workspace.ModelDefaults
	err := d.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := Authority(ctx, tx, a, a.ScopeID, id, false); err != nil {
			return err
		}
		var row struct {
			Revision      int64
			DefaultModels []byte
		}
		q := tx.Raw("SELECT revision,default_models FROM workspace.project WHERE id=?", id).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrForbidden
		}
		out.ProjectID = id
		out.Revision = row.Revision
		return json.Unmarshal(row.DefaultModels, &out.DefaultModels)
	})
	return out, err
}

// SaveModelDefaults saves verified model versions with permanent command identity.
func (d Defaults) SaveModelDefaults(ctx context.Context, a identity.Principal, in workspace.ModelDefaultsChange, _ identity.OutboxEvent) (workspace.ModelDefaults, error) {
	var out workspace.ModelDefaults
	hash := domain.Digest(canonical(in))
	err := d.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := Authority(ctx, tx, a, a.ScopeID, in.ProjectID, false); err != nil {
			return err
		}
		if err := tx.Exec("SELECT id FROM workspace.project WHERE id=? FOR UPDATE", in.ProjectID).Error; err != nil {
			return err
		}
		var old struct {
			InputHash string
			Response  []byte
		}
		q := tx.Raw("SELECT input_hash,response FROM ai.project_command WHERE actor_id=? AND command_key=?", a.ID, in.IdempotencyKey).Scan(&old)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if old.InputHash != hash {
				return domain.ErrConflict
			}
			return json.Unmarshal(old.Response, &out)
		}
		for capability, name := range in.DefaultModels {
			var n int64
			if err := tx.Raw("SELECT count(*) FROM ai.available_model WHERE name=? AND capability=?", name, capability).Scan(&n).Error; err != nil {
				return err
			}
			if n != 1 {
				return workspace.ErrDefaultModelUnavailable
			}
		}
		q = tx.Exec("UPDATE workspace.project SET default_models=?::jsonb,revision=revision+1,update_time=clock_timestamp() WHERE id=? AND revision=? AND status='active'", string(canonical(in.DefaultModels)), in.ProjectID, in.ExpectedRevision)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrConflict
		}
		out = workspace.ModelDefaults{ProjectID: in.ProjectID, Revision: in.ExpectedRevision + 1, DefaultModels: in.DefaultModels}
		return tx.Exec("INSERT INTO ai.project_command(actor_id,command_key,input_hash,response) VALUES(?,?,?,?::jsonb)", a.ID, in.IdempotencyKey, hash, string(canonical(out))).Error
	})
	return out, err
}

// FindBudget projects the new optional project cap; an uncapped project is bounded by its actual wallet.
func (d Defaults) FindBudget(ctx context.Context, a identity.Principal, p uuid.UUID) (billingdomain.Budget, error) {
	out := billingdomain.Budget{ID: p, ProjectID: p, Revision: 1}
	err := d.Runtime.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := Authority(ctx, tx, a, a.ScopeID, p, false); err != nil {
			return err
		}
		var row struct{ LimitCredits, Held, Spent int64 }
		q := tx.Raw("SELECT limit_credits,held,spent FROM ai.budget WHERE scope_id=? AND id=?", a.ScopeID, p).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 0 {
			var available int64
			if err := tx.Raw("SELECT COALESCE((SELECT available FROM ai.account WHERE scope_id=?),0)", a.ScopeID).Row().Scan(&available); err != nil {
				return err
			}
			row.LimitCredits = available
		}
		if row.LimitCredits > 92233720368547 || row.Held > 92233720368547 || row.Spent > 92233720368547 {
			return domain.ErrInvalid
		}
		out.LimitMicros = row.LimitCredits * 100000
		out.ReservedMicros = row.Held * 100000
		out.SettledMicros = row.Spent * 100000
		return out.Validate()
	})
	return out, err
}
