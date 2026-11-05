package agentbusiness_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	scriptobjects "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/objects"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	sourcepostgres "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/postgres"
	script "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	scriptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

type sourceBytes struct {
	data        map[uuid.UUID][]byte
	puts, reads int
	beforePut   func(context.Context)
}

func (s *sourceBytes) GetObject(_ context.Context, f scriptdomain.ObjectFact) (io.ReadCloser, error) {
	s.reads++
	b, ok := s.data[f.LocationID]
	if !ok {
		return nil, script.ErrObjectMissing
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (s *sourceBytes) PutObjectIfAbsent(ctx context.Context, f scriptdomain.ObjectFact, r io.Reader) error {
	if s.beforePut != nil {
		s.beforePut(ctx)
	}
	if _, ok := s.data[f.LocationID]; ok {
		return script.ErrObjectExists
	}
	b, err := io.ReadAll(r)
	if err == nil {
		s.data[f.LocationID] = b
		s.puts++
	}
	return err
}
func (s *sourceBytes) RemoveObject(_ context.Context, f scriptdomain.ObjectFact) error {
	delete(s.data, f.LocationID)
	return nil
}
func sourceRoute(t *testing.T, db *gorm.DB) {
	t.Helper()
	profile := "agent-script-" + uuid.NewString()[:8]
	if err := db.Exec(`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity)VALUES(?,'minio','http://127.0.0.1','http://127.0.0.1','us-east-1','agent-script','path',?)`, profile, domain.ContentHash(profile)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO media.storage_route(singleton,revision,profile_id)VALUES(1,?,?) ON CONFLICT(singleton)DO UPDATE SET revision=EXCLUDED.revision,profile_id=EXCLUDED.profile_id`, "1", profile).Error; err != nil {
		t.Fatal(err)
	}
}
func sourcePublishInput() map[string]any {
	return map[string]any{"action": "create", "expected_revision": 0, "rights_confirmed": true, "sources": []any{map[string]any{"source_kind": "chapter", "title": "第一章", "status": "draft", "document": map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "雨夜，主角走入街道。"}}}}}}}}
}
func TestSourcePublicationActualPGOriginalIntentBytesReplayAndReceiptRecovery(t *testing.T) {
	db, b, _, _, g := realCanvasBoundary(t)
	sourceRoute(t, db)
	objects := &sourceBytes{data: map[uuid.UUID][]byte{}}
	objects.beforePut = func(ctx context.Context) {
		if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return tx.Exec(`SELECT root_run_id FROM agent_business.run_authorization WHERE root_run_id=? FOR UPDATE NOWAIT`, g.RootRunID).Error
		}); err != nil {
			t.Fatal("source I/O retained DB locks", err)
		}
	}
	b.WithSourceObjects(objects)
	router := gin.New()
	b.BusinessHandler(businessToken).Register(router)
	status, first, message := businessToolRequest(t, router, g, "script_source_publish", "publish", sourcePublishInput())
	if status != 200 || first.Status != "completed" {
		t.Fatal(status, message)
	}
	var receipt script.SourceReceipt
	if err := json.Unmarshal(first.Result, &receipt); err != nil || receipt.VersionID == uuid.Nil || objects.puts != 4 {
		t.Fatal("formal source receipt", string(first.Result), objects.puts, err)
	}
	var counts struct{ Sources, Versions, Bindings, Accepted int64 }
	if err := db.Raw(`SELECT (SELECT count(*) FROM script.script_source WHERE project_id=?) sources,(SELECT count(*) FROM script.script_version WHERE project_id=?) versions,(SELECT count(*) FROM agent_business.source_authorization WHERE receipt_id=?) bindings,(SELECT count(*) FROM script.object_intent i JOIN media.object_location l ON l.id=i.object_location_id WHERE i.project_id=? AND l.state='accepted') accepted`, g.Scope.ProjectID, g.Scope.ProjectID, first.ReceiptID, g.Scope.ProjectID).Scan(&counts).Error; err != nil || counts.Sources != 1 || counts.Versions != 1 || counts.Bindings != 1 || counts.Accepted != 4 {
		t.Fatal(counts, err)
	}
	beforeReads := objects.reads
	status, replay, message := businessToolRequest(t, router, g, "script_source_publish", "publish", sourcePublishInput())
	if status != 200 || replay.ReceiptID != first.ReceiptID || objects.puts != 4 || objects.reads != beforeReads {
		t.Fatal("source replay rebuilt original", message)
	}
	// Simulate a lost final business receipt after the owning source committed.
	if err := db.Exec(`UPDATE agent_business.call_receipt SET status='unknown',result='{}' WHERE receipt_id=?`, first.ReceiptID).Error; err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(os.Getenv("GLOWLOOM_TEST_AGENT_CANVAS_DB_DSN"))
	query := parsed.Query()
	query.Set("options", "-c role=glowloom_app")
	parsed.RawQuery = query.Encode()
	runtime, openErr := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{})
	if openErr != nil {
		t.Fatal(openErr)
	}
	connection, openErr := runtime.DB()
	if openErr != nil {
		t.Fatal(openErr)
	}
	t.Cleanup(func() { _ = connection.Close() })
	reopened := agentwire.NewBoundary(runtime, nil).WithSourceObjects(objects)
	if _, err := reopened.Capabilities.ToolRecover(t.Context(), first.ReceiptID); err != nil {
		t.Fatal("durable original receipt recovery", err)
	}
	original, err := b.Store.ReadCall(t.Context(), g.RootRunID, g.SubmissionID, first.Call)
	if err != nil || original.Status != "completed" || domain.Hash(original.Result) != domain.Hash(first.Result) || objects.puts != 4 || objects.reads != beforeReads {
		t.Fatal("recovery changed original result or wrote", original, err)
	}
}
func TestSourcePublicationStopDuringObjectIOCannotPublishOrReplaceUnknownIntent(t *testing.T) {
	db, b, _, actor, g := realCanvasBoundary(t)
	sourceRoute(t, db)
	objects := &sourceBytes{data: map[uuid.UUID][]byte{}}
	objects.beforePut = func(ctx context.Context) {
		objects.beforePut = nil
		if _, err := b.Store.Decide(ctx, actor, g.Scope.ProjectID, g.RootRunID, domain.DecisionInput{RequestID: uuid.New(), Action: "stop", ExpectedAuthorizationRevision: 1}); err != nil {
			t.Fatal(err)
		}
	}
	b.WithSourceObjects(objects)
	router := gin.New()
	b.BusinessHandler(businessToken).Register(router)
	status, first, message := businessToolRequest(t, router, g, "script_source_publish", "stopped-source", sourcePublishInput())
	if status != 200 || first.Status != "unknown" {
		t.Fatal("stopped source claimed publication", status, message)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM script.script_source WHERE project_id=?`, g.Scope.ProjectID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("source published afterStop", count, err)
	}
	before := objects.puts
	status, replay, message := businessToolRequest(t, router, g, "script_source_publish", "stopped-source", sourcePublishInput())
	if status != 200 || replay.ReceiptID != first.ReceiptID || replay.Status != "unknown" || objects.puts != before {
		t.Fatal("unknown source reexecuted", message)
	}
	// Even a newly composed formal worker store must consult the persisted grant;
	// it cannot resume the original source plan after Stop through another caller.
	var row struct{ Plan []byte }
	if err := db.Raw(`SELECT plan FROM script.command WHERE actor_id=? AND request_id=?`, actor.ID, first.ReceiptID).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	var plan script.WritePlan
	if err := json.Unmarshal(row.Plan, &plan); err != nil {
		t.Fatal(err)
	}
	store := sourcepostgres.NewSourceStore(db, func(tx *gorm.DB) script.ProjectAccess { return scopewire.ProvideWorkerProjectContentAccess(tx) }).WithPublicationAuthorization(scriptwire.ProvideSourcePublicationAuthorization)
	if _, err := store.FinishWrite(t.Context(), g.Scope.Principal(), plan); !errors.Is(err, domain.ErrTerminal) {
		t.Fatal("generic source owner bypassed Stop", err)
	}
}

type lostSourcePut struct {
	*sourceBytes
	lost bool
}

func (s *lostSourcePut) PutObjectIfAbsent(ctx context.Context, f scriptdomain.ObjectFact, r io.Reader) error {
	if err := s.sourceBytes.PutObjectIfAbsent(ctx, f, r); err != nil {
		return err
	}
	if !s.lost {
		s.lost = true
		return errors.New("fixture lost source put response")
	}
	return nil
}
func TestCancelledOriginalSourceProofClosesUnknownCallWithoutReexecuting(t *testing.T) {
	db, b, _, _, g := realCanvasBoundary(t)
	sourceRoute(t, db)
	objects := &lostSourcePut{sourceBytes: &sourceBytes{data: map[uuid.UUID][]byte{}}}
	b.WithSourceObjects(objects)
	router := gin.New()
	b.BusinessHandler(businessToken).Register(router)
	status, first, message := businessToolRequest(t, router, g, "script_source_publish", "cancel-original", sourcePublishInput())
	if status != 200 || first.Status != "unknown" {
		t.Fatal(status, message)
	}
	store := sourceStoreForFixture(restrictedSourceRuntime(t))
	owner := script.NewSourceService(store, objects, time.Now)
	recovery := script.NewSourceRecovery(store, owner, time.Now)
	page, err := recovery.List(t.Context(), g.Scope.Principal(), g.Scope.ProjectID, 0, 10)
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	command := script.SourceControlCommand{ProjectID: g.Scope.ProjectID, IntentID: page.Items[0].ID, Key: uuid.New(), RequestID: uuid.New(), Action: "cancel", ExpectedRevision: page.Items[0].Revision}
	if _, err := recovery.Control(t.Context(), g.Scope.Principal(), command); err != nil {
		t.Fatal(err)
	}
	current, err := recovery.Get(t.Context(), g.Scope.Principal(), g.Scope.ProjectID, command.IntentID)
	if err != nil || current.Status != "pending" || !current.NeedsReconciliation || len(objects.data) != 1 {
		t.Fatal("bounded unknown write was deleted early", current, len(objects.data), err)
	}
	var work struct {
		ID     uuid.UUID
		Closed bool
	}
	if err := db.Raw(`SELECT w.id,w.closed_at IS NOT NULL closed FROM script.command c JOIN media.storage_work_intent w ON w.id=(c.plan->>'storage_intent_id')::uuid WHERE c.actor_id=? AND c.request_id=?`, g.Scope.ActorID, first.ReceiptID).Scan(&work).Error; err != nil || !work.Closed {
		t.Fatal("cancel clock not durable", work, err)
	}
	// Explicit isolated-fixture clock advance; no owner state, deletion permit,
	// source result or business receipt is fabricated by the fixture.
	if err := db.Transaction(func(tx *gorm.DB) error {
		// The owner's immutability trigger also protects production timestamps.
		// This administrator-only test transaction changes only the clock fixture.
		if err := tx.Exec(`SET LOCAL session_replication_role='replica'`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE media.storage_work_intent SET closed_at=clock_timestamp()-interval '11 minutes' WHERE id=?`, work.ID).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE media.object_location SET write_until=clock_timestamp()-interval '1 second' WHERE owner_intent_id=? AND write_until IS NOT NULL`, work.ID).Error
	}); err != nil {
		t.Fatal(err)
	}
	command.Action = "reconcile"
	command.Key = uuid.New()
	command.RequestID = uuid.New()
	command.ExpectedRevision = current.Revision
	if _, err := recovery.Control(t.Context(), g.Scope.Principal(), command); err != nil {
		t.Fatal(err)
	}
	current, err = recovery.Get(t.Context(), g.Scope.Principal(), g.Scope.ProjectID, command.IntentID)
	if err != nil || current.Status != "cancelled" || len(objects.data) != 0 {
		t.Fatal("formal cleanup not complete", current, len(objects.data), err)
	}
	writes, reads := objects.puts, objects.reads
	out, err := b.Capabilities.ToolRecover(t.Context(), first.ReceiptID)
	if err != nil || out.Status != "failed" || !bytes.Contains(out.Result, []byte(`"source_cancelled"`)) || writes != objects.puts || reads != objects.reads {
		t.Fatal("original cancellation proof did not settle original unknown", out, err)
	}
	status, next, message := businessToolRequest(t, router, g, "script_source_publish", "after-original-cancel", sourcePublishInput())
	if status != 200 || next.Status != "completed" {
		t.Fatal("resolved cancellation kept root blocked", status, message)
	}
}
func sourceStoreForFixture(db *gorm.DB) *sourcepostgres.SourceStore {
	return sourcepostgres.NewSourceStore(db, func(tx *gorm.DB) script.ProjectAccess { return scopewire.ProvideWorkerProjectContentAccess(tx) }).WithPublicationAuthorization(scriptwire.ProvideSourcePublicationAuthorization).WithObjectLocations(scriptwire.ProvideScriptObjectLocations)
}

func TestSourcePublicationActualMinioBytesAndSavedHistory(t *testing.T) {
	file := os.Getenv("GLOWLOOM_TEST_RECEIPT_STORAGE_CONFIG")
	if file == "" {
		t.Skip("explicit isolated local object fixture required")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal("read explicit object fixture")
	}
	var settings struct {
		Endpoint, Bucket, Region string
		AccessKey                string `json:"access_key"`
		SecretKey                string `json:"secret_key"`
	}
	if json.Unmarshal(raw, &settings) != nil {
		t.Fatal("invalid object fixture")
	}
	endpoint, err := url.Parse(settings.Endpoint)
	if err != nil || endpoint.Hostname() != "127.0.0.1" || settings.AccessKey == "" || settings.SecretKey == "" {
		t.Fatal("isolated loopback object fixture required")
	}
	db, b, _, _, g := realCanvasBoundary(t)
	profileID := "agent-source-" + domain.ContentHash(settings.Endpoint + "/" + settings.Bucket)[:12]
	input := config.ObjectStorageConfig{SchemaVersion: 1, Revision: "1", DefaultWriteProfile: profileID,
		Upload:   config.StorageUploadPolicy{Mode: config.StorageUploadRelay, SignedURLTTL: time.Minute, SessionTTL: time.Hour, StagingRetention: 24 * time.Hour, MultipartThresholdBytes: 8 << 20, PartSizeBytes: 5 << 20, PartConcurrency: 1},
		Access:   config.StorageAccessPolicy{PreviewTTL: time.Minute, MaxSignedURLTTL: 15 * time.Minute},
		Limits:   config.StorageLimits{MaxImageBytes: 20 << 20, MaxVideoBytes: 500 << 20, MaxAudioBytes: 100 << 20, MaxModelBytes: 64 << 20, MaxDocumentBytes: 20 << 20},
		Profiles: map[string]config.StorageProfile{profileID: {Provider: config.StorageProviderMinIO, Mode: config.StorageProfileReadWrite, Endpoint: settings.Endpoint, SigningEndpoint: settings.Endpoint, Bucket: settings.Bucket, Region: settings.Region, AddressingStyle: config.StorageAddressingPath, Credentials: config.StorageCredentialsRef{Source: config.StorageCredentialsEnvironment, AccessKeyEnv: "AGENT_SOURCE_TEST_ACCESS", SecretKeyEnv: "AGENT_SOURCE_TEST_SECRET"}}}}
	encoded, _ := json.Marshal(input)
	cfg, err := config.LoadObjectStorageEnvironment(string(encoded), "local", func(key string) string {
		if key == "AGENT_SOURCE_TEST_ACCESS" {
			return settings.AccessKey
		}
		if key == "AGENT_SOURCE_TEST_SECRET" {
			return settings.SecretKey
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	if err := pgmedia.NewStorageRoutes(db).RegisterProfiles(t.Context(), []media.StorageProfile{{ID: profileID, Provider: "minio", Endpoint: settings.Endpoint, SigningEndpoint: settings.Endpoint, Region: settings.Region, Bucket: settings.Bucket, AddressingStyle: "path"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO media.storage_route(singleton,revision,profile_id)VALUES(1,?,?) ON CONFLICT(singleton)DO UPDATE SET revision=EXCLUDED.revision,profile_id=EXCLUDED.profile_id`, "1", profileID).Error; err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(os.Getenv("GLOWLOOM_TEST_AGENT_CANVAS_DB_DSN"))
	query := parsed.Query()
	query.Set("options", "-c role=glowloom_app")
	parsed.RawQuery = query.Encode()
	runtime, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := runtime.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	objects := scriptobjects.NewRoutedStorage(registry, pgmedia.NewStorageRoutes(runtime))
	b.WithSourceObjects(objects)
	router := gin.New()
	b.BusinessHandler(businessToken).Register(router)
	status, first, message := businessToolRequest(t, router, g, "script_source_publish", "minio-source", sourcePublishInput())
	if status != 200 || first.Status != "completed" {
		t.Fatal("official object publication", status, message)
	}
	base, err := sourceStoreForFixture(runtime).LoadBase(t.Context(), g.Scope.Principal(), g.Scope.ProjectID, nil)
	if err != nil || len(base.Sources) != 1 || base.Sources[0].Rich.LocationID == uuid.Nil {
		t.Fatal("saved history missing", base, err)
	}
	body, err := objects.GetObject(t.Context(), base.Sources[0].Rich)
	if err != nil {
		t.Fatal("actual saved rich bytes unavailable", err)
	}
	data, readErr := io.ReadAll(body)
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || !bytes.Contains(data, []byte("雨夜，主角走入街道。")) || scriptdomain.ContentSHA(data) != base.Sources[0].Rich.SHA256 {
		t.Fatal("actual Minio bytes mismatch", readErr, closeErr)
	}
	status, replay, message := businessToolRequest(t, router, g, "script_source_publish", "minio-source", sourcePublishInput())
	if status != 200 || replay.ReceiptID != first.ReceiptID {
		t.Fatal("original official source replay", message)
	}
}

func restrictedSourceRuntime(t *testing.T) *gorm.DB {
	t.Helper()
	parsed, _ := url.Parse(os.Getenv("GLOWLOOM_TEST_AGENT_CANVAS_DB_DSN"))
	query := parsed.Query()
	query.Set("options", "-c role=glowloom_app")
	parsed.RawQuery = query.Encode()
	runtime, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := runtime.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	var role string
	if err := runtime.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "glowloom_app" {
		t.Fatal("actual restricted source owner required", role, err)
	}
	return runtime
}
