package http

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// SessionHTTPPolicy controls issuance presentation; existing v2 credentials remain verifiable during rollback.
type SessionHTTPPolicy struct {
	V2Enabled          bool
	LegacyUpgradeUntil time.Time
}

// WithSessionPolicy installs immutable HTTP configuration before routes are served.
func (h *EmailAuthHandler) WithSessionPolicy(policy SessionHTTPPolicy) *EmailAuthHandler {
	h.policy = policy
	return h
}
func sessionCookieName(secure bool, kind string) string {
	if secure {
		return "__Host-glowloom_" + kind
	}
	return "glowloom_" + kind
}

// A distinct transport error prevents logout from trying a refresh fallback.
var errSessionCookieConflict = errors.New("ambiguous session cookies")

func recognizedSessionCookie(name string) bool {
	switch name {
	case sessionCookie, "glowloom_access", "glowloom_refresh", "glowloom_csrf", "__Host-glowloom_access", "__Host-glowloom_refresh", "__Host-glowloom_csrf":
		return true
	default:
		return false
	}
}

// sessionCookies rejects ambiguity before any credential is consumed. Count raw
// names as well: net/http may omit a malformed second value when parsing Cookies.
func (h *EmailAuthHandler) sessionCookies(c *gin.Context) (map[string]string, error) {
	counts := map[string]int{}
	for _, header := range c.Request.Header.Values("Cookie") {
		for _, part := range strings.Split(header, ";") {
			name, _, _ := strings.Cut(strings.TrimSpace(part), "=")
			name = strings.TrimSpace(name)
			if !recognizedSessionCookie(name) {
				continue
			}
			counts[name]++
			if counts[name] > 1 {
				return nil, errSessionCookieConflict
			}
		}
	}
	values := map[string]string{}
	for _, cookie := range c.Request.Cookies() {
		if recognizedSessionCookie(cookie.Name) {
			values[cookie.Name] = cookie.Value
		}
	}
	v2 := false
	for name := range counts {
		if _, ok := values[name]; !ok {
			return nil, errSessionCookieConflict
		}
		if name == sessionCookie {
			continue
		}
		v2 = true
		if strings.HasPrefix(name, "__Host-") != h.secure {
			return nil, errSessionCookieConflict
		}
	}
	if v2 && counts[sessionCookie] > 0 {
		return nil, errSessionCookieConflict
	}
	return values, nil
}
func expireSessionCookie(c *gin.Context, name string, secure bool) {
	if strings.HasPrefix(name, "__Host-") {
		secure = true
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Path: "/", Secure: secure, HttpOnly: name != "glowloom_csrf" && name != "__Host-glowloom_csrf", SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0).UTC()})
}
func clearSessionCookies(c *gin.Context, secure bool) {
	for _, prefix := range []bool{false, true} {
		for _, kind := range []string{"access", "refresh", "csrf"} {
			expireSessionCookie(c, sessionCookieName(prefix, kind), secure)
		}
	}
	expireSessionCookie(c, sessionCookie, secure)
}

