package generation_test

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	assetgorm "github.com/StephenQiu30/lanverse/backend/internal/asset/adapter/gormdb"
	assetapp "github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	costgorm "github.com/StephenQiu30/lanverse/backend/internal/cost/adapter/gormdb"
	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	generationasset "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/asset"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstore"
	quotagorm "github.com/StephenQiu30/lanverse/backend/internal/quota/adapter/gormdb"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
)

var (
	_ generationapp.MaterializableProviderResultOwner = (*generationapp.ProviderService)(nil)
	_ generationapp.ProviderOutputAssetOwner          = (*generationasset.ProviderOutputReadiness)(nil)
	_ generationapp.ReadyCandidateOwner               = (*generationapp.Service)(nil)
)

type stagedOutputPlan struct {
	outputKey                     string
	objectKeyOverride             string
	contents                      []byte
	upload                        bool
	declaredWidth, declaredHeight int
	failureCode                   string
}

type loadProviderOutputArtifact func(string) (model.Artifact, error)

type stagingProviderGateway struct {
	t       *testing.T
	objects *objectstore.Client
	cleanup *minio.Client
	bucket  string

	mu           sync.Mutex
	plan         []stagedOutputPlan
	uploadedKeys map[string]struct{}
}

func (gateway *stagingProviderGateway) setPlan(plan ...stagedOutputPlan) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	gateway.plan = append([]stagedOutputPlan(nil), plan...)
}

func (gateway *stagingProviderGateway) Preflight(
	context.Context,
	generationapp.ProviderSubmission,
) error {
	return nil
}

func (gateway *stagingProviderGateway) Submit(
	ctx context.Context,
	submission generationapp.ProviderSubmission,
) (generationapp.ProviderOutcome, error) {
	gateway.mu.Lock()
	plan := append([]stagedOutputPlan(nil), gateway.plan...)
	gateway.mu.Unlock()
	if submission.WorkspaceID == "" || submission.ProjectID == "" || submission.ProviderJobID == "" ||
		submission.ProviderCallID == "" || submission.RequestID == "" || submission.IntentID == "" ||
		submission.CandidateIndex < 1 || submission.RequestedOutputCount != 1 {
		return generationapp.ProviderOutcome{}, errors.New("Provider submission did not freeze owner identities")
	}
	if submission.CandidateIndex > len(plan) {
		return generationapp.ProviderOutcome{}, errors.New("Provider submission Candidate has no staged output plan")
	}
	item := plan[submission.CandidateIndex-1]
	if item.failureCode != "" {
		return generationapp.ProviderOutcome{
			Status: generationapp.ProviderOutcomeFailed, FailureCode: item.failureCode,
			ProviderEventID: "staging-failure-" + submission.ProviderCallID,
		}, nil
	}
	objectKey := "staging/" + submission.WorkspaceID + "/" + submission.ProviderJobID + "/" +
		submission.ProviderCallID + "/" + item.outputKey + ".png"
	if item.objectKeyOverride != "" {
		objectKey = item.objectKeyOverride
	}
	if item.upload {
		putObject(gateway.t, ctx, gateway.objects, objectKey, item.contents)
		gateway.mu.Lock()
		gateway.uploadedKeys[objectKey] = struct{}{}
		gateway.mu.Unlock()
	}
	output := generationapp.ProviderOutput{
		OutputKey: item.outputKey, StagingObjectKey: objectKey, SHA256: sha256Hex(item.contents),
		Bytes: int64(len(item.contents)), MediaType: "image/png",
		Width: item.declaredWidth, Height: item.declaredHeight,
	}
	return generationapp.ProviderOutcome{
		Status:          generationapp.ProviderOutcomeSucceeded,
		ProviderEventID: "staging-event-" + submission.ProviderCallID,
		Output:          &output, ProviderUsageObservation: generationdomain.ProviderUsageObservation{ImageCount: 1},
	}, nil
}

func (gateway *stagingProviderGateway) cleanupUploads(ctx context.Context) {
	gateway.mu.Lock()
	keys := make([]string, 0, len(gateway.uploadedKeys))
	for key := range gateway.uploadedKeys {
		keys = append(keys, key)
	}
	gateway.mu.Unlock()
	for _, key := range keys {
		if err := gateway.cleanup.RemoveObject(ctx, gateway.bucket, key, minio.RemoveObjectOptions{}); err != nil {
			gateway.t.Errorf("remove test-owned staged Provider object: %v", err)
		}
	}
}

