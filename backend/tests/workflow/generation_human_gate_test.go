package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"

	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationreview "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/review"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	reviewdomain "github.com/StephenQiu30/lanverse/backend/internal/review/domain"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgeneration "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/generation"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowreview "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/review"
	workflowtemporal "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type generationReviewRepository struct{ task reviewdomain.HumanTask }

func (repo *generationReviewRepository) FindTaskByNode(context.Context, string, string) (reviewdomain.HumanTask, error) {
	return reviewdomain.HumanTask{}, reviewapp.ErrNotFound
}

func (repo *generationReviewRepository) GetDecision(context.Context, reviewapp.Actor, string) (reviewdomain.DecisionResult, error) {
	return reviewdomain.DecisionResult{}, reviewapp.ErrNotFound
}

func (repo *generationReviewRepository) EnsureTask(_ context.Context, task reviewdomain.HumanTask) (reviewdomain.HumanTask, error) {
	repo.task = task
	return task, nil
}

func (repo *generationReviewRepository) Claim(context.Context, reviewapp.Actor, reviewapp.ClaimCommand, string, time.Time, time.Time) (reviewdomain.ClaimResult, error) {
	return reviewdomain.ClaimResult{}, errors.New("not used")
}

func (repo *generationReviewRepository) Renew(context.Context, reviewapp.Actor, reviewapp.RenewCommand, time.Time, time.Time) (reviewdomain.ClaimResult, error) {
	return reviewdomain.ClaimResult{}, errors.New("not used")
}

func (repo *generationReviewRepository) Release(context.Context, reviewapp.Actor, reviewapp.ReleaseCommand, time.Time) (reviewdomain.HumanTask, error) {
	return reviewdomain.HumanTask{}, errors.New("not used")
}

func (repo *generationReviewRepository) ExpireClaims(context.Context, int, time.Time) (int, error) {
	return 0, errors.New("not used")
}

func (repo *generationReviewRepository) Decide(context.Context, reviewapp.Actor, reviewapp.DecideCommand, reviewdomain.ReviewDecision, time.Time) (reviewdomain.DecisionResult, error) {
	return reviewdomain.DecisionResult{}, errors.New("not used")
}

type generationCandidateSetSource struct {
	set   generationdomain.CandidateSet
	actor generationapp.Actor
	id    string
}

type generationCandidateSetExecutor struct{ set generationdomain.CandidateSet }

func (executor *generationCandidateSetExecutor) Execute(
	_ context.Context,
	command workflow.NodeExecutorCommand,
) (workflow.NodeExecutorResult, error) {
	if command.Executor != "test.generation_candidate_set" || len(command.OutputPorts) != 1 ||
		command.OutputPorts[0].Key != "candidates" || command.OutputPorts[0].ValueType != "generation_candidate_set" {
		return workflow.NodeExecutorResult{}, errors.New("unexpected Generation CandidateSet source contract")
	}
	output, _, _, err := workflow.BuildNodeOutput(workflow.NodeOutputSnapshot{
		SchemaVersion: workflow.NodeOutputSchemaVersion,
		Bindings: []workflow.NodeOutputBinding{{
			Port: "candidates", ValueType: "generation_candidate_set", ReferenceID: executor.set.ID,
			ReferenceVersion: "1", ContentHash: executor.set.ContentHash,
		}},
	})
	return workflow.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, err
}

type generationCandidateReadiness struct {
	bundles map[string]generationdomain.CandidateWithReport
}

type loseFirstGenerationSignal struct {
	delegate workflowapp.WorkflowSignaler
	mu       sync.Mutex
	lost     bool
	requests []workflow.SignalRequest
}

