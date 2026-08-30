package generation_test

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

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
)

var (
	_ generationapp.SucceededProviderResultOwner = (*generationapp.ProviderService)(nil)
	_ generationapp.ProviderOutputAssetOwner     = (*generationasset.ProviderOutputReadiness)(nil)
	_ generationapp.ReadyCandidateOwner          = (*generationapp.Service)(nil)
)

type stagedOutputPlan struct {
	outputKey                     string
	objectKeyOverride             string
	contents                      []byte
	upload                        bool
	declaredWidth, declaredHeight int
}

type loadProviderOutputArtifact func(string) (model.Artifact, error)

type stagingProviderGateway struct {
	t       *testing.T
	objects *objectstore.Client

	mu   sync.Mutex
	plan []stagedOutputPlan
}

func (gateway *stagingProviderGateway) setPlan(plan ...stagedOutputPlan) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	gateway.plan = append([]stagedOutputPlan(nil), plan...)
}

func (gateway *stagingProviderGateway) Submit(
	ctx context.Context,
	submission generationapp.ProviderSubmission,
) (generationapp.ProviderOutcome, error) {
	gateway.mu.Lock()
	plan := append([]stagedOutputPlan(nil), gateway.plan...)
	gateway.mu.Unlock()
	if submission.WorkspaceID == "" || submission.ProjectID == "" || submission.ProviderJobID == "" ||
		submission.RequestID == "" || submission.IntentID == "" {
		return generationapp.ProviderOutcome{}, errors.New("Provider submission did not freeze owner identities")
	}
	outputs := make([]generationapp.ProviderOutput, 0, len(plan))
	for _, item := range plan {
		objectKey := "staging/" + submission.WorkspaceID + "/" + submission.ProviderJobID + "/" + item.outputKey + ".png"
		if item.objectKeyOverride != "" {
			objectKey = item.objectKeyOverride
		}
		if item.upload {
			putObject(gateway.t, ctx, gateway.objects, objectKey, item.contents)
		}
		outputs = append(outputs, generationapp.ProviderOutput{
			OutputKey: item.outputKey, StagingObjectKey: objectKey, SHA256: sha256Hex(item.contents),
			Bytes: int64(len(item.contents)), MediaType: "image/png",
			Width: item.declaredWidth, Height: item.declaredHeight,
		})
	}
	return generationapp.ProviderOutcome{
		Status: generationapp.ProviderOutcomeSucceeded, ProviderJobKey: "staging-job-" + submission.ProviderJobID,
		ProviderEventID: "staging-event-" + submission.ProviderJobID, ActualUnits: int64(len(outputs)), Outputs: outputs,
	}, nil
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
	providerJobID, outputKey string,
) (generationdomain.CandidateWithReport, error) {
	return owner.delegate.RequireEvaluatedProviderOutput(ctx, actor, providerJobID, outputKey)
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

	now := time.Date(2026, time.August, 26, 23, 0, 0, 0, time.UTC)
	create := func(value any) error { return database.Create(value).Error }
	countRecords := func(value any, query string, arguments ...any) (int64, error) {
		var count int64
		err := database.Model(value).Where(query, arguments...).Count(&count).Error
		return count, err
	}
	loadArtifact := func(providerJobID string) (model.Artifact, error) {
		var artifact model.Artifact
		err := database.Where("source_type = ? AND source_id = ?", "generation_provider_job", providerJobID).
			First(&artifact).Error
		return artifact, err
	}
	fixture := seedPreparationFixture(t, create, generationgorm.NewTargetStore(database), now, "provider-output")
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
			return database.Where(query, arguments...).Delete(value).Error
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
	gateway := &stagingProviderGateway{t: t, objects: objects}
	bindingResolver := &controlledBindingResolver{}
	bindingResolver.set(seedControlledProjectProviderBinding(
		t, create, fixture, "staging-image", "image-quality", 1,
	))
	providers := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig), gateway,
		generationapp.ProviderConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString, Bindings: bindingResolver,
		},
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
				MinWidth: 4, MinHeight: 3, MaxPixels: 100,
			},
		},
	)
	materializer := generationapp.NewOutputMaterializationService(
		providers, generationasset.NewProviderOutputReadiness(assetService), candidateService,
	)
	invalidStagingClaim := prepareAndClaimProviderIntent(
		t, ctx, preparations, fixture, 1, "materialize-invalid-staging", strings.Repeat("d", 64),
	)
	gateway.setPlan(stagedOutputPlan{
		outputKey: "image-1", objectKeyOverride: "staging/another-workspace/another-job/image-1.png",
		contents: testPNG(t, 4, 3), upload: false, declaredWidth: 4, declaredHeight: 3,
	})
	invalidStaging, err := providers.SubmitImageRequest(ctx, invalidStagingClaim.Authorization, generationapp.SubmitImageRequestCommand{
		IntentID: invalidStagingClaim.Intent.ID, IdempotencyKey: "provider-output-submit-invalid-staging",
	})
	if err != nil || invalidStaging.Job.Status != generationdomain.ProviderJobUnknown ||
		invalidStaging.Intent.Status != generationdomain.IntentOutcomeUnknown || invalidStaging.ProviderReceipt.ID != "" {
		t.Fatalf("Provider output outside frozen Staging prefix was accepted: result=%#v err=%v", invalidStaging, err)
	}
	if _, err = materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: invalidStaging.Job.ID,
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("unknown invalid-Staging Provider result was materialized: %T %v", err, err)
	}

	firstBytes, secondBytes := testPNG(t, 4, 3), testPNG(t, 5, 4)
	success := submitStagedProviderJob(t, ctx, preparations, providers, gateway, fixture, "materialize-main",
		stagedOutputPlan{outputKey: "image-1", contents: firstBytes, upload: true, declaredWidth: 4, declaredHeight: 3},
		stagedOutputPlan{outputKey: "image-2", contents: secondBytes, upload: true, declaredWidth: 5, declaredHeight: 4},
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
		if len(result.Outputs) != 2 || result.ProviderReceiptID != success.ProviderReceipt.ID {
			t.Fatalf("unexpected Provider materialization result: %#v", result)
		}
		if canonical.ProviderReceiptID == "" {
			canonical = result
			continue
		}
		for index := range result.Outputs {
			if result.Outputs[index].Artifact.ID != canonical.Outputs[index].Artifact.ID ||
				result.Outputs[index].Candidate.ID != canonical.Outputs[index].Candidate.ID ||
				result.Outputs[index].Report.ID != canonical.Outputs[index].Report.ID {
				t.Fatalf("concurrent materialization did not converge: first=%#v next=%#v", canonical, result)
			}
		}
	}
	assertMaterializedFactCounts(t, countRecords, fixture.workspaceID.String(), 2, 2)
	if canonical.CandidateSet.ID != success.Job.ID || canonical.CandidateSet.ProviderReceiptID != success.ProviderReceipt.ID ||
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

	replayed, err := materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: success.Job.ID,
	})
	if err != nil || replayed.Outputs[0].Candidate.ID != canonical.Outputs[0].Candidate.ID ||
		replayed.Outputs[1].Candidate.ID != canonical.Outputs[1].Candidate.ID {
		t.Fatalf("replay Provider output materialization: result=%#v err=%v", replayed, err)
	}

	partial := submitStagedProviderJob(t, ctx, preparations, providers, gateway, fixture, "materialize-partial",
		stagedOutputPlan{outputKey: "image-1", contents: firstBytes, upload: true, declaredWidth: 4, declaredHeight: 3},
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
		stagedOutputPlan{outputKey: "image-1", contents: firstBytes, upload: false, declaredWidth: 4, declaredHeight: 3},
	)
	if _, err = materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: missing.Job.ID,
	}); generationErrorCode(err) != "dependency_unavailable" {
		t.Fatalf("missing private Staging object did not remain retryable: %T %v", err, err)
	}
	assertArtifactState(t, loadArtifact, countRecords, missing.Job.ID, "PENDING_VALIDATION", "")

	corrupt := []byte("\x89PNG\r\n\x1a\nnot-a-decodable-png")
	quarantined := submitStagedProviderJob(t, ctx, preparations, providers, gateway, fixture, "materialize-quarantine",
		stagedOutputPlan{outputKey: "image-1", contents: corrupt, upload: true, declaredWidth: 4, declaredHeight: 3},
	)
	if _, err = materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: quarantined.Job.ID,
	}); generationErrorCode(err) != "artifact_not_ready" {
		t.Fatalf("quarantined Provider output created a Candidate: %T %v", err, err)
	}
	assertArtifactState(t, loadArtifact, countRecords, quarantined.Job.ID, "QUARANTINED", "image_decode_failed")

	dimensionDrift := submitStagedProviderJob(t, ctx, preparations, providers, gateway, fixture, "materialize-dimension-drift",
		stagedOutputPlan{outputKey: "image-1", contents: firstBytes, upload: true, declaredWidth: 5, declaredHeight: 3},
	)
	if _, err = materializer.MaterializeSucceededOutputs(ctx, fixture.editor, generationapp.MaterializeProviderOutputsCommand{
		ProviderJobID: dimensionDrift.Job.ID,
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Provider output dimension drift was accepted: %T %v", err, err)
	}
	assertArtifactState(t, loadArtifact, countRecords, dimensionDrift.Job.ID, "READY", "")

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
	gateway.setPlan(outputs...)
	claim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, int64(len(outputs)), suffix, strings.Repeat("e", 64))
	result, err := providers.SubmitImageRequest(ctx, claim.Authorization, generationapp.SubmitImageRequestCommand{
		IntentID: claim.Intent.ID, IdempotencyKey: "provider-output-submit-" + suffix,
	})
	if err != nil || result.Job.Status != generationdomain.ProviderJobSucceeded || result.ProviderReceipt.Status != generationdomain.ProviderResultSucceeded {
		t.Fatalf("submit staged Provider job %s: result=%#v err=%v", suffix, result, err)
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
	providerJobID, status, failureCode string,
) {
	t.Helper()
	artifact, err := loadArtifact(providerJobID)
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
	candidateCount, err := countRecords(&model.GenerationCandidate{}, "provider_job_id = ?", providerJobID)
	if err != nil {
		t.Fatalf("count Provider output Candidates: %v", err)
	}
	if candidateCount != 0 {
		t.Fatalf("non-materializable Provider output created %d Candidates", candidateCount)
	}
}