func (gateway *stagingProviderGateway) Query(
	context.Context,
	generationapp.ProviderSubmission,
) (generationapp.ProviderOutcome, error) {
	return generationapp.ProviderOutcome{Status: generationapp.ProviderOutcomeUnknown}, nil
}

type failOnceCandidateOwner struct {
	delegate *generationapp.Service
	mu       sync.Mutex
	failed   bool
}

func (owner *failOnceCandidateOwner) RegisterReadyCandidate(
	ctx context.Context,
	actor generationapp.Actor,
	command generationapp.RegisterReadyCandidateCommand,
) (generationapp.RegisterCandidateResult, error) {
	owner.mu.Lock()
	if !owner.failed {
		owner.failed = true
		owner.mu.Unlock()
		return generationapp.RegisterCandidateResult{}, errors.New("injected Candidate Owner outage")
	}
	owner.mu.Unlock()
	return owner.delegate.RegisterReadyCandidate(ctx, actor, command)
}

func (owner *failOnceCandidateOwner) RequireEvaluatedProviderOutput(
	ctx context.Context,
	actor generationapp.Actor,
	providerJobID, providerCallID, providerReceiptID, outputKey string,
) (generationdomain.CandidateWithReport, error) {
	return owner.delegate.RequireEvaluatedProviderOutput(
		ctx,
		actor,
		providerJobID,
		providerCallID,
		providerReceiptID,
		outputKey,
	)
}

