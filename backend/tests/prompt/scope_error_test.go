package prompt_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	http "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
)

type scopeFailureStore struct {
	application.DraftStore
	failure error
}

func (s scopeFailureStore) ListDrafts(_ context.Context, _ application.Actor, _ uuid.UUID, _ string) (application.DraftPage, error) {
	return application.DraftPage{}, s.failure
}
func TestScopeAuthorizationHTTPErrorChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name   string
		cause  error
		status int
	}{{"removed membership", workspacesource.ErrScopeNotFound, 404}, {"revoked exact session", application.ErrSessionInvalid, 401}, {"authority unavailable", errors.New("authority unavailable"), 503}} {
		t.Run(test.name, func(t *testing.T) {
			failure := errors.Join(errors.New("consumer failure"), test.cause)
			router := gin.New()
			http.NewDraftHandler(scopeFailureStore{failure: failure}, func(*gin.Context) (application.Actor, error) { return compilerActor(), nil }).Register(router.Group("/api"))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), "GET", "/api/projects/"+uuid.NewString()+"/prompt-drafts", nil))
			if w.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, test.status, w.Body.String())
			}
		})
	}
}
