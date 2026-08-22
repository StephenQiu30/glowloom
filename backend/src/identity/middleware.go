package identity

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
)

type contextKey struct{}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	value, ok := ctx.Value(contextKey{}).(Principal)
	return value, ok
}

func Require(identityService *IdentityService, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workspaceID, err := uuid.Parse(r.Header.Get("X-Workspace-Id"))
		if err != nil {
			httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "Workspace 上下文缺失", "提供 X-Workspace-Id 后重试"))
			return
		}
		authorization := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if len(authorization) <= len(prefix) || authorization[:len(prefix)] != prefix {
			httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusUnauthorized, httpapi.CodeUnauthorized, "Bearer 会话缺失", "创建或刷新当前 Workspace 会话"))
			return
		}
		principal, err := identityService.Authenticate(r.Context(), authorization[len(prefix):], workspaceID)
		if err != nil {
			httpapi.WriteError(w, r, httpapi.NewError(httpapi.StatusForbidden, httpapi.CodeForbidden, "Workspace 访问被拒绝", "确认当前会话拥有该 Workspace 权限"))
			return
		}
		if err := identityService.AuthorizePath(r.Context(), workspaceID, r.URL.Path); err != nil {
			httpapi.WriteError(w, r, httpapi.NotFound("资源"))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, principal)))
	})
}

func RequireForBusiness(identityService *IdentityService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if (r.Method == http.MethodPost && (r.URL.Path == "/api/workspaces" || r.URL.Path == "/api/sessions")) || r.URL.Path == "/readyz" || r.URL.Path == "/api/openapi.json" || r.URL.Path == "/api/docs" {
				next.ServeHTTP(w, r)
				return
			}
			Require(identityService, next).ServeHTTP(w, r)
		})
	}
}
