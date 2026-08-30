package generation_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	authoringdomain "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	costgorm "github.com/StephenQiu30/lanverse/backend/internal/cost/adapter/gormdb"
	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	quotagorm "github.com/StephenQiu30/lanverse/backend/internal/quota/adapter/gormdb"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	quotadomain "github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const preparationClaimTTL = 5 * time.Minute

type preparationFixture struct {
	workspaceID, projectID uuid.UUID
	workflowRunID          uuid.UUID
	workflowDefinitionID   uuid.UUID
	runInputSnapshotID     uuid.UUID
	ownerID, editorID      uuid.UUID
	owner, editor          generationapp.Actor
	create                 func(any) error
	targets                *generationgorm.TargetStore
	provider               generationapp.ResolvedProjectProviderBinding
	now                    time.Time
}

type countPreparationRecords func(any, string, ...any) (int64, error)

func TestExpensiveImagePreparationClaimAndCancellationAreAtomic(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Generation expensive operation journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open Generation preparation test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}
	assertPreparationSchema(t,
		func(constraint string) bool {
			return database.Migrator().HasConstraint(&model.GenerationIntent{}, constraint)
		},
		func(index string) bool { return database.Migrator().HasIndex(&model.GenerationIntent{}, index) },
	)
	assertGenerationTargetSchema(t,
		func(constraint string) bool {
			return database.Migrator().HasConstraint(&model.GenerationTarget{}, constraint)
		},
		func(index string) bool { return database.Migrator().HasIndex(&model.GenerationTarget{}, index) },
	)
	create := func(value any) error { return database.Create(value).Error }
	countRecords := func(value any, query string, arguments ...any) (int64, error) {
		var count int64
		err := database.Model(value).Where(query, arguments...).Count(&count).Error
		return count, err
	}

	now := time.Date(2026, time.August, 26, 18, 0, 0, 0, time.UTC)
	currentTime := now
	targets := generationgorm.NewTargetStore(database)
	main := seedPreparationFixture(t, create, targets, now, "main")
	quotaFailure := seedPreparationFixture(t, create, targets, now, "quota-failure")
	main.provider = seedControlledProjectProviderBinding(t, create, main, "controlled-image", "image-quality", 1)
	quotaFailure.provider = seedControlledProjectProviderBinding(
		t, create, quotaFailure, "controlled-image", "image-quality", 1,
	)
	t.Cleanup(func() {
		for _, fixture := range []preparationFixture{main, quotaFailure} {
			deletions := []struct {
				name string
				err  error
			}{
				{"Generation intents", database.Where("workspace_id = ?", fixture.workspaceID).Delete(&model.GenerationIntent{}).Error},
				{"Workflow node runs", database.Where("workflow_run_id = ?", fixture.workflowRunID).Delete(&model.NodeRunProjection{}).Error},
				{"Workflow run", database.Where("id = ?", fixture.workflowRunID).Delete(&model.WorkflowRun{}).Error},
				{"Workflow input snapshot", database.Where("id = ?", fixture.runInputSnapshotID).Delete(&model.RunInputSnapshot{}).Error},
				{"Workflow definition", database.Where("id = ?", fixture.workflowDefinitionID).Delete(&model.WorkflowDefinitionVersion{}).Error},
			}
			for _, deletion := range deletions {
				if deletion.err != nil {
					t.Errorf("clean test-owned %s: %v", deletion.name, deletion.err)
				}
			}
		}
	})

	costConfig := costapp.Config{Now: func() time.Time { return currentTime }, NewID: uuid.NewString}
	quotaConfig := quotaapp.Config{Now: func() time.Time { return currentTime }, NewID: uuid.NewString}
	costs := costapp.NewService(costgorm.New(database), costConfig)
	quotas := quotaapp.NewService(quotagorm.New(database), quotaConfig)
	configurePreparationLimits(t, ctx, costs, quotas, main, "1000.000000", "10.000000", 100)
	configurePreparationLimits(t, ctx, costs, quotas, quotaFailure, "1000.000000", "10.000000", 1)

	preparations := generationapp.NewPreparationService(
		generationgorm.NewPreparationStore(database, costConfig, quotaConfig),
		generationapp.PreparationConfig{
			Now: func() time.Time { return currentTime }, NewID: uuid.NewString, ClaimTTL: preparationClaimTTL,
		},
	)

	failedCommand := newPreparationCommand(t, quotaFailure, "quota-failure", strings.Repeat("f", 64))
	if _, err = preparations.PrepareImageGeneration(ctx, quotaFailure.editor, failedCommand); generationErrorCode(err) != "quota_exceeded" {
		t.Fatalf("quota failure did not reject atomic preparation: %T %v", err, err)
	}
	assertNoPreparedFacts(t, countRecords, quotaFailure)

	command := newPreparationCommand(t, main, "main", strings.Repeat("a", 64))
	assertGenerationTargetPersistence(
		t, ctx, main.targets, command.TargetID, command.TargetHash,
		func(id string) error {
			return database.Model(&model.GenerationTarget{}).Where("id = ?", id).
				Update("target_hash", strings.Repeat("0", 64)).Error
		},
		func(id string) error { return database.Where("id = ?", id).Delete(&model.GenerationTarget{}).Error },
	)
	const callers = 8
	results := make(chan generationapp.PreparationResult, callers)
	errorsFound := make(chan error, callers)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, prepareErr := preparations.PrepareImageGeneration(ctx, main.editor, command)
			if prepareErr != nil {
				errorsFound <- prepareErr
				return
			}
			results <- result
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsFound)
	for prepareErr := range errorsFound {
		t.Fatalf("prepare identical Generation intent concurrently: %T %v", prepareErr, prepareErr)
	}
	var prepared generationapp.PreparationResult
	for result := range results {
		if prepared.Intent.ID == "" {
			prepared = result
		}
		if result.Intent.ID != prepared.Intent.ID || result.Receipt.ID != prepared.Receipt.ID ||
			result.Intent.Status != generationdomain.IntentPrepared ||
			result.Intent.CostEstimateID != prepared.CostEstimate.ID ||
			result.Intent.CostReservationID != prepared.CostReservation.ID ||
			result.Intent.QuotaReservationID != prepared.QuotaReservation.ID ||
			result.CostReservation.Status != costdomain.ReservationReserved ||
			result.QuotaReservation.Status != quotadomain.ReservationReserved {
			t.Fatalf("concurrent Generation preparation drifted: %#v", result)
		}
	}
	assertPreparedFactCounts(t, countRecords, main, 1, 1, 1, 1, 1)

	replayed, err := preparations.PrepareImageGeneration(ctx, main.editor, command)
	if err != nil || replayed.Intent.ID != prepared.Intent.ID || replayed.Receipt.ID != prepared.Receipt.ID {
		t.Fatalf("replay Generation preparation: result=%#v err=%v", replayed, err)
	}
	redeliveredCommand := command
	redeliveredCommand.IdempotencyKey = "generation-prepare-main-redelivery"
	redelivered, err := preparations.PrepareImageGeneration(ctx, main.editor, redeliveredCommand)
	if err != nil || redelivered.Intent.ID != prepared.Intent.ID || redelivered.Receipt.ID == prepared.Receipt.ID ||
		redelivered.Intent.CostEstimateReceiptID != prepared.Intent.CostEstimateReceiptID ||
		redelivered.Intent.CostReservationReceiptID != prepared.Intent.CostReservationReceiptID ||
		redelivered.Intent.QuotaReservationReceiptID != prepared.Intent.QuotaReservationReceiptID {
		t.Fatalf("redeliver Generation preparation by source: result=%#v err=%v", redelivered, err)
	}
	if prepared.Intent.EstimatedUnits != 4 {
		t.Fatalf("Generation preparation units = %d, want target-owned 4", prepared.Intent.EstimatedUnits)
	}
	drifted := command
	drifted.TargetHash = strings.Repeat("b", 64)
	if _, err = preparations.PrepareImageGeneration(ctx, main.editor, drifted); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Generation preparation accepted idempotent Target Hash drift: %T %v", err, err)
	}
	forgedSource := command
	forgedSource.NodeRunID, forgedSource.IdempotencyKey = uuid.NewString(), "generation-prepare-forged-node"
	if _, err = preparations.PrepareImageGeneration(ctx, main.editor, forgedSource); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Generation preparation accepted a non-Workflow Node source: %T %v", err, err)
	}

	view, err := preparations.GetIntent(ctx, main.owner, prepared.Intent.ID)
	if err != nil || view.Intent.ID != prepared.Intent.ID || view.CostReservation.ID != prepared.CostReservation.ID ||
		view.QuotaReservation.ID != prepared.QuotaReservation.ID {
		t.Fatalf("read prepared Generation intent: view=%#v err=%v", view, err)
	}

	cancelPreparation := prepareDistinctIntent(t, ctx, preparations, main, "cancel", strings.Repeat("c", 64))
	cancelled, err := preparations.CancelPreparedIntent(ctx, main.editor, generationapp.CancelPreparedIntentCommand{
		IntentID: cancelPreparation.Intent.ID, IdempotencyKey: "generation-cancel-main",
	})
	if err != nil || cancelled.Intent.Status != generationdomain.IntentCancelled ||
		cancelled.CostReservation.Status != costdomain.ReservationReleased ||
		cancelled.QuotaReservation.Status != quotadomain.ReservationReleased {
		t.Fatalf("cancel prepared Generation intent: result=%#v err=%v", cancelled, err)
	}
	replayedCancel, err := preparations.CancelPreparedIntent(ctx, main.editor, generationapp.CancelPreparedIntentCommand{
		IntentID: cancelPreparation.Intent.ID, IdempotencyKey: "generation-cancel-main",
	})
	if err != nil || replayedCancel.Receipt.ID != cancelled.Receipt.ID {
		t.Fatalf("replay Generation cancellation: result=%#v err=%v", replayedCancel, err)
	}

	claimPreparation := prepareDistinctIntent(t, ctx, preparations, main, "claim", strings.Repeat("d", 64))
	claimCommand := generationapp.AcquireExecutionClaimCommand{
		IntentID: claimPreparation.Intent.ID, Claimant: "model-gateway:test-worker", IdempotencyKey: "generation-claim-main",
	}
	claimed, err := preparations.AcquireExecutionClaim(ctx, claimCommand)
	if err != nil || claimed.Intent.Status != generationdomain.IntentClaimed || claimed.Intent.ClaimFencingVersion != 1 ||
		claimed.Authorization.IntentID != claimed.Intent.ID || claimed.Authorization.ClaimToken == "" ||
		!claimed.Authorization.ExpiresAt.Equal(now.Add(preparationClaimTTL)) {
		t.Fatalf("claim prepared Generation intent: result=%#v err=%v", claimed, err)
	}
	if err = preparations.VerifyExecutionAuthorization(ctx, claimed.Authorization); err != nil {
		t.Fatalf("verify current Generation authorization: %v", err)
	}
	replayedClaim, err := preparations.AcquireExecutionClaim(ctx, claimCommand)
	if err != nil || replayedClaim.Receipt.ID != claimed.Receipt.ID ||
		replayedClaim.Authorization.ClaimToken != claimed.Authorization.ClaimToken {
		t.Fatalf("replay Generation claim: result=%#v err=%v", replayedClaim, err)
	}
	otherClaim := claimCommand
	otherClaim.Claimant, otherClaim.IdempotencyKey = "model-gateway:other-worker", "generation-claim-other-worker"
	if _, err = preparations.AcquireExecutionClaim(ctx, otherClaim); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("active claim was replaced by another claimant: %T %v", err, err)
	}
	if _, err = preparations.CancelPreparedIntent(ctx, main.editor, generationapp.CancelPreparedIntentCommand{
		IntentID: claimed.Intent.ID, IdempotencyKey: "generation-cancel-claimed",
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("claimed Generation intent was released as unexecuted: %T %v", err, err)
	}

	if err = database.Model(&model.UserAccount{}).Where("id = ?", main.editorID).Update("token_version", 2).Error; err != nil {
		t.Fatalf("revoke Generation initiator: %v", err)
	}
	if err = preparations.VerifyExecutionAuthorization(ctx, claimed.Authorization); generationErrorCode(err) != "unauthenticated" {
		t.Fatalf("revoked initiator retained Generation authorization: %T %v", err, err)
	}
	if err = database.Model(&model.UserAccount{}).Where("id = ?", main.editorID).Update("token_version", 1).Error; err != nil {
		t.Fatalf("restore Generation initiator: %v", err)
	}

	racePreparation := prepareDistinctIntent(t, ctx, preparations, main, "claim-cancel-race", strings.Repeat("e", 64))
	raceStart := make(chan struct{})
	claimErrors, cancelErrors := make(chan error, 1), make(chan error, 1)
	var raceClaim generationapp.ExecutionClaimResult
	var raceCancel generationapp.CancellationResult
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-raceStart
		var claimErr error
		raceClaim, claimErr = preparations.AcquireExecutionClaim(ctx, generationapp.AcquireExecutionClaimCommand{
			IntentID: racePreparation.Intent.ID, Claimant: "model-gateway:race-worker", IdempotencyKey: "generation-race-claim",
		})
		claimErrors <- claimErr
	}()
	go func() {
		defer workers.Done()
		<-raceStart
		var cancelErr error
		raceCancel, cancelErr = preparations.CancelPreparedIntent(ctx, main.editor, generationapp.CancelPreparedIntentCommand{
			IntentID: racePreparation.Intent.ID, IdempotencyKey: "generation-race-cancel",
		})
		cancelErrors <- cancelErr
	}()
	close(raceStart)
	workers.Wait()
	claimErr, cancelErr := <-claimErrors, <-cancelErrors
	if (claimErr == nil) == (cancelErr == nil) {
		t.Fatalf("claim/cancel race did not choose exactly one terminal: claim=%v cancel=%v", claimErr, cancelErr)
	}
	if claimErr == nil {
		if raceClaim.Intent.Status != generationdomain.IntentClaimed || generationErrorCode(cancelErr) != "state_conflict" {
			t.Fatalf("claim race winner facts drifted: claim=%#v cancel=%v", raceClaim, cancelErr)
		}
	} else if raceCancel.Intent.Status != generationdomain.IntentCancelled || generationErrorCode(claimErr) != "state_conflict" {
		t.Fatalf("cancel race winner facts drifted: cancel=%#v claim=%v", raceCancel, claimErr)
	}

	currentTime = now.Add(preparationClaimTTL)
	if err = preparations.VerifyExecutionAuthorization(ctx, claimed.Authorization); generationErrorCode(err) != "execution_authorization_expired" {
		t.Fatalf("expired Generation authorization remained valid: %T %v", err, err)
	}

	currentTime = now
	if _, err = costs.ReleaseReservation(ctx, costapp.Actor{UserID: main.editor.UserID, TokenVersion: 1}, costapp.ReleaseReservationCommand{
		ReservationID: claimed.Intent.CostReservationID, IdempotencyKey: "generation-injected-cost-release",
	}); err != nil {
		t.Fatalf("inject terminal Cost reservation drift: %v", err)
	}
	if err = preparations.VerifyExecutionAuthorization(ctx, claimed.Authorization); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Generation authorization ignored Cost reservation drift: %T %v", err, err)
	}
	var preparingCount int64
	if err = database.Model(&model.GenerationIntent{}).Where("status = ?", generationdomain.IntentPreparing).
		Count(&preparingCount).Error; err != nil {
		t.Fatalf("count committed PREPARING intents: %v", err)
	}
	if preparingCount != 0 {
		t.Fatalf("Generation committed %d transaction-local PREPARING intents", preparingCount)
	}
}

