package media_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediahttp "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/http"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func libraryRouter(db *gorm.DB, actor identityapp.Principal) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://127.0.0.1:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	mediahttp.NewLibraryHandler(pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)).Register(group)
	return router
}

func libraryHTTPRequest(t *testing.T, router *gin.Engine, method, path string, body []byte, key uuid.UUID) *httptest.ResponseRecorder {
	request := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(body))
	request.Header.Set("Origin", "http://127.0.0.1:3000")
	request.Header.Set("Content-Type", "application/json")
	if key != uuid.Nil {
		request.Header.Set("Idempotency-Key", key.String())
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestLibraryHTTPRealScopedPersistenceStrictInputAndReplay(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, _ := mediaStoreProject(t, db)
	router := libraryRouter(db, actor)
	for _, body := range []string{`{"scope":{"kind":"personal","personal_actor_id":"wrong"},"action":"create_text"}`, `{"scope":{"kind":"personal"},"action":"create_text","operation_id":"wrong"}`} {
		if response := libraryHTTPRequest(t, router, http.MethodPost, "/api/media/library/commands", []byte(body), uuid.New()); response.Code != 422 {
			t.Fatal("unknown private/execution field accepted", response.Code, response.Body.String())
		}
	}
	text := "HTTP保存的原文字"
	command := mediaapp.LibraryCommand{Scope: domain.LibraryScope{Kind: domain.LibraryPersonal}, Action: "create_text", Metadata: &mediaapp.LibraryMetadata{PlainText: &text, Title: "HTTP文字", Category: "other", Tags: []string{"测试"}}}
	body, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	created := libraryHTTPRequest(t, router, http.MethodPost, "/api/media/library/commands", body, key)
	if created.Code != 200 {
		t.Fatal("actual nonowner HTTP command", created.Code, created.Body.String())
	}
	var receipt mediaapp.LibraryReceipt
	if json.Unmarshal(created.Body.Bytes(), &receipt) != nil || len(receipt.Items) != 1 || receipt.Revision != 1 {
		t.Fatal("invalid safe receipt", created.Body.String())
	}
	reloaded := libraryRouter(db, actor)
	replay := libraryHTTPRequest(t, reloaded, http.MethodPost, "/api/media/library/commands", body, key)
	if replay.Code != 200 || !bytes.Equal(created.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatal("fresh handler broke permanent exact result", replay.Code, replay.Body.String())
	}
	page := libraryHTTPRequest(t, reloaded, http.MethodGet, "/api/media/library?scope=personal&page=1&page_size=40", nil, uuid.Nil)
	var result mediaapp.LibraryPage
	if page.Code != 200 || json.Unmarshal(page.Body.Bytes(), &result) != nil || result.Total != 1 || result.CurrentActorID != actor.ID {
		t.Fatal("safe scoped HTTP list", page.Code, page.Body.String())
	}
	detail := libraryHTTPRequest(t, reloaded, http.MethodGet, "/api/media/library/items/"+receipt.Items[0].ID.String()+"?scope=personal", nil, uuid.Nil)
	var actual mediaapp.LibraryItemDetail
	if detail.Code != 200 || json.Unmarshal(detail.Body.Bytes(), &actual) != nil || actual.PlainText == nil || *actual.PlainText != text {
		t.Fatal("formal text detail", detail.Code, detail.Body.String())
	}
	foreign, _ := mediaStoreProject(t, db)
	if response := libraryHTTPRequest(t, libraryRouter(db, foreign), http.MethodGet, "/api/media/library/items/"+actual.ID.String()+"?scope=personal", nil, uuid.Nil); response.Code != 404 {
		t.Fatal("foreign personal text exposed", response.Code, response.Body.String())
	}
	if response := libraryHTTPRequest(t, reloaded, http.MethodGet, "/api/media/library?scope=personal&actor_id="+foreign.ID.String(), nil, uuid.Nil); response.Code != 422 {
		t.Fatal("caller selected private owner", response.Code)
	}
	if response := libraryHTTPRequest(t, reloaded, http.MethodPost, "/api/media/library/commands", body, uuid.New()); response.Code != 409 {
		t.Fatal("stale library CAS accepted", response.Code, response.Body.String())
	}
}
