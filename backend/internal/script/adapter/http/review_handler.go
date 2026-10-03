package http

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// ReviewHandler serves actual rules/manual editing and immutable history only.
type ReviewHandler struct {
	episodes *application.EpisodeService
	history  *application.HistoryService
}

// NewReviewHandler injects actual formal review and the authorized private-history reader.
func NewReviewHandler(episodes *application.EpisodeService, history *application.HistoryService) *ReviewHandler {
	return &ReviewHandler{episodes: episodes, history: history}
}

// Register keeps Gin command components literal and distinct from wildcard identifiers.
func (h *ReviewHandler) Register(g *gin.RouterGroup) {
	g.GET("/projects/:pid/script-versions", h.Versions)
	g.GET("/projects/:pid/script-sources/:lineage/history", h.SourceHistory)
	g.GET("/projects/:pid/script-source-snapshots/:sid", h.SourceSnapshot)
	g.GET("/projects/:pid/episodes", h.Episodes)
	g.POST("/projects/:pid/episodes/split/resplit", h.Resplit)
	g.POST("/projects/:pid/episodes/split/confirm", h.ConfirmSplit)
	g.GET("/episodes/:eid/source-text", h.SourceText)
	g.GET("/episodes/:eid/structure", h.Structure)
	g.GET("/episodes/:eid/structure/versions", h.StructureVersions)
	g.GET("/episodes/:eid/structure/versions/:version", h.StructureVersion)
	g.POST("/episodes/:eid/structure", h.SaveStructure)
	g.POST("/episodes/:eid/structure/confirm", h.ConfirmStructure)
}

// SplitReviewRequest binds complete reviewed boundaries to both visible heads.
type SplitReviewRequest struct {
	VersionID             uuid.UUID                `json:"version_id"`
	ExpectedRevision      *int64                   `json:"expected_revision"`
	ExpectedSplitRevision *int64                   `json:"expected_split_revision"`
	CandidateSetID        uuid.UUID                `json:"candidate_set_id"`
	Boundaries            []domain.EpisodeBoundary `json:"boundaries,omitempty"`
	Preface               *domain.ScalarSpan       `json:"preface,omitempty"`
	AckInvalidate         bool                     `json:"ack_invalidate"`
}

// StructureSaveRequest is a complete immutable manual candidate, not a partial JSON patch.
type StructureSaveRequest struct {
	ExpectedRevision        *int64                    `json:"expected_revision"`
	ExpectedEpisodeRevision *int64                    `json:"expected_episode_revision"`
	BaseStructureVersionNo  *int64                    `json:"base_structure_version_no"`
	Document                *domain.StructureDocument `json:"document"`
}

// StructureConfirmRequest cannot replace a candidate while requesting confirmation.
type StructureConfirmRequest struct {
	ExpectedRevision        *int64 `json:"expected_revision"`
	ExpectedEpisodeRevision *int64 `json:"expected_episode_revision"`
	BaseStructureVersionNo  *int64 `json:"base_structure_version_no"`
	AckInvalidate           bool   `json:"ack_invalidate"`
}

// StructureDetail retains safe episode status with the selected immutable body.
type StructureDetail struct {
	Episode   domain.Episode          `json:"episode"`
	Structure domain.EpisodeStructure `json:"structure"`
}

// Versions lists all immutable source-manifest versions without eager private body reads.
// @Summary 完整剧本版本历史
// @Tags script
// @ID listScriptVersions
// @Produce json
// @Param pid path string true "项目UUID"
// @Param before_version_no query integer false "严格早于版本号"
// @Param limit query integer false "1..100"
// @Success 200 {object} application.VersionPage
// @Router /api/projects/{pid}/script-versions [get]
func (h *ReviewHandler) Versions(c *gin.Context) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	before, limit, ok := pageQuery(c, "before_version_no")
	if !ok {
		return
	}
	out, err := h.history.Versions(c.Request.Context(), identityhttp.Principal(c), pid, before, limit)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// SourceHistory preserves stable-lineage history, including snapshots no longer in the draft.
// @Summary 稳定章节身份的全部快照
// @Tags script
// @ID listScriptSourceHistory
// @Produce json
// @Param pid path string true "项目UUID"
// @Param lineage path string true "稳定来源UUID"
// @Param before_revision query integer false "严格早于来源修订"
// @Param limit query integer false "1..100"
// @Success 200 {object} application.SourceHistoryPage
// @Router /api/projects/{pid}/script-sources/{lineage}/history [get]
func (h *ReviewHandler) SourceHistory(c *gin.Context) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	id, ok := httpapi.PathUUID(c, "lineage")
	if !ok {
		return
	}
	before, limit, ok := pageQuery(c, "before_revision")
	if !ok {
		return
	}
	out, err := h.history.SourceHistory(c.Request.Context(), identityhttp.Principal(c), pid, id, before, limit)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// SourceSnapshot proves exact original HTML/rich data only for this selected historical source.
