package media_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	http "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type scopeFailureStore struct {
	application.LibraryRepository
	failure error
}

func (s scopeFailureStore) ListLibrary(_ context.Context, _ identity.Principal, _ domain.LibraryScope, _ application.LibraryQuery) (application.LibraryPage, error) {
	return application.LibraryPage{}, s.failure
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
			http.NewLibraryHandler(scopeFailureStore{failure: failure}).Register(router.Group("/api"))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), "GET", "/api/media/library", nil))
			if w.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, test.status, w.Body.String())
			}
		})
	}
}
