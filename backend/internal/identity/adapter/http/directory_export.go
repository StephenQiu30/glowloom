package http

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// DirectoryExportFilter has explicit scope-specific server validation.
type DirectoryExportFilter = application.ExportFilter

// DirectoryExportRequest contains the complete frozen filter for one request key.
type DirectoryExportRequest struct {
	Filter DirectoryExportFilter `json:"filter" binding:"required"`
}

// DirectoryExportView omits all object identifiers, credentials and snapshots.
type DirectoryExportView struct {
	ID            uuid.UUID  `json:"id" binding:"required"`
	Status        string     `json:"status" binding:"required"`
	AsOf          time.Time  `json:"as_of" binding:"required"`
	RowCount      int        `json:"row_count" binding:"required"`
	ExpiresAt     *time.Time `json:"expires_at" binding:"required"`
	ErrorCategory string     `json:"error_category,omitempty"`
}

// PlatformStatisticsView separates account and membership counts.
type PlatformStatisticsView struct {
	AsOf            time.Time             `json:"as_of" binding:"required"`
	Filter          DirectoryExportFilter `json:"filter" binding:"required"`
	TotalUsers      int                   `json:"total_users" binding:"required"`
	VerifiedUsers   int                   `json:"verified_users" binding:"required"`
	ActiveTeamUsers int                   `json:"active_team_users" binding:"required"`
	MembershipCount int                   `json:"membership_count" binding:"required"`
	AccountStates   map[string]int        `json:"account_states" binding:"required"`
}

// TeamMemberStatisticsView exposes only the authorized team's membership aggregate.
type TeamMemberStatisticsView struct {
	TeamID       uuid.UUID             `json:"team_id" binding:"required"`
	AsOf         time.Time             `json:"as_of" binding:"required"`
	Filter       DirectoryExportFilter `json:"filter" binding:"required"`
	TotalMembers int                   `json:"total_members" binding:"required"`
	MemberStates map[string]int        `json:"member_states" binding:"required"`
}

// DirectoryExportHandler registers current-authority statistics and CSV streams.
type DirectoryExportHandler struct {
	exports    *application.DirectoryExport
	statistics *application.DirectoryStatistics
}

// NewDirectoryExportHandler injects services without starting background work.
func NewDirectoryExportHandler(exports *application.DirectoryExport, statistics *application.DirectoryStatistics) *DirectoryExportHandler {
	return &DirectoryExportHandler{exports: exports, statistics: statistics}
}

// Register requires the existing authenticated same-origin session middleware.
func (h *DirectoryExportHandler) Register(g *gin.RouterGroup) {
	g.GET("/platform/user-statistics", h.platformStatistics)
	g.GET("/teams/:id/member-statistics", h.teamStatistics)
	g.POST("/platform/user-exports", h.createPlatform)
	g.GET("/platform/user-exports/:id", h.readPlatform)
	g.GET("/platform/user-exports/:id/download", h.downloadPlatform)
	g.POST("/teams/:id/member-exports", h.createTeam)
	g.GET("/teams/:id/member-exports/:eid", h.readTeam)
	g.GET("/teams/:id/member-exports/:eid/download", h.downloadTeam)
}
func exportView(j application.DirectoryExportJob) DirectoryExportView {
	var expires *time.Time
	if j.Status == "completed" {
		expires = &j.ExpiresAt
	}
	return DirectoryExportView{ID: j.ID, Status: j.Status, AsOf: j.AsOf, RowCount: j.RowCount, ExpiresAt: expires, ErrorCategory: j.ErrorCategory}
}
func exportProblem(c *gin.Context, err error) {
	switch {
	case errors.Is(err, application.ErrExportLimit):
		httpapi.WriteProblem(c, 409, "export_limit_exceeded", nil)
	case errors.Is(err, application.ErrExportConflict):
		httpapi.WriteProblem(c, 409, "idempotency_conflict", nil)
	case errors.Is(err, application.ErrExportNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, application.ErrExportUnavailable):
		httpapi.WriteProblem(c, 503, "export_unavailable", nil)
	default:
		directoryProblem(c, err)
	}
}
func parseStatisticsFilter(raw string, team bool) (application.ExportFilter, error) {
	q, err := url.ParseQuery(raw)
	if err != nil {
		return application.ExportFilter{}, application.ErrInvalidDirectoryFilter
	}
	role := ""
	if team {
		if v, ok := q["role"]; ok {
			if len(v) != 1 {
				return application.ExportFilter{}, application.ErrInvalidDirectoryFilter
			}
			role = v[0]
			q.Del("role")
		}
	}
	for key := range q {
		if key == "limit" || key == "cursor" || team && (key == "verified" || key == "in_team" || key == "platform_role" || key == "order") {
			return application.ExportFilter{}, application.ErrInvalidDirectoryFilter
		}
	}
	f, err := parseDirectoryFilter(q.Encode())
	if err != nil {
		return application.ExportFilter{}, err
	}
	return application.ExportFilter{Search: f.Search, Status: f.Status, Role: role, PlatformRole: f.PlatformRole, Order: f.Order, Verified: f.Verified, InTeam: f.InTeam, From: f.From, To: f.To}, nil
}

