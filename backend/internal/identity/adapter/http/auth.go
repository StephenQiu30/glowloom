package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

const sessionCookie = "lanverse_session"

// Actor contains only safe account identity and credential version.
type Actor struct {
	ID                 string `json:"id" binding:"required"`
	LoginName          string `json:"login_name" binding:"required"`
	DisplayName        string `json:"display_name" binding:"required"`
	Role               string `json:"role" binding:"required" enums:"creator,admin"`
	MustChangePassword bool   `json:"must_change_password" binding:"required"`
	CredentialRevision int64  `json:"credential_revision" binding:"required"`
}

// SessionView is public session state without token or digest.
type SessionView struct {
	Actor             Actor     `json:"actor" binding:"required"`
	SessionID         string    `json:"session_id" binding:"required"`
	LastActiveAt      time.Time `json:"last_active_at" binding:"required"`
	AbsoluteExpiresAt time.Time `json:"absolute_expires_at" binding:"required"`
	Persistent        bool      `json:"persistent" binding:"required"`
}

// RegisterRequest does not accept a role or organization supplied by the browser.
type RegisterRequest struct {
	LoginName       string `json:"login_name" binding:"required"`
	DisplayName     string `json:"display_name" binding:"required"`
	Password        string `json:"password" binding:"required"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}

// LoginRequest controls only the current browser's cookie persistence.
type LoginRequest struct {
	LoginName  string `json:"login_name" binding:"required"`
	Password   string `json:"password" binding:"required"`
	Persistent bool   `json:"persistent" binding:"required"`
}

// PasswordChange requires the current credential and expected credential version.
type PasswordChange struct {
	ExpectedCredentialRevision int64  `json:"expected_credential_revision" binding:"required"`
	CurrentPassword            string `json:"current_password" binding:"required"`
	NewPassword                string `json:"new_password" binding:"required"`
	ConfirmPassword            string `json:"confirm_password" binding:"required"`
}

// AvailabilityView is an advisory registration precheck.
type AvailabilityView struct {
	Available bool `json:"available" binding:"required"`
}

// AuthHandler binds the accepted identity resources to the authentication service.
type AuthHandler struct {
	auth   *application.Auth
	secure bool
}

// NewAuthHandler injects the service and the externally configured cookie policy.
func NewAuthHandler(auth *application.Auth, secure bool) *AuthHandler {
	return &AuthHandler{auth: auth, secure: secure}
}

// Register publishes only the current M1 identity slice.
func (h *AuthHandler) Register(api *gin.RouterGroup) {
	api.POST("/accounts/register", h.register)
	api.POST("/sessions", h.login)
	api.GET("/session", h.current)
	api.DELETE("/sessions/:session_id", h.logout)
	api.POST("/me/password", h.password)
	api.GET("/accounts/availability", h.availability)
}

// RequireSession replaces the former fixed workspace identity on business routes.
func (h *AuthHandler) RequireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := h.auth.Current(c.Request.Context(), cookieToken(c))
		c.Header("Cache-Control", "no-store")
		if err != nil {
			authProblem(c, err)
			return
		}
		u := result.User
		if u.MustChangePassword {
			httpapi.WriteProblem(c, 403, "password_change_required", nil)
			return
		}
		if strings.HasPrefix(c.FullPath(), "/api/admin/") && u.Role != domain.RoleAdmin {
			httpapi.WriteProblem(c, 403, "forbidden", nil)
			return
		}
		c.Set("principal", application.Principal{ID: u.ID, OrgID: u.OrgID, LoginName: u.LoginName, DisplayName: u.DisplayName, Role: u.Role, MustChangePassword: u.MustChangePassword})
		c.Next()
	}
}

// @Summary Register a creator and establish a session
// @ID registerAccount
// @Accept json
// @Produce json
// @Param body body RegisterRequest true "Creator registration"
// @Success 201 {object} SessionView
// @Failure 422 {object} httpapi.Problem
// @Router /api/accounts/register [post]
func (h *AuthHandler) register(c *gin.Context) {
	var request RegisterRequest
	if !httpapi.Decode(c, &request) {
		return
	}
	result, err := h.auth.Register(c.Request.Context(), request.LoginName, request.DisplayName, request.Password, request.ConfirmPassword)
	h.respond(c, result, err, 201)
}

// @Summary Establish a real session
// @ID createSession
// @Accept json
// @Produce json
// @Param body body LoginRequest true "Login"
// @Success 201 {object} SessionView
// @Failure 401 {object} httpapi.Problem
// @Router /api/sessions [post]
func (h *AuthHandler) login(c *gin.Context) {
	var request LoginRequest
	if !httpapi.Decode(c, &request) {
		return
	}
	result, err := h.auth.Login(c.Request.Context(), request.LoginName, request.Password, request.Persistent)
	h.respond(c, result, err, 201)
}

// @Summary Read the current session, including a restricted account
// @ID getSession
// @Produce json
// @Success 200 {object} SessionView
// @Failure 401 {object} httpapi.Problem
// @Router /api/session [get]
func (h *AuthHandler) current(c *gin.Context) {
	result, err := h.auth.Current(c.Request.Context(), cookieToken(c))
	h.respond(c, result, err, 200)
}

// @Summary Revoke the current device session
// @ID deleteSession
// @Param session_id path string true "Current session UUID"
// @Success 204
// @Failure 403 {object} httpapi.Problem
// @Router /api/sessions/{session_id} [delete]
func (h *AuthHandler) logout(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "session_id")
	if !ok {
		return
	}
	if err := h.auth.Logout(c.Request.Context(), cookieToken(c), id); err != nil {
		authProblem(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0).UTC()})
	c.Status(204)
}

// @Summary Change a password and rotate this device's session
// @ID changePassword
// @Accept json
// @Produce json
// @Param body body PasswordChange true "Password change"
// @Success 200 {object} SessionView
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/me/password [post]
func (h *AuthHandler) password(c *gin.Context) {
	var request PasswordChange
	if !httpapi.Decode(c, &request) {
		return
	}
	result, err := h.auth.ChangePassword(c.Request.Context(), cookieToken(c), request.ExpectedCredentialRevision, request.CurrentPassword, request.NewPassword, request.ConfirmPassword)
	h.respond(c, result, err, 200)
}

// @Summary Check login availability before registration
// @ID getLoginAvailability
// @Param login_name query string true "ASCII login name"
// @Produce json
// @Success 200 {object} AvailabilityView
// @Failure 422 {object} httpapi.Problem
// @Router /api/accounts/availability [get]
func (h *AuthHandler) availability(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	available, err := h.auth.Available(c.Request.Context(), c.Query("login_name"))
	if err != nil {
		authProblem(c, err)
		return
	}
	c.JSON(200, AvailabilityView{Available: available})
}
func cookieToken(c *gin.Context) string { token, _ := c.Cookie(sessionCookie); return token }
func (h *AuthHandler) respond(c *gin.Context, result application.AuthResult, err error, status int) {
	c.Header("Cache-Control", "no-store")
	if err != nil {
		authProblem(c, err)
		return
	}
	if result.Token != "" {
		cookie := &http.Cookie{Name: sessionCookie, Value: result.Token, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode}
		if result.Session.Persistent {
			cookie.Expires = result.Session.AbsoluteExpiresAt
		}
		http.SetCookie(c.Writer, cookie)
	}
	role := "creator"
	if result.User.Role == domain.RoleAdmin {
		role = "admin"
	}
	c.JSON(status, SessionView{Actor: Actor{ID: result.User.ID.String(), LoginName: result.User.LoginName, DisplayName: result.User.DisplayName, Role: role, MustChangePassword: result.User.MustChangePassword, CredentialRevision: result.User.SessionEpoch}, SessionID: result.Session.ID.String(), LastActiveAt: result.Session.LastActiveAt, AbsoluteExpiresAt: result.Session.AbsoluteExpiresAt, Persistent: result.Session.Persistent})
}
func authProblem(c *gin.Context, err error) {
	c.Header("Cache-Control", "no-store")
	status, code := 503, "dependency_unavailable"
	switch {
	case errors.Is(err, application.ErrInvalidCredentials):
		status, code = 401, "invalid_credentials"
	case errors.Is(err, application.ErrSessionInvalid):
		status, code = 401, "session_invalid"
	case errors.Is(err, application.ErrLoginTaken):
		status, code = 422, "login_name_taken"
	case errors.Is(err, application.ErrCurrentPassword):
		status, code = 422, "current_password_invalid"
	case errors.Is(err, application.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, domain.ErrWeakPassword):
		status, code = 422, "weak_password"
	case errors.Is(err, domain.ErrPasswordReused):
		status, code = 422, "password_reused"
	case errors.Is(err, domain.ErrRevisionConflict):
		status, code = 409, "credential_conflict"
	case errors.Is(err, domain.ErrInvalidProfile):
		status, code = 422, "invalid_request"
	}
	httpapi.WriteProblem(c, status, code, nil)
}