// @Summary 读取指定不可变章节快照
// @Tags script
// @ID getScriptSourceSnapshot
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "来源快照UUID"
// @Success 200 {object} application.SourceSnapshotDetail
// @Router /api/projects/{pid}/script-source-snapshots/{sid} [get]
func (h *ReviewHandler) SourceSnapshot(c *gin.Context) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	id, ok := httpapi.PathUUID(c, "sid")
	if !ok {
		return
	}
	out, err := h.history.SourceSnapshot(c.Request.Context(), identityhttp.Principal(c), pid, id)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// Episodes separates proposed split boundaries from confirmed formal identities.
// @Summary 指定版本的候选与正式分集
// @Tags script
// @ID listScriptEpisodes
// @Produce json
// @Param pid path string true "项目UUID"
// @Param version_id query string true "剧本版本UUID"
// @Success 200 {object} application.EpisodeView
// @Router /api/projects/{pid}/episodes [get]
func (h *ReviewHandler) Episodes(c *gin.Context) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	version, ok := versionQuery(c)
	if !ok {
		return
	}
	if version == nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	out, err := h.episodes.Episodes(c.Request.Context(), identityhttp.Principal(c), pid, *version)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// Resplit executes only actual local heading rules, without a simulated model call.
// @Summary 按正文标题规则重新生成候选分集
// @Tags script
// @ID resplitScriptEpisodes
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body SplitReviewRequest true "当前头与候选修订"
// @Success 200 {object} application.SplitReceipt
// @Router /api/projects/{pid}/episodes/split/resplit [post]
func (h *ReviewHandler) Resplit(c *gin.Context) { h.split(c, false) }

// ConfirmSplit commits the complete reviewed boundary set and immutable confirmation.
// @Summary 确认完整分集边界与身份映射
// @Tags script
// @ID confirmScriptEpisodeSplit
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body SplitReviewRequest true "完整分集边界及影响确认"
// @Success 200 {object} application.SplitReceipt
// @Failure 409 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/episodes/split/confirm [post]
func (h *ReviewHandler) ConfirmSplit(c *gin.Context) { h.split(c, true) }
func (h *ReviewHandler) split(c *gin.Context, confirm bool) {
	var r SplitReviewRequest
	if !readJSON(c, &r) {
		return
	}
	base, ok := commandInput(c, "split", r.ExpectedRevision, nil)
	if !ok {
		return
	}
	if r.ExpectedSplitRevision == nil {
		scriptProblem(c, domain.ErrInvalidSpan)
		return
	}
	input := application.SplitCommand{ProjectID: base.ProjectID, VersionID: r.VersionID, Key: base.Key, RequestID: base.RequestID, ExpectedRevision: base.ExpectedRevision, ExpectedSplitRevision: *r.ExpectedSplitRevision, CandidateSetID: r.CandidateSetID, Boundaries: r.Boundaries, Preface: r.Preface, AckInvalidate: r.AckInvalidate}
	var out application.SplitReceipt
	var err error
	if confirm {
		out, err = h.episodes.ConfirmSplit(c.Request.Context(), identityhttp.Principal(c), input)
	} else {
		out, err = h.episodes.Resplit(c.Request.Context(), identityhttp.Principal(c), input)
	}
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// SourceText limits reads by immutable global Unicode scalar coordinates.
// @Summary 读取分集规范正文区间
// @Tags script
// @ID getScriptEpisodeSourceText
// @Produce json
// @Param eid path string true "正式分集UUID"
// @Param from query integer true "包含起点，Unicode scalar"
// @Param to query integer true "不包含终点，最多65536scalar"
// @Success 200 {object} application.EpisodeText
// @Router /api/episodes/{eid}/source-text [get]
func (h *ReviewHandler) SourceText(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "eid")
	if !ok {
		return
	}
	from, err := strconv.Atoi(c.Query("from"))
	to, toErr := strconv.Atoi(c.Query("to"))
	if err != nil || toErr != nil {
		scriptProblem(c, domain.ErrInvalidSpan)
		return
	}
	out, err := h.history.EpisodeText(c.Request.Context(), identityhttp.Principal(c), id, from, to)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// Structure returns the current manual head with its retained formal episode status.
// @Summary 当前手工结构
// @Tags script
// @ID getScriptEpisodeStructure
// @Produce json
// @Param eid path string true "正式分集UUID"
// @Success 200 {object} StructureDetail
// @Router /api/episodes/{eid}/structure [get]
func (h *ReviewHandler) Structure(c *gin.Context) { h.structure(c, 0) }

// StructureVersions lists actual immutable history including superseded candidates.
// @Summary 手工结构全部版本
// @Tags script
// @ID listScriptEpisodeStructureVersions
// @Produce json
// @Param eid path string true "正式分集UUID"
// @Param before_version_no query integer false "严格早于结构版本号"
// @Param limit query integer false "1..100"
// @Success 200 {object} application.StructurePage
// @Router /api/episodes/{eid}/structure/versions [get]
func (h *ReviewHandler) StructureVersions(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "eid")
	if !ok {
		return
	}
	before, limit, ok := pageQuery(c, "before_version_no")
	if !ok {
		return
	}
	out, err := h.history.Structures(c.Request.Context(), identityhttp.Principal(c), id, before, limit)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// StructureVersion selects one immutable version without replacing current pointers.
// @Summary 指定历史手工结构
// @Tags script
// @ID getScriptEpisodeStructureVersion
// @Produce json
// @Param eid path string true "正式分集UUID"
// @Param version path integer true "结构版本号"
// @Success 200 {object} StructureDetail
// @Router /api/episodes/{eid}/structure/versions/{version} [get]
func (h *ReviewHandler) StructureVersion(c *gin.Context) {
	version, err := strconv.ParseInt(c.Param("version"), 10, 64)
	if err != nil || version < 1 {
		scriptProblem(c, domain.ErrInvalidStructure)
		return
	}
	h.structure(c, version)
}
func (h *ReviewHandler) structure(c *gin.Context, version int64) {
	id, ok := httpapi.PathUUID(c, "eid")
	if !ok {
		return
	}
	actor := identityhttp.Principal(c)
	e, err := h.history.Episode(c.Request.Context(), actor, id)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	s, err := h.history.Structure(c.Request.Context(), actor, id, version)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, StructureDetail{Episode: e, Structure: s})
}

