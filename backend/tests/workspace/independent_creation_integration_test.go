package workspace_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspacehttp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/http"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

func TestIndependentCreationScopeAndCanvasListing(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	org := insertWorkspaceOrganization(ctx, t, db)
	actor := insertWorkspaceActor(ctx, t, db, org)
	other := insertWorkspaceActor(ctx, t, db, org)
	store := workspaceStore(db)
	type outcome struct {
		id  uuid.UUID
		err error
	}
	created := make(chan outcome, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			p, err := store.EnsureIndependentCreation(ctx, actor)
			created <- outcome{p.ID, err}
		})
	}
	wg.Wait()
	a, b := <-created, <-created
	require.NoError(t, a.err)
	require.NoError(t, b.err)
	require.NotEqual(t, uuid.Nil, a.id)
	require.Equal(t, a.id, b.id)
	otherContext, err := store.EnsureIndependentCreation(ctx, other)
	require.NoError(t, err)
	require.NotEqual(t, a.id, otherContext.ID)

	command := workspace.NewCreateProjectCommand(store, time.Now)
	input := workspace.CreateProjectInput{
		IndependentCanvas: true, Name: "独立画布", AspectRatio: "16:9",
		StyleType: "realistic", RequestID: uuid.NewString(),
	}
	key := uuid.New()
	canvas, err := command.ExecuteIdempotent(ctx, actor, key, input)
	require.NoError(t, err)
	replay, err := command.ExecuteIdempotent(ctx, actor, key, input)
	require.NoError(t, err)
	require.Equal(t, canvas.ID, replay.ID)
	require.Equal(t, canvas.CanvasID, replay.CanvasID)

	page, err := store.ListProjectsForActor(ctx, actor, workspace.ListProjectsInput{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, page.Projects)
	canvases, err := store.ListProjectsForActor(ctx, actor, workspace.ListProjectsInput{Limit: 10, IncludeIndependentCanvases: true})
	require.NoError(t, err)
	require.Len(t, canvases.Projects, 1)
	require.Equal(t, canvas.ID, canvases.Projects[0].ID)
	var accounts, credits int
	require.NoError(t, db.Raw("SELECT count(*) FROM billing.wallet_account WHERE scope_id=?", actor.ScopeID).Scan(&accounts).Error)
	require.NoError(t, db.Raw("SELECT count(*) FROM billing.wallet_lot WHERE scope_id=?", actor.ScopeID).Scan(&credits).Error)
	require.Equal(t, 1, accounts)
	require.Zero(t, credits)
}

func TestIndependentCreationReturnsNavigableContext(t *testing.T) {
	createdAt := time.Date(2026, time.November, 6, 0, 0, 0, 0, time.UTC)
	project := domain.Project{
		ID: uuid.New(), Name: "独立创作", AspectRatio: "16:9",
		Status: domain.ProjectStatusActive, Revision: 1,
		CreateTime: createdAt, UpdateTime: createdAt,
	}
	router := gin.New()
	workspacehttp.RegisterIndependentCreation(router.Group("/api"), func(context.Context, identity.Principal) (domain.Project, error) {
		return project, nil
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/creation-context", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var body workspacehttp.ProjectResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, project.ID, body.ID)
	require.Nil(t, body.CanvasID)
	require.Equal(t, "created", body.LastModifiedBasis)
	require.Equal(t, createdAt, body.LastModifiedAt)
}
