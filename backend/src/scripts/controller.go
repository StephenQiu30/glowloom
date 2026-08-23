package scripts

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	router.Get("/api/swagger.json", h.swaggerSchema)
	router.Get("/api/docs", h.swagger)
	router.Route("/api", func(router chi.Router) {
		router.Post("/workspaces", h.createWorkspace)
		router.Post("/workspaces/{workspaceID}/projects", h.createProject)
		router.Get("/workspaces/{workspaceID}/projects", h.listProjects)
		router.Post("/projects/{projectID}/script-revisions", h.createScriptRevision)
		router.Post("/script-revisions/{revisionID}/analyze", h.analyzeScript)
		router.Get("/script-revisions/{revisionID}/analysis-draft", h.getAnalysisDraft)
		router.Post("/script-revisions/{revisionID}/analysis-draft/revisions", h.reviseAnalysisDraft)
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

type createShotsRequest struct {
	Count int `json:"count"`
}

type createFixtureCandidateRequest struct {
	Purpose string `json:"purpose"`
}

type selectCandidateRequest struct {
	Purpose string `json:"purpose"`
}

type reviseAnalysisDraftRequest struct {
	ExpectedSourceHash string                      `json:"expected_source_hash"`
	Operations         []EpisodeBreakdownOperation `json:"operations"`
}

// ready 返回服务就绪状态。
// @Summary 服务就绪检查
// @Tags system
// @ID system_ready
// @Produce json
// @Success 200 {object} map[string]string
// @Router /readyz [get]
func (h *ScriptController) ready(writer http.ResponseWriter, request *http.Request) {
	httpapi.WriteData(writer, httpapi.StatusOK, map[string]string{"status": "ready"})
}

// swaggerSchema 返回由 Swagger 注释生成的机器可读文档。
// @Summary 获取 Swagger 文档
// @Tags system
// @ID system_swagger
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} httpapi.ErrorEnvelope
// @Router /api/swagger.json [get]
func (h *ScriptController) swaggerSchema(writer http.ResponseWriter, request *http.Request) {
	path := os.Getenv("SWAGGER_SCHEMA_PATH")
	if path == "" {
		path = "docs/swagger.json"
	}
	schema, err := os.ReadFile(path)
	if err != nil {
		httpapi.WriteError(writer, request, httpapi.NewError(httpapi.StatusServiceUnavailable, httpapi.CodeSchemaUnavailable, "当前 Swagger 文档不可用", "先运行 make swagger 生成当前文档后重试"))
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(schema)
}

func (h *ScriptController) swagger(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>Lanverse Swagger</title><link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.10/swagger-ui.css"></head><body><div id="swagger-ui"></div><script src="https://unpkg.com/swagger-ui-dist@5.11.10/swagger-ui-bundle.js"></script><script>window.onload=()=>SwaggerUIBundle({url:'/api/swagger.json',dom_id:'#swagger-ui'});</script></body></html>`))
}

// createWorkspace 创建 Workspace。
// @Summary 创建 Workspace
// @Tags workspace
// @ID workspace_create
// @Accept json
// @Produce json
// @Param request body createWorkspaceRequest true "Workspace 参数"
// @Security BearerAccessToken
// @Success 201 {object} WorkspaceEnvelope
// @Failure 422 {object} httpapi.ErrorEnvelope
// @Router /api/workspaces [post]
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

// createProject 创建项目。
// @Summary 创建项目
// @Tags project
// @ID project_create
// @Accept json
// @Produce json
// @Param workspaceID path string true "Workspace UUID"
// @Param request body createProjectRequest true "项目参数"
// @Security BearerAccessToken
// @Success 201 {object} ProjectEnvelope
// @Failure 422 {object} httpapi.ErrorEnvelope
// @Router /api/workspaces/{workspaceID}/projects [post]
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

// listProjects 查询当前 Workspace 的项目和最新可恢复剧本工作流。
// @Summary 查询项目列表
// @Tags project
// @ID project_list
// @Produce json
// @Param workspaceID path string true "Workspace UUID"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Security BearerAccessToken
// @Success 200 {object} ProjectPageEnvelope
// @Failure 404 {object} httpapi.ErrorEnvelope
// @Failure 422 {object} httpapi.ErrorEnvelope
// @Router /api/workspaces/{workspaceID}/projects [get]
func (h *ScriptController) listProjects(writer http.ResponseWriter, request *http.Request) {
	workspaceID, ok := parseID(writer, request, chi.URLParam(request, "workspaceID"))
	if !ok {
		return
	}
	page, err := parseProjectQueryInt(request, "page", 1)
	if err != nil {
		httpapi.WriteError(writer, request, err)
		return
	}
	pageSize, err := parseProjectQueryInt(request, "page_size", 20)
	if err != nil {
		httpapi.WriteError(writer, request, err)
		return
	}
	projects, err := h.service.ListProjects(request.Context(), workspaceID, ProjectQuery{Page: page, PageSize: pageSize})
	if err != nil {
		httpapi.WriteError(writer, request, err)
		return
	}
	httpapi.WriteData(writer, httpapi.StatusOK, projects)
}

func parseProjectQueryInt(request *http.Request, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(request.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, httpapi.Validation(key+" 必须是整数", "修改查询参数后重试")
	}
	return value, nil
}

// createScriptRevision 创建剧本修订。
// @Summary 创建剧本修订
// @Tags script
// @ID script_revision_create
// @Accept multipart/form-data
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param file formData file true "DOCX、Markdown 或 TXT 剧本原件"
// @Security BearerAccessToken
// @Success 201 {object} ScriptRevisionEnvelope
// @Failure 422 {object} httpapi.ErrorEnvelope
// @Router /api/projects/{projectID}/script-revisions [post]
func (h *ScriptController) createScriptRevision(writer http.ResponseWriter, request *http.Request) {
	projectID, ok := parseID(writer, request, chi.URLParam(request, "projectID"))
	if !ok {
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxSourceBytes+(1<<20))
	if err := request.ParseMultipartForm(1 << 20); err != nil {
		httpapi.WriteError(writer, request, httpapi.NewError(httpapi.StatusBadRequest, httpapi.CodeScriptInvalid, "剧本上传请求无效或超过大小限制", "选择不超过 32 MiB 的 DOCX、Markdown 或 TXT 文件后重试"))
		return
	}
	if request.MultipartForm != nil {
		defer request.MultipartForm.RemoveAll()
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		httpapi.WriteError(writer, request, httpapi.NewError(httpapi.StatusBadRequest, httpapi.CodeScriptInvalid, "未收到剧本文件", "选择 DOCX、Markdown 或 TXT 文件后重试"))
		return
	}
	defer file.Close()
	original, err := io.ReadAll(io.LimitReader(file, maxSourceBytes+1))
	if err != nil || len(original) > maxSourceBytes {
		httpapi.WriteError(writer, request, httpapi.NewError(httpapi.StatusBadRequest, httpapi.CodeScriptInvalid, "读取剧本原件失败或超过大小限制", "选择不超过 32 MiB 的剧本文件后重试"))
		return
	}
	revision, err := h.service.CreateScriptRevision(request.Context(), projectID, SourceUpload{
		FileName:  filepath.Base(strings.TrimSpace(header.Filename)),
		MediaType: header.Header.Get("Content-Type"),
		Original:  original,
	})
	if err != nil {
		httpapi.WriteError(writer, request, httpapi.Wrap(err, httpapi.StatusUnprocessableEntity, httpapi.CodeScriptInvalid, "剧本文件格式无效或内容损坏", "确认文件是未加密、可正常打开的 DOCX、Markdown 或 UTF-8 TXT 后重试"))
		return
	}
	httpapi.WriteData(writer, httpapi.StatusCreated, revision)
}

// analyzeScript 将剧本分析任务加入队列。
// @Summary 排队分析剧本
// @Tags script
// @ID script_analysis_queue
// @Produce json
// @Param revisionID path string true "Script Revision UUID"
// @Security BearerAccessToken
// @Success 202 {object} OperationEnvelope
// @Failure 404 {object} httpapi.ErrorEnvelope
// @Router /api/script-revisions/{revisionID}/analyze [post]
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

// getOperation 查询任务状态。
// @Summary 查询任务状态
// @Tags operation
// @ID operation_get
// @Produce json
// @Param operationID path string true "Operation UUID"
// @Security BearerAccessToken
// @Success 200 {object} OperationEnvelope
// @Failure 404 {object} httpapi.ErrorEnvelope
// @Router /api/operations/{operationID} [get]
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

// approveAnalysis 批准剧本分析结果。
// @Summary 批准剧本分析
// @Tags script
// @ID script_analysis_approve
// @Produce json
// @Param revisionID path string true "Script Revision UUID"
// @Security BearerAccessToken
// @Success 200 {object} AnalysisEnvelope
// @Failure 422 {object} httpapi.ErrorEnvelope
// @Router /api/script-revisions/{revisionID}/approve [post]
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

// getAnalysisDraft 获取待批准的剧本分析结果。
// @Summary 获取分析草稿
// @Tags script
// @ID script_analysis_draft
// @Produce json
// @Param revisionID path string true "Script Revision UUID"
// @Security BearerAccessToken
// @Success 200 {object} AnalysisEnvelope
// @Failure 404 {object} httpapi.ErrorEnvelope
// @Router /api/script-revisions/{revisionID}/analysis-draft [get]
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

// reviseAnalysisDraft 以当前来源 hash 为基线创建新的 EpisodeBreakdownRevision。
// @Summary 修订剧集拆解草稿
// @Tags script
// @ID script_analysis_draft_revise
// @Accept json
// @Produce json
// @Param revisionID path string true "Script Revision UUID"
// @Param request body reviseAnalysisDraftRequest true "剧集拆解操作"
// @Security BearerAccessToken
// @Success 201 {object} AnalysisEnvelope
// @Failure 404 {object} httpapi.ErrorEnvelope
// @Failure 409 {object} httpapi.ErrorEnvelope
// @Failure 422 {object} httpapi.ErrorEnvelope
// @Router /api/script-revisions/{revisionID}/analysis-draft/revisions [post]
func (h *ScriptController) reviseAnalysisDraft(writer http.ResponseWriter, request *http.Request) {
	revisionID, ok := parseID(writer, request, chi.URLParam(request, "revisionID"))
	if !ok {
		return
	}
	var body reviseAnalysisDraftRequest
	if !httpapi.DecodeJSON(writer, request, &body, 64<<10) {
		return
	}
	analysis, err := h.service.ReviseAnalysisDraft(request.Context(), revisionID, strings.TrimSpace(body.ExpectedSourceHash), body.Operations)
	if err != nil {
		httpapi.WriteError(writer, request, err)
		return
	}
	httpapi.WriteData(writer, httpapi.StatusCreated, analysis)
}

// getProjectAnalysis 获取项目分析结果。
// @Summary 获取项目分析
// @Tags project
// @ID project_analysis_get
// @Produce json
// @Param projectID path string true "Project UUID"
// @Security BearerAccessToken
// @Success 200 {object} AnalysisEnvelope
// @Failure 404 {object} httpapi.ErrorEnvelope
// @Router /api/projects/{projectID}/analysis [get]
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

// createShots 生成镜头计划。
// @Summary 创建镜头
// @Tags shot
// @ID shot_create
// @Accept json
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param contentUnitID path string true "Content Unit UUID"
// @Param request body createShotsRequest true "镜头参数"
// @Security BearerAccessToken
// @Success 201 {object} ShotListEnvelope
// @Failure 422 {object} httpapi.ErrorEnvelope
// @Router /api/projects/{projectID}/content-units/{contentUnitID}/shots [post]
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

// listShots 查询镜头列表。
// @Summary 查询镜头
// @Tags shot
// @ID shot_list
// @Produce json
// @Param projectID path string true "Project UUID"
// @Param contentUnitID path string true "Content Unit UUID"
// @Security BearerAccessToken
// @Success 200 {object} ShotListEnvelope
// @Router /api/projects/{projectID}/content-units/{contentUnitID}/shots [get]
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

// createFixtureCandidate 创建 Fixture 候选。
// @Summary 创建候选
// @Tags candidate
// @ID candidate_create_fixture
// @Accept json
// @Produce json
// @Param shotID path string true "Shot UUID"
// @Param request body createFixtureCandidateRequest true "候选参数"
// @Security BearerAccessToken
// @Success 201 {object} CandidateEnvelope
// @Router /api/shots/{shotID}/fixture-candidates [post]
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

// selectCandidate 确认候选选择。
// @Summary 选择候选
// @Tags candidate
// @ID candidate_select
// @Accept json
// @Produce json
// @Param candidateID path string true "Candidate UUID"
// @Param request body selectCandidateRequest true "选择参数"
// @Security BearerAccessToken
// @Success 201 {object} SelectionEnvelope
// @Router /api/candidates/{candidateID}/selections [post]
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
