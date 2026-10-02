package bible_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	biblehttp "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/http"
	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func TestBibleHTTPCurrentAuthorityClosedBodyAndPermanentRecovery(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, project := bibleActorProject(t, owner)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	biblehttp.NewHandler(app.NewService(bibleStore(db), time.Now)).Register(group)
	call := func(method, path, body string, key uuid.UUID) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://localhost:3000")
		request.Header.Set("Idempotency-Key", key.String())
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	path := "/api/projects/" + project.String() + "/bible/character"
	key := uuid.New()
	body := `{"expected_revision":0,"character":{"name":"王总","definition":{"appearance":"黑发"}}}`
	first := call("POST", path, body, key)
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	var receipt app.Receipt
	if err := json.Unmarshal(first.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	replay := call("POST", path, body, key)
	if replay.Code != 200 || replay.Body.String() != first.Body.String() {
		t.Fatal("original recovery", replay.Code, replay.Body.String())
	}
	for _, body := range []string{`{"expected_revision":0,"expected_revision":1,"character":{"name":"重复键","definition":{}}}`, `{"expected_revision":0,"character":{"name":"私有资料","definition":{},"provider_json":{}}}`, `{"expected_revision":0,"character":{"name":"伪AI","definition":{}},"origin":"ai"}`} {
		response := call("POST", path, body, uuid.New())
		if response.Code != 422 {
			t.Fatal("invalid body accepted", response.Code, response.Body.String())
		}
	}
	large := call("POST", path, `{"expected_revision":0,"character":{"name":"角色","description":"`+strings.Repeat("a", 1<<20)+`","definition":{}}}`, uuid.New())
	if large.Code != 413 {
		t.Fatal("global public body budget", large.Code, large.Body.String())
	}
	get := call("GET", path+"/"+receipt.EntryID.String(), "", uuid.New())
	if get.Code != 200 || strings.Contains(get.Body.String(), "object_key") {
		t.Fatal("current detail", get.Code, get.Body.String())
	}
	if err := owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	denied := call("POST", path, body, key)
	if denied.Code != 403 {
		t.Fatal("disabled replay", denied.Code, denied.Body.String())
	}
}

func TestBibleHTTPPaginationScopeAndHistoryPinnedRead(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, project := bibleActorProject(t, owner)
	service := app.NewService(bibleStore(db), time.Now)
	first, err := service.Change(t.Context(), actor, createCharacter(project))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Change(t.Context(), actor, createCharacter(project)); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	biblehttp.NewHandler(service).Register(group)
	get := func(path string) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), "GET", path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	base := "/api/projects/" + project.String() + "/bible/character"
	response := get(base + "?limit=1")
	var page biblehttp.PageResponse
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || page.CurrentActorID != actor.ID || page.CurrentOrgID != actor.OrgID || len(page.Entries) != 1 || page.NextCursor == "" {
		t.Fatal("scope pagination", response.Code, response.Body.String())
	}
	next := get(base + "?limit=1&cursor=" + page.NextCursor)
	if next.Code != 200 {
		t.Fatal(next.Code, next.Body.String())
	}
	var page2 biblehttp.PageResponse
	if json.Unmarshal(next.Body.Bytes(), &page2) != nil || len(page2.Entries) != 1 || page2.Entries[0].Head.ID == page.Entries[0].Head.ID || page2.NextCursor != "" {
		t.Fatal("stable keyset", next.Body.String())
	}
	pinned := get(base + "/" + first.EntryID.String() + "/versions/" + first.VersionID.String())
	if pinned.Code != 200 || !strings.Contains(pinned.Body.String(), first.ContentSHA256) {
		t.Fatal("pinned", pinned.Code, pinned.Body.String())
	}
	history := get(base + "/" + first.EntryID.String() + "/versions")
	if history.Code != 200 {
		t.Fatal("history", history.Code, history.Body.String())
	}
}
