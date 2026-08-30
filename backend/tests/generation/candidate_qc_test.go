package generation_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	assetgorm "github.com/StephenQiu30/lanverse/backend/internal/asset/adapter/gormdb"
	assetapp "github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	generationasset "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/asset"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstore"
)

func TestReadyArtifactCreatesOneCandidateAndDeterministicQC(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	minioEndpoint := os.Getenv("LANVERSE_TEST_MINIO_ENDPOINT")
	minioAccessKey := os.Getenv("LANVERSE_TEST_MINIO_ACCESS_KEY")
	minioSecretKey := os.Getenv("LANVERSE_TEST_MINIO_SECRET_KEY")
	minioBucket := os.Getenv("LANVERSE_TEST_MINIO_BUCKET")
	if databaseURL == "" || minioEndpoint == "" || minioAccessKey == "" || minioSecretKey == "" || minioBucket == "" {
		t.Skip("set PostgreSQL and MinIO test variables to run the generation candidate journey")
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

	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	workspaceID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	if err = database.Create(&model.UserAccount{ID: userID, EmailNormalized: "generation-owner-" + userID.String() + "@example.test", PasswordHash: "test", TokenVersion: 1, DisplayName: "Generation Owner", Status: "active", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed generation owner: %v", err)
	}
	if err = database.Create(&model.Workspace{ID: workspaceID, Name: "Generation Candidate", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed generation workspace: %v", err)
	}
	if err = database.Create(&model.Membership{ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, Role: "editor", Status: "active", JoinedAt: now}).Error; err != nil {
		t.Fatalf("seed generation membership: %v", err)
	}
	if err = database.Create(&model.Project{ID: projectID, WorkspaceID: workspaceID, Name: "Generation Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed generation project: %v", err)
	}

	assetService := assetapp.NewService(assetgorm.New(database), objects, assetapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
		Bucket: minioBucket, StorageProfile: "private-primary", Region: "us-east-1", MaxImageBytes: 20 << 20,
	})
	actor := generationapp.Actor{UserID: userID.String(), TokenVersion: 1}
	generationStore := generationgorm.New(database)
	service := generationapp.NewService(generationStore, generationasset.NewReadiness(assetService), generationapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
		ImageQC: generationapp.ImageQCPolicy{
			Version: "image-deterministic", AllowedMediaTypes: []string{"image/jpeg", "image/png"},
			MinWidth: 4, MinHeight: 3, MaxPixels: 100,
		},
	})

	ready := createArtifact(t, ctx, objects, assetService, assetapp.Actor(actor), workspaceID.String(), projectID.String(), 4, 3, "ready", true)
	command := generationapp.RegisterReadyCandidateCommand{ArtifactID: ready.ID, IdempotencyKey: "candidate-ready"}

	const callers = 8
	results := make(chan generationapp.RegisterCandidateResult, callers)
	errorsFound := make(chan error, callers)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, registerErr := service.RegisterReadyCandidate(ctx, actor, command)
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
		t.Fatalf("register ready candidate concurrently: %v", registerErr)
	}
	var candidateID, reportID, receiptID string
	for result := range results {
		if candidateID == "" {
			candidateID, reportID, receiptID = result.Candidate.ID, result.Report.ID, result.Receipt.ID
		}
		if result.Candidate.ID != candidateID || result.Report.ID != reportID || result.Receipt.ID != receiptID ||
			result.Candidate.Status != generationdomain.CandidateQCPassed || result.Report.Status != generationdomain.QCPassed ||
			result.Candidate.ArtifactID != ready.ID || result.Candidate.ArtifactRevision != ready.Revision ||
			result.Report.Policy.Version != "image-deterministic" || result.Report.PolicyHash == "" || len(result.Report.FailureCodes) != 0 {
			t.Fatalf("concurrent candidate registration drifted: %#v", result)
		}
	}
	passed, err := service.RequireQCPassed(ctx, actor, candidateID)
	if err != nil || passed.Candidate.ID != candidateID || passed.Report.ID != reportID {
		t.Fatalf("require QC-passed candidate: result=%#v err=%v", passed, err)
	}
	replayed, err := service.RegisterReadyCandidate(ctx, actor, command)
	if err != nil || replayed.Receipt.ID != receiptID || replayed.Candidate.ID != candidateID {
		t.Fatalf("replay candidate command: result=%#v err=%v", replayed, err)
	}
	redelivered, err := service.RegisterReadyCandidate(ctx, actor, generationapp.RegisterReadyCandidateCommand{
		ArtifactID: ready.ID, IdempotencyKey: "candidate-ready-redelivery",
	})
	if err != nil || redelivered.Candidate.ID != candidateID || redelivered.Report.ID != reportID || redelivered.Receipt.ID == receiptID {
		t.Fatalf("redeliver ready artifact with a new command key: result=%#v err=%v", redelivered, err)
	}

	tooSmall := createArtifact(t, ctx, objects, assetService, assetapp.Actor(actor), workspaceID.String(), projectID.String(), 2, 2, "small", true)
	failed, err := service.RegisterReadyCandidate(ctx, actor, generationapp.RegisterReadyCandidateCommand{
		ArtifactID: tooSmall.ID, IdempotencyKey: "candidate-small",
	})
	if err != nil {
		t.Fatalf("register deterministic QC failure: %v", err)
	}
	if failed.Candidate.Status != generationdomain.CandidateQCFailed || failed.Report.Status != generationdomain.QCFailed ||
		len(failed.Report.FailureCodes) != 2 || failed.Report.FailureCodes[0] != "width_below_minimum" ||
		failed.Report.FailureCodes[1] != "height_below_minimum" {
		t.Fatalf("deterministic QC failure = %#v", failed)
	}
	if _, err = service.RequireQCPassed(ctx, actor, failed.Candidate.ID); err == nil {
		t.Fatal("QC-failed candidate passed RequireQCPassed")
	}
	tooLarge := createArtifact(t, ctx, objects, assetService, assetapp.Actor(actor), workspaceID.String(), projectID.String(), 11, 10, "large", true)
	largeFailed, err := service.RegisterReadyCandidate(ctx, actor, generationapp.RegisterReadyCandidateCommand{
		ArtifactID: tooLarge.ID, IdempotencyKey: "candidate-large",
	})
	if err != nil || len(largeFailed.Report.FailureCodes) != 1 || largeFailed.Report.FailureCodes[0] != "pixel_count_exceeded" {
		t.Fatalf("maximum pixel QC failure: result=%#v err=%v", largeFailed, err)
	}
	pngNotAllowed := createArtifact(t, ctx, objects, assetService, assetapp.Actor(actor), workspaceID.String(), projectID.String(), 4, 3, "png-not-allowed", true)
	jpegOnlyService := generationapp.NewService(generationStore, generationasset.NewReadiness(assetService), generationapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
		ImageQC: generationapp.ImageQCPolicy{Version: "jpeg-only", AllowedMediaTypes: []string{"image/jpeg"}, MinWidth: 4, MinHeight: 3, MaxPixels: 100},
	})
	mediaFailed, err := jpegOnlyService.RegisterReadyCandidate(ctx, actor, generationapp.RegisterReadyCandidateCommand{
		ArtifactID: pngNotAllowed.ID, IdempotencyKey: "candidate-png-not-allowed",
	})
	if err != nil || len(mediaFailed.Report.FailureCodes) != 1 || mediaFailed.Report.FailureCodes[0] != "media_type_not_allowed" {
		t.Fatalf("allowed media type QC failure: result=%#v err=%v", mediaFailed, err)
	}

	if err = database.Model(&model.UserAccount{}).Where("id = ?", userID).Update("token_version", 2).Error; err != nil {
		t.Fatalf("revoke generation actor token: %v", err)
	}
	if _, err = service.RequireQCPassed(ctx, actor, candidateID); err == nil {
		t.Fatal("revoked actor consumed a QC-passed candidate")
	}
	if err = database.Model(&model.UserAccount{}).Where("id = ?", userID).Update("token_version", 1).Error; err != nil {
		t.Fatalf("restore generation actor token for remaining assertions: %v", err)
	}

	pending := createArtifact(t, ctx, objects, assetService, assetapp.Actor(actor), workspaceID.String(), projectID.String(), 4, 3, "pending", false)
	if _, err = service.RegisterReadyCandidate(ctx, actor, generationapp.RegisterReadyCandidateCommand{
		ArtifactID: pending.ID, IdempotencyKey: "candidate-pending",
	}); err == nil {
		t.Fatal("pending artifact created a generation candidate")
	}

	driftArtifact := createArtifact(t, ctx, objects, assetService, assetapp.Actor(actor), workspaceID.String(), projectID.String(), 4, 3, "drift", true)
	if _, err = service.RegisterReadyCandidate(ctx, actor, generationapp.RegisterReadyCandidateCommand{
		ArtifactID: driftArtifact.ID, IdempotencyKey: command.IdempotencyKey,
	}); err == nil {
		t.Fatal("candidate idempotency key accepted a different artifact")
	}
	driftedPolicyService := generationapp.NewService(generationStore, generationasset.NewReadiness(assetService), generationapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
		ImageQC: generationapp.ImageQCPolicy{Version: "image-deterministic-strict-width", AllowedMediaTypes: []string{"image/jpeg", "image/png"}, MinWidth: 5, MinHeight: 3, MaxPixels: 100},
	})
	if _, err = driftedPolicyService.RegisterReadyCandidate(ctx, actor, generationapp.RegisterReadyCandidateCommand{
		ArtifactID: ready.ID, IdempotencyKey: "candidate-ready-policy-drift",
	}); err == nil {
		t.Fatal("existing candidate accepted a different QC policy binding")
	}

	nonProviderArtifactID := uuid.New()
	nonProviderLocationID := uuid.New()
	nonProviderHash := sha256Hex(testPNG(t, 4, 3))
	if err = database.Create(&model.Artifact{
		ID: nonProviderArtifactID, WorkspaceID: workspaceID, ProjectID: projectID, SourceType: "upload", SourceID: uuid.New(), OutputKey: "uploaded-reference",
		MediaType: "image/png", SHA256: nonProviderHash, SizeBytes: 1, Status: assetdomain.ReadinessReady, Width: integerPointer(4), Height: integerPointer(3), Revision: 2, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed non-provider ready artifact: %v", err)
	}
	if err = database.Create(&model.ArtifactLocation{
		ID: nonProviderLocationID, WorkspaceID: workspaceID, ArtifactID: nonProviderArtifactID, LocationNo: 1, StorageProfile: "private-primary", Bucket: minioBucket,
		ObjectKey: "uploads/" + workspaceID.String() + "/reference.png", Region: "us-east-1", Checksum: nonProviderHash, Status: assetdomain.LocationPrimary, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed non-provider artifact location: %v", err)
	}
	if _, err = service.RegisterReadyCandidate(ctx, actor, generationapp.RegisterReadyCandidateCommand{
		ArtifactID: nonProviderArtifactID.String(), IdempotencyKey: "candidate-upload",
	}); err == nil {
		t.Fatal("non-provider artifact created a generation candidate")
	}

	var candidateCount, reportCount, receiptCount int64
	if err = database.Model(&model.GenerationCandidate{}).Where("workspace_id = ?", workspaceID).Count(&candidateCount).Error; err != nil {
		t.Fatalf("count generation candidates: %v", err)
	}
	if err = database.Model(&model.GenerationQCReport{}).Where("workspace_id = ?", workspaceID).Count(&reportCount).Error; err != nil {
		t.Fatalf("count generation QC reports: %v", err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where("workspace_id = ? AND operation = ?", workspaceID, "generation.candidate.register_ready").Count(&receiptCount).Error; err != nil {
		t.Fatalf("count generation candidate receipts: %v", err)
	}
	if candidateCount != 4 || reportCount != 4 || receiptCount != 5 {
		t.Fatalf("generation owner fact counts = candidates %d reports %d receipts %d", candidateCount, reportCount, receiptCount)
	}
	if err = database.Model(&model.GenerationQCReport{}).Where("id = ?", reportID).Update("report_hash", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatalf("inject generation QC report drift: %v", err)
	}
	if _, err = service.RequireQCPassed(ctx, actor, candidateID); err == nil {
		t.Fatal("drifted deterministic QC report passed RequireQCPassed")
	}
}

func createArtifact(t *testing.T, ctx context.Context, objects *objectstore.Client, service *assetapp.Service, actor assetapp.Actor, workspaceID, projectID string, width, height int, key string, validate bool) assetdomain.Artifact {
	t.Helper()
	contents := testPNG(t, width, height)
	providerJobID := uuid.NewString()
	objectKey := "staging/" + workspaceID + "/" + providerJobID + "/" + key + ".png"
	putObject(t, ctx, objects, objectKey, contents)
	registered, err := service.RegisterStaged(ctx, actor, assetapp.RegisterStagedCommand{
		WorkspaceID: workspaceID, ProjectID: projectID, SourceType: "generation_provider_job", SourceID: providerJobID,
		OutputKey: key, ObjectKey: objectKey, MediaType: "image/png", SHA256: sha256Hex(contents), SizeBytes: int64(len(contents)),
		IdempotencyKey: "register-" + key,
	})
	if err != nil {
		t.Fatalf("register %s artifact: %v", key, err)
	}
	if !validate {
		return registered.Artifact
	}
	validated, err := service.ValidateReady(ctx, actor, assetapp.ValidateReadyCommand{
		ArtifactID: registered.Artifact.ID, ExpectedRevision: registered.Artifact.Revision, IdempotencyKey: "validate-" + key,
	})
	if err != nil {
		t.Fatalf("validate %s artifact: %v", key, err)
	}
	return validated.Artifact
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

func putObject(t *testing.T, ctx context.Context, objects *objectstore.Client, key string, contents []byte) {
	t.Helper()
	putURL, err := objects.PresignPut(ctx, key, time.Minute)
	if err != nil {
		t.Fatalf("presign test object: %v", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL.String(), bytes.NewReader(contents))
	if err != nil {
		t.Fatalf("build test object request: %v", err)
	}
	request.Header.Set("Content-Type", "image/png")
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

func integerPointer(value int) *int { return &value }
