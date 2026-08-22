package identity

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
	"github.com/stephenqiu30/lanverse/backend/src/platform/toolkit"
)

type contextKey struct{}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	value, ok := ctx.Value(contextKey{}).(Principal)
	return value, ok
}

func Require(identityService *IdentityService, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workspaceID, err := uuid.Parse(strings.TrimSpace(r.Header.Get("X-Workspace-Id")))
		if err != nil || workspaceID == uuid.Nil {
			httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "Workspace 上下文缺失", "提供 X-Workspace-Id 后重试"))
			return
		}
		rawAccessToken, ok := toolkit.BearerToken(r.Header.Get("Authorization"))
		if !ok {
			httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "Bearer 会话缺失", "刷新登录会话后重试"))
			return
		}
		workspaceContext := database.WithWorkspaceID(r.Context(), workspaceID)
		principal, err := identityService.Authenticate(workspaceContext, rawAccessToken, workspaceID)
		if err != nil {
			httpapi.WriteError(w, r, err)
			return
		}
		if err := identityService.AuthorizePath(workspaceContext, workspaceID, r.URL.Path); err != nil {
			httpapi.WriteError(w, r, httpapi.NotFound("资源"))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(workspaceContext, contextKey{}, principal)))
	})
}

func RequireForBusiness(identityService *IdentityService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicIdentityRoute(r) || r.URL.Path == "/readyz" || r.URL.Path == "/api/openapi.json" || r.URL.Path == "/api/docs" {
				next.ServeHTTP(w, r)
				return
			}
			Require(identityService, next).ServeHTTP(w, r)
		})
	}
}

func isPublicIdentityRoute(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/api/auth/register", "/api/auth/login", "/api/auth/refresh", "/api/auth/logout":
		return true
	default:
		return false
	}
}
