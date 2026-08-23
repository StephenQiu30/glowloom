package identity

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
	"github.com/stephenqiu30/lanverse/backend/src/platform/toolkit"
)

type IdentityController struct {
	service *IdentityService
}

func NewIdentityController(service *IdentityService) *IdentityController {
	return &IdentityController{service: service}
}

func (h *IdentityController) Mount(router chi.Router) {
	router.Route("/api/auth", func(router chi.Router) {
		router.Post("/register", h.register)
		router.Post("/login", h.login)
		router.Post("/refresh", h.refresh)
		router.Post("/logout", h.logout)
		router.Get("/me", h.me)
	})
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Workspace   string `json:"workspace_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// register 使用邮箱创建账户、Workspace 和 Admin Membership。
// @Summary 邮箱注册
// @Tags auth
// @ID auth_register
// @Accept json
// @Produce json
// @Param request body registerRequest true "注册参数"
// @Success 201 {object} AuthResponseEnvelope
// @Failure 409 {object} httpapi.ErrorEnvelope
// @Failure 422 {object} httpapi.ErrorEnvelope
// @Router /api/auth/register [post]
func (h *IdentityController) register(w http.ResponseWriter, r *http.Request) {
	var body registerRequest
	if !httpapi.DecodeJSON(w, r, &body, 64<<10) {
		return
	}
	response, issue, err := h.service.Register(r.Context(), RegisterInput{Email: body.Email, Password: body.Password, DisplayName: body.DisplayName, Workspace: body.Workspace}, remoteIP(r))
	if err != nil {
		httpapi.WriteError(w, r, err)
		return
	}
	h.setRefreshCookie(w, issue.RefreshToken, issue.RefreshExpiresAt)
	httpapi.WriteData(w, httpapi.StatusCreated, response)
}

// login 使用邮箱和密码创建登录会话，Workspace 由账号的有效成员关系自动确定。
// @Summary 邮箱登录
// @Tags auth
// @ID auth_login
// @Accept json
// @Produce json
// @Param request body loginRequest true "登录参数"
// @Success 200 {object} AuthResponseEnvelope
// @Failure 401 {object} httpapi.ErrorEnvelope
// @Failure 422 {object} httpapi.ErrorEnvelope
// @Router /api/auth/login [post]
func (h *IdentityController) login(w http.ResponseWriter, r *http.Request) {
	var body loginRequest
	if !httpapi.DecodeJSON(w, r, &body, 64<<10) {
		return
	}
	response, issue, err := h.service.Login(r.Context(), body.Email, body.Password, remoteIP(r))
	if err != nil {
		httpapi.WriteError(w, r, err)
		return
	}
	h.setRefreshCookie(w, issue.RefreshToken, issue.RefreshExpiresAt)
	httpapi.WriteData(w, httpapi.StatusOK, response)
}

// refresh 轮换 HttpOnly refresh cookie 并签发新的短期 JWT。
// @Summary 无感刷新登录会话
// @Tags auth
// @ID auth_refresh
// @Produce json
// @Security RefreshCookie
// @Success 200 {object} AuthResponseEnvelope
// @Failure 401 {object} httpapi.ErrorEnvelope
// @Router /api/auth/refresh [post]
func (h *IdentityController) refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(h.service.config.RefreshCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "刷新会话缺失", "重新登录后重试"))
		return
	}
	response, issue, err := h.service.Refresh(r.Context(), cookie.Value, remoteIP(r))
	if err != nil {
		httpapi.WriteError(w, r, err)
		return
	}
	h.setRefreshCookie(w, issue.RefreshToken, issue.RefreshExpiresAt)
	httpapi.WriteData(w, httpapi.StatusOK, response)
}

// logout 撤销当前 refresh session 并清理 cookie。
// @Summary 退出登录
// @Tags auth
// @ID auth_logout
// @Success 204
// @Router /api/auth/logout [post]
func (h *IdentityController) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(h.service.config.RefreshCookieName); err == nil {
		if err := h.service.Logout(r.Context(), cookie.Value); err != nil {
			httpapi.WriteError(w, r, err)
			return
		}
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// me 返回当前 JWT 对应的有效身份。
// @Summary 获取当前身份
// @Tags auth
// @ID auth_me
// @Produce json
// @Security BearerAccessToken
// @Success 200 {object} CurrentIdentityEnvelope
// @Failure 401 {object} httpapi.ErrorEnvelope
// @Router /api/auth/me [get]
func (h *IdentityController) me(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "登录会话缺失", "重新登录后重试"))
		return
	}
	httpapi.WriteData(w, httpapi.StatusOK, CurrentIdentity{
		UserID:       principal.UserID,
		WorkspaceID:  principal.WorkspaceID,
		MembershipID: principal.MembershipID,
		SessionID:    principal.SessionID,
		Role:         principal.Role,
	})
}

func (h *IdentityController) setRefreshCookie(w http.ResponseWriter, value string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{Name: h.service.config.RefreshCookieName, Value: value, Path: h.service.config.RefreshCookiePath, Domain: h.service.config.RefreshCookieDomain, Expires: expiresAt.UTC(), MaxAge: maxAge(expiresAt), HttpOnly: true, Secure: h.service.config.RefreshCookieSecure, SameSite: http.SameSiteStrictMode})
}

func (h *IdentityController) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: h.service.config.RefreshCookieName, Value: "", Path: h.service.config.RefreshCookiePath, Domain: h.service.config.RefreshCookieDomain, MaxAge: -1, HttpOnly: true, Secure: h.service.config.RefreshCookieSecure, SameSite: http.SameSiteStrictMode})
}

func remoteIP(r *http.Request) string {
	return toolkit.ClientIP(r)
}

func maxAge(expiresAt time.Time) int {
	seconds := int(time.Until(expiresAt).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}