func TestSucceededProviderOutputsMaterializeThroughAssetReadinessAndCandidateQC(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	minioEndpoint := os.Getenv("LANVERSE_TEST_MINIO_ENDPOINT")
	minioAccessKey := os.Getenv("LANVERSE_TEST_MINIO_ACCESS_KEY")
	minioSecretKey := os.Getenv("LANVERSE_TEST_MINIO_SECRET_KEY")
	minioBucket := os.Getenv("LANVERSE_TEST_MINIO_BUCKET")
	if databaseURL == "" || minioEndpoint == "" || minioAccessKey == "" || minioSecretKey == "" || minioBucket == "" {
		t.Skip("set PostgreSQL and MinIO test variables to run the Provider output materialization journey")
	}

	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open Provider output materialization database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize Provider output materialization GORM catalog: %v", err)
	}
	objects, err := objectstore.Open(objectstore.Config{
		Endpoint: minioEndpoint, PublicEndpoint: minioEndpoint,
		AccessKey: minioAccessKey, SecretKey: minioSecretKey,
		Bucket: minioBucket, Region: "us-east-1",
	})
	if err != nil {
		t.Fatalf("open Provider output materialization MinIO client: %v", err)
	}
	if err = objects.EnsureBucket(ctx); err != nil {
		t.Fatalf("ensure Provider output materialization MinIO bucket: %v", err)
	}
	cleanup, err := minio.New(minioEndpoint, &minio.Options{
		Creds: credentials.NewStaticV4(minioAccessKey, minioSecretKey, ""), Region: "us-east-1",
	})
	if err != nil {
		t.Fatalf("open Provider output materialization cleanup client: %v", err)
	}

	now := time.Date(2026, time.August, 26, 23, 0, 0, 0, time.UTC)
	create := func(value any) error { return database.Create(value).Error }
	countRecords := func(value any, query string, arguments ...any) (int64, error) {
		var count int64
		err := database.Model(value).Where(query, arguments...).Count(&count).Error
		return count, err
	}
	loadArtifact := func(providerReceiptID string) (model.Artifact, error) {
		var artifact model.Artifact
		err := database.Where("source_type = ? AND source_id = ?", "generation_provider_receipt", providerReceiptID).
			First(&artifact).Error
		return artifact, err
	}
	fixture := seedPreparationFixture(t, database, create, generationgorm.NewTargetStore(database), now, "provider-output")
	fixture.provider = seedControlledProjectProviderBinding(
		t, create, fixture, "staging-image", "image-quality", 1,
	)
	t.Cleanup(func() {
		deletions := []struct {
			name string
			err  error
		}{
			{"Generation QC reports", database.Where("workspace_id = ?", fixture.workspaceID).Delete(&model.GenerationQCReport{}).Error},
			{"Generation candidates", database.Where("workspace_id = ?", fixture.workspaceID).Delete(&model.GenerationCandidate{}).Error},
			{"Artifact locations", database.Where("workspace_id = ?", fixture.workspaceID).Delete(&model.ArtifactLocation{}).Error},
			{"Artifacts", database.Where("workspace_id = ?", fixture.workspaceID).Delete(&model.Artifact{}).Error},
		}
		for _, deletion := range deletions {
			if deletion.err != nil {
				t.Errorf("clean test-owned %s: %v", deletion.name, deletion.err)
			}
		}
		cleanupProviderFixture(t, func(value any, query string, arguments ...any) error {
			return generationtestgorm.DeleteWithoutHooks(database, value, query, arguments...)
		}, fixture)
	})

	costConfig := costapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}
	quotaConfig := quotaapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}
	costs := costapp.NewService(costgorm.New(database), costConfig)
	quotas := quotaapp.NewService(quotagorm.New(database), quotaConfig)
	configurePreparationLimits(t, ctx, costs, quotas, fixture, "1000.000000", "10.000000", 100)
	preparations := generationapp.NewPreparationService(
		generationgorm.NewPreparationStore(database, costConfig, quotaConfig),
		generationapp.PreparationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString, ClaimTTL: preparationClaimTTL},
	)
	gateway := &stagingProviderGateway{
		t: t, objects: objects, cleanup: cleanup, bucket: minioBucket,
		uploadedKeys: make(map[string]struct{}),
	}
	t.Cleanup(func() { gateway.cleanupUploads(context.Background()) })
	providers := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig), gateway,
		generationapp.ProviderConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	assetService := assetapp.NewService(assetgorm.New(database), objects, assetapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
		Bucket: minioBucket, StorageProfile: "private-primary", Region: "us-east-1", MaxImageBytes: 20 << 20,
	})
	candidateService := generationapp.NewService(
		generationgorm.New(database), generationasset.NewReadiness(assetService), generationapp.Config{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			ImageQC: generationapp.ImageQCPolicy{
				Version: "provider-output-qc", AllowedMediaTypes: []string{"image/png"},
				MinWidth: 128, MinHeight: 128, MaxPixels: 2_000_000,
			},
		},
	)
	materializer := generationapp.NewOutputMaterializationService(
		providers, generationasset.NewProviderOutputReadiness(assetService), candidateService,
	)
	invalidStagingClaim := prepareAndClaimProviderIntent(
		t, ctx, preparations, fixture, "materialize-invalid-staging", strings.Repeat("d", 64),
	)
	gateway.setPlan(stagedOutputPlan{
		outputKey: "image-1", objectKeyOverride: "staging/another-workspace/another-job/image-1.png",
		contents: testPNG(t, 1536, 1024), upload: false, declaredWidth: 1536, declaredHeight: 1024,
	})
	invalidStaging, err := providers.SubmitImageRequest(ctx, invalidStagingClaim.Authorization, generationapp.SubmitImageRequestCommand{
		IntentID: invalidStagingClaim.Intent.ID, IdempotencyKey: "provider-output-submit-invalid-staging",
	})
	if generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Provider output outside frozen Call Staging prefix was accepted: result=%#v err=%v", invalidStaging, err)
	}
	var invalidJob model.GenerationProviderJob
	if err = database.Where("intent_id = ?", invalidStagingClaim.Intent.ID).First(&invalidJob).Error; err != nil {
		t.Fatalf("load invalid-Staging Provider Job: %v", err)
	}
	invalidStaging = reconcileProvider(
		t, ctx, providers, invalidJob.ID.String(), "provider-output-recover-invalid-staging",
	)
	if invalidStaging.Job.Status != generationdomain.ProviderJobOutcomeUnknown ||
		invalidStaging.Intent.Status != generationdomain.IntentOutcomeUnknown || len(invalidStaging.Receipts) != 0 {
		t.Fatalf("invalid-Staging dispatch was not fenced as OUTCOME_UNKNOWN: %#v", invalidStaging)
	}
	if _, err = materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: invalidStaging.Job.ID,
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("unknown invalid-Staging Provider result was materialized: %T %v", err, err)
	}

	firstBytes, secondBytes := testPNG(t, 1536, 1024), testPNG(t, 1536, 1024)
	success := submitStagedProviderJob(t, ctx, preparations, providers, gateway, fixture, "materialize-main",
		stagedOutputPlan{outputKey: "output-1", contents: firstBytes, upload: true, declaredWidth: 1536, declaredHeight: 1024},
		stagedOutputPlan{outputKey: "output-1", contents: secondBytes, upload: true, declaredWidth: 1536, declaredHeight: 1024},
	)
	const callers = 8
	results := make(chan generationapp.OutputMaterializationResult, callers)
	errorsFound := make(chan error, callers)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, materializeErr := materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
				ProviderJobID: success.Job.ID,
			})
			if materializeErr != nil {
				errorsFound <- materializeErr
				return
			}
			results <- result
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsFound)
	for materializeErr := range errorsFound {
		t.Fatalf("materialize Provider outputs concurrently: %T %v", materializeErr, materializeErr)
	}
	var canonical generationapp.OutputMaterializationResult
	for result := range results {
		if len(result.Outputs) != 2 || len(result.ProviderReceiptSetHash) != 64 {
			t.Fatalf("unexpected Provider materialization result: %#v", result)
		}
		if canonical.ProviderReceiptSetHash == "" {
			canonical = result
			continue
		}
		if result.ProviderReceiptSetHash != canonical.ProviderReceiptSetHash {
			t.Fatalf("concurrent materialization receipt set drifted: first=%#v next=%#v", canonical, result)
		}
		for index := range result.Outputs {
			if result.Outputs[index].Artifact.ID != canonical.Outputs[index].Artifact.ID ||
				result.Outputs[index].Candidate.ID != canonical.Outputs[index].Candidate.ID ||
				result.Outputs[index].Report.ID != canonical.Outputs[index].Report.ID {
				t.Fatalf("concurrent materialization did not converge: first=%#v next=%#v", canonical, result)
			}
		}
	}
	if canonical.Outputs[0].Output.OutputKey != "output-1" || canonical.Outputs[1].Output.OutputKey != "output-1" ||
		canonical.Outputs[0].ProviderCallID == canonical.Outputs[1].ProviderCallID ||
		canonical.Outputs[0].ProviderReceiptID == canonical.Outputs[1].ProviderReceiptID ||
		canonical.Outputs[0].Artifact.ID == canonical.Outputs[1].Artifact.ID ||
		canonical.Outputs[0].Candidate.ID == canonical.Outputs[1].Candidate.ID {
		t.Fatalf("independent Provider Calls with the same remote OutputKey did not retain distinct identities: %#v", canonical.Outputs)
	}
	for _, output := range canonical.Outputs {
		if output.Artifact.SourceType != "generation_provider_receipt" ||
			output.Artifact.SourceID != output.ProviderReceiptID ||
			output.Candidate.ProviderJobID != success.Job.ID ||
			output.Candidate.ProviderCallID != output.ProviderCallID ||
			output.Candidate.ProviderReceiptID != output.ProviderReceiptID ||
			output.Candidate.OutputKey != "output-1" {
			t.Fatalf("materialized Provider output lost Job/Call/Receipt/output identity: %#v", output)
		}
	}
	assertMaterializedFactCounts(t, countRecords, fixture.workspaceID.String(), 2, 2)
	if canonical.CandidateSet.ID != success.Job.ID ||
		canonical.CandidateSet.ProviderReceiptSetHash != canonical.ProviderReceiptSetHash ||
		canonical.CandidateSet.WorkspaceID != fixture.workspaceID.String() ||
		canonical.CandidateSet.ProjectID != fixture.projectID.String() || canonical.CandidateSet.Revision != 1 ||
		len(canonical.CandidateSet.Candidates) != 2 || len(canonical.CandidateSet.ContentHash) != 64 {
		t.Fatalf("materialized Provider outputs did not expose a canonical CandidateSet: %#v", canonical.CandidateSet)
	}
	requiredSet, err := materializer.RequireCandidateSet(ctx, fixture.editor, success.Job.ID)
	if err != nil || requiredSet.ContentHash != canonical.CandidateSet.ContentHash ||
		!slices.Equal(requiredSet.Candidates, canonical.CandidateSet.Candidates) {
		t.Fatalf("rebuild materialized Provider CandidateSet: set=%#v err=%v", requiredSet, err)
	}
	if err = database.Model(&model.GenerationCandidate{}).Where("id = ?", canonical.Outputs[0].Candidate.ID).
		Update("artifact_sha256", strings.Repeat("9", 64)).Error; err != nil {
		t.Fatalf("inject CandidateSet Candidate drift: %v", err)
	}
	if _, err = materializer.RequireCandidateSet(ctx, fixture.editor, success.Job.ID); err == nil {
		t.Fatal("CandidateSet rebuild accepted a Candidate/QC/Artifact drift")
	}
	if err = database.Model(&model.GenerationCandidate{}).Where("id = ?", canonical.Outputs[0].Candidate.ID).
		Update("artifact_sha256", canonical.Outputs[0].Candidate.ArtifactSHA256).Error; err != nil {
		t.Fatalf("restore CandidateSet Candidate after drift test: %v", err)
	}
	for _, drift := range []struct {
		column, value, restore string
	}{
		{column: "provider_job_id", value: uuid.NewString(), restore: canonical.Outputs[0].Candidate.ProviderJobID},
		{column: "provider_call_id", value: uuid.NewString(), restore: canonical.Outputs[0].ProviderCallID},
		{column: "provider_receipt_id", value: canonical.Outputs[1].ProviderReceiptID, restore: canonical.Outputs[0].ProviderReceiptID},
	} {
		if err = database.Model(&model.GenerationCandidate{}).Where("id = ?", canonical.Outputs[0].Candidate.ID).
			Update(drift.column, drift.value).Error; err != nil {
			t.Fatalf("inject CandidateSet %s drift: %v", drift.column, err)
		}
		if _, err = materializer.RequireCandidateSet(ctx, fixture.editor, success.Job.ID); err == nil {
			t.Fatalf("CandidateSet rebuild accepted %s drift", drift.column)
		}
		if err = database.Model(&model.GenerationCandidate{}).Where("id = ?", canonical.Outputs[0].Candidate.ID).
			Update(drift.column, drift.restore).Error; err != nil {
			t.Fatalf("restore CandidateSet %s after drift test: %v", drift.column, err)
		}
	}

	replayed, err := materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: success.Job.ID,
	})
	if err != nil || replayed.Outputs[0].Candidate.ID != canonical.Outputs[0].Candidate.ID ||
		replayed.Outputs[1].Candidate.ID != canonical.Outputs[1].Candidate.ID {
		t.Fatalf("replay Provider output materialization: result=%#v err=%v", replayed, err)
	}

	partial := submitStagedProviderJob(t, ctx, preparations, providers, gateway, fixture, "materialize-partial",
		stagedOutputPlan{outputKey: "image-1", contents: firstBytes, upload: true, declaredWidth: 1536, declaredHeight: 1024},
	)
	failOnce := &failOnceCandidateOwner{delegate: candidateService}
	resumable := generationapp.NewOutputMaterializationService(
		providers, generationasset.NewProviderOutputReadiness(assetService), failOnce,
	)
	if _, err = resumable.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: partial.Job.ID,
	}); err == nil {
		t.Fatal("injected Candidate Owner outage did not interrupt materialization")
	}
	assertMaterializedFactCounts(t, countRecords, fixture.workspaceID.String(), 3, 2)
	if _, err = resumable.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: partial.Job.ID,
	}); err != nil {
		t.Fatalf("resume materialization from persisted Asset receipts: %v", err)
	}
	assertMaterializedFactCounts(t, countRecords, fixture.workspaceID.String(), 3, 3)

	missing := submitStagedProviderJob(t, ctx, preparations, providers, gateway, fixture, "materialize-missing",
		stagedOutputPlan{outputKey: "image-1", contents: firstBytes, upload: false, declaredWidth: 1536, declaredHeight: 1024},
	)
	if _, err = materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: missing.Job.ID,
	}); generationErrorCode(err) != "dependency_unavailable" {
		t.Fatalf("missing private Staging object did not remain retryable: %T %v", err, err)
	}
	assertArtifactState(t, loadArtifact, countRecords, successfulProviderReceiptID(t, missing), "PENDING_VALIDATION", "")

	corrupt := []byte("\x89PNG\r\n\x1a\nnot-a-decodable-png")
	quarantined := submitStagedProviderJob(t, ctx, preparations, providers, gateway, fixture, "materialize-quarantine",
		stagedOutputPlan{outputKey: "image-1", contents: corrupt, upload: true, declaredWidth: 1536, declaredHeight: 1024},
	)
	if _, err = materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: quarantined.Job.ID,
	}); generationErrorCode(err) != "artifact_not_ready" {
		t.Fatalf("quarantined Provider output created a Candidate: %T %v", err, err)
	}
	assertArtifactState(t, loadArtifact, countRecords, successfulProviderReceiptID(t, quarantined), "QUARANTINED", "image_decode_failed")

	smallBytes := testPNG(t, 4, 3)
	dimensionDrift := submitStagedProviderJob(t, ctx, preparations, providers, gateway, fixture, "materialize-dimension-drift",
		stagedOutputPlan{outputKey: "image-1", contents: smallBytes, upload: true, declaredWidth: 1536, declaredHeight: 1024},
	)
	if _, err = materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: dimensionDrift.Job.ID,
	}); generationErrorCode(err) != "artifact_not_ready" {
		t.Fatalf("Provider output dimension drift was not quarantined: %T %v", err, err)
	}
	dimensionReceiptID := successfulProviderReceiptID(t, dimensionDrift)
	assertArtifactState(t, loadArtifact, countRecords, dimensionReceiptID, "QUARANTINED", "image_dimensions_mismatch")
	dimensionArtifact, err := loadArtifact(dimensionReceiptID)
	if err != nil {
		t.Fatalf("load dimension-quarantined Provider output Artifact: %v", err)
	}
	if dimensionArtifact.Width == nil || *dimensionArtifact.Width != 4 ||
		dimensionArtifact.Height == nil || *dimensionArtifact.Height != 3 || dimensionArtifact.Revision != 2 {
		t.Fatalf("dimension-quarantined Provider output did not retain actual metadata: %#v", dimensionArtifact)
	}
	var dimensionLocation model.ArtifactLocation
	if err = database.Where("artifact_id = ? AND location_no = ?", dimensionArtifact.ID, 1).First(&dimensionLocation).Error; err != nil {
		t.Fatalf("load dimension-quarantined Provider output Location: %v", err)
	}
	if dimensionLocation.Status != "STAGING" {
		t.Fatalf("dimension-quarantined Provider output Location = %s, want STAGING", dimensionLocation.Status)
	}
	if _, err = materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: dimensionDrift.Job.ID,
	}); generationErrorCode(err) != "artifact_not_ready" {
		t.Fatalf("replayed Provider output dimension quarantine drifted: %T %v", err, err)
	}
	var dimensionValidationReceipts int64
	if err = database.Model(&model.CommandReceipt{}).
		Where("operation = ? AND resource_id = ?", "asset.artifact.validate_ready", dimensionArtifact.ID).
		Count(&dimensionValidationReceipts).Error; err != nil {
		t.Fatalf("count dimension-quarantined Provider output validation receipts: %v", err)
	}
	if dimensionValidationReceipts != 1 {
		t.Fatalf("dimension-quarantined Provider output validation receipts = %d, want 1", dimensionValidationReceipts)
	}

	selfConsistentMismatch := submitStagedProviderJob(
		t,
		ctx,
		preparations,
		providers,
		gateway,
		fixture,
		"materialize-self-consistent-target-mismatch",
		stagedOutputPlan{
			outputKey: "output-1", contents: smallBytes, upload: true,
			declaredWidth: 4, declaredHeight: 3,
		},
	)
	if _, err = materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: selfConsistentMismatch.Job.ID,
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("self-consistent 4x3 Provider output crossed the frozen 1536x1024 Target: %T %v", err, err)
	}
	selfConsistentReceiptID := successfulProviderReceiptID(t, selfConsistentMismatch)
	for _, check := range []struct {
		name  string
		model any
		query string
	}{
		{name: "Artifact", model: &model.Artifact{}, query: "source_id = ?"},
		{name: "Candidate", model: &model.GenerationCandidate{}, query: "provider_receipt_id = ?"},
	} {
		count, countErr := countRecords(check.model, check.query, selfConsistentReceiptID)
		if countErr != nil || count != 0 {
			t.Fatalf("target-mismatched Provider output created a %s: count=%d err=%v", check.name, count, countErr)
		}
	}

	if err = database.Model(&model.UserAccount{}).Where("id = ?", fixture.editorID).Update("token_version", 2).Error; err != nil {
		t.Fatalf("revoke Provider output actor: %v", err)
	}
	if _, err = materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: success.Job.ID,
	}); generationErrorCode(err) != "unauthenticated" {
		t.Fatalf("revoked Provider output actor was accepted: %T %v", err, err)
	}
}

