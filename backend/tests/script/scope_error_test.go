package script_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	http "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type scopeFailureStore struct {
	application.HistoryStore
	failure error
}

func (s scopeFailureStore) Confirmations(_ context.Context, _ identity.Principal, _ uuid.UUID, _ uuid.UUID, _ int64, _ int) (application.ConfirmationPage, error) {
	return application.ConfirmationPage{}, s.failure
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
			http.NewVersionHandler(application.NewHistoryService(scopeFailureStore{failure: failure}, nil), nil).Register(router.Group("/api"))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), "GET", "/api/projects/"+uuid.NewString()+"/script-versions/"+uuid.NewString()+"/split-confirmations", nil))
			if w.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, test.status, w.Body.String())
			}
		})
	}
}
