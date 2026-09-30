// Package http adapts identity commands to the public cookie-authenticated API.
package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// OrganizationReader resolves the single active MVP organization server-side.
type OrganizationReader interface {
	FindBootstrapOrganization(context.Context) (uuid.UUID, error)
}

// Handler owns public identity commands and their session-cookie lifecycle.
type Handler struct {
	login         *application.LoginCommand
	logout        *application.LogoutCommand
	change        *application.ChangePasswordCommand
	auth          *application.Authenticator
	organizations OrganizationReader
	secure        bool
	maxAge        int
}

// NewHandler explicitly injects commands, auth and browser cookie policy.
func NewHandler(login *application.LoginCommand, logout *application.LogoutCommand, change *application.ChangePasswordCommand, auth *application.Authenticator, organizations OrganizationReader, secure bool, ttl time.Duration) *Handler {
	return &Handler{login: login, logout: logout, change: change, auth: auth, organizations: organizations, secure: secure, maxAge: int(ttl.Seconds())}
}

// LoginRequest never becomes an audit or replay payload.
type LoginRequest struct {
	LoginName string `json:"login_name"`
	Password  string `json:"password"`
}

// UserResponse is the safe public principal projection.
type UserResponse struct {
	ID                 uuid.UUID `json:"id"`
	OrgID              uuid.UUID `json:"org_id"`
	LoginName          string    `json:"login_name"`
	DisplayName        string    `json:"display_name"`
	Role               string    `json:"role"`
	MustChangePassword bool      `json:"must_change_password"`
}

// SessionResponse contains no session token; it is sent only in an HttpOnly cookie.
type SessionResponse struct {
	User               UserResponse `json:"user"`
	MustChangePassword bool         `json:"must_change_password"`
}

// PasswordRequest updates the current user's password.
type PasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// PasswordResponse confirms the new account revision.
type PasswordResponse struct {
	MustChangePassword bool  `json:"must_change_password"`
	Revision           int64 `json:"revision"`
}

// Register installs login and current-user/session mutation routes.
func (h *Handler) Register(group *gin.RouterGroup) {
	group.POST("/auth/login", h.Login)
	group.GET("/auth/me", h.RequireSession(), h.Me)
	group.POST("/auth/logout", h.Logout)
	group.POST("/auth/password", h.RequireSession(), h.ChangePassword)
}

// RequireSession checks Redis and current durable account state for every request.
func (h *Handler) RequireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		a, err := h.auth.Authenticate(c.Request.Context(), httpapi.Token(c))
		if err != nil {
			writeError(c, err)
			return
		}
		c.Set("principal", a)
		c.Next()
	}
}

// Principal returns the middleware-authenticated account to business adapters.
func Principal(c *gin.Context) application.Principal {
	a, _ := c.Get("principal")
	p, _ := a.(application.Principal)
	return p
}

// Login starts a fresh opaque session.
// @Summary 密码登录
// @Tags auth
// @ID login
// @Accept json
// @Produce json
// @Param body body LoginRequest true "登录请求"
// @Success 200 {object} SessionResponse
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 429 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var input LoginRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	org, err := h.organizations.FindBootstrapOrganization(c.Request.Context())
	if err != nil || org == uuid.Nil {
		httpapi.WriteProblem(c, 503, "authentication_unavailable", nil)
		return
	}
	result, err := h.login.Execute(c.Request.Context(), application.LoginInput{OrgID: org, LoginName: input.LoginName, Password: input.Password, ClientIP: c.ClientIP(), RequestID: c.GetString("request_id")})
	if result.RetryAfter > 0 {
		c.Header("Retry-After", strconv.FormatInt(max(int64(result.RetryAfter.Seconds()), 1), 10))
	}
	if err != nil {
		writeError(c, err)
		return
	}
	h.cookies(c, result.Token, h.maxAge)
	c.JSON(200, session(result.User))
}

// Me returns the current account.
// @Summary 当前账号
// @Tags auth
// @ID currentUser
// @Produce json
// @Success 200 {object} SessionResponse
// @Failure 401 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/auth/me [get]
func (h *Handler) Me(c *gin.Context) { c.JSON(200, session(Principal(c))) }

// Logout revokes the current session before clearing its cookies.
// @Summary 登出
// @Tags auth
// @ID logout
// @Param Idempotency-Key header string true "UUID"
// @Param X-CSRF-Token header string true "会话CSRF令牌"
// @Success 204
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	result, err := h.logout.Execute(c.Request.Context(), application.LogoutInput{Token: httpapi.Token(c), RequestID: c.GetString("request_id"), ClientIP: c.ClientIP()})
	if result.Revoked || errors.Is(err, application.ErrUnauthenticated) {
		h.cookies(c, "", -1)
	}
	if errors.Is(err, application.ErrUnauthenticated) {
		c.Status(204)
		return
	}
	if err != nil {
		writeError(c, err)
		return
	}
	c.Status(204)
}

// ChangePassword rotates this session after a durable password change.
// @Summary 修改本人密码
// @Tags auth
// @ID changePassword
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "UUID"
// @Param X-CSRF-Token header string true "会话CSRF令牌"
// @Param body body PasswordRequest true "当前与新密码"
// @Success 200 {object} PasswordResponse
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/auth/password [post]
func (h *Handler) ChangePassword(c *gin.Context) {
	var input PasswordRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	result, err := h.change.Execute(c.Request.Context(), application.ChangePasswordInput{Token: httpapi.Token(c), CurrentPassword: input.CurrentPassword, NewPassword: input.NewPassword, RequestID: c.GetString("request_id"), ClientIP: c.ClientIP()})
	if err != nil {
		writeError(c, err)
		return
	}
	h.cookies(c, result.Token, h.maxAge)
	c.JSON(200, PasswordResponse{MustChangePassword: result.MustChangePassword, Revision: result.Revision})
}
func (h *Handler) cookies(c *gin.Context, token string, age int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("lv_session", token, age, "/", "", h.secure, true)
	csrf := ""
	if token != "" {
		csrf = httpapi.CSRFToken(token)
	}
	c.SetCookie("lv_csrf", csrf, age, "/", "", h.secure, false)
}
func session(a application.Principal) SessionResponse {
	return SessionResponse{User: UserResponse{ID: a.ID, OrgID: a.OrgID, LoginName: a.LoginName, DisplayName: a.DisplayName, Role: string(a.Role), MustChangePassword: a.MustChangePassword}, MustChangePassword: a.MustChangePassword}
}
func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, application.ErrUnauthenticated), errors.Is(err, application.ErrInvalidCredentials), errors.Is(err, application.ErrCurrentPasswordInvalid):
		httpapi.WriteProblem(c, 401, "unauthenticated", nil)
	case errors.Is(err, application.ErrLoginRateLimited), errors.Is(err, domain.ErrAccountLocked):
		httpapi.WriteProblem(c, 429, "login_locked", nil)
	case errors.Is(err, application.ErrInvalidLoginInput), errors.Is(err, application.ErrInvalidChangePassword), errors.Is(err, domain.ErrWeakPassword), errors.Is(err, domain.ErrPasswordReused):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "authentication_unavailable", nil)
	}
}
