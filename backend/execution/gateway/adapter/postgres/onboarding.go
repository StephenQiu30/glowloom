package postgres

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// OnboardingKind is the Execution worker job for an approved verification.
const OnboardingKind = "ai.onboard"

// OnboardingSummary is safe to list without test inputs, credentials or provider output.
type OnboardingSummary struct {
	ID       uuid.UUID `json:"id"`
	ModelID  uuid.UUID `json:"model_id"`
	State    string    `json:"state"`
	Upstream string    `json:"upstream_model"`
}

// OnboardingReview is the persisted verification plan and its current progress.
type OnboardingReview struct {
	OnboardingSummary
	Plan    application.OnboardingPlan `json:"plan"`
	Scope   uuid.UUID                  `json:"scope_id"`
	Project uuid.UUID                  `json:"project_id"`
	Probes  []OnboardingProgress       `json:"progress"`
}

// OnboardingProgress identifies a probe and its original ledger state.
type OnboardingProgress struct {
	RequestID uuid.UUID `json:"request_id"`
	State     string    `json:"state"`
}

type onboardingRow struct {
	ID, ModelID            uuid.UUID
	PreviousModelID        *uuid.UUID
	State                  string
	Document, Model, Actor []byte
}

type onboardingContract struct {
	application.Onboarding
	Plan application.OnboardingPlan `json:"plan"`
}

// OnboardingWork carries the immutable candidate and confirmed actor to the worker.
type OnboardingWork struct {
	Actor identity.Principal
	Input application.Onboarding
	Model domain.Model
	Plan  application.OnboardingPlan
	State string
}

func lockAlias(tx *gorm.DB, name string) error {
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "ai-model/"+name).Error
}

func activeModel(tx *gorm.DB, model domain.Model) (domain.Model, error) {
	var row struct{ Document []byte }
	r := tx.Raw("SELECT document FROM ai.model WHERE name=? AND document->>'protocol'=? AND enabled", model.Name, model.Protocol).Scan(&row)
	if r.Error != nil {
		return domain.Model{}, r.Error
	}
	var out domain.Model
	if r.RowsAffected == 1 && json.Unmarshal(row.Document, &out) != nil {
		return out, domain.ErrInvalid
	}
	return out, nil
}

// StageModel saves the reviewed preset and sale without changing public availability.
func (s *Store) StageModel(ctx context.Context, m domain.Model) error {
	if m.Validate() != nil || strings.TrimSpace(m.Name) != m.Name || len(m.Name) > 160 {
		return domain.ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		raw := string(mustJSON(m))
		if err := tx.Exec("INSERT INTO ai.model(id,name,document,draft) VALUES(?,?,?::jsonb,true) ON CONFLICT DO NOTHING", m.ID, m.Name, raw).Error; err != nil {
			return err
		}
		var same bool
		if err := tx.Raw("SELECT document=?::jsonb FROM ai.model WHERE id=?", raw, m.ID).Row().Scan(&same); err != nil {
			return err
		}
		if !same {
			return domain.ErrConflict
		}
		return nil
	})
}

