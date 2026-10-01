package canvas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	canvashttp "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/http"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

const directorOutputOrigin = "http://127.0.0.1:3140"

func directorOutputRouter(database *gorm.DB, actor identityapp.Principal) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware(directorOutputOrigin))
	api := router.Group("/api")
	api.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	canvashttp.NewHandler(application.NewService(canvasStore(database))).Register(api)
	return router
}

func directorOutputRequest(ctx context.Context, t *testing.T, router http.Handler, method, path, key string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		request.Header.Set("Origin", directorOutputOrigin)
		request.Header.Set("Idempotency-Key", key)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code >= 400 && (!strings.HasPrefix(response.Header().Get("Content-Type"), "application/problem+json") || response.Header().Get("X-Request-Id") == "") {
		t.Fatalf("invalid problem response headers: %v", response.Header())
	}
	return response
}

func TestDirectorOutputHTTPClosedJSONAndPersistentRefresh(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	router := directorOutputRouter(database, actor)
	create := directorOutputRequest(t.Context(), t, router, "POST", "/api/projects/"+project.String()+"/canvases", uuid.NewString(), []byte(`{"name":"HTTP图库","scope":{}}`))
	var doc domain.Document
	if create.Code != 201 || json.Unmarshal(create.Body.Bytes(), &doc) != nil {
		t.Fatalf("create canvas: %d %s", create.Code, create.Body.String())
	}
	node := directorNode()
	asset := seedCanvasMedia(t, database, project, "image")
	screenshot := directorScreenshot()
	screenshot.AssetID = asset
	node.Config.Director.Shots[0].Screenshots = []domain.DirectorScreenshot{screenshot}
	node.Config.Director.Cover = &domain.DirectorCover{AssetID: asset, ShotID: node.Config.Director.Shots[0].ID}
	body, err := json.Marshal(application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/canvases/" + doc.ID.String()
	key := uuid.NewString()
	saved := directorOutputRequest(t.Context(), t, router, "POST", path+"/commands", key, body)
	if saved.Code != 200 || !strings.Contains(saved.Body.String(), `"asset_id":"`+asset.String()) || !strings.Contains(saved.Body.String(), `"created_at":"2026-10-01T10:00:00Z"`) {
		t.Fatalf("save screenshot: %d %s", saved.Code, saved.Body.String())
	}
	freshRouter := directorOutputRouter(database, actor)
	replay := directorOutputRequest(t.Context(), t, freshRouter, "POST", path+"/commands", key, body)
	if replay.Code != 200 || !bytes.Equal(saved.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatalf("durable HTTP replay changed: %d %s", replay.Code, replay.Body.String())
	}
	read := directorOutputRequest(t.Context(), t, freshRouter, "GET", path, "", nil)
	if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &doc) != nil || doc.Revision != 2 || doc.Nodes[0].Config.Director.Shots[0].Screenshots[0].AssetID != asset || doc.Nodes[0].Config.Director.Cover.AssetID != asset {
		t.Fatalf("refresh lost typed outputs: %d %s", read.Code, read.Body.String())
	}
	validUpdate, err := json.Marshal(application.CommandsInput{ExpectedRevision: 2, Commands: []domain.Command{{Type: "UpdateNodeConfig", ID: node.ID, Config: &node.Config}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, before, after string }{
		{"gallery URL", `"screenshots":[{`, `"screenshots":[{"url":"https://example.invalid/secret",`},
		{"gallery object key", `"screenshots":[{`, `"screenshots":[{"object_key":"private/key",`},
		{"gallery execution", `"screenshots":[{`, `"screenshots":[{"operation_id":"` + uuid.NewString() + `",`},
		{"cover URL", `"cover":{`, `"cover":{"url":"https://example.invalid/secret",`},
		{"cover object key", `"cover":{`, `"cover":{"storage_key":"private/key",`},
		{"cover execution", `"cover":{`, `"cover":{"task_id":"` + uuid.NewString() + `",`},
		{"invalid capture time", `2026-10-01T10:00:00Z`, `yesterday`},
		{"invalid timezone hour", `2026-10-01T10:00:00Z`, `2026-10-01T10:00:00+24:00`},
		{"invalid timezone minute", `2026-10-01T10:00:00Z`, `2026-10-01T10:00:00+00:60`},
		{"invalid capture fractional separator", `2026-10-01T10:00:00Z`, `2026-10-01T10:00:00,123Z`},
		{"NUL gallery name", `机位截图 1`, `图\u0000`},
		{"invalid raw UTF8 name", `机位截图 1`, string([]byte{0xff})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			malformed := strings.Replace(string(validUpdate), tc.before, tc.after, 1)
			if malformed == string(validUpdate) {
				t.Fatal("test fixture did not replace target")
			}
			response := directorOutputRequest(t.Context(), t, freshRouter, "POST", path+"/commands", uuid.NewString(), []byte(malformed))
			if response.Code != 422 {
				t.Fatalf("malformed output accepted: %d %s", response.Code, response.Body.String())
			}
		})
	}
	_, otherProject := seedCanvasActor(t, database)
	foreignAsset := seedCanvasMedia(t, database, otherProject, "image")
	for _, target := range []string{"gallery", "cover"} {
		t.Run("foreign "+target, func(t *testing.T) {
			config := *node.Config.Director
			config.Shots = append([]domain.DirectorShot(nil), config.Shots...)
			config.Shots[0].Screenshots = []domain.DirectorScreenshot{screenshot}
			config.Cover = &domain.DirectorCover{AssetID: asset, ShotID: config.Shots[0].ID}
			if target == "gallery" {
				config.Shots[0].Screenshots[0].AssetID = foreignAsset
			} else {
				config.Cover.AssetID = foreignAsset
			}
			body, err := json.Marshal(application.CommandsInput{ExpectedRevision: 2, Commands: []domain.Command{{Type: "UpdateNodeConfig", ID: node.ID, Config: &domain.NodeConfig{Director: &config}}}})
			if err != nil {
				t.Fatal(err)
			}
			response := directorOutputRequest(t.Context(), t, freshRouter, "POST", path+"/commands", uuid.NewString(), body)
			if response.Code != 404 || !strings.Contains(response.Body.String(), `"command_index":0`) {
				t.Fatalf("foreign output not hidden: %d %s", response.Code, response.Body.String())
			}
		})
	}
	after := directorOutputRequest(t.Context(), t, freshRouter, "GET", path, "", nil)
	if after.Code != 200 || !bytes.Equal(after.Body.Bytes(), read.Body.Bytes()) {
		t.Fatalf("rejected output changed persisted document: %d %s", after.Code, after.Body.String())
	}
}
