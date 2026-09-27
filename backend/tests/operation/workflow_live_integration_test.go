package operation_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func TestOperationWorkflowResumesAfterFlowWorkerRestartOnLocalServices(t *testing.T) {
	dsn := os.Getenv("LV_TEST_OPERATION_STORE_DB_DSN")
	addr := os.Getenv("LV_TEST_TEMPORAL_ADDR")
	namespace := os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if dsn == "" || addr == "" || namespace == "" || os.Getenv("LV_TEST_AGENT_WORKER") != "1" ||
		os.Getenv("LV_TEST_OBJECT_STORAGE") != "1" || os.Getenv("LV_ENV_FILE") == "" {
		t.Skip("set isolated PostgreSQL, local Temporal, root .env, MinIO, and a running native agent worker")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open isolated PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	database := conn.DB
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load native MinIO configuration: %v", err)
	}
	objects, err := objectstorage.Open(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket,
		cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageRegion)
	if err != nil {
		t.Fatalf("open MinIO client: %v", err)
	}
	if err := objects.Ping(ctx); err != nil {
		t.Fatalf("ping MinIO: %v", err)
	}
	var picture bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&picture, img); err != nil {
		t.Fatal(err)
	}
	resultServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(picture.Bytes())
	}))
	t.Cleanup(resultServer.Close)
	pool := x509.NewCertPool()
	pool.AddCert(resultServer.Certificate())
	operationID, projectID := seedLiveMockOperation(t, database, resultServer.URL, "accepted")

	temporalClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatalf("connect local Temporal: %v", err)
	}
	t.Cleanup(temporalClient.Close)
	mediaActivities, err := mediaflow.NewActivities(database, objects, mediaflow.DownloadPolicy{
		AllowedOrigins: []string{resultServer.URL}, AllowTestLoopbackTLS: true,
		TLSRootCAs: pool,
	})
	if err != nil {
		t.Fatalf("construct media activities: %v", err)
	}
	mediaWorker := worker.New(temporalClient, "media", worker.Options{MaxConcurrentActivityExecutionSize: 4})
	mediaflow.Register(mediaWorker, mediaActivities)
	if err := mediaWorker.Start(); err != nil {
		t.Fatalf("start media worker: %v", err)
	}
	t.Cleanup(mediaWorker.Stop)
	newFlowWorker := func() worker.Worker {
		flowWorker := worker.New(temporalClient, "flow", worker.Options{})
		store := pgoperation.NewStore(database)
		finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
		operationflow.Register(flowWorker, operationflow.NewActivities(store, finalizer))
		return flowWorker
	}
	flowWorker := newFlowWorker()
	if err := flowWorker.Start(); err != nil {
		t.Fatalf("start flow worker: %v", err)
	}
	flowRunning := true
	t.Cleanup(func() {
		if flowRunning {
			flowWorker.Stop()
		}
	})
	run, err := temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID: "operation/" + operationID.String(), TaskQueue: "flow",
	}, operationflow.OperationWorkflow, operationflow.OperationInput{OperationID: operationID.String()})
	if err != nil {
		t.Fatalf("start mock operation workflow: %v", err)
	}
	waitForOperationStatus(ctx, t, database, operationID, "submitted")
	flowWorker.Stop()
	flowRunning = false
	flowWorker = newFlowWorker()
	if err := flowWorker.Start(); err != nil {
		t.Fatalf("restart flow worker: %v", err)
	}
	flowRunning = true
	if err := run.Get(ctx, nil); err != nil {
		t.Fatalf("resumed mock operation workflow: %v", err)
	}
	assertLiveOperationResult(ctx, t, database, objects, operationID, projectID)
	store := pgoperation.NewStore(database)
	finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	if err := finalizer.FinalizeOperation(ctx, operationflow.SettlementInput{
		OperationID: operationID.String(), From: domain.StatusIngesting,
		To: domain.StatusCompleted,
	}); err != nil {
		t.Fatalf("replay terminal settlement: %v", err)
	}
	assertLiveOperationResult(ctx, t, database, objects, operationID, projectID)
	unknownID, unknownProjectID := seedLiveMockOperation(t, database, resultServer.URL, "unknown")
	unknownRun, err := temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID: "operation/" + unknownID.String(), TaskQueue: "flow",
	}, operationflow.OperationWorkflow, operationflow.OperationInput{OperationID: unknownID.String()})
	if err != nil {
		t.Fatalf("start uncertain mock operation workflow: %v", err)
	}
	if err := unknownRun.Get(ctx, nil); err != nil {
		t.Fatalf("reconcile uncertain mock operation: %v", err)
	}
	assertLiveOperationResult(ctx, t, database, objects, unknownID, unknownProjectID)
	var uncertainStates []string
	if err := database.WithContext(ctx).Raw(`
		SELECT to_status FROM operation.operation_event
		WHERE operation_id = ?::uuid ORDER BY create_time, id
	`, unknownID.String()).Scan(&uncertainStates).Error; err != nil {
		t.Fatalf("read uncertain operation timeline: %v", err)
	}
	if !containsWorkflowStates(uncertainStates, "unknown", "reconciling", "submitted") {
		t.Fatalf("uncertain operation did not reconcile: %v", uncertainStates)
	}
}

