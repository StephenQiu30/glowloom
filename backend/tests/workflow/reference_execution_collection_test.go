package workflow_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgeneration "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/generation"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	temporaladapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
	"github.com/google/uuid"
	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"
)

func assertReferenceExecutionCollection(t *testing.T, parent context.Context, database *generationtestgorm.Database, actor genapp.Actor, fixture referencePreparationFixture, configuration referenceExecutionFixture) {
	t.Helper()
	address := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if address == "" {
		t.Log("Temporal not configured: full Reference collection not exercised")
		return
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	store := generationgorm.New(database)
	query := genapp.NewReferenceExecutionQuery(store)
	before, err := query.Get(ctx, actor, fixture.execution.ProjectID, fixture.execution.ID)
	if err != nil || before.Total < 2 {
		t.Fatalf("collection fixture: %+v %v", before, err)
	}
	requests := 0
	factory, objects := referenceExecutionHTTPFactory(t, before.Total, 2, func() {
		requests++
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var row model.GenerationReferenceProviderCall
		if err := database.WithContext(checkCtx).Where("call_key = ?", before.Calls[requests-1].CallKey).First(&row).Error; err != nil || row.Status != gen.ProviderCallDispatching {
			t.Errorf("collection sent before committed claim: %v", err)
		}
	})
	registry, err := genapp.NewMediaFactoryRegistry([]genapp.MediaAdapterFactory{factory})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := genapp.NewReferenceCallExecutionService(store, registry, configuration.secrets, time.Now, uuid.NewString)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := genapp.NewReferenceCallDispatchService(store, registry, time.Now, uuid.NewString)
	if err != nil {
		t.Fatal(err)
	}
	media, err := genapp.NewReferenceStagedMediaService(store, objects, gen.ReferenceObjectStoreRef{Profile: "minio", Bucket: "lanverse"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	executor, err := workflowgeneration.NewReferenceObservationNodeExecutor(execution, recovery, media, query)
	if err != nil {
		t.Fatal(err)
	}
	replyLoss := &referenceCallNodeReplyLoss{NodeExecutor: executor}
	workflowStore := workflowgorm.New(database)
	activities := &referenceCallTemporalActivities{RuntimeService: workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{Now: time.Now, NewID: uuid.NewString, Executor: replyLoss}), dropResponse: true}
	runtime, err := temporaladapter.New(temporaladapter.Config{Address: address, Namespace: "default", TaskQueue: "lanverse-reference-collection-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runtimeWorker, err := runtime.NewWorker(activities)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimeWorker.Start(); err != nil {
		t.Fatal(err)
	}
	defer runtimeWorker.Stop()
	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	authoringStore := authoringgorm.New(database)
	if _, err := authoringStore.EnsureCatalog(ctx, catalog, time.Now().UTC(), uuid.NewString); err != nil {
		t.Fatal(err)
	}
	authors := authoringapp.NewService(authoringStore, authoringapp.Config{Now: time.Now, NewID: uuid.NewString})
	graph, err := workflowapp.BuildReferenceExecutionGraph(before)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authors.Create(ctx, authoringapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, authoringapp.CreateCommand{
		ProjectID: fixture.execution.ProjectID, AuthoringMode: "GUIDED", Graph: graph, Layout: json.RawMessage(`{"guided":{"step":1}}`),
		FrozenInputs: []authoring.FrozenReference{{Kind: "reference_execution", ID: before.ExecutionRef.ID, Version: "1", Hash: before.JobHash}},
		CatalogKey:   catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "reference-collection-invalid-freeze",
	}); err == nil {
		t.Fatal("Authoring accepted a foreign frozen Execution hash")
	}
	compiler := workflowapp.NewService(workflowauthoring.New(authors), workflowStore, workflowapp.Config{Now: time.Now, NewID: uuid.NewString})
	starter := workflowapp.NewStartService(compiler, workflowStore, runtime, workflowapp.StartConfig{Now: time.Now, NewID: uuid.NewString})
	collection, err := workflowapp.NewReferenceExecutionStartService(query, authors, starter)
	if err != nil {
		t.Fatal(err)
	}
	command := workflowapp.StartReferenceExecutionCommand{ProjectID: fixture.execution.ProjectID, ExecutionRef: before.ExecutionRef, IdempotencyKey: "reference-collection"}
	runActor := workflowapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}
	bad := command
	bad.ExecutionRef.ContentHash = before.JobHash
	if _, err := collection.Start(ctx, runActor, bad); err == nil {
		t.Fatal("foreign execution hash started collection")
	}
	run, err := collection.Start(ctx, runActor, command)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := client.Dial(client.Options{HostPort: address, Namespace: "default"})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	finished := false
	defer func() {
		if !finished {
			cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			_ = reader.CancelWorkflow(cleanup, run.TemporalWorkflowID, "")
		}
	}()
	var result temporaladapter.RunResult
	if err := reader.GetWorkflow(ctx, run.TemporalWorkflowID, "").Get(ctx, &result); err != nil {
		t.Fatal(err)
	}
	finished = true
	if result.Status != "SUCCEEDED" {
		t.Fatalf("collection did not finish observations: %+v", result)
	}
	after, err := query.Get(ctx, actor, command.ProjectID, command.ExecutionRef.ID)
	if err != nil || !after.Terminal || after.Status != gen.ProviderJobPartialSucceeded || after.Failed != 1 || after.Succeeded != before.Total-1 || after.Pending != 0 || after.OutcomeUnknown != 0 {
		t.Fatalf("complete explicit outcomes: %+v %v", after, err)
	}
	if !replyLoss.lost.Load() || !activities.lost.Load() || activities.activityCalls.Load() != int64(before.Total+3) {
		t.Fatalf("missing commit/reply recovery: %d", activities.activityCalls.Load())
	}
	repeated, err := collection.Start(ctx, runActor, command)
	if err != nil || repeated.ID != run.ID {
		t.Fatalf("collection restart created a second Run: %+v %v", repeated, err)
	}
	var nodes []model.NodeRunProjection
	if err := database.Where("workflow_run_id = ?", run.ID).Find(&nodes).Error; err != nil || len(nodes) != before.Total+1 {
		t.Fatalf("missing durable Call nodes: %d %v", len(nodes), err)
	}
	for _, node := range nodes {
		if node.Status != "SUCCEEDED" || node.CacheKey != nil {
			t.Fatal("collection node was not durably completed without cache")
		}
	}
	var mediaCount, assets int64
	if err := database.Model(&model.GenerationReferenceStagedMedia{}).Where("project_id = ? AND call_key IN ?", command.ProjectID, referenceCollectionKeys(before)).Count(&mediaCount).Error; err != nil || mediaCount != int64(before.Total-1) {
		t.Fatalf("missing staged media: %d %v", mediaCount, err)
	}
	if err := database.Model(&model.Artifact{}).Where("project_id = ?", command.ProjectID).Count(&assets).Error; err != nil || assets != 0 {
		t.Fatal("collection published formal assets")
	}
	fullHistory, _, _, _ := loadRecoveredWorkflowHistory(t, ctx, reader, run.TemporalWorkflowID)
	for _, event := range fullHistory.Events {
		var payloadSets []*commonpb.Payloads
		if attributes := event.GetWorkflowExecutionStartedEventAttributes(); attributes != nil {
			payloadSets = append(payloadSets, attributes.Input)
		}
		if attributes := event.GetActivityTaskScheduledEventAttributes(); attributes != nil {
			payloadSets = append(payloadSets, attributes.Input)
		}
		if attributes := event.GetActivityTaskCompletedEventAttributes(); attributes != nil {
			payloadSets = append(payloadSets, attributes.Result)
		}
		for _, set := range payloadSets {
			for _, payload := range set.GetPayloads() {
				for _, forbidden := range []string{"sk-reference-execution-fixture", "staging/reference/", "api_key", "ciphertext", "b64_json"} {
					if strings.Contains(string(payload.Data), forbidden) {
						t.Fatalf("collection history leaked %s", forbidden)
					}
				}
			}
		}
	}
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(temporaladapter.EpisodeProductionWorkflow, temporalworkflow.RegisterOptions{Name: temporaladapter.EpisodeProductionWorkflowName})
	if err := replayer.ReplayWorkflowHistory(nil, fullHistory); err != nil {
		t.Fatal(err)
	}
}

func referenceCollectionKeys(progress gen.ReferenceJobProgress) []string {
	keys := make([]string, len(progress.Calls))
	for i, call := range progress.Calls {
		keys[i] = call.CallKey
	}
	return keys
}
