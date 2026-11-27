package aigateway_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	gatewayhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/http"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

const arkPresetVersion = "6da07a12-0000-4000-8000-000000000001/"

func TestSeedancePresetsCheckKeyAndEnableWithoutGeneratingOrCharging(t *testing.T) {
	for _, item := range []struct{ key, upstream string }{
		{"ark-video-mini", "doubao-seedance-2-0-mini-260615"},
		{"ark-video-fast", "doubao-seedance-2-0-fast-260128"},
		{"ark-video", "doubao-seedance-2-0-260128"},
		{"ark-video-25-flash", "doubao-seedance-2-5-flash-261115"},
		{"ark-video-25", "doubao-seedance-2-5-260628"},
	} {
		t.Run(item.key, func(t *testing.T) {
			db, fixtureStore, _, actor := fixture(t)
			ctx := t.Context()
			project := entrypointProject(t, db, actor)
			entrypointSession(t, db, actor)
			store := pg.New(db, actor, aigateway.Authority)
			require.NoError(t, db.Exec("UPDATE ai.account SET available=0 WHERE scope_id=?", actor.ScopeID).Error)
			upstreamStatus := http.StatusUnauthorized
			upstreamBody := `{"error":{"message":"provider-private-detail"}}`
			readCount := 0
			client := mediaClient(func(req *http.Request) (*http.Response, error) {
				readCount++
				require.Equal(t, http.MethodGet, req.Method)
				require.Equal(t, "ark.cn-beijing.volces.com", req.Host)
				require.Equal(t, "/api/v3/models", req.URL.Path)
				require.Equal(t, "Bearer simulated-key", req.Header.Get("Authorization"))
				require.True(t, req.Body == nil || req.Body == http.NoBody)
				return mediaResponse(upstreamStatus, upstreamBody), nil
			})
			engine := gin.New()
			api := engine.Group("/api")
			session := api.Group("", func(c *gin.Context) { c.Set("principal", actor) })
			gatewayhttp.New(db, aigateway.Authority, client, onboardingSealer{}, gatewayhttp.KeyVerification{Verify: client.VerifyProviderKey, MACKey: testProviderKeyMAC(t)}).Register(api, session)
			post := func(path string, input any) *httptest.ResponseRecorder {
				t.Helper()
				raw, err := json.Marshal(input)
				require.NoError(t, err)
				req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/ai/admin/onboarding/"+path, bytes.NewReader(raw))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, req)
				return w
			}
			in := domain.QuickOnboardingInput{PresetKey: arkPresetVersion + item.key, ScopeID: actor.ScopeID, ProjectID: project}
			prepared := post("preset", in)
			require.Equal(t, 200, prepared.Code, prepared.Body.String())
			var review pg.OnboardingReview
			require.NoError(t, json.Unmarshal(prepared.Body.Bytes(), &review))
			require.Equal(t, item.upstream, review.Upstream)
			require.Equal(t, "prepared", review.State)
			require.Equal(t, application.ReadOnlyKeyVerification, review.Plan.Method)
			require.Zero(t, review.Plan.Credits)
			require.Zero(t, review.Plan.PurchaseMicros)
			require.NotNil(t, review.Plan.Probes)
			require.Empty(t, review.Plan.Probes)
			require.Nil(t, review.Plan.EstimatedCredits)
			require.Zero(t, readCount)
			_, err := fixtureStore.Model(ctx, review.Upstream, "media")
			require.ErrorIs(t, err, domain.ErrUnavailable)

			if item.key == "ark-video-fast" {
				// Old unconfirmed drafts previously advertised a 120-credit video probe.
				legacyID := uuid.New()
				legacyCredential := uuid.NewSHA1(legacyID, []byte("credential"))
				require.NoError(t, db.Exec(`INSERT INTO ai.onboarding(id,model_id,previous_model_id,document)
				 SELECT ?,model_id,previous_model_id,
				        jsonb_set(jsonb_set(jsonb_set(document-'verification','{route,id}',to_jsonb(?::text)),
				            '{route,credential}',to_jsonb(?::text)),
				            '{plan}','{"credits":120,"purchase_micros":11988000,"probes":[{"name":"text2video","input":{}}]}'::jsonb)
				 FROM ai.onboarding WHERE id=?`, legacyID, legacyID.String(), legacyCredential.String(), review.ID).Error)
				review.ID = legacyID
				reread, err := store.ReviewOnboarding(ctx, review.ID)
				require.NoError(t, err)
				require.Equal(t, review.Plan, reread.Plan)
			}
			path := review.ID.String() + "/verify"
			budget := map[string]any{"key": "simulated-key", "credits": 120, "purchase_micros": 11988000}
			require.Equal(t, 402, post(path, budget).Code)
			require.Zero(t, readCount)
			budget["credits"], budget["purchase_micros"] = 0, 0
			rejected := post(path, budget)
			require.Equal(t, 422, rejected.Code)
			require.Contains(t, rejected.Body.String(), "provider_key_rejected")
			require.NotContains(t, rejected.Body.String(), "provider-private-detail")
			upstreamStatus = http.StatusForbidden
			denied := post(path, budget)
			require.Equal(t, 422, denied.Code)
			require.Contains(t, denied.Body.String(), "provider_key_scope")
			stillPrepared, err := store.ReviewOnboarding(ctx, review.ID)
			require.NoError(t, err)
			require.Equal(t, "prepared", stillPrepared.State)
			var routes int
			require.NoError(t, db.Raw("SELECT count(*) FROM ai.route WHERE id=?", review.ID).Scan(&routes).Error)
			require.Zero(t, routes)

			upstreamStatus, upstreamBody = http.StatusOK, `{"object":"list","data":[]}`
			confirmed := post(path, budget)
			require.Equal(t, 200, confirmed.Code, confirmed.Body.String())
			var active pg.OnboardingReview
			require.NoError(t, json.Unmarshal(confirmed.Body.Bytes(), &active))
			require.Equal(t, "active", active.State)
			require.True(t, active.Available)
			require.Equal(t, application.ReadOnlyKeyVerification, active.Plan.Method)
			require.NotContains(t, confirmed.Body.String(), "simulated-key")
			require.Equal(t, 200, post(path, budget).Code)
			require.Equal(t, 3, readCount, "an active route reuses the verification")
			otherProject := in
			otherProject.ProjectID = entrypointProject(t, db, actor)
			reused, err := store.PrepareFromPreset(ctx, otherProject)
			require.NoError(t, err)
			require.Equal(t, review.ID, reused.ID)
			current, err := fixtureStore.Model(ctx, review.Upstream, "media")
			require.NoError(t, err)
			require.Equal(t, review.ModelID, current.ID)

			var requests, budgets, charges int
			require.NoError(t, db.Raw(`SELECT
			 (SELECT count(*) FROM ai.request WHERE run_id=?),
			 (SELECT count(*) FROM ai.budget WHERE id=?),
			 (SELECT count(*) FROM ai.money_event WHERE scope_id=? AND kind IN ('freeze','settle'))`, review.ID, review.ID, actor.ScopeID).Row().Scan(&requests, &budgets, &charges))
			require.Zero(t, requests)
			require.Zero(t, budgets)
			require.Zero(t, charges)
			require.NoError(t, store.RollbackOnboarding(ctx, review.ID))
			_, err = fixtureStore.Model(ctx, review.Upstream, "media")
			require.ErrorIs(t, err, domain.ErrUnavailable)
			retry, err := store.PrepareFromPreset(ctx, in)
			require.NoError(t, err)
			require.NotEqual(t, review.ID, retry.ID)
			require.Equal(t, "prepared", retry.State)
		})
	}
}

func TestSeedanceKeyVerificationRejectsCallerChosenEndpoint(t *testing.T) {
	d := mediaDispatch(t, "volcengine_ark", "video.generate", "text2video", "doubao-seedance-2-0-fast-260128")
	d.Route.BaseURL = "https://other.example/api/v3"
	client := mediaClient(func(*http.Request) (*http.Response, error) {
		t.Fatal("credentials must only be sent to the official Ark origin")
		return nil, nil
	})
	require.ErrorIs(t, client.VerifyProviderKey(t.Context(), d.Route, "simulated-key"), domain.ErrInvalid)

	db, store, _, actor := fixture(t)
	input := domain.QuickOnboardingInput{
		PresetKey: arkPresetVersion + "ark-video-fast",
		ScopeID:   uuid.New(), ProjectID: entrypointProject(t, db, actor),
	}
	_, err := store.PrepareFromPreset(t.Context(), input)
	require.ErrorIs(t, err, domain.ErrForbidden)
}