// @Summary getPlatformUserStatistics
// @ID getPlatformUserStatistics
// @Tags identity
// @Produce json
// @Param search query string false "Filter"
// @Param status query string false "Account status"
// @Param platform_role query string false "Platform role"
// @Param verified query boolean false "Verified email"
// @Param in_team query boolean false "Available team member"
// @Param order query string false "Directory order"
// @Param from query string false "Created from inclusive RFC3339"
// @Param to query string false "Created before RFC3339"
// @Success 200 {object} PlatformStatisticsView
// @Router /api/platform/user-statistics [get]
func (h *DirectoryExportHandler) platformStatistics(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := parseStatisticsFilter(c.Request.URL.RawQuery, false)
	if err != nil {
		exportProblem(c, err)
		return
	}
	s, err := h.statistics.Users(c.Request.Context(), Principal(c), f)
	if err != nil {
		exportProblem(c, err)
		return
	}
	c.JSON(200, PlatformStatisticsView{AsOf: s.AsOf, Filter: s.Filter, TotalUsers: s.TotalUsers, VerifiedUsers: s.VerifiedUsers, ActiveTeamUsers: s.ActiveTeamUsers, MembershipCount: s.MembershipCount, AccountStates: s.AccountStates})
}

// @Summary getTeamMemberStatistics
// @ID getTeamMemberStatistics
// @Tags identity
// @Produce json
// @Param id path string true "Team ID"
// @Param search query string false "Filter"
// @Param status query string false "Membership status"
// @Param role query string false "Team role"
// @Param from query string false "Joined from inclusive RFC3339"
// @Param to query string false "Joined before RFC3339"
// @Success 200 {object} TeamMemberStatisticsView
// @Router /api/teams/{id}/member-statistics [get]
func (h *DirectoryExportHandler) teamStatistics(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	f, err := parseStatisticsFilter(c.Request.URL.RawQuery, true)
	if err != nil {
		exportProblem(c, err)
		return
	}
	s, err := h.statistics.Members(c.Request.Context(), Principal(c), id, f)
	if err != nil {
		exportProblem(c, err)
		return
	}
	c.JSON(200, TeamMemberStatisticsView{TeamID: s.TeamID, AsOf: s.AsOf, Filter: s.Filter, TotalMembers: s.TotalMembers, MemberStates: s.MemberStates})
}

// @Summary createPlatformUserExport
// @ID createPlatformUserExport
// @Tags identity
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "Request UUID"
// @Param body body DirectoryExportRequest true "Filtered export"
// @Success 202 {object} DirectoryExportView
// @Router /api/platform/user-exports [post]
func (h *DirectoryExportHandler) createPlatform(c *gin.Context) { h.create(c, nil) }

// @Summary createTeamMemberExport
// @ID createTeamMemberExport
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param Idempotency-Key header string true "Request UUID"
// @Param body body DirectoryExportRequest true "Filtered member export"
// @Success 202 {object} DirectoryExportView
// @Router /api/teams/{id}/member-exports [post]
func (h *DirectoryExportHandler) createTeam(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if ok {
		h.create(c, &id)
	}
}
func (h *DirectoryExportHandler) create(c *gin.Context, team *uuid.UUID) {
	c.Header("Cache-Control", "no-store")
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		exportProblem(c, application.ErrInvalidDirectoryFilter)
		return
	}
	var body DirectoryExportRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	job, err := h.exports.Create(c.Request.Context(), Principal(c), team, body.Filter, key)
	if err != nil {
		exportProblem(c, err)
		return
	}
	c.JSON(202, exportView(job))
}

// @Summary getPlatformUserExport
// @ID getPlatformUserExport
// @Tags identity
// @Produce json
// @Param id path string true "Export ID"
// @Success 200 {object} DirectoryExportView
// @Router /api/platform/user-exports/{id} [get]
func (h *DirectoryExportHandler) readPlatform(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if ok {
		h.read(c, nil, id)
	}
}

// @Summary getTeamMemberExport
// @ID getTeamMemberExport
// @Tags identity
// @Produce json
// @Param id path string true "Team ID"
// @Param eid path string true "Export ID"
// @Success 200 {object} DirectoryExportView
// @Router /api/teams/{id}/member-exports/{eid} [get]
func (h *DirectoryExportHandler) readTeam(c *gin.Context) {
	team, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	id, ok := httpapi.PathUUID(c, "eid")
	if ok {
		h.read(c, &team, id)
	}
}
func (h *DirectoryExportHandler) read(c *gin.Context, team *uuid.UUID, id uuid.UUID) {
	c.Header("Cache-Control", "no-store")
	job, err := h.exports.Read(c.Request.Context(), Principal(c), team, id)
	if err != nil {
		exportProblem(c, err)
		return
	}
	c.JSON(200, exportView(job))
}

// @Summary downloadPlatformUserExport
// @ID downloadPlatformUserExport
// @Tags identity
// @Produce text/csv
// @Param id path string true "Export ID"
// @Success 200 {file} file
// @Router /api/platform/user-exports/{id}/download [get]
func (h *DirectoryExportHandler) downloadPlatform(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if ok {
		h.download(c, nil, id)
	}
}

// @Summary downloadTeamMemberExport
// @ID downloadTeamMemberExport
// @Tags identity
// @Produce text/csv
// @Param id path string true "Team ID"
// @Param eid path string true "Export ID"
// @Success 200 {file} file
// @Router /api/teams/{id}/member-exports/{eid}/download [get]
func (h *DirectoryExportHandler) downloadTeam(c *gin.Context) {
	team, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	id, ok := httpapi.PathUUID(c, "eid")
	if ok {
		h.download(c, &team, id)
	}
}
func (h *DirectoryExportHandler) download(c *gin.Context, team *uuid.UUID, id uuid.UUID) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	controller := http.NewResponseController(c.Writer)
	if err := controller.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		exportProblem(c, application.ErrExportUnavailable)
		return
	}
	defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
	err := h.exports.Stream(c.Request.Context(), Principal(c), team, id, func(reader io.Reader) error {
		c.Header("Content-Disposition", `attachment; filename="directory-export.csv"`)
		c.Header("Content-Type", "text/csv; charset=utf-8")
		_, err := io.Copy(c.Writer, reader)
		return err
	})
	if err != nil && !c.Writer.Written() {
		exportProblem(c, err)
	}
}
