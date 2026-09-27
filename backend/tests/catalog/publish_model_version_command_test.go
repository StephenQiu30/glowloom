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

type publishVersionValidator struct {
	called  bool
	version domain.ModelVersion
	err     error
}

func (v *publishVersionValidator) Validate(version domain.ModelVersion) error {
	v.called = true
	v.version = version
	return v.err
}

type publishVersionStore struct {
	called           bool
	actorID          uuid.UUID
	orgID            uuid.UUID
	version          domain.ModelVersion
	expectedRevision int64
	event            identityapp.OutboxEvent
	err              error
}

func (s *publishVersionStore) PublishModelVersionWithAudit(_ context.Context, actorID, orgID uuid.UUID, version domain.ModelVersion, expectedRevision int64, event identityapp.OutboxEvent) (domain.ModelVersion, error) {
	s.called = true
	s.actorID, s.orgID = actorID, orgID
	s.version, s.expectedRevision, s.event = version, expectedRevision, event
	if s.err != nil {
		return domain.ModelVersion{}, s.err
	}
	version.CreateTime = time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	return version, nil
}

func validPublishModelVersionInput(modelID uuid.UUID) catalogapp.PublishModelVersionInput {
	return catalogapp.PublishModelVersionInput{
		ModelID: modelID, ExpectedRevision: 1, VersionNo: 1,
		ProviderModelID: "seedance-2", Modes: []string{"image2video"},
		Limits:        json.RawMessage(`{"max_outputs":1}`),
		ParamSchema:   json.RawMessage(`[{"field":"prompt","label":"提示词","type":"string","component":"textarea","description":"private-parameter-description"}]`),
		SupportsQuery: true, SupportsCancel: true, SupportsCallback: false,
		ExpectedMaxMS: 60000, Moderation: domain.ModerationProvider, Queue: "agent",
		RequestID: uuid.NewString(),
	}
}

func TestPublishModelVersionCommandBuildsSafeAuditAndVersion(t *testing.T) {
	actor := adminPrincipal()
	store := &publishVersionStore{}
	validator := &publishVersionValidator{}
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	input := validPublishModelVersionInput(uuid.New())
	result, err := catalogapp.NewPublishModelVersionCommand(store, validator, func() time.Time { return now }).Execute(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if !validator.called || !store.called || store.actorID != actor.ID || store.orgID != actor.OrgID ||
		store.expectedRevision != input.ExpectedRevision || store.version.CreateBy != actor.ID ||
		store.version.ModelID != input.ModelID || store.version.VersionNo != input.VersionNo ||
		store.version.ProviderModelID != input.ProviderModelID ||
		!bytes.Equal(store.version.ParamSchema, input.ParamSchema) ||
		result.ID != store.version.ID || result.ModelID != input.ModelID ||
		result.VersionNo != input.VersionNo || result.Revision != input.ExpectedRevision+1 {
		t.Fatalf("version publish mismatch: result %+v, saved %+v", result, store.version)
	}
	if validator.version.ID != store.version.ID || validator.version.ModelID != store.version.ModelID {
		t.Fatal("validator did not see the version committed by the store")
	}
	if store.event.Topic != "lanverse.audit.recorded.v1" || store.event.PartitionKey != actor.OrgID.String() ||
		bytes.Contains(store.event.Payload, []byte("private-parameter-description")) {
		t.Fatalf("unsafe model version audit envelope: %+v", store.event)
	}
	audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: store.event.Topic, Key: []byte(store.event.PartitionKey), Value: store.event.Payload,
	})
	if err != nil || audit.Action != "model.version_published" || audit.ObjectType != "model_profile" ||
		audit.ObjectID != input.ModelID.String() || audit.RequestID != input.RequestID {
		t.Fatalf("invalid version audit %+v: %v", audit, err)
	}
	var after map[string]any
	if err := json.Unmarshal(audit.After, &after); err != nil ||
		after["version_no"] != float64(1) || after["provider_model_id"] != input.ProviderModelID {
		t.Fatalf("version audit summary %+v: %v", after, err)
	}
	for _, forbidden := range []string{"limits", "param_schema", "private-parameter-description"} {
		if bytes.Contains(store.event.Payload, []byte(forbidden)) {
			t.Fatalf("version audit contains %q", forbidden)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(store.event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	payload["data"].(map[string]any)["after"].(map[string]any)["secret"] = "forbidden"
	unsafePayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: store.event.Topic, Key: []byte(store.event.PartitionKey), Value: unsafePayload,
	}); !errors.Is(err, auditapp.ErrInvalidEvent) {
		t.Fatalf("model version audit accepted undeclared field: %v", err)
	}
}

