package catalog_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialschema"
	cataloghttp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
)

type metadataAccess struct {
	err   error
	calls int
}

func (a *metadataAccess) AuthorizeAdapterMetadata(context.Context, identityapp.Principal) error {
	a.calls++
	return a.err
}
func installedProtocol(id string) bool { _, ok := providerhttp.GetDriver(id); return ok }
func TestAdapterMetadataSafeProjectionAndAuthorization(t *testing.T) {
	access := &metadataAccess{}
	query := application.NewAdapterMetadataQuery(access, credentialschema.NewRegistry(), installedProtocol)
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	items, err := query.List(t.Context(), actor)
	require.NoError(t, err)
	require.Len(t, items, 10)
	for _, item := range items {
		require.NotEmpty(t, item.DisplayName)
		require.NotEmpty(t, item.LogoKey)
		require.NotEmpty(t, item.CredentialFields)
		for _, id := range item.ProtocolIDs {
			require.True(t, installedProtocol(id))
		}
	}
	data, err := json.Marshal(items)
	require.NoError(t, err)
	for _, private := range []string{"base_url", "Authorization", "secret_access_key\":\"", "ciphertext", "last4"} {
		require.NotContains(t, string(data), private)
	}
	_, err = query.Find(t.Context(), actor, "uninstalled")
	require.ErrorIs(t, err, application.ErrAdapterNotFound)
	actor.PlatformRole = "member"
	_, err = query.List(t.Context(), actor)
	require.ErrorIs(t, err, identityapp.ErrForbidden)
	access.err = identityapp.ErrSessionInvalid
	actor.PlatformRole = "platform_admin"
	_, err = query.List(t.Context(), actor)
	require.ErrorIs(t, err, identityapp.ErrSessionInvalid)
}
func TestAdapterMetadataHTTPUsesAdminRoute(t *testing.T) {
	for _, role := range []string{"platform_admin", "member"} {
		t.Run(role, func(t *testing.T) {
			router := gin.New()
			actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: role}
			router.Use(func(c *gin.Context) { c.Set("principal", actor) })
			cataloghttp.NewAdminHandler(cataloghttp.AdminDependencies{Adapters: application.NewAdapterMetadataQuery(&metadataAccess{}, credentialschema.NewRegistry(), installedProtocol)}).Register(router.Group("/api"))
			result := httptest.NewRecorder()
			router.ServeHTTP(result, httptest.NewRequestWithContext(t.Context(), "GET", "/api/admin/provider-adapters", nil))
			if role == "member" {
				require.Equal(t, 403, result.Code)
			} else {
				require.Equal(t, 200, result.Code)
				require.Equal(t, "private, no-store", result.Header().Get("Cache-Control"))
				require.NotContains(t, result.Body.String(), "base_url")
			}
		})
	}
}