// PrepareOnboarding freezes a route, procurement price and tests without a key or a send.
func (s *Store) PrepareOnboarding(ctx context.Context, in application.Onboarding) error {
	if in.Route.ID == uuid.Nil {
		return domain.ErrInvalid
	}
	in.Route.Credential = uuid.NewSHA1(in.Route.ID, []byte("credential"))
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		if err := s.check(ctx, tx, in.Scope, in.Project, false); err != nil {
			return err
		}
		var model domain.Model
		var snapshot struct{ Document []byte }
		q := tx.Raw("SELECT document FROM ai.model WHERE id=? AND (enabled OR draft)", in.Route.ModelID).Scan(&snapshot)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 || json.Unmarshal(snapshot.Document, &model) != nil {
			return domain.ErrInvalid
		}
		if err := validateRoute(model, in.Route, in.Concurrency, in.RPM); err != nil {
			return err
		}
		var incompatibleQuota bool
		if err := tx.Raw("SELECT EXISTS(SELECT 1 FROM ai.quota WHERE name=? AND (concurrency<>? OR requests_per_minute<>?))", in.Route.Group, in.Concurrency, in.RPM).Row().Scan(&incompatibleQuota); err != nil {
			return err
		}
		if incompatibleQuota {
			return domain.ErrConflict
		}
		plan, err := application.PlanOnboarding(in, model)
		if err != nil {
			return err
		}
		// Freeze normalized media probe inputs, including default parameters.
		for i := range in.MediaTests {
			in.MediaTests[i] = plan.Probes[i].Raw
		}
		if err = lockAlias(tx, model.Name); err != nil {
			return err
		}
		live, err := activeModel(tx, model)
		if err != nil {
			return err
		}
		if !preservesModes(live, model) {
			return domain.ErrInvalid
		}
		if in.Replacement != uuid.Nil {
			if live.ID != model.ID {
				return domain.ErrInvalid
			}
			var exists bool
			if err := tx.Raw("SELECT EXISTS(SELECT 1 FROM ai.route WHERE id=? AND model_id=? AND enabled)", in.Replacement, model.ID).Row().Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return domain.ErrConflict
			}
		}
		doc := string(mustJSON(onboardingContract{Onboarding: in, Plan: plan}))
		if err := tx.Exec("INSERT INTO ai.onboarding(id,model_id,previous_model_id,replaces_route_id,document) SELECT ?,?,?,?,?::jsonb WHERE NOT EXISTS (SELECT 1 FROM ai.route WHERE id=?) ON CONFLICT DO NOTHING", in.Route.ID, model.ID, nullable(live.ID), nullable(in.Replacement), doc, in.Route.ID).Error; err != nil {
			return err
		}
		var same bool
		if err := tx.Raw("SELECT EXISTS (SELECT 1 FROM ai.onboarding WHERE document=?::jsonb AND deleted_at IS NULL AND id=?)", doc, in.Route.ID).Row().Scan(&same); err != nil {
			return err
		}
		if !same {
			return domain.ErrConflict
		}
		return nil
	})
}

func preservesModes(old, next domain.Model) bool {
	if old.ID == uuid.Nil {
		return true
	}
	if old.Tools && !next.Tools || old.Stream && !next.Stream || old.Capability != next.Capability && old.Protocol == "media" {
		return false
	}
	for _, mode := range old.Modes {
		if !slices.Contains(next.Modes, mode) {
			return false
		}
	}
	return true
}

func loadOnboarding(tx *gorm.DB, id uuid.UUID, lock bool) (onboardingRow, OnboardingWork, error) {
	var row onboardingRow
	var work OnboardingWork
	query := "SELECT o.id,o.model_id,o.previous_model_id,o.state,o.document,o.actor,m.document AS model FROM ai.onboarding o JOIN ai.model m ON m.id=o.model_id WHERE o.id=? AND o.deleted_at IS NULL"
	if lock {
		query += " FOR UPDATE OF o"
	}
	r := tx.Raw(query, id).Scan(&row)
	if r.Error != nil {
		return row, work, r.Error
	}
	if r.RowsAffected != 1 {
		return row, work, domain.ErrUnavailable
	}
	var contract onboardingContract
	if json.Unmarshal(row.Document, &contract) != nil || json.Unmarshal(row.Model, &work.Model) != nil ||
		(len(row.Actor) > 0 && json.Unmarshal(row.Actor, &work.Actor) != nil) || len(contract.Plan.Probes) == 0 {
		return row, work, domain.ErrInvalid
	}
	work.Input, work.Plan, work.State = contract.Onboarding, contract.Plan, row.State
	return row, work, nil
}

// ReviewOnboarding shows the approved inputs, scoped budget and probe progress.
func (s *Store) ReviewOnboarding(ctx context.Context, id uuid.UUID) (OnboardingReview, error) {
	var result OnboardingReview
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		row, work, err := loadOnboarding(tx, id, false)
		if err != nil {
			return err
		}
		result = OnboardingReview{
			OnboardingSummary: OnboardingSummary{ID: id, ModelID: row.ModelID, State: row.State, Upstream: work.Input.Route.UpstreamModel},
			Plan:              work.Plan, Scope: work.Input.Scope, Project: work.Input.Project,
			Probes: make([]OnboardingProgress, 0, len(work.Plan.Probes)),
		}
		for i, probe := range work.Plan.Probes {
			call, err := application.OnboardingCall(work.Input, work.Model, work.Actor.ID, probe, i)
			if err != nil {
				return err
			}
			var state string
			if err := tx.Raw("SELECT state FROM ai.request WHERE id=? AND origin=? AND run_id=?", call.ID, application.OnboardingOrigin, id).Scan(&state).Error; err != nil {
				return err
			}
			if state != "" {
				result.Probes = append(result.Probes, OnboardingProgress{RequestID: call.ID, State: state})
			}
		}
		return nil
	})
	return result, err
}

