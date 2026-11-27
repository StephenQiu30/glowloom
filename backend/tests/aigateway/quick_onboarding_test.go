package aigateway_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	gatewayhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/http"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

var seedancePresetCases = []struct {
	key, upstream                      string
	estimate, maximum, purchaseMaximum int64
	charged, purchaseActual            int64
}{
	{"ark-video-mini", "doubao-seedance-2-0-mini-260615", 25, 75, 7_452_000, 26, 2_504_700},
	{"ark-video-fast", "doubao-seedance-2-0-fast-260128", 40, 120, 11_988_000, 41, 4_029_300},
	{"ark-video", "doubao-seedance-2-0-260128", 50, 150, 14_904_000, 51, 5_009_400},
	{"ark-video-25-flash", "doubao-seedance-2-5-flash-261115", 50, 299, 29_808_000, 51, 5_009_400},
	{"ark-video-25", "doubao-seedance-2-5-260628", 76, 454, 45_360_000, 77, 7_623_000},
}

const arkPresetVersion = "6da07a12-0000-4000-8000-000000000001/"

func TestSeedancePresetsPrepareFromAnEmptyCatalogueOfCandidates(t *testing.T) {
	db, store, _, actor := fixture(t, 1_000)
	project := entrypointProject(t, db, actor)
	ctx := t.Context()
	for _, item := range seedancePresetCases {
		t.Run(item.key, func(t *testing.T) {
			key := arkPresetVersion + item.key
			req := domain.QuickOnboardingInput{PresetKey: key, ScopeID: actor.ScopeID, ProjectID: project}
			first, err := store.PrepareFromPreset(ctx, req)
			require.NoError(t, err)
			require.Equal(t, item.upstream, first.Upstream)
			require.Equal(t, "prepared", first.State)
			require.Equal(t, item.maximum, first.Plan.Credits)
			require.Equal(t, item.purchaseMaximum, first.Plan.PurchaseMicros)
			require.Len(t, first.Plan.Probes, 1)
			require.Equal(t, "text2video", first.Plan.Probes[0].Name)
			var probe domain.MediaInput
			require.NoError(t, json.Unmarshal(first.Plan.Probes[0].Raw, &probe))
			require.EqualValues(t, 5, probe.Params["duration"])
			require.Equal(t, "720p", probe.Params["resolution"])
			require.Empty(t, probe.References)
			again, err := store.PrepareFromPreset(ctx, req)
			require.NoError(t, err)
			require.Equal(t, first.ID, again.ID)
			_, err = store.PrepareFromPreset(ctx, domain.QuickOnboardingInput{PresetKey: key, ScopeID: uuid.New(), ProjectID: project})
			require.ErrorIs(t, err, domain.ErrForbidden)
		})
	}
	var count int
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.request WHERE scope_id=?", actor.ScopeID).Scan(&count).Error)
	require.Zero(t, count)
	balance, err := store.Wallet(ctx, actor.ScopeID)
	require.NoError(t, err)
	require.EqualValues(t, 1_000, balance["available"])
	require.EqualValues(t, 0, balance["held"])
}

