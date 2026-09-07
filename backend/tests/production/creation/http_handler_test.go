package creation_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	api "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/httpapi"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	"github.com/google/uuid"
)

type creationAuth struct{}

func (creationAuth) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{UserID: "81000000-0000-4000-8000-000000000001", TokenVersion: 7}, nil
}

type creationHTTPService struct {
	api.Service
	command app.CreateCommand
	actor   app.Actor
	limit   int
	called  bool
}

func (s *creationHTTPService) Create(_ context.Context, a app.Actor, c app.CreateCommand) (domain.Run, error) {
	s.actor, s.command, s.called = a, c, true
	return domain.Run{Endpoint: "https://private.invalid", IdempotencyKey: "private-key", Command: domain.Command{RunID: uuid.NewString(), ProjectID: c.ProjectID}, Status: domain.Queued}, nil
}
func (s *creationHTTPService) List(_ context.Context, _ app.Actor, _ string, limit int) ([]domain.Run, error) {
	s.limit = limit
	return []domain.Run{}, nil
}
func TestCreationHTTPRejectsCallerSelectedRuntimeAndHidesInternalRoute(t *testing.T) {
	service := &creationHTTPService{}
	mux := http.NewServeMux()
	api.New(service, creationAuth{}).Register(mux)
	path := "/api/projects/" + uuid.NewString() + "/creation-runs"
	body := `{"document_revision_id":"` + uuid.NewString() + `","source_hash":"` + strings.Repeat("a", 64) + `","idempotency_key":"one"}`
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(strings.TrimSuffix(body, "}")+`,"endpoint":"https://other.invalid"}`)))
	if recorder.Code != 422 || service.called {
		t.Fatal("caller changed trusted route")
	}
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	if recorder.Code != 202 || !service.called || service.actor.TokenVersion != 7 || strings.Contains(recorder.Body.String(), "private") {
		t.Fatalf("response=%d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path+"?limit=3", nil))
	if recorder.Code != 200 || service.limit != 3 {
		t.Fatal("bounded list not wired")
	}
}
