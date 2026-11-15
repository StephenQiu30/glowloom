package microservices_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/httpprojects"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

func TestCreationWorkspaceCookiePolicyPreservesProtocolAndPublicFailures(t *testing.T) {
	scope := uuid.NewString()
	for _, test := range []struct {
		name, cookie, csrf, scope string
		secure                    bool
		status                    int
		code, access, legacy      string
	}{
		{"legacy", "glowloom_session=original", "proof", scope, false, 200, "", "", "original"},
		{"access", "glowloom_access=access; glowloom_csrf=proof", "proof", scope, false, 200, "", "access", ""},
		{"secure-access", "__Host-glowloom_access=access; __Host-glowloom_csrf=proof", "proof", scope, true, 200, "", "access", ""},
		{"duplicate-raw-cookie", "glowloom_session=first; glowloom_session=second", "proof", scope, false, 401, "session_invalid", "", ""},
		{"mixed-protocol", "glowloom_session=original; glowloom_access=access; glowloom_csrf=proof", "proof", scope, false, 401, "session_invalid", "", ""},
		{"wrong-secure-namespace", "glowloom_access=access; glowloom_csrf=proof", "proof", scope, true, 401, "session_invalid", "", ""},
		{"secure-cookie-over-http", "__Host-glowloom_access=access; __Host-glowloom_csrf=proof", "proof", scope, false, 401, "session_invalid", "", ""},
		{"refresh-only", "glowloom_refresh=refresh; glowloom_csrf=proof", "proof", scope, false, 401, "access_expired", "", ""},
		{"missing-access", "glowloom_csrf=proof", "proof", scope, false, 401, "session_invalid", "", ""},
		{"missing-csrf-cookie", "glowloom_access=access", "proof", scope, false, 403, "csrf_invalid", "", ""},
		{"mismatched-csrf", "glowloom_access=access; glowloom_csrf=proof", "other", scope, false, 403, "csrf_invalid", "", ""},
		{"missing-legacy-csrf", "glowloom_session=original", "", scope, false, 403, "csrf_invalid", "", ""},
		{"missing-session", "", "proof", scope, false, 401, "session_invalid", "", ""},
		{"bad-scope", "glowloom_session=original", "proof", "not-a-scope", false, 422, "invalid_creation_scope", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			engine := gin.New()
			engine.GET("/", func(c *gin.Context) {
				credential, actualScope, err := httpprojects.CookieResolver(test.secure)(c)
				if err != nil || c.IsAborted() {
					return
				}
				access, legacy, csrf := credential.IdentityProof()
				if access != test.access || legacy != test.legacy || csrf != test.csrf || actualScope.String() != scope {
					t.Error("original credential/scope selection was replaced")
				}
				c.Status(200)
			})
			request := httptest.NewRequestWithContext(t.Context(), "GET", "/", nil)
			request.Header.Set("Cookie", test.cookie)
			request.Header.Set("X-CSRF-Token", test.csrf)
			request.Header.Set("X-Creation-Scope", test.scope)
			engine.ServeHTTP(response, request)
			if response.Code != test.status || (test.code != "" && !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`)) {
				t.Fatalf("original public failure changed: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCreationWorkspacePublicOriginPrecedesMutationAndScopedConfigIsStrict(t *testing.T) {
	response := httptest.NewRecorder()
	engine := gin.New()
	engine.Use(httpapi.Middleware("https://creation.invalid"))
	called := false
	engine.POST("/api/projects", func(c *gin.Context) { called = true; c.Status(201) })
	request := httptest.NewRequestWithContext(t.Context(), "POST", "/api/projects", nil)
	request.Header.Set("Origin", "https://other.invalid")
	request.Header.Set("Idempotency-Key", uuid.NewString())
	engine.ServeHTTP(response, request)
	if response.Code != 403 || called || !strings.Contains(response.Body.String(), `"code":"origin_forbidden"`) {
		t.Fatal("native API accepted a foreign origin")
	}
	for _, value := range []string{"", `{}`, `{"unexpected":true}`, `{} {}`} {
		if _, err := workspaceapp.LoadEnvironment(func(name string) (string, bool) {
			if name != "GLOWLOOM_CREATION_WORKSPACE_CONFIG" {
				t.Fatal("read unrelated configuration")
			}
			return value, true
		}); err == nil {
			t.Fatalf("incomplete/unknown configuration accepted: %s", value)
		}
	}
}
