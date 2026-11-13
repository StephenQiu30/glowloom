package http

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

type grantProtocolStore struct {
	app.BusinessStore
	called bool
}

func (s *grantProtocolStore) RegisterGrant(context.Context, domain.GrantRequest) (domain.Grant, error) {
	s.called = true
	return domain.Grant{}, nil
}

func TestBusinessWritesUseServiceIdentityWithGlobalProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := strings.Repeat("b", 48)
	for _, tc := range []struct {
		name, authorization, audience string
		status                        int
	}{
		{"missing identity", "", "", 401},
		{"wrong token", "Bearer " + strings.Repeat("x", 48), "agent-business", 401},
		{"wrong audience", "Bearer " + token, "agent-runtime", 401},
		{"authenticated service", "Bearer " + token, "agent-business", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &grantProtocolStore{}
			router := gin.New()
			router.Use(httpapi.Middleware("http://localhost:1122"))
			NewBusinessHandler(store, app.Capabilities{}, token).Register(router)
			request := httptest.NewRequestWithContext(t.Context(), "POST", "/internal/agent-business/grants", strings.NewReader(`{}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", tc.authorization)
			request.Header.Set("X-Agent-Audience", tc.audience)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			require.Equal(t, tc.status == 200, store.called)
			require.NotEmpty(t, response.Header().Get("X-Request-Id"))
		})
	}
}

func TestPublicWritesKeepOriginAndIdempotencyChecks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		origin, key string
		status      int
	}{
		{"", "", 403},
		{"https://untrusted.example", uuid.NewString(), 403},
		{"http://localhost:1122", "", 422},
		{"http://localhost:1122", uuid.NewString(), 200},
	} {
		router := gin.New()
		router.Use(httpapi.Middleware("http://localhost:1122"))
		router.POST("/api/write", func(c *gin.Context) { c.Status(200) })
		request := httptest.NewRequestWithContext(t.Context(), "POST", "/api/write", nil)
		request.Header.Set("Origin", tc.origin)
		request.Header.Set("Idempotency-Key", tc.key)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, tc.status, response.Code, response.Body.String())
	}
}
