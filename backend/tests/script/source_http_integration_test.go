package script_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	scripthttp "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/http"
	scriptobjects "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/objects"
	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func scriptStorage(t *testing.T) *objectstorage.Client {
	t.Helper()
	file := os.Getenv("LV_TEST_RECEIPT_STORAGE_CONFIG")
	if file == "" {
		t.Skip("set isolated LV_TEST_RECEIPT_STORAGE_CONFIG; no business configuration is read")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal("read synthetic object test configuration")
	}
	var config struct {
		Endpoint, Bucket, Region string
		AccessKey                string `json:"access_key"`
		SecretKey                string `json:"secret_key"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal("decode synthetic object test configuration")
	}
	client, err := objectstorage.Open(config.Endpoint, config.Bucket, config.AccessKey, config.SecretKey, config.Region)
	if err != nil {
		t.Fatal("create synthetic object client")
	}
	return client
}
func scriptRouter(service *app.SourceService, actor identityapp.Principal) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://127.0.0.1:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	scripthttp.NewSourceHandler(service).Register(group)
	return router
}
func scriptHTTP(t *testing.T, router http.Handler, method, path string, key uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://127.0.0.1:3000")
	request.Header.Set("Idempotency-Key", key.String())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestScriptSourceHTTPPrivateHTMLRichAndPermanentRecovery(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	client := scriptStorage(t)
	objects := scriptobjects.NewStorage(client)
	service := app.NewSourceService(scriptStore(db), objects, time.Now)
	router := scriptRouter(service, actor)
	path := "/api/projects/" + pid.String()
	key := uuid.New()
	revision := int64(0)
	html := "<h2 style=\"text-align: center\">章节😀</h2><p><strong>é</strong><br>中文</p><hr><p></p>"
	body := scripthttp.SourceWriteRequest{ExpectedRevision: &revision, RightsConfirmed: true, Kind: "chapter", Title: "第一章", Status: "draft", OriginalHTML: &html}
	response := scriptHTTP(t, router, "POST", path+"/script-sources", key, body)
	if response.Code != 200 {
		t.Fatal("real source HTTP", response.Code, response.Body.String())
	}
	var receipt app.SourceReceipt
	if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	base, err := scriptStore(db).LoadBase(t.Context(), actor, pid, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, fact := range []domain.ObjectFact{base.Sources[0].Original, base.Sources[0].Rich, base.Version.Text, base.Version.Rich} {
			if err := client.Remove(ctx, fact.Key); err != nil {
				t.Error("remove exact owned script test object")
			}
		}
	})
	original, err := client.Get(t.Context(), base.Sources[0].Original.Key)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(original)
	closeErr := original.Close()
	if err := errors.Join(readErr, closeErr); err != nil || string(data) != html {
		t.Fatal("original HTML bytes lost", err)
	}
	detail := scriptHTTP(t, router, "GET", path+"/script-sources/"+receipt.Mappings[0].LineageID.String(), uuid.Nil, nil)
	if detail.Code != 200 || strings.Contains(detail.Body.String(), "projects/") || strings.Contains(detail.Body.String(), "original_key") || strings.Contains(detail.Body.String(), "rich_key") {
		t.Fatal("selected private DTO", detail.Code, detail.Body.String())
	}
	var selected app.SourceDetail
	if err := json.Unmarshal(detail.Body.Bytes(), &selected); err != nil || selected.PlainText != "章节😀\n\né\n中文\n\n\n\n" {
		t.Fatal("canonical actual selected body", selected.PlainText, err)
	}
	replay := scriptHTTP(t, scriptRouter(app.NewSourceService(scriptStore(db), objects, time.Now), actor), "POST", path+"/script-sources", key, body)
	if replay.Code != 200 || replay.Body.String() != response.Body.String() {
		t.Fatal("durable identical original public response", replay.Code, replay.Body.String())
	}
	page := scriptHTTP(t, router, "GET", path+"/script-sources", uuid.Nil, nil)
	if page.Code != 200 || strings.Contains(page.Body.String(), "document") || strings.Contains(page.Body.String(), "plain_text") {
		t.Fatal("list eagerly exposes bodies", page.Body.String())
	}
	body.Document = &domain.RichDocument{Type: "doc"}
	invalid := scriptHTTP(t, router, "POST", path+"/script-sources", uuid.New(), body)
	if invalid.Code != 422 {
		t.Fatal("ambiguous original and rich accepted", invalid.Code)
	}
}

type lostPrivatePut struct {
	app.PrivateObjects
	lost bool
}

func (s *lostPrivatePut) PutIfAbsent(ctx context.Context, key string, r io.Reader, size int64, mime, hash string) error {
	err := s.PrivateObjects.PutIfAbsent(ctx, key, r, size, mime, hash)
	if err != nil {
		return err
	}
	if !s.lost {
		s.lost = true
		return errors.New("synthetic response lost after actual private put")
	}
	return nil
}

func TestScriptSourcePGActualUnknownPrivatePutFenceAndSameKeyRecovery(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	client := scriptStorage(t)
	objects := scriptobjects.NewStorage(client)
	loss := &lostPrivatePut{PrivateObjects: objects}
	_, input := sourceCommand()
	input.ProjectID = pid
	service := app.NewSourceService(scriptStore(db), loss, time.Now)
	if _, err := service.Write(t.Context(), actor, input); !errors.Is(err, app.ErrNeedsReconciliation) {
		t.Fatal("actual unknown put published", err)
	}
	if s, v, c := scriptCounts(t, owner, pid); s != 0 || v != 0 || c != 1 {
		t.Fatal("unknown transaction facts", s, v, c)
	}
	if blocked, err := scriptStore(db).HasInflightWork(t.Context(), actor, pid); err != nil || !blocked {
		t.Fatal("lost put lifecycle fence", blocked, err)
	}
	receipt, err := app.NewSourceService(scriptStore(db), objects, time.Now).Write(t.Context(), actor, input)
	if err != nil || receipt.ScriptRevision != 1 {
		t.Fatal("actual same-key recovery", receipt, err)
	}
	base, err := scriptStore(db).LoadBase(t.Context(), actor, pid, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, fact := range []domain.ObjectFact{base.Sources[0].Original, base.Sources[0].Rich, base.Version.Text, base.Version.Rich} {
			if err := client.Remove(ctx, fact.Key); err != nil {
				t.Error("remove exact owned recovery test object")
			}
		}
	})
	if blocked, err := scriptStore(db).HasInflightWork(t.Context(), actor, pid); err != nil || blocked {
		t.Fatal("completed original intent still fenced", blocked, err)
	}
	if s, v, c := scriptCounts(t, owner, pid); s != 1 || v != 1 || c != 1 {
		t.Fatal("unknown duplicate publication", s, v, c)
	}
}
