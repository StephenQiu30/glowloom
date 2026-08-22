package agents

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
)

type AgentController struct{ service *AgentService }

func NewAgentController(service *AgentService) *AgentController {
	return &AgentController{service: service}
}

type startRequest struct {
	ProjectID   uuid.UUID `json:"project_id"`
	OperationID uuid.UUID `json:"operation_id"`
	Skill       string    `json:"skill"`
	Stage       string    `json:"stage"`
	RequestHash string    `json:"request_hash"`
	SnapshotRef string    `json:"snapshot_ref"`
}

func (h *AgentController) Router() http.Handler {
	router := chi.NewRouter()
	h.Mount(router)
	return router
}

func (h *AgentController) Mount(router chi.Router) {
	router.Post("/api/agent-runs", h.start)
	router.Get("/api/agent-runs/{agentRunID}", h.get)
	router.Post("/api/agent-runs/{agentRunID}/cancel", h.cancel)
}

func (h *AgentController) start(w http.ResponseWriter, r *http.Request) {
	var body startRequest
	if !httpapi.DecodeJSON(w, r, &body, 1<<20) {
		return
	}
	run, items, err := h.service.Start(r.Context(), StartInput{ProjectID: body.ProjectID, OperationID: body.OperationID, Skill: body.Skill, Stage: body.Stage, RequestHash: body.RequestHash, SnapshotRef: body.SnapshotRef})
	if err != nil {
		httpapi.WriteError(w, r, err)
		return
	}
	httpapi.WriteData(w, httpapi.StatusAccepted, map[string]any{"run": run, "items": items})
}

func (h *AgentController) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "agentRunID"))
	if err != nil {
		httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusBadRequest, httpapi.CodeInvalidID, "AgentRun ID 无效", "检查 URL 中的资源 ID"))
		return
	}
	run, items, err := h.service.Get(r.Context(), id)
	if err != nil {
		httpapi.WriteError(w, r, err)
		return
	}
	httpapi.WriteData(w, httpapi.StatusOK, map[string]any{"run": run, "items": items})
}

func (h *AgentController) cancel(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "agentRunID"))
	if err != nil {
		httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusBadRequest, httpapi.CodeInvalidID, "AgentRun ID 无效", "检查 URL 中的资源 ID"))
		return
	}
	run, items, err := h.service.Cancel(r.Context(), id)
	if err != nil {
		httpapi.WriteError(w, r, err)
		return
	}
	httpapi.WriteData(w, httpapi.StatusOK, map[string]any{"run": run, "items": items})
}