func (signaler *loseFirstGenerationSignal) Signal(
	ctx context.Context,
	request workflow.SignalRequest,
) (workflow.SignalObservation, error) {
	signaler.mu.Lock()
	signaler.requests = append(signaler.requests, request)
	lose := !signaler.lost
	if lose {
		signaler.lost = true
	}
	signaler.mu.Unlock()
	observation, err := signaler.delegate.Signal(ctx, request)
	if err != nil {
		return observation, err
	}
	if lose {
		return workflow.SignalObservation{Outcome: workflow.SignalOutcomeUnknown}, errors.New("injected lost Temporal Signal result")
	}
	return observation, nil
}

func (signaler *loseFirstGenerationSignal) Requests() []workflow.SignalRequest {
	signaler.mu.Lock()
	defer signaler.mu.Unlock()
	return append([]workflow.SignalRequest(nil), signaler.requests...)
}

func (readiness generationCandidateReadiness) RequireQCPassed(
	_ context.Context,
	_ generationapp.Actor,
	candidateID string,
) (generationdomain.CandidateWithReport, error) {
	bundle, found := readiness.bundles[candidateID]
	if !found {
		return generationdomain.CandidateWithReport{}, generationapp.ErrNotFound
	}
	return bundle, nil
}

func (source *generationCandidateSetSource) RequireCandidateSet(
	_ context.Context,
	actor generationapp.Actor,
	id string,
) (generationdomain.CandidateSet, error) {
	source.actor, source.id = actor, id
	return source.set, nil
}

type selectedGenerationCandidateOwner struct {
	result  generationapp.ApplySelectionResult
	actor   generationapp.Actor
	command generationapp.ApplySelectionCommand
}

func (owner *selectedGenerationCandidateOwner) ApplySelection(
	_ context.Context,
	actor generationapp.Actor,
	command generationapp.ApplySelectionCommand,
) (generationapp.ApplySelectionResult, error) {
	owner.actor, owner.command = actor, command
	return owner.result, nil
}

func TestGenerationHumanGateAppliesFrozenCandidateSetThroughSelectionOwner(t *testing.T) {
	const (
		workspaceID = "00000000-0000-0000-0000-000000000101"
		projectID   = "00000000-0000-0000-0000-000000000102"
		runID       = "00000000-0000-0000-0000-000000000103"
		nodeRunID   = "00000000-0000-0000-0000-000000000104"
		taskID      = "00000000-0000-0000-0000-000000000105"
		decisionID  = "00000000-0000-0000-0000-000000000106"
		setID       = "00000000-0000-0000-0000-000000000107"
		selectionID = "00000000-0000-0000-0000-000000000108"
		candidateID = "00000000-0000-0000-0000-000000000109"
		userID      = "00000000-0000-0000-0000-000000000110"
		receiptID   = "00000000-0000-0000-0000-000000000111"
	)
	setHash := strings.Repeat("a", 64)
	selectionHash := strings.Repeat("b", 64)
	owner := &selectedGenerationCandidateOwner{result: generationapp.ApplySelectionResult{
		Selection: generationdomain.CandidateSelection{
			ID: selectionID, WorkspaceID: workspaceID, ProjectID: projectID,
			WorkflowRunID: runID, NodeRunID: nodeRunID, HumanTaskID: taskID,
			ReviewDecisionID: decisionID, SubjectType: "generation_candidate_selection",
			SubjectID: nodeRunID, SubjectRevision: 2, CandidateSetHash: setHash,
			SelectedCandidateID: candidateID, ContentHash: selectionHash, Revision: 1, CreatedBy: userID,
		},
		Receipt: platformcommand.Receipt{
			ID: receiptID, WorkspaceID: workspaceID, Operation: "generation.candidate.select",
			ResourceID: selectionID, CreatedBy: userID,
		},
	}}
	applier := workflowgeneration.NewHumanGateApplier(owner)
	result, err := applier.ApplyHumanGateDecision(context.Background(), workflowapp.Actor{
		UserID: userID, TokenVersion: 1,
	}, workflow.HumanGateOwnerApplication{
		WorkspaceID: workspaceID, ProjectID: projectID, WorkflowRunID: runID, NodeRunID: nodeRunID,
		HumanTaskID: taskID, ReviewDecisionID: decisionID, SubjectRevision: 2,
		Decision: "selected", Executor: "gate.generation_image_review",
		Candidate: workflow.NodeInputBinding{
			Port: "candidates", ValueType: "generation_candidate_set", SourceKind: workflow.NodeInputSourceNodeOutput,
			ReferenceID: setID, ReferenceVersion: "1", ContentHash: setHash,
		},
		OutputPort: "selection", OutputValueType: "generation_candidate_selection",
	})
	if err != nil || result.ReceiptID != receiptID || result.Operation != "generation.candidate.select" ||
		result.OutputHash == "" || len(result.Output.Bindings) != 1 ||
		result.Output.Bindings[0].ReferenceID != selectionID || result.Output.Bindings[0].ContentHash != selectionHash {
		t.Fatalf("apply Generation selection through Workflow Human Gate: result=%#v err=%v", result, err)
	}
	if owner.actor.UserID != userID || owner.actor.TokenVersion != 1 || owner.command.ReviewDecisionID != decisionID ||
		owner.command.IdempotencyKey != "workflow-review:"+decisionID {
		t.Fatalf("Generation selection owner command drifted: actor=%#v command=%#v", owner.actor, owner.command)
	}
}

