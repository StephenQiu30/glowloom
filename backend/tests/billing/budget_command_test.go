package billing_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	billingapp "github.com/StephenQiu30/lanverse/backend/internal/billing/application"
	"github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

type budgetCommandStore struct {
	budget   domain.Budget
	finds    int
	writes   int
	actor    identityapp.Principal
	before   domain.Budget
	after    domain.Budget
	entry    domain.LedgerEntry
	events   []identityapp.OutboxEvent
	findErr  error
	writeErr error
	saved    *domain.Budget
}

func (s *budgetCommandStore) FindBudget(_ context.Context, actor identityapp.Principal, _ uuid.UUID) (domain.Budget, error) {
	s.finds++
	s.actor = actor
	if s.findErr != nil {
		return domain.Budget{}, s.findErr
	}
	return s.budget, nil
}

func (s *budgetCommandStore) ChangeBudgetWithEvents(_ context.Context, actor identityapp.Principal, before, after domain.Budget, entry domain.LedgerEntry, events []identityapp.OutboxEvent) (domain.Budget, error) {
	s.writes++
	s.actor, s.before, s.after, s.entry, s.events = actor, before, after, entry, events
	if s.writeErr != nil {
		return domain.Budget{}, s.writeErr
	}
	if s.saved != nil {
		return *s.saved, nil
	}
	return after, nil
}

func budgetCommandActor(role identitydomain.Role) identityapp.Principal {
	return identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: role}
}

func budgetCommandFixture() domain.Budget {
	return domain.Budget{
		ID: uuid.New(), ProjectID: uuid.New(), LimitMicros: 400_000_000,
		ReservedMicros: 50_000_000, SettledMicros: 300_000_000, Revision: 2,
	}
}

func budgetChangeInput(budget domain.Budget) billingapp.ChangeBudgetInput {
	return billingapp.ChangeBudgetInput{
		ProjectID: budget.ProjectID, LimitMicros: 500_000_000,
		ExpectedRevision: budget.Revision, RequestID: uuid.NewString(),
	}
}