func assertPreparationSchema(t *testing.T, hasConstraint, hasIndex func(string) bool) {
	t.Helper()
	for _, constraint := range []string{
		"ck_gen_intent_state", "ck_gen_intent_claim_fields", "fk_gen_intents_workflow_run",
		"fk_gen_intents_node_run", "fk_gen_intents_cost_estimate", "fk_gen_intents_cost_reservation",
		"fk_gen_intents_quota_reservation", "fk_gen_intents_cost_estimate_receipt",
		"fk_gen_intents_cost_reservation_receipt", "fk_gen_intents_quota_reservation_receipt",
	} {
		if !hasConstraint(constraint) {
			t.Fatalf("Generation intent schema is missing constraint %s", constraint)
		}
	}
	for _, index := range []string{"uq_gen_intent_node_run", "idx_gen_intents_claim_token"} {
		if !hasIndex(index) {
			t.Fatalf("Generation intent schema is missing index %s", index)
		}
	}
}

func assertGenerationTargetSchema(t *testing.T, hasConstraint, hasIndex func(string) bool) {
	t.Helper()
	for _, constraint := range []string{
		"ck_gen_target_kind", "ck_gen_target_source_hash", "ck_gen_target_policy_hash",
		"ck_gen_target_hash", "ck_gen_target_revision",
	} {
		if !hasConstraint(constraint) {
			t.Fatalf("GenerationTarget schema is missing constraint %s", constraint)
		}
	}
	if !hasIndex("ix_gen_targets_workspace_hash") {
		t.Fatal("GenerationTarget schema is missing workspace/hash index")
	}
}

