package canvas_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	canvashttp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type assetCreationRecorder struct {
	calls int
	input application.AssetCreationInput
}

func (r *assetCreationRecorder) CreateAsset(_ context.Context, _ identity.Principal, _ uuid.UUID, input application.AssetCreationInput) (application.CreatedAssetCanvas, error) {
	r.calls++
	r.input = input
	return application.CreatedAssetCanvas{ID: uuid.New(), ProjectID: uuid.New(), Name: input.Name, Kind: input.Kind, Collection: "current", Revision: 1, ProjectRevision: 1, TemplateKey: input.TemplateKey}, nil
}
func TestCanvasAssetCreationHTTPUsesExplicitCatalogAndRejectsInvalidCommand(t *testing.T) {
	store := &assetCreationRecorder{}
	router := gin.New()
	canvashttp.NewAssetCreationHandler(application.NewAssetCreator(store)).Register(router.Group("/api"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "GET", "/api/canvas-assets/templates?kind=short_drama", nil))
	var catalog application.AssetTemplateCatalog
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &catalog) != nil || len(catalog.Items) != 1 || catalog.Items[0].Kind != "short_drama" || !catalog.Items[0].IsTemplate {
		t.Fatal(response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "GET", "/api/canvas-assets/templates?kind=legacy", nil))
	if response.Code != 422 {
		t.Fatal(response.Code, response.Body.String())
	}
	for _, test := range []struct{ key, body string }{
		{"", `{}`}, {"00000000-0000-0000-0000-000000000000", `{}`}, {uuid.NewString(), `{"project_id":"ignored"}`}, {uuid.NewString(), `{"kind":"short_drama","template_key":"text-to-image-v1"}`}, {uuid.NewString(), `{"template_key":"unknown"}`},
	} {
		request := httptest.NewRequestWithContext(t.Context(), "POST", "/api/canvas-assets", strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", test.key)
		response = httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != 422 || store.calls != 0 {
			t.Fatal("invalid creation reached store", response.Code, response.Body.String(), store.calls)
		}
	}
	request := httptest.NewRequestWithContext(t.Context(), "POST", "/api/canvas-assets", strings.NewReader(`{"template_key":"text-to-image-v1"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", uuid.NewString())
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var created application.CreatedAssetCanvas
	if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &created) != nil || store.calls != 1 || created.Kind != "canvas" || created.Name != "文生图工作流" || created.IsTemplate || created.TemplateKey != "text-to-image-v1" {
		t.Fatal(response.Code, response.Body.String(), store.calls)
	}
}
