package bible_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	biblehttp "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/http"
	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func TestBibleHTTPLookExplicitEmptyScopesRemainReadableWithoutHistoryRewrite(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, project := bibleActorProject(t, owner)
	service := app.NewService(bibleStore(db), time.Now)
	first, err := service.Change(t.Context(), actor, createCharacter(project))
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	biblehttp.NewHandler(service).Register(group)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://localhost:3000")
		request.Header.Set("Idempotency-Key", uuid.NewString())
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	base := "/api/projects/" + project.String() + "/bible/character/" + first.EntryID.String()
	created := call("POST", base+"/looks", `{"expected_revision":1,"look":{"name":"雨天造型😀","description":"  原样正文  ","default":false,"applies_to":[]}}`)
	var receipt app.Receipt
	if created.Code != 200 || json.Unmarshal(created.Body.Bytes(), &receipt) != nil {
		t.Fatal("actual look command", created.Code, created.Body.String())
	}
	var before struct{ Content, Scopes, SHA string }
	if err := owner.Raw(`SELECT v.content::text AS content,v.content_sha256 AS sha,l.applies_to::text AS scopes FROM bible.character_version v JOIN bible.look_version l ON l.character_version_id=v.id WHERE v.id=? AND l.position=1`, receipt.VersionID).Scan(&before).Error; err != nil || before.Scopes != "[]" {
		t.Fatal("explicit empty materialized history", before.Scopes, err)
	}
	for _, path := range []string{base, base + "/versions/" + receipt.VersionID.String(), "/api/projects/" + project.String() + "/bible/character", base + "/versions/" + first.VersionID.String()} {
		response := call("GET", path, "")
		if response.Code != 200 {
			t.Fatalf("saved appearance must stay readable: %s: %d %s", path, response.Code, response.Body.String())
		}
	}
	var after struct{ Content, Scopes, SHA string }
	if err := owner.Raw(`SELECT v.content::text AS content,v.content_sha256 AS sha,l.applies_to::text AS scopes FROM bible.character_version v JOIN bible.look_version l ON l.character_version_id=v.id WHERE v.id=? AND l.position=1`, receipt.VersionID).Scan(&after).Error; err != nil || after != before {
		t.Fatal("reader rewrote immutable history", err)
	}
}