func seedLiveMockOperation(t *testing.T, database *gorm.DB, resultOrigin, outcome string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	actor, projectID := operationStoreProject(t, database)
	_, operationID, _, reservationID := operationStoreRows(t, database, actor, projectID)
	var frozen struct {
		ModelProfileVersionID uuid.UUID
		PriceRuleVersionID    uuid.UUID
	}
	if err := database.Raw(`
		SELECT model_profile_version_id, price_rule_version_id
		FROM operation.operation WHERE id = ?::uuid
	`, operationID.String()).Scan(&frozen).Error; err != nil {
		t.Fatalf("read mock frozen model IDs: %v", err)
	}
	providerID, profileID := uuid.New(), uuid.New()
	if err := database.Exec(`
		INSERT INTO catalog.provider (id, key, name, adapter_key, region)
		VALUES (?::uuid, ?, 'Mock', 'mock', 'domestic')
	`, providerID.String(), "mock-"+providerID.String()).Error; err != nil {
		t.Fatalf("seed mock provider: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.capability (id, key, output_type, modes, input_roles)
		VALUES (?::uuid, 'image.generate', 'image', ARRAY['text_to_image'], ARRAY['prompt'])
		ON CONFLICT (key) DO NOTHING
	`, uuid.NewString()).Error; err != nil {
		t.Fatalf("seed image capability: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.model_profile
		  (id, model_key, provider_id, capability, display_name, status)
		VALUES (?::uuid, ?, ?::uuid, 'image.generate', 'Mock Image', 'active')
	`, profileID.String(), "mock-"+profileID.String(), providerID.String()).Error; err != nil {
		t.Fatalf("seed mock model: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.model_profile_version
		  (id, model_profile_id, version_no, provider_model_id, modes, limits, param_schema,
		   supports_query, supports_cancel, supports_callback, expected_max_ms, moderation, queue)
		VALUES (?::uuid, ?::uuid, 1, 'mock-image-v1', ARRAY['text_to_image'], '{}'::jsonb,
		        '[]'::jsonb, true, false, false, 60000, 'platform', 'agent.mock')
	`, frozen.ModelProfileVersionID.String(), profileID.String()).Error; err != nil {
		t.Fatalf("seed mock model version: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO catalog.price_rule_version
		  (id, model_profile_id, version_no, unit, rule, effective_from)
		VALUES (?::uuid, ?::uuid, 1, 'per_image', '{}'::jsonb, now() - interval '1 minute')
	`, frozen.PriceRuleVersionID.String(), profileID.String()).Error; err != nil {
		t.Fatalf("seed mock price version: %v", err)
	}
	params := `{"mock_outcome":"` + outcome + `","mock_delay_ms":12000,"mock_result_urls":["` + resultOrigin + `/result.png"],"mock_moderation_status":"passed"}`
	if err := database.Exec(`
		UPDATE operation.operation SET batch_id = NULL, params = ?::jsonb WHERE id = ?::uuid
	`, params, operationID.String()).Error; err != nil {
		t.Fatalf("seed mock operation parameters: %v", err)
	}
	if err := database.Exec(`
		UPDATE billing.budget SET reserved_micros = 10 WHERE project_id = ?::uuid
	`, projectID.String()).Error; err != nil {
		t.Fatalf("reserve mock budget: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO billing.ledger_entry (id, project_id, entry_type, amount_micros, operation_id)
		VALUES (?::uuid, ?::uuid, 'reserve', 10, ?::uuid)
	`, uuid.NewString(), projectID.String(), operationID.String()).Error; err != nil {
		t.Fatalf("seed reserve ledger: %v", err)
	}
	_ = reservationID
	return operationID, projectID
}

func waitForOperationStatus(ctx context.Context, t *testing.T, database *gorm.DB, id uuid.UUID, want string) {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var status string
		if err := database.WithContext(ctx).Raw(`SELECT status FROM operation.operation WHERE id = ?::uuid`, id.String()).Scan(&status).Error; err != nil {
			t.Fatalf("read operation status: %v", err)
		}
		if status == want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for operation %s: last status %q: %v", want, status, ctx.Err())
		case <-ticker.C:
		}
	}
}

func assertLiveOperationResult(ctx context.Context, t *testing.T, database *gorm.DB,
	objects *objectstorage.Client, operationID, projectID uuid.UUID,
) {
	t.Helper()
	var operation struct {
		Status        string
		SettledMicros *int64
	}
	if err := database.WithContext(ctx).Raw(`
		SELECT status, settled_micros FROM operation.operation WHERE id = ?::uuid
	`, operationID.String()).Scan(&operation).Error; err != nil || operation.Status != "completed" ||
		operation.SettledMicros == nil || *operation.SettledMicros != 0 {
		t.Fatalf("completed operation = %+v: %v", operation, err)
	}
	var output struct {
		ObjectKey        string
		ModerationStatus string
		MediaStatus      string
	}
	if err := database.WithContext(ctx).Raw(`
		SELECT a.object_key, o.moderation_status, a.status AS media_status
		FROM operation.operation_output AS o
		JOIN media.media_asset AS a ON a.id = o.media_asset_id
		WHERE o.operation_id = ?::uuid AND o.project_id = ?::uuid
	`, operationID.String(), projectID.String()).Scan(&output).Error; err != nil ||
		output.ObjectKey == "" || output.ModerationStatus != "passed" || output.MediaStatus != "ready" {
		t.Fatalf("completed candidate = %+v: %v", output, err)
	}
	if _, err := objects.Stat(ctx, output.ObjectKey); err != nil {
		t.Fatalf("stat generated MinIO object: %v", err)
	}
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := objects.Remove(cleanCtx, output.ObjectKey); err != nil {
			t.Errorf("remove exact generated fixture object: %v", err)
		}
	})
	var count int64
	for _, table := range []string{"billing.ledger_entry", "operation.operation_output", "operation.operation_event"} {
		if err := database.WithContext(ctx).Raw(`SELECT count(*) FROM `+table+` WHERE operation_id = ?::uuid`, operationID.String()).Scan(&count).Error; err != nil || count == 0 {
			t.Fatalf("%s rows = %d: %v", table, count, err)
		}
	}
	var settled, releases int64
	if err := database.WithContext(ctx).Raw(`SELECT count(*) FROM billing.ledger_entry WHERE operation_id = ?::uuid AND entry_type = 'settle'`, operationID.String()).Scan(&settled).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.WithContext(ctx).Raw(`SELECT count(*) FROM billing.ledger_entry WHERE operation_id = ?::uuid AND entry_type = 'release'`, operationID.String()).Scan(&releases).Error; err != nil {
		t.Fatal(err)
	}
	if settled != 1 || releases != 1 {
		t.Fatalf("settlement replay duplicated ledger: settle=%d release=%d", settled, releases)
	}
}

func containsWorkflowStates(states []string, required ...string) bool {
	seen := make(map[string]bool, len(states))
	for _, status := range states {
		seen[status] = true
	}
	for _, status := range required {
		if !seen[status] {
			return false
		}
	}
	return true
}
