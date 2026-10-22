package canvas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	canvasfixture "github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	canvashttp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
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

func TestDirectorOutputOperationsClosedJSONAndHTTPRefresh(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	router := directorOutputRouter(database, actor)
	create := directorOutputRequest(t.Context(), t, router, "POST", "/api/projects/"+project.String()+"/canvases", uuid.NewString(), []byte(`{"name":"HTTP图库","scope":{}}`))
	var doc application.DocumentDTO
	if create.Code != 201 || json.Unmarshal(create.Body.Bytes(), &doc) != nil {
		t.Fatalf("create canvas: %d %s", create.Code, create.Body.String())
	}
	node := directorNode()
	asset := seedCanvasMedia(t, database, project, "image")
	screenshot := directorScreenshot()
	screenshot.AssetID = asset
	node.Config.Director.Shots[0].Screenshots = []domain.DirectorScreenshot{screenshot}
	node.Config.Director.Cover = &domain.DirectorCover{AssetID: asset, ShotID: node.Config.Director.Shots[0].ID}
	input := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &node}}}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/canvases/" + doc.ID.String()
	key := uuid.NewString()
	svc := application.NewCollaborationService(canvasStore(database))
	saved, err := svc.Submit(t.Context(), actor, doc.ID, key, input)
	if err != nil || saved.Status != "accepted" {
		t.Fatal("save screenshot", saved, err)
	}
	freshRouter := directorOutputRouter(database, actor)
	replay, err := application.NewCollaborationService(canvasStore(database)).Resolve(t.Context(), actor, doc.ID, key, input)
	if err != nil || replay == nil || *replay != saved {
		t.Fatal("durable operation replay changed", replay, err)
	}
	read := directorOutputRequest(t.Context(), t, freshRouter, "GET", path, "", nil)
	if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &doc) != nil || doc.Revision != 2 || doc.Nodes[0].Config.Director.Shots[0].Screenshots[0].AssetID != asset || doc.Nodes[0].Config.Director.Cover.AssetID != asset {
		t.Fatalf("refresh lost typed outputs: %d %s", read.Code, read.Body.String())
	}
	validCreate := body
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			malformed := strings.Replace(string(validCreate), tc.before, tc.after, 1)
			if malformed == string(validCreate) {
				t.Fatal("test fixture did not replace target")
			}
			malformed = strings.Replace(malformed, node.ID.String(), uuid.NewString(), 1)
			var request application.CollaborationInput
			if err := json.Unmarshal([]byte(malformed), &request); err != nil {
				return
			} // malformed typed JSON cannot reach submission
			request.BaseRevision = 2
			response, err := svc.Submit(t.Context(), actor, doc.ID, uuid.NewString(), request)
			if err != nil || response.Status != "rejected" || response.Code != "invalid_operation" {
				t.Fatalf("malformed output accepted: %+v %v", response, err)
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
			foreignNode := node
			foreignNode.ID = uuid.New()
			foreignNode.Config = domain.NodeConfig{Director: &config}
			request := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{{Type: "create_node", Node: &foreignNode}}}
			response, err := svc.Submit(t.Context(), actor, doc.ID, uuid.NewString(), request)
			if err != nil || response.Status != "rejected" || response.Code != "source_unavailable" {
				t.Fatalf("foreign output not hidden: %+v %v", response, err)
			}
		})
	}
	after := directorOutputRequest(t.Context(), t, freshRouter, "GET", path, "", nil)
	if after.Code != 200 || !bytes.Equal(after.Body.Bytes(), read.Body.Bytes()) {
		t.Fatalf("rejected output changed persisted document: %d %s", after.Code, after.Body.String())
	}
}
