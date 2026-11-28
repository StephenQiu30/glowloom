package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

// SaleChange changes the price of an existing release without replacing its routes.
type SaleChange struct {
	Version int64        `json:"version"`
	Price   domain.Price `json:"price"`
}

// ModelSale validates a proposed price against the currently available release.
func (s *Store) ModelSale(ctx context.Context, id uuid.UUID, change SaleChange, publish bool) (domain.Model, error) {
	var model domain.Model
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		if publish {
			if err := tx.Exec("SELECT id FROM ai.model WHERE id=? FOR UPDATE", id).Error; err != nil {
				return err
			}
		}
		var row struct{ Document []byte }
		q := tx.Raw("SELECT document FROM ai.available_model WHERE id=?", id).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 || json.Unmarshal(row.Document, &model) != nil {
			return domain.ErrUnavailable
		}
		if model.SaleVersion != change.Version {
			return domain.ErrConflict
		}
		if !model.Price.SameSaleMeter(change.Price) {
			return domain.ErrInvalid
		}
		unchanged := string(mustJSON(model.Price)) == string(mustJSON(change.Price))
		model.Price = change.Price
		if err := model.Validate(); err != nil {
			return err
		}
		if !publish || unchanged {
			return nil
		}
		model.SaleVersion++
		return tx.Exec("INSERT INTO ai.model_sale(model_id,version,price,actor_id) VALUES(?,?,?::jsonb,?)",
			id, model.SaleVersion, string(mustJSON(change.Price)), s.actor.ID).Error
	})
	return model, err
}

// checkCallSale accepts the current exact contract or the caller's unexpired
// frozen quote. Onboarding probes use the immutable model configuration.
func checkCallSale(tx *gorm.DB, call domain.Call, verification, useQuote bool) error {
	var row struct {
		Enabled bool
		Matches bool
	}
	model := string(mustJSON(call.Model))
	q := tx.Raw("SELECT enabled, document=?::jsonb AS matches FROM ai.model WHERE id=? FOR SHARE", model, call.Model.ID).Scan(&row)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return domain.ErrUnavailable
	}
	if verification {
		if row.Matches {
			return nil
		}
		return domain.ErrUnavailable
	}
	var current bool
	if row.Enabled {
		if err := tx.Raw("SELECT document=?::jsonb FROM ai.model_current WHERE id=?", model, call.Model.ID).Row().Scan(&current); err != nil {
			return err
		}
	}
	if current {
		return nil
	}
	if useQuote {
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM ai.quote WHERE id=? AND scope_id=? AND actor_id=? AND input_hash=?
 AND contract->'Model'=?::jsonb AND contract->>'Origin' IS DISTINCT FROM ?
 AND expires_at>clock_timestamp())`, call.ID, call.Scope, call.Actor, call.Hash, model, application.OnboardingOrigin).Row().Scan(&current); err != nil {
			return err
		}
		if current {
			return nil
		}
	}
	return domain.ErrUnavailable
}

// PublishRoutePurchase updates procurement pricing for future attempts.
func (s *Store) PublishRoutePurchase(ctx context.Context, id uuid.UUID, change SaleChange) (domain.Route, error) {
	var route domain.Route
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		if err := tx.Exec("SELECT id FROM ai.route WHERE id=? AND deleted_at IS NULL FOR UPDATE", id).Error; err != nil {
			return err
		}
		var row struct{ Route, Model []byte }
		q := tx.Raw(`SELECT r.document AS route,m.document AS model FROM ai.route_current r
 JOIN ai.model m ON m.id=r.model_id
 WHERE r.id=? AND r.deleted_at IS NULL
 AND NOT EXISTS (SELECT 1 FROM ai.onboarding o WHERE o.id=r.id AND o.state='verifying')`, id).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		var model domain.Model
		if q.RowsAffected != 1 || json.Unmarshal(row.Route, &route) != nil || json.Unmarshal(row.Model, &model) != nil {
			return domain.ErrUnavailable
		}
		if route.PurchaseVersion != change.Version {
			return domain.ErrConflict
		}
		if change.Price.ExplicitFree || change.Price.Validate() != nil || !route.Purchase.SameSaleMeter(change.Price) {
			return domain.ErrInvalid
		}
		if string(mustJSON(route.Purchase)) == string(mustJSON(change.Price)) {
			return nil
		}
		route.Purchase = change.Price
		bound, err := application.PurchaseBound(model, route.Purchase)
		if err != nil || bound > model.PurchaseCap {
			return domain.ErrBudget
		}
		route.PurchaseVersion++
		return tx.Exec("INSERT INTO ai.route_purchase(route_id,version,price,actor_id) VALUES(?,?,?::jsonb,?)",
			id, route.PurchaseVersion, string(mustJSON(route.Purchase)), s.actor.ID).Error
	})
	return route, err
}
