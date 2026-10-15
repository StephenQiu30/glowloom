package bible_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	http "github.com/StephenQiu30/glowloom-server/backend/internal/bible/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/bible/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type scopeFailureStore struct {
	application.Store
	failure error
}

func (s scopeFailureStore) List(_ context.Context, _ identity.Principal, _ application.ListInput) (application.Page, error) {
	return application.Page{}, s.failure
}
func TestScopeAuthorizationHTTPErrorChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name   string
		cause  error
		status int
	}{{"removed membership", workspace.ErrScopeNotFound, 404}, {"revoked exact session", identity.ErrSessionInvalid, 401}, {"authority unavailable", errors.New("authority unavailable"), 503}} {
		t.Run(test.name, func(t *testing.T) {
			failure := errors.Join(errors.New("consumer failure"), test.cause)
			router := gin.New()
			http.NewHandler(application.NewService(scopeFailureStore{failure: failure}, time.Now)).Register(router.Group("/api"))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), "GET", "/api/projects/"+uuid.NewString()+"/bible/character", nil))
			if w.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, test.status, w.Body.String())
			}
		})
	}
}
