package aigateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/creation/prompt/domain"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/domain"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/adapter/http"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

const videoBillingProfile = `{"resolution":"720p","frames_per_second":24,"pixels_per_token":1024,"geometries":{"16:9":{"width":1280,"height":720}}}`

func TestVideoTokenEstimateUsesConfirmationMaximumWithoutReservingCredits(t *testing.T) {
	db, store, _, actor := fixture(t, 300)
	model := videoBillingModel(t, store)
	project := entrypointProject(t, db, actor)
	version, mode := model.ID.String(), "text2video"
	creation := aigateway.Creation{
		Runtime: &aigateway.Runtime{DB: db}, Authorize: fixtureAuthority,
	}
	draft := prompt.Draft{
		SchemaVersion: 1, ModelVersion: &version, Mode: &mode,
		Params: map[string]any{"duration": float64(4)},
		Document: prompt.Document{Type: "doc", Content: []prompt.Paragraph{{
			Type: "paragraph", Content: []prompt.Inline{{Type: "text", Text: "A silent test scene"}},
		}}},
		References: []prompt.Reference{},
		LabelState: prompt.LabelState{
			HighWater:   map[string]int64{"image": 0, "video": 0, "audio": 0, "subject": 0},
			Allocations: map[string]prompt.LabelAllocation{},
		},
	}
	estimate, err := creation.Estimate(t.Context(), actor, project, draft)
	require.NoError(t, err)
	require.Equal(t, "87", estimate.Credits)
	require.Equal(t, "100", estimate.MaximumCredits)
	require.Equal(t, "86400", estimate.Quantity)
	balance, err := store.Wallet(t.Context(), actor.ScopeID)
	require.NoError(t, err)
	require.EqualValues(t, 300, balance["available"])
	require.EqualValues(t, 0, balance["held"])
}

