package aigateway_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	gatewayhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/http"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

func TestArkVideoEstimateMatchesOfficialPricing(t *testing.T) {
	db, store, _, actor := fixture(t, 300)
	ctx := t.Context()
	engine := gin.New()
	api := engine.Group("/api")
	session := api.Group("", func(c *gin.Context) { c.Set("principal", actor) })
	gatewayhttp.New(db, fixtureAuthority, nil, nil).Register(api, session)
	const standard = "doubao-seedance-2-5-260628"
	const mini = "doubao-seedance-2-0-mini-260615"
	for _, tc := range []struct {
		name, model, resolution, ratio string
		reference, output, count       int64
		want                           string
	}{
		{"console screenshot", standard, "720p", "16:9", 5, 15, 1, "22.68"},
		{"console long reference", standard, "720p", "16:9", 15, 15, 1, "27.22"},
		{"console fractional threshold", standard, "720p", "16:9", 2, 4, 1, "6.35"},
		{"round total, not each video", standard, "720p", "16:9", 15, 15, 3, "81.65"},
		{"official short reference table", standard, "720p", "16:9", 2, 5, 1, "8.16"},
		{"official 4k table", standard, "4k", "16:9", 2, 5, 1, "33.24"},
		{"official mini without reference", mini, "720p", "16:9", 0, 5, 1, "2.48"},
		{"official mini 480p geometry", mini, "480p", "16:9", 2, 5, 1, "1.27"},
		{"portrait geometry", standard, "720p", "9:16", 5, 15, 1, "22.68"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(domain.ArkVideoEstimateInput{
				Model: tc.model, Resolution: tc.resolution, Ratio: tc.ratio,
				ReferenceSeconds: tc.reference, OutputSeconds: tc.output, Count: tc.count,
			})
			require.NoError(t, err)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/ai/admin/ark-video-estimate", bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var got domain.ArkVideoEstimate
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &got))
			require.Equal(t, tc.want, got.AmountCNY)
		})
	}

	in := domain.ArkVideoEstimateInput{Model: mini, Resolution: "720p", Ratio: "16:9", OutputSeconds: 5, Count: 1}
	nonAdmin := actor
	nonAdmin.PlatformRole = "creator"
	require.NoError(t, db.Exec(`UPDATE identity."user" SET platform_role = 'creator' WHERE id = ?`, actor.ID).Error)
	_, err := pg.New(db, nonAdmin, fixtureAuthority).EstimateArkVideo(ctx, in)
	require.ErrorIs(t, err, domain.ErrForbidden)
	require.NoError(t, db.Exec(`UPDATE identity."user" SET platform_role = 'platform_admin' WHERE id = ?`, actor.ID).Error)
	for _, change := range []func(*domain.ArkVideoEstimateInput){
		func(in *domain.ArkVideoEstimateInput) { in.Resolution = "4k" },
		func(in *domain.ArkVideoEstimateInput) { in.Ratio = "adaptive" },
		func(in *domain.ArkVideoEstimateInput) { in.ReferenceSeconds = 1 },
		func(in *domain.ArkVideoEstimateInput) { in.OutputSeconds = 16 },
		func(in *domain.ArkVideoEstimateInput) { in.Count = 0 },
		func(in *domain.ArkVideoEstimateInput) { in.Model = "unknown" },
	} {
		invalid := in
		change(&invalid)
		_, err := store.EstimateArkVideo(ctx, invalid)
		require.ErrorIs(t, err, domain.ErrInvalid)
	}
	configuration, err := store.Configuration(ctx)
	require.NoError(t, err)
	presets, err := json.Marshal(configuration["model_presets"])
	require.NoError(t, err)
	require.Contains(t, string(presets), `"cny_per_million_with_video":42`)
	requests := int64(-1)
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.request WHERE scope_id=?", actor.ScopeID).Scan(&requests).Error)
	require.Zero(t, requests)
	balance, err := store.Wallet(ctx, actor.ScopeID)
	require.NoError(t, err)
	require.EqualValues(t, 300, balance["available"])
	require.EqualValues(t, 0, balance["held"])
}
