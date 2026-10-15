package workspace_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspacehttp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/http"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

type headerScopeResolver struct {
	scope   domain.Scope
	writes  []bool
	failure error
}

func (s *headerScopeResolver) Resolve(_ context.Context, _ identity.Principal, id uuid.UUID, write bool) (domain.Scope, error) {
	s.writes = append(s.writes, write)
	if s.failure != nil {
		return domain.Scope{}, s.failure
	}
	if id != s.scope.ID {
		return domain.Scope{}, workspace.ErrScopeNotFound
	}
	return s.scope, nil
}
func TestCreationScopeHeaderIsExplicitAndReplacesPhysicalScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &headerScopeResolver{scope: domain.Scope{ID: uuid.New(), PhysicalScopeID: uuid.New()}}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("principal", identity.Principal{ID: uuid.New(), OrgID: uuid.New()}) }, workspacehttp.RequireCreationScope(s))
	router.Any("/content", func(c *gin.Context) {
		a := identityhttp.Principal(c)
		if a.OrgID != s.scope.PhysicalScopeID || a.ScopeID != s.scope.ID {
			t.Fatal("scope mapping not applied")
		}
		c.Status(204)
	})
	for _, test := range []struct {
		method, header string
		want           int
	}{{"GET", "", 422}, {"GET", "bad", 422}, {"GET", uuid.NewString(), 404}, {"GET", s.scope.ID.String(), 204}, {"POST", s.scope.ID.String(), 204}} {
		req := httptest.NewRequestWithContext(t.Context(), test.method, "/content", nil)
		req.Header.Set("X-Creation-Scope", test.header)
		req.Header.Set("X-Org-ID", uuid.NewString())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != test.want {
			t.Errorf("%s/%s status=%d want=%d", test.method, test.header, w.Code, test.want)
		}
	}
	if len(s.writes) != 3 || s.writes[1] || !s.writes[2] {
		t.Fatalf("read/write distinction=%v", s.writes)
	}
}

func TestCreationScopeAuthorityErrorsNeverEnterContentHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name   string
		cause  error
		status int
	}{{"revoked session", identity.ErrSessionInvalid, 401}, {"removed membership", workspace.ErrScopeNotFound, 404}, {"unavailable authority", errors.New("scope store unavailable"), 503}} {
		t.Run(test.name, func(t *testing.T) {
			scopes := &headerScopeResolver{scope: domain.Scope{ID: uuid.New()}, failure: test.cause}
			entered := false
			r := gin.New()
			r.Use(workspacehttp.RequireCreationScope(scopes))
			r.POST("/content", func(c *gin.Context) { entered = true; c.Status(201) })
			req := httptest.NewRequestWithContext(t.Context(), "POST", "/content", nil)
			req.Header.Set("X-Creation-Scope", scopes.scope.ID.String())
			response := httptest.NewRecorder()
			r.ServeHTTP(response, req)
			if response.Code != test.status || entered {
				t.Fatalf("authorization status=%d want=%d handler_entered=%v", response.Code, test.status, entered)
			}
		})
	}
}
