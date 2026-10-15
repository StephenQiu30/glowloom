package http

import (
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// RegistrationStartRequest is the typed email authentication HTTP contract.
type RegistrationStartRequest struct {
	Email      string `json:"email" binding:"required"`
	Invitation string `json:"invitation_token" binding:"required"`
}

// RegistrationVerifyRequest is the typed email authentication HTTP contract.
type RegistrationVerifyRequest struct {
	Code string `json:"code" binding:"required"`
}

// RegistrationVerifyView is the typed email authentication HTTP contract.
type RegistrationVerifyView struct {
	Outcome string `json:"outcome" binding:"required"`
	Grant   string `json:"registration_grant" binding:"required"`
}

// RegistrationCompleteRequest is the typed email authentication HTTP contract.
type RegistrationCompleteRequest struct {
	Grant         string `json:"registration_grant" binding:"required"`
	DisplayName   string `json:"display_name" binding:"required"`
	Password      string `json:"password" binding:"required"`
	Confirmation  string `json:"confirm_password" binding:"required"`
	TermsVersion  string `json:"terms_version" binding:"required"`
	TermsAccepted bool   `json:"terms_accepted" binding:"required"`
}

// RegistrationCompleteView is the typed email authentication HTTP contract.
type RegistrationCompleteView struct {
	AccountID uuid.UUID `json:"account_id" binding:"required"`
	Next      string    `json:"next" binding:"required"`
}

// EmailLoginRequest is the typed email authentication HTTP contract.
type EmailLoginRequest struct {
	Email      string `json:"email" binding:"required"`
	Password   string `json:"password" binding:"required"`
	Persistent bool   `json:"persistent" binding:"required"`
}

// PasswordResetStartRequest is the typed email authentication HTTP contract.
type PasswordResetStartRequest struct {
	Email string `json:"email" binding:"required"`
}

// PasswordResetCompleteRequest is the typed email authentication HTTP contract.
type PasswordResetCompleteRequest struct {
	Token        string `json:"token" binding:"required"`
	Password     string `json:"password" binding:"required"`
	Confirmation string `json:"confirm_password" binding:"required"`
}

// ReauthenticationRequest is the typed email authentication HTTP contract.
type ReauthenticationRequest struct {
	Password string `json:"password" binding:"required"`
}

// EmailPasswordChangeRequest is the typed email authentication HTTP contract.
type EmailPasswordChangeRequest struct {
	Epoch        string `json:"expected_credential_epoch" binding:"required"`
	Current      string `json:"current_password" binding:"required"`
	New          string `json:"new_password" binding:"required"`
	Confirmation string `json:"confirm_password" binding:"required"`
}

const registrationFlowCookie = "glowloom_registration_flow"

// MeView exposes only the signed-in account profile and capabilities.
type MeView struct {
	ID              uuid.UUID  `json:"id" binding:"required"`
	Email           string     `json:"email" binding:"required"`
	EmailVerifiedAt time.Time  `json:"email_verified_at" binding:"required"`
	DisplayName     string     `json:"display_name" binding:"required"`
	AvatarAssetID   *uuid.UUID `json:"avatar_asset_id" binding:"required"`
	PlatformRole    string     `json:"platform_role" binding:"required"`
	Status          string     `json:"status" binding:"required"`
	Revision        string     `json:"revision" binding:"required"`
	Capabilities    []string   `json:"capabilities" binding:"required"`
}

// EmailSessionView returns the session metadata and session-bound CSRF token.
type EmailSessionView struct {
	Actor             MeView    `json:"actor" binding:"required"`
	SessionID         uuid.UUID `json:"session_id" binding:"required"`
	LastActiveAt      time.Time `json:"last_active_at" binding:"required"`
	AbsoluteExpiresAt time.Time `json:"absolute_expires_at" binding:"required"`
	Persistent        bool      `json:"persistent" binding:"required"`
	CredentialEpoch   string    `json:"credential_epoch" binding:"required"`
	CSRFToken         string    `json:"csrf_token" binding:"required"`
}

// AcceptedView acknowledges a queued operation without claiming mail delivery.
type AcceptedView struct {
	OperationID       uuid.UUID `json:"operation_id" binding:"required"`
	RetryAfterSeconds int       `json:"retry_after_seconds" binding:"required"`
}

// EmailAuthHandler registers only email authentication. App cutover must also
// install scope authorization before exposing these routes to public traffic.
type EmailAuthHandler struct {
	auth   *application.Auth
	origin string
	secure bool
	revoke gin.HandlerFunc
}

// NewEmailAuthHandler constructs EmailAuthHandler with explicit caller-supplied dependencies.
func NewEmailAuthHandler(auth *application.Auth, origin string, secure bool) *EmailAuthHandler {
	return &EmailAuthHandler{auth: auth, origin: origin, secure: secure}
}

// Register installs email authentication and recovery endpoints.
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
	routes.POST("/me/reauthentication", h.authenticated(), h.reauthenticate)
	routes.POST("/me/password", h.authenticated(), h.password)
	routes.DELETE("/sessions/:id", h.authenticated(), h.revokeSession)
}