func assertGenerationTargetPersistence(
	t *testing.T,
	ctx context.Context,
	store *generationgorm.TargetStore,
	targetID, targetHash string,
	updateTarget, deleteTarget func(string) error,
) {
	t.Helper()
	target, err := store.Find(ctx, targetID)
	if err != nil || target.TargetHash != targetHash || generationdomain.ValidateGenerationTarget(target) != nil {
		t.Fatalf("load canonical GenerationTarget: target=%#v err=%v", target, err)
	}
	replayed, err := store.Ensure(ctx, target)
	if err != nil || !generationdomain.SameGenerationTarget(replayed, target) {
		t.Fatalf("replay canonical GenerationTarget: target=%#v err=%v", replayed, err)
	}
	payload := *target.ReferenceAsset
	payload.PositivePrompt = "drifted character reference sheet"
	drifted, err := generationdomain.NewGenerationTarget(generationdomain.GenerationTargetInput{
		ID: target.ID, WorkspaceID: target.WorkspaceID, ProjectID: target.ProjectID, Kind: target.Kind,
		SourceOwnerRef: target.SourceOwnerRef, PolicySnapshotRef: target.PolicySnapshotRef,
		ReferenceAsset: &payload, Revision: target.Revision, CreatedBy: target.CreatedBy, CreatedAt: target.CreatedAt,
	})
	if err != nil {
		t.Fatalf("build drifted GenerationTarget fixture: %v", err)
	}
	if _, err = store.Ensure(ctx, drifted); err == nil {
		t.Fatal("GenerationTarget identity accepted different immutable facts")
	}
	if err = updateTarget(target.ID); !errors.Is(err, model.ErrImmutableGenerationTarget) {
		t.Fatalf("GenerationTarget update was not blocked: %v", err)
	}
	if err = deleteTarget(target.ID); !errors.Is(err, model.ErrImmutableGenerationTarget) {
		t.Fatalf("GenerationTarget delete was not blocked: %v", err)
	}
}