func videoBillingModel(t *testing.T, store *pg.Store) domain.Model {
	t.Helper()
	sale := domain.Price{Output: 100000000, Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1"}
	m := domain.Model{ID: uuid.New(), Name: "video-" + uuid.NewString(), Protocol: "media", Capability: "video.generate", Modes: []string{"text2video"}, OutputPolicy: "provider_success_v1", Price: sale, MaxInput: 1, MaxOutput: 100000, PurchaseCap: 6000000,
		Parameters: map[string]domain.Parameter{
			"resolution": {Type: "string", Default: json.RawMessage(`"720p"`)},
			"ratio":      {Type: "string", Default: json.RawMessage(`"16:9"`)},
			"duration":   {Type: "integer"},
		},
	}
	require.NoError(t, json.Unmarshal([]byte(`{"video_metering":`+videoBillingProfile+`}`), &m))
	require.NoError(t, store.PublishModel(t.Context(), m))
	spec, ok := catalog.GetAdapterSpec("volcengine_ark")
	require.True(t, ok)
	cfg := catalog.ProviderConfig{AdapterKey: "volcengine_ark", BaseURL: spec.AllowedBaseURL, Region: spec.AllowedRegion, Capabilities: []string{"video"}, TimeoutSeconds: 5, ConcurrencyLimit: 1, RateLimitPerMin: 60}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	purchase := sale
	purchase.Output = 50000000
	route := domain.Route{ID: uuid.New(), ModelID: m.ID, Supplier: uuid.New(), Credential: uuid.New(), Group: uuid.NewString(), Protocol: m.Protocol, BaseURL: cfg.BaseURL, UpstreamModel: "synthetic-video-sku", Purchase: purchase, DriverConfig: raw}
	require.NoError(t, store.PublishRoute(t.Context(), route, []byte("synthetic"), "fixture", 1, 60))
	return m
}

type videoBillingDownload struct{ bytes []byte }

func (d videoBillingDownload) Download(ctx context.Context, _ string) (*mediaapp.Downloaded, error) {
	return mediaapp.ReadUpload(ctx, bytes.NewReader(d.bytes), "fixture.mp4")
}

func (videoBillingDownload) DownloadAuthenticated(context.Context, string, string) (*mediaapp.Downloaded, error) {
	return nil, domain.ErrInvalid
}

func TestVideoTokenBillingThroughCreationAndOriginalPoll(t *testing.T) {
	for _, tc := range []struct {
		name, usage, state        string
		tokens, charged, released int64
	}{
		{"actual-usage", `{"completion_tokens":93600,"total_tokens":999999}`, "settled", 93600, 94, 6},
		{"confirmed-cap", `{"completion_tokens":112000}`, "settled", 112000, 100, 0},
		{"missing-usage", `{"total_tokens":93600}`, "unknown", 0, 0, 0},
		{"confirmed-failure", `{"completion_tokens":93600}`, "released", 93600, 0, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, store, _, actor := fixture(t, 300)
			m := videoBillingModel(t, store)
			project := entrypointProject(t, db, actor)
			output, err := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=s=1280x720:r=24", "-t", "4", "-c:v", "libx264", "-movflags", "frag_keyframe+empty_moov", "-f", "mp4", "pipe:1").Output()
			require.NoError(t, err)
			objects := &durableBytes{t: t, db: db, routes: pgmedia.NewStorageRoutes(db), files: map[string][]byte{}}
			var submissions, polls int
			client := mediaClient(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodPost {
					submissions++
					return mediaResponse(200, `{"id":"video-billing-fixture"}`), nil
				}
				polls++
				require.Equal(t, "/api/v3/contents/generations/tasks/video-billing-fixture", req.URL.Path)
				if tc.state == "released" {
					return mediaResponse(200, fmt.Sprintf(`{"id":"video-billing-fixture","status":"failed","usage":%s}`, tc.usage)), nil
				}
				return mediaResponse(200, fmt.Sprintf(`{"id":"video-billing-fixture","status":"succeeded","content":{"video_url":"https://fixture.tos-cn-beijing.volces.com/out.mp4"},"usage":%s}`, tc.usage)), nil
			})
			runtime := &aigateway.Runtime{DB: db, Objects: fixtureMediaStorage(t, db), Provider: client,
				ResultDownloads: videoBillingDownload{output},
				MediaArchive:    mediaapp.NewGeneratedOutputArchiver(objects.routes, objects, mediaflow.FFProber{}, mediaflow.FFRenderer{}),
			}
			creation := aigateway.Creation{Runtime: runtime, Authorize: fixtureAuthority}
			input := operationhttp.QuoteItemRequest{ModelKey: m.Name, Capability: m.Capability, Mode: "text2video", Prompt: "A silent test scene", Params: json.RawMessage(`{"duration":4}`), OutputCount: 1}
			quote, err := creation.Quote(t.Context(), actor, project, uuid.NewString(), input)
			require.NoError(t, err)
			require.EqualValues(t, 10000000, quote.QuoteMicros)
			require.Contains(t, string(quote.QuoteDetail), `"estimated_credits":87`)
			require.Contains(t, string(quote.QuoteDetail), `"estimated_tokens":86400`)
			input.Params = json.RawMessage(`{"duration":5}`)
			_, err = creation.Quote(t.Context(), actor, project, uuid.NewString(), input)
			require.ErrorIs(t, err, domain.ErrInvalid)
			before, err := store.Wallet(t.Context(), actor.ScopeID)
			require.NoError(t, err)
			require.EqualValues(t, 300, before["available"])
			for range 2 {
				_, err = creation.Confirm(t.Context(), actor, project, quote.OperationID, "video-confirm")
				require.NoError(t, err)
			}
			record, err := store.Read(t.Context(), actor.ScopeID, quote.OperationID)
			require.NoError(t, err)
			require.EqualValues(t, 100, record.Held)
			_, err = creation.Execute(t.Context(), taskruntime.Job{Kind: pg.ExecutionKind, BusinessID: quote.OperationID.String()})
			require.NoError(t, err)
			handlers := map[string]taskruntime.Handler{}
			runtime.RegisterWorker(handlers)
			run := func(kind string) {
				t.Helper()
				_, err := handlers[kind](t.Context(), taskruntime.Job{Kind: kind, BusinessID: quote.OperationID.String()})
				require.NoError(t, err)
			}
			run("ai.poll")
			record, err = store.Read(t.Context(), actor.ScopeID, quote.OperationID)
			require.NoError(t, err)
			require.Zero(t, record.Charged, "provider success is not delivery")
			if tc.state == "released" {
				require.Equal(t, tc.state, record.State)
				require.Equal(t, tc.released, record.Released)
				require.Equal(t, tc.tokens*50, record.Receipt.PurchaseMicros)
				require.Zero(t, objects.writes)
				require.Equal(t, 1, submissions)
				require.Equal(t, 1, polls)
				return
			}
			run("ai.deliver")
			run("ai.settle")
			run("ai.deliver")
			run("ai.poll")
			record, err = store.Read(t.Context(), actor.ScopeID, quote.OperationID)
			require.NoError(t, err)
			require.Equal(t, tc.state, record.State)
			require.Equal(t, tc.charged, record.Charged)
			require.Equal(t, tc.released, record.Released)
			require.Equal(t, tc.tokens, record.Receipt.Usage.Output)
			require.Equal(t, tc.tokens*50, record.Receipt.PurchaseMicros)
			task, err := creation.Task(t.Context(), actor, project, quote.OperationID)
			require.NoError(t, err)
			require.Equal(t, "ready", task.BillingOutcome.DeliveryState)
			require.Len(t, task.Outputs, 1)
			var freezes, charges int64
			require.NoError(t, db.Raw("SELECT count(*) FILTER (WHERE kind='freeze'),count(*) FILTER (WHERE kind='settle') FROM ai.money_event WHERE request_id=?", quote.OperationID).Row().Scan(&freezes, &charges))
			require.EqualValues(t, 1, freezes)
			if tc.tokens == 0 {
				require.True(t, task.BillingOutcome.ReconciliationRequired)
				require.Zero(t, charges)
				config, err := store.Configuration(t.Context())
				require.NoError(t, err)
				unknown, err := json.Marshal(config["unknown"])
				require.NoError(t, err)
				require.Contains(t, string(unknown), quote.OperationID.String())
				var attempt uuid.UUID
				require.NoError(t, db.Raw("SELECT id FROM ai.attempt WHERE request_id=?", quote.OperationID).Row().Scan(&attempt))
				evidence := pg.Reconciliation{Request: quote.OperationID, Attempt: attempt, Reference: "video-billing-fixture", Reason: "verified original task usage", Usage: &domain.Usage{Output: 93600}}
				wrong := evidence
				wrong.Reference = "another-task"
				_, err = store.Reconcile(t.Context(), wrong)
				require.ErrorIs(t, err, domain.ErrConflict)
				resolved, err := store.Reconcile(t.Context(), evidence)
				require.NoError(t, err)
				require.EqualValues(t, 94, resolved.Charged)
				require.EqualValues(t, 6, resolved.Released)
				replayed, err := store.Reconcile(t.Context(), evidence)
				require.NoError(t, err)
				require.Equal(t, resolved, replayed)
			} else {
				require.EqualValues(t, 1, charges)
			}
			require.Equal(t, 1, submissions)
			require.Equal(t, 1, polls)
		})
	}
}

