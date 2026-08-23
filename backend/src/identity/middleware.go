package identity

import (
	"context"
	"net/http"

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
		rawAccessToken, ok := toolkit.BearerToken(r.Header.Get("Authorization"))
		if !ok {
			httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "Bearer 会话缺失", "刷新登录会话后重试"))
			return
		}
		principal, err := identityService.Authenticate(r.Context(), rawAccessToken)
		if err != nil {
			httpapi.WriteError(w, r, err)
			return
		}
		workspaceContext := database.WithWorkspaceID(r.Context(), principal.WorkspaceID)
		if err := identityService.AuthorizePath(workspaceContext, principal.WorkspaceID, r.URL.Path); err != nil {
			httpapi.WriteError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(workspaceContext, contextKey{}, principal)))
	})
}

func RequireForBusiness(identityService *IdentityService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicIdentityRoute(r) || r.URL.Path == "/readyz" || r.URL.Path == "/api/swagger.json" || r.URL.Path == "/api/docs" {
				next.ServeHTTP(w, r)
				return
			}
			Require(identityService, next).ServeHTTP(w, r)
		})
	}
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok || !principal.Role.IsAdmin() {
			httpapi.WriteError(w, r, httpapi.Forbidden("只有管理员可以访问此内容", "请联系管理员"))
			return
		}
		next.ServeHTTP(w, r)
	})
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
