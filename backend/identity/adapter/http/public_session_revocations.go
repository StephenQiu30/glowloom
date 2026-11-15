package http

import (
	"context"
	"crypto/subtle"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// SessionRevocationCommands accepts only original, bounded public recovery intent.
// Its result is final completion, not a locally revoked or currently invalid parent.
type SessionRevocationCommands interface {
	Revoke(context.Context, application.SessionRevocationCredentials, uuid.UUID, uuid.UUID) (bool, error)
	RevokeOthers(context.Context, application.SessionRevocationCredentials, uuid.UUID) error
}

// WithPublicSessionRevocations installs all public mutation routes before serving.
// The caller must register the directory's reads only; mutation retries cannot sit
// behind RequireSession, which correctly rejects their already-revoked credential.
func (h *EmailAuthHandler) WithPublicSessionRevocations(commands SessionRevocationCommands) *EmailAuthHandler {
	h.publicRevocations = commands
	return h
}

func (h *EmailAuthHandler) revocationCredentials(c *gin.Context) (application.SessionRevocationCredentials, bool) {
	cookies, err := h.sessionCookies(c)
	if err != nil {
		emailAuthProblem(c, err)
		return application.SessionRevocationCredentials{}, false
	}
	if h.origin == "" || c.GetHeader("Origin") != h.origin {
		httpapi.WriteProblem(c, 403, "origin_forbidden", nil)
		return application.SessionRevocationCredentials{}, false
	}
	csrf := c.GetHeader("X-CSRF-Token")
	if csrf == "" {
		httpapi.WriteProblem(c, 403, "csrf_invalid", nil)
		return application.SessionRevocationCredentials{}, false
	}
	if legacy, present := cookies[sessionCookie]; present {
		if legacy == "" {
			emailAuthProblem(c, application.ErrSessionInvalid)
			return application.SessionRevocationCredentials{}, false
		}
		// Legacy has no double-submit CSRF cookie. The immutable proof store checks
		// this header against its original accepted session hash on every retry.
		return application.SessionRevocationCredentials{LegacyToken: legacy, CSRFToken: csrf}, true
	}
	cookieCSRF := cookies[sessionCookieName(h.secure, "csrf")]
	if cookieCSRF == "" || subtle.ConstantTimeCompare([]byte(cookieCSRF), []byte(csrf)) != 1 {
		httpapi.WriteProblem(c, 403, "csrf_invalid", nil)
		return application.SessionRevocationCredentials{}, false
	}
	access, refresh := cookies[sessionCookieName(h.secure, "access")], cookies[sessionCookieName(h.secure, "refresh")]
	if access == "" && refresh == "" {
		emailAuthProblem(c, application.ErrSessionInvalid)
		return application.SessionRevocationCredentials{}, false
	}
	return application.SessionRevocationCredentials{AccessToken: access, RefreshToken: refresh, CSRFToken: csrf}, true
}

func publicRevocationKey(c *gin.Context) (uuid.UUID, bool) {
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return uuid.Nil, false
	}
	return key, true
}

func (h *EmailAuthHandler) revokeSessionPublic(c *gin.Context) {
	target, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	credentials, ok := h.revocationCredentials(c)
	if !ok {
		return
	}
	key, ok := publicRevocationKey(c)
	if !ok {
		return
	}
	clearCookies, err := h.publicRevocations.Revoke(c.Request.Context(), credentials, key, target)
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	if clearCookies {
		clearSessionCookies(c, h.secure)
	}
	c.Status(204)
}

func (h *EmailAuthHandler) revokeOtherSessionsPublic(c *gin.Context) {
	credentials, ok := h.revocationCredentials(c)
	if !ok {
		return
	}
	key, ok := publicRevocationKey(c)
	if !ok {
		return
	}
	if err := h.publicRevocations.RevokeOthers(c.Request.Context(), credentials, key); err != nil {
		emailAuthProblem(c, err)
		return
	}
	c.Status(204)
}
