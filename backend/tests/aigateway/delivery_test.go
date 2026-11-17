package aigateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	catalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/domain"
	pg "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
	media "github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

func TestGatewayGeneratedSourceMustBeExactlyOneAuthority(t *testing.T) {
	id, project, request := uuid.New(), uuid.New(), uuid.New()
	provider, model, region := "supplier", "image-model", "overseas"
	now := time.Now().UTC()
	a := media.MediaAsset{ID: id, ProjectID: project, Kind: media.KindImage, Origin: media.OriginGenerated, Status: media.StatusProcessing, ObjectKey: "projects/" + project.String() + "/image/2026/11/" + id.String() + ".png", MimeType: "image/png", ByteSize: 64, SourceAIRequestID: &request, ProviderKey: &provider, ModelKey: &model, Region: &region, ModerationStatus: media.ModerationPending, Revision: 1, CreateTime: now, UpdateTime: now}
	require.NoError(t, a.Validate())
	a.SourceOperationID = &request
	require.ErrorIs(t, a.Validate(), media.ErrInvalidMediaAsset)
	a.SourceAIRequestID = nil
	require.NoError(t, a.Validate())
	a.SourceOperationID = nil
	require.ErrorIs(t, a.Validate(), media.ErrInvalidMediaAsset)
}

// durableBytes is a test object backend, retaining real bytes and the production
// PostgreSQL object ledger. FFmpeg and both Media metadata acceptors stay real.
type durableBytes struct {
	t        *testing.T
	db       *gorm.DB
	routes   *pgmedia.StorageRoutes
	files    map[string][]byte
	mu       sync.Mutex
	writes   int
	failOnce bool
}

