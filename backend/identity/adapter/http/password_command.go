package http

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// RegisterPreparedCredentialChanges installs the source-owned self-password
// command. Original-proof recovery deliberately precedes live authentication.
func (h *EmailAuthHandler) RegisterPreparedCredentialChanges(api *gin.RouterGroup) {
	routes := api.Group("", func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() }, httpapi.Middleware(h.origin))
	routes.POST("/me/password", h.passwordCommand)
	routes.POST("/password-resets", h.resetStart)
	routes.POST("/password-resets/complete", h.resetComplete)
}
func (h *EmailAuthHandler) passwordCommand(c *gin.Context) {
	var input EmailPasswordChangeRequest
	if !httpapi.Decode(c, &input) {
		return
	}
	epoch, err := strconv.ParseInt(input.Epoch, 10, 64)
	if err != nil || epoch < 1 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	id, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || id == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	credential, ok := h.originalCredential(c)
	if !ok {
		return
	}
	result, err := h.auth.ChangePasswordCommand(c.Request.Context(), application.PasswordChangeInput{RequestID: id, Credential: credential, CredentialEpoch: epoch, CurrentPassword: input.Current, NewPassword: input.New, Confirmation: input.Confirmation})
	h.respond(c, result, err, 200)
}

func (h *EmailAuthHandler) originalCredential(c *gin.Context) (application.SessionCredential, bool) {
	cookies, err := h.sessionCookies(c)
	if err != nil {
		emailAuthProblem(c, err)
		return application.SessionCredential{}, false
	}
	credential := application.SessionCredential{CSRFToken: c.GetHeader("X-CSRF-Token")}
	_, accessPresent := cookies[sessionCookieName(h.secure, "access")]
	_, refreshPresent := cookies[sessionCookieName(h.secure, "refresh")]
	_, csrfPresent := cookies[sessionCookieName(h.secure, "csrf")]
	if accessPresent || refreshPresent || csrfPresent {
		if !h.refreshCSRF(c) {
			return application.SessionCredential{}, false
		}
		credential.AccessToken = cookies[sessionCookieName(h.secure, "access")]
		if credential.AccessToken == "" {
			emailAuthProblem(c, application.ErrAccessExpired)
			return application.SessionCredential{}, false
		}
	} else {
		credential.LegacyToken = cookies[sessionCookie]
		if credential.LegacyToken == "" {
			emailAuthProblem(c, application.ErrSessionInvalid)
			return application.SessionCredential{}, false
		}
	}
	return credential, true
}
