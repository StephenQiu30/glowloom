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

type createModelStore struct {
	created domain.ModelProfile
	event   identityapp.OutboxEvent
	called  bool
}

func (s *createModelStore) CreateModelWithAudit(_ context.Context, _, _ uuid.UUID, model domain.ModelProfile, event identityapp.OutboxEvent) (domain.ModelProfile, error) {
	s.called = true
	s.created = model
	s.event = event
	s.created.CreateTime = time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	s.created.UpdateTime = s.created.CreateTime
	return s.created, nil
}

func TestCreateModelCommandBuildsSafeAuditAndDisabledModel(t *testing.T) {
	store := &createModelStore{}
	actor := adminPrincipal()
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	requestID := uuid.NewString()
	providerID := uuid.New()
	created, err := catalogapp.NewCreateModelCommand(store, func() time.Time { return now }).Execute(
		t.Context(), actor, catalogapp.CreateModelInput{
			Key: "ark.seedance-2-pro", ProviderID: providerID,
			Capability: "video.generate", DisplayName: "Seedance 2 Pro", RequestID: requestID,
		})
	if err != nil || !store.called || created.ID != store.created.ID ||
		created.Status != domain.ModelDisabled || created.Revision != 1 ||
		created.CurrentVersionID != uuid.Nil {
		t.Fatalf("created model %+v: %v", created, err)
	}
	if store.event.Topic != "lanverse.audit.recorded.v1" || store.event.PartitionKey != actor.OrgID.String() {
		t.Fatalf("wrong audit event: %+v", store.event)
	}
	audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: store.event.Topic, Key: []byte(store.event.PartitionKey), Value: store.event.Payload,
	})
	if err != nil || audit.Action != "model.created" || audit.ObjectType != "model_profile" ||
		audit.ObjectID != created.ID.String() || audit.RequestID != requestID {
		t.Fatalf("invalid model audit %+v: %v", audit, err)
	}
	var after map[string]any
	if err := json.Unmarshal(audit.After, &after); err != nil || after["key"] != created.Key ||
		after["provider_id"] != providerID.String() || after["capability"] != "video.generate" ||
		after["display_name"] != "Seedance 2 Pro" || after["status"] != "disabled" {
		t.Fatalf("model audit summary %+v: %v", after, err)
	}
	var changed map[string]any
	if err := json.Unmarshal(store.event.Payload, &changed); err != nil {
		t.Fatal(err)
	}
	changed["data"].(map[string]any)["after"].(map[string]any)["secret"] = "forbidden"
	unsafePayload, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: store.event.Topic, Key: []byte(store.event.PartitionKey), Value: unsafePayload,
	}); !errors.Is(err, auditapp.ErrInvalidEvent) {
		t.Fatalf("model audit accepted undeclared field: %v", err)
	}
}

func TestCreateModelCommandRejectsInvalidCallerAndModel(t *testing.T) {
	actor := adminPrincipal()
	valid := catalogapp.CreateModelInput{
		Key: "ark.seedance-2-pro", ProviderID: uuid.New(),
		Capability: "video.generate", DisplayName: "Seedance 2 Pro", RequestID: uuid.NewString(),
	}
	for _, tc := range []struct {
		name   string
		actor  identityapp.Principal
		change func(*catalogapp.CreateModelInput)
	}{
		{"producer", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleProducer}, nil},
		{"first login", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleAdmin, MustChangePassword: true}, nil},
		{"missing provider", actor, func(v *catalogapp.CreateModelInput) { v.ProviderID = uuid.Nil }},
		{"bad key", actor, func(v *catalogapp.CreateModelInput) { v.Key = "Ark/Seedance" }},
		{"blank name", actor, func(v *catalogapp.CreateModelInput) { v.DisplayName = " " }},
		{"missing capability", actor, func(v *catalogapp.CreateModelInput) { v.Capability = "" }},
		{"bad request", actor, func(v *catalogapp.CreateModelInput) { v.RequestID = "no" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := valid
			if tc.change != nil {
				tc.change(&input)
			}
			store := &createModelStore{}
			_, err := catalogapp.NewCreateModelCommand(store, time.Now).Execute(t.Context(), tc.actor, input)
			if err == nil || store.called {
				t.Fatalf("invalid model reached storage: %v", err)
			}
		})
	}
	store := &createModelStore{}
	command := catalogapp.NewCreateModelCommand(store, func() time.Time { return time.Time{} })
	if _, err := command.Execute(t.Context(), actor, valid); !errors.Is(err, catalogapp.ErrInvalidCreateModel) || store.called {
		t.Fatalf("zero clock reached storage: %v", err)
	}
}
