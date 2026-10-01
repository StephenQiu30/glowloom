package catalog_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	cataloghttp "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/http"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

type publicModelStore struct {
	input catalogapp.ListModelsInput
	page  catalogapp.ModelCatalogPage
	calls int
}

func (s *publicModelStore) ListModelsForProject(_ context.Context, _ identityapp.Principal, input catalogapp.ListModelsInput) (catalogapp.ModelCatalogPage, error) {
	s.calls++
	s.input = input
	return s.page, nil
}

func TestPublicModelCatalogReturnsSchemaAndBindsCursorToProject(t *testing.T) {
	project := uuid.New()
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer}
	store := &publicModelStore{page: catalogapp.ModelCatalogPage{
		Models: []catalogapp.ModelCatalogItem{{ID: uuid.New(), Key: "image-model", DisplayName: "Image", Capability: "image.generate", Status: domain.ModelActive,
			CurrentVersion: &catalogapp.ModelCatalogVersion{ID: uuid.New(), VersionNo: 1, Modes: []string{"text_to_image"}, Limits: json.RawMessage(`{"max_outputs":1}`), ParamSchema: json.RawMessage(`[{"key":"seed","type":"integer"}]`)}}},
		Next: &catalogapp.ModelCatalogCursor{ID: uuid.New(), Key: "image-model"},
	}}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("principal", actor) })
	cataloghttp.NewHandler(catalogapp.NewListModelsQuery(store)).Register(router.Group("/api"))
	request := func(path string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequestWithContext(t.Context(), "GET", path, nil))
		return res
	}
	res := request("/api/models?project_id=" + project.String() + "&capability=image.generate&limit=1")
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"param_schema":[{"key":"seed","type":"integer"}]`) || store.input.ProjectID != project || store.input.Limit != 1 {
		t.Fatalf("catalog response = %d %s, input %+v", res.Code, res.Body, store.input)
	}
	var page cataloghttp.ModelPage
	if err := json.Unmarshal(res.Body.Bytes(), &page); err != nil || page.NextCursor == nil {
		t.Fatalf("catalog cursor: %+v %v", page, err)
	}
	res = request("/api/models?project_id=" + uuid.NewString() + "&capability=image.generate&limit=1&cursor=" + *page.NextCursor)
	if res.Code != 422 || store.calls != 1 {
		t.Fatalf("cross-project cursor reached store: %d %s, calls=%d", res.Code, res.Body, store.calls)
	}
	res = request("/api/models?project_id=" + project.String() + "&capability=image.generate&limit=1&cursor=" + *page.NextCursor)
	if res.Code != 200 || store.input.After == nil || store.input.After.Key != "image-model" {
		t.Fatalf("same-project cursor: %d %s, input=%+v", res.Code, res.Body, store.input)
	}
}

func TestPublicModelCatalogRejectsInvalidScopeBeforeReading(t *testing.T) {
	store := &publicModelStore{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("principal", identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer})
	})
	cataloghttp.NewHandler(catalogapp.NewListModelsQuery(store)).Register(router.Group("/api"))
	for _, path := range []string{"/api/models", "/api/models?project_id=invalid", "/api/models?project_id=" + uuid.NewString() + "&limit=201", "/api/models?project_id=" + uuid.NewString() + "&limit=-1"} {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequestWithContext(t.Context(), "GET", path, nil))
		if res.Code != 422 || store.calls != 0 {
			t.Fatalf("invalid request reached storage: %s, %d %s", path, res.Code, res.Body)
		}
	}
}
