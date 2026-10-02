package media_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediahttp "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/http"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func personalUploadRouter(db *gorm.DB, actor identityapp.Principal, objects *uploadObjectsFake) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	store := pgmedia.NewStore(db)
	service := mediaapp.NewScopedUploadService(store, store, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	mediahttp.NewUploadHandler(service).Register(group)
	return router
}
func TestPersonalUploadHTTPCurrentActorStrictBodyAndFreshRouterReplay(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, _ := mediaStoreProject(t, db)
	objects := &uploadObjectsFake{}
	router := personalUploadRouter(db, actor, objects)
	key := uuid.New()
	request := uploadHTTPRequest(t, []string{"file", "local_review_confirmed=true"})
	request.URL.Path = "/api/media/library/uploads"
	request.Header.Set("Idempotency-Key", key.String())
	first := httptest.NewRecorder()
	router.ServeHTTP(first, request)
	var result mediaapp.PersonalUploadResult
	if first.Code != 201 || json.Unmarshal(first.Body.Bytes(), &result) != nil || result.Asset.ID == uuid.Nil || bytes.Contains(first.Body.Bytes(), []byte("project_id")) {
		t.Fatal("personal HTTP failed", first.Code, first.Body.String())
	}
	request = uploadHTTPRequest(t, []string{"file", "local_review_confirmed=true"})
	request.URL.Path = "/api/media/library/uploads"
	request.Header.Set("Idempotency-Key", key.String())
	replay := httptest.NewRecorder()
	personalUploadRouter(db, actor, objects).ServeHTTP(replay, request)
	if replay.Code != 201 || !bytes.Equal(replay.Body.Bytes(), first.Body.Bytes()) || len(objects.items) != 3 {
		t.Fatal("fresh router same key created different response", replay.Code, replay.Body.String())
	}
	for _, field := range []string{"personal_actor_id=" + uuid.NewString(), "project_id=" + uuid.NewString(), "portraitCertified=true"} {
		request = uploadHTTPRequest(t, []string{"file", field, "local_review_confirmed=true"})
		request.URL.Path = "/api/media/library/uploads"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != 422 {
			t.Fatal("personal ownership/review override accepted", field, response.Code)
		}
	}
	owner := libraryOwnerDB(t)
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := owner.Exec(`UPDATE identity."user" SET status='active' WHERE id=?`, actor.ID).Error; err != nil {
			t.Error(err)
		}
	}()
	body := &unreadUploadBody{}
	request = httptest.NewRequestWithContext(t.Context(), "POST", "/api/media/library/uploads", body)
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Idempotency-Key", key.String())
	request.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, request)
	if denied.Code != 403 || body.reads.Load() != 0 {
		t.Fatal("revoked actor receipt replay read body", denied.Code, body.reads.Load())
	}
}
