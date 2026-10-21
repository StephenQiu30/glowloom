package operation_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/codex"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trial"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestTrialLiveHTTPTemporalNativeProbe(t *testing.T) {
	address, namespace, binary := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE"), os.Getenv("LV_TEST_CODEX_BINARY")
	if address == "" || namespace == "" || binary == "" {
		t.Skip("requires isolated Temporal namespace, migrated DB and explicit native CLI")
	}
	database := operationStoreDB(t)
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	require.NoError(t, database.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Live Trial Admin','producer','synthetic',false,'platform_admin')`, actor.ID, actor.OrgID, "live-trial-"+actor.ID.String()).Error)
	actor = identityfixture.Session(t, database, actor)
	c, err := client.Dial(client.Options{HostPort: address, Namespace: namespace})
	require.NoError(t, err)
	defer c.Close()
	flow := worker.New(c, "flow", worker.Options{})
	flow.RegisterWorkflow(operationflow.TrialWorkflow)
	require.NoError(t, flow.Start())
	defer flow.Stop()
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0700))
	launcher, err := codex.NewLocalProcessLauncher(binary, root)
	require.NoError(t, err)
	store := app.NewCatalogStore(database)
	runner := &trial.Runner{Store: trial.NewStore(database, store), Launcher: launcher, WorkRoot: root}
	native := worker.New(c, "agent.codex", worker.Options{MaxConcurrentActivityExecutionSize: 1})
	native.RegisterActivityWithOptions(runner.Run, activity.RegisterOptions{Name: "trial.Run"})
	require.NoError(t, native.Start())
	defer native.Stop()
	router := gin.New()
	router.Use(func(ctx *gin.Context) { ctx.Set("principal", actor) })
	operationhttp.NewTrialHandler(c, store, nil, "fixture-target").Register(router.Group("/api"))
	request := application.TrialRequest{RequestID: uuid.New(), Source: "codex", Kind: "probe"}
	body, err := json.Marshal(request)
	require.NoError(t, err)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "POST", "/api/admin/ai-trials", strings.NewReader(string(body))))
	require.Equal(t, 202, response.Code, response.Body.String())
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	run := c.GetWorkflow(ctx, application.TrialWorkflowID(actor, request.RequestID), "")
	var final application.TrialState
	require.NoError(t, run.Get(ctx, &final))
	require.Equal(t, "succeeded", final.State)
	require.NotEmpty(t, final.Turns[0].Models)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "GET", "/api/admin/ai-trials/"+request.RequestID.String(), nil))
	require.Equal(t, 200, response.Code, response.Body.String())
	require.NotContains(t, response.Body.String(), "thread_id")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "POST", "/api/admin/ai-trials", strings.NewReader(string(body))))
	require.Equal(t, 202, response.Code, response.Body.String())
	t.Logf("HTTP -> current DB administrator -> Temporal -> native CLI probe -> durable HTTP result passed; authenticated=%t models=%d; no inference", final.Turns[0].Authenticated, len(final.Turns[0].Models))
}
