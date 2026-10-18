package http

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// SessionPageView contains only safe device summaries.
type SessionPageView struct {
	Items      []SessionItemView `json:"items" binding:"required"`
	NextCursor *string           `json:"next_cursor" binding:"required"`
	AsOf       time.Time         `json:"as_of" binding:"required"`
}

// SessionDirectoryHandler serves the current account session directory and revocation.
type SessionDirectoryHandler struct {
	sessions *application.SessionDirectory
	secure   bool
}

// NewSessionDirectoryHandler constructs SessionDirectoryHandler with explicit caller-supplied dependencies.
func NewSessionDirectoryHandler(sessions *application.SessionDirectory, secure bool) *SessionDirectoryHandler {
	return &SessionDirectoryHandler{sessions: sessions, secure: secure}
}

// Register requires email authentication and Origin/CSRF protection on group.
func (h *SessionDirectoryHandler) Register(group *gin.RouterGroup) {
	group.GET("/me/sessions", h.list)
	group.POST("/me/sessions/revoke-others", h.revokeOthers)
}

// Revoke handles /sessions/:id after the email handler's current-only adapter is retired.
// @Summary deleteSession
// @ID deleteSession
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Resource UUID"
// @Success 204
// @Failure 401 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/sessions/{id} [delete]
func (h *SessionDirectoryHandler) Revoke(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	actor := Principal(c)
	if err := h.sessions.Revoke(c.Request.Context(), actor, id); err != nil {
		emailAuthProblem(c, err)
		return
	}
	if id == actor.SessionID {
		clearSessionCookies(c, h.secure)
	}
	c.Status(204)
}

// SessionItemView excludes token material from a session description.
type SessionItemView struct {
	ID                uuid.UUID `json:"id" binding:"required"`
	Current           bool      `json:"current" binding:"required"`
	DeviceSummary     string    `json:"device_summary" binding:"required"`
	CreatedAt         time.Time `json:"created_at" binding:"required"`
	LastActiveAt      time.Time `json:"last_active_at" binding:"required"`
	AbsoluteExpiresAt time.Time `json:"absolute_expires_at" binding:"required"`
}

// @Summary getMySessions
// @ID getMySessions
// @Tags identity
// @Accept json
// @Produce json
// @Success 200 {object} SessionPageView
// @Failure 401 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/me/sessions [get]
func (h *SessionDirectoryHandler) list(c *gin.Context) {
	sessions, err := h.sessions.List(c.Request.Context(), Principal(c))
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	items := make([]SessionItemView, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, SessionItemView{ID: s.ID, Current: s.Current, DeviceSummary: s.DeviceSummary, CreatedAt: s.CreatedAt, LastActiveAt: s.LastActiveAt, AbsoluteExpiresAt: s.AbsoluteExpiresAt})
	}
	c.JSON(200, SessionPageView{Items: items, AsOf: time.Now().UTC()})
}

// @Summary revokeOtherSessions
// @ID revokeOtherSessions
// @Tags identity
// @Accept json
// @Produce json
// @Success 204
// @Failure 401 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/me/sessions/revoke-others [post]
func (h *SessionDirectoryHandler) revokeOthers(c *gin.Context) {
	if err := h.sessions.RevokeOthers(c.Request.Context(), Principal(c)); err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.Status(204)
}
