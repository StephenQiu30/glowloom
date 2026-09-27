package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/credentialschema"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

type createProviderStore struct {
	created domain.Provider
	events  []identityapp.OutboxEvent
	called  bool
}

func (s *createProviderStore) CreateProviderWithAudit(_ context.Context, _, _ uuid.UUID, provider domain.Provider, event identityapp.OutboxEvent) (domain.Provider, error) {
	s.called = true
	s.created = provider
	s.events = []identityapp.OutboxEvent{event}
	s.created.CreateTime = time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	s.created.UpdateTime = s.created.CreateTime
	return s.created, nil
}

func TestCreateProviderCommandCommitsSafeAudit(t *testing.T) {
	store := &createProviderStore{}
	actor := adminPrincipal()
	command := catalogapp.NewCreateProviderCommand(store, credentialschema.NewRegistry(), time.Now)
	created, err := command.Execute(t.Context(), actor, catalogapp.CreateProviderInput{
		Key: "openrouter", Name: "OpenRouter", AdapterKey: "openrouter",
		Region: domain.RegionOverseas, ConcurrencyLimit: 10, RateLimitPerMin: 60,
		RequestID: uuid.NewString(),
	})
	if err != nil || !store.called || created.ID != store.created.ID || created.Revision != 1 ||
		created.Status != domain.ProviderActive || created.AdapterKey != "openrouter" {
		t.Fatalf("provider result %+v: %v", created, err)
	}
	if len(store.events) != 1 || store.events[0].Topic != "lanverse.audit.recorded.v1" ||
		store.events[0].PartitionKey != actor.OrgID.String() {
		t.Fatalf("wrong provider audit: %+v", store.events)
	}
	audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: store.events[0].Topic, Key: []byte(actor.OrgID.String()), Value: store.events[0].Payload,
	})
	if err != nil || audit.Action != "provider.created" || audit.ObjectID != created.ID.String() {
		t.Fatalf("invalid provider audit %+v: %v", audit, err)
	}
	var after map[string]any
	if err := json.Unmarshal(audit.After, &after); err != nil || after["key"] != "openrouter" ||
		after["adapter_key"] != "openrouter" || after["region"] != "overseas" ||
		after["status"] != "active" || after["concurrency_limit"] != float64(10) ||
		after["rate_limit_per_min"] != float64(60) {
		t.Fatalf("provider audit summary %+v: %v", after, err)
	}
	var unsafe map[string]any
	if err := json.Unmarshal(store.events[0].Payload, &unsafe); err != nil {
		t.Fatal(err)
	}
	unsafe["data"].(map[string]any)["after"] = map[string]any{"secret": "forbidden"}
	unsafePayload, err := json.Marshal(unsafe)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: store.events[0].Topic, Key: []byte(actor.OrgID.String()), Value: unsafePayload,
	}); !errors.Is(err, auditapp.ErrInvalidEvent) {
		t.Fatalf("provider audit accepted secret field: %v", err)
	}
}

func TestCreateProviderCommandRejectsInvalidCallerAndAdapter(t *testing.T) {
	actor := adminPrincipal()
	input := catalogapp.CreateProviderInput{
		Key: "openrouter", Name: "OpenRouter", AdapterKey: "openrouter",
		Region: domain.RegionOverseas, ConcurrencyLimit: 10, RateLimitPerMin: 60,
		RequestID: uuid.NewString(),
	}
	for _, tc := range []struct {
		name   string
		actor  identityapp.Principal
		change func(*catalogapp.CreateProviderInput)
	}{
		{"producer", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleProducer}, nil},
		{"first login", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleAdmin, MustChangePassword: true}, nil},
		{"unknown adapter", actor, func(v *catalogapp.CreateProviderInput) { v.AdapterKey = "unknown" }},
		{"invalid key", actor, func(v *catalogapp.CreateProviderInput) { v.Key = "Open Router" }},
		{"invalid request", actor, func(v *catalogapp.CreateProviderInput) { v.RequestID = "not-a-uuid" }},
		{"nil request ID", actor, func(v *catalogapp.CreateProviderInput) { v.RequestID = uuid.Nil.String() }},
		{"invalid rate", actor, func(v *catalogapp.CreateProviderInput) { v.RateLimitPerMin = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := input
			if tc.change != nil {
				tc.change(&candidate)
			}
			store := &createProviderStore{}
			_, err := catalogapp.NewCreateProviderCommand(store, credentialschema.NewRegistry(), time.Now).Execute(t.Context(), tc.actor, candidate)
			if err == nil || store.called {
				t.Fatalf("invalid provider reached storage: %v", err)
			}
		})
	}
}