func TestSeedancePresetsActivateOnlyAfterOriginalVideoAndUsage(t *testing.T) {
	output, err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=s=1280x720:r=24",
		"-t", "5", "-c:v", "libx264", "-movflags", "frag_keyframe+empty_moov", "-f", "mp4", "pipe:1").Output()
	require.NoError(t, err)
	for _, item := range seedancePresetCases {
		t.Run(item.key, func(t *testing.T) {
			db, fixtureStore, _, actor := fixture(t, 1000)
			ctx := t.Context()
			project := entrypointProject(t, db, actor)
			entrypointSession(t, db, actor)
			store := pg.New(db, actor, aigateway.Authority)
			engine := gin.New()
			api := engine.Group("/api")
			session := api.Group("", func(c *gin.Context) { c.Set("principal", actor) })
			gatewayhttp.New(db, aigateway.Authority, nil, onboardingSealer{}).Register(api, session)
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
			require.NotNil(t, review.Plan.EstimatedCredits)
			require.EqualValues(t, item.estimate, *review.Plan.EstimatedCredits)
			_, err := fixtureStore.Model(ctx, review.Upstream, "media")
			require.ErrorIs(t, err, domain.ErrUnavailable)
			otherProject := in
			otherProject.ProjectID = entrypointProject(t, db, actor)
			unconfirmed, err := store.PrepareFromPreset(ctx, otherProject)
			require.NoError(t, err)
			require.NotEqual(t, review.ID, unconfirmed.ID)
			budget := map[string]any{"key": "simulated-key", "credits": item.estimate, "purchase_micros": review.Plan.PurchaseMicros}
			require.Equal(t, 402, post(review.ID.String()+"/verify", budget).Code)
			budget["credits"] = review.Plan.Credits
			confirmed := post(review.ID.String()+"/verify", budget)
			require.Equal(t, 202, confirmed.Code, confirmed.Body.String())
			inFlight, err := store.PrepareFromPreset(ctx, otherProject)
			require.NoError(t, err)
			require.Equal(t, review.ID, inFlight.ID)
			require.Equal(t, "verifying", inFlight.State)
			reused, err := store.ConfirmOnboarding(ctx, unconfirmed.ID, unconfirmed.Plan.Credits, unconfirmed.Plan.PurchaseMicros, func(uuid.UUID, uuid.UUID) ([]byte, string, error) {
				t.Error("an equivalent verification must not seal a second credential")
				return nil, "", domain.ErrInvalid
			})
			require.NoError(t, err)
			require.Equal(t, review.ID, reused)
			objects := &durableBytes{t: t, db: db, routes: pgmedia.NewStorageRoutes(db), files: map[string][]byte{}}
			sends, polls := 0, 0
			client := mediaClient(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodPost {
					sends++
					var payload map[string]any
					require.NoError(t, json.NewDecoder(req.Body).Decode(&payload))
					require.Equal(t, review.Upstream, payload["model"])
					require.EqualValues(t, 5, payload["duration"])
					require.Equal(t, "720p", payload["resolution"])
					require.Equal(t, "16:9", payload["ratio"])
					return mediaResponse(200, `{"id":"seedance-quick-fixture"}`), nil
				}
				polls++
				require.Equal(t, "/api/v3/contents/generations/tasks/seedance-quick-fixture", req.URL.Path)
				return mediaResponse(200, `{"id":"seedance-quick-fixture","status":"succeeded","content":{"video_url":"https://fixture.tos-cn-beijing.volces.com/out.mp4"},"usage":{"completion_tokens":108900}}`), nil
			})
			runtime := &aigateway.Runtime{DB: db, Objects: fixtureMediaStorage(t, db), Provider: client,
				ResultDownloads: videoBillingDownload{output},
				MediaArchive:    mediaapp.NewGeneratedOutputArchiver(objects.routes, objects, mediaflow.FFProber{}, mediaflow.FFRenderer{}),
			}
			jobs := map[string]taskruntime.Handler{}
			runtime.RegisterWorker(jobs)
			run := func(kind string, id uuid.UUID) {
				t.Helper()
				_, err := jobs[kind](ctx, taskruntime.Job{Kind: kind, BusinessID: id.String()})
				require.NoError(t, err)
			}
			run(pg.OnboardingKind, review.ID)
			run(pg.OnboardingKind, review.ID)
			pending, err := store.ReviewOnboarding(ctx, review.ID)
			require.NoError(t, err)
			require.Len(t, pending.Probes, 1)
			require.Equal(t, "pending", pending.Probes[0].State)
			request := pending.Probes[0].RequestID
			run("ai.poll", request)
			run("ai.deliver", request)
			run("ai.settle", request)
			run(pg.OnboardingKind, review.ID)
			current, err := fixtureStore.Model(ctx, review.Upstream, "media")
			require.NoError(t, err)
			require.Equal(t, review.ModelID, current.ID)
			var available int
			require.NoError(t, db.Raw("SELECT count(*) FROM ai.available_model WHERE id=? AND route_id=?", current.ID, review.ID).Scan(&available).Error)
			require.Equal(t, 1, available)
			again, err := store.PrepareFromPreset(ctx, otherProject)
			require.NoError(t, err)
			require.Equal(t, review.ID, again.ID)
			require.True(t, again.Available)
			record, err := fixtureStore.Read(ctx, actor.ScopeID, request)
			require.NoError(t, err)
			require.Equal(t, "settled", record.State)
			require.EqualValues(t, item.charged, record.Charged)
			require.EqualValues(t, item.maximum-item.charged, record.Released)
			require.EqualValues(t, item.purchaseActual, record.Receipt.PurchaseMicros)
			require.Positive(t, objects.writes)
			require.Equal(t, 1, sends)
			require.Equal(t, 1, polls)
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
