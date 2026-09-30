package identity_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

type workspaceStore struct {
	user domain.User
	err  error
}

func (s *workspaceStore) EnsureWorkspace(context.Context) (domain.User, error) { return s.user, s.err }

func TestWorkspaceDoesNotRequireOrCreateBrowserCredentials(t *testing.T) {
	store := &workspaceStore{user: domain.User{ID: uuid.New(), OrgID: uuid.New(), LoginName: "local-workspace", DisplayName: "本机工作区", Role: domain.RoleProducer, Status: domain.StatusActive}}
	router := gin.New()
	router.Use(httpapi.Middleware("http://127.0.0.1:3000"))
	api := router.Group("/api")
	api.Use(identityhttp.Workspace(store))
	api.GET("/projects", func(c *gin.Context) {
		if identityhttp.Principal(c).ID != store.user.ID {
			t.Error("workspace identity was not injected")
		}
		c.Status(200)
	})
	api.POST("/write", func(c *gin.Context) { c.Status(204) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/projects", nil))
	if response.Code != 200 || len(response.Result().Cookies()) != 0 {
		t.Fatalf("workspace status %d", response.Code)
	}
	for _, tc := range []struct {
		name, origin, key string
		want              int
	}{
		{"no cookies", "http://127.0.0.1:3000", uuid.NewString(), 204},
		{"foreign origin", "https://other.example", uuid.NewString(), 403},
		{"missing origin", "", uuid.NewString(), 403},
		{"missing idempotency key", "http://127.0.0.1:3000", "", 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/write", nil)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Idempotency-Key", tc.key)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.want {
				t.Fatalf("status %d, want %d", response.Code, tc.want)
			}
		})
	}
	store.err = application.ErrForbidden
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/projects", nil))
	if response.Code != 403 {
		t.Fatalf("disabled workspace status %d, want 403", response.Code)
	}
}
