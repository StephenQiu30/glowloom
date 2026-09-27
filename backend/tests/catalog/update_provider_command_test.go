package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

type updateProviderStore struct {
	provider domain.Provider
	before   domain.Provider
	after    domain.Provider
	events   []identityapp.OutboxEvent
	writes   int
}

func (s *updateProviderStore) FindProviderForAdmin(_ context.Context, _, _, _ uuid.UUID) (domain.Provider, error) {
	return s.provider, nil
}

func (s *updateProviderStore) UpdateProviderWithEvents(_ context.Context, _, _ uuid.UUID, before, after domain.Provider, events []identityapp.OutboxEvent) (domain.Provider, error) {
	s.writes++
	s.before, s.after, s.events = before, after, events
	after.CreateTime = time.Now()
	after.UpdateTime = after.CreateTime
	return after, nil
}

func TestUpdateProviderCommandEmitsSafeEvents(t *testing.T) {
	actor := adminPrincipal()
	store := &updateProviderStore{provider: validProvider()}
	status := domain.ProviderDisabled
	rate := 120
	input := catalogapp.UpdateProviderInput{
		ProviderID: store.provider.ID, ExpectedRevision: 1,
		Status: &status, RateLimitPerMin: &rate, RequestID: uuid.NewString(),
	}
	result, err := catalogapp.NewUpdateProviderCommand(store, time.Now).Execute(t.Context(), actor, input)
	if err != nil || store.writes != 1 || result.Revision != 2 || result.Status != status || result.RateLimitPerMin != rate {
		t.Fatalf("updated provider %+v, writes %d: %v", result, store.writes, err)
	}
	if store.before.Revision != 1 || store.after.Revision != 2 || len(store.events) != 2 ||
		store.events[0].Topic != "lanverse.catalog.provider_changed.v1" ||
		store.events[1].Topic != "lanverse.audit.recorded.v1" {
		t.Fatalf("provider events %+v", store.events)
	}
	audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: store.events[1].Topic, Key: []byte(actor.OrgID.String()), Value: store.events[1].Payload,
	})
	if err != nil || audit.Action != "provider.updated" || audit.ObjectID != result.ID.String() {
		t.Fatalf("provider audit %+v: %v", audit, err)
	}
	var before, after map[string]any
	if err := json.Unmarshal(audit.Before, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(audit.After, &after); err != nil || before["status"] != "active" ||
		before["rate_limit_per_min"] != float64(60) || after["status"] != "disabled" ||
		after["rate_limit_per_min"] != float64(120) || after["concurrency_limit"] != float64(10) {
		t.Fatalf("unsafe or incomplete audit before=%v after=%v: %v", before, after, err)
	}
	var changed struct {
		Data struct {
			ProviderID uuid.UUID `json:"provider_id"`
			Revision   int64     `json:"revision"`
		} `json:"data"`
	}
	if err := json.Unmarshal(store.events[0].Payload, &changed); err != nil ||
		changed.Data.ProviderID != result.ID || changed.Data.Revision != 2 {
		t.Fatalf("change event %+v: %v", changed, err)
	}
}

func TestUpdateProviderCommandRejectsInvalidOrStaleRequest(t *testing.T) {
	actor := adminPrincipal()
	status := domain.ProviderDisabled
	zero := 0
	input := catalogapp.UpdateProviderInput{
		ProviderID: uuid.New(), ExpectedRevision: 1, Status: &status, RequestID: uuid.NewString(),
	}
	for _, tc := range []struct {
		name   string
		actor  identityapp.Principal
		change func(*catalogapp.UpdateProviderInput)
		want   error
	}{
		{"producer", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleProducer}, nil, identityapp.ErrForbidden},
		{"first login", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleAdmin, MustChangePassword: true}, nil, identityapp.ErrForbidden},
		{"nil provider", actor, func(v *catalogapp.UpdateProviderInput) { v.ProviderID = uuid.Nil }, catalogapp.ErrInvalidUpdateProvider},
		{"invalid request", actor, func(v *catalogapp.UpdateProviderInput) { v.RequestID = "bad" }, catalogapp.ErrInvalidUpdateProvider},
		{"empty patch", actor, func(v *catalogapp.UpdateProviderInput) { v.Status = nil }, catalogapp.ErrInvalidUpdateProvider},
		{"invalid rate", actor, func(v *catalogapp.UpdateProviderInput) { v.RateLimitPerMin = &zero }, catalogapp.ErrInvalidUpdateProvider},
		{"stale revision", actor, func(v *catalogapp.UpdateProviderInput) { v.ExpectedRevision = 2 }, domain.ErrProviderRevisionConflict},
		{"no change", actor, func(v *catalogapp.UpdateProviderInput) { active := domain.ProviderActive; v.Status = &active }, catalogapp.ErrInvalidUpdateProvider},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &updateProviderStore{provider: validProvider()}
			candidate := input
			candidate.ProviderID = store.provider.ID
			if tc.change != nil {
				tc.change(&candidate)
			}
			_, err := catalogapp.NewUpdateProviderCommand(store, time.Now).Execute(t.Context(), tc.actor, candidate)
			if !errors.Is(err, tc.want) || store.writes != 0 {
				t.Fatalf("invalid request wrote provider: %v, writes=%d", err, store.writes)
			}
		})
	}
}