func submitStagedProviderJob(
	t *testing.T,
	ctx context.Context,
	preparations *generationapp.PreparationService,
	providers *generationapp.ProviderService,
	gateway *stagingProviderGateway,
	fixture preparationFixture,
	suffix string,
	outputs ...stagedOutputPlan,
) generationapp.ProviderExecutionResult {
	t.Helper()
	if len(outputs) == 0 || len(outputs) > 4 {
		t.Fatalf("staged Provider job %s requires between one and four successful output plans", suffix)
	}
	plan := append([]stagedOutputPlan(nil), outputs...)
	for len(plan) < 4 {
		plan = append(plan, stagedOutputPlan{failureCode: "provider.not_generated"})
	}
	gateway.setPlan(plan...)
	claim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, suffix, strings.Repeat("e", 64))
	var result generationapp.ProviderExecutionResult
	var err error
	for index := range plan {
		result, err = providers.SubmitImageRequest(ctx, claim.Authorization, generationapp.SubmitImageRequestCommand{
			IntentID:       claim.Intent.ID,
			IdempotencyKey: "provider-output-submit-" + suffix + "-" + strconv.Itoa(index+1),
		})
		if err != nil {
			t.Fatalf("submit staged Provider Call %s/%d: result=%#v err=%v", suffix, index+1, result, err)
		}
	}
	wantStatus := generationdomain.ProviderJobPartialSucceeded
	if len(outputs) == 4 {
		wantStatus = generationdomain.ProviderJobSucceeded
	}
	if result.Job.Status != wantStatus || len(result.Receipts) != len(plan) {
		t.Fatalf("submit staged Provider job %s: result=%#v err=%v", suffix, result, err)
	}
	succeeded := 0
	for _, receipt := range result.Receipts {
		if receipt.Status == generationdomain.ProviderResultSucceeded {
			if receipt.Output == nil {
				t.Fatalf("successful staged Provider Call has no output: %#v", receipt)
			}
			succeeded++
		} else if receipt.Status != generationdomain.ProviderResultFailed || receipt.Output != nil {
			t.Fatalf("staged Provider Call receipt drifted: %#v", receipt)
		}
	}
	if succeeded != len(outputs) {
		t.Fatalf("staged Provider job %s succeeded Calls = %d, want %d", suffix, succeeded, len(outputs))
	}
	return result
}

