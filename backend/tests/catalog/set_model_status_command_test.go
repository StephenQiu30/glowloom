package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
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

type setModelStatusStore struct {
	called           bool
	actorID          uuid.UUID
	orgID            uuid.UUID
	modelID          uuid.UUID
	status           domain.ModelStatus
	expectedRevision int64
	event            identityapp.OutboxEvent
	saved            domain.ModelProfile
	err              error
}

func (s *setModelStatusStore) SetModelStatusWithAudit(_ context.Context, actorID, orgID, modelID uuid.UUID, status domain.ModelStatus, expectedRevision int64, event identityapp.OutboxEvent) (domain.ModelProfile, error) {
	s.called = true
	s.actorID, s.orgID, s.modelID = actorID, orgID, modelID
	s.status, s.expectedRevision, s.event = status, expectedRevision, event
	if s.err != nil {
		return domain.ModelProfile{}, s.err
	}
	if s.saved.ID != uuid.Nil {
		return s.saved, nil
	}
	s.saved = domain.ModelProfile{
		ID: modelID, Key: "ark.seedance-2-pro", ProviderID: uuid.New(),
		Capability: "video.generate", DisplayName: "Seedance 2 Pro",
		Status: status, CurrentVersionID: uuid.New(), Revision: expectedRevision + 1,
		UpdateTime: time.Date(2026, 9, 27, 10, 1, 0, 0, time.UTC),
	}
	return s.saved, nil
}

func validSetModelStatusInput(status domain.ModelStatus) catalogapp.SetModelStatusInput {
	return catalogapp.SetModelStatusInput{
		ModelID: uuid.New(), ExpectedRevision: 3, Status: status, RequestID: uuid.NewString(),
	}
}

func TestSetModelStatusCommandBuildsSafeAuditAndReturnsChangedModel(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    domain.ModelStatus
		oldStatus domain.ModelStatus
		action    string
	}{
		{name: "enable", status: domain.ModelActive, oldStatus: domain.ModelDisabled, action: "model.enabled"},
		{name: "disable", status: domain.ModelDisabled, oldStatus: domain.ModelActive, action: "model.disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actor := adminPrincipal()
			store := &setModelStatusStore{}
			now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
			input := validSetModelStatusInput(tc.status)
			changed, err := catalogapp.NewSetModelStatusCommand(store, func() time.Time { return now }).Execute(t.Context(), actor, input)
			if err != nil {
				t.Fatal(err)
			}
			if !store.called || store.actorID != actor.ID || store.orgID != actor.OrgID ||
				store.modelID != input.ModelID || store.status != input.Status ||
				store.expectedRevision != input.ExpectedRevision ||
				changed.ID != input.ModelID || changed.Status != tc.status ||
				changed.Revision != input.ExpectedRevision+1 ||
				!changed.UpdateTime.Equal(store.saved.UpdateTime) {
				t.Fatalf("status change mismatch: changed %+v, store %+v", changed, store)
			}
			if store.event.Topic != "lanverse.audit.recorded.v1" || store.event.PartitionKey != actor.OrgID.String() {
				t.Fatalf("wrong audit envelope: %+v", store.event)
			}
			audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
				Topic: store.event.Topic, Key: []byte(store.event.PartitionKey), Value: store.event.Payload,
			})
			if err != nil || audit.Action != tc.action || audit.ObjectType != "model_profile" ||
				audit.ObjectID != input.ModelID.String() || audit.RequestID != input.RequestID {
				t.Fatalf("invalid status audit %+v: %v", audit, err)
			}
			var before map[string]any
			if err := json.Unmarshal(audit.Before, &before); err != nil || len(before) != 2 ||
				before["status"] != string(tc.oldStatus) || before["revision"] != float64(input.ExpectedRevision) {
				t.Fatalf("status audit before summary %+v: %v", before, err)
			}
			var after map[string]any
			if err := json.Unmarshal(audit.After, &after); err != nil || len(after) != 2 ||
				after["status"] != string(tc.status) || after["revision"] != float64(input.ExpectedRevision+1) {
				t.Fatalf("status audit summary %+v: %v", after, err)
			}
			for _, summary := range []map[string]any{before, after} {
				for name, value := range summary {
					switch value.(type) {
					case nil, bool, float64, string:
					default:
						t.Fatalf("nested status audit field %q: %T", name, value)
					}
				}
			}
			var payload map[string]any
			if err := json.Unmarshal(store.event.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			payload["data"].(map[string]any)["after"].(map[string]any)["provider_model_id"] = "forbidden"
			unsafePayload, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
				Topic: store.event.Topic, Key: []byte(store.event.PartitionKey), Value: unsafePayload,
			}); !errors.Is(err, auditapp.ErrInvalidEvent) {
				t.Fatalf("status audit accepted undeclared field: %v", err)
			}
		})
	}
}

