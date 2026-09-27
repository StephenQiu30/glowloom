package catalog_test

import (
	"bytes"
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

type publishPriceValidator struct {
	called bool
	price  domain.PriceRuleVersion
	err    error
}

func (v *publishPriceValidator) Validate(price domain.PriceRuleVersion) error {
	v.called, v.price = true, price
	return v.err
}

type publishPriceStore struct {
	called           bool
	actorID          uuid.UUID
	orgID            uuid.UUID
	price            domain.PriceRuleVersion
	expectedRevision int64
	event            identityapp.OutboxEvent
	err              error
}

func (s *publishPriceStore) PublishPriceRuleWithAudit(_ context.Context, actorID, orgID uuid.UUID, price domain.PriceRuleVersion, expectedRevision int64, event identityapp.OutboxEvent) (domain.PriceRuleVersion, error) {
	s.called = true
	s.actorID, s.orgID = actorID, orgID
	s.price, s.expectedRevision, s.event = price, expectedRevision, event
	if s.err != nil {
		return domain.PriceRuleVersion{}, s.err
	}
	price.CreateTime = time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	return price, nil
}

func validPublishPriceRuleInput(modelID uuid.UUID) catalogapp.PublishPriceRuleInput {
	return catalogapp.PublishPriceRuleInput{
		ModelID: modelID, ExpectedRevision: 2, VersionNo: 1,
		Unit:     domain.PricePerSecond,
		Rule:     json.RawMessage(`{"base_micros":1000,"by_mode":{"secret-mode-123":1.25}}`),
		Currency: "CNY", EffectiveFrom: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		RequestID: uuid.NewString(),
	}
}

func TestPublishPriceRuleCommandBuildsSafeAuditAndVersion(t *testing.T) {
	actor := adminPrincipal()
	store := &publishPriceStore{}
	validator := &publishPriceValidator{}
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	input := validPublishPriceRuleInput(uuid.New())
	result, err := catalogapp.NewPublishPriceRuleCommand(store, validator, func() time.Time { return now }).Execute(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if !validator.called || !store.called || store.actorID != actor.ID || store.orgID != actor.OrgID ||
		store.expectedRevision != input.ExpectedRevision || store.price.CreateBy != actor.ID ||
		store.price.ModelID != input.ModelID || store.price.VersionNo != input.VersionNo ||
		store.price.Unit != input.Unit || !bytes.Equal(store.price.Rule, input.Rule) ||
		store.price.Currency != input.Currency || store.price.FXRateToCNY != input.FXRateToCNY ||
		!store.price.EffectiveFrom.Equal(input.EffectiveFrom) ||
		result.ID != store.price.ID || result.ModelID != input.ModelID ||
		result.VersionNo != input.VersionNo || result.Revision != input.ExpectedRevision+1 ||
		result.CreateTime.IsZero() {
		t.Fatalf("price publication mismatch: result %+v, saved %+v", result, store.price)
	}
	if validator.price.ID != store.price.ID || validator.price.ModelID != store.price.ModelID ||
		!bytes.Equal(validator.price.Rule, store.price.Rule) {
		t.Fatal("validator did not see the price published by the store")
	}
	if store.event.Topic != "lanverse.audit.recorded.v1" || store.event.PartitionKey != actor.OrgID.String() ||
		bytes.Contains(store.event.Payload, []byte("secret-mode-123")) ||
		bytes.Contains(store.event.Payload, []byte("base_micros")) {
		t.Fatalf("unsafe price audit envelope: %+v", store.event)
	}
	audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: store.event.Topic, Key: []byte(store.event.PartitionKey), Value: store.event.Payload,
	})
	if err != nil || audit.Action != "price.published" || audit.ObjectType != "model_profile" ||
		audit.ObjectID != input.ModelID.String() || audit.RequestID != input.RequestID {
		t.Fatalf("invalid price audit %+v: %v", audit, err)
	}
	var after map[string]any
	if err := json.Unmarshal(audit.After, &after); err != nil ||
		after["version_id"] != store.price.ID.String() || after["version_no"] != float64(1) || after["unit"] != string(input.Unit) ||
		after["currency"] != input.Currency {
		t.Fatalf("price audit summary %+v: %v", after, err)
	}
	for name, value := range after {
		switch value.(type) {
		case nil, bool, float64, string:
		default:
			t.Fatalf("nested price audit field %q: %T", name, value)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(store.event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	payload["data"].(map[string]any)["after"].(map[string]any)["private_rule"] = "forbidden"
	unsafePayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: store.event.Topic, Key: []byte(store.event.PartitionKey), Value: unsafePayload,
	}); !errors.Is(err, auditapp.ErrInvalidEvent) {
		t.Fatalf("price audit accepted undeclared field: %v", err)
	}
}

func TestPublishPriceRuleCommandRejectsBeforeStore(t *testing.T) {
	actor := adminPrincipal()
	input := validPublishPriceRuleInput(uuid.New())
	for _, tc := range []struct {
		name   string
		actor  identityapp.Principal
		change func(*catalogapp.PublishPriceRuleInput)
	}{
		{"producer", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleProducer}, nil},
		{"first login", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleAdmin, MustChangePassword: true}, nil},
		{"missing model", actor, func(v *catalogapp.PublishPriceRuleInput) { v.ModelID = uuid.Nil }},
		{"missing revision", actor, func(v *catalogapp.PublishPriceRuleInput) { v.ExpectedRevision = 0 }},
		{"missing version number", actor, func(v *catalogapp.PublishPriceRuleInput) { v.VersionNo = 0 }},
		{"bad request", actor, func(v *catalogapp.PublishPriceRuleInput) { v.RequestID = "not-a-uuid" }},
		{"missing unit", actor, func(v *catalogapp.PublishPriceRuleInput) { v.Unit = "" }},
		{"wrong JSON shape", actor, func(v *catalogapp.PublishPriceRuleInput) { v.Rule = json.RawMessage(`[]`) }},
		{"missing currency", actor, func(v *catalogapp.PublishPriceRuleInput) { v.Currency = "" }},
		{"missing effective time", actor, func(v *catalogapp.PublishPriceRuleInput) { v.EffectiveFrom = time.Time{} }},
		{"foreign currency missing rate", actor, func(v *catalogapp.PublishPriceRuleInput) { v.Currency = "USD" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := input
			if tc.change != nil {
				tc.change(&candidate)
			}
			store := &publishPriceStore{}
			_, err := catalogapp.NewPublishPriceRuleCommand(store, &publishPriceValidator{}, time.Now).Execute(t.Context(), tc.actor, candidate)
			if err == nil || store.called {
				t.Fatalf("invalid publication reached storage: %v", err)
			}
		})
	}
	validationErr := errors.New("price rule rejected")
	store := &publishPriceStore{}
	validator := &publishPriceValidator{err: validationErr}
	_, err := catalogapp.NewPublishPriceRuleCommand(store, validator, time.Now).Execute(t.Context(), actor, input)
	if !errors.Is(err, validationErr) || !validator.called || store.called {
		t.Fatalf("validator rejection reached storage: %v", err)
	}
	store = &publishPriceStore{}
	_, err = catalogapp.NewPublishPriceRuleCommand(store, &publishPriceValidator{}, func() time.Time { return time.Time{} }).Execute(t.Context(), actor, input)
	if err == nil || store.called {
		t.Fatalf("zero clock reached storage: %v", err)
	}
}

func TestPublishPriceRuleCommandPreservesStoreFailure(t *testing.T) {
	storeErr := domain.ErrModelRevisionConflict
	store := &publishPriceStore{err: storeErr}
	_, err := catalogapp.NewPublishPriceRuleCommand(store, &publishPriceValidator{}, time.Now).Execute(
		t.Context(), adminPrincipal(), validPublishPriceRuleInput(uuid.New()),
	)
	if !errors.Is(err, storeErr) || !store.called {
		t.Fatalf("store conflict hidden: %v", err)
	}
}
