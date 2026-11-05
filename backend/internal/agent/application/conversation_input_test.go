package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

type draftInputFixture struct{ err error }

func (f draftInputFixture) FreezeConversationInput(_ context.Context, _ identity.Principal, _ uuid.UUID, d prompt.Draft) (prompt.ConversationInput, error) {
	if f.err != nil {
		return prompt.ConversationInput{}, f.err
	}
	return prompt.NewConversationInput(d)
}

type planningCatalogFixture struct{ image bool }

func (f planningCatalogFixture) ListModelsForProject(context.Context, identity.Principal, catalog.ListModelsInput) (catalog.ModelCatalogPage, error) {
	roles := []string{"prompt"}
	limits := json.RawMessage(`{}`)
	if f.image {
		roles = append(roles, "reference_image")
		limits = json.RawMessage(`{"roles":{"reference_image":{"types":["image"],"max_count":2}}}`)
	}
	return catalog.ModelCatalogPage{Models: []catalog.ModelCatalogItem{{ID: uuid.New(), ReleaseID: uuid.New(), Key: "planner", Capability: "text.generate", Status: "active", ProviderStatus: "active", CredentialPresent: true, InputRoles: roles, CurrentVersion: &catalog.ModelCatalogVersion{ID: uuid.New(), ContextWindow: 32000, Limits: limits}, CurrentPrice: &catalog.ModelCatalogPrice{}}}}, nil
}
func conversationJSON(t *testing.T, image bool) json.RawMessage {
	t.Helper()
	raw := json.RawMessage(`{"schemaVersion":1,"document":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"创作"}]}]},"references":[],"modelVersion":null,"mode":null,"params":{},"labelState":{"highWater":{"image":0,"video":0,"audio":0,"subject":0},"allocations":{}}}`)
	if !image {
		return raw
	}
	d, err := prompt.DecodeDraft(raw)
	if err != nil {
		t.Fatal(err)
	}
	d.Document.Content[0].Content = nil
	d.References = []prompt.Reference{{ID: "ref", Source: prompt.ReferenceSource{Kind: "media", SourceID: uuid.NewString(), SourceVersion: "2"}, BindingGeneration: 1, MediaKind: "image", Role: "reference_image"}}
	d.LabelState.HighWater["image"] = 1
	d.LabelState.Allocations["ref"] = prompt.LabelAllocation{LabelKind: "image", Ordinal: 1}
	raw, err = json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestConversationInputUsesLiveModelMediaCapabilities(t *testing.T) {
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New()}
	project, session := uuid.New(), uuid.New()
	text := NewConversationInputService(draftInputFixture{}, planningCatalogFixture{}, nil)
	_, model, err := text.Prepare(t.Context(), actor, project, session, conversationJSON(t, false), nil, "planner")
	if err != nil || model.ReleaseID == uuid.Nil {
		t.Fatal("text planner missing pinned release", err)
	}
	if _, _, err = text.Prepare(t.Context(), actor, project, session, conversationJSON(t, true), nil, "planner"); !errors.Is(err, prompt.ErrReferenceUnavailable) {
		t.Fatal("image silently accepted by a text-only planner", err)
	}
	image := NewConversationInputService(draftInputFixture{}, planningCatalogFixture{image: true}, nil)
	frozen, _, err := image.Prepare(t.Context(), actor, project, session, conversationJSON(t, true), nil, "planner")
	if err != nil || frozen.PlainText != "" || len(frozen.MediaPins) != 1 {
		t.Fatal("image-only intent changed", err)
	}
}
func TestConversationInputCannotBypassSourceRevocation(t *testing.T) {
	revoked := errors.New("source revoked")
	service := NewConversationInputService(draftInputFixture{err: revoked}, planningCatalogFixture{image: true}, nil)
	_, _, err := service.Prepare(t.Context(), identity.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New()}, uuid.New(), uuid.New(), conversationJSON(t, true), nil, "planner")
	if !errors.Is(err, revoked) {
		t.Fatal("source authorization ignored", err)
	}
}