// Remove received noncanonical credentials on this host after successful
// credential issuance. Login itself does not consume the stale credentials.
func (h *EmailAuthHandler) clearReplacedSessionCookies(c *gin.Context, v2 bool) {
	cleared := map[string]bool{}
	for _, cookie := range c.Request.Cookies() {
		if !recognizedSessionCookie(cookie.Name) || cleared[cookie.Name] {
			continue
		}
		stale := cookie.Name == sessionCookie
		if !v2 {
			stale = cookie.Name != sessionCookie
		} else {
			for _, kind := range []string{"access", "refresh", "csrf"} {
				if cookie.Name == sessionCookieName(!h.secure, kind) {
					stale = true
				}
			}
		}
		if stale {
			expireSessionCookie(c, cookie.Name, h.secure)
			cleared[cookie.Name] = true
		}
	}
}
func optionalTime(v time.Time) *time.Time {
	if v.IsZero() {
		return nil
	}
	return &v
}
func sessionProtocol(s domain.AuthSession) string {
	if s.ProtocolVersion == 2 {
		return "v2"
	}
	return "v1"
}
func sessionIdleDeadline(s domain.AuthSession) time.Time {
	return s.IdleExpiresAt()
}
func (h *EmailAuthHandler) writeSessionCookies(c *gin.Context, r application.AuthResult) {
	end := r.Session.AbsoluteExpiresAt
	for _, item := range []struct {
		kind, value string
		end         time.Time
	}{{"access", r.AccessToken, r.AccessExpiresAt}, {"refresh", r.RefreshToken, end}, {"csrf", r.CSRFToken, end}} {
		cookie := &http.Cookie{Name: sessionCookieName(h.secure, item.kind), Value: item.value, Path: "/", Secure: h.secure, HttpOnly: item.kind != "csrf", SameSite: http.SameSiteLaxMode}
		if r.Session.Persistent {
			cookie.Expires = item.end
			// A recovered receipt retains its original ServerTime. Relative Max-Age
			// would override Expires and extend retention on recovery or delivery delay.
		}
		http.SetCookie(c.Writer, cookie)
	}
}
func (h *EmailAuthHandler) authenticateRequest(c *gin.Context, activity application.SessionActivity) (application.AuthResult, error) {
	cookies, err := h.sessionCookies(c)
	if err != nil {
		return application.AuthResult{}, err
	}
	access, ae := cookies[sessionCookieName(h.secure, "access")]
	refresh, re := cookies[sessionCookieName(h.secure, "refresh")]
	csrf, ce := cookies[sessionCookieName(h.secure, "csrf")]
	// Presence selects protocol; foreign or conflicting namespaces were rejected.
	if ae || re || ce {
		if access == "" {
			if refresh != "" {
				return application.AuthResult{}, application.ErrAccessExpired
			}
			return application.AuthResult{}, application.ErrSessionInvalid
		}
		return h.auth.CurrentAccess(c.Request.Context(), access, csrf, activity)
	}
	legacy := cookies[sessionCookie]
	if legacy == "" {
		return application.AuthResult{}, application.ErrSessionInvalid
	}
	return h.auth.CurrentWithActivity(c.Request.Context(), legacy, activity)
}
func (h *EmailAuthHandler) authorizeWrite(c *gin.Context, r application.AuthResult) bool {
	switch c.Request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	if h.origin == "" || c.GetHeader("Origin") != h.origin {
		httpapi.WriteProblem(c, 403, "origin_forbidden", nil)
		return false
	}
	if r.CSRFToken == "" || subtle.ConstantTimeCompare([]byte(c.GetHeader("X-CSRF-Token")), []byte(r.CSRFToken)) != 1 {
		httpapi.WriteProblem(c, 403, "csrf_invalid", nil)
		return false
	}
	return true
}
func (h *EmailAuthHandler) refreshCSRF(c *gin.Context) bool {
	cookies, err := h.sessionCookies(c)
	if err != nil {
		emailAuthProblem(c, err)
		return false
	}
	csrf := cookies[sessionCookieName(h.secure, "csrf")]
	if h.origin == "" || c.GetHeader("Origin") != h.origin {
		httpapi.WriteProblem(c, 403, "origin_forbidden", nil)
		return false
	}
	if csrf == "" || subtle.ConstantTimeCompare([]byte(csrf), []byte(c.GetHeader("X-CSRF-Token"))) != 1 {
		httpapi.WriteProblem(c, 403, "csrf_invalid", nil)
		return false
	}
	return true
}
func (h *EmailAuthHandler) setPrincipal(c *gin.Context, r application.AuthResult) {
	u := r.User
	c.Set("principal", application.Principal{ID: u.ID, OrgID: u.OrgID, SessionID: r.Session.ID, CredentialEpoch: u.SessionEpoch, PlatformEpoch: u.PlatformEpoch, PlatformRole: u.PlatformRole, LoginName: u.LoginName, DisplayName: u.DisplayName})
	c.Set("email_session", r)
}

// @Summary refreshSession
// @ID refreshSession
// @Tags identity
// @Produce json
// @Success 200 {object} EmailSessionView
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 429 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/session/refresh [post]
func (h *EmailAuthHandler) refresh(c *gin.Context) {
	if !h.refreshCSRF(c) {
		return
	}
	operation, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || operation == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	token, _ := c.Cookie(sessionCookieName(h.secure, "refresh"))
	csrf, _ := c.Cookie(sessionCookieName(h.secure, "csrf"))
	r, err := h.auth.Refresh(c.Request.Context(), token, csrf, requestSource(c), operation)
	h.respond(c, r, err, 200)
}

// @Summary upgradeSession
// @ID upgradeSession
// @Tags identity
// @Produce json
// @Success 200 {object} EmailSessionView
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 429 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/session/upgrade [post]
func (h *EmailAuthHandler) upgrade(c *gin.Context) {
	cookies, err := h.sessionCookies(c)
	if err != nil {
		emailAuthProblem(c, err)
		return
	}
	if h.origin == "" || c.GetHeader("Origin") != h.origin {
		httpapi.WriteProblem(c, 403, "origin_forbidden", nil)
		return
	}
	operation, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || operation == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	r, err := h.auth.UpgradeLegacy(c.Request.Context(), cookies[sessionCookie], c.GetHeader("X-CSRF-Token"), requestSource(c), operation)
	h.respond(c, r, err, 200)
}

// sessionBusinessActivity is a server-owned allowlist. Polling, streams, previews,
// job status and ambiguous routes remain NoActivity regardless of caller headers.
func sessionBusinessActivity(c *gin.Context) bool {
	if c.Request.Method == http.MethodGet {
		switch c.FullPath() {
		case "/api/me", "/api/me/preferences", "/api/me/spaces", "/api/me/sessions", "/api/projects", "/api/projects/:pid", "/api/project-folders", "/api/teams/:id", "/api/teams/:id/members", "/api/teams/:id/members/:uid", "/api/media/library", "/api/media/library/items/:item_id", "/api/projects/:pid/media", "/api/projects/:pid/model-defaults", "/api/models", "/api/style-presets":
			return true
		default:
			return false
		}
	}
	switch c.Request.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		switch c.FullPath() {
		case "/api/me", "/api/me/preferences", "/api/projects", "/api/projects/:pid", "/api/project-folders", "/api/project-folders/:folder_id", "/api/teams/:id", "/api/projects/:pid/canvases", "/api/canvases/:id", "/api/projects/:pid/prompt-submissions", "/api/projects/:pid/operations", "/api/me/reauthentication":
			return true
		}
	}
	return false
}