// SaveStructure preserves every reviewed item and creates a new immutable candidate.
// @Summary 保存完整手工结构候选
// @Tags script
// @ID saveScriptEpisodeStructure
// @Accept json
// @Produce json
// @Param eid path string true "正式分集UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body StructureSaveRequest true "完整结构与两级CAS"
// @Success 200 {object} application.StructureReceipt
// @Router /api/episodes/{eid}/structure [post]
func (h *ReviewHandler) SaveStructure(c *gin.Context) {
	var r StructureSaveRequest
	if !readJSON(c, &r) {
		return
	}
	input, ok := h.structureCommand(c, r.ExpectedRevision, r.ExpectedEpisodeRevision, r.BaseStructureVersionNo)
	if !ok {
		return
	}
	if r.Document == nil {
		scriptProblem(c, domain.ErrInvalidStructure)
		return
	}
	input.Document = *r.Document
	out, err := h.episodes.SaveStructure(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// ConfirmStructure requires current formal identity and explicit downstream impact evidence.
// @Summary 明确确认当前手工结构
// @Tags script
// @ID confirmScriptEpisodeStructure
// @Accept json
// @Produce json
// @Param eid path string true "正式分集UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body StructureConfirmRequest true "当前结构CAS与影响确认"
// @Success 200 {object} application.StructureReceipt
// @Failure 409 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/episodes/{eid}/structure/confirm [post]
func (h *ReviewHandler) ConfirmStructure(c *gin.Context) {
	var r StructureConfirmRequest
	if !readJSON(c, &r) {
		return
	}
	input, ok := h.structureCommand(c, r.ExpectedRevision, r.ExpectedEpisodeRevision, r.BaseStructureVersionNo)
	if !ok {
		return
	}
	input.AckInvalidate = r.AckInvalidate
	out, err := h.episodes.ConfirmStructure(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, out)
}
func (h *ReviewHandler) structureCommand(c *gin.Context, revision, episodeRevision, baseVersion *int64) (application.StructureCommand, bool) {
	if revision == nil || episodeRevision == nil || baseVersion == nil {
		scriptProblem(c, domain.ErrInvalidStructure)
		return application.StructureCommand{}, false
	}
	id, ok := httpapi.PathUUID(c, "eid")
	if !ok {
		return application.StructureCommand{}, false
	}
	actor := identityhttp.Principal(c)
	e, err := h.history.Episode(c.Request.Context(), actor, id)
	if err != nil {
		scriptProblem(c, err)
		return application.StructureCommand{}, false
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	request, requestErr := uuid.Parse(c.GetString("request_id"))
	if err != nil || requestErr != nil || key == uuid.Nil || request == uuid.Nil {
		scriptProblem(c, domain.ErrInvalidStructure)
		return application.StructureCommand{}, false
	}
	return application.StructureCommand{ProjectID: e.ProjectID, EpisodeID: id, Key: key, RequestID: request, ExpectedRevision: *revision, ExpectedEpisodeRevision: *episodeRevision, BaseStructureVersionNo: *baseVersion}, true
}
