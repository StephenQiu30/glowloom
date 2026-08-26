package project_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	projecthttp "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

func TestCreateProjectPreservesPublicContract(t *testing.T) {
	service := &stubService{}
	handler := projecthttp.New(service, stubAuthenticator{})
	mux := http.NewServeMux()
	handler.Register(mux)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"workspace_id":"workspace-1","name":"Harbor","idempotency_key":"create-project-1"}`))
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"name":"Harbor"`) || strings.Contains(response.Body.String(), `"budget_limit"`) || strings.Contains(response.Body.String(), `"currency"`) {
		t.Fatalf("body = %s", response.Body.String())
	}
	if service.created.WorkspaceID != "workspace-1" || service.actor.UserID != "user-1" {
		t.Fatalf("command=%#v actor=%#v", service.created, service.actor)
	}
}
func TestCreateProjectRejectsUnknownFieldsBeforeApplication(t *testing.T) {
	service := &stubService{}
	handler := projecthttp.New(service, stubAuthenticator{})
	mux := http.NewServeMux()
	handler.Register(mux)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"workspace_id":"workspace-1","name":"Harbor","idempotency_key":"create-project-1","owner":"browser"}`))
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"validation_failed"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.created.Name != "" {
		t.Fatal("invalid request reached application")
	}
}
func TestProjectRouteRejectsMissingBearerToken(t *testing.T) {
	handler := projecthttp.New(&stubService{}, stubAuthenticator{err: authentication.ErrUnauthenticated})
	mux := http.NewServeMux()
	handler.Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/projects?workspace_id=workspace-1", nil))
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"unauthenticated"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestProjectHandlerDoesNotExposeLegacyBudgetRoute(t *testing.T) {
	handler := projecthttp.New(&stubService{}, stubAuthenticator{})
	mux := http.NewServeMux()
	handler.Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/budget-limit", strings.NewReader(`{}`)))
	if response.Code != http.StatusNotFound {
		t.Fatalf("legacy budget route status = %d body=%s", response.Code, response.Body.String())
	}
}

type stubAuthenticator struct{ err error }

func (auth stubAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	if auth.err != nil {
		return authentication.Claims{}, auth.err
	}
	return authentication.Claims{UserID: "user-1", TokenVersion: 1}, nil
}

type stubService struct {
	actor   application.Actor
	created application.CreateCommand
}

func (service *stubService) Create(_ context.Context, actor application.Actor, command application.CreateCommand) (domain.Project, error) {
	service.actor = actor
	service.created = command
	return domain.Project{ID: "project-1", WorkspaceID: command.WorkspaceID, Name: command.Name, AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 90000, Status: domain.StatusActive, Revision: 1}, nil
}
func (*stubService) List(context.Context, application.Actor, application.ListQuery) ([]domain.Project, int, error) {
	return nil, 0, errors.New("not implemented")
}
func (*stubService) Get(context.Context, application.Actor, string) (domain.Project, error) {
	return domain.Project{}, errors.New("not implemented")
}
func (*stubService) Update(context.Context, application.Actor, string, application.UpdateCommand) (domain.Project, error) {
	return domain.Project{}, errors.New("not implemented")
}
func (*stubService) SetArchived(context.Context, application.Actor, string, application.StateCommand, bool) (domain.Project, error) {
	return domain.Project{}, errors.New("not implemented")
}
func (*stubService) DeletePreflight(context.Context, application.Actor, string) (application.DeletePreflight, error) {
	return application.DeletePreflight{}, errors.New("not implemented")
}
func (*stubService) Delete(context.Context, application.Actor, string, int, string) error {
	return errors.New("not implemented")
}
