package identity_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspacehttp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/http"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

type filterFailureStore struct{ workspace.TeamStore }

func (filterFailureStore) TeamTransaction(context.Context, identity.Principal, func() time.Time, func(workspace.TeamTx) error) error {
	return workspace.ErrScopeNotFound
}

func TestPlatformTeamFilterRequiredQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	workspacehttp.NewPlatformTeamHandler(workspace.NewPlatformTeams(filterFailureStore{}, completionProtector(t), time.Now)).Register(router.Group("/api"))
	request := func(query string) int {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "GET", "/api/platform/teams?"+query, nil))
		return response.Code
	}

	for _, query := range []string{
		"id=11111111-1111-4111-8111-111111111111",
		"administrator_id=11111111-1111-4111-8111-111111111111",
		"created_from=2026-10-15T00:00:00Z&created_to=2026-10-16T00:00:00Z",
	} {
		if status := request(query); status != 404 {
			t.Errorf("required filter rejected %s: %v", query, status)
		}
	}
	for _, query := range []string{
		"id=", "id=invalid", "id=00000000-0000-0000-0000-000000000000", "administrator_id=invalid",
		"created_from=2026-10-15", "created_to=invalid", "created_from=2026-10-16T00:00:00Z&created_to=2026-10-15T00:00:00Z",
		"created_from=2026-10-15T00:00:00Z&created_to=2026-10-15T00:00:00Z", "created_to=2026-10-15T00:00:00Z&created_to=2026-10-15T00:00:00Z",
		"id=11111111-1111-4111-8111-111111111111&id=11111111-1111-4111-8111-111111111111", "unknown_filter=x",
	} {
		if status := request(query); status != 422 {
			t.Errorf("invalid filter accepted: %s", query)
		}
	}
}