// WithSessionRevocation installs the self-owned session directory revocation adapter.
func (h *EmailAuthHandler) WithSessionRevocation(revoke gin.HandlerFunc) *EmailAuthHandler {
	h.revoke = revoke
	return h
}
func (h *EmailAuthHandler) revokeSession(c *gin.Context) {
	if h.revoke != nil {
		h.revoke(c)
		return
	}
	h.logout(c)
}

// RequireSession protects all business writes with the session-bound CSRF proof.
// Business handlers must independently resolve the explicit creation scope.
func (h *EmailAuthHandler) RequireSession() gin.HandlerFunc { return h.authenticated() }
func (h *EmailAuthHandler) authenticated() gin.HandlerFunc {
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
		if strings.HasPrefix(c.FullPath(), "/api/platform/") && u.PlatformRole != "platform_admin" {
			httpapi.WriteProblem(c, 403, "forbidden", nil)
			return
		}
		// OrgID is the server-resolved personal storage coordinate, never authority.
		// Content middleware replaces it with the explicitly authorized creation scope.
		c.Set("principal", application.Principal{ID: u.ID, OrgID: u.OrgID, SessionID: result.Session.ID, CredentialEpoch: u.SessionEpoch, PlatformEpoch: u.PlatformEpoch, PlatformRole: u.PlatformRole, LoginName: u.LoginName, DisplayName: u.DisplayName})
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

// @Summary startRegistration
// @ID startRegistration
// @Tags identity
// @Accept json
// @Produce json
// @Param body body RegistrationStartRequest true "Request"
// @Success 202 {object} AcceptedView
// @Failure 422 {object} httpapi.Problem
// @Router /api/registrations [post]
func (h *EmailAuthHandler) start(c *gin.Context) {
	var input RegistrationStartRequest
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

// @Summary resendRegistration
// @ID resendRegistration
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Operation UUID"
// @Success 202 {object} AcceptedView
// @Failure 422 {object} httpapi.Problem
// @Router /api/registrations/{id}/resend [post]
func (h *EmailAuthHandler) resend(c *gin.Context) {
	id, flow, ok := registrationFlow(c)
	if !ok {
		return
	}
	r, err := h.auth.ResendRegistrationFrom(c.Request.Context(), id, flow, requestSource(c))
	h.accepted(c, r, err)
}

// @Summary verifyRegistration
// @ID verifyRegistration
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Operation UUID"
// @Param body body RegistrationVerifyRequest true "Request"
// @Success 200 {object} RegistrationVerifyView
// @Failure 422 {object} httpapi.Problem
// @Router /api/registrations/{id}/verify [post]
func (h *EmailAuthHandler) verify(c *gin.Context) {
	id, flow, ok := registrationFlow(c)
	if !ok {
		return
	}
	var input RegistrationVerifyRequest
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
	c.JSON(200, RegistrationVerifyView{r.Outcome, r.Grant})
}

// @Summary completeRegistration
// @ID completeRegistration
// @Tags identity
// @Accept json
// @Produce json
// @Param id path string true "Operation UUID"
// @Param body body RegistrationCompleteRequest true "Request"
// @Success 201 {object} RegistrationCompleteView
// @Failure 422 {object} httpapi.Problem
// @Router /api/registrations/{id}/complete [post]
func (h *EmailAuthHandler) complete(c *gin.Context) {
	id, flow, ok := registrationFlow(c)
	if !ok {
		return
	}
	var input RegistrationCompleteRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	account, err := h.auth.CompleteRegistration(c.Request.Context(), application.CompleteRegistrationInput{OperationID: id, FlowSecret: flow, Grant: input.Grant, DisplayName: input.DisplayName, Password: input.Password, ConfirmPassword: input.Confirmation, TermsVersion: input.TermsVersion, TermsAccepted: input.TermsAccepted})
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: registrationFlowCookie, Path: "/api/registrations", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	c.JSON(201, RegistrationCompleteView{account, "login"})
}

// @Summary createSession
// @ID createSession
// @Tags identity
// @Accept json
// @Produce json
// @Param body body EmailLoginRequest true "Request"
// @Success 201 {object} EmailSessionView
// @Failure 401 {object} httpapi.Problem
// @Failure 429 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/sessions [post]
func (h *EmailAuthHandler) login(c *gin.Context) {
	var input EmailLoginRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	r, err := h.auth.LoginFrom(c.Request.Context(), input.Email, input.Password, input.Persistent, requestSource(c))
	h.respond(c, r, err, 201)
}

// @Summary getSession
// @ID getSession
// @Tags identity
// @Accept json
// @Produce json
// @Success 200 {object} EmailSessionView
// @Failure 422 {object} httpapi.Problem
// @Router /api/session [get]
func (h *EmailAuthHandler) current(c *gin.Context) {
	r, err := h.auth.Current(c.Request.Context(), cookieToken(c))
	h.respond(c, r, err, 200)
}

// @Summary startPasswordReset
// @ID startPasswordReset
// @Tags identity
// @Accept json
// @Produce json
// @Param body body PasswordResetStartRequest true "Request"
// @Success 202 {object} AcceptedView
// @Failure 422 {object} httpapi.Problem
// @Router /api/password-resets [post]
func (h *EmailAuthHandler) resetStart(c *gin.Context) {
	var input PasswordResetStartRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	r, err := h.auth.StartPasswordReset(c.Request.Context(), input.Email, requestSource(c))
	h.accepted(c, r, err)
}

// @Summary completePasswordReset
// @ID completePasswordReset
// @Tags identity
// @Accept json
// @Produce json
// @Param body body PasswordResetCompleteRequest true "Request"
// @Success 204
// @Failure 422 {object} httpapi.Problem
// @Router /api/password-resets/complete [post]
func (h *EmailAuthHandler) resetComplete(c *gin.Context) {
	var input PasswordResetCompleteRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	if err := h.auth.CompletePasswordReset(c.Request.Context(), input.Token, input.Password, input.Confirmation); err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.Status(204)
}

// @Summary reauthenticate
// @ID reauthenticate
// @Tags identity
// @Accept json
// @Produce json
// @Param body body ReauthenticationRequest true "Request"
// @Success 204
// @Failure 422 {object} httpapi.Problem
// @Router /api/me/reauthentication [post]
func (h *EmailAuthHandler) reauthenticate(c *gin.Context) {
	var input ReauthenticationRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	if err := h.auth.Reauthenticate(c.Request.Context(), cookieToken(c), input.Password); err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.Status(204)
}

// @Summary changePassword
// @ID changePassword
// @Tags identity
// @Accept json
// @Produce json
// @Param body body EmailPasswordChangeRequest true "Request"
// @Success 200 {object} EmailSessionView
// @Failure 422 {object} httpapi.Problem
// @Router /api/me/password [post]
func (h *EmailAuthHandler) password(c *gin.Context) {
	var input EmailPasswordChangeRequest
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
	capabilities := []string{"profile:write"}
	if u.PlatformRole == "platform_admin" {
		capabilities = append(capabilities, "platform:manage")
	}
	c.JSON(status, EmailSessionView{Actor: MeView{ID: u.ID, Email: u.Email, EmailVerifiedAt: u.EmailVerifiedAt, DisplayName: u.DisplayName, AvatarAssetID: u.AvatarAssetID, PlatformRole: u.PlatformRole, Status: string(u.Status), Revision: strconv.FormatInt(u.Revision, 10), Capabilities: capabilities}, SessionID: r.Session.ID, LastActiveAt: r.Session.LastActiveAt, AbsoluteExpiresAt: r.Session.AbsoluteExpiresAt, Persistent: r.Session.Persistent, CredentialEpoch: strconv.FormatInt(u.SessionEpoch, 10), CSRFToken: r.CSRFToken})
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
	case errors.Is(err, application.ErrReauthenticationRequired):
		status, code = 403, "reauthentication_required"
	case errors.Is(err, application.ErrDeletionDuties):
		status, code = 409, "administrator_transfer_required"
	case errors.Is(err, application.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, domain.ErrRevisionConflict):
		status, code = 409, "revision_conflict"
	case errors.Is(err, domain.ErrInvalidEmail), errors.Is(err, domain.ErrInvalidProfile), errors.Is(err, domain.ErrWeakPassword), errors.Is(err, domain.ErrPasswordReused), errors.Is(err, application.ErrCurrentPassword):
		status, code = 422, "invalid_request"
	}
	httpapi.WriteProblem(c, status, code, nil)
}