func TestGenerationHumanGateOpenerExpandsAuthoritativeCandidateSet(t *testing.T) {
	const (
		workspaceID = "00000000-0000-0000-0000-000000000201"
		projectID   = "00000000-0000-0000-0000-000000000202"
		runID       = "00000000-0000-0000-0000-000000000203"
		nodeRunID   = "00000000-0000-0000-0000-000000000204"
		setID       = "00000000-0000-0000-0000-000000000205"
		candidateA  = "00000000-0000-0000-0000-000000000206"
		candidateB  = "00000000-0000-0000-0000-000000000207"
		userID      = "00000000-0000-0000-0000-000000000208"
	)
	setHash := strings.Repeat("c", 64)
	repository := &generationReviewRepository{}
	reviews := reviewapp.NewService(repository, reviewapp.Config{
		Now:   func() time.Time { return time.Date(2026, time.August, 27, 1, 0, 0, 0, time.UTC) },
		NewID: func() string { return "00000000-0000-0000-0000-000000000209" },
	})
	sets := &generationCandidateSetSource{set: generationdomain.CandidateSet{
		ID: setID, WorkspaceID: workspaceID, ProjectID: projectID, ProviderReceiptID: "00000000-0000-0000-0000-000000000210",
		Candidates: []generationdomain.CandidateReference{{ID: candidateB}, {ID: candidateA}}, ContentHash: setHash, Revision: 1,
	}}
	opener := workflowreview.NewWithGeneration(reviews, sets)
	err := opener.OpenHumanTask(context.Background(), workflow.HumanGateBinding{
		WorkspaceID: workspaceID, ProjectID: projectID, WorkflowRunID: runID, NodeRunID: nodeRunID,
		Executor: "gate.generation_image_review", InitiatorUserID: userID, InitiatorTokenVersion: 1,
		SubjectType: "workflow_node_output", SubjectID: nodeRunID, SubjectRevision: 2,
		CandidateSet: workflow.NodeInputBinding{
			Port: "candidates", ValueType: "generation_candidate_set", SourceKind: workflow.NodeInputSourceNodeOutput,
			ReferenceID: setID, ReferenceVersion: "1", ContentHash: setHash,
		},
		RubricVersion: "gate.generation_image_review@1.0.0",
	})
	if err != nil {
		t.Fatalf("open Generation CandidateSet HumanTask: %v", err)
	}
	if repository.task.SubjectType != "generation_candidate_selection" || repository.task.SubjectID != nodeRunID ||
		repository.task.SubjectRevision != 2 || !slices.Equal(repository.task.CandidateIDs, []string{candidateA, candidateB}) {
		t.Fatalf("Generation HumanTask did not freeze expanded candidates: %#v", repository.task)
	}
	if sets.id != setID || sets.actor.UserID != userID || sets.actor.TokenVersion != 1 {
		t.Fatalf("Generation CandidateSet source call drifted: id=%s actor=%#v", sets.id, sets.actor)
	}
}

