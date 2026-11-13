package catalog_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	cataloghttp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/http"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	ophttp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestRoutePoolPublication(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip()
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	actor := adminPrincipal()
	require.NoError(t, conn.DB.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password,platform_role) VALUES(?,?,?,'Fixture','producer','synthetic',false,'platform_admin')`, actor.ID, actor.OrgID, "test-"+actor.ID.String()).Error)
	actor = identityfixture.Session(t, conn.DB, actor)
	store := app.NewCatalogStore(conn.DB)
	project := insertModelCatalogProject(t.Context(), t, conn.DB, actor, true)
	var releases []uuid.UUID
	for _, name := range []string{"甲", "乙"} {
		connection, err := catalogapp.NewProviderPresets(store).Connect(t.Context(), actor, uuid.MustParse("6da07a10-0000-4000-8000-000000000001"), catalogapp.ConnectPresetInput{VersionID: uuid.MustParse("6da07a11-0000-4000-8000-000000000001"), Key: "accept-" + uuid.NewString(), Name: name, RequestID: uuid.New()})
		require.NoError(t, err)
		cred, err := newCredentialCommand(t, store).Execute(t.Context(), actor, catalogapp.SetCredentialInput{ProviderID: connection.ProviderID, Label: "fixture", Secret: json.RawMessage(`{"api_key":"isolated-test-only"}`), RequestID: uuid.NewString()})
		require.NoError(t, err)
		require.NoError(t, conn.DB.Exec("UPDATE catalog.provider SET status='active' WHERE id=?", connection.ProviderID).Error)
		model, version := insertModelCatalogModel(t.Context(), t, conn.DB, "test-"+uuid.NewString(), connection.ProviderID, "video.generate", "active", "text2video", false)
		require.NoError(t, conn.DB.Exec("UPDATE catalog.model_profile_version SET queue='provider' WHERE id=?", version).Error)
		purchase, sale, release := uuid.New(), uuid.New(), uuid.New()
		for index, id := range []uuid.UUID{purchase, sale} {
			rule := map[string]any{"base_micros": 7560000}
			if index == 1 {
				rule["procurement_price_version_id"] = purchase
			}
			raw, _ := json.Marshal(rule)
			require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,effective_from,source_url,reviewed_at) VALUES(?,?,?,'per_request',?::jsonb,'CNY',now()-interval '1 hour','https://example.invalid/test',now()-interval '2 hour')`, id, model, index+1, string(raw)).Error)
		}
		require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.model_release(id,model_id,model_version_id,price_version_id,config_version_id,status,upstream_cost) VALUES(?,?,?,?,?,'active','{"currency":"CNY","max_micros":7560000}')`, release, model, version, sale, connection.Config.ID).Error)
		require.NoError(t, conn.DB.Exec("UPDATE catalog.model_profile SET active_release_id=? WHERE id=?", release, model).Error)
		seedOnboardingModeEvidence(t, conn.DB, project, cred.ID, catalogapp.OnboardingReleaseReview{ModelRelease: catalogapp.ModelRelease{ID: release}}, "text2video")
		releases = append(releases, release)
	}
	expected := catalogapp.PoolPublication{RequestID: uuid.New(), Backups: []uuid.UUID{releases[1]}, Equivalent: true}
	current, err := store.PublishRoutePool(t.Context(), actor, releases[0], expected)
	require.NoError(t, err)
	require.Equal(t, 1, current.Revision)
	replay, err := store.PublishRoutePool(t.Context(), actor, releases[0], expected)
	require.NoError(t, err)
	require.Equal(t, current, replay)
	expected.RequestID = uuid.New()
	_, err = store.PublishRoutePool(t.Context(), actor, releases[0], expected)
	require.ErrorIs(t, err, catalogapp.ErrPresetConflict)
	persisted, err := store.ReadRoutePool(t.Context(), actor, releases[0])
	require.NoError(t, err)
	require.Equal(t, current, persisted)
	t.Run("retail-only publication preserves old sale until the execution contract changes", func(t *testing.T) {
		release := releases[1]
		priceID, nextID := uuid.New(), uuid.New()
		require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,effective_from,source_url,reviewed_at)
 SELECT ?,p.model_profile_id,3,p.unit,p.rule||'{"discount_factor":"0.8"}'::jsonb,p.currency,p.effective_from,p.source_url,p.reviewed_at FROM catalog.model_release r JOIN catalog.price_rule_version p ON p.id=r.price_version_id WHERE r.id=?`, priceID, release).Error)
		require.NoError(t, conn.DB.Exec("UPDATE catalog.model_release SET status='retired' WHERE id=?", release).Error)
		require.NoError(t, conn.DB.Exec(`INSERT INTO catalog.model_release(id,model_id,model_version_id,price_version_id,config_version_id,status,upstream_cost) SELECT ?,model_id,model_version_id,?,config_version_id,'active',upstream_cost FROM catalog.model_release WHERE id=?`, nextID, priceID, release).Error)
		require.NoError(t, conn.DB.Exec("UPDATE catalog.model_profile SET active_release_id=? WHERE active_release_id=?", nextID, release).Error)
		var usable bool
		check := func(want bool) {
			t.Helper()
			require.NoError(t, conn.DB.Raw("SELECT EXISTS(SELECT 1 FROM catalog.route_pool_ready WHERE primary_release_id=? AND release_id=?)", releases[0], release).Row().Scan(&usable))
			require.Equal(t, want, usable)
		}
		check(true)
		require.NoError(t, conn.DB.Exec("UPDATE catalog.model_release SET upstream_cost='{}' WHERE id=?", nextID).Error)
		check(false)
		require.NoError(t, conn.DB.Exec("UPDATE catalog.model_release SET status='retired' WHERE id=?", nextID).Error)
		require.NoError(t, conn.DB.Exec("UPDATE catalog.model_release SET status='active' WHERE id=?", release).Error)
		require.NoError(t, conn.DB.Exec("UPDATE catalog.model_profile SET active_release_id=? WHERE active_release_id=?", release, nextID).Error)
	})
	if os.Getenv("GLOWLOOM_POOL_BROWSER") != "1" {
		return
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	api := router.Group("/api")
	cataloghttp.RoutePoolHandler{Store: store}.Register(api)
	api.POST("/admin/model-price-preview", ophttp.NewHandler(ophttp.Dependencies{}).PreviewSalePrice)
	vite, _ := url.Parse("http://127.0.0.1:5197")
	mux := http.NewServeMux()
	mux.Handle("/api/", router)
	mux.Handle("/", httputil.NewSingleHostReverseProxy(vite))
	server := httptest.NewServer(mux)
	defer server.Close()
	t.Log("BROWSER_URL", server.URL+"/tests/browser/pool-sale.html?id="+releases[0].String())
	<-time.After(3 * time.Minute)
}
