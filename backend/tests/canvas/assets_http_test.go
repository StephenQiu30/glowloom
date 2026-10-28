package canvas_test

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	canvashttp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestAssetCanvasCursorBindsIdentityScopeAndEveryQueryDimension(t *testing.T) {
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New()}
	position := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	store := &assetQueryRecorder{page: application.AssetPage{Items: []application.AssetCanvas{}, Next: &application.AssetCursor{ID: uuid.New(), Time: position}}}
	router := gin.New()
	group := router.Group("/api", func(c *gin.Context) { c.Set("principal", actor) })
	canvashttp.NewAssetHandler(application.NewAssetQuery(store)).Register(group)
	query := url.Values{"kind": {"short_drama"}, "search": {"画布"}, "sort": {"created"}, "order": {"asc"}, "limit": {"2"}, "created_from": {"2026-10-01T00:00:00Z"}, "created_before": {"2026-11-01T00:00:00Z"}}
	get := func(values url.Values) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "GET", "/api/canvases?"+values.Encode(), nil))
		return response
	}
	response := get(query)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var page canvashttp.AssetListResponse
	if json.Unmarshal(response.Body.Bytes(), &page) != nil || page.NextCursor == nil {
		t.Fatal(response.Body.String())
	}
	query.Set("cursor", *page.NextCursor)
	if response := get(query); response.Code != 200 || store.input.After == nil || !store.input.After.Time.Equal(position) {
		t.Fatal(response.Code, store.input)
	}
	for key, changed := range map[string]string{"collection": "legacy", "favorite_only": "true", "kind": "canvas", "search": "其他", "sort": "updated", "order": "desc", "limit": "3", "created_from": "2026-10-02T00:00:00Z", "created_before": "2026-11-02T00:00:00Z"} {
		before := query.Get(key)
		query.Set(key, changed)
		calls := store.calls
		if response := get(query); response.Code != 422 || store.calls != calls {
			t.Fatalf("%s escaped binding: %d calls %d/%d", key, response.Code, store.calls, calls)
		}
		query.Set(key, before)
	}
	for _, field := range []*uuid.UUID{&actor.ID, &actor.OrgID, &actor.ScopeID} {
		before := *field
		*field = uuid.New()
		calls := store.calls
		if response := get(query); response.Code != 422 || store.calls != calls {
			t.Fatalf("identity escaped cursor binding: %d", response.Code)
		}
		*field = before
	}
}

func TestAssetCanvasHTTPRejectsMalformedDateAndCursorBeforeQuery(t *testing.T) {
	store := &assetQueryRecorder{}
	router := gin.New()
	canvashttp.NewAssetHandler(application.NewAssetQuery(store)).Register(router.Group("/api"))
	for _, query := range []string{"collection=archived", "favorite_only=banana", "created_from=2026-10-28", "cursor=not-base64", "limit=0", "kind=video", "created_from=2026-10-28T00:00:00Z&created_before=2026-10-28T00:00:00Z"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "GET", "/api/canvases?"+query, nil))
		if response.Code != 422 || store.calls != 0 {
			t.Fatal(query, response.Code, store.calls)
		}
	}
}
