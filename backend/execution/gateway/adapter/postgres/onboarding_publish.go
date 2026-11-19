package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

type onboardingAdmission func(context.Context, *gorm.DB, domain.Call) error

// WithOnboarding binds a worker store to one approved validation and its media preparation.
func (s *Store) WithOnboarding(id uuid.UUID, prepare func(context.Context, *gorm.DB, domain.Call) error) *Store {
	bound := *s
	bound.onboarding = id
	bound.prepareOnboarding = prepare
	return &bound
}

func (s *Store) checkOnboardingCall(ctx context.Context, tx *gorm.DB, call domain.Call) error {
	if s.onboarding == uuid.Nil || call.Run != s.onboarding || call.Origin != application.OnboardingOrigin || call.Actor != s.actor.ID {
		return domain.ErrForbidden
	}
	if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
		return err
	}
	row, work, err := loadOnboarding(tx, s.onboarding, true)
	if err != nil {
		return err
	}
	if row.State != "verifying" || work.Actor.ID != call.Actor || work.Input.Scope != call.Scope ||
		work.Input.Project != call.Project || work.Model.ID != call.Model.ID {
		return domain.ErrForbidden
	}
	bound, err := application.PurchaseBound(work.Model, work.Input.Route.Purchase)
	if err != nil || bound > work.Plan.PurchaseMicros/int64(len(work.Plan.Probes)) {
		return domain.ErrBudget
	}
	for i, probe := range work.Plan.Probes {
		expected, err := application.OnboardingCall(work.Input, work.Model, work.Actor.ID, probe, i)
		if err == nil && expected.ID == call.ID && expected.Hash == call.Hash && expected.Key == call.Key {
			return checkPrevious(tx, row, work.Model)
		}
	}
	return domain.ErrForbidden
}

// PublishVerified enables the complete candidate and preserves the previous version until commit.
func (s *Store) PublishVerified(ctx context.Context, id uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, work, err := loadOnboarding(tx, id, false)
		if err != nil {
			return err
		}
		if err := lockAlias(tx, work.Model.Name); err != nil {
			return err
		}
		row, work, err := loadOnboarding(tx, id, true)
		if err != nil || row.State != "verifying" {
			return err
		}
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		if work.Actor.ID != s.actor.ID {
			return domain.ErrForbidden
		}
		if err = checkPrevious(tx, row, work.Model); err != nil {
			if errors.Is(err, domain.ErrConflict) {
				return tx.Exec("UPDATE ai.onboarding SET state='stale' WHERE id=?", id).Error
			}
			return err
		}
		for i, probe := range work.Plan.Probes {
			call, err := application.OnboardingCall(work.Input, work.Model, work.Actor.ID, probe, i)
			if err != nil {
				return err
			}
			record, err := onboardingRecord(tx, call)
			if err != nil {
				return err
			}
			if record.State != "settled" || !application.ProbePassed(work.Model.Protocol, probe, record.Receipt) {
				return domain.ErrUnknown
			}
			if record.Receipt.PurchaseMicros > work.Plan.PurchaseMicros/int64(len(work.Plan.Probes)) {
				return domain.ErrBudget
			}
			if work.Model.Protocol == "media" {
				var archived bool
				if err := tx.Raw("SELECT EXISTS(SELECT 1 FROM ai.output WHERE request_id=? AND state IN ('pending','ready'))", call.ID).Row().Scan(&archived); err != nil {
					return err
				}
				if !archived {
					return domain.ErrUnknown
				}
			}
		}

		if err := lockOnboardingModels(tx, row); err != nil {
			return err
		}
		// Check and hold the precise credential and route through alias activation.
		route := work.Input.Route
		var eligible struct{ ID uuid.UUID }
		q := tx.Raw("SELECT r.id FROM ai.route r JOIN ai.credential c ON c.id=r.credential_id WHERE r.id=? AND r.model_id=? AND NOT r.enabled AND c.state='active' FOR UPDATE OF r FOR SHARE OF c", route.ID, work.Model.ID).Scan(&eligible)
		if q.Error != nil {
			return q.Error
		}
		if eligible.ID != route.ID {
			return domain.ErrUnavailable
		}
		if work.Input.Replacement != uuid.Nil {
			changed := tx.Exec("UPDATE ai.route SET enabled=false WHERE id=? AND model_id=? AND enabled", work.Input.Replacement, work.Model.ID)
			if changed.Error != nil {
				return changed.Error
			}
			if changed.RowsAffected != 1 {
				return tx.Exec("UPDATE ai.onboarding SET state='stale' WHERE id=?", id).Error
			}
		}
		if row.PreviousModelID != nil && *row.PreviousModelID != work.Model.ID {
			if err := tx.Exec("UPDATE ai.model SET enabled=false WHERE id=? AND enabled", *row.PreviousModelID).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("UPDATE ai.model SET enabled=true,draft=false WHERE id=?", work.Model.ID).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE ai.route SET enabled=true WHERE id=?", id).Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE ai.onboarding SET state='active' WHERE id=?", id).Error
	})
}

