package catalog_test

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/providerprobe"
	flow "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/task"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

type serviceTestingFixture struct {
	db                          *gorm.DB
	store                       *pgcatalog.Store
	actor                       identity.Principal
	provider, credential, model uuid.UUID
	envelope                    []byte
	unsealer                    *credentialseal.Unsealer
}

func newServiceTestingFixture(t *testing.T, slugs ...string) serviceTestingFixture {
	slug := "deepseek"
	if len(slugs) > 0 {
		slug = slugs[0]
	}
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated catalog database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	actor := insertModelCatalogActor(t.Context(), t, conn.DB, insertModelCatalogOrganization(t.Context(), t, conn.DB))
	require.NoError(t, conn.DB.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error)
	actor = identityfixture.Session(t, conn.DB, actor)
	store := app.NewCatalogStore(conn.DB)
	presets := catalog.NewProviderPresets(store)
	items, err := presets.List(t.Context(), actor)
	require.NoError(t, err)
	var connected catalog.PresetConnection
	for _, p := range items {
		if p.Slug == slug {
			connected, err = presets.Connect(t.Context(), actor, p.ID, catalog.ConnectPresetInput{RequestID: uuid.New(), VersionID: p.VersionID, CatalogVersion: p.ModelCatalog.Version, CandidateKeys: []string{p.ModelCatalog.Models[0].Key}, DirectUse: true})
			require.NoError(t, err)
		}
	}
	seedReviewedOnboardingPrices(t, conn.DB, actor, connected, false)
	private, public := testCredentialKey(t)
	der, err := x509.MarshalPKCS8PrivateKey(private)
	require.NoError(t, err)
	unsealer, err := credentialseal.NewUnsealer("service-check-fixture", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	require.NoError(t, err)
	sealer, err := credentialseal.NewSealer("service-check-fixture", public)
	require.NoError(t, err)
	credential := uuid.New()
	envelope, err := sealer.Seal(connected.ProviderID, credential, json.RawMessage(`{"api_key":"synthetic-service-key"}`))
	require.NoError(t, err)
	require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.provider_credential(id,provider_id,label,ciphertext,key_id,last4) VALUES(?,?,'fixture',?,'service-check-fixture','-key')`, credential, connected.ProviderID, envelope).Error)
	return serviceTestingFixture{conn.DB, store, actor, connected.ProviderID, credential, connected.ModelIDs[0], envelope, unsealer}
}
func TestServiceTestingFailureBoundaries(t *testing.T) {
	t.Run("queued network closes after configuration changes", func(t *testing.T) {
		f := newServiceTestingFixture(t)
		key := uuid.NewString()
		accepted, err := catalog.NewRequestCredentialTestCommand(f.store).ExecuteReachability(t.Context(), f.actor, f.provider, key)
		require.NoError(t, err)
		require.NoError(t, f.db.Exec(`UPDATE catalog.provider SET current_config_version_id=NULL WHERE id=?`, f.provider).Error)
		worker := providerprobe.NewScopedActivities(f.db, nil, providerhttp.NewClient(nil), f.store)
		require.NoError(t, worker.TestReachability(t.Context(), flow.Input{TestID: accepted.TestID, ProviderID: f.provider, ActorID: f.actor.ID, OrgID: f.actor.OrgID, RequestID: key}))
		check, err := f.store.ReadProviderCheck(t.Context(), f.actor, accepted.CheckID)
		require.NoError(t, err)
		require.Equal(t, "failed", check.Status)
		require.False(t, check.Current)
	})
	t.Run("accepted model request merges until final result", func(t *testing.T) {
		f := newServiceTestingFixture(t)
		command := catalog.NewRequestCredentialTestCommand(f.store)
		original, err := command.ExecuteModel(t.Context(), f.actor, f.provider, f.credential, f.model, uuid.NewString())
		require.NoError(t, err)
		require.NoError(t, f.db.Exec(`UPDATE catalog.provider_check SET status='completed',auth_result='ok',model_result='accepted',task_id='task-fixture',dispatch_started_at=now() WHERE id=?`, original.CheckID).Error)
		next, err := command.ExecuteModel(t.Context(), f.actor, f.provider, f.credential, f.model, uuid.NewString())
		require.NoError(t, err)
		require.Equal(t, original.CheckID, next.CheckID)
	})
	t.Run("old version success is not current onboarding evidence", func(t *testing.T) {
		f := newServiceTestingFixture(t)
		original, err := catalog.NewRequestCredentialTestCommand(f.store).ExecuteModel(t.Context(), f.actor, f.provider, f.credential, f.model, uuid.NewString())
		require.NoError(t, err)
		require.NoError(t, f.db.Exec(`UPDATE catalog.provider_check SET status='completed',auth_result='ok',model_result='completed',dispatch_started_at=now() WHERE id=?`, original.CheckID).Error)
		version := uuid.New()
		require.NoError(t, f.db.Exec(`INSERT INTO catalog.model_profile_version SELECT (jsonb_populate_record(NULL::catalog.model_profile_version,to_jsonb(v)||jsonb_build_object('id',?::uuid,'version_no',v.version_no+1))).* FROM catalog.model_profile_version v JOIN catalog.model_profile m ON m.current_version_id=v.id WHERE m.id=?`, version, f.model).Error)
		require.NoError(t, f.db.Exec(`UPDATE catalog.model_profile SET current_version_id=? WHERE id=?`, version, f.model).Error)
		state, err := catalog.NewProviderOnboarding(f.store).Read(t.Context(), f.actor, f.provider)
		require.NoError(t, err)
		require.Nil(t, state.Check)
	})
	t.Run("read only credential requests consume the shared rate", func(t *testing.T) {
		f := newServiceTestingFixture(t)
		require.NoError(t, f.db.Exec(`UPDATE catalog.provider SET rate_limit_per_min=1 WHERE id=?`, f.provider).Error)
		command := catalog.NewRequestCredentialTestCommand(f.store)
		key := uuid.NewString()
		accepted, err := command.Execute(t.Context(), f.actor, f.provider, f.credential, key)
		require.NoError(t, err)
		request := catalog.CredentialTestRequest{TestID: accepted.TestID, ProviderID: f.provider, CredentialID: f.credential, ActorID: f.actor.ID, OrgID: f.actor.OrgID, RequestID: key}
		service := catalog.NewCredentialTestService(f.store)
		_, _, err = service.Load(t.Context(), request)
		require.NoError(t, err)
		client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
			require.Equal(t, "GET", r.Method)
			return providerResponse(200, `{"data":[]}`), nil
		})})
		evidence, err := providerprobe.NewScopedActivities(f.db, f.unsealer, client, f.store).TestCredential(t.Context(), flow.AgentTestInput{TestID: accepted.TestID, ProviderID: f.provider, ActorID: f.actor.ID, OrgID: f.actor.OrgID, RequestID: key, AdapterKey: "deepseek", Credential: flow.AgentCredential{ID: f.credential, KeyID: "service-check-fixture", Ciphertext: base64.StdEncoding.EncodeToString(f.envelope)}})
		require.NoError(t, err)
		require.Equal(t, "ok", evidence.AuthResult)
		require.NoError(t, service.Record(t.Context(), catalog.CredentialTestObservation{CredentialTestRequest: request, Result: evidence.Result, Last4: "-key", TestedAt: time.Now().UTC(), AuthResult: evidence.AuthResult, AdapterResult: evidence.AdapterResult, DiscoveryResult: evidence.DiscoveryResult}))
		_, err = command.Execute(t.Context(), f.actor, f.provider, f.credential, uuid.NewString())
		require.ErrorIs(t, err, catalog.ErrProviderCheckCapacity)
	})
}

func TestHistoricalAsyncCheckQueriesItsPinnedDrainingKey(t *testing.T) {
	f := newServiceTestingFixture(t, "volcengine-ark")
	command := catalog.NewRequestCredentialTestCommand(f.store)
	original, err := command.ExecuteModel(t.Context(), f.actor, f.provider, f.credential, f.model, uuid.NewString())
	require.NoError(t, err)
	require.NoError(t, f.db.Exec(`UPDATE catalog.provider_check SET status='completed',model_result='accepted',auth_result='ok',task_id='task-fixture',dispatch_started_at=now() WHERE id=?`, original.CheckID).Error)
	require.NoError(t, f.db.Exec(`UPDATE catalog.provider_credential SET status='draining' WHERE id=?`, f.credential).Error)
	config := uuid.New()
	require.NoError(t, f.db.Exec(`INSERT INTO catalog.provider_config_version SELECT (jsonb_populate_record(NULL::catalog.provider_config_version,to_jsonb(v)||jsonb_build_object('id',?::uuid,'version_no',v.version_no+1))).* FROM catalog.provider_config_version v JOIN catalog.provider p ON p.current_config_version_id=v.id WHERE p.id=?`, config, f.provider).Error)
	require.NoError(t, f.db.Exec(`UPDATE catalog.provider SET current_config_version_id=? WHERE id=?`, config, f.provider).Error)
	key := uuid.NewString()
	query, err := catalog.NewProviderChecks(f.store).Query(t.Context(), f.actor, original.CheckID, key)
	require.NoError(t, err)
	request := catalog.CredentialTestRequest{Kind: "model_query", TestID: query.TestID, ProviderID: f.provider, CredentialID: f.credential, ActorID: f.actor.ID, OrgID: f.actor.OrgID, RequestID: key}
	service := catalog.NewCredentialTestService(f.store)
	_, _, err = service.Load(t.Context(), request)
	require.NoError(t, err)
	calls := 0
	client := providerhttp.NewClient(&http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "/api/v3/contents/generations/tasks/task-fixture", r.URL.Path)
		require.Equal(t, "Bearer synthetic-service-key", r.Header.Get("Authorization"))
		return providerResponse(200, `{"id":"task-fixture","status":"succeeded","content":{"video_url":"https://ark-acg-cn-beijing.tos-cn-beijing.volces.com/fixture.mp4"}}`), nil
	})})
	evidence, err := providerprobe.NewScopedActivities(f.db, f.unsealer, client, f.store).TestCredential(t.Context(), flow.AgentTestInput{TestID: query.TestID, ProviderID: f.provider, ActorID: f.actor.ID, OrgID: f.actor.OrgID, RequestID: key, AdapterKey: "volcengine_ark", Credential: flow.AgentCredential{ID: f.credential, KeyID: "service-check-fixture", Ciphertext: base64.StdEncoding.EncodeToString(f.envelope)}})
	require.NoError(t, err)
	require.Equal(t, "completed", evidence.ModelResult)
	require.Equal(t, 1, calls)
	require.NoError(t, service.Record(t.Context(), catalog.CredentialTestObservation{CredentialTestRequest: request, Result: evidence.Result, Last4: "-key", TestedAt: time.Now().UTC(), AuthResult: evidence.AuthResult, AdapterResult: evidence.AdapterResult, DiscoveryResult: evidence.DiscoveryResult, ModelResult: evidence.ModelResult, TaskID: evidence.TaskID}))
	check, err := f.store.ReadProviderCheck(t.Context(), f.actor, original.CheckID)
	require.NoError(t, err)
	require.Equal(t, "completed", check.ModelResult)
	require.False(t, check.Current)
}