func TestChangeBudgetCommandWritesSignedLedgerAndSafeEvents(t *testing.T) {
	actor := budgetCommandActor(identitydomain.RoleProducer)
	before := budgetCommandFixture()
	store := &budgetCommandStore{budget: before}
	input := budgetChangeInput(before)
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	saved, err := billingapp.NewChangeBudgetCommand(store, func() time.Time { return now }).Execute(t.Context(), actor, input)
	if err != nil || store.finds != 1 || store.writes != 1 {
		t.Fatalf("change budget: saved=%+v finds=%d writes=%d err=%v", saved, store.finds, store.writes, err)
	}
	if store.before != before || store.after.ID != before.ID || store.after.ProjectID != before.ProjectID ||
		store.after.LimitMicros != input.LimitMicros || store.after.ReservedMicros != before.ReservedMicros ||
		store.after.SettledMicros != before.SettledMicros || store.after.IsOverrun ||
		store.after.Revision != before.Revision+1 || saved != store.after {
		t.Fatalf("budget transition: before=%+v after=%+v saved=%+v", store.before, store.after, saved)
	}
	if store.entry.ID == uuid.Nil || store.entry.ProjectID != before.ProjectID ||
		store.entry.EntryType != "budget_change" || store.entry.AmountMicros != 100_000_000 ||
		store.entry.CreateBy == nil || *store.entry.CreateBy != actor.ID ||
		!store.entry.CreateTime.Equal(now) || store.entry.Note != "" ||
		store.entry.OperationID != nil || store.entry.EpisodeID != nil || store.entry.ShotID != nil {
		t.Fatalf("budget change ledger entry: %+v", store.entry)
	}
	if len(store.events) != 2 || store.events[0].ID == store.events[1].ID {
		t.Fatalf("budget events = %+v, want two distinct events", store.events)
	}
	events := make(map[string]identityapp.OutboxEvent, 2)
	for _, event := range store.events {
		if event.ID == uuid.Nil || event.PartitionKey != before.ProjectID.String() {
			t.Fatalf("bad budget event: %+v", event)
		}
		events[event.Topic] = event
	}
	changed, ok := events["lanverse.billing.budget_changed.v1"]
	if !ok {
		t.Fatal("missing budget changed event")
	}
	var change struct {
		EventID    uuid.UUID `json:"event_id"`
		EventType  string    `json:"event_type"`
		OccurredAt time.Time `json:"occurred_at"`
		OrgID      uuid.UUID `json:"org_id"`
		ProjectID  uuid.UUID `json:"project_id"`
		Actor      struct {
			Kind string    `json:"kind"`
			ID   uuid.UUID `json:"id"`
		} `json:"actor"`
		Aggregate struct {
			Type     string    `json:"type"`
			ID       uuid.UUID `json:"id"`
			Revision int64     `json:"revision"`
		} `json:"aggregate"`
		Data struct {
			LimitMicros     int64 `json:"limit_micros"`
			AvailableMicros int64 `json:"available_micros"`
			IsOverrun       bool  `json:"is_overrun"`
		} `json:"data"`
	}
	if err := json.Unmarshal(changed.Payload, &change); err != nil ||
		change.EventID != changed.ID || change.EventType != changed.Topic ||
		!change.OccurredAt.Equal(now) || change.OrgID != actor.OrgID ||
		change.ProjectID != before.ProjectID || change.Actor.Kind != "user" || change.Actor.ID != actor.ID ||
		change.Aggregate.Type != "budget" || change.Aggregate.ID != before.ID ||
		change.Aggregate.Revision != before.Revision+1 || change.Data.LimitMicros != input.LimitMicros ||
		change.Data.AvailableMicros != 150_000_000 || change.Data.IsOverrun {
		t.Fatalf("invalid budget change event: %+v err=%v", change, err)
	}
	auditEvent, ok := events["lanverse.audit.recorded.v1"]
	if !ok {
		t.Fatal("missing budget audit event")
	}
	audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: auditEvent.Topic, Key: []byte(auditEvent.PartitionKey), Value: auditEvent.Payload,
	})
	if err != nil || audit.Action != "budget.changed" || audit.OrgID != actor.OrgID ||
		audit.ProjectID == nil || *audit.ProjectID != before.ProjectID ||
		audit.ObjectType != "budget" || audit.ObjectID != before.ID.String() ||
		audit.RequestID != input.RequestID {
		t.Fatalf("invalid budget audit: %+v err=%v", audit, err)
	}
	var oldSummary, newSummary map[string]any
	if err := json.Unmarshal(audit.Before, &oldSummary); err != nil {
		t.Fatalf("decode audit before: %v", err)
	}
	if err := json.Unmarshal(audit.After, &newSummary); err != nil {
		t.Fatalf("decode audit after: %v", err)
	}
	if len(oldSummary) != 3 || len(newSummary) != 3 ||
		oldSummary["limit_micros"] != float64(before.LimitMicros) ||
		oldSummary["revision"] != float64(before.Revision) || oldSummary["is_overrun"] != false ||
		newSummary["limit_micros"] != float64(input.LimitMicros) ||
		newSummary["revision"] != float64(before.Revision+1) || newSummary["is_overrun"] != false {
		t.Fatalf("unsafe or incomplete budget audit: before=%+v after=%+v", oldSummary, newSummary)
	}
}

func TestChangeBudgetCommandDecreaseAndNoOp(t *testing.T) {
	actor := budgetCommandActor(identitydomain.RoleAdmin)
	before := budgetCommandFixture()
	store := &budgetCommandStore{budget: before}
	input := budgetChangeInput(before)
	input.LimitMicros = 350_000_000
	saved, err := billingapp.NewChangeBudgetCommand(store, time.Now).Execute(t.Context(), actor, input)
	if err != nil || saved.LimitMicros != input.LimitMicros ||
		store.entry.AmountMicros != -50_000_000 || store.after.Revision != 3 {
		t.Fatalf("decrease budget: saved=%+v entry=%+v err=%v", saved, store.entry, err)
	}
	store = &budgetCommandStore{budget: before}
	input.LimitMicros = before.LimitMicros
	saved, err = billingapp.NewChangeBudgetCommand(store, time.Now).Execute(t.Context(), actor, input)
	if err != nil || saved != before || store.finds != 1 || store.writes != 0 {
		t.Fatalf("unchanged budget wrote: saved=%+v finds=%d writes=%d err=%v", saved, store.finds, store.writes, err)
	}
	overrun := before
	overrun.LimitMicros = 300_000_000
	overrun.IsOverrun = true
	store = &budgetCommandStore{budget: overrun}
	input.LimitMicros = overrun.LimitMicros
	saved, err = billingapp.NewChangeBudgetCommand(store, time.Now).Execute(t.Context(), actor, input)
	if err != nil || saved != overrun || store.writes != 0 {
		t.Fatalf("unchanged overrun budget wrote: saved=%+v writes=%d err=%v", saved, store.writes, err)
	}
}

