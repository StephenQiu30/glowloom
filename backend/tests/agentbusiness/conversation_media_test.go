package agentbusiness_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"

	"github.com/google/uuid"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

type multimediaCatalog struct{ maxImages int }

func (f multimediaCatalog) ListModelsForProject(context.Context, identity.Principal, catalog.ListModelsInput) (catalog.ModelCatalogPage, error) {
	page := catalog.ModelCatalogPage{Models: []catalog.ModelCatalogItem{{ID: uuid.New(), ReleaseID: uuid.New(), Key: "planner", Capability: "text.generate", Status: "active", ProviderStatus: "active", CredentialPresent: true, InputRoles: []string{"prompt", "reference_image"}, CurrentVersion: &catalog.ModelCatalogVersion{ID: uuid.New(), ContextWindow: 32000}, CurrentPrice: &catalog.ModelCatalogPrice{}}}}
	page.Models[0].CurrentVersion.Limits = json.RawMessage(fmt.Sprintf(`{"roles":{"reference_image":{"types":["image"],"max_count":%d}}}`, f.maxImages))
	return page, nil
}
func TestConversationAdmissionBoundsSourcesAndCountsVideoFrameProjection(t *testing.T) {
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New()}
	d, err := prompt.DecodeDraft(json.RawMessage(`{"schemaVersion":1,"document":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"创作"}]}]},"references":[],"modelVersion":null,"mode":null,"params":{},"labelState":{"highWater":{"image":0,"video":0,"audio":0,"subject":0},"allocations":{}}}`))
	if err != nil {
		t.Fatal(err)
	}
	d.LabelState.HighWater["document"] = 0
	for _, kind := range []string{"image", "video", "document"} {
		for i := 0; i < 30; i++ {
			id := fmt.Sprintf("%s-%d", kind, i)
			d.References = append(d.References, prompt.Reference{ID: id, Source: prompt.ReferenceSource{Kind: "media", SourceID: uuid.NewString(), SourceVersion: "1"}, BindingGeneration: 1, MediaKind: kind, Role: "reference_" + kind, Order: len(d.References)})
			d.LabelState.HighWater[kind]++
			d.LabelState.Allocations[id] = prompt.LabelAllocation{LabelKind: kind, Ordinal: int64(i + 1)}
		}
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	service := app.NewConversationInputService(draftInputFixture{}, multimediaCatalog{maxImages: 120}, nil)
	if _, _, err := service.Prepare(t.Context(), actor, uuid.New(), uuid.New(), raw, nil, "planner"); err != nil {
		t.Fatal("30 images + 30 videos + 30 documents rejected", err)
	}
	service = app.NewConversationInputService(draftInputFixture{}, multimediaCatalog{maxImages: 119}, nil)
	if _, _, err := service.Prepare(t.Context(), actor, uuid.New(), uuid.New(), raw, nil, "planner"); !errors.Is(err, prompt.ErrReferenceUnavailable) {
		t.Fatal("projected frame limit bypassed", err)
	}
	d.References = append(d.References, prompt.Reference{ID: "image-31", Source: prompt.ReferenceSource{Kind: "media", SourceID: uuid.NewString(), SourceVersion: "1"}, BindingGeneration: 1, MediaKind: "image", Role: "reference_image", Order: len(d.References)})
	d.LabelState.HighWater["image"] = 31
	d.LabelState.Allocations["image-31"] = prompt.LabelAllocation{LabelKind: "image", Ordinal: 31}
	raw, _ = json.Marshal(d)
	service = app.NewConversationInputService(draftInputFixture{}, multimediaCatalog{maxImages: 121}, nil)
	if _, _, err := service.Prepare(t.Context(), actor, uuid.New(), uuid.New(), raw, nil, "planner"); !errors.Is(err, prompt.ErrReferenceUnavailable) {
		t.Fatal("31st image accepted", err)
	}
}

type draftInputFixture struct{}

func (draftInputFixture) FreezeConversationInput(_ context.Context, _ identity.Principal, _ uuid.UUID, d prompt.Draft) (prompt.ConversationInput, error) {
	return prompt.NewConversationInput(d)
}