func (o *durableBytes) CreateOperationObject(ctx context.Context, intent mediaapp.StorageWorkIntent, key string, file *mediaapp.Downloaded) (media.ObjectLocation, error) {
	return o.write(ctx, intent, key, file)
}
func (o *durableBytes) CreateGeneratedAudioObject(ctx context.Context, intent mediaapp.StorageWorkIntent, key string, file *mediaapp.Downloaded) (media.ObjectLocation, error) {
	return o.write(ctx, intent, key, file)
}
func (o *durableBytes) StatOperationObject(_ context.Context, l media.ObjectLocation) (mediaapp.ObjectInfo, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	b, ok := o.files[l.Ref.Key]
	if !ok {
		return mediaapp.ObjectInfo{}, mediaapp.ErrUnavailable
	}
	return mediaapp.ObjectInfo{Size: int64(len(b)), ContentType: *l.MIMEType, SHA256: domain.Digest(b)}, nil
}
func (o *durableBytes) write(ctx context.Context, intent mediaapp.StorageWorkIntent, key string, file *mediaapp.Downloaded) (media.ObjectLocation, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	var n int64
	require.NoError(o.t, o.db.WithContext(ctx).Raw("SELECT count(*) FROM ai.output WHERE request_id=? AND state='preparing' AND sha256 ~ '^[a-f0-9]{64}$' AND manifest_sha256 ~ '^[a-f0-9]{64}$'", intent.Owner.WorkID).Scan(&n).Error)
	require.EqualValues(o.t, 1, n, "manifest must commit before every object write")
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	l, err := o.routes.ReserveWorkLocation(ctx, intent.ID, key)
	if err != nil {
		return l, err
	}
	if err = o.routes.BeginWorkWrite(ctx, intent.ID, l); err != nil {
		return l, err
	}
	b, err := io.ReadAll(io.NewSectionReader(file.File, 0, file.Size))
	if err != nil {
		return l, err
	}
	if prior, ok := o.files[key]; ok {
		if !bytes.Equal(prior, b) {
			return l, mediaapp.ErrObjectMismatch
		}
	} else {
		o.files[key] = b
		o.writes++
	}
	if o.failOnce {
		o.failOnce = false
		return l, mediaapp.ErrUnavailable
	}
	info := mediaapp.ObjectInfo{Size: int64(len(b)), ContentType: file.MIMEType, SHA256: domain.Digest(b)}
	if info.Size != file.Size || info.SHA256 != file.SHA256 {
		return l, mediaapp.ErrObjectMismatch
	}
	if err = o.routes.ConfirmWorkLocation(ctx, intent.ID, l, info); err != nil {
		return l, err
	}
	l.State = media.ObjectLocationPresent
	l.ByteSize = &info.Size
	l.MIMEType = &info.ContentType
	l.SHA256 = &info.SHA256
	return l, nil
}
func fixtureMediaStorage(t *testing.T, db *gorm.DB) *objectstorage.Registry {
	t.Helper()
	routes := pgmedia.NewStorageRoutes(db)
	profile := mediaapp.StorageProfile{ID: "ai-delivery-fixture", Provider: "cos", Endpoint: "https://ai-delivery-fixture-1250000000.cos.ap-beijing.myqcloud.com", SigningEndpoint: "https://ai-delivery-fixture-1250000000.cos.ap-beijing.myqcloud.com", Region: "ap-beijing", Bucket: "ai-delivery-fixture-1250000000"}
	require.NoError(t, routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{profile}))
	require.NoError(t, db.Exec("INSERT INTO media.storage_config_revision(revision,config_sha256,profile_id,upload_policy,session_ttl_seconds) VALUES('1',repeat('f',64),?,'{}',86400) ON CONFLICT DO NOTHING", profile.ID).Error)
	require.NoError(t, db.Exec("INSERT INTO media.storage_route(singleton,revision,profile_id) VALUES(1,'1',?) ON CONFLICT(singleton) DO UPDATE SET revision='1',profile_id=excluded.profile_id", profile.ID).Error)
	cfg := config.ObjectStorageConfig{SchemaVersion: 1, Revision: "1", DefaultWriteProfile: profile.ID, Profiles: map[string]config.StorageProfile{profile.ID: {Provider: "cos", Mode: "read_write", Endpoint: profile.Endpoint, SigningEndpoint: profile.SigningEndpoint, Bucket: profile.Bucket, Region: profile.Region, Credentials: config.StorageCredentialsRef{Source: "env", AccessKeyEnv: "AI_DELIVERY_FIXTURE_ACCESS", SecretKeyEnv: "AI_DELIVERY_FIXTURE_SECRET"}}}, Upload: config.StorageUploadPolicy{Mode: "relay", SignedURLTTL: 10 * time.Minute, SessionTTL: 24 * time.Hour, MultipartThresholdBytes: 32 << 20, PartSizeBytes: 8 << 20, PartConcurrency: 3, StagingRetention: 48 * time.Hour}, Access: config.StorageAccessPolicy{PreviewTTL: 5 * time.Minute, MaxSignedURLTTL: 15 * time.Minute}, Limits: config.StorageLimits{MaxImageBytes: 20 << 20, MaxVideoBytes: 500 << 20, MaxAudioBytes: 100 << 20, MaxModelBytes: 64 << 20, MaxDocumentBytes: 20 << 20}}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	validated, err := config.LoadObjectStorageEnvironment(string(raw), "test", func(name string) string {
		if name == "" {
			return ""
		}
		return "isolated-fixture"
	})
	require.NoError(t, err)
	registry, err := objectstorage.OpenProfiles(validated)
	require.NoError(t, err)
	t.Cleanup(registry.Close)
	return registry
}
func mediaModel(t *testing.T, store *pg.Store, capability, policy string) domain.Model {
	t.Helper()
	price := domain.Price{Request: 100000, Currency: "CNY", FX: "1", Multiplier: "1", Discount: "1"}
	m := domain.Model{ID: uuid.New(), Name: "media-" + uuid.NewString(), Protocol: "media", Capability: capability, Modes: []string{"text2image"}, OutputPolicy: policy, Price: price, MaxInput: 8192, MaxOutput: 1000, PurchaseCap: 100000}
	if capability == "audio.generate" {
		m.Modes = []string{"text2audio"}
		m.Parameters = map[string]domain.Parameter{"speaker": {Type: "string", Enum: []json.RawMessage{json.RawMessage(`"fixture-speaker"`)}, Default: json.RawMessage(`"fixture-speaker"`)}}
	}
	require.NoError(t, store.PublishModel(t.Context(), m))
	adapter := "volcengine_ark"
	if capability == "audio.generate" {
		adapter = "volcengine_speech"
	}
	spec, ok := catalog.GetAdapterSpec(adapter)
	require.True(t, ok)
	config := catalog.ProviderConfig{AdapterKey: adapter, BaseURL: spec.AllowedBaseURL, Region: spec.AllowedRegion, Capabilities: []string{strings.TrimSuffix(capability, ".generate")}, TimeoutSeconds: 5, ConcurrencyLimit: 1, RateLimitPerMin: 60}
	require.NoError(t, config.Validate())
	configRaw, err := json.Marshal(config)
	require.NoError(t, err)
	route := domain.Route{ID: uuid.New(), ModelID: m.ID, Supplier: uuid.New(), Credential: uuid.New(), Group: uuid.NewString(), Protocol: "media", BaseURL: config.BaseURL, UpstreamModel: "synthetic-media", Purchase: price, DriverConfig: configRaw}
	require.NoError(t, store.PublishRoute(t.Context(), route, []byte("synthetic"), "fixture", 2, 10000))
	return m
}
func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 32, 32))))
	return b.Bytes()
}
func paidMediaReceipt(t *testing.T, store *pg.Store, c domain.Call, body []byte, mime string) (application.Dispatch, domain.Receipt) {
	t.Helper()
	_, err := store.Quote(t.Context(), c)
	require.NoError(t, err)
	d, err := store.Admit(t.Context(), c)
	require.NoError(t, err)
	result := operation.GenerationResult{Outcome: operation.ProviderOutcomeCompleted, State: operation.ProviderTaskSucceeded, TaskID: "fixture-original-task", Usage: json.RawMessage(`{"provider_native":"retained"}`)}
	if strings.HasPrefix(mime, "audio/") {
		result.Audio = []operation.GeneratedAudio{{Bytes: body, MediaType: mime}}
	} else {
		result.Images = []operation.GeneratedImage{{Bytes: body, MediaType: mime}}
	}
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	cost, err := d.Route.Purchase.Cost(domain.Usage{})
	require.NoError(t, err)
	receipt := domain.Receipt{Body: raw, UpstreamID: result.TaskID, PurchaseMicros: cost}
	require.NoError(t, store.SaveReceipt(t.Context(), c.ID, d.Attempt, receipt))
	_, err = store.Settle(t.Context(), c.ID)
	require.NoError(t, err)
	return d, receipt
}
func TestGatewayMediaDeliveryActualBytesReplayAndIndependentMoney(t *testing.T) {
	for _, policy := range []string{"provider_success_v1", ""} {
		t.Run("policy-"+policy, func(t *testing.T) {
			db, store, _, a := fixture(t)
			require.NoError(t, db.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=clock_timestamp() WHERE id=?`, a.ID.String()+"@example.invalid", a.ID).Error)
			m := mediaModel(t, store, "image.generate", policy)
			pid := uuid.New()
			require.NoError(t, db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'media delivery','16:9','realistic')`, pid, a.OrgID, a.ID, a.ScopeID).Error)
			registry := fixtureMediaStorage(t, db)
			r := &aigateway.Runtime{DB: db, Objects: registry}
			raw, _ := json.Marshal(domain.MediaInput{Model: m.Name, Capability: m.Capability, Mode: "text2image", Prompt: "actual fixture", Params: map[string]any{}, OutputCount: 1})
			c, err := application.Prepare(m, a.ID, a.ScopeID, pid, uuid.Nil, uuid.NewString(), "test", raw)
			require.NoError(t, err)
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return r.PinMediaOutput(t.Context(), tx, c) }))
			_, receipt := paidMediaReceipt(t, store, c, pngBytes(t), "image/png")
			objects := &durableBytes{t: t, db: db, routes: pgmedia.NewStorageRoutes(db), files: map[string][]byte{}, failOnce: true}
			r.MediaArchive = mediaapp.NewGeneratedOutputArchiver(objects.routes, objects, mediaflow.FFProber{}, mediaflow.FFRenderer{})
			require.ErrorIs(t, r.Deliver(t.Context(), c.ID), mediaapp.ErrUnavailable)
			var held, charged, released int64
			require.NoError(t, db.Raw("SELECT amount,charged,released FROM ai.hold WHERE request_id=?", c.ID).Row().Scan(&held, &charged, &released))
			require.EqualValues(t, 1, charged)
			require.NoError(t, r.Deliver(t.Context(), c.ID))
			require.NoError(t, r.Deliver(t.Context(), c.ID))
			require.Equal(t, 3, objects.writes)
			var output struct {
				AssetID       uuid.UUID
				State, SHA256 string
			}
			require.NoError(t, db.Raw("SELECT asset_id,state,sha256 FROM ai.output WHERE request_id=?", c.ID).Scan(&output).Error)
			wantState, wantStatus, wantModeration := "ready", "ready", "passed"
			if policy == "" {
				wantState, wantStatus, wantModeration = "pending", "processing", "pending"
			}
			require.Equal(t, wantState, output.State)
			var asset struct {
				SourceAIRequestID        uuid.UUID
				SourceOperationID        *uuid.UUID
				Status, ModerationStatus string
				ModerationDetail         []byte
				ObjectKey                string
			}
			require.NoError(t, db.Raw("SELECT * FROM media.media_asset WHERE id=?", output.AssetID).Scan(&asset).Error)
			require.Equal(t, c.ID, asset.SourceAIRequestID)
			require.Nil(t, asset.SourceOperationID)
			require.Equal(t, wantStatus, asset.Status)
			require.Equal(t, wantModeration, asset.ModerationStatus)
			require.Equal(t, pngBytes(t), objects.files[asset.ObjectKey])
			require.Equal(t, domain.Digest(objects.files[asset.ObjectKey]), output.SHA256)
			if policy != "" {
				require.Contains(t, string(asset.ModerationDetail), output.SHA256)
				require.NotContains(t, string(asset.ModerationDetail), "owner_review")
			}
			record, err := store.Read(t.Context(), a.ScopeID, c.ID)
			require.NoError(t, err)
			require.JSONEq(t, string(receipt.Body), string(record.Receipt.Body))
			require.EqualValues(t, 1, record.Charged)
			var count int64
			require.NoError(t, db.Raw("SELECT count(*) FROM operation.operation WHERE id=?", c.ID).Scan(&count).Error)
			require.Zero(t, count)
			require.NoError(t, db.Raw("SELECT count(*) FROM ai.money_event WHERE request_id=? AND kind='settle'", c.ID).Scan(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}

func TestGatewayDeliveryRejectsUnsafeReceiptURLWithoutNewSendOrWrites(t *testing.T) {
	db, store, _, a := fixture(t)
	m := mediaModel(t, store, "image.generate", "provider_success_v1")
	pid := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'unsafe URL','16:9','realistic')`, pid, a.OrgID, a.ID, a.ScopeID).Error)
	raw, _ := json.Marshal(domain.MediaInput{Model: m.Name, Capability: m.Capability, Mode: "text2image", Prompt: "unsafe result fixture", Params: map[string]any{}, OutputCount: 1})
	c, err := application.Prepare(m, a.ID, a.ScopeID, pid, uuid.Nil, uuid.NewString(), "test", raw)
	require.NoError(t, err)
	_, err = store.Quote(t.Context(), c)
	require.NoError(t, err)
	d, err := store.Admit(t.Context(), c)
	require.NoError(t, err)
	result := operation.GenerationResult{Outcome: operation.ProviderOutcomeCompleted, State: operation.ProviderTaskSucceeded, TaskID: "original", URLs: []string{"https://127.0.0.1/private-metadata"}}
	body, err := json.Marshal(result)
	require.NoError(t, err)
	cost, err := d.Route.Purchase.Cost(domain.Usage{})
	require.NoError(t, err)
	require.NoError(t, store.SaveReceipt(t.Context(), c.ID, d.Attempt, domain.Receipt{Body: body, PurchaseMicros: cost}))
	_, err = store.Settle(t.Context(), c.ID)
	require.NoError(t, err)
	var sends atomic.Int64
	runtime := &aigateway.Runtime{DB: db, Provider: providerFunc(func(context.Context, application.Dispatch, func(json.RawMessage) error) (domain.Receipt, error) {
		sends.Add(1)
		return domain.Receipt{}, domain.ErrInvalid
	})}
	require.ErrorIs(t, runtime.Deliver(t.Context(), c.ID), mediaflow.ErrUnsafeResultURL)
	require.Zero(t, sends.Load())
	var n int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.output WHERE request_id=?", c.ID).Scan(&n).Error)
	require.Zero(t, n)
	record, err := store.Read(t.Context(), a.ScopeID, c.ID)
	require.NoError(t, err)
	require.Equal(t, "settled", record.State)
	require.EqualValues(t, 1, record.Charged)
}

func TestGatewayMediaDeliveryRejectsChangedUnknownObjectAndPreservesDebt(t *testing.T) {
	db, store, _, a := fixture(t)
	require.NoError(t, db.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=clock_timestamp() WHERE id=?`, a.ID.String()+"@example.invalid", a.ID).Error)
	m := mediaModel(t, store, "image.generate", "provider_success_v1")
	pid := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'conflicting object','16:9','realistic')`, pid, a.OrgID, a.ID, a.ScopeID).Error)
	r := &aigateway.Runtime{DB: db, Objects: fixtureMediaStorage(t, db)}
	raw, _ := json.Marshal(domain.MediaInput{Model: m.Name, Capability: m.Capability, Mode: "text2image", Prompt: "conflict fixture", Params: map[string]any{}, OutputCount: 1})
	c, err := application.Prepare(m, a.ID, a.ScopeID, pid, uuid.Nil, uuid.NewString(), "test", raw)
	require.NoError(t, err)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return r.PinMediaOutput(t.Context(), tx, c) }))
	paidMediaReceipt(t, store, c, pngBytes(t), "image/png")
	objects := &durableBytes{t: t, db: db, routes: pgmedia.NewStorageRoutes(db), files: map[string][]byte{}, failOnce: true}
	r.MediaArchive = mediaapp.NewGeneratedOutputArchiver(objects.routes, objects, mediaflow.FFProber{}, mediaflow.FFRenderer{})
	require.ErrorIs(t, r.Deliver(t.Context(), c.ID), mediaapp.ErrUnavailable)
	for key := range objects.files {
		objects.files[key] = []byte("foreign unknown object")
	}
	require.ErrorIs(t, r.Deliver(t.Context(), c.ID), mediaapp.ErrObjectMismatch)
	require.Equal(t, 1, objects.writes)
	for _, data := range objects.files {
		require.Equal(t, []byte("foreign unknown object"), data)
	}
	var row struct{ State, Aspect string }
	require.NoError(t, db.Raw("SELECT state,manifest->>'aspect_ratio' AS aspect FROM ai.output WHERE request_id=?", c.ID).Scan(&row).Error)
	require.Equal(t, "preparing", row.State)
	require.Equal(t, "16:9", row.Aspect)
	var n int64
	require.NoError(t, db.Raw("SELECT count(*) FROM media.media_asset WHERE source_ai_request_id=?", c.ID).Scan(&n).Error)
	require.Zero(t, n)
	record, err := store.Read(t.Context(), a.ScopeID, c.ID)
	require.NoError(t, err)
	require.Equal(t, "settled", record.State)
	require.EqualValues(t, 1, record.Charged)
}

func (o *durableBytes) Get(_ context.Context, ref objectstorage.Ref) (io.ReadCloser, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	b, ok := o.files[ref.Key]
	if !ok {
		return nil, mediaapp.ErrUnavailable
	}
	return io.NopCloser(bytes.NewReader(bytes.Clone(b))), nil
}

type localResultDownloads struct {
	serverURL string
	client    *http.Client
}

func (d localResultDownloads) Download(ctx context.Context, _ string) (*mediaapp.Downloaded, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, d.serverURL+"/original.png", nil)
	if err != nil {
		return nil, err
	}
	response, err := d.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, mediaflow.ErrResultExpired
	}
	return mediaapp.ReadUpload(ctx, io.LimitReader(response.Body, 512<<20), "original.png")
}
func (d localResultDownloads) DownloadAuthenticated(ctx context.Context, raw, _ string) (*mediaapp.Downloaded, error) {
	return d.Download(ctx, raw)
}

func TestGatewayURLDeliveryRecoversOwnOriginalAfterSQLFailureAndUpstreamExpiry(t *testing.T) {
	for _, storageCase := range []string{"present_complete", "unknown_complete", "unknown_truncated"} {
		t.Run(storageCase, func(t *testing.T) {
			db, store, _, a := fixture(t)
			require.NoError(t, db.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=clock_timestamp() WHERE id=?`, a.ID.String()+"@example.invalid", a.ID).Error)
			m := mediaModel(t, store, "image.generate", "provider_success_v1")
			pid := uuid.New()
			require.NoError(t, db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'expired URL recovery','16:9','realistic')`, pid, a.OrgID, a.ID, a.ScopeID).Error)
			body := pngBytes(t)
			var expired atomic.Bool
			var requests, sends atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				if expired.Load() {
					w.WriteHeader(http.StatusGone)
					return
				}
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(body)
			}))
			defer server.Close()
			objects := &durableBytes{t: t, db: db, routes: pgmedia.NewStorageRoutes(db), files: map[string][]byte{}}
			registry := fixtureMediaStorage(t, db)
			runtime := &aigateway.Runtime{DB: db, Objects: registry, OutputReads: objects, ResultDownloads: localResultDownloads{serverURL: server.URL, client: server.Client()}, MediaArchive: mediaapp.NewGeneratedOutputArchiver(objects.routes, objects, mediaflow.FFProber{}, mediaflow.FFRenderer{}), Provider: providerFunc(func(context.Context, application.Dispatch, func(json.RawMessage) error) (domain.Receipt, error) {
				sends.Add(1)
				return domain.Receipt{}, domain.ErrInvalid
			})}
			raw, _ := json.Marshal(domain.MediaInput{Model: m.Name, Capability: m.Capability, Mode: "text2image", Prompt: "expired URL fixture", Params: map[string]any{}, OutputCount: 1})
			call, err := application.Prepare(m, a.ID, a.ScopeID, pid, uuid.Nil, uuid.NewString(), "test", raw)
			require.NoError(t, err)
			_, err = store.Quote(t.Context(), call)
			require.NoError(t, err)
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return runtime.PinMediaOutput(t.Context(), tx, call) }))
			dispatch, err := store.Admit(t.Context(), call)
			require.NoError(t, err)
			result := operation.GenerationResult{Outcome: operation.ProviderOutcomeCompleted, State: operation.ProviderTaskSucceeded, TaskID: "frozen-url-task", URLs: []string{"https://fixture.tos-cn-beijing.volces.com/signed-original.png?expires=short"}, Usage: json.RawMessage(`{"native_units":1}`)}
			receiptBody, err := json.Marshal(result)
			require.NoError(t, err)
			cost, err := dispatch.Route.Purchase.Cost(domain.Usage{})
			require.NoError(t, err)
			require.NoError(t, store.SaveReceipt(t.Context(), call.ID, dispatch.Attempt, domain.Receipt{Body: receiptBody, UpstreamID: result.TaskID, PurchaseMicros: cost}))
			_, err = store.Settle(t.Context(), call.ID)
			require.NoError(t, err)
			injected := errors.New("fixture final output SQL failed")
			var fail atomic.Bool
			fail.Store(true)
			const callback = "fixture:ai-output-commit-failure"
			require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register(callback, func(tx *gorm.DB) {
				if strings.HasPrefix(tx.Statement.SQL.String(), "UPDATE ai.output SET state=") && fail.CompareAndSwap(true, false) {
					_ = tx.AddError(injected)
				}
			}))
			t.Cleanup(func() { _ = db.Callback().Raw().Remove(callback) })
			require.ErrorIs(t, runtime.Deliver(t.Context(), call.ID), injected)
			require.EqualValues(t, 1, requests.Load())
			require.Equal(t, 3, objects.writes)
			var prior struct {
				AssetID                       uuid.UUID
				SHA256, ManifestSHA256, State string
			}
			require.NoError(t, db.Raw("SELECT asset_id,sha256,manifest_sha256,state FROM ai.output WHERE request_id=?", call.ID).Scan(&prior).Error)
			require.Equal(t, "preparing", prior.State)
			var n int64
			require.NoError(t, db.Raw("SELECT count(*) FROM media.media_asset WHERE source_ai_request_id=?", call.ID).Scan(&n).Error)
			require.Zero(t, n)
			if storageCase != "present_complete" {
				originalSuffix := "/" + prior.AssetID.String() + ".png"
				var key string
				require.NoError(t, db.Raw("SELECT object_key FROM media.object_location WHERE owner_intent_id=(SELECT id FROM media.storage_work_intent WHERE kind='operation' AND owner_id=? AND work_id=?) AND right(object_key,?)=?", pid, call.ID, len(originalSuffix), originalSuffix).Row().Scan(&key))
				require.NoError(t, db.Exec("UPDATE media.object_location SET state='unknown',byte_size=NULL,sha256=NULL,mime_type=NULL WHERE object_key=? AND owner_intent_id=(SELECT id FROM media.storage_work_intent WHERE kind='operation' AND owner_id=? AND work_id=?)", key, pid, call.ID).Error)
				if storageCase == "unknown_truncated" {
					objects.mu.Lock()
					objects.files[key] = bytes.Clone(body[:len(body)/2])
					objects.mu.Unlock()
				}
			}
			expired.Store(true)
			// A restarted consumer has only the persisted manifest and original physical
			// objects. Its upstream URL is gone; a second download must not be attempted.
			restarted := *runtime
			restarted.MediaArchive = mediaapp.NewGeneratedOutputArchiver(pgmedia.NewStorageRoutes(db), objects, mediaflow.FFProber{}, mediaflow.FFRenderer{})
			if storageCase == "unknown_truncated" {
				require.ErrorIs(t, restarted.Deliver(t.Context(), call.ID), mediaapp.ErrObjectMismatch)
				require.EqualValues(t, 1, requests.Load())
				require.Zero(t, sends.Load())
				require.Equal(t, 3, objects.writes)
				var state string
				require.NoError(t, db.Raw("SELECT state FROM ai.output WHERE request_id=?", call.ID).Row().Scan(&state))
				require.Equal(t, "preparing", state)
				require.NoError(t, db.Raw("SELECT count(*) FROM media.media_asset WHERE source_ai_request_id=?", call.ID).Scan(&n).Error)
				require.Zero(t, n)
				require.NoError(t, db.Raw("SELECT count(*) FROM media.object_location WHERE owner_intent_id=(SELECT id FROM media.storage_work_intent WHERE kind='operation' AND owner_id=? AND work_id=?) AND state='unknown'", pid, call.ID).Scan(&n).Error)
				require.EqualValues(t, 1, n)
				return
			}
			require.NoError(t, restarted.Deliver(t.Context(), call.ID))
			require.EqualValues(t, 1, requests.Load())
			require.Zero(t, sends.Load())
			require.Equal(t, 3, objects.writes)
			var after struct {
				AssetID                       uuid.UUID
				SHA256, ManifestSHA256, State string
			}
			require.NoError(t, db.Raw("SELECT asset_id,sha256,manifest_sha256,state FROM ai.output WHERE request_id=?", call.ID).Scan(&after).Error)
			require.Equal(t, prior.AssetID, after.AssetID)
			require.Equal(t, prior.SHA256, after.SHA256)
			require.Equal(t, prior.ManifestSHA256, after.ManifestSHA256)
			require.Equal(t, "ready", after.State)
			record, err := store.Read(t.Context(), a.ScopeID, call.ID)
			require.NoError(t, err)
			require.EqualValues(t, 1, record.Charged)
			require.JSONEq(t, string(receiptBody), string(record.Receipt.Body))
			require.NoError(t, db.Raw("SELECT count(*) FROM ai.money_event WHERE request_id=? AND kind='settle'", call.ID).Scan(&n).Error)
			require.EqualValues(t, 1, n)
			require.NoError(t, db.Raw("SELECT count(*) FROM ai.attempt WHERE request_id=?", call.ID).Scan(&n).Error)
			require.EqualValues(t, 1, n)
			require.NoError(t, db.Raw("SELECT count(*) FROM ai.hold WHERE request_id=?", call.ID).Scan(&n).Error)
			require.EqualValues(t, 1, n)
		})
	}
}
