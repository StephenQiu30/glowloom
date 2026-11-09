package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/storageoperator"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

// This regression uses the production router, real session/scope middleware and
// real runtime role. Catalog evidence and funds are synthetic and isolated; it
// never starts a worker or calls a supplier.
func TestBusinessRouterComposesExecutionAvailabilityForQuotesAndConfirmation(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_AVAILABILITY_HTTP_DB_DSN")
	if dsn == "" {
		t.Skip("requires the task-owned availability database")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Equal(t, "/glowloom_availability_20261109", parsed.Path, "never run this fixture against a business database")
	owner, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, owner.Close()) })
	var database string
	require.NoError(t, owner.DB.Raw(`SELECT current_database()`).Row().Scan(&database))
	require.Equal(t, "glowloom_availability_20261109", database)
	query := parsed.Query()
	query.Set("options", "-c role=glowloom_app")
	parsed.RawQuery = query.Encode()
	runtime, err := db.Open(t.Context(), parsed.String(), noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	var role string
	require.NoError(t, runtime.DB.Raw(`SELECT current_user`).Row().Scan(&role))
	require.Equal(t, "glowloom_app", role)
	cfg := identityfixture.Configure(t, config.Config{Env: "local", HTTPAddr: "127.0.0.1:18089", PublicOrigin: "http://localhost:3000", ObjectStorage: businessUnitStorage(t)})
	environment, err := storageoperator.Configuration(*cfg.ObjectStorage)
	require.NoError(t, err)
	_, err = pgmedia.NewStorageRoutes(owner.DB).ApplyEnvironment(t.Context(), environment)
	require.NoError(t, err)
	account := identityfixture.Create(t, runtime.DB, cfg, "Synthetic availability HTTP", "SyntheticAvailability123")
	scope := identityfixture.Scope(t, runtime.DB, account.User.ID)
	router, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), cfg, runtime.DB, businessUnitRegistry(t), identityruntime.SessionDependencies{})
	require.NoError(t, err)
	call := func(method, path string, body any, key string) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", cfg.PublicOrigin)
		req.Header.Set("X-CSRF-Token", account.CSRFToken)
		req.Header.Set("X-Creation-Scope", scope.String())
		req.Header.Set("Idempotency-Key", key)
		req.AddCookie(&http.Cookie{Name: "glowloom_session", Value: account.Token})
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out
	}
	created := call("POST", "/api/projects", map[string]string{"name": "Synthetic availability", "aspect_ratio": "16:9", "style_type": "realistic"}, uuid.NewString())
	require.Equal(t, 201, created.Code, created.Body.String())
	var project struct {
		ID uuid.UUID `json:"id"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &project))
	require.NotEqual(t, uuid.Nil, project.ID)
	require.NoError(t, owner.DB.Exec(`UPDATE workspace.project SET allow_overseas_models=true WHERE id=?`, project.ID).Error)
	require.NoError(t, owner.DB.Exec(`UPDATE billing.budget SET limit_micros=1000000 WHERE project_id=?`, project.ID).Error)
	var payer, lot uuid.UUID
	require.NoError(t, owner.DB.Raw(`SELECT billing.bind_project_wallet(?)`, project.ID).Row().Scan(&payer))
	require.NoError(t, owner.DB.Raw(`SELECT billing.issue_promotional_credit(?,?,1000000,now()+interval '1 hour',NULL,'synthetic router regression')`, payer, uuid.New()).Row().Scan(&lot))
	provider, configuration, credential, model, version, purchase, sale, release, validation, evidence := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	modelKey := "000-router-availability-" + model.String()
	require.NoError(t, owner.DB.Exec(`INSERT INTO catalog.provider(id,key,name,adapter_key,region,status) VALUES(?,?,'Synthetic HTTP fixture','openrouter','overseas','active')`, provider, "router-"+provider.String()).Error)
	require.NoError(t, owner.DB.Exec(`INSERT INTO catalog.provider_config_version(id,provider_id,preset_version_id,version_no,config,content_hash) VALUES(?,?,'6da07a11-0000-4000-8000-000000000002',1,'{"adapter_key":"openrouter","base_url":"https://openrouter.ai/api/v1","region":"overseas","capabilities":["text"],"timeout_seconds":30,"concurrency_limit":1,"rate_limit_per_min":10}',repeat('a',64))`, configuration, provider).Error)
	require.NoError(t, owner.DB.Exec(`UPDATE catalog.provider SET current_config_version_id=? WHERE id=?`, configuration, provider).Error)
	require.NoError(t, owner.DB.Exec(`INSERT INTO catalog.provider_credential(id,provider_id,label,last4,ciphertext,key_id,status) VALUES(?,?,'Synthetic only','test',decode('abcd','hex'),'synthetic','active')`, credential, provider).Error)
	require.NoError(t, owner.DB.Exec(`INSERT INTO catalog.model_profile(id,model_key,provider_id,capability,display_name,status) VALUES(?,?,?,'text.generate','Synthetic HTTP model','active')`, model, modelKey, provider).Error)
	require.NoError(t, owner.DB.Exec(`INSERT INTO catalog.model_profile_version(id,model_profile_id,version_no,provider_model_id,modes,limits,param_schema,supports_query,supports_cancel,supports_callback,expected_max_ms,moderation,queue) VALUES(?,?,1,'synthetic/text',ARRAY['text_to_text'],'{"max_outputs":1}','[{"field":"max_tokens","label":"最大输出","type":"integer","component":"input","min":1,"max":32768}]',false,false,false,60000,'provider','provider')`, version, model).Error)
	require.NoError(t, owner.DB.Exec(`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,effective_from,source_url,reviewed_at) VALUES(?,?,1,'per_request','{"base_micros":10}','CNY',now()-interval '1 hour','https://example.invalid/router-pricing',now()-interval '1 hour')`, purchase, model).Error)
	rule := `{"base_micros":20,"procurement_price_version_id":"` + purchase.String() + `"}`
	require.NoError(t, owner.DB.Exec(`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,effective_from,source_url,reviewed_at) VALUES(?,?,2,'per_request',?::jsonb,'CNY',now()-interval '1 hour','https://example.invalid/router-pricing',now()-interval '1 hour')`, sale, model, rule).Error)
	require.NoError(t, owner.DB.Exec(`INSERT INTO catalog.model_release(id,model_id,model_version_id,price_version_id,config_version_id,status,upstream_cost) VALUES(?,?,?,?,?,'active','{}')`, release, model, version, sale, configuration).Error)
	require.NoError(t, owner.DB.Exec(`UPDATE catalog.model_profile SET current_version_id=?,active_release_id=? WHERE id=?`, version, release, model).Error)
	require.NoError(t, owner.DB.Exec(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,input_hash,origin,status,region,model_profile_version_id,price_rule_version_id) VALUES(?,?,'free','text.generate','text_to_text',?,'system','completed','overseas',?,?)`, evidence, project.ID, uuid.NewString(), version, sale).Error)
	require.NoError(t, owner.DB.Exec(`INSERT INTO operation.provider_binding(operation_id,release_id) VALUES(?,?)`, evidence, release).Error)
	require.NoError(t, owner.DB.Exec(`INSERT INTO operation.provider_credential_binding(operation_id,credential_id) VALUES(?,?)`, evidence, credential).Error)
	require.NoError(t, owner.DB.Exec(`INSERT INTO catalog.model_validation(id,release_id,credential_id,adapter_version,mode,input_digest,operation_id,status,evidence,completed_at) VALUES(?,?,?,'ai-provider-v1','text_to_text',?,?,'succeeded','{"kind":"real_provider"}',now())`, validation, release, credential, uuid.NewString(), evidence).Error)
	// Follow the production keyset cursor so prior isolated test runs cannot
	// hide this fixture behind the first catalog page.
	catalogPath := "/api/models?project_id=" + project.ID.String() + "&capability=text.generate&mode=text_to_text&limit=200"
	found := false
	for page := 0; page < 1000; page++ {
		catalog := call("GET", catalogPath, nil, uuid.NewString())
		require.Equal(t, 200, catalog.Code, catalog.Body.String())
		var visible struct {
			Items []struct {
				ID        uuid.UUID `json:"id"`
				Available bool      `json:"available"`
			} `json:"items"`
			NextCursor *string `json:"next_cursor"`
		}
		require.NoError(t, json.Unmarshal(catalog.Body.Bytes(), &visible))
		for _, item := range visible.Items {
			if item.ID == model {
				found = item.Available
			}
		}
		if found || visible.NextCursor == nil {
			break
		}
		catalogPath = "/api/models?project_id=" + project.ID.String() + "&capability=text.generate&mode=text_to_text&limit=200&cursor=" + url.QueryEscape(*visible.NextCursor)
	}
	require.True(t, found, "real catalog must independently validate the synthetic fixture")
	body := map[string]any{"model_key": modelKey, "capability": "text.generate", "mode": "text_to_text", "prompt": "Synthetic bounded quote only", "params": map[string]any{"max_tokens": 100}, "output_count": 1, "force_regenerate": true, "media_inputs": []any{}}
	path := "/api/projects/" + project.ID.String() + "/free-operations"
	quoted := call("POST", path, body, uuid.NewString())
	require.Equal(t, 201, quoted.Code, quoted.Body.String())
	var quote struct {
		ID uuid.UUID `json:"operation_id"`
	}
	require.NoError(t, json.Unmarshal(quoted.Body.Bytes(), &quote))
	require.NotEqual(t, uuid.Nil, quote.ID)
	require.NoError(t, owner.DB.Exec(`UPDATE catalog.model_validation SET status='failed' WHERE id=?`, validation).Error)
	rejected := call("POST", path, body, uuid.NewString())
	require.Equal(t, 409, rejected.Code, rejected.Body.String())
	require.Contains(t, rejected.Body.String(), `"code":"model_unavailable"`)
	confirm := call("POST", "/api/operations/"+quote.ID.String()+"/confirm", map[string]any{"project_id": project.ID}, uuid.NewString())
	require.Equal(t, 409, confirm.Code, confirm.Body.String())
	require.Contains(t, confirm.Body.String(), `"code":"quote_stale"`)
	var reservations, quotes int64
	require.NoError(t, owner.DB.Raw(`SELECT count(*) FROM billing.reservation WHERE project_id=?`, project.ID).Row().Scan(&reservations))
	require.Zero(t, reservations)
	require.NoError(t, owner.DB.Raw(`SELECT count(*) FROM operation.operation WHERE project_id=? AND status<>'completed'`, project.ID).Row().Scan(&quotes))
	require.EqualValues(t, 1, quotes, "unavailable quote must not persist another operation")
}
