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

type disableCredentialStore struct {
	target catalogapp.CredentialToDisable
	events []identityapp.OutboxEvent
	called bool
}

func (s *disableCredentialStore) FindCredentialForAdmin(_ context.Context, _, _, _, _ uuid.UUID) (catalogapp.CredentialToDisable, error) {
	return s.target, nil
}

func (s *disableCredentialStore) DisableCredentialWithEvents(_ context.Context, _, _ uuid.UUID, target catalogapp.CredentialToDisable, events []identityapp.OutboxEvent) (catalogapp.SavedCredential, error) {
	s.called = true
	s.events = events
	return catalogapp.SavedCredential{
		ID: target.ID, ProviderID: target.ProviderID, Label: "primary",
		Last4: target.Last4, Status: domain.CredentialDisabled,
		CreateTime: time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC),
		UpdateTime: time.Date(2026, 9, 27, 1, 0, 0, 0, time.UTC),
	}, nil
}

func TestDisableCredentialCommandReturnsSafeSummaryAndEvents(t *testing.T) {
	actor := adminPrincipal()
	target := catalogapp.CredentialToDisable{ID: uuid.New(), ProviderID: uuid.New(), Last4: "1234"}
	store := &disableCredentialStore{target: target}
	command := catalogapp.NewDisableCredentialCommand(store, func() time.Time {
		return time.Date(2026, 9, 27, 1, 0, 0, 0, time.UTC)
	})
	summary, err := command.Execute(t.Context(), actor, catalogapp.DisableCredentialInput{
		ProviderID: target.ProviderID, CredentialID: target.ID, RequestID: uuid.NewString(),
	})
	if err != nil || !store.called || summary.ID != target.ID || summary.Status != domain.CredentialDisabled || summary.Last4 != "1234" {
		t.Fatalf("disable summary %+v: %v", summary, err)
	}
	encoded, err := json.Marshal(summary)
	if err != nil || bytes.Contains(encoded, []byte("ciphertext")) || bytes.Contains(encoded, []byte("key_id")) {
		t.Fatalf("unsafe disable summary %s: %v", encoded, err)
	}
	if len(store.events) != 2 || store.events[0].Topic != "lanverse.catalog.credential_changed.v1" ||
		store.events[1].Topic != "lanverse.audit.recorded.v1" {
		t.Fatalf("wrong disable events: %+v", store.events)
	}
	var changed struct {
		Data struct {
			Change string `json:"change"`
		} `json:"data"`
	}
	if err := json.Unmarshal(store.events[0].Payload, &changed); err != nil || changed.Data.Change != "disabled" {
		t.Fatalf("invalid changed event: %v %+v", err, changed)
	}
	audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: store.events[1].Topic, Key: []byte(actor.OrgID.String()), Value: store.events[1].Payload,
	})
	if err != nil || audit.Action != "credential.disabled" || string(audit.After) != `{"last4":"1234"}` {
		t.Fatalf("invalid disable audit: %+v error %v", audit, err)
	}
	var unsafe map[string]any
	if err := json.Unmarshal(store.events[1].Payload, &unsafe); err != nil {
		t.Fatal(err)
	}
	unsafe["data"].(map[string]any)["after"] = map[string]any{"api_key": "forbidden"}
	unsafePayload, err := json.Marshal(unsafe)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: store.events[1].Topic, Key: []byte(actor.OrgID.String()), Value: unsafePayload,
	}); !errors.Is(err, auditapp.ErrInvalidEvent) {
		t.Fatalf("disable audit accepted credential material: %v", err)
	}
}

func TestDisableCredentialCommandRejectsInvalidCallerAndTarget(t *testing.T) {
	actor := adminPrincipal()
	target := catalogapp.CredentialToDisable{ID: uuid.New(), ProviderID: uuid.New(), Last4: "1234"}
	for _, tc := range []struct {
		name  string
		actor identityapp.Principal
		input catalogapp.DisableCredentialInput
	}{
		{"producer", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleProducer}, catalogapp.DisableCredentialInput{ProviderID: target.ProviderID, CredentialID: target.ID, RequestID: uuid.NewString()}},
		{"first login", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleAdmin, MustChangePassword: true}, catalogapp.DisableCredentialInput{ProviderID: target.ProviderID, CredentialID: target.ID, RequestID: uuid.NewString()}},
		{"wrong provider", actor, catalogapp.DisableCredentialInput{ProviderID: uuid.New(), CredentialID: target.ID, RequestID: uuid.NewString()}},
		{"untrusted request", actor, catalogapp.DisableCredentialInput{ProviderID: target.ProviderID, CredentialID: target.ID, RequestID: "not-a-uuid"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &disableCredentialStore{target: target}
			_, err := catalogapp.NewDisableCredentialCommand(store, time.Now).Execute(t.Context(), tc.actor, tc.input)
			if err == nil || store.called {
				t.Fatalf("invalid disable reached storage: %v", err)
			}
		})
	}
}