func TestPublishModelVersionCommandRejectsBeforeStore(t *testing.T) {
	actor := adminPrincipal()
	input := validPublishModelVersionInput(uuid.New())
	for _, tc := range []struct {
		name   string
		actor  identityapp.Principal
		change func(*catalogapp.PublishModelVersionInput)
	}{
		{"producer", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleProducer}, nil},
		{"first login", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleAdmin, MustChangePassword: true}, nil},
		{"missing model", actor, func(v *catalogapp.PublishModelVersionInput) { v.ModelID = uuid.Nil }},
		{"missing revision", actor, func(v *catalogapp.PublishModelVersionInput) { v.ExpectedRevision = 0 }},
		{"missing version number", actor, func(v *catalogapp.PublishModelVersionInput) { v.VersionNo = 0 }},
		{"bad request", actor, func(v *catalogapp.PublishModelVersionInput) { v.RequestID = "not-a-uuid" }},
		{"missing provider model", actor, func(v *catalogapp.PublishModelVersionInput) { v.ProviderModelID = " " }},
		{"empty modes", actor, func(v *catalogapp.PublishModelVersionInput) { v.Modes = nil }},
		{"wrong JSON shape", actor, func(v *catalogapp.PublishModelVersionInput) { v.ParamSchema = json.RawMessage(`{}`) }},
		{"missing queue", actor, func(v *catalogapp.PublishModelVersionInput) { v.Queue = " " }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := input
			if tc.change != nil {
				tc.change(&candidate)
			}
			store := &publishVersionStore{}
			_, err := catalogapp.NewPublishModelVersionCommand(store, &publishVersionValidator{}, time.Now).Execute(t.Context(), tc.actor, candidate)
			if err == nil || store.called {
				t.Fatalf("invalid publication reached storage: %v", err)
			}
		})
	}
	validationErr := errors.New("configuration rejected")
	store := &publishVersionStore{}
	validator := &publishVersionValidator{err: validationErr}
	_, err := catalogapp.NewPublishModelVersionCommand(store, validator, time.Now).Execute(t.Context(), actor, input)
	if !errors.Is(err, validationErr) || !validator.called || store.called {
		t.Fatalf("validator rejection reached storage: %v", err)
	}
	store = &publishVersionStore{}
	_, err = catalogapp.NewPublishModelVersionCommand(store, &publishVersionValidator{}, func() time.Time { return time.Time{} }).Execute(t.Context(), actor, input)
	if err == nil || store.called {
		t.Fatalf("zero clock reached storage: %v", err)
	}
}

func TestPublishModelVersionCommandPreservesStoreFailure(t *testing.T) {
	storeErr := domain.ErrModelRevisionConflict
	store := &publishVersionStore{err: storeErr}
	_, err := catalogapp.NewPublishModelVersionCommand(store, &publishVersionValidator{}, time.Now).Execute(
		t.Context(), adminPrincipal(), validPublishModelVersionInput(uuid.New()),
	)
	if !errors.Is(err, storeErr) || !store.called {
		t.Fatalf("store conflict hidden: %v", err)
	}
}