func TestGenerationCandidateSetSelectionPersistsThroughWorkflowSignal(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	temporalAddress := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if databaseURL == "" || temporalAddress == "" {
		t.Skip("set PostgreSQL and Temporal test variables to run the Generation Human Gate journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open Generation Human Gate database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize Generation Human Gate GORM catalog: %v", err)
	}
	now := time.Date(2026, time.August, 27, 2, 0, 0, 0, time.UTC)
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)

	setID, receiptID := uuid.New(), uuid.New()
	candidateIDs := []uuid.UUID{uuid.New(), uuid.New()}
	slices.SortFunc(candidateIDs, func(left, right uuid.UUID) int { return strings.Compare(left.String(), right.String()) })
	references := make([]generationdomain.CandidateReference, len(candidateIDs))
	bundles := make(map[string]generationdomain.CandidateWithReport, len(candidateIDs))
	for index, candidateID := range candidateIDs {
		artifactID, reportID := uuid.New(), uuid.New()
		artifactHash, reportHash := strings.Repeat(string(rune('d'+index)), 64), strings.Repeat(string(rune('f'+index)), 64)
		width, height := 4+index, 3+index
		if err = database.Create(&model.Artifact{
			ID: artifactID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
			SourceType: "generation_provider_job", SourceID: setID, OutputKey: "image-" + string(rune('1'+index)),
			MediaType: "image/png", SHA256: artifactHash, SizeBytes: 100, Status: "READY",
			Width: &width, Height: &height, Revision: 2, CreatedAt: now, UpdatedAt: now,
		}).Error; err != nil {
			t.Fatalf("seed Generation Human Gate Artifact: %v", err)
		}
		if err = database.Create(&model.GenerationCandidate{
			ID: candidateID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
			ProviderJobID: setID, OutputKey: "image-" + string(rune('1'+index)), ArtifactID: artifactID,
			ArtifactRevision: 2, ArtifactSHA256: artifactHash, MediaType: "image/png", Width: width, Height: height,
			Status: "QC_PASSED", Revision: 1, CreatedBy: fixture.userID, CreatedAt: now, UpdatedAt: now,
		}).Error; err != nil {
			t.Fatalf("seed Generation Human Gate Candidate: %v", err)
		}
		references[index] = generationdomain.CandidateReference{
			ID: candidateID.String(), Revision: 1, ArtifactID: artifactID.String(), ArtifactRevision: 2,
			ArtifactSHA256: artifactHash, QCReportID: reportID.String(), QCReportHash: reportHash,
		}
		bundles[candidateID.String()] = generationdomain.CandidateWithReport{
			Candidate: generationdomain.Candidate{
				ID: candidateID.String(), WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
				ProviderJobID: setID.String(), OutputKey: "image-" + string(rune('1'+index)), ArtifactID: artifactID.String(),
				ArtifactRevision: 2, ArtifactSHA256: artifactHash, MediaType: "image/png", Width: width, Height: height,
				Status: generationdomain.CandidateQCPassed, Revision: 1, CreatedBy: fixture.userID.String(),
			},
			Report: generationdomain.QCReport{
				ID: reportID.String(), WorkspaceID: fixture.workspaceID.String(), CandidateID: candidateID.String(),
				Status: generationdomain.QCPassed, ReportHash: reportHash,
			},
		}
	}
	setHash, err := platformcommand.InputHash(struct {
		Candidates []generationdomain.CandidateReference `json:"candidates"`
	}{Candidates: references})
	if err != nil {
		t.Fatalf("hash Generation CandidateSet: %v", err)
	}
	set := generationdomain.CandidateSet{
		ID: setID.String(), WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		ProviderReceiptID: receiptID.String(), Candidates: references, ContentHash: setHash, Revision: 1,
	}

	port := func(key, valueType string) authoring.PortDefinition {
		return authoring.PortDefinition{Key: key, ValueType: valueType, Required: true}
	}
	catalog, err := authoring.NewCatalog("lanverse.production", "99.0.0", []authoring.NodeDefinition{
		{
			Key: "test.generation_candidate_set", Version: "1.0.0", Name: "Generation CandidateSet",
			Category: "test", Executor: "test.generation_candidate_set", OutputPorts: []authoring.PortDefinition{port("candidates", "generation_candidate_set")},
			ConfigSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), CachePolicy: "never", RiskLevel: "low", Executable: true,
		},
		{
			Key: "human.generation_image_review", Version: "1.0.0", Name: "Generation Image Review",
			Category: "human", Executor: "gate.generation_image_review",
			InputPorts:   []authoring.PortDefinition{port("candidates", "generation_candidate_set")},
			OutputPorts:  []authoring.PortDefinition{port("selection", "generation_candidate_selection")},
			ConfigSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), CachePolicy: "never", RiskLevel: "human_gate", Executable: true,
		},
	})
	if err != nil {
		t.Fatalf("build Generation Human Gate catalog: %v", err)
	}
	authoringStore := authoringgorm.New(database)
	if _, err = authoringStore.EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist Generation Human Gate catalog: %v", err)
	}
	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString})
	authoringActor := authoringapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	draft, err := authoringService.Create(ctx, authoringActor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "CANVAS",
		Graph: authoring.Graph{
			Nodes: []authoring.Node{
				{ID: "candidate-set", DefinitionKey: "test.generation_candidate_set", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
				{ID: "image-review", DefinitionKey: "human.generation_image_review", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			},
			Edges: []authoring.Edge{{
				ID: "candidate-set-review", FromNodeID: "candidate-set", FromPort: "candidates",
				ToNodeID: "image-review", ToPort: "candidates",
			}},
		},
		Layout: json.RawMessage(`{}`), FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
		}},
		CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "generation-review-authoring-create",
	})
	if err != nil {
		t.Fatalf("create Generation Human Gate Authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "generation-review-authoring-publish",
	})
	if err != nil {
		t.Fatalf("publish Generation Human Gate Authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore, workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	reviewService := reviewapp.NewService(reviewgorm.New(database), reviewapp.Config{
		Now: func() time.Time { now = now.Add(time.Second); return now }, NewID: uuid.NewString, ClaimLease: time.Minute,
	})
	setSource := &generationCandidateSetSource{set: set}
	runtime := workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{
		Now: func() time.Time { now = now.Add(time.Second); return now }, NewID: uuid.NewString,
		Executor: &generationCandidateSetExecutor{set: set}, HumanTasks: workflowreview.NewWithGeneration(reviewService, setSource),
	})
	temporalRuntime, err := workflowtemporal.New(workflowtemporal.Config{
		Address: temporalAddress, Namespace: "default", TaskQueue: "lanverse-generation-review-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("connect Generation Human Gate Temporal: %v", err)
	}
	t.Cleanup(temporalRuntime.Close)
	runtimeWorker, err := temporalRuntime.NewWorker(runtime)
	if err != nil {
		t.Fatalf("compose Generation Human Gate Temporal Worker: %v", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start Generation Human Gate Temporal Worker: %v", err)
	}
	t.Cleanup(runtimeWorker.Stop)
	startService := workflowapp.NewStartService(compiler, workflowStore, temporalRuntime, workflowapp.StartConfig{
		Now: func() time.Time { now = now.Add(time.Second); return now }, NewID: uuid.NewString,
	})
	workflowActor := workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	startResult, err := startService.Start(ctx, workflowActor, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "generation-review-start",
	})
	if err != nil || startResult.Status != "RUNNING" {
		t.Fatalf("start Generation Human Gate Workflow: run=%#v err=%v", startResult, err)
	}
	var task model.HumanTask
	deadline := time.Now().Add(20 * time.Second)
	for {
		err = database.Where("workflow_run_id = ?", startResult.ID).First(&task).Error
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("wait for Generation CandidateSet HumanTask: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	var gateNode model.NodeRunProjection
	if err = database.First(&gateNode, "id = ?", task.NodeRunID).Error; err != nil {
		t.Fatalf("load Generation Human Gate NodeRun: %v", err)
	}
	var frozenCandidates []string
	if err = json.Unmarshal(task.CandidateIDs, &frozenCandidates); err != nil ||
		task.SubjectType != "generation_candidate_selection" || !slices.Equal(frozenCandidates, []string{candidateIDs[0].String(), candidateIDs[1].String()}) {
		t.Fatalf("Generation CandidateSet HumanTask drifted: task=%#v candidates=%v err=%v", task, frozenCandidates, err)
	}
	reviewActor := reviewapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	claim, err := reviewService.Claim(ctx, reviewActor, reviewapp.ClaimCommand{
		TaskID: task.ID.String(), ExpectedRevision: task.Revision, IdempotencyKey: "generation-review-claim",
	})
	if err != nil {
		t.Fatalf("claim Generation image review: %v", err)
	}
	decision, err := reviewService.Decide(ctx, reviewActor, reviewapp.DecideCommand{
		TaskID: task.ID.String(), ClaimToken: claim.ClaimToken, ExpectedTaskRevision: claim.Task.Revision,
		ExpectedSubjectRevision: claim.Task.SubjectRevision, Decision: "selected",
		SelectedCandidateID: candidateIDs[1].String(), IdempotencyKey: "generation-review-select",
	})
	if err != nil {
		t.Fatalf("select Generation Candidate: %v", err)
	}
	selectionService := generationapp.NewSelectionService(
		generationgorm.New(database), generationCandidateReadiness{bundles: bundles},
		generationreview.NewDecisionReader(reviewService),
		generationapp.SelectionConfig{Now: func() time.Time { now = now.Add(time.Second); return now }, NewID: uuid.NewString},
	)
	signaler := &loseFirstGenerationSignal{delegate: temporalRuntime}
	signals := workflowapp.NewSignalService(workflowStore, signaler, workflowapp.SignalConfig{
		Now: func() time.Time { now = now.Add(time.Second); return now }, NewID: uuid.NewString,
		Owner: workflowgeneration.NewHumanGateApplier(selectionService),
	})
	signalCommand := workflowapp.SignalHumanGateCommand{
		WorkspaceID: fixture.workspaceID.String(), WorkflowRunID: startResult.ID, NodeRunID: gateNode.ID.String(),
		HumanTaskID: decision.Task.ID, ReviewDecisionID: decision.Decision.ID,
		SubjectRevision: decision.Decision.SubjectRevision, Decision: "selected", IdempotencyKey: "generation-review-signal",
	}
	unknown, err := signals.SignalHumanGate(ctx, workflowActor, signalCommand)
	if err != nil || unknown.Status != "unknown" {
		t.Fatalf("persist unknown Generation selection Signal: intent=%#v err=%v", unknown, err)
	}
	signals = workflowapp.NewSignalService(workflowStore, signaler, workflowapp.SignalConfig{
		Now: func() time.Time { now = now.Add(time.Second); return now }, NewID: uuid.NewString,
		Owner: workflowgeneration.NewHumanGateApplier(selectionService),
	})
	completed, err := signals.SignalHumanGate(ctx, workflowActor, signalCommand)
	if err != nil || completed.Status != "completed" || completed.ID != unknown.ID {
		t.Fatalf("reconcile Generation selection Signal: intent=%#v err=%v", completed, err)
	}
	const replayCallers = 8
	replayErrors := make(chan error, replayCallers)
	var replayWorkers sync.WaitGroup
	for range replayCallers {
		replayWorkers.Add(1)
		go func() {
			defer replayWorkers.Done()
			replayed, replayErr := signals.SignalHumanGate(ctx, workflowActor, signalCommand)
			if replayErr != nil {
				replayErrors <- replayErr
				return
			}
			if replayed.ID != completed.ID || replayed.Status != "completed" {
				replayErrors <- errors.New("concurrent Generation Signal replay returned different facts")
			}
		}()
	}
	replayWorkers.Wait()
	close(replayErrors)
	for replayErr := range replayErrors {
		t.Fatalf("replay Generation selection Signal concurrently: %v", replayErr)
	}
	requests := signaler.Requests()
	if len(requests) != 2 || requests[0].OwnerReceiptID == "" || len(requests[0].Output.Bindings) != 1 ||
		requests[0].Output.Bindings[0].ValueType != "generation_candidate_selection" {
		t.Fatalf("Generation selection Signal evidence drifted: %#v", requests)
	}
	var completedRun model.WorkflowRun
	deadline = time.Now().Add(20 * time.Second)
	for {
		err = database.First(&completedRun, "id = ?", startResult.ID).Error
		if err == nil && completedRun.Status == "SUCCEEDED" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("wait for Generation selection Workflow completion: run=%#v err=%v", completedRun, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	temporalClient, err := client.Dial(client.Options{HostPort: temporalAddress, Namespace: "default"})
	if err != nil {
		t.Fatalf("dial Generation Human Gate Temporal history: %v", err)
	}
	t.Cleanup(temporalClient.Close)
	iterator := temporalClient.GetWorkflowHistory(
		ctx, startResult.TemporalWorkflowID, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT,
	)
	history := &historypb.History{}
	for iterator.HasNext() {
		event, historyErr := iterator.Next()
		if historyErr != nil {
			t.Fatalf("read Generation Human Gate Temporal history: %v", historyErr)
		}
		history.Events = append(history.Events, event)
	}
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(
		workflowtemporal.EpisodeProductionWorkflow,
		temporalworkflow.RegisterOptions{Name: workflowtemporal.EpisodeProductionWorkflowName},
	)
	if err = replayer.ReplayWorkflowHistory(nil, history); err != nil {
		t.Fatalf("replay Generation Human Gate Temporal history: %v", err)
	}
	var selectionCount, ownerReceiptCount, applyCount, signalIntentCount int64
	checks := []struct {
		value any
		query string
		args  []any
		count *int64
	}{
		{&model.GenerationCandidateSelection{}, "workflow_run_id = ?", []any{startResult.ID}, &selectionCount},
		{&model.CommandReceipt{}, "workspace_id = ? AND operation = ?", []any{fixture.workspaceID, "generation.candidate.select"}, &ownerReceiptCount},
		{&model.WorkflowHumanGateApplyReceipt{}, "workflow_run_id = ?", []any{startResult.ID}, &applyCount},
		{&model.WorkflowSignalIntent{}, "workflow_run_id = ?", []any{startResult.ID}, &signalIntentCount},
	}
	for _, check := range checks {
		if err = database.Model(check.value).Where(check.query, check.args...).Count(check.count).Error; err != nil {
			t.Fatalf("count Generation Human Gate facts: %v", err)
		}
	}
	if selectionCount != 1 || ownerReceiptCount != 1 || applyCount != 1 || signalIntentCount != 1 {
		t.Fatalf("Generation Human Gate facts duplicated: selection=%d owner=%d apply=%d signal=%d", selectionCount, ownerReceiptCount, applyCount, signalIntentCount)
	}
	var gateProjection model.NodeRunProjection
	if err = database.First(&gateProjection, "id = ?", gateNode.ID).Error; err != nil ||
		gateProjection.Status != "SUCCEEDED" || gateProjection.OutputHash == nil || len(gateProjection.Output) == 0 {
		t.Fatalf("Generation Human Gate projection not completed: projection=%#v err=%v", gateProjection, err)
	}
}

var _ workflowapp.HumanGateOwnerApplier = (*workflowgeneration.HumanGateApplier)(nil)