func TestSetModelStatusCommandRejectsInvalidInputBeforeStore(t *testing.T) {
	actor := adminPrincipal()
	valid := validSetModelStatusInput(domain.ModelActive)
	for _, tc := range []struct {
		name   string
		actor  identityapp.Principal
		change func(*catalogapp.SetModelStatusInput)
		want   error
	}{
		{name: "producer", actor: identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleProducer}, want: identityapp.ErrForbidden},
		{name: "first login", actor: identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleAdmin, MustChangePassword: true}, want: identityapp.ErrForbidden},
		{name: "missing actor", actor: identityapp.Principal{OrgID: actor.OrgID, Role: identitydomain.RoleAdmin}, want: identityapp.ErrForbidden},
		{name: "missing organization", actor: identityapp.Principal{ID: actor.ID, Role: identitydomain.RoleAdmin}, want: identityapp.ErrForbidden},
		{name: "missing model", actor: actor, change: func(v *catalogapp.SetModelStatusInput) { v.ModelID = uuid.Nil }},
		{name: "zero revision", actor: actor, change: func(v *catalogapp.SetModelStatusInput) { v.ExpectedRevision = 0 }},
		{name: "negative revision", actor: actor, change: func(v *catalogapp.SetModelStatusInput) { v.ExpectedRevision = -1 }},
		{name: "maximum revision", actor: actor, change: func(v *catalogapp.SetModelStatusInput) { v.ExpectedRevision = math.MaxInt32 }},
		{name: "missing status", actor: actor, change: func(v *catalogapp.SetModelStatusInput) { v.Status = "" }},
		{name: "unknown status", actor: actor, change: func(v *catalogapp.SetModelStatusInput) { v.Status = "paused" }},
		{name: "bad request", actor: actor, change: func(v *catalogapp.SetModelStatusInput) { v.RequestID = "not-a-uuid" }},
		{name: "missing request", actor: actor, change: func(v *catalogapp.SetModelStatusInput) { v.RequestID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := valid
			if tc.change != nil {
				tc.change(&input)
			}
			store := &setModelStatusStore{}
			_, err := catalogapp.NewSetModelStatusCommand(store, time.Now).Execute(t.Context(), tc.actor, input)
			if err == nil || store.called || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("invalid status change reached store: %v", err)
			}
		})
	}
	store := &setModelStatusStore{}
	_, err := catalogapp.NewSetModelStatusCommand(store, func() time.Time { return time.Time{} }).Execute(t.Context(), actor, valid)
	if err == nil || store.called {
		t.Fatalf("zero clock reached store: %v", err)
	}
}

func TestSetModelStatusCommandPreservesStoreFailure(t *testing.T) {
	for _, storeErr := range []error{domain.ErrModelRevisionConflict, domain.ErrModelNotPublishable, domain.ErrInvalidModelTransition} {
		store := &setModelStatusStore{err: storeErr}
		_, err := catalogapp.NewSetModelStatusCommand(store, time.Now).Execute(
			t.Context(), adminPrincipal(), validSetModelStatusInput(domain.ModelActive),
		)
		if !errors.Is(err, storeErr) || !store.called {
			t.Fatalf("store failure %v hidden: %v", storeErr, err)
		}
	}
}