func TestChangeBudgetCommandRejectsInvalidInputBeforeStorage(t *testing.T) {
	actor := budgetCommandActor(identitydomain.RoleProducer)
	budget := budgetCommandFixture()
	for _, tc := range []struct {
		name   string
		actor  identityapp.Principal
		change func(*billingapp.ChangeBudgetInput)
		want   error
	}{
		{"missing actor", identityapp.Principal{OrgID: actor.OrgID, Role: actor.Role}, nil, identityapp.ErrForbidden},
		{"missing organization", identityapp.Principal{ID: actor.ID, Role: actor.Role}, nil, identityapp.ErrForbidden},
		{"unknown role", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID}, nil, identityapp.ErrForbidden},
		{"password change required", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: actor.Role, MustChangePassword: true}, nil, identityapp.ErrForbidden},
		{"missing project", actor, func(v *billingapp.ChangeBudgetInput) { v.ProjectID = uuid.Nil }, billingapp.ErrInvalidChangeBudget},
		{"missing revision", actor, func(v *billingapp.ChangeBudgetInput) { v.ExpectedRevision = 0 }, billingapp.ErrInvalidChangeBudget},
		{"revision overflow", actor, func(v *billingapp.ChangeBudgetInput) { v.ExpectedRevision = math.MaxInt32 }, billingapp.ErrInvalidChangeBudget},
		{"negative limit", actor, func(v *billingapp.ChangeBudgetInput) { v.LimitMicros = -1 }, billingapp.ErrInvalidChangeBudget},
		{"bad request ID", actor, func(v *billingapp.ChangeBudgetInput) { v.RequestID = "bad" }, billingapp.ErrInvalidChangeBudget},
		{"noncanonical request ID", actor, func(v *billingapp.ChangeBudgetInput) { v.RequestID = strings.ToUpper(v.RequestID) }, billingapp.ErrInvalidChangeBudget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := budgetChangeInput(budget)
			if tc.change != nil {
				tc.change(&input)
			}
			store := &budgetCommandStore{budget: budget}
			_, err := billingapp.NewChangeBudgetCommand(store, time.Now).Execute(t.Context(), tc.actor, input)
			if !errors.Is(err, tc.want) || store.finds != 0 || store.writes != 0 {
				t.Fatalf("invalid input reached store: err=%v finds=%d writes=%d", err, store.finds, store.writes)
			}
		})
	}
}

func TestChangeBudgetCommandRejectsStaleCrossProjectAndCommittedLimit(t *testing.T) {
	actor := budgetCommandActor(identitydomain.RoleProducer)
	budget := budgetCommandFixture()
	for _, tc := range []struct {
		name   string
		mutate func(*domain.Budget, *billingapp.ChangeBudgetInput)
		want   error
	}{
		{"stale revision", func(_ *domain.Budget, input *billingapp.ChangeBudgetInput) { input.ExpectedRevision++ }, domain.ErrBudgetRevision},
		{"cross project", func(_ *domain.Budget, input *billingapp.ChangeBudgetInput) { input.ProjectID = uuid.New() }, billingapp.ErrInvalidChangeBudget},
		{"invalid row", func(row *domain.Budget, _ *billingapp.ChangeBudgetInput) { row.ID = uuid.Nil }, billingapp.ErrInvalidChangeBudget},
		{"below committed", func(_ *domain.Budget, input *billingapp.ChangeBudgetInput) { input.LimitMicros = 349_999_999 }, domain.ErrLimitBelowCommitted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := budget
			input := budgetChangeInput(budget)
			tc.mutate(&row, &input)
			store := &budgetCommandStore{budget: row}
			_, err := billingapp.NewChangeBudgetCommand(store, time.Now).Execute(t.Context(), actor, input)
			if !errors.Is(err, tc.want) || store.finds != 1 || store.writes != 0 {
				t.Fatalf("invalid state reached writer: err=%v finds=%d writes=%d", err, store.finds, store.writes)
			}
		})
	}
}

