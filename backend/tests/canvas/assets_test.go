package canvas_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type assetQueryRecorder struct {
	calls int
	input application.AssetListInput
	page  application.AssetPage
}

func (r *assetQueryRecorder) ListAssets(_ context.Context, _ identity.Principal, input application.AssetListInput) (application.AssetPage, error) {
	r.calls++
	r.input = input
	if r.page.Items == nil {
		return application.AssetPage{Items: []application.AssetCanvas{}}, nil
	}
	return r.page, nil
}

func TestAssetCanvasKindsAreExplicitAndDefaultToCanvas(t *testing.T) {
	for _, value := range []string{"", "canvas", "short_drama"} {
		kind, err := domain.ParseDocumentKind(value)
		if err != nil || kind == "" {
			t.Fatal(value, kind, err)
		}
	}
	for _, value := range []string{"video", "story", "history", "short-drama", " canvas "} {
		if _, err := domain.ParseDocumentKind(value); !errors.Is(err, domain.ErrInvalidCommand) {
			t.Fatalf("unconfirmed classification accepted: %q %v", value, err)
		}
	}
}

func TestAssetCanvasQueryRejectsInvalidFiltersBeforeStore(t *testing.T) {
	from := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	before := from.Add(-time.Hour)
	for _, input := range []application.AssetListInput{
		{Kind: "history", Limit: 20},
		{Collection: "archived"},
		{Limit: 201},
		{Limit: 20, Sort: "name"},
		{Limit: 20, Order: "sideways"},
		{Limit: 20, CreatedFrom: &from, CreatedBefore: &before},
		{Limit: 20, After: &application.AssetCursor{ID: uuid.New()}},
	} {
		store := &assetQueryRecorder{}
		_, err := application.NewAssetQuery(store).List(t.Context(), identity.Principal{}, input)
		if !errors.Is(err, domain.ErrInvalidCommand) || store.calls != 0 {
			t.Fatalf("invalid filter reached store: %+v %v calls=%d", input, err, store.calls)
		}
	}
}

func TestAssetCanvasQueryNormalizesDefaultsAndSearch(t *testing.T) {
	store := &assetQueryRecorder{}
	_, err := application.NewAssetQuery(store).List(t.Context(), identity.Principal{}, application.AssetListInput{Search: "  画布  "})
	if err != nil || store.calls != 1 || store.input.Kind != "canvas" || store.input.Sort != "updated" || store.input.Order != "desc" || store.input.Limit != 30 || store.input.Search != "画布" {
		t.Fatal(store, err)
	}
}

func TestAssetCanvasCopyPreservesExplicitKindAndLegacyHashShape(t *testing.T) {
	for _, kind := range []string{"", "short_drama"} {
		source := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Name: "测试", Kind: kind, Revision: 1, Viewport: domain.Viewport{Zoom: 1}, Scope: json.RawMessage(`{}`), Nodes: []domain.Node{}, Edges: []domain.Edge{}}
		copied, err := domain.CopyProjectDocument(source, uuid.New(), uuid.New(), nil)
		if err != nil || copied.Document.Kind != kind {
			t.Fatal(copied, err)
		}
		dto := application.Snapshot(source)
		want := kind
		if want == "" {
			want = "canvas"
		}
		if dto.Kind != want {
			t.Fatalf("public kind=%q want=%q", dto.Kind, want)
		}
		if kind == "" {
			body, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if json.Unmarshal(body, &fields) != nil || fields["kind"] != nil {
				t.Fatal("default kind changed legacy frozen JSON", string(body))
			}
		}
	}
}

func TestAssetCanvasLegacyCopyRefusesWithoutProtocolGuessing(t *testing.T) {
	source := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), AssetCollection: "legacy", Name: "旧来源", Revision: 1, Viewport: domain.Viewport{Zoom: 1}, Scope: json.RawMessage(`{}`), Nodes: []domain.Node{}, Edges: []domain.Edge{}}
	for _, version := range []int{0, 1} {
		source.OperationVersion = version
		if _, err := domain.CopyProjectDocument(source, uuid.New(), uuid.New(), nil); !errors.Is(err, domain.ErrUnsupportedProjectCopy) {
			t.Fatal("legacy copy inferred from command protocol", version, err)
		}
	}
}
