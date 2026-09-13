package preset_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	presethttp "github.com/StephenQiu30/lanverse/backend/internal/preset/adapter/httpapi"
	presetapp "github.com/StephenQiu30/lanverse/backend/internal/preset/application"
	presetcatalog "github.com/StephenQiu30/lanverse/backend/internal/preset/catalog"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

func TestProjectPresetHandlerListsExactCuratedReleases(t *testing.T) {
	handler := presethttp.New(presetcatalog.CuratedReleases, &presetSelectionStub{}, projectReaderStub{}, presetAuthenticatorStub{})
	mux := http.NewServeMux()
	handler.Register(mux)
	request := httptest.NewRequest(http.MethodGet, "/api/presets", nil)
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"key":"urban-cinematic-realism"`) ||
		!strings.Contains(response.Body.String(), `"release":"2026.09.13"`) || strings.Contains(response.Body.String(), `"release":"current"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestProjectPresetHandlerSelectsForAuthorizedProject(t *testing.T) {
	selection := validProjectSelection(t)
	service := &presetSelectionStub{selected: selection}
	handler := presethttp.New(presetcatalog.CuratedReleases, service, projectReaderStub{
		project: projectdomain.Project{ID: selection.ProjectID, WorkspaceID: selection.WorkspaceID, Status: projectdomain.StatusActive},
	}, presetAuthenticatorStub{})
	mux := http.NewServeMux()
	handler.Register(mux)
	request := httptest.NewRequest(http.MethodPut, "/api/projects/"+selection.ProjectID+"/preset-selection", strings.NewReader(
		`{"preset_key":"urban-cinematic-realism","preset_release":"2026.09.13","application_mode":"faithful","expected_revision":0,"idempotency_key":"select-preset-1"}`,
	))
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"content_hash":"`+selection.ContentHash+`"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.command.WorkspaceID != selection.WorkspaceID || service.command.ProjectID != selection.ProjectID ||
		service.command.SelectedBy != selection.SelectedBy || service.command.IdempotencyKey != "select-preset-1" {
		t.Fatalf("command = %#v", service.command)
	}
}

func TestProjectPresetHandlerReadsAuthorizedCurrentSelection(t *testing.T) {
	selection := validProjectSelection(t)
	service := &presetSelectionStub{current: selection}
	handler := presethttp.New(presetcatalog.CuratedReleases, service, projectReaderStub{
		project: projectdomain.Project{ID: selection.ProjectID, WorkspaceID: selection.WorkspaceID, Status: projectdomain.StatusActive},
	}, presetAuthenticatorStub{})
	mux := http.NewServeMux()
	handler.Register(mux)
	request := httptest.NewRequest(http.MethodGet, "/api/projects/"+selection.ProjectID+"/preset-selection", nil)
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"`+selection.ID+`"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.currentWorkspaceID != selection.WorkspaceID || service.currentProjectID != selection.ProjectID {
		t.Fatalf("current scope = %q/%q", service.currentWorkspaceID, service.currentProjectID)
	}
}

func TestProjectPresetHandlerRejectsUnknownSelectionFields(t *testing.T) {
	service := &presetSelectionStub{}
	handler := presethttp.New(presetcatalog.CuratedReleases, service, projectReaderStub{
		project: projectdomain.Project{ID: uuid.NewString(), WorkspaceID: uuid.NewString(), Status: projectdomain.StatusActive},
	}, presetAuthenticatorStub{})
	mux := http.NewServeMux()
	handler.Register(mux)
	request := httptest.NewRequest(http.MethodPut, "/api/projects/"+uuid.NewString()+"/preset-selection", strings.NewReader(
		`{"preset_key":"urban-cinematic-realism","preset_release":"2026.09.13","application_mode":"faithful","expected_revision":0,"idempotency_key":"select-preset-1","visual_style":"free"}`,
	))
	request.Header.Set("Authorization", "Bearer valid")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"validation_failed"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.command.ProjectID != "" {
		t.Fatal("invalid request reached selection service")
	}
}

func TestProjectPresetHandlerRequiresAuthentication(t *testing.T) {
	handler := presethttp.New(presetcatalog.CuratedReleases, &presetSelectionStub{}, projectReaderStub{}, presetAuthenticatorStub{err: authentication.ErrUnauthenticated})
	mux := http.NewServeMux()
	handler.Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/presets", nil))
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"unauthenticated"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type presetAuthenticatorStub struct{ err error }

func (authenticator presetAuthenticatorStub) Authenticate(*http.Request) (authentication.Claims, error) {
	if authenticator.err != nil {
		return authentication.Claims{}, authenticator.err
	}
	return authentication.Claims{UserID: "b64cf75b-50b0-4270-b394-b71805698e35", TokenVersion: 3}, nil
}

type projectReaderStub struct {
	project projectdomain.Project
	err     error
}

func (stub projectReaderStub) Get(context.Context, projectapp.Actor, string) (projectdomain.Project, error) {
	return stub.project, stub.err
}

type presetSelectionStub struct {
	command            presetapp.SelectProjectPresetCommand
	selected           presetdomain.ProjectSelection
	current            presetdomain.ProjectSelection
	currentWorkspaceID string
	currentProjectID   string
	err                error
}

func (stub *presetSelectionStub) Select(_ context.Context, command presetapp.SelectProjectPresetCommand) (presetdomain.ProjectSelection, error) {
	stub.command = command
	return stub.selected, stub.err
}

func (stub *presetSelectionStub) Current(_ context.Context, workspaceID string, projectID string) (presetdomain.ProjectSelection, error) {
	stub.currentWorkspaceID, stub.currentProjectID = workspaceID, projectID
	if stub.err != nil {
		return presetdomain.ProjectSelection{}, stub.err
	}
	return stub.current, nil
}

func validProjectSelection(t *testing.T) presetdomain.ProjectSelection {
	t.Helper()
	release, found, err := presetcatalog.FindCuratedRelease("urban-cinematic-realism", "2026.09.13")
	if err != nil || !found {
		t.Fatalf("curated release: found=%v err=%v", found, err)
	}
	selection, _, err := presetdomain.NewProjectSelection(uuid.NewString(), presetdomain.ProjectSelectionInput{
		WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), Revision: 1,
		PresetRelease:   presetdomain.ProjectSelectionRelease{Key: release.Key, Release: release.Release, ContentHash: release.ContentHash},
		ApplicationMode: "faithful", SelectedBy: "b64cf75b-50b0-4270-b394-b71805698e35",
		SelectedAt: time.Date(2026, time.September, 12, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return selection
}