func seedPreparationFixture(
	t *testing.T,
	create func(any) error,
	targets *generationgorm.TargetStore,
	now time.Time,
	suffix string,
) preparationFixture {
	t.Helper()
	fixture := preparationFixture{
		workspaceID: uuid.New(), projectID: uuid.New(), ownerID: uuid.New(), editorID: uuid.New(),
		create: create, targets: targets, now: now,
	}
	users := []model.UserAccount{
		{ID: fixture.ownerID, EmailNormalized: "generation-preparation-owner-" + suffix + "-" + fixture.ownerID.String() + "@example.test", PasswordHash: "test", TokenVersion: 1, DisplayName: "Owner", Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: fixture.editorID, EmailNormalized: "generation-preparation-editor-" + suffix + "-" + fixture.editorID.String() + "@example.test", PasswordHash: "test", TokenVersion: 1, DisplayName: "Editor", Status: "active", CreatedAt: now, UpdatedAt: now},
	}
	if err := create(&users); err != nil {
		t.Fatalf("seed Generation preparation users: %v", err)
	}
	if err := create(&model.Workspace{ID: fixture.workspaceID, Name: "Generation " + suffix, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed Generation preparation workspace: %v", err)
	}
	memberships := []model.Membership{
		{ID: uuid.New(), WorkspaceID: fixture.workspaceID, UserID: fixture.ownerID, Role: "owner", Status: "active", JoinedAt: now},
		{ID: uuid.New(), WorkspaceID: fixture.workspaceID, UserID: fixture.editorID, Role: "editor", Status: "active", JoinedAt: now},
	}
	if err := create(&memberships); err != nil {
		t.Fatalf("seed Generation preparation memberships: %v", err)
	}
	if err := create(&model.Project{
		ID: fixture.projectID, WorkspaceID: fixture.workspaceID, Name: "Generation " + suffix,
		AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60000,
		Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed Generation preparation project: %v", err)
	}
	fixture.owner = generationapp.Actor{UserID: fixture.ownerID.String(), TokenVersion: 1}
	fixture.editor = generationapp.Actor{UserID: fixture.editorID.String(), TokenVersion: 1}
	fixture.workflowRunID, fixture.workflowDefinitionID, fixture.runInputSnapshotID =
		seedPreparationWorkflowRun(t, create, fixture, now, suffix)
	return fixture
}

func seedPreparationWorkflowRun(
	t *testing.T,
	create func(any) error,
	fixture preparationFixture,
	now time.Time,
	suffix string,
) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	catalogID, draftID, revisionID := uuid.New(), uuid.New(), uuid.New()
	definitionID, snapshotID, runID := uuid.New(), uuid.New(), uuid.New()
	hash := strings.Repeat("1", 64)
	records := []any{
		&model.NodeCatalogVersion{
			ID: catalogID, Key: "generation-preparation-" + suffix + "-" + catalogID.String(), Version: "1.0.0",
			Definitions: []byte(`[]`), ContentHash: hash, ExecutionHash: hash, Status: "published", CreatedAt: now,
		},
		&model.AuthoringDraft{
			ID: draftID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID, AuthoringMode: "CANVAS",
			Graph: []byte(`{"nodes":[],"edges":[]}`), Layout: []byte(`{}`), FrozenInputs: []byte(`[]`),
			NodeCatalogVersionID: catalogID, Status: "active", Revision: 1, CreatedBy: fixture.editorID,
			CreatedAt: now, UpdatedAt: now,
		},
		&model.AuthoringRevision{
			ID: revisionID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID, DraftID: draftID,
			RevisionNo: 1, AuthoringMode: "CANVAS", Graph: []byte(`{"nodes":[],"edges":[]}`),
			Layout: []byte(`{}`), FrozenInputs: []byte(`[]`), NodeCatalogVersionID: catalogID,
			CatalogContentHash: hash, CatalogExecutionHash: hash, ExecutionHash: hash, ContentHash: hash,
			CreatedBy: fixture.editorID, CreatedAt: now,
		},
		&model.WorkflowDefinitionVersion{
			ID: definitionID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
			AuthoringRevisionID: revisionID, NodeCatalogVersionID: catalogID, CompilerVersion: "generation-preparation",
			WorkflowType: "generation-preparation", WorkflowTypeVersion: "1.0.0", RuntimeContractVersion: "1.0.0",
			Definition: []byte(`{"nodes":[]}`), ContentHash: hash, CreatedBy: fixture.editorID, CreatedAt: now,
		},
		&model.RunInputSnapshot{
			ID: snapshotID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
			WorkflowDefinitionVersionID: definitionID, AuthoringRevisionID: revisionID,
			Snapshot: []byte(`{"frozen_inputs":[]}`), ContentHash: hash, CreatedBy: fixture.editorID, CreatedAt: now,
		},
		&model.WorkflowRun{
			ID: runID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
			AuthoringRevisionID: revisionID, WorkflowDefinitionVersionID: definitionID, RunInputSnapshotID: snapshotID,
			TemporalWorkflowID: "lanverse:generation-preparation:" + runID.String(), StartInputHash: hash,
			Status: "RUNNING", ProgressStage: "executing", Revision: 1, CreatedBy: fixture.editorID,
			InitiatorTokenVersion: 1, CreatedAt: now, UpdatedAt: now,
		},
	}
	for _, record := range records {
		if err := create(record); err != nil {
			t.Fatalf("seed Generation Workflow source %T: %v", record, err)
		}
	}
	return runID, definitionID, snapshotID
}

func configurePreparationLimits(
	t *testing.T,
	ctx context.Context,
	costs *costapp.Service,
	quotas *quotaapp.Service,
	fixture preparationFixture,
	budget, unitPrice string,
	quota int64,
) {
	t.Helper()
	costActor := costapp.Actor{UserID: fixture.owner.UserID, TokenVersion: fixture.owner.TokenVersion}
	if _, err := costs.SetBudget(ctx, costActor, costapp.SetBudgetCommand{
		ProjectID: fixture.projectID.String(), LimitAmount: budget, Currency: "USD",
		ExpectedRevision: 0, IdempotencyKey: "generation-preparation-budget-" + fixture.projectID.String(),
	}); err != nil {
		t.Fatalf("configure Generation preparation budget: %v", err)
	}
	if _, err := costs.SetPriceQuote(ctx, costActor, costapp.SetPriceQuoteCommand{
		ProjectID: fixture.projectID.String(), ModelProfileVersionID: fixture.provider.Profile.ID,
		ReservationUnitAmount: unitPrice, Currency: "USD", ExpectedRevision: 0,
		IdempotencyKey: "generation-preparation-price-" + fixture.projectID.String(),
	}); err != nil {
		t.Fatalf("configure Generation preparation price: %v", err)
	}
	if _, err := quotas.SetDailyPolicy(ctx, quotaapp.Actor{UserID: fixture.owner.UserID, TokenVersion: fixture.owner.TokenVersion}, quotaapp.SetDailyPolicyCommand{
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		Metric: quotadomain.MetricGenerationImageCall, LimitUnits: quota, ExpectedRevision: 0,
		IdempotencyKey: "generation-preparation-quota-" + fixture.projectID.String(),
	}); err != nil {
		t.Fatalf("configure Generation preparation quota: %v", err)
	}
}

func newPreparationCommand(
	t *testing.T,
	fixture preparationFixture,
	suffix, frozenHash string,
) generationapp.PrepareImageGenerationCommand {
	t.Helper()
	target, err := generationdomain.NewGenerationTarget(generationdomain.GenerationTargetInput{
		ID: uuid.NewString(), WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		Kind: generationdomain.GenerationTargetReferenceAsset,
		SourceOwnerRef: generationdomain.FrozenOwnerReference{
			Owner: "storyboard", Resource: "approved_storyboard_intents", ID: uuid.NewString(),
			Revision: 1, ContentHash: frozenHash,
		},
		PolicySnapshotRef: generationdomain.FrozenOwnerReference{
			Owner: "preset", Resource: "effective_style_snapshot", ID: uuid.NewString(),
			Revision: 1, ContentHash: strings.Repeat("9", 64),
		},
		ReferenceAsset: &generationdomain.ReferenceAssetTarget{
			AssetID: uuid.NewString(), AssetKind: "character",
			SpecificationVersionRef: generationdomain.FrozenOwnerReference{
				Owner: "production", Resource: "production_bible_specification_version", ID: uuid.NewString(),
				Revision: 1, ContentHash: strings.Repeat("8", 64),
			},
			AssetStateRef: generationdomain.FrozenOwnerReference{
				Owner: "asset", Resource: "asset_state", ID: uuid.NewString(),
				Revision: 1, ContentHash: strings.Repeat("7", 64),
			},
			OutputKind: "reference_sheet", RequiredViewRoles: []string{"back", "front", "profile"},
			PromptVersion: "character-reference-sheet", PositivePrompt: "character reference sheet",
			NegativePrompt: "identity drift", Width: 1536, Height: 1024, NumberResults: 4, OutputFormat: "png",
		},
		Revision: 1, CreatedBy: fixture.editorID.String(), CreatedAt: fixture.now,
	})
	if err != nil {
		t.Fatalf("build frozen GenerationTarget: %v", err)
	}
	target, err = fixture.targets.Ensure(context.Background(), target)
	if err != nil {
		t.Fatalf("persist frozen GenerationTarget: %v", err)
	}
	_, input, inputHash, err := workflowdomain.BuildNodeInput(workflowdomain.NodeInputSnapshot{
		SchemaVersion: workflowdomain.NodeInputSchemaVersion,
		Config:        json.RawMessage(`{"generation":"image","variant":"` + suffix + `"}`),
		FrozenInputs: []authoringdomain.FrozenReference{{
			Kind: "generation_target", ID: target.ID, Version: "1", Hash: target.TargetHash,
		}},
	})
	if err != nil {
		t.Fatalf("build frozen Generation Workflow input: %v", err)
	}
	nodeRunID, claimToken := uuid.New(), uuid.New()
	if err = fixture.create(&model.NodeRunProjection{
		ID: nodeRunID, WorkspaceID: fixture.workspaceID, WorkflowRunID: fixture.workflowRunID,
		NodeID: "image-" + suffix, DefinitionKey: "agent.image_generation", DefinitionVersion: "1.0.0",
		Executor: "activity.image_generation", RiskLevel: "external_ai", Status: "RUNNING", Attempt: 1,
		ActiveClaimToken: &claimToken, Input: []byte(input), InputHash: &inputHash,
		Revision: 2, CreatedAt: fixture.now, UpdatedAt: fixture.now,
	}); err != nil {
		t.Fatalf("seed frozen Generation Workflow node: %v", err)
	}
	return generationapp.PrepareImageGenerationCommand{
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		WorkflowRunID: fixture.workflowRunID.String(), NodeRunID: nodeRunID.String(), WorkflowInputHash: inputHash,
		TargetID: target.ID, TargetHash: target.TargetHash, IdempotencyKey: "generation-prepare-" + suffix,
	}
}

func prepareDistinctIntent(
	t *testing.T,
	ctx context.Context,
	service *generationapp.PreparationService,
	fixture preparationFixture,
	suffix, inputHash string,
) generationapp.PreparationResult {
	t.Helper()
	result, err := service.PrepareImageGeneration(ctx, fixture.editor, newPreparationCommand(t, fixture, suffix, inputHash))
	if err != nil {
		t.Fatalf("prepare distinct Generation intent %s: %v", suffix, err)
	}
	return result
}

func assertNoPreparedFacts(t *testing.T, countRecords countPreparationRecords, fixture preparationFixture) {
	t.Helper()
	assertPreparedFactCounts(t, countRecords, fixture, 0, 0, 0, 0, 0)
	operations := []string{
		"generation.intent.prepare", "cost.estimate.create", "cost.reservation.reserve", "quota.reservation.reserve",
	}
	for _, operation := range operations {
		count, err := countRecords(&model.CommandReceipt{}, "workspace_id = ? AND operation = ?", fixture.workspaceID, operation)
		if err != nil {
			t.Fatalf("count rolled-back %s receipts: %v", operation, err)
		}
		if count != 0 {
			t.Fatalf("atomic preparation failure kept %d %s receipts", count, operation)
		}
	}
}

func assertPreparedFactCounts(
	t *testing.T,
	countRecords countPreparationRecords,
	fixture preparationFixture,
	intents, estimates, costReservations, ledgers, quotaReservations int64,
) {
	t.Helper()
	checks := []struct {
		model any
		want  int64
		name  string
	}{
		{&model.GenerationIntent{}, intents, "Generation intents"},
		{&model.CostEstimate{}, estimates, "Cost estimates"},
		{&model.CostReservation{}, costReservations, "Cost reservations"},
		{&model.CostLedgerEntry{}, ledgers, "Cost ledger entries"},
		{&model.QuotaReservation{}, quotaReservations, "Quota reservations"},
	}
	for _, check := range checks {
		count, err := countRecords(check.model, "workspace_id = ? AND project_id = ?", fixture.workspaceID, fixture.projectID)
		if err != nil {
			t.Fatalf("count %s: %v", check.name, err)
		}
		if count != check.want {
			t.Fatalf("%s count = %d, want %d", check.name, count, check.want)
		}
	}
}

func generationErrorCode(err error) string {
	var typed *generationapp.Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return ""
}
