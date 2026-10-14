package http

import (
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const registrationFlowCookie = "glowloom_registration_flow"

type MeView struct {
	ID              uuid.UUID  `json:"id"`
	Email           string     `json:"email"`
	EmailVerifiedAt time.Time  `json:"email_verified_at"`
	DisplayName     string     `json:"display_name"`
	AvatarAssetID   *uuid.UUID `json:"avatar_asset_id"`
	PlatformRole    string     `json:"platform_role"`
	Status          string     `json:"status"`
	Revision        string     `json:"revision"`
	Capabilities    []string   `json:"capabilities"`
}
type EmailSessionView struct {
	Actor              MeView    `json:"actor"`
	SessionID          uuid.UUID `json:"session_id"`
	LastActiveAt       time.Time `json:"last_active_at"`
	AbsoluteExpiresAt  time.Time `json:"absolute_expires_at"`
	Persistent         bool      `json:"persistent"`
	MustChangePassword bool      `json:"must_change_password"`
	CredentialEpoch    string    `json:"credential_epoch"`
	CSRFToken          string    `json:"csrf_token"`
}
type AcceptedView struct {
	OperationID       uuid.UUID `json:"operation_id"`
	RetryAfterSeconds int       `json:"retry_after_seconds"`
}

// EmailAuthHandler registers only email authentication. App cutover must also
// install scope authorization before exposing these routes to public traffic.
type EmailAuthHandler struct {
	auth   *application.Auth
	origin string
	secure bool
}

func NewEmailAuthHandler(auth *application.Auth, origin string, secure bool) *EmailAuthHandler {
	return &EmailAuthHandler{auth: auth, origin: origin, secure: secure}
}
func (h *EmailAuthHandler) Register(api *gin.RouterGroup) {
	routes := api.Group("", httpapi.Middleware(h.origin), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	routes.POST("/registrations", h.start)
	routes.POST("/registrations/:id/resend", h.resend)
	routes.POST("/registrations/:id/verify", h.verify)
	routes.POST("/registrations/:id/complete", h.complete)
	routes.POST("/sessions", h.login)
	routes.GET("/session", h.current)
	routes.POST("/password-resets", h.resetStart)
	routes.POST("/password-resets/complete", h.resetComplete)
	routes.POST("/me/reauthentication", h.authenticated(true), h.reauthenticate)
	routes.POST("/me/password", h.authenticated(true), h.password)
	routes.DELETE("/sessions/:id", h.authenticated(true), h.logout)
}

// RequireSession protects all business writes with the session-bound CSRF proof.
// Business handlers must independently resolve the explicit creation scope.
func (h *EmailAuthHandler) RequireSession() gin.HandlerFunc { return h.authenticated(false) }
func (h *EmailAuthHandler) authenticated(allowRestricted bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		result, err := h.auth.Current(c.Request.Context(), cookieToken(c))
		if err != nil {
			emailAuthProblem(c, err)
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
			if h.origin == "" || c.GetHeader("Origin") != h.origin || result.CSRFToken == "" || subtle.ConstantTimeCompare([]byte(c.GetHeader("X-CSRF-Token")), []byte(result.CSRFToken)) != 1 {
				httpapi.WriteProblem(c, 403, "forbidden", nil)
				return
			}
		}
		u := result.User
		if u.MustChangePassword && !allowRestricted {
			httpapi.WriteProblem(c, 403, "password_change_required", nil)
			return
		}
		if strings.HasPrefix(c.FullPath(), "/api/platform/") && u.PlatformRole != "platform_admin" {
			httpapi.WriteProblem(c, 403, "forbidden", nil)
			return
		}
		c.Set("principal", application.Principal{ID: u.ID, SessionID: result.Session.ID, CredentialEpoch: u.SessionEpoch, PlatformEpoch: u.PlatformEpoch, PlatformRole: u.PlatformRole, MustChangePassword: u.MustChangePassword, LoginName: u.LoginName, DisplayName: u.DisplayName})
		c.Set("email_session", result)
		c.Next()
	}
}
func requestSource(c *gin.Context) string {
	// Forwarded headers are not accepted as trusted ingress configuration.
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return "unknown"
	}
	return host
}
func (h *EmailAuthHandler) accepted(c *gin.Context, r application.RegistrationStart, err error) {
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.JSON(202, AcceptedView{OperationID: r.OperationID, RetryAfterSeconds: r.RetryAfterSeconds})
}
func (h *EmailAuthHandler) start(c *gin.Context) {
	var input struct {
		Email      string `json:"email"`
		Invitation string `json:"invitation_token"`
	}
	if !httpapi.Decode(c, &input) {
		return
	}
	r, err := h.auth.StartRegistrationFrom(c.Request.Context(), input.Email, input.Invitation, requestSource(c))
	if err == nil {
		http.SetCookie(c.Writer, &http.Cookie{Name: registrationFlowCookie, Value: r.OperationID.String() + "." + r.FlowSecret, Path: "/api/registrations", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: 1200})
	}
	h.accepted(c, r, err)
}
func registrationFlow(c *gin.Context) (uuid.UUID, string, bool) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return uuid.Nil, "", false
	}
	cookie, _ := c.Cookie(registrationFlowCookie)
	operation, secret, found := strings.Cut(cookie, ".")
	if !found || operation != id.String() || secret == "" {
		emailAuthProblem(c, application.ErrProofInvalid)
		return uuid.Nil, "", false
	}
	return id, secret, true
}
func (h *EmailAuthHandler) resend(c *gin.Context) {
	id, flow, ok := registrationFlow(c)
	if !ok {
		return
	}
	r, err := h.auth.ResendRegistrationFrom(c.Request.Context(), id, flow, requestSource(c))
	h.accepted(c, r, err)
}
func (h *EmailAuthHandler) verify(c *gin.Context) {
	id, flow, ok := registrationFlow(c)
	if !ok {
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if !httpapi.Decode(c, &input) {
		return
	}
	if len(input.Code) != 6 || strings.Trim(input.Code, "0123456789") != "" {
		emailAuthProblem(c, application.ErrProofInvalid)
		return
	}
	r, err := h.auth.VerifyRegistration(c.Request.Context(), id, flow, input.Code)
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.JSON(200, struct {
		Outcome string `json:"outcome"`
		Grant   string `json:"registration_grant,omitempty"`
	}{r.Outcome, r.Grant})
}
func (h *EmailAuthHandler) complete(c *gin.Context) {
	id, flow, ok := registrationFlow(c)
	if !ok {
		return
	}
	var input struct {
		Grant         string `json:"registration_grant"`
		DisplayName   string `json:"display_name"`
		Password      string `json:"password"`
		Confirmation  string `json:"confirm_password"`
		TermsVersion  string `json:"terms_version"`
		TermsAccepted bool   `json:"terms_accepted"`
	}
	if !httpapi.Decode(c, &input) {
		return
	}
	account, err := h.auth.CompleteRegistration(c.Request.Context(), application.CompleteRegistrationInput{OperationID: id, FlowSecret: flow, Grant: input.Grant, DisplayName: input.DisplayName, Password: input.Password, ConfirmPassword: input.Confirmation, TermsVersion: input.TermsVersion, TermsAccepted: input.TermsAccepted})
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: registrationFlowCookie, Path: "/api/registrations", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	c.JSON(201, struct {
		AccountID uuid.UUID `json:"account_id"`
		Next      string    `json:"next"`
	}{account, "login"})
}
func (h *EmailAuthHandler) login(c *gin.Context) {
	var input struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		Persistent bool   `json:"persistent"`
	}
	if !httpapi.Decode(c, &input) {
		return
	}
	r, err := h.auth.Login(c.Request.Context(), input.Email, input.Password, input.Persistent)
	h.respond(c, r, err, 201)
}
func (h *EmailAuthHandler) current(c *gin.Context) {
	r, err := h.auth.Current(c.Request.Context(), cookieToken(c))
	h.respond(c, r, err, 200)
}
func (h *EmailAuthHandler) resetStart(c *gin.Context) {
	var input struct {
		Email string `json:"email"`
	}
	if !httpapi.Decode(c, &input) {
		return
	}
	r, err := h.auth.StartPasswordReset(c.Request.Context(), input.Email, requestSource(c))
	h.accepted(c, r, err)
}
func (h *EmailAuthHandler) resetComplete(c *gin.Context) {
	var input struct {
		Token        string `json:"token"`
		Password     string `json:"password"`
		Confirmation string `json:"confirm_password"`
	}
	if !httpapi.Decode(c, &input) {
		return
	}
	if err := h.auth.CompletePasswordReset(c.Request.Context(), input.Token, input.Password, input.Confirmation); err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.Status(204)
}
func (h *EmailAuthHandler) reauthenticate(c *gin.Context) {
	var input struct {
		Password string `json:"password"`
	}
	if !httpapi.Decode(c, &input) {
		return
	}
	if err := h.auth.Reauthenticate(c.Request.Context(), cookieToken(c), input.Password); err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.Status(204)
}
func (h *EmailAuthHandler) password(c *gin.Context) {
	var input struct {
		Epoch        string `json:"expected_credential_epoch"`
		Current      string `json:"current_password"`
		New          string `json:"new_password"`
		Confirmation string `json:"confirm_password"`
	}
	if !httpapi.Decode(c, &input) {
		return
	}
	epoch, err := strconv.ParseInt(input.Epoch, 10, 64)
	if err != nil || epoch < 1 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	r, err := h.auth.ChangePassword(c.Request.Context(), cookieToken(c), epoch, input.Current, input.New, input.Confirmation)
	h.respond(c, r, err, 200)
}
func (h *EmailAuthHandler) logout(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.auth.Logout(c.Request.Context(), cookieToken(c), id); err != nil {
		emailAuthProblem(c, err)
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	c.Status(204)
}
func (h *EmailAuthHandler) respond(c *gin.Context, r application.AuthResult, err error, status int) {
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	if r.Token != "" {
		cookie := &http.Cookie{Name: sessionCookie, Value: r.Token, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode}
		if r.Session.Persistent {
			cookie.Expires = r.Session.AbsoluteExpiresAt
		}
		http.SetCookie(c.Writer, cookie)
	}
	u := r.User
	capabilities := []string{}
	if !u.MustChangePassword {
		capabilities = append(capabilities, "profile:write")
		if u.PlatformRole == "platform_admin" {
			capabilities = append(capabilities, "platform:manage")
		}
	}
	c.JSON(status, EmailSessionView{Actor: MeView{ID: u.ID, Email: u.Email, EmailVerifiedAt: u.EmailVerifiedAt, DisplayName: u.DisplayName, AvatarAssetID: u.AvatarAssetID, PlatformRole: u.PlatformRole, Status: string(u.Status), Revision: strconv.FormatInt(u.Revision, 10), Capabilities: capabilities}, SessionID: r.Session.ID, LastActiveAt: r.Session.LastActiveAt, AbsoluteExpiresAt: r.Session.AbsoluteExpiresAt, Persistent: r.Session.Persistent, MustChangePassword: u.MustChangePassword, CredentialEpoch: strconv.FormatInt(u.SessionEpoch, 10), CSRFToken: r.CSRFToken})
}
func emailAuthProblem(c *gin.Context, err error) {
	c.Header("Cache-Control", "no-store")
	status, code := 503, "temporarily_unavailable"
	var limited *application.RateLimit
	switch {
	case errors.As(err, &limited):
		status, code = 429, "rate_limited"
		c.Header("Retry-After", strconv.FormatInt(max(1, int64((limited.RetryAfter+time.Second-1)/time.Second)), 10))
	case errors.Is(err, application.ErrInvalidCredentials):
		status, code = 401, "invalid_credentials"
	case errors.Is(err, application.ErrSessionInvalid):
		status, code = 401, "unauthenticated"
	case errors.Is(err, application.ErrProofInvalid):
		status, code = 422, "proof_invalid"
	case errors.Is(err, application.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, domain.ErrRevisionConflict):
		status, code = 409, "revision_conflict"
	case errors.Is(err, domain.ErrInvalidEmail), errors.Is(err, domain.ErrInvalidProfile), errors.Is(err, domain.ErrWeakPassword), errors.Is(err, domain.ErrPasswordReused), errors.Is(err, application.ErrCurrentPassword):
		status, code = 422, "invalid_request"
	}
	httpapi.WriteProblem(c, status, code, nil)
}
