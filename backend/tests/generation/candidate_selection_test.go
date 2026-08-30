package generation_test

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	assetgorm "github.com/StephenQiu30/lanverse/backend/internal/asset/adapter/gormdb"
	assetapp "github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	generationasset "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/asset"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationreview "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/review"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstore"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	reviewdomain "github.com/StephenQiu30/lanverse/backend/internal/review/domain"
)

func TestSelectedReviewDecisionCreatesOneGenerationSelection(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	minioEndpoint := os.Getenv("LANVERSE_TEST_MINIO_ENDPOINT")
	minioAccessKey := os.Getenv("LANVERSE_TEST_MINIO_ACCESS_KEY")
	minioSecretKey := os.Getenv("LANVERSE_TEST_MINIO_SECRET_KEY")
	minioBucket := os.Getenv("LANVERSE_TEST_MINIO_BUCKET")
	if databaseURL == "" || minioEndpoint == "" || minioAccessKey == "" || minioSecretKey == "" || minioBucket == "" {
		t.Skip("set PostgreSQL and MinIO test variables to run the generation selection journey")
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

	now := time.Date(2026, time.August, 26, 14, 0, 0, 0, time.UTC)
	workspaceID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	if err = database.Create(&model.UserAccount{ID: userID, EmailNormalized: "generation-selector-" + userID.String() + "@example.test", PasswordHash: "test", TokenVersion: 1, DisplayName: "Generation Selector", Status: "active", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed generation selector: %v", err)
	}
	if err = database.Create(&model.Workspace{ID: workspaceID, Name: "Generation Selection", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed selection workspace: %v", err)
	}
	if err = database.Create(&model.Membership{ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, Role: "editor", Status: "active", JoinedAt: now}).Error; err != nil {
		t.Fatalf("seed selection membership: %v", err)
	}
	if err = database.Create(&model.Project{ID: projectID, WorkspaceID: workspaceID, Name: "Selection Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed selection project: %v", err)
	}

	assetService := assetapp.NewService(assetgorm.New(database), objects, assetapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
		Bucket: minioBucket, StorageProfile: "private-primary", Region: "us-east-1", MaxImageBytes: 20 << 20,
	})
	generationStore := generationgorm.New(database)
	candidateService := generationapp.NewService(generationStore, generationasset.NewReadiness(assetService), generationapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
		ImageQC: generationapp.ImageQCPolicy{Version: "selection-image-qc", AllowedMediaTypes: []string{"image/png"}, MinWidth: 4, MinHeight: 3, MaxPixels: 100},
	})
	actor := generationapp.Actor{UserID: userID.String(), TokenVersion: 1}
	assetActor := assetapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}
	firstArtifact := createArtifact(t, ctx, objects, assetService, assetActor, workspaceID.String(), projectID.String(), 4, 3, "selection-first", true)
	secondArtifact := createArtifact(t, ctx, objects, assetService, assetActor, workspaceID.String(), projectID.String(), 5, 4, "selection-second", true)
	failedArtifact := createArtifact(t, ctx, objects, assetService, assetActor, workspaceID.String(), projectID.String(), 2, 2, "selection-failed", true)
	firstCandidate := registerCandidate(t, ctx, candidateService, actor, firstArtifact.ID, "selection-candidate-first")
	secondCandidate := registerCandidate(t, ctx, candidateService, actor, secondArtifact.ID, "selection-candidate-second")
	failedCandidate := registerCandidate(t, ctx, candidateService, actor, failedArtifact.ID, "selection-candidate-failed")

	reviewService := reviewapp.NewService(reviewgorm.New(database), reviewapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString, ClaimLease: 5 * time.Minute,
	})
	decision := selectedDecision(t, ctx, reviewService, reviewapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, now,
		workspaceID.String(), projectID.String(), []string{firstCandidate.Candidate.ID, secondCandidate.Candidate.ID}, secondCandidate.Candidate.ID, "selection-main")
	selectionService := generationapp.NewSelectionService(
		generationStore, candidateService, generationreview.NewDecisionReader(reviewService),
		generationapp.SelectionConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	command := generationapp.ApplySelectionCommand{ReviewDecisionID: decision.Decision.ID, IdempotencyKey: "apply-selection-main"}

	const callers = 8
	results := make(chan generationapp.ApplySelectionResult, callers)
	errorsFound := make(chan error, callers)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, applyErr := selectionService.ApplySelection(ctx, actor, command)
			if applyErr != nil {
				errorsFound <- applyErr
				return
			}
			results <- result
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsFound)
	for applyErr := range errorsFound {
		t.Fatalf("apply generation selection concurrently: %v", applyErr)
	}
	var selectionID, receiptID, contentHash string
	for result := range results {
		if selectionID == "" {
			selectionID, receiptID, contentHash = result.Selection.ID, result.Receipt.ID, result.Selection.ContentHash
		}
		if result.Selection.ID != selectionID || result.Receipt.ID != receiptID || result.Selection.ContentHash != contentHash ||
			result.Selection.SelectedCandidateID != secondCandidate.Candidate.ID || result.Selection.ReviewDecisionID != decision.Decision.ID ||
			result.Selection.HumanTaskID != decision.Task.ID || result.Selection.ReviewerID != actor.UserID ||
			len(result.Selection.Candidates) != 2 || result.Selection.CandidateSetHash == "" || result.Selection.ContentHash == "" {
			t.Fatalf("concurrent generation selection drifted: %#v", result)
		}
	}
	selected, err := selectionService.RequireSelected(ctx, actor, selectionID)
	if err != nil || selected.ID != selectionID || selected.SelectedCandidateID != secondCandidate.Candidate.ID {
		t.Fatalf("require selected generation candidate: selection=%#v err=%v", selected, err)
	}
	replayed, err := selectionService.ApplySelection(ctx, actor, command)
	if err != nil || replayed.Selection.ID != selectionID || replayed.Receipt.ID != receiptID {
		t.Fatalf("replay generation selection: result=%#v err=%v", replayed, err)
	}
	redelivered, err := selectionService.ApplySelection(ctx, actor, generationapp.ApplySelectionCommand{
		ReviewDecisionID: decision.Decision.ID, IdempotencyKey: "apply-selection-main-redelivery",
	})
	if err != nil || redelivered.Selection.ID != selectionID || redelivered.Receipt.ID == receiptID {
		t.Fatalf("redeliver generation selection with a new key: result=%#v err=%v", redelivered, err)
	}

	invalidTask, err := reviewService.Open(ctx, reviewapp.OpenCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
		SubjectType: "generation_candidate_selection", SubjectID: uuid.NewString(), SubjectRevision: 1,
		SubjectHash: strings.Repeat("b", 64), CandidateIDs: []string{firstCandidate.Candidate.ID},
		AllowedDecisions: []string{"changes_requested", "rejected", "selected"}, RubricVersion: "generation-image-selection",
	})
	if err != nil {
		t.Fatalf("open frozen Generation selection rubric: %v", err)
	}
	invalidClaim, err := reviewService.Claim(ctx, reviewapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, reviewapp.ClaimCommand{
		TaskID: invalidTask.ID, ExpectedRevision: invalidTask.Revision, IdempotencyKey: "selection-approved-claim",
	})
	if err != nil {
		t.Fatalf("claim frozen Generation selection rubric: %v", err)
	}
	if _, err = reviewService.Decide(ctx, reviewapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, reviewapp.DecideCommand{
		TaskID: invalidTask.ID, ClaimToken: invalidClaim.ClaimToken, ExpectedTaskRevision: invalidClaim.Task.Revision,
		ExpectedSubjectRevision: invalidTask.SubjectRevision, ExpectedSubjectHash: invalidTask.SubjectHash,
		Decision: "approved", IdempotencyKey: "selection-approved-decision",
	}); err == nil {
		t.Fatal("Generation selection rubric accepted an approved decision")
	}
	failed := selectedDecision(t, ctx, reviewService, reviewapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, now,
		workspaceID.String(), projectID.String(), []string{firstCandidate.Candidate.ID, failedCandidate.Candidate.ID}, firstCandidate.Candidate.ID, "selection-qc-failed")
	if _, err = selectionService.ApplySelection(ctx, actor, generationapp.ApplySelectionCommand{
		ReviewDecisionID: failed.Decision.ID, IdempotencyKey: "apply-selection-qc-failed",
	}); err == nil {
		t.Fatal("QC-failed candidate created a CandidateSelection")
	}
	drift := selectedDecision(t, ctx, reviewService, reviewapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, now,
		workspaceID.String(), projectID.String(), []string{firstCandidate.Candidate.ID}, firstCandidate.Candidate.ID, "selection-drift")
	if _, err = selectionService.ApplySelection(ctx, actor, generationapp.ApplySelectionCommand{
		ReviewDecisionID: drift.Decision.ID, IdempotencyKey: command.IdempotencyKey,
	}); err == nil {
		t.Fatal("selection idempotency key accepted a different ReviewDecision")
	}

	if err = database.Model(&model.UserAccount{}).Where("id = ?", userID).Update("token_version", 2).Error; err != nil {
		t.Fatalf("revoke selection actor token: %v", err)
	}
	if _, err = selectionService.RequireSelected(ctx, actor, selectionID); err == nil {
		t.Fatal("revoked actor consumed a CandidateSelection")
	}
	if err = database.Model(&model.UserAccount{}).Where("id = ?", userID).Update("token_version", 1).Error; err != nil {
		t.Fatalf("restore selection actor token: %v", err)
	}

	var selectionCount, receiptCount int64
	if err = database.Model(&model.GenerationCandidateSelection{}).Where("workspace_id = ?", workspaceID).Count(&selectionCount).Error; err != nil {
		t.Fatalf("count generation selections: %v", err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where("workspace_id = ? AND operation = ?", workspaceID, "generation.candidate.select").Count(&receiptCount).Error; err != nil {
		t.Fatalf("count generation selection receipts: %v", err)
	}
	if selectionCount != 1 || receiptCount != 2 {
		t.Fatalf("generation selection fact counts = selections %d receipts %d", selectionCount, receiptCount)
	}
	if err = database.Model(&model.ReviewDecision{}).Where("id = ?", decision.Decision.ID).
		Update("selected_candidate_id", firstCandidate.Candidate.ID).Error; err != nil {
		t.Fatalf("inject review decision drift: %v", err)
	}
	if _, err = selectionService.RequireSelected(ctx, actor, selectionID); err == nil {
		t.Fatal("drifted ReviewDecision passed RequireSelected")
	}
	if err = database.Model(&model.ReviewDecision{}).Where("id = ?", decision.Decision.ID).
		Update("selected_candidate_id", secondCandidate.Candidate.ID).Error; err != nil {
		t.Fatalf("restore review decision: %v", err)
	}
	if err = database.Model(&model.GenerationQCReport{}).Where("id = ?", secondCandidate.Report.ID).
		Update("report_hash", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatalf("inject selected candidate QC drift: %v", err)
	}
	if _, err = selectionService.RequireSelected(ctx, actor, selectionID); err == nil {
		t.Fatal("drifted selected candidate QC passed RequireSelected")
	}
	if err = database.Model(&model.GenerationQCReport{}).Where("id = ?", secondCandidate.Report.ID).
		Update("report_hash", secondCandidate.Report.ReportHash).Error; err != nil {
		t.Fatalf("restore selected candidate QC report: %v", err)
	}
	if err = database.Model(&model.GenerationCandidateSelection{}).Where("id = ?", selectionID).Update("content_hash", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatalf("inject generation selection drift: %v", err)
	}
	if _, err = selectionService.RequireSelected(ctx, actor, selectionID); err == nil {
		t.Fatal("drifted CandidateSelection passed RequireSelected")
	}
}

func registerCandidate(t *testing.T, ctx context.Context, service *generationapp.Service, actor generationapp.Actor, artifactID, key string) generationapp.RegisterCandidateResult {
	t.Helper()
	result, err := service.RegisterReadyCandidate(ctx, actor, generationapp.RegisterReadyCandidateCommand{ArtifactID: artifactID, IdempotencyKey: key})
	if err != nil {
		t.Fatalf("register %s generation candidate: %v", key, err)
	}
	return result
}

func selectedDecision(t *testing.T, ctx context.Context, service *reviewapp.Service, actor reviewapp.Actor, now time.Time, workspaceID, projectID string, candidateIDs []string, selectedCandidateID, key string) reviewdomain.DecisionResult {
	t.Helper()
	return decidedReview(t, ctx, service, actor, now, workspaceID, projectID, candidateIDs, "selected", selectedCandidateID, key)
}

func decidedReview(t *testing.T, ctx context.Context, service *reviewapp.Service, actor reviewapp.Actor, now time.Time, workspaceID, projectID string, candidateIDs []string, decision, selectedCandidateID, key string) reviewdomain.DecisionResult {
	t.Helper()
	task, err := service.Open(ctx, reviewapp.OpenCommand{
		WorkspaceID: workspaceID, ProjectID: projectID, WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
		SubjectType: "generation_candidate_selection", SubjectID: uuid.NewString(), SubjectRevision: 1,
		SubjectHash: strings.Repeat("a", 64), CandidateIDs: candidateIDs,
		AllowedDecisions: []string{"changes_requested", "rejected", "selected"}, RubricVersion: "generation-image-selection",
	})
	if err != nil {
		t.Fatalf("open %s generation selection review: %v", key, err)
	}
	claimed, err := service.Claim(ctx, actor, reviewapp.ClaimCommand{TaskID: task.ID, ExpectedRevision: task.Revision, IdempotencyKey: key + "-claim"})
	if err != nil {
		t.Fatalf("claim %s generation selection review: %v", key, err)
	}
	result, err := service.Decide(ctx, actor, reviewapp.DecideCommand{
		TaskID: task.ID, ClaimToken: claimed.ClaimToken, Decision: decision, SelectedCandidateID: selectedCandidateID,
		ExpectedTaskRevision: claimed.Task.Revision, ExpectedSubjectRevision: task.SubjectRevision,
		ExpectedSubjectHash: task.SubjectHash, IdempotencyKey: key + "-decide",
	})
	if err != nil {
		t.Fatalf("decide %s generation selection review at %s: %v", key, now, err)
	}
	return result
}
