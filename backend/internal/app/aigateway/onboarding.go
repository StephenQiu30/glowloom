package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// onboard resumes a reviewed set of at-most-once probes using the original payer,
// route and session. Existing workers settle, poll and archive their receipts.
func (r *Runtime) onboard(ctx context.Context, job taskruntime.Job) (time.Time, error) {
	id, err := uuid.Parse(job.BusinessID)
	if err != nil {
		return time.Time{}, err
	}
	admin := r.Store(identity.Principal{})
	work, err := admin.OnboardingWork(ctx, id)
	if err != nil || work.State != "verifying" {
		return time.Time{}, err
	}
	store := r.Store(work.Actor).WithOnboarding(id, func(ctx context.Context, tx *gorm.DB, call domain.Call) error {
		return r.prepareOnboardingMedia(ctx, tx, work.Actor, call)
	})
	next := time.Now().Add(5 * time.Second)
	for i, probe := range work.Plan.Probes {
		call, err := application.OnboardingCall(work.Input, work.Model, work.Actor.ID, probe, i)
		if err != nil {
			return time.Time{}, err
		}
		record, err := store.OnboardingRecord(ctx, call)
		if err != nil {
			return time.Time{}, err
		}
		started := record.ID == uuid.Nil
		if started {
			if r.Provider == nil {
				return time.Time{}, store.FailOnboarding(ctx, id)
			}
			record, err = application.New(store, r.Provider).Execute(ctx, call, nil)
			if err != nil {
				if record.State == "released" || (record.ID == uuid.Nil && deniedOnboarding(err)) {
					return time.Time{}, store.FailOnboarding(ctx, id)
				}
				if record.ID != uuid.Nil {
					return next, nil
				}
				return time.Time{}, err
			}
		}
		switch record.State {
		case "released":
			return time.Time{}, store.FailOnboarding(ctx, id)
		case "settled":
			if !application.ProbePassed(work.Model.Protocol, probe, record.Receipt) {
				return time.Time{}, store.FailOnboarding(ctx, id)
			}
			if started && i+1 < len(work.Plan.Probes) {
				return time.Now(), nil
			}
		case "unknown":
			return time.Now().Add(30 * time.Second), nil
		default:
			return next, nil
		}
	}
	err = store.PublishVerified(ctx, id)
	switch {
	case errors.Is(err, domain.ErrUnknown):
		return next, nil
	case deniedOnboarding(err):
		return time.Time{}, store.FailOnboarding(ctx, id)
	default:
		return time.Time{}, err
	}
}

func deniedOnboarding(err error) bool {
	return errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrConflict) ||
		errors.Is(err, domain.ErrFunds) || errors.Is(err, domain.ErrBudget) ||
		errors.Is(err, domain.ErrUnavailable) || errors.Is(err, domain.ErrInvalid)
}

func (r *Runtime) prepareOnboardingMedia(ctx context.Context, tx *gorm.DB, actor identity.Principal, call domain.Call) error {
	if call.Model.Protocol != "media" {
		return nil
	}
	if err := r.PinMediaOutput(ctx, tx, call); err != nil {
		return err
	}
	if _, err := pg.New(tx, actor, Authority).Quote(ctx, call); err != nil {
		return err
	}
	// The media reference resolver and delivery worker use the same quoted
	// contract and actor as ordinary gateway media calls.
	raw, err := json.Marshal(actor)
	if err != nil {
		return err
	}
	return tx.Exec("INSERT INTO ai.job(request_id,actor) VALUES(?,?::jsonb)", call.ID, string(raw)).Error
}
