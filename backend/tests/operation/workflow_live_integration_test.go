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
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/tests/taskfixture"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/staged"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func TestOperationTaskResumesAfterWorkerRestartOnLocalServices(t *testing.T) {
	if os.Getenv("GLOWLOOM_TEST_OPERATION_STORE_DB_DSN") == "" || os.Getenv("GLOWLOOM_OBJECT_STORAGE_CONFIG") == "" {
		t.Skip("set isolated PostgreSQL and object-storage fixture configuration")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	database := operationStoreDB(t).WithContext(ctx)
	objects := businessStorageRegistry(t)
	require.NotNil(t, objects, "configured object-storage profile is required for this explicit fixture")
	require.NoError(t, objects.Ping(ctx))
	var picture bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	require.NoError(t, png.Encode(&picture, img))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(picture.Bytes())
	}))
	defer server.Close()
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	receipts := operationapp.NewRoutedProviderStageService(staging.NewRoutedFactory(objects, pgmedia.NewStorageRoutes(database)), operationStore(database))
	media, err := mediaflow.NewRoutedScopedActivitiesWithStaging(database, objects, mediaflow.DownloadPolicy{AllowedOrigins: []string{server.URL}, AllowTestLoopbackTLS: true, TLSRootCAs: pool}, staged.NewReader(receipts), app.NewMediaWorkerScopeAccess)
	require.NoError(t, err)
	for _, outcome := range []string{"accepted", "unknown"} {
		t.Run(outcome, func(t *testing.T) {
			id, project := seedLiveMockOperation(t, database, server.URL, outcome)
			businessPinOperation(t, database, operationapp.ProviderDispatchIdentity{OperationID: id, ProjectID: project})
			s := operationStore(database)
			f := app.NewOperationFinalizer(database, s, pgbilling.NewStore(database))
			h := map[string]taskruntime.Handler{}
			operationflow.RegisterFlow(h, s, f, nil)
			operationflow.RegisterProvider(h, s, nil)
			operationflow.RegisterMedia(h, s, media, nil)
			require.NoError(t, taskruntime.Enqueue(ctx, database, "operation.run", id.String(), time.Now()))
			stop := taskfixture.Start(t, database, h)
			waitForOperationStatus(ctx, t, database, id, "submitted")
			stop()
			taskfixture.Start(t, database, h)
			waitForOperationStatus(ctx, t, database, id, "completed")
			assertLiveOperationResult(ctx, t, database, objects, id, project)
			require.NoError(t, f.FinalizeOperation(ctx, operationflow.SettlementInput{OperationID: id.String(), From: domain.StatusIngesting, To: domain.StatusCompleted}))
			assertLiveOperationResult(ctx, t, database, objects, id, project)
		})
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
	objects *objectstorage.Registry, operationID, projectID uuid.UUID,
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
		ObjectKey, ProfileID, Bucket string
		LocationID                   uuid.UUID
		ModerationStatus             string
		MediaStatus                  string
	}
	if err := database.WithContext(ctx).Raw(`
		SELECT a.object_key, location.id AS location_id,location.profile_id,location.bucket, o.moderation_status, a.status AS media_status
		FROM operation.operation_output AS o
		JOIN media.media_asset AS a ON a.id = o.media_asset_id
 JOIN media.object_location AS location ON location.id=a.object_location_id AND location.object_key=a.object_key
		WHERE o.operation_id = ?::uuid AND o.project_id = ?::uuid
	`, operationID.String(), projectID.String()).Scan(&output).Error; err != nil ||
		output.ObjectKey == "" || output.ModerationStatus != "passed" || output.MediaStatus != "ready" {
		t.Fatalf("completed candidate = %+v: %v", output, err)
	}
	if _, err := objects.Stat(ctx, objectstorage.Ref{ProfileID: output.ProfileID, Bucket: output.Bucket, Key: output.ObjectKey}); err != nil {
		t.Fatalf("stat generated MinIO object: %v", err)
	}
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := objects.Remove(cleanCtx, objectstorage.Ref{ProfileID: output.ProfileID, Bucket: output.Bucket, Key: output.ObjectKey}); err != nil {
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
