package httptransport

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/internal/modules/scripts/application"
)

type Handler struct {
	service *application.Service
}

func NewHandler(service *application.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Router() http.Handler {
	router := chi.NewRouter()
	router.Use(jsonContentType)
	router.Get("/readyz", h.ready)
	router.Route("/api", func(router chi.Router) {
		router.Post("/workspaces", h.createWorkspace)
		router.Post("/workspaces/{workspaceID}/projects", h.createProject)
		router.Post("/projects/{projectID}/script-revisions", h.createScriptRevision)
		router.Post("/script-revisions/{revisionID}/analyze", h.analyzeScript)
		router.Get("/script-revisions/{revisionID}/analysis-draft", h.getAnalysisDraft)
		router.Post("/script-revisions/{revisionID}/approve", h.approveAnalysis)
		router.Get("/operations/{operationID}", h.getOperation)
		router.Get("/projects/{projectID}/analysis", h.getProjectAnalysis)
	})
	return router
}

type createWorkspaceRequest struct {
	Name string `json:"name"`
}

type createProjectRequest struct {
	Name string `json:"name"`
}

type createScriptRevisionRequest struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func (h *Handler) ready(writer http.ResponseWriter, request *http.Request) {
	writeData(writer, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) createWorkspace(writer http.ResponseWriter, request *http.Request) {
	var body createWorkspaceRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	workspace, err := h.service.CreateWorkspace(request.Context(), strings.TrimSpace(body.Name))
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "workspace_invalid", err.Error(), "提供 1—120 个字符的 Workspace 名称")
		return
	}
	writeData(writer, http.StatusCreated, workspace)
}

func (h *Handler) createProject(writer http.ResponseWriter, request *http.Request) {
	workspaceID, ok := parseID(writer, chi.URLParam(request, "workspaceID"))
	if !ok {
		return
	}
	var body createProjectRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	project, err := h.service.CreateProject(request.Context(), workspaceID, strings.TrimSpace(body.Name))
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "project_invalid", err.Error(), "确认 Workspace 存在并提供项目名称")
		return
	}
	writeData(writer, http.StatusCreated, project)
}

func (h *Handler) createScriptRevision(writer http.ResponseWriter, request *http.Request) {
	projectID, ok := parseID(writer, chi.URLParam(request, "projectID"))
	if !ok {
		return
	}
	var body createScriptRevisionRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	revision, err := h.service.CreateScriptRevision(request.Context(), projectID, strings.TrimSpace(body.Name), body.Content)
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "script_invalid", err.Error(), "提供有内容的整本剧本")
		return
	}
	writeData(writer, http.StatusCreated, revision)
}

func (h *Handler) analyzeScript(writer http.ResponseWriter, request *http.Request) {
	revisionID, ok := parseID(writer, chi.URLParam(request, "revisionID"))
	if !ok {
		return
	}
	operation, err := h.service.QueueAnalysis(request.Context(), revisionID)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeData(writer, http.StatusAccepted, operation)
}

func (h *Handler) getOperation(writer http.ResponseWriter, request *http.Request) {
	operationID, ok := parseID(writer, chi.URLParam(request, "operationID"))
	if !ok {
		return
	}
	operation, err := h.service.GetOperation(request.Context(), operationID)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeData(writer, http.StatusOK, operation)
}

func (h *Handler) approveAnalysis(writer http.ResponseWriter, request *http.Request) {
	revisionID, ok := parseID(writer, chi.URLParam(request, "revisionID"))
	if !ok {
		return
	}
	analysis, err := h.service.ApproveAnalysis(request.Context(), revisionID)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeData(writer, http.StatusOK, analysis)
}

func (h *Handler) getAnalysisDraft(writer http.ResponseWriter, request *http.Request) {
	revisionID, ok := parseID(writer, chi.URLParam(request, "revisionID"))
	if !ok {
		return
	}
	analysis, err := h.service.GetAnalysisDraft(request.Context(), revisionID)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeData(writer, http.StatusOK, analysis)
}

func (h *Handler) getProjectAnalysis(writer http.ResponseWriter, request *http.Request) {
	projectID, ok := parseID(writer, chi.URLParam(request, "projectID"))
	if !ok {
		return
	}
	analysis, err := h.service.GetProjectAnalysis(request.Context(), projectID)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeData(writer, http.StatusOK, analysis)
}

func parseID(writer http.ResponseWriter, value string) (uuid.UUID, bool) {
	id, err := uuid.Parse(value)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_id", "资源 ID 格式无效", "检查 URL 中的资源 ID")
		return uuid.Nil, false
	}
	return id, true
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 8<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_json", "请求体不是有效的当前 JSON 合同", "按照 OpenAPI 请求模型重新提交")
		return false
	}
	return true
}

func writeServiceError(writer http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "request_failed"
	message := err.Error()
	if strings.Contains(message, "not found") {
		status = http.StatusNotFound
		code = "not_found"
	} else if strings.Contains(message, "already") || strings.Contains(message, "required") || strings.Contains(message, "empty") {
		status = http.StatusUnprocessableEntity
		code = "business_rule_blocked"
	}
	writeError(writer, status, code, message, "查看 Operation 或修正当前输入后重试")
}

func writeData(writer http.ResponseWriter, status int, data any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"data": data})
}

func writeError(writer http.ResponseWriter, status int, code, message, nextAction string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"error": map[string]string{"code": code, "message": message, "next_action": nextAction}})
}

func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.Header().Set("Content-Type", "application/json")
		}
		writer.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:8123")
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(writer, request)
	})
}
