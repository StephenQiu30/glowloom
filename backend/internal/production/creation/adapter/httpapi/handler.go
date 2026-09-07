package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

type Service interface {
	Create(context.Context, app.Actor, app.CreateCommand) (domain.Run, error)
	Get(context.Context, app.Actor, string) (domain.Run, error)
	List(context.Context, app.Actor, string, int) ([]domain.Run, error)
	Retry(context.Context, app.Actor, string, int64) (domain.Run, error)
}
type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}
type Handler struct {
	service   Service
	auth      Authenticator
	validator *validation.Validator
}

func New(service Service, auth Authenticator) *Handler {
	return &Handler{service: service, auth: auth, validator: validation.New()}
}
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/projects/{project_id}/creation-runs", h.create)
	mux.HandleFunc("GET /api/projects/{project_id}/creation-runs", h.list)
	mux.HandleFunc("GET /api/creation-runs/{run_id}", h.get)
	mux.HandleFunc("POST /api/creation-runs/{run_id}/retry-delivery", h.retry)
}

type createRequest struct {
	DocumentRevisionID string `json:"document_revision_id" validate:"required,uuid"`
	SourceHash         string `json:"source_hash" validate:"required,len=64,hexadecimal"`
	IdempotencyKey     string `json:"idempotency_key" validate:"required,max=200"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input createRequest
	if !platformhttp.DecodeStrict(w, r, h.validator, &input) {
		return
	}
	run, err := h.service.Create(r.Context(), actor, app.CreateCommand{ProjectID: r.PathValue("project_id"), DocumentRevisionID: input.DocumentRevisionID, SourceHash: input.SourceHash, IdempotencyKey: input.IdempotencyKey})
	h.write(w, r, run, err, http.StatusAccepted)
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	run, err := h.service.Get(r.Context(), actor, r.PathValue("run_id"))
	h.write(w, r, run, err, http.StatusOK)
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	limit := 20
	if raw, exists := r.URL.Query()["limit"]; exists {
		var err error
		if len(raw) != 1 {
			writeError(w, r, app.Problem("validation_failed", 422))
			return
		}
		limit, err = strconv.Atoi(raw[0])
		if err != nil {
			writeError(w, r, app.Problem("validation_failed", 422))
			return
		}
	}
	runs, err := h.service.List(r.Context(), actor, r.PathValue("project_id"), limit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	data := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		data = append(data, present(run))
	}
	platformhttp.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}
func (h *Handler) retry(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input struct {
		ExpectedRevision int64 `json:"expected_revision" validate:"required,min=1"`
	}
	if !platformhttp.DecodeStrict(w, r, h.validator, &input) {
		return
	}
	run, err := h.service.Retry(r.Context(), actor, r.PathValue("run_id"), input.ExpectedRevision)
	h.write(w, r, run, err, http.StatusOK)
}
func (h *Handler) actor(w http.ResponseWriter, r *http.Request) (app.Actor, bool) {
	claims, err := h.auth.Authenticate(r)
	if err != nil {
		writeError(w, r, app.Problem("unauthenticated", 401))
		return app.Actor{}, false
	}
	return app.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}
func (h *Handler) write(w http.ResponseWriter, r *http.Request, run domain.Run, err error, status int) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	platformhttp.WriteJSON(w, status, map[string]any{"data": present(run)})
}
func present(run domain.Run) map[string]any {
	return map[string]any{
		"id": run.Command.RunID, "project_id": run.Command.ProjectID, "workspace_id": run.Command.WorkspaceID,
		"source": run.Command.Source, "flow_type": run.Command.FlowType, "workflow_id": run.Command.WorkflowID,
		"status": run.Status, "revision": run.Revision, "last_error": run.LastError, "acceptance": run.Acceptance,
		"created_at": run.CreatedAt, "updated_at": run.UpdatedAt,
	}
}
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var problem *app.Error
	if errors.Is(err, app.ErrNotFound) {
		problem = &app.Error{Code: "not_found", Status: 404}
	} else if !errors.As(err, &problem) {
		problem = &app.Error{Code: "internal_error", Status: 500}
	}
	platformhttp.WriteProblem(w, r, platformhttp.Problem{Code: problem.Code, Message: problem.Code, Status: problem.Status})
}