// ConfirmOnboarding durably records approval before any supplier request can occur.
func (s *Store) ConfirmOnboarding(ctx context.Context, id uuid.UUID, credits, purchase int64, seal func(uuid.UUID, uuid.UUID) ([]byte, string, error)) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.check(ctx, tx, s.actor.ScopeID, uuid.Nil, true); err != nil {
			return err
		}
		row, work, err := loadOnboarding(tx, id, true)
		if err != nil {
			return err
		}
		if err = s.check(ctx, tx, work.Input.Scope, work.Input.Project, false); err != nil {
			return err
		}
		if credits != work.Plan.Credits || purchase != work.Plan.PurchaseMicros {
			return domain.ErrBudget
		}
		if row.State != "prepared" {
			// The first accepted confirmation owns the existing verification.
			if work.Actor.ID != s.actor.ID {
				return domain.ErrConflict
			}
			return nil
		}
		if err := checkPrevious(tx, row, work.Model); err != nil {
			return err
		}
		var available int64
		if err := tx.Raw("SELECT COALESCE((SELECT available FROM ai.account WHERE scope_id=?),0)", work.Input.Scope).Row().Scan(&available); err != nil {
			return err
		}
		if available < credits {
			return domain.ErrFunds
		}
		if err := tx.Exec("INSERT INTO ai.account(scope_id) VALUES(?) ON CONFLICT DO NOTHING", work.Input.Scope).Error; err != nil {
			return err
		}
		route := work.Input.Route
		cipher, keyID, err := seal(route.Supplier, route.Credential)
		if err != nil {
			return err
		}
		if len(cipher) == 0 || keyID == "" {
			return domain.ErrInvalid
		}
		if err := insertRoute(tx, route, cipher, keyID, work.Input.Concurrency, work.Input.RPM, false); err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO ai.budget(scope_id,id,limit_credits) VALUES(?,?,?)", work.Input.Scope, id, credits).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE ai.onboarding SET state='verifying',actor=?::jsonb,confirmed_at=clock_timestamp() WHERE id=?", string(mustJSON(s.actor)), id).Error; err != nil {
			return err
		}
		return taskruntime.Enqueue(ctx, tx, OnboardingKind, id.String(), time.Now())
	})
}

func checkPrevious(tx *gorm.DB, row onboardingRow, model domain.Model) error {
	live, err := activeModel(tx, model)
	if err != nil {
		return err
	}
	expected := uuid.Nil
	if row.PreviousModelID != nil {
		expected = *row.PreviousModelID
	}
	if live.ID != expected {
		return domain.ErrConflict
	}
	return nil
}

// OnboardingWork loads the recorded administrator session for a durable job.
func (s *Store) OnboardingWork(ctx context.Context, id uuid.UUID) (OnboardingWork, error) {
	_, work, err := loadOnboarding(s.db.WithContext(ctx), id, false)
	return work, err
}

// FailOnboarding closes verification while leaving the current live alias available.
func (s *Store) FailOnboarding(ctx context.Context, id uuid.UUID) error {
	return s.db.WithContext(ctx).Exec("UPDATE ai.onboarding SET state='failed' WHERE id=? AND state='verifying'", id).Error
}

// OnboardingRecord reads the original evidence without creating an attempt.
func (s *Store) OnboardingRecord(ctx context.Context, call domain.Call) (domain.Record, error) {
	return onboardingRecord(s.db.WithContext(ctx), call)
}

func onboardingRecord(tx *gorm.DB, call domain.Call) (domain.Record, error) {
	var present bool
	if err := tx.Raw("SELECT EXISTS(SELECT 1 FROM ai.request WHERE id=? AND origin=? AND run_id=? AND input_hash=?)", call.ID, application.OnboardingOrigin, call.Run, call.Hash).Row().Scan(&present); err != nil {
		return domain.Record{}, err
	}
	if !present {
		return domain.Record{}, nil
	}
	return scanRecord(tx, call.ID)
}