// RollbackOnboarding stops this verified route and restores a still-qualified predecessor.
func (s *Store) RollbackOnboarding(ctx context.Context, id uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		_, work, err := loadOnboarding(tx, id, false)
		if err != nil {
			return err
		}
		if err := lockAlias(tx, work.Model.Name); err != nil {
			return err
		}
		row, work, err := loadOnboarding(tx, id, true)
		if err != nil || row.State == "rolled_back" {
			return err
		}
		live, err := activeModel(tx, work.Model)
		if err != nil {
			return err
		}
		if row.State != "active" || live.ID != work.Model.ID {
			return domain.ErrConflict
		}
		if err := lockOnboardingModels(tx, row); err != nil {
			return err
		}
		disabled := tx.Exec("UPDATE ai.route SET enabled=false WHERE id=? AND enabled", id)
		if disabled.Error != nil {
			return disabled.Error
		}
		if disabled.RowsAffected != 1 {
			return domain.ErrConflict
		}
		// A route-only addition may have other live channels. Keep their model
		// alias and restore the exact replaced route when its key is still valid.
		if work.Input.Replacement != uuid.Nil {
			if _, err := enableEligibleRoute(tx, work.Model, work.Input.Replacement); err != nil {
				return err
			}
		}
		remaining, err := qualifiedRoute(tx, work.Model)
		if err != nil {
			return err
		}
		if !remaining {
			if err := tx.Exec("UPDATE ai.model SET enabled=false WHERE id=?", work.Model.ID).Error; err != nil {
				return err
			}
			if row.PreviousModelID != nil && *row.PreviousModelID != work.Model.ID {
				var previous struct{ Document []byte }
				q := tx.Raw("SELECT document FROM ai.model WHERE id=? AND NOT draft FOR UPDATE", *row.PreviousModelID).Scan(&previous)
				if q.Error != nil {
					return q.Error
				}
				var model domain.Model
				if q.RowsAffected == 1 && json.Unmarshal(previous.Document, &model) == nil {
					ok, err := qualifiedRoute(tx, model)
					if err != nil {
						return err
					}
					if ok {
						if err := tx.Exec("UPDATE ai.model SET enabled=true WHERE id=?", model.ID).Error; err != nil {
							return err
						}
					}
				}
			}
		}
		return tx.Exec("UPDATE ai.onboarding SET state='rolled_back' WHERE id=?", id).Error
	})
}

func enableEligibleRoute(tx *gorm.DB, model domain.Model, id uuid.UUID) (bool, error) {
	var row struct{ Document []byte }
	q := tx.Raw("SELECT r.document FROM ai.route r JOIN ai.credential c ON c.id=r.credential_id WHERE r.id=? AND r.model_id=? AND c.state='active' FOR UPDATE OF r FOR SHARE OF c", id, model.ID).Scan(&row)
	if q.Error != nil || q.RowsAffected != 1 {
		return false, q.Error
	}
	if !withinPurchaseCap(model, row.Document) {
		return false, nil
	}
	return true, tx.Exec("UPDATE ai.route SET enabled=true WHERE id=?", id).Error
}

func qualifiedRoute(tx *gorm.DB, model domain.Model) (bool, error) {
	var routes []struct{ Document []byte }
	err := tx.Raw("SELECT r.document FROM ai.route r JOIN ai.credential c ON c.id=r.credential_id WHERE r.model_id=? AND r.enabled AND c.state='active' ORDER BY r.id FOR SHARE OF r,c", model.ID).Scan(&routes).Error
	for _, route := range routes {
		if withinPurchaseCap(model, route.Document) {
			return true, err
		}
	}
	return false, err
}

func withinPurchaseCap(model domain.Model, document []byte) bool {
	var route domain.Route
	if json.Unmarshal(document, &route) != nil || route.Protocol != model.Protocol || route.ModelID != model.ID {
		return false
	}
	cost, err := application.PurchaseBound(model, route.Purchase)
	return err == nil && cost <= model.PurchaseCap
}

func lockOnboardingModels(tx *gorm.DB, row onboardingRow) error {
	return tx.Exec("SELECT id FROM ai.model WHERE id IN (?,?) ORDER BY id FOR UPDATE", row.ModelID, row.PreviousModelID).Error
}
