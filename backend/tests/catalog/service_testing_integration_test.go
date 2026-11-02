package catalog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/providerprobe"
	flow "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/workflow"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestServiceReachabilityWithoutCredentialIsDurableAndDeduplicated(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated catalog database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	org := insertModelCatalogOrganization(t.Context(), t, conn.DB)
	actor := insertModelCatalogActor(t.Context(), t, conn.DB, org)
	require.NoError(t, conn.DB.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error)
	actor = identityfixture.Session(t, conn.DB, actor)
	store := app.NewCatalogStore(conn.DB)
	presets := catalog.NewProviderPresets(store)
	items, err := presets.List(t.Context(), actor)
	require.NoError(t, err)
	var connection uuid.UUID
	for _, p := range items {
		if p.Slug == "deepseek" {
			r, e := presets.Connect(t.Context(), actor, p.ID, catalog.ConnectPresetInput{RequestID: uuid.New(), VersionID: p.VersionID, CatalogVersion: p.ModelCatalog.Version, CandidateKeys: []string{p.ModelCatalog.Models[0].Key}, DirectUse: true})
			require.NoError(t, e)
			connection = r.ProviderID
		}
	}
	require.NotEqual(t, uuid.Nil, connection)
	command := catalog.NewRequestCredentialTestCommand(store)
	key := uuid.NewString()
	accepted, err := command.ExecuteReachability(t.Context(), actor, connection, key)
	require.NoError(t, err)
	again, err := command.ExecuteReachability(t.Context(), actor, connection, key)
	require.NoError(t, err)
	require.Equal(t, accepted, again)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			r, e := command.ExecuteReachability(context.Background(), actor, connection, uuid.NewString())
			require.NoError(t, e)
			require.Equal(t, accepted.CheckID, r.CheckID)
		})
	}
	wg.Wait()
	calls := 0
	client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Empty(t, r.Header.Get("Authorization"))
		return providerResponse(401, `{"error":"not authorized"}`), nil
	})})
	worker := providerprobe.NewScopedActivities(conn.DB, nil, client, store)
	input := flow.Input{TestID: accepted.TestID, ProviderID: connection, ActorID: actor.ID, OrgID: actor.OrgID, RequestID: key}
	require.NoError(t, worker.TestReachability(t.Context(), input))
	check, err := store.ReadProviderCheck(t.Context(), actor, accepted.CheckID)
	require.NoError(t, err)
	require.Equal(t, "reachable", check.NetworkResult)
	require.Equal(t, 401, check.HTTPStatus)
	require.Equal(t, "unknown", check.AuthResult)
	require.Equal(t, "completed", check.Status)
	require.NoError(t, worker.TestReachability(t.Context(), input))
	require.Equal(t, 1, calls)
	page, err := store.ListProviderChecks(t.Context(), actor, connection, 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.True(t, page.Items[0].Current)
	data, err := json.Marshal(page)
	require.NoError(t, err)
	require.NotContains(t, string(data), "ciphertext")
	credential := uuid.New()
	require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.provider_credential(id,provider_id,label,ciphertext,key_id,last4) VALUES(?,?,'fixture',?,'fixture','test')`, credential, connection, []byte("synthetic-envelope")).Error)
	var model struct{ ID, VersionID uuid.UUID }
	require.NoError(t, conn.DB.Raw(`SELECT id,current_version_id AS version_id FROM catalog.model_profile WHERE provider_id=? LIMIT 1`, connection).Scan(&model).Error)
	unknown := uuid.New()
	require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.provider_check(id,actor_id,provider_id,config_version_id,credential_id,kind,status,model_id,model_version_id,model_result,dispatch_started_at) SELECT ?,?,id,current_config_version_id,?,'model_access','failed',?,?,'unknown',now() FROM catalog.provider WHERE id=?`, unknown, actor.ID, credential, model.ID, model.VersionID, connection).Error)
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.provider SET concurrency_limit=1 WHERE id=?`, connection).Error)
	_, capacityErr := command.ExecuteReachability(t.Context(), actor, connection, uuid.NewString())
	require.ErrorIs(t, capacityErr, catalog.ErrProviderCheckCapacity)
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.provider SET concurrency_limit=2 WHERE id=?`, connection).Error)
	blocked, err := command.ExecuteModel(t.Context(), actor, connection, credential, model.ID, uuid.NewString())
	require.NoError(t, err)
	require.Equal(t, unknown, blocked.CheckID)
	ended, err := store.EndProviderCheckTracking(t.Context(), actor, unknown, uuid.New(), "已核对控制台，无法取得回执")
	require.NoError(t, err)
	require.Equal(t, "unknown", ended.ModelResult)
	require.NotNil(t, ended.TrackingEndedAt)
	next, err := command.ExecuteModel(t.Context(), actor, connection, credential, model.ID, uuid.NewString())
	require.NoError(t, err)
	require.NotEqual(t, unknown, next.CheckID)
	newCheck, err := store.ReadProviderCheck(t.Context(), actor, next.CheckID)
	require.NoError(t, err)
	require.Equal(t, &unknown, newCheck.ParentCheckID)
	require.NoError(t, conn.DB.Exec(`UPDATE catalog.provider_check SET status='completed',auth_result='ok',model_result='accepted',task_id='task-fixture',dispatch_started_at=now() WHERE id=?`, next.CheckID).Error)
	checks := catalog.NewProviderChecks(store)
	query, err := checks.Query(t.Context(), actor, next.CheckID, uuid.NewString())
	require.NoError(t, err)
	require.NotEqual(t, next.CheckID, query.CheckID)
	var queryKind string
	require.NoError(t, conn.DB.Raw(`SELECT kind FROM catalog.provider_check WHERE id=?`, query.CheckID).Scan(&queryKind).Error)
	require.Equal(t, "model_query", queryKind)
	require.NoError(t, store.FailProviderCheck(t.Context(), query.CheckID, actor.ID, connection, credential))
	original, err := store.ReadProviderCheck(t.Context(), actor, next.CheckID)
	require.NoError(t, err)
	require.Equal(t, "unknown", original.ModelResult)
	require.Equal(t, "failed", original.Status)
	require.Equal(t, "task-fixture", original.TaskID)

	require.NoError(t, conn.DB.Exec(`UPDATE identity."user" SET platform_role='creator',platform_epoch=platform_epoch+1 WHERE id=?`, actor.ID).Error)
	_, err = store.ListProviderChecks(t.Context(), actor, connection, 0)
	require.Error(t, err)
}
