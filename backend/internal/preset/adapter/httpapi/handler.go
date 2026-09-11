package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
	presetapp "github.com/StephenQiu30/lanverse/backend/internal/preset/application"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

type Catalog func() ([]presetdomain.Release, error)

type SelectionService interface {
	Select(context.Context, presetapp.SelectProjectPresetCommand) (presetdomain.ProjectSelection, error)
	Current(context.Context, string, string) (presetdomain.ProjectSelection, error)
}

type ProjectReader interface {
	Get(context.Context, projectapp.Actor, string) (projectdomain.Project, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}

type Handler struct {
	catalog       Catalog
	selections    SelectionService
	projects      ProjectReader
	authenticator Authenticator
	validator     *platformvalidation.Validator
}

func New(catalog Catalog, selections SelectionService, projects ProjectReader, authenticator Authenticator) *Handler {
	return &Handler{
		catalog: catalog, selections: selections, projects: projects,
		authenticator: authenticator, validator: platformvalidation.New(),
	}
}

func (handler *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/presets", handler.list)
	mux.HandleFunc("GET /api/projects/{project_id}/preset-selection", handler.current)
	mux.HandleFunc("PUT /api/projects/{project_id}/preset-selection", handler.selectPreset)
}

type selectRequest struct {
	PresetKey        string `json:"preset_key" validate:"required,max=64"`
	PresetRelease    string `json:"preset_release" validate:"required,len=10"`
	ApplicationMode  string `json:"application_mode" validate:"required,oneof=faithful world_adaptation"`
	ExpectedRevision *int64 `json:"expected_revision" validate:"required,gte=0"`
	IdempotencyKey   string `json:"idempotency_key" validate:"required,max=200"`
}

func (handler *Handler) list(writer http.ResponseWriter, request *http.Request) {
	if _, ok := handler.actor(writer, request); !ok {
		return
	}
	releases, err := handler.catalog()
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{"items": releases}})
}

func (handler *Handler) current(writer http.ResponseWriter, request *http.Request) {
	claims, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	project, ok := handler.project(writer, request, claims)
	if !ok {
		return
	}
	selection, err := handler.selections.Current(request.Context(), project.WorkspaceID, project.ID)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": selection})
}

func (handler *Handler) selectPreset(writer http.ResponseWriter, request *http.Request) {
	claims, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	var payload selectRequest
	if !platformhttp.DecodeStrict(writer, request, handler.validator, &payload) {
		return
	}
	project, ok := handler.project(writer, request, claims)
	if !ok {
		return
	}
	selection, err := handler.selections.Select(request.Context(), presetapp.SelectProjectPresetCommand{
		WorkspaceID: project.WorkspaceID, ProjectID: project.ID, SelectedBy: claims.UserID,
		PresetKey: payload.PresetKey, PresetRelease: payload.PresetRelease,
		ApplicationMode: payload.ApplicationMode, ExpectedRevision: *payload.ExpectedRevision,
		IdempotencyKey: payload.IdempotencyKey,
	})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": selection})
}

func (handler *Handler) actor(writer http.ResponseWriter, request *http.Request) (authentication.Claims, bool) {
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		platformhttp.WriteProblem(writer, request, platformhttp.Problem{
			Code: "unauthenticated", Message: "Invalid credentials", Status: http.StatusUnauthorized, NextAction: "login",
		})
		return authentication.Claims{}, false
	}
	return claims, true
}

func (handler *Handler) project(
	writer http.ResponseWriter,
	request *http.Request,
	claims authentication.Claims,
) (projectdomain.Project, bool) {
	projectID := request.PathValue("project_id")
	project, err := handler.projects.Get(request.Context(), projectapp.Actor{
		UserID: claims.UserID, TokenVersion: claims.TokenVersion,
	}, projectID)
	if err != nil {
		handler.writeError(writer, request, err)
		return projectdomain.Project{}, false
	}
	if project.ID != projectID || project.WorkspaceID == "" {
		handler.writeError(writer, request, errors.New("authorized Project identity has drifted"))
		return projectdomain.Project{}, false
	}
	return project, true
}

func (handler *Handler) writeError(writer http.ResponseWriter, request *http.Request, err error) {
	problem := platformhttp.Problem{Code: "internal_error", Message: "Internal server error", Status: http.StatusInternalServerError}
	var projectError *projectapp.Error
	if errors.As(err, &projectError) {
		problem = platformhttp.Problem{
			Code: projectError.Code, Message: projectError.Message, Status: projectError.Status,
			NextAction: projectError.NextAction, Details: projectError.Details,
		}
	} else if errors.Is(err, presetapp.ErrProjectSelectionNotFound) {
		problem = platformhttp.Problem{Code: "project_preset_selection_not_found", Message: "Project Preset selection not found", Status: http.StatusNotFound}
	} else {
		var selectionError *presetapp.ProjectSelectionError
		if errors.As(err, &selectionError) {
			problem.Code, problem.Message = selectionError.Code, selectionError.Message
			switch selectionError.Code {
			case "invalid_project_preset_selection":
				problem.Status = http.StatusUnprocessableEntity
			case "project_preset_selection_conflict", "project_preset_selection_idempotency_conflict":
				problem.Status = http.StatusConflict
			case "project_preset_selection_forbidden":
				problem.Status = http.StatusForbidden
			case "project_preset_selection_scope_invalid":
				problem.Status = http.StatusNotFound
			}
		}
	}
	platformhttp.WriteProblem(writer, request, problem)
}