func TestVideoTokenQuoteRequiresPublishedGeometry(t *testing.T) {
	d := mediaDispatch(t, "volcengine_ark", "video.generate", "text2video", "synthetic-video")
	d.Call.Model.Price.Output, d.Call.Model.Price.Request = 100000000, 0
	d.Call.Model.MaxOutput = 100000
	d.Call.Model.OutputPolicy = "provider_success_v1"
	d.Call.Model.Parameters = map[string]domain.Parameter{"resolution": {Type: "string", Default: json.RawMessage(`"720p"`)}, "duration": {Type: "integer"}, "ratio": {Type: "string"}}
	require.NoError(t, json.Unmarshal([]byte(`{"video_metering":`+videoBillingProfile+`}`), &d.Call.Model))
	mediaChange(t, &d, func(in *domain.MediaInput) { in.Params["duration"], in.Params["ratio"] = 4, "16:9" })
	_, err := application.Prepare(d.Call.Model, d.Call.Actor, d.Call.Scope, d.Call.Project, uuid.Nil, "video-quote", "test", d.Call.Raw)
	require.NoError(t, err)
	mediaChange(t, &d, func(in *domain.MediaInput) { in.Params["ratio"] = "adaptive" })
	_, err = application.Prepare(d.Call.Model, d.Call.Actor, d.Call.Scope, d.Call.Project, uuid.Nil, "video-quote", "test", d.Call.Raw)
	require.ErrorIs(t, err, domain.ErrInvalid)
}