func assertMaterializedFactCounts(
	t *testing.T,
	countRecords countPreparationRecords,
	workspaceID string,
	artifacts, candidates int64,
) {
	t.Helper()
	checks := []struct {
		name  string
		model any
		want  int64
	}{
		{name: "Artifacts", model: &model.Artifact{}, want: artifacts},
		{name: "Artifact locations", model: &model.ArtifactLocation{}, want: artifacts},
		{name: "Generation candidates", model: &model.GenerationCandidate{}, want: candidates},
		{name: "Generation QC reports", model: &model.GenerationQCReport{}, want: candidates},
	}
	for _, check := range checks {
		count, err := countRecords(check.model, "workspace_id = ?", workspaceID)
		if err != nil {
			t.Fatalf("count materialized %s: %v", check.name, err)
		}
		if count != check.want {
			t.Fatalf("materialized %s count = %d, want %d", check.name, count, check.want)
		}
	}
}

func assertArtifactState(
	t *testing.T,
	loadArtifact loadProviderOutputArtifact,
	countRecords countPreparationRecords,
	providerReceiptID, status, failureCode string,
) {
	t.Helper()
	artifact, err := loadArtifact(providerReceiptID)
	if err != nil {
		t.Fatalf("load Provider output Artifact: %v", err)
	}
	actualFailureCode := ""
	if artifact.FailureCode != nil {
		actualFailureCode = *artifact.FailureCode
	}
	if artifact.Status != status || actualFailureCode != failureCode {
		t.Fatalf("Provider output Artifact state = %s/%s, want %s/%s", artifact.Status, actualFailureCode, status, failureCode)
	}
	candidateCount, err := countRecords(&model.GenerationCandidate{}, "provider_receipt_id = ?", providerReceiptID)
	if err != nil {
		t.Fatalf("count Provider output Candidates: %v", err)
	}
	if candidateCount != 0 {
		t.Fatalf("non-materializable Provider output created %d Candidates", candidateCount)
	}
}

func successfulProviderReceiptID(t *testing.T, result generationapp.ProviderExecutionResult) string {
	t.Helper()
	for _, receipt := range result.Receipts {
		if receipt.Status == generationdomain.ProviderResultSucceeded {
			return receipt.ID
		}
	}
	t.Fatal("Provider result has no successful receipt")
	return ""
}
