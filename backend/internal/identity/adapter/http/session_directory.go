package http

import (
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"time"
)

type SessionDirectoryHandler struct {
	sessions *application.SessionDirectory
	secure   bool
}

func NewSessionDirectoryHandler(sessions *application.SessionDirectory, secure bool) *SessionDirectoryHandler {
	return &SessionDirectoryHandler{sessions: sessions, secure: secure}
}

// Register requires email authentication and Origin/CSRF protection on group.
func (h *SessionDirectoryHandler) Register(group *gin.RouterGroup) {
	group.GET("/me/sessions", h.list)
	group.POST("/me/sessions/revoke-others", h.revokeOthers)
}

// Revoke handles /sessions/:id after the email handler's current-only adapter is retired.
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
		http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	}
	c.Status(204)
}

type SessionItemView struct {
	ID                uuid.UUID `json:"id"`
	Current           bool      `json:"current"`
	DeviceSummary     string    `json:"device_summary"`
	CreatedAt         time.Time `json:"created_at"`
	LastActiveAt      time.Time `json:"last_active_at"`
	AbsoluteExpiresAt time.Time `json:"absolute_expires_at"`
}

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
	c.JSON(200, struct {
		Items      []SessionItemView `json:"items"`
		NextCursor *string           `json:"next_cursor"`
		AsOf       time.Time         `json:"as_of"`
	}{Items: items, AsOf: time.Now().UTC()})
}
func (h *SessionDirectoryHandler) revokeOthers(c *gin.Context) {
	if err := h.sessions.RevokeOthers(c.Request.Context(), Principal(c)); err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.Status(204)
}
