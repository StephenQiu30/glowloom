package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	mediahttp "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/http"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func uploadHTTPRouter(repo *uploadRepoFake, prober mediaapp.Prober) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) {
		c.Set("principal", identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer})
		c.Next()
	})
	service := mediaapp.NewUploadService(repo, prober, mediaflow.FFUploadRenderer{}, &uploadObjectsFake{}, time.Now)
	mediahttp.NewUploadHandler(service).Register(group)
	group.POST("/json", func(c *gin.Context) {
		var input struct {
			Name string `json:"name"`
		}
		if httpapi.Decode(c, &input) {
			c.Status(201)
		}
	})
	return router
}

func uploadHTTPRequest(t *testing.T, fields []string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, field := range fields {
		if field == "file" {
			part, err := writer.CreateFormFile("file", "参考.png")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(uploadPNG(t)); err != nil {
				t.Fatal(err)
			}
		} else {
			name, value, _ := strings.Cut(field, "=")
			if err := writer.WriteField(name, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/projects/"+uuid.NewString()+"/media/uploads", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	return req
}

func TestUploadHTTPMultipartIsStrictAndPreservesJSONLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields []string
		status int
	}{
		{"review before file", []string{"local_review_confirmed=true", "file"}, 201},
		{"file before review", []string{"file", "local_review_confirmed=true"}, 201},
		{"review absent", []string{"file"}, 422},
		{"review false", []string{"file", "local_review_confirmed=false"}, 422},
		{"duplicate review", []string{"file", "local_review_confirmed=true", "local_review_confirmed=true"}, 422},
		{"duplicate file", []string{"file", "file", "local_review_confirmed=true"}, 422},
		{"unknown field", []string{"file", "unknown=true"}, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &uploadRepoFake{}
			router := uploadHTTPRouter(repo, mediaflow.FFUploadProber{})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, uploadHTTPRequest(t, tc.fields))
			if response.Code != tc.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if tc.status == 201 {
				var result mediaapp.UploadResult
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Asset.ID == uuid.Nil || bytes.Contains(response.Body.Bytes(), []byte("object_key")) {
					t.Fatalf("unsafe result=%s err=%v", response.Body.String(), err)
				}
			} else if repo.commits != 0 {
				t.Fatalf("invalid multipart committed %d assets", repo.commits)
			}
		})
	}
	router := uploadHTTPRouter(&uploadRepoFake{}, mediaflow.FFUploadProber{})
	req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/json", strings.NewReader(`{"name":"`+strings.Repeat("a", 1<<20)+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != 413 {
		t.Fatalf("JSON body cap changed: status=%d", response.Code)
	}
}

type unreadUploadBody struct{ reads atomic.Int32 }

func (r *unreadUploadBody) Read([]byte) (int, error) { r.reads.Add(1); return 0, io.EOF }
func TestUploadHTTPAuthorizesAndChecksOriginBeforeReadingBody(t *testing.T) {
	for _, tc := range []struct {
		name, origin string
		authErr      error
		status       int
	}{
		{"foreign origin", "http://other.local", nil, 403},
		{"inaccessible project", "http://localhost:3000", mediaapp.ErrNotFound, 404},
		{"revoked actor", "http://localhost:3000", identityapp.ErrForbidden, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &unreadUploadBody{}
			router := uploadHTTPRouter(&uploadRepoFake{authorizeErr: tc.authErr}, mediaflow.FFUploadProber{})
			req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/projects/"+uuid.NewString()+"/media/uploads", body)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Idempotency-Key", uuid.NewString())
			req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status || body.reads.Load() != 0 {
				t.Fatalf("denied upload read body: status=%d reads=%d", response.Code, body.reads.Load())
			}
		})
	}
}

type uploadBlockingProber struct {
	entered chan struct{}
	release chan struct{}
}

func (p uploadBlockingProber) Probe(ctx context.Context, file *mediaapp.Downloaded) (mediaapp.ProbeResult, error) {
	p.entered <- struct{}{}
	select {
	case <-p.release:
		return (mediaflow.FFUploadProber{}).Probe(ctx, file)
	case <-ctx.Done():
		return mediaapp.ProbeResult{}, ctx.Err()
	}
}
func TestUploadHTTPConcurrencyGateRejectsThirdUnreadBody(t *testing.T) {
	prober := uploadBlockingProber{entered: make(chan struct{}, 2), release: make(chan struct{})}
	router := uploadHTTPRouter(&uploadRepoFake{}, prober)
	var wg sync.WaitGroup
	for range 2 {
		req := uploadHTTPRequest(t, []string{"file", "local_review_confirmed=true"})
		wg.Go(func() { router.ServeHTTP(httptest.NewRecorder(), req) })
	}
	for range 2 {
		select {
		case <-prober.entered:
		case <-time.After(5 * time.Second):
			close(prober.release)
			wg.Wait()
			t.Fatal("uploads did not reach blocking probe")
		}
	}
	body := &unreadUploadBody{}
	req := httptest.NewRequestWithContext(t.Context(), "POST", "/api/projects/"+uuid.NewString()+"/media/uploads", body)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	close(prober.release)
	wg.Wait()
	if response.Code != 429 || body.reads.Load() != 0 {
		t.Fatalf("busy upload: status=%d reads=%d", response.Code, body.reads.Load())
	}
}
