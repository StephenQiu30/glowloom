package httpprojects

import (
	"crypto/subtle"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// CookieResolver preserves the existing secure/nonsecure namespaces, explicit
// protocol selection and conflict rejection. Identity checks the actual cookie
// and stored CSRF; this adapter neither verifies JWTs nor trusts caller actors.
func CookieResolver(secure bool) NativeProjectResolver {
	return func(c *gin.Context) (source.Credential, uuid.UUID, error) {
		invalid := func(code string, status int) (source.Credential, uuid.UUID, error) {
			httpapi.WriteProblem(c, status, code, nil)
			return source.Credential{}, uuid.Nil, source.ErrSourceSessionInvalid
		}
		recognized := func(name string) bool {
			switch name {
			case "glowloom_session", "glowloom_access", "glowloom_refresh", "glowloom_csrf", "__Host-glowloom_access", "__Host-glowloom_refresh", "__Host-glowloom_csrf":
				return true
			}
			return false
		}
		counts := map[string]int{}
		for _, header := range c.Request.Header.Values("Cookie") {
			for _, part := range strings.Split(header, ";") {
				name, _, _ := strings.Cut(strings.TrimSpace(part), "=")
				name = strings.TrimSpace(name)
				if recognized(name) {
					counts[name]++
					if counts[name] > 1 {
						return invalid("session_invalid", 401)
					}
				}
			}
		}
		values := map[string]string{}
		for _, cookie := range c.Request.Cookies() {
			if recognized(cookie.Name) {
				values[cookie.Name] = cookie.Value
			}
		}
		modern := false
		for name := range counts {
			if _, ok := values[name]; !ok {
				return invalid("session_invalid", 401)
			}
			if name != "glowloom_session" {
				modern = true
				if strings.HasPrefix(name, "__Host-") != secure {
					return invalid("session_invalid", 401)
				}
			}
		}
		if modern && counts["glowloom_session"] > 0 {
			return invalid("session_invalid", 401)
		}
		prefix := ""
		if secure {
			prefix = "__Host-"
		}
		csrf := c.GetHeader("X-CSRF-Token")
		var credential source.Credential
		if modern {
			access := values[prefix+"glowloom_access"]
			if access == "" {
				if values[prefix+"glowloom_refresh"] != "" {
					return invalid("access_expired", 401)
				}
				return invalid("session_invalid", 401)
			}
			cookieCSRF := values[prefix+"glowloom_csrf"]
			if csrf == "" || cookieCSRF == "" || subtle.ConstantTimeCompare([]byte(csrf), []byte(cookieCSRF)) != 1 {
				return invalid("csrf_invalid", 403)
			}
			credential = source.AccessCredential(access, csrf)
		} else {
			legacy := values["glowloom_session"]
			if legacy == "" {
				return invalid("session_invalid", 401)
			}
			if csrf == "" {
				return invalid("csrf_invalid", 403)
			}
			credential = source.LegacyCredential(legacy, csrf)
		}
		raw := c.GetHeader("X-Creation-Scope")
		scope, err := uuid.Parse(raw)
		if err != nil || scope == uuid.Nil || scope.String() != raw {
			return invalid("invalid_creation_scope", 422)
		}
		c.Header("Cache-Control", "private, no-store")
		return credential, scope, nil
	}
}
