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
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/credentialschema"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/credentialseal"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

type credentialCommandStore struct {
	provider domain.Provider
	saved    domain.Credential
	events   []identityapp.OutboxEvent
	called   bool
	saveErr  error
}

func (s *credentialCommandStore) FindProviderForAdmin(_ context.Context, _, _, _ uuid.UUID) (domain.Provider, error) {
	return s.provider, nil
}

func (s *credentialCommandStore) ReplaceWithEvents(_ context.Context, _, _ uuid.UUID, credential domain.Credential, events []identityapp.OutboxEvent) (domain.Credential, error) {
	s.called = true
	if s.saveErr != nil {
		return domain.Credential{}, s.saveErr
	}
	s.saved, s.events = credential, events
	s.saved.CreateTime = time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	s.saved.UpdateTime = s.saved.CreateTime
	return s.saved, nil
}

func newCredentialCommand(t *testing.T, store catalogapp.SetCredentialStore) *catalogapp.SetCredentialCommand {
	t.Helper()
	_, publicPEM := testCredentialKey(t)
	sealer, err := credentialseal.NewSealer("agent-2026", publicPEM)
	if err != nil {
		t.Fatal(err)
	}
	return catalogapp.NewSetCredentialCommand(store, credentialschema.NewRegistry(), sealer, time.Now)
}

func adminPrincipal() identityapp.Principal {
	return identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleAdmin}
}

func TestSetCredentialCommandReturnsOnlySummaryAndSafeEvents(t *testing.T) {
	store := &credentialCommandStore{provider: validProvider()}
	store.provider.AdapterKey = "minimax"
	command := newCredentialCommand(t, store)
	actor := adminPrincipal()
	summary, err := command.Execute(t.Context(), actor, catalogapp.SetCredentialInput{
		ProviderID: store.provider.ID, Label: "primary",
		Secret:    json.RawMessage(`{"api_key":"local-test-value","group_id":"test-group"}`),
		RequestID: uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("save credential: %v", err)
	}
	if !store.called || summary.ID != store.saved.ID || summary.Last4 != "alue" ||
		summary.Label != "primary" || summary.Status != domain.CredentialActive ||
		store.saved.KeyID != "agent-2026" || len(store.saved.Ciphertext) == 0 {
		t.Fatalf("unsafe or missing save result: summary %+v", summary)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("local-test-value")) || bytes.Contains(encoded, []byte("ciphertext")) ||
		bytes.Contains(encoded, []byte("key_id")) {
		t.Fatalf("response exposed credential material: %s", encoded)
	}
	if len(store.events) != 2 || store.events[0].Topic != "lanverse.catalog.credential_changed.v1" ||
		store.events[1].Topic != "lanverse.audit.recorded.v1" {
		t.Fatalf("wrong outbox topics: %+v", store.events)
	}
	for _, event := range store.events {
		if event.PartitionKey != actor.OrgID.String() ||
			bytes.Contains(event.Payload, []byte("local-test-value")) ||
			bytes.Contains(event.Payload, []byte("test-group")) ||
			bytes.Contains(event.Payload, []byte("ciphertext")) ||
			bytes.Contains(event.Payload, []byte("api_key")) {
			t.Fatal("outbox exposed secret or used the wrong partition key")
		}
	}
	audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: store.events[1].Topic, Key: []byte(store.events[1].PartitionKey),
		Value: store.events[1].Payload,
	})
	if err != nil || audit.Action != "credential.set" || string(audit.After) != `{"last4":"alue"}` {
		t.Fatalf("credential audit summary: action %q after %s error %v", audit.Action, audit.After, err)
	}
	var unsafe map[string]any
	if err := json.Unmarshal(store.events[1].Payload, &unsafe); err != nil {
		t.Fatal(err)
	}
	unsafe["data"].(map[string]any)["after"] = map[string]any{"api_key": "local-test-value"}
	unsafePayload, err := json.Marshal(unsafe)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: store.events[1].Topic, Key: []byte(store.events[1].PartitionKey),
		Value: unsafePayload,
	}); !errors.Is(err, auditapp.ErrInvalidEvent) {
		t.Fatalf("audit parser accepted credential plaintext: %v", err)
	}
}

func TestSetCredentialCommandRejectsInvalidActorAndSecret(t *testing.T) {
	actor := adminPrincipal()
	for _, candidate := range []struct {
		name     string
		actor    identityapp.Principal
		adapter  string
		secret   json.RawMessage
		disabled bool
	}{
		{name: "producer", actor: identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleProducer}, adapter: "minimax", secret: json.RawMessage(`{"api_key":"test1234","group_id":"g"}`)},
		{name: "first login", actor: identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleAdmin, MustChangePassword: true}, adapter: "minimax", secret: json.RawMessage(`{"api_key":"test1234","group_id":"g"}`)},
		{name: "unknown adapter", actor: actor, adapter: "unknown", secret: json.RawMessage(`{"api_key":"test1234"}`)},
		{name: "missing group ID", actor: actor, adapter: "minimax", secret: json.RawMessage(`{"api_key":"test1234"}`)},
		{name: "disabled provider", actor: actor, adapter: "minimax", secret: json.RawMessage(`{"api_key":"test1234","group_id":"g"}`), disabled: true},
	} {
		t.Run(candidate.name, func(t *testing.T) {
			store := &credentialCommandStore{provider: validProvider()}
			store.provider.AdapterKey = candidate.adapter
			if candidate.disabled {
				store.provider.Status = domain.ProviderDisabled
			}
			_, err := newCredentialCommand(t, store).Execute(t.Context(), candidate.actor, catalogapp.SetCredentialInput{
				ProviderID: store.provider.ID, Label: "primary", Secret: candidate.secret,
				RequestID: uuid.NewString(),
			})
			if err == nil || store.called {
				t.Fatalf("invalid credential reached storage: %v", err)
			}
		})
	}
	store := &credentialCommandStore{provider: validProvider(), saveErr: errors.New("test transaction failure")}
	_, err := newCredentialCommand(t, store).Execute(t.Context(), actor, catalogapp.SetCredentialInput{
		ProviderID: store.provider.ID, Label: "primary", Secret: json.RawMessage(`{"api_key":"test1234","group_id":"g"}`),
		RequestID: uuid.NewString(),
	})
	if err == nil || !store.called {
		t.Fatalf("store failure was not propagated: %v", err)
	}
	store = &credentialCommandStore{provider: validProvider()}
	_, err = newCredentialCommand(t, store).Execute(t.Context(), actor, catalogapp.SetCredentialInput{
		ProviderID: store.provider.ID, Label: "primary",
		Secret:    json.RawMessage(`{"api_key":"test1234","group_id":"g"}`),
		RequestID: "secret-or-untrusted-request-id",
	})
	if !errors.Is(err, catalogapp.ErrInvalidSetCredential) || store.called {
		t.Fatalf("untrusted request ID reached storage: %v", err)
	}
}
