package scripts

import (
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
)

type ScriptController struct {
	service *ScriptAnalysisService
}

func NewScriptController(service *ScriptAnalysisService) *ScriptController {
	return &ScriptController{service: service}
}

func (h *ScriptController) Router() http.Handler {
	router := chi.NewRouter()
	h.Mount(router)
	return router
}

func (h *ScriptController) Mount(router chi.Router) {
	router.Use(jsonContentType)
	router.Get("/readyz", h.ready)
	router.Get("/api/openapi.json", h.openapi)
	router.Get("/api/docs", h.swagger)
	router.Route("/api", func(router chi.Router) {
		router.Post("/workspaces", h.createWorkspace)
		router.Post("/workspaces/{workspaceID}/projects", h.createProject)
		router.Post("/projects/{projectID}/script-revisions", h.createScriptRevision)
		router.Post("/script-revisions/{revisionID}/analyze", h.analyzeScript)
		router.Get("/script-revisions/{revisionID}/analysis-draft", h.getAnalysisDraft)
		router.Post("/script-revisions/{revisionID}/approve", h.approveAnalysis)
		router.Get("/operations/{operationID}", h.getOperation)
		router.Get("/projects/{projectID}/analysis", h.getProjectAnalysis)
		router.Post("/projects/{projectID}/content-units/{contentUnitID}/shots", h.createShots)
		router.Get("/projects/{projectID}/content-units/{contentUnitID}/shots", h.listShots)
		router.Post("/shots/{shotID}/fixture-candidates", h.createFixtureCandidate)
		router.Post("/candidates/{candidateID}/selections", h.selectCandidate)
	})
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

type createShotsRequest struct {
	Count int `json:"count"`
}

type createFixtureCandidateRequest struct {
	Purpose string `json:"purpose"`
}

type selectCandidateRequest struct {
	Purpose string `json:"purpose"`
}

func (h *ScriptController) ready(writer http.ResponseWriter, request *http.Request) {
	httpapi.WriteData(writer, httpapi.StatusOK, map[string]string{"status": "ready"})
}

func (h *ScriptController) openapi(writer http.ResponseWriter, request *http.Request) {
	path := os.Getenv("OPENAPI_SCHEMA_PATH")
	if path == "" {
		path = "api/openapi.json"
	}
	schema, err := os.ReadFile(path)
	if err != nil {
		httpapi.WriteError(writer, request, httpapi.NewError(httpapi.StatusServiceUnavailable, httpapi.CodeSchemaUnavailable, "当前 OpenAPI 合同不可用", "恢复本地当前 Schema 后重试"))
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(schema)
}

func (h *ScriptController) swagger(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>Lanverse Swagger</title><link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.10/swagger-ui.css"></head><body><div id="swagger-ui"></div><script src="https://unpkg.com/swagger-ui-dist@5.11.10/swagger-ui-bundle.js"></script><script>window.onload=()=>SwaggerUIBundle({url:'/api/openapi.json',dom_id:'#swagger-ui'});</script></body></html>`))
}

func (h *ScriptController) createWorkspace(writer http.ResponseWriter, request *http.Request) {
	var body createWorkspaceRequest
	if !httpapi.DecodeJSON(writer, request, &body, 8<<20) {
		return
	}
	workspace, err := h.service.CreateWorkspace(request.Context(), strings.TrimSpace(body.Name))
	if err != nil {
		httpapi.WriteError(writer, request, httpapi.Wrap(err, httpapi.StatusUnprocessableEntity, httpapi.CodeWorkspaceInvalid, err.Error(), "提供 1—120 个字符的 Workspace 名称"))
		return
	}
	httpapi.WriteData(writer, httpapi.StatusCreated, workspace)
}

func (h *ScriptController) createProject(writer http.ResponseWriter, request *http.Request) {
	workspaceID, ok := parseID(writer, request, chi.URLParam(request, "workspaceID"))
	if !ok {
		return
	}
	var body createProjectRequest
	if !httpapi.DecodeJSON(writer, request, &body, 8<<20) {
		return
	}
	project, err := h.service.CreateProject(request.Context(), workspaceID, strings.TrimSpace(body.Name))
	if err != nil {
		httpapi.WriteError(writer, request, httpapi.Wrap(err, httpapi.StatusUnprocessableEntity, httpapi.CodeProjectInvalid, err.Error(), "确认 Workspace 存在并提供项目名称"))
		return
	}
	httpapi.WriteData(writer, httpapi.StatusCreated, project)
}

func (h *ScriptController) createScriptRevision(writer http.ResponseWriter, request *http.Request) {
	projectID, ok := parseID(writer, request, chi.URLParam(request, "projectID"))
	if !ok {
		return
	}
	var body createScriptRevisionRequest
	if !httpapi.DecodeJSON(writer, request, &body, 8<<20) {
		return
	}
	revision, err := h.service.CreateScriptRevision(request.Context(), projectID, strings.TrimSpace(body.Name), body.Content)
	if err != nil {
		httpapi.WriteError(writer, request, httpapi.Wrap(err, httpapi.StatusUnprocessableEntity, httpapi.CodeScriptInvalid, err.Error(), "提供有内容的整本剧本"))
		return
	}
	httpapi.WriteData(writer, httpapi.StatusCreated, revision)
}

func (h *ScriptController) analyzeScript(writer http.ResponseWriter, request *http.Request) {
	revisionID, ok := parseID(writer, request, chi.URLParam(request, "revisionID"))
	if !ok {
		return
	}
	operation, err := h.service.QueueAnalysis(request.Context(), revisionID)
	if err != nil {
		httpapi.WriteError(writer, request, err)
		return
	}
	httpapi.WriteData(writer, httpapi.StatusAccepted, operation)
}

func (h *ScriptController) getOperation(writer http.ResponseWriter, request *http.Request) {
	operationID, ok := parseID(writer, request, chi.URLParam(request, "operationID"))
	if !ok {
		return
	}
	operation, err := h.service.GetOperation(request.Context(), operationID)
	if err != nil {
		httpapi.WriteError(writer, request, err)
		return
	}
	httpapi.WriteData(writer, httpapi.StatusOK, operation)
}

func (h *ScriptController) approveAnalysis(writer http.ResponseWriter, request *http.Request) {
	revisionID, ok := parseID(writer, request, chi.URLParam(request, "revisionID"))
	if !ok {
		return
	}
	analysis, err := h.service.ApproveAnalysis(request.Context(), revisionID)
	if err != nil {
		httpapi.WriteError(writer, request, err)
		return
	}
	httpapi.WriteData(writer, httpapi.StatusOK, analysis)
}

func (h *ScriptController) getAnalysisDraft(writer http.ResponseWriter, request *http.Request) {
	revisionID, ok := parseID(writer, request, chi.URLParam(request, "revisionID"))
	if !ok {
		return
	}
	analysis, err := h.service.GetAnalysisDraft(request.Context(), revisionID)
	if err != nil {
		httpapi.WriteError(writer, request, err)
		return
	}
	httpapi.WriteData(writer, httpapi.StatusOK, analysis)
}

func (h *ScriptController) getProjectAnalysis(writer http.ResponseWriter, request *http.Request) {
	projectID, ok := parseID(writer, request, chi.URLParam(request, "projectID"))
	if !ok {
		return
	}
	analysis, err := h.service.GetProjectAnalysis(request.Context(), projectID)
	if err != nil {
		httpapi.WriteError(writer, request, err)
		return
	}
	httpapi.WriteData(writer, httpapi.StatusOK, analysis)
}

func (h *ScriptController) createShots(writer http.ResponseWriter, request *http.Request) {
	projectID, ok := parseID(writer, request, chi.URLParam(request, "projectID"))
	if !ok {
		return
	}
	contentUnitID, ok := parseID(writer, request, chi.URLParam(request, "contentUnitID"))
	if !ok {
		return
	}
	var body createShotsRequest
	if !httpapi.DecodeJSON(writer, request, &body, 8<<20) {
		return
	}
	shots, err := h.service.CreateShots(request.Context(), projectID, contentUnitID, body.Count)
	if err != nil {
		httpapi.WriteError(writer, request, err)
		return
	}
	httpapi.WriteData(writer, httpapi.StatusCreated, map[string]any{"items": shots})
}

func (h *ScriptController) listShots(writer http.ResponseWriter, request *http.Request) {
	projectID, ok := parseID(writer, request, chi.URLParam(request, "projectID"))
	if !ok {
		return
	}
	contentUnitID, ok := parseID(writer, request, chi.URLParam(request, "contentUnitID"))
	if !ok {
		return
	}
	shots, err := h.service.ListShots(request.Context(), projectID, contentUnitID)
	if err != nil {
		httpapi.WriteError(writer, request, err)
		return
	}
	httpapi.WriteData(writer, httpapi.StatusOK, map[string]any{"items": shots})
}

func (h *ScriptController) createFixtureCandidate(writer http.ResponseWriter, request *http.Request) {
	shotID, ok := parseID(writer, request, chi.URLParam(request, "shotID"))
	if !ok {
		return
	}
	var body createFixtureCandidateRequest
	if !httpapi.DecodeJSON(writer, request, &body, 8<<20) {
		return
	}
	candidate, err := h.service.CreateFixtureCandidate(request.Context(), shotID, strings.TrimSpace(body.Purpose))
	if err != nil {
		httpapi.WriteError(writer, request, err)
		return
	}
	httpapi.WriteData(writer, httpapi.StatusCreated, candidate)
}

func (h *ScriptController) selectCandidate(writer http.ResponseWriter, request *http.Request) {
	candidateID, ok := parseID(writer, request, chi.URLParam(request, "candidateID"))
	if !ok {
		return
	}
	var body selectCandidateRequest
	if !httpapi.DecodeJSON(writer, request, &body, 8<<20) {
		return
	}
	selection, err := h.service.SelectCandidate(request.Context(), candidateID, strings.TrimSpace(body.Purpose))
	if err != nil {
		httpapi.WriteError(writer, request, err)
		return
	}
	httpapi.WriteData(writer, httpapi.StatusCreated, selection)
}

func parseID(writer http.ResponseWriter, request *http.Request, value string) (uuid.UUID, bool) {
	id, err := uuid.Parse(value)
	if err != nil {
		httpapi.WriteError(writer, request, httpapi.NewError(httpapi.StatusBadRequest, httpapi.CodeInvalidID, "资源 ID 格式无效", "检查 URL 中的资源 ID"))
		return uuid.Nil, false
	}
	return id, true
}

func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.Header().Set("Content-Type", "application/json")
		}
		next.ServeHTTP(writer, request)
	})
}
