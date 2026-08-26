package asset_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	assetgorm "github.com/StephenQiu30/lanverse/backend/internal/asset/adapter/gormdb"
	assetapp "github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstore"
)

func TestArtifactReadinessPersistsOneOwnerFactWithRealPostgreSQLAndMinIO(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	minioEndpoint := os.Getenv("LANVERSE_TEST_MINIO_ENDPOINT")
	minioAccessKey := os.Getenv("LANVERSE_TEST_MINIO_ACCESS_KEY")
	minioSecretKey := os.Getenv("LANVERSE_TEST_MINIO_SECRET_KEY")
	minioBucket := os.Getenv("LANVERSE_TEST_MINIO_BUCKET")
	if databaseURL == "" || minioEndpoint == "" || minioAccessKey == "" || minioSecretKey == "" || minioBucket == "" {
		t.Skip("set PostgreSQL and MinIO test variables to run the artifact readiness journey")
	}

	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}
	objects, err := objectstore.Open(objectstore.Config{
		Endpoint: minioEndpoint, PublicEndpoint: minioEndpoint,
		AccessKey: minioAccessKey, SecretKey: minioSecretKey,
		Bucket: minioBucket, Region: "us-east-1",
	})
	if err != nil {
		t.Fatalf("open MinIO client: %v", err)
	}
	if err = objects.EnsureBucket(ctx); err != nil {
		t.Fatalf("ensure MinIO test bucket: %v", err)
	}

	now := time.Date(2026, time.August, 26, 10, 0, 0, 0, time.UTC)
	workspaceID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	if err = database.Create(&model.UserAccount{ID: userID, EmailNormalized: "asset-owner-" + userID.String() + "@example.test", PasswordHash: "test", TokenVersion: 1, DisplayName: "Asset Owner", Status: "active", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed asset owner: %v", err)
	}
	if err = database.Create(&model.Workspace{ID: workspaceID, Name: "Asset Readiness", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed asset workspace: %v", err)
	}
	if err = database.Create(&model.Membership{ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, Role: "editor", Status: "active", JoinedAt: now}).Error; err != nil {
		t.Fatalf("seed asset membership: %v", err)
	}
	if err = database.Create(&model.Project{ID: projectID, WorkspaceID: workspaceID, Name: "Asset Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed asset project: %v", err)
	}
	actor := assetapp.Actor{UserID: userID.String(), TokenVersion: 1}
	store := assetgorm.New(database)
	serviceConfig := assetapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
		Bucket: minioBucket, StorageProfile: "private-primary", Region: "us-east-1", MaxImageBytes: 20 << 20,
	}
	service := assetapp.NewService(store, objects, serviceConfig)

	readyBytes := testPNG(t, 4, 3)
	readyHash := sha256Hex(readyBytes)
	providerJobID := uuid.NewString()
	readyKey := "staging/" + workspaceID.String() + "/" + providerJobID + "/frame-0001.png"
	putObject(t, ctx, objects, readyKey, "image/png", readyBytes)
	registerCommand := assetapp.RegisterStagedCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), SourceType: "generation_provider_job",
		SourceID: providerJobID, OutputKey: "frame-0001", ObjectKey: readyKey,
		MediaType: "image/png", SHA256: readyHash, SizeBytes: int64(len(readyBytes)), IdempotencyKey: "register-frame-0001",
	}

	const callers = 8
	results := make(chan assetapp.RegisterResult, callers)
	errorsFound := make(chan error, callers)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, registerErr := service.RegisterStaged(ctx, actor, registerCommand)
			if registerErr != nil {
				errorsFound <- registerErr
				return
			}
			results <- result
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsFound)
	for registerErr := range errorsFound {
		t.Fatalf("register staged artifact concurrently: %v", registerErr)
	}
	var artifactID, registerReceiptID string
	for result := range results {
		if artifactID == "" {
			artifactID, registerReceiptID = result.Artifact.ID, result.Receipt.ID
		}
		if result.Artifact.ID != artifactID || result.Receipt.ID != registerReceiptID ||
			result.Artifact.Status != assetdomain.ReadinessPendingValidation ||
			result.Location.Status != assetdomain.LocationStaging {
			t.Fatalf("concurrent registration drifted: %#v", result)
		}
	}
	if _, err = service.RequireReady(ctx, actor, artifactID); err == nil {
		t.Fatal("pending artifact passed RequireReady")
	}
	driftedRegistration := registerCommand
	driftedRegistration.SHA256 = "f" + driftedRegistration.SHA256[1:]
	if driftedRegistration.SHA256 == registerCommand.SHA256 {
		driftedRegistration.SHA256 = "e" + driftedRegistration.SHA256[1:]
	}
	if _, err = service.RegisterStaged(ctx, actor, driftedRegistration); err == nil {
		t.Fatal("registration idempotency key accepted drifted input")
	}

	validated, err := service.ValidateReady(ctx, actor, assetapp.ValidateReadyCommand{
		ArtifactID: artifactID, ExpectedRevision: 1, IdempotencyKey: "validate-frame-0001",
	})
	if err != nil {
		t.Fatalf("validate ready artifact: %v", err)
	}
	if validated.Artifact.Status != assetdomain.ReadinessReady || validated.Artifact.Revision != 2 ||
		validated.Artifact.Width != 4 || validated.Artifact.Height != 3 ||
		validated.Location.Status != assetdomain.LocationPrimary || validated.Receipt.ID == "" {
		t.Fatalf("ready artifact result = %#v", validated)
	}
	replayed, err := service.ValidateReady(ctx, actor, assetapp.ValidateReadyCommand{
		ArtifactID: artifactID, ExpectedRevision: 1, IdempotencyKey: "validate-frame-0001",
	})
	if err != nil || replayed.Receipt.ID != validated.Receipt.ID || replayed.Artifact.Revision != 2 {
		t.Fatalf("replay artifact validation: result=%#v err=%v", replayed, err)
	}
	ready, err := service.RequireReady(ctx, actor, artifactID)
	if err != nil || ready.ID != artifactID || ready.Status != assetdomain.ReadinessReady {
		t.Fatalf("require ready artifact: artifact=%#v err=%v", ready, err)
	}
	redeliveryCommand := registerCommand
	redeliveryCommand.IdempotencyKey = "register-frame-0001-redelivery"
	redelivered, err := service.RegisterStaged(ctx, actor, redeliveryCommand)
	if err != nil || redelivered.Artifact.ID != artifactID || redelivered.Artifact.Status != assetdomain.ReadinessReady {
		t.Fatalf("redeliver same source output: result=%#v err=%v", redelivered, err)
	}
	driftedSourceOutput := redeliveryCommand
	driftedSourceOutput.IdempotencyKey = "register-frame-0001-drifted-source"
	driftedSourceOutput.ObjectKey = "staging/" + workspaceID.String() + "/" + providerJobID + "/different.png"
	if _, err = service.RegisterStaged(ctx, actor, driftedSourceOutput); err == nil {
		t.Fatal("source output identity accepted a different object key")
	}

	quarantinedBytes := testPNG(t, 2, 2)
	quarantinedJobID := uuid.NewString()
	quarantinedKey := "staging/" + workspaceID.String() + "/" + quarantinedJobID + "/frame-bad.png"
	putObject(t, ctx, objects, quarantinedKey, "image/png", quarantinedBytes)
	badHash := sha256Hex(append([]byte(nil), quarantinedBytes...))
	badHash = "0" + badHash[1:]
	if badHash == sha256Hex(quarantinedBytes) {
		badHash = "1" + badHash[1:]
	}
	quarantined, err := service.RegisterStaged(ctx, actor, assetapp.RegisterStagedCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), SourceType: "generation_provider_job",
		SourceID: quarantinedJobID, OutputKey: "frame-bad", ObjectKey: quarantinedKey,
		MediaType: "image/png", SHA256: badHash, SizeBytes: int64(len(quarantinedBytes)), IdempotencyKey: "register-frame-bad",
	})
	if err != nil {
		t.Fatalf("register corrupt declaration: %v", err)
	}
	quarantineResult, err := service.ValidateReady(ctx, actor, assetapp.ValidateReadyCommand{
		ArtifactID: quarantined.Artifact.ID, ExpectedRevision: 1, IdempotencyKey: "validate-frame-bad",
	})
	if err != nil {
		t.Fatalf("persist corrupt artifact quarantine: %v", err)
	}
	if quarantineResult.Artifact.Status != assetdomain.ReadinessQuarantined ||
		quarantineResult.Location.Status != assetdomain.LocationStaging || quarantineResult.Artifact.FailureCode != "checksum_mismatch" {
		t.Fatalf("quarantined artifact result = %#v", quarantineResult)
	}
	if _, err = service.RequireReady(ctx, actor, quarantined.Artifact.ID); err == nil {
		t.Fatal("quarantined artifact passed RequireReady")
	}

	mismatchJobID := uuid.NewString()
	mismatchBytes := testJPEG(t, 2, 1)
	mismatchKey := "staging/" + workspaceID.String() + "/" + mismatchJobID + "/frame-mismatch.png"
	putObject(t, ctx, objects, mismatchKey, "image/jpeg", mismatchBytes)
	mismatch, err := service.RegisterStaged(ctx, actor, assetapp.RegisterStagedCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), SourceType: "generation_provider_job",
		SourceID: mismatchJobID, OutputKey: "frame-mismatch", ObjectKey: mismatchKey,
		MediaType: "image/png", SHA256: sha256Hex(mismatchBytes), SizeBytes: int64(len(mismatchBytes)), IdempotencyKey: "register-frame-mismatch",
	})
	if err != nil {
		t.Fatalf("register media type mismatch: %v", err)
	}
	mismatchResult, err := service.ValidateReady(ctx, actor, assetapp.ValidateReadyCommand{
		ArtifactID: mismatch.Artifact.ID, ExpectedRevision: 1, IdempotencyKey: "validate-frame-mismatch",
	})
	if err != nil || mismatchResult.Artifact.Status != assetdomain.ReadinessQuarantined || mismatchResult.Artifact.FailureCode != "media_type_mismatch" {
		t.Fatalf("quarantine media type mismatch: result=%#v err=%v", mismatchResult, err)
	}

	pendingJobID := uuid.NewString()
	pendingBytes := testPNG(t, 1, 1)
	pending, err := service.RegisterStaged(ctx, actor, assetapp.RegisterStagedCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), SourceType: "generation_provider_job",
		SourceID: pendingJobID, OutputKey: "frame-unavailable",
		ObjectKey: "staging/" + workspaceID.String() + "/" + pendingJobID + "/frame-unavailable.png",
		MediaType: "image/png", SHA256: sha256Hex(pendingBytes), SizeBytes: int64(len(pendingBytes)), IdempotencyKey: "register-frame-unavailable",
	})
	if err != nil {
		t.Fatalf("register artifact for unavailable storage: %v", err)
	}
	unavailableService := assetapp.NewService(store, unavailableReader{}, serviceConfig)
	if _, err = unavailableService.ValidateReady(ctx, actor, assetapp.ValidateReadyCommand{
		ArtifactID: pending.Artifact.ID, ExpectedRevision: 1, IdempotencyKey: "validate-frame-unavailable",
	}); err == nil {
		t.Fatal("unavailable object storage was reported as a completed validation")
	}
	var pendingRecord model.Artifact
	if err = database.First(&pendingRecord, "id = ?", pending.Artifact.ID).Error; err != nil {
		t.Fatalf("load pending artifact after dependency failure: %v", err)
	}
	if pendingRecord.Status != assetdomain.ReadinessPendingValidation || pendingRecord.Revision != 1 {
		t.Fatalf("dependency failure changed artifact: %#v", pendingRecord)
	}

	var artifactCount, locationCount, registerReceiptCount, validateReceiptCount int64
	if err = database.Model(&model.Artifact{}).Where("workspace_id = ?", workspaceID).Count(&artifactCount).Error; err != nil {
		t.Fatalf("count artifacts: %v", err)
	}
	if err = database.Model(&model.ArtifactLocation{}).Where("workspace_id = ?", workspaceID).Count(&locationCount).Error; err != nil {
		t.Fatalf("count artifact locations: %v", err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where("workspace_id = ? AND operation = ?", workspaceID, "asset.artifact.register_staged").Count(&registerReceiptCount).Error; err != nil {
		t.Fatalf("count register receipts: %v", err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where("workspace_id = ? AND operation = ?", workspaceID, "asset.artifact.validate_ready").Count(&validateReceiptCount).Error; err != nil {
		t.Fatalf("count validation receipts: %v", err)
	}
	if artifactCount != 4 || locationCount != 4 || registerReceiptCount != 5 || validateReceiptCount != 3 {
		t.Fatalf("owner fact counts = artifacts %d locations %d register receipts %d validation receipts %d", artifactCount, locationCount, registerReceiptCount, validateReceiptCount)
	}
}

type unavailableReader struct{}

func (unavailableReader) ReadVerified(context.Context, string, int64, string, int64) ([]byte, error) {
	return nil, io.ErrUnexpectedEOF
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			value.Set(x, y, color.RGBA{R: uint8(20 + x), G: uint8(40 + y), B: 80, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, value); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	return encoded.Bytes()
}

func testJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			value.Set(x, y, color.RGBA{R: uint8(80 + x), G: uint8(60 + y), B: 20, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, value, nil); err != nil {
		t.Fatalf("encode test JPEG: %v", err)
	}
	return encoded.Bytes()
}

func putObject(t *testing.T, ctx context.Context, objects *objectstore.Client, key, mediaType string, contents []byte) {
	t.Helper()
	putURL, err := objects.PresignPut(ctx, key, time.Minute)
	if err != nil {
		t.Fatalf("presign test object: %v", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL.String(), bytes.NewReader(contents))
	if err != nil {
		t.Fatalf("build test object request: %v", err)
	}
	request.Header.Set("Content-Type", mediaType)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("upload test object: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("upload test object status %d: %s", response.StatusCode, body)
	}
}

func sha256Hex(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}