func TestChangeBudgetCommandPreservesStoreErrorsAndRejectsWrongSavedRow(t *testing.T) {
	actor := budgetCommandActor(identitydomain.RoleAdmin)
	budget := budgetCommandFixture()
	input := budgetChangeInput(budget)
	sentinel := errors.New("database failed")
	store := &budgetCommandStore{budget: budget, findErr: sentinel}
	_, err := billingapp.NewChangeBudgetCommand(store, time.Now).Execute(t.Context(), actor, input)
	if !errors.Is(err, sentinel) || store.writes != 0 {
		t.Fatalf("find error chain: err=%v writes=%d", err, store.writes)
	}
	store = &budgetCommandStore{budget: budget, writeErr: sentinel}
	_, err = billingapp.NewChangeBudgetCommand(store, time.Now).Execute(t.Context(), actor, input)
	if !errors.Is(err, sentinel) || store.writes != 1 {
		t.Fatalf("write error chain: err=%v writes=%d", err, store.writes)
	}
	wrong := budget
	wrong.LimitMicros = input.LimitMicros
	wrong.Revision++
	wrong.ProjectID = uuid.New()
	store = &budgetCommandStore{budget: budget, saved: &wrong}
	_, err = billingapp.NewChangeBudgetCommand(store, time.Now).Execute(t.Context(), actor, input)
	if !errors.Is(err, billingapp.ErrInvalidChangeBudget) {
		t.Fatalf("wrong saved row accepted: %v", err)
	}
}

func TestChangeBudgetEmitsLowEventOnlyWhenCrossingThreshold(t *testing.T) {
	actor := budgetCommandActor(identitydomain.RoleProducer)
	for _, tc := range []struct {
		name      string
		limit     int64
		committed int64
		next      int64
		wantLow   bool
	}{
		{"cross from exact twenty percent", 500, 400, 499, true},
		{"reach exact twenty percent", 600, 400, 500, false},
		{"remain below threshold", 400, 350, 375, false},
		{"recover above threshold", 400, 350, 500, false},
		{"zero budget increase", 0, 0, 100, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget := domain.Budget{
				ID: uuid.New(), ProjectID: uuid.New(), LimitMicros: tc.limit,
				SettledMicros: tc.committed, Revision: 1,
			}
			store := &budgetCommandStore{budget: budget}
			input := billingapp.ChangeBudgetInput{
				ProjectID: budget.ProjectID, LimitMicros: tc.next,
				ExpectedRevision: 1, RequestID: uuid.NewString(),
			}
			occurredAt := time.Date(2026, 9, 27, 12, 30, 0, 0, time.UTC)
			_, err := billingapp.NewChangeBudgetCommand(store, func() time.Time { return occurredAt }).Execute(t.Context(), actor, input)
			if err != nil {
				t.Fatalf("change budget: %v", err)
			}
			wantCount := 2
			if tc.wantLow {
				wantCount = 3
			}
			if len(store.events) != wantCount {
				t.Fatalf("budget events = %+v, want %d", store.events, wantCount)
			}
			var lowEvents int
			for _, event := range store.events {
				if event.Topic != "lanverse.billing.budget_low.v1" {
					continue
				}
				lowEvents++
				var payload struct {
					EventID    uuid.UUID `json:"event_id"`
					EventType  string    `json:"event_type"`
					OccurredAt time.Time `json:"occurred_at"`
					OrgID      uuid.UUID `json:"org_id"`
					ProjectID  uuid.UUID `json:"project_id"`
					Aggregate  struct {
						Type     string    `json:"type"`
						ID       uuid.UUID `json:"id"`
						Revision int64     `json:"revision"`
					} `json:"aggregate"`
					Data struct {
						LimitMicros     int64 `json:"limit_micros"`
						AvailableMicros int64 `json:"available_micros"`
					} `json:"data"`
				}
				if err := json.Unmarshal(event.Payload, &payload); err != nil ||
					payload.EventID != event.ID || payload.EventType != event.Topic ||
					!payload.OccurredAt.Equal(occurredAt) || payload.OrgID != actor.OrgID ||
					payload.ProjectID != budget.ProjectID || event.PartitionKey != budget.ProjectID.String() ||
					payload.Aggregate.Type != "budget" || payload.Aggregate.ID != budget.ID ||
					payload.Aggregate.Revision != 2 || payload.Data.LimitMicros != tc.next ||
					payload.Data.AvailableMicros != tc.next-tc.committed {
					t.Fatalf("low event = %+v, error = %v", payload, err)
				}
			}
			if tc.wantLow && lowEvents != 1 || !tc.wantLow && lowEvents != 0 {
				t.Fatalf("low event count = %d, want low %t", lowEvents, tc.wantLow)
			}
		})
	}
}
