package canvas_test

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	canvashttp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/http"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
)

func TestMainCanvasProjectsCurrentNameAndRejectsDirectMutation(t *testing.T) {
	db := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	service := application.NewService(pgcanvas.NewScopedStore(db, nil, app.NewCanvasProjectAccess))
	created, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "原名称"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE workspace.project SET initial_canvas_id=?,name='当前项目名',revision=2 WHERE id=?`, created.ID, project).Error; err != nil {
		t.Fatal(err)
	}
	doc, err := service.Get(t.Context(), actor, created.ID)
	if err != nil || doc.Name != "当前项目名" || doc.Revision != 1 {
		t.Fatalf("main name projection: %+v %v", doc, err)
	}
	if _, err := service.Rename(t.Context(), actor, doc.ID, uuid.NewString(), application.RenameInput{Name: "旁路改名", ExpectedRevision: 1}); err == nil {
		t.Fatal("direct main rename accepted")
	}
	if _, err := service.Delete(t.Context(), actor, doc.ID, uuid.NewString(), application.DeleteInput{ExpectedRevision: 1}); err == nil {
		t.Fatal("direct main delete accepted")
	}
	router := gin.New()
	api := router.Group("/api")
	api.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	canvashttp.NewHandler(service).Register(api)
	for _, method := range []string{"PATCH", "DELETE"} {
		body := `{"name":"非法旁路","expected_revision":1}`
		if method == "DELETE" {
			body = `{"expected_revision":1}`
		}
		request := httptest.NewRequestWithContext(t.Context(), method, "/api/canvases/"+doc.ID.String(), strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", uuid.NewString())
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != 409 {
			t.Fatalf("main %s HTTP status=%d %s", method, response.Code, response.Body.String())
		}
	}
}

func TestMainCanvasGuardPreservesValidOldReceiptBeforeRejectingExpiredKey(t *testing.T) {
	db := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	service := application.NewService(pgcanvas.NewScopedStore(db, nil, app.NewCanvasProjectAccess))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "历史名称"})
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()
	input := application.RenameInput{Name: "历史改名", ExpectedRevision: 1}
	prior, err := service.Rename(t.Context(), actor, doc.ID, key, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE workspace.project SET initial_canvas_id=?,name='当前统一名' WHERE id=?`, doc.ID, project).Error; err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Rename(t.Context(), actor, doc.ID, key, input)
	if err != nil || replayed.Revision != prior.Revision || replayed.Name != prior.Name {
		t.Fatalf("valid receipt changed: %+v %v", replayed, err)
	}
	if err := db.Exec(`UPDATE infra.idempotency_record SET expires_at=now()-interval '1 second' WHERE actor_id=? AND idem_key=?`, actor.ID, key).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Rename(t.Context(), actor, doc.ID, key, input); !errors.Is(err, application.ErrMainCanvasManagedByProject) {
		t.Fatalf("expired receipt bypassed guard: %v", err)
	}
	current, err := service.Get(t.Context(), actor, doc.ID)
	if err != nil || current.Name != "当前统一名" || current.Revision != prior.Revision {
		t.Fatalf("main identity or version changed: %+v %v", current, err)
	}
}

func TestMainCanvasDeleteGuardPreservesOldReceiptThenRejectsExpiredKey(t *testing.T) {
	db := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	service := application.NewService(canvasStore(db))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "历史文档"})
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()
	input := application.DeleteInput{ExpectedRevision: 1}
	prior, err := service.Delete(t.Context(), actor, doc.ID, key, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE workspace.project SET initial_canvas_id=? WHERE id=?`, doc.ID, project).Error; err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Delete(t.Context(), actor, doc.ID, key, input)
	if err != nil || replayed != prior {
		t.Fatalf("valid delete receipt changed: %+v %v", replayed, err)
	}
	if err := db.Exec(`UPDATE infra.idempotency_record SET expires_at=now()-interval '1 second' WHERE actor_id=? AND idem_key=?`, actor.ID, key).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Delete(t.Context(), actor, doc.ID, key, input); !errors.Is(err, application.ErrMainCanvasRequired) {
		t.Fatalf("expired delete bypass: %v", err)
	}
}
