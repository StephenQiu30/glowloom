package workflow_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"

	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoringdomain "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	costgorm "github.com/StephenQiu30/lanverse/backend/internal/cost/adapter/gormdb"
	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	quotagorm "github.com/StephenQiu30/lanverse/backend/internal/quota/adapter/gormdb"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	quotadomain "github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgeneration "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/generation"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	temporaladapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
)

const (
	formalGenerationWorkerHelperFlag    = "LANVERSE_FORMAL_GENERATION_WORKER_HELPER"
	formalGenerationWorkerAddress       = "LANVERSE_FORMAL_GENERATION_WORKER_ADDRESS"
	formalGenerationWorkerTaskQueue     = "LANVERSE_FORMAL_GENERATION_WORKER_TASK_QUEUE"
	formalGenerationWorkerMode          = "LANVERSE_FORMAL_GENERATION_WORKER_MODE"
	formalGenerationWorkerAuditPath     = "LANVERSE_FORMAL_GENERATION_WORKER_AUDIT_PATH"
	formalGenerationWorkerTargetID      = "LANVERSE_FORMAL_GENERATION_WORKER_TARGET_ID"
	formalGenerationWorkerModeSubmit    = "submit"
	formalGenerationWorkerModeReconcile = "reconcile"
	formalGenerationSourceNodeID        = "approved-intents"
	formalGenerationNodeID              = "reference-assets"
	formalGenerationRemoteRequestID     = "formal-reference-remote-request"
	formalGenerationRemoteJobID         = "formal-reference-remote-task"
	formalGenerationProviderKey         = "controlled-image"
	formalGenerationExternalModelID     = "image-quality"
	formalGenerationAdapterContract     = "controlled-image"
	formalGenerationCatalogVersion      = "99.21.0"
)

type formalGenerationFixture struct {
	project         compilerProjectFixture
	target          generationdomain.GenerationTarget
	approvedID      string
	approvedHash    string
	modelProfileID  string
	workflowRunID   string
	workflowID      string
	nodeRunID       string
	sourceNodeRunID string
}

type formalGenerationAuditEvent struct {
	Operation       string `json:"operation"`
	ProviderJobID   string `json:"provider_job_id"`
	ProviderCallID  string `json:"provider_call_id"`
	RemoteRequestID string `json:"remote_request_id"`
	RemoteJobID     string `json:"remote_job_id"`
}

func TestTemporalEpisodeGenerationRecoversThroughFormalRuntimeAcrossWorkerProcesses(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	temporalAddress := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if databaseURL == "" || temporalAddress == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL and LANVERSE_TEST_TEMPORAL_ADDRESS to run the formal Generation recovery journey")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open formal Generation recovery database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize formal Generation recovery GORM catalog: %v", err)
	}

	taskQueue := "lanverse-formal-generation-recovery-" + uuid.NewString()
	temporalRuntime, err := temporaladapter.New(temporaladapter.Config{
		Address: temporalAddress, Namespace: "default", TaskQueue: taskQueue,
	})
	if err != nil {
		t.Fatalf("connect formal Generation Temporal runtime: %v", err)
	}
	t.Cleanup(temporalRuntime.Close)
	if err = temporalRuntime.Ping(ctx); err != nil {
		t.Fatalf("check formal Generation Temporal health: %v", err)
	}

	temporalClient := mustTemporalClient(t, temporalAddress)
	fixture := seedFormalGenerationRecovery(t, ctx, database, temporalRuntime, temporalClient)
	auditPath := t.TempDir() + "/formal-generation-boundaries.jsonl"
	firstWorker, firstOutput := startFormalGenerationWorkerProcess(
		t, databaseURL, temporalAddress, taskQueue, formalGenerationWorkerModeSubmit, auditPath, fixture.target.ID,
	)
	firstActivityID := "execute-node:" + fixture.nodeRunID
	waitForCompletedActivity(t, ctx, temporalClient, fixture.workflowID, firstActivityID)
	waitForReferenceAssetPollTimer(t, ctx, temporalClient, fixture.workflowID, firstActivityID)
	stopWorkflowWorkerProcess(t, firstWorker, firstOutput)
	firstFacts := loadFormalGenerationFacts(t, database, fixture)
	assertFormalGenerationSubmitted(t, firstFacts)

	secondWorker, secondOutput := startFormalGenerationWorkerProcess(
		t, databaseURL, temporalAddress, taskQueue, formalGenerationWorkerModeReconcile, auditPath, fixture.target.ID,
	)
	var result temporaladapter.RunResult
	if err = temporalClient.GetWorkflow(ctx, fixture.workflowID, "").Get(ctx, &result); err != nil {
		t.Fatalf("wait for formal Generation Workflow recovery: %v\n%s", err, secondOutput.String())
	}
	if result.WorkflowRunID != fixture.workflowRunID || result.Status != workflowdomain.NodeActivityNeedsAttention {
		t.Fatalf("formal Generation Workflow recovery result drifted: %#v", result)
	}
	stopWorkflowWorkerProcess(t, secondWorker, secondOutput)

	finalFacts := loadFormalGenerationFacts(t, database, fixture)
	assertFormalGenerationUnknown(t, firstFacts, finalFacts)
	assertFormalGenerationCancellationFenced(t, ctx, database, fixture, finalFacts)
	assertFormalGenerationAudit(t, auditPath, firstFacts)
	assertFormalGenerationHistory(t, ctx, temporalClient, fixture, firstActivityID)
}

func seedFormalGenerationRecovery(
	t *testing.T,
	ctx context.Context,
	database *generationtestgorm.Database,
	temporalRuntime *temporaladapter.Client,
	temporalClient client.Client,
) formalGenerationFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	create := func(value any) error { return database.Create(value).Error }
	project := seedCompilerProject(t, create, now)
	catalog := formalGenerationCatalog(t)
	authoringStore := authoringgorm.New(database)
	if _, err := authoringStore.EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist formal Generation node catalog: %v", err)
	}

	approvedID, approvedHash := uuid.NewString(), strings.Repeat("a", 64)
	target := formalGenerationTarget(t, project, approvedID, approvedHash, now)
	target, err := generationgorm.NewTargetStore(database).Ensure(ctx, target)
	if err != nil {
		t.Fatalf("persist formal Generation target: %v", err)
	}
	modelProfileID := seedFormalGenerationProvider(t, create, project, now)
	configureFormalGenerationOwners(t, ctx, database, project, modelProfileID, now)

	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	actor := authoringapp.Actor{UserID: project.userID.String(), TokenVersion: 1}
	draft, err := authoringService.Create(ctx, actor, authoringapp.CreateCommand{
		ProjectID: project.projectID.String(), AuthoringMode: "GUIDED",
		Graph: formalGenerationGraph(target), Layout: json.RawMessage(`{"guided":{"step":1}}`),
		FrozenInputs: []authoringdomain.FrozenReference{{
			Kind: "script_revision", ID: project.scriptRevisionID.String(), Version: "1", Hash: project.normalizedHash,
		}},
		CatalogKey: catalog.Key, CatalogVersion: catalog.Version,
		IdempotencyKey: "formal-generation-recovery-create-" + project.projectID.String(),
	})
	if err != nil {
		t.Fatalf("create formal Generation authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, actor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision,
		IdempotencyKey: "formal-generation-recovery-publish-" + project.projectID.String(),
	})
	if err != nil {
		t.Fatalf("publish formal Generation authoring revision: %v", err)
	}
	workflowStore := workflowgorm.New(database)
	compiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore,
		workflowapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	startService := workflowapp.NewStartService(compiler, workflowStore, temporalRuntime, workflowapp.StartConfig{
		Now: func() time.Time {
			now = now.Add(time.Microsecond)
			return now
		},
		NewID: uuid.NewString,
	})
	run, err := startService.Start(ctx, workflowapp.Actor{
		UserID: project.userID.String(), TokenVersion: 1,
	}, workflowapp.StartCommand{
		AuthoringRevisionID: revision.ID,
		IdempotencyKey:      "formal-generation-recovery-start-" + project.projectID.String(),
	})
	if err != nil || run.Status != "RUNNING" {
		t.Fatalf("start formal Generation Episode Workflow: run=%#v err=%v", run, err)
	}
	registerFormalGenerationWorkflowCleanup(t, temporalClient, run.TemporalWorkflowID)

	var nodes []model.NodeRunProjection
	if err = database.Where("workflow_run_id = ?", run.ID).Find(&nodes).Error; err != nil {
		t.Fatalf("load formal Generation node projections: %v", err)
	}
	byNode := make(map[string]model.NodeRunProjection, len(nodes))
	for _, node := range nodes {
		byNode[node.NodeID] = node
	}
	source, sourceFound := byNode[formalGenerationSourceNodeID]
	generation, generationFound := byNode[formalGenerationNodeID]
	if !sourceFound || !generationFound {
		t.Fatalf("formal Generation run projections are incomplete: %#v", nodes)
	}
	_, sourceInput, sourceInputHash, err := workflowdomain.BuildNodeInput(workflowdomain.NodeInputSnapshot{
		SchemaVersion: workflowdomain.NodeInputSchemaVersion,
		Config:        json.RawMessage(`{}`),
		FrozenInputs: []authoringdomain.FrozenReference{{
			Kind: "script_revision", ID: project.scriptRevisionID.String(), Version: "1", Hash: project.normalizedHash,
		}},
	})
	if err != nil {
		t.Fatalf("build formal approved Intent source input: %v", err)
	}
	_, sourceOutput, sourceOutputHash, err := workflowdomain.BuildNodeOutput(workflowdomain.NodeOutputSnapshot{
		SchemaVersion: workflowdomain.NodeOutputSchemaVersion,
		Bindings: []workflowdomain.NodeOutputBinding{{
			Port: "intents", ValueType: "approved_storyboard_intents", ReferenceID: approvedID,
			ReferenceVersion: "1", ContentHash: approvedHash,
		}},
	})
	if err != nil {
		t.Fatalf("build formal approved Intent source output: %v", err)
	}
	if err = database.Model(&model.NodeRunProjection{}).Where("id = ?", source.ID).Updates(map[string]any{
		"status": "SUCCEEDED", "attempt": 1, "input": sourceInput, "input_hash": sourceInputHash,
		"output": sourceOutput, "output_hash": sourceOutputHash, "revision": source.Revision + 1, "updated_at": now,
	}).Error; err != nil {
		t.Fatalf("freeze formal approved Intent source projection: %v", err)
	}

	return formalGenerationFixture{
		project: project, target: target, approvedID: approvedID, approvedHash: approvedHash,
		modelProfileID: modelProfileID, workflowRunID: run.ID, workflowID: run.TemporalWorkflowID,
		nodeRunID: generation.ID.String(), sourceNodeRunID: source.ID.String(),
	}
}

func registerFormalGenerationWorkflowCleanup(t *testing.T, temporalClient client.Client, workflowID string) {
	t.Helper()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		description, err := temporalClient.DescribeWorkflowExecution(cleanupCtx, workflowID, "")
		if err != nil {
			t.Errorf("describe formal Generation Workflow during cleanup: %v", err)
			return
		}
		if description.GetWorkflowExecutionInfo().GetStatus() != enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING {
			return
		}
		if err = temporalClient.TerminateWorkflow(
			cleanupCtx, workflowID, "", "formal Generation recovery test cleanup",
		); err != nil {
			t.Errorf("terminate running formal Generation Workflow during cleanup: %v", err)
		}
	})
}

func formalGenerationCatalog(t *testing.T) authoringdomain.Catalog {
	t.Helper()
	required := func(key, valueType string) authoringdomain.PortDefinition {
		return authoringdomain.PortDefinition{Key: key, ValueType: valueType, Required: true}
	}
	catalog, err := authoringdomain.NewCatalog("lanverse.production", formalGenerationCatalogVersion, []authoringdomain.NodeDefinition{
		{
			Key: "test.approved_intents", Version: "1.0.0", Name: "Approved Intents",
			Category: "test", Executor: "test.approved_intents", OutputPorts: []authoringdomain.PortDefinition{
				required("intents", "approved_storyboard_intents"),
			},
			ConfigSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			CachePolicy:  "never", RiskLevel: "low", Executable: true,
		},
		{
			Key: "generation.reference_asset", Version: "1.0.0", Name: "Reference Asset Generation",
			Category: "generation", Executor: "activity.reference_asset_generation",
			InputPorts:   []authoringdomain.PortDefinition{required("intents", "approved_storyboard_intents")},
			OutputPorts:  []authoringdomain.PortDefinition{required("candidates", "generation_candidate_set")},
			ConfigSchema: json.RawMessage(`{"type":"object","properties":{"asset_id":{"type":"string","format":"uuid"},"asset_state_id":{"type":"string","format":"uuid"}},"required":["asset_id","asset_state_id"],"additionalProperties":false}`),
			CachePolicy:  "by_inputs", RiskLevel: "external_ai", Executable: true,
		},
	})
	if err != nil {
		t.Fatalf("build formal Generation node catalog: %v", err)
	}
	return catalog
}

func formalGenerationGraph(target generationdomain.GenerationTarget) authoringdomain.Graph {
	return authoringdomain.Graph{
		Nodes: []authoringdomain.Node{
			{ID: formalGenerationSourceNodeID, DefinitionKey: "test.approved_intents", DefinitionVersion: "1.0.0", Config: json.RawMessage(`{}`)},
			{ID: formalGenerationNodeID, DefinitionKey: "generation.reference_asset", DefinitionVersion: "1.0.0", Config: json.RawMessage(
				`{"asset_id":"` + target.ReferenceAsset.AssetID + `","asset_state_id":"` + target.ReferenceAsset.AssetStateRef.ID + `"}`,
			)},
		},
		Edges: []authoringdomain.Edge{{
			ID: "approved-intents-to-reference-assets", FromNodeID: formalGenerationSourceNodeID,
			FromPort: "intents", ToNodeID: formalGenerationNodeID, ToPort: "intents",
		}},
	}
}

func formalGenerationTarget(
	t *testing.T,
	project compilerProjectFixture,
	approvedID string,
	approvedHash string,
	now time.Time,
) generationdomain.GenerationTarget {
	t.Helper()
	target, err := generationdomain.NewGenerationTarget(generationdomain.GenerationTargetInput{
		ID: uuid.NewString(), WorkspaceID: project.workspaceID.String(), ProjectID: project.projectID.String(),
		Kind: generationdomain.GenerationTargetReferenceAsset,
		SourceOwnerRef: generationdomain.FrozenOwnerReference{
			Owner: "storyboard", Resource: "approved_storyboard_intents", ID: approvedID,
			Revision: 1, ContentHash: approvedHash,
		},
		PolicySnapshotRef: generationdomain.FrozenOwnerReference{
			Owner: "preset", Resource: "effective_style_snapshot", ID: uuid.NewString(),
			Revision: 1, ContentHash: strings.Repeat("b", 64),
		},
		ReferenceAsset: &generationdomain.ReferenceAssetTarget{
			AssetID: uuid.NewString(), AssetKind: "character",
			SpecificationVersionRef: generationdomain.FrozenOwnerReference{
				Owner: "production", Resource: "production_bible_specification_version", ID: uuid.NewString(),
				Revision: 1, ContentHash: strings.Repeat("c", 64),
			},
			AssetStateRef: generationdomain.FrozenOwnerReference{
				Owner: "asset", Resource: "asset_state", ID: uuid.NewString(),
				Revision: 1, ContentHash: strings.Repeat("d", 64),
			},
			OutputKind: "reference_sheet", RequiredViewRoles: []string{"front", "profile", "back"},
			PromptVersion: "character-reference-sheet", PositivePrompt: "formal character reference sheet",
			NegativePrompt: "identity drift", Width: 1536, Height: 1024, NumberResults: 4, OutputFormat: "png",
		},
		Revision: 1, CreatedBy: project.userID.String(), CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("build formal Generation target: %v", err)
	}
	return target
}

func seedFormalGenerationProvider(
	t *testing.T,
	create func(any) error,
	project compilerProjectFixture,
	now time.Time,
) string {
	t.Helper()
	credentialID, connectionID, profileID, bindingID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	connectionKey := "formal-generation-primary"
	connectionDomain := generationdomain.ProviderConnectionVersion{
		ID: connectionID.String(), WorkspaceID: project.workspaceID.String(), ConnectionKey: connectionKey,
		Revision: 1, SourcePresetKey: "controlled.image", SourcePresetVersion: 1,
		PresetSnapshotHash: strings.Repeat("e", 64), ProviderKey: formalGenerationProviderKey,
		DisplayName: "Formal Generation controlled Provider", CredentialVersionID: credentialID.String(),
		ResolvedConfig: map[string]any{}, State: generationdomain.ProviderStateEnabled,
		AdapterContractVersion: formalGenerationAdapterContract, CreatedBy: project.userID.String(), CreatedAt: now,
	}
	connectionHashInput := connectionDomain
	connectionHashInput.ID, connectionHashInput.ContentHash = "", ""
	connectionHashInput.CreatedBy, connectionHashInput.CreatedAt = "", time.Time{}
	connectionHash, err := platformcommand.InputHash(connectionHashInput)
	if err != nil {
		t.Fatalf("hash formal Generation Provider connection: %v", err)
	}
	connectionDomain.ContentHash = connectionHash
	profileDomain := generationdomain.ProviderModelProfileVersion{
		ID: profileID.String(), WorkspaceID: project.workspaceID.String(), ProfileKey: "formal-generation-image",
		Revision: 1, CreationSource: map[string]any{"kind": "test"}, ConnectionKey: connectionKey,
		ProviderKey: formalGenerationProviderKey, ExternalModelID: formalGenerationExternalModelID,
		Modality: generationdomain.MediaModalityImage, Family: "controlled_image",
		AdapterTransportContract: formalGenerationAdapterContract,
		CapabilitySchemaVersion:  formalGenerationAdapterContract,
		BillingMetric:            costdomain.MetricGenerationImageCall, Defaults: map[string]any{},
		State: generationdomain.ProviderStateEnabled, CreatedBy: project.userID.String(), CreatedAt: now,
	}
	profileHashInput := profileDomain
	profileHashInput.ID, profileHashInput.ContentHash = "", ""
	profileHashInput.CreatedBy, profileHashInput.CreatedAt = "", time.Time{}
	profileHash, err := platformcommand.InputHash(profileHashInput)
	if err != nil {
		t.Fatalf("hash formal Generation Provider profile: %v", err)
	}
	profileDomain.ContentHash = profileHash
	bindingDomain := generationdomain.ProjectProviderBindingVersion{
		ID: bindingID.String(), WorkspaceID: project.workspaceID.String(), ProjectID: project.projectID.String(),
		Purpose: generationdomain.ProviderPurposeReferenceAsset, Revision: 1,
		ConnectionVersionID: connectionID.String(), CredentialVersionID: credentialID.String(),
		ModelProfileVersionID: profileID.String(), ProviderKey: formalGenerationProviderKey,
		Modality: generationdomain.MediaModalityImage, AdapterContractVersion: formalGenerationAdapterContract,
		CreatedBy: project.userID.String(), CreatedAt: now,
	}
	bindingHashInput := bindingDomain
	bindingHashInput.ID, bindingHashInput.ContentHash = "", ""
	bindingHashInput.CreatedBy, bindingHashInput.CreatedAt = "", time.Time{}
	bindingHash, err := platformcommand.InputHash(bindingHashInput)
	if err != nil {
		t.Fatalf("hash formal Generation Project Provider binding: %v", err)
	}
	bindingDomain.ContentHash = bindingHash
	records := []any{
		&model.ProviderCredentialVersion{
			ID: credentialID, WorkspaceID: project.workspaceID, ConnectionKey: connectionKey, Revision: 1,
			ProviderKey: formalGenerationProviderKey, CipherSuite: generationdomain.ProviderCipherAES256GCM,
			KeyID: "formal-test-key", Nonce: []byte("0123456789ab"), Ciphertext: []byte("0123456789abcdef"),
			SecretFingerprint: strings.Repeat("f", 64), CreatedBy: project.userID, CreatedAt: now,
		},
		&model.ProviderConnectionVersion{
			ID: connectionID, WorkspaceID: project.workspaceID, ConnectionKey: connectionKey, Revision: 1,
			SourcePresetKey: "controlled.image", SourcePresetVersion: 1, PresetSnapshotHash: strings.Repeat("e", 64),
			ProviderKey: formalGenerationProviderKey, DisplayName: connectionDomain.DisplayName,
			CredentialVersionID: credentialID, ResolvedConfig: []byte(`{}`), State: generationdomain.ProviderStateEnabled,
			AdapterContractVersion: formalGenerationAdapterContract, ContentHash: connectionHash,
			CreatedBy: project.userID, CreatedAt: now,
		},
		&model.ProviderModelProfileVersion{
			ID: profileID, WorkspaceID: project.workspaceID, ProfileKey: profileDomain.ProfileKey, Revision: 1,
			CreationSource: []byte(`{"kind":"test"}`), ConnectionKey: connectionKey,
			ProviderKey: formalGenerationProviderKey, ExternalModelID: formalGenerationExternalModelID,
			Modality: generationdomain.MediaModalityImage, Family: "controlled_image",
			AdapterTransportContract: formalGenerationAdapterContract,
			CapabilitySchemaVersion:  formalGenerationAdapterContract,
			BillingMetric:            costdomain.MetricGenerationImageCall, Defaults: []byte(`{}`),
			State: generationdomain.ProviderStateEnabled, ContentHash: profileHash,
			CreatedBy: project.userID, CreatedAt: now,
		},
		&model.ProjectProviderBindingVersion{
			ID: bindingID, WorkspaceID: project.workspaceID, ProjectID: project.projectID,
			Purpose: generationdomain.ProviderPurposeReferenceAsset, Revision: 1,
			ConnectionVersionID: connectionID, CredentialVersionID: credentialID, ModelProfileVersionID: profileID,
			ProviderKey: formalGenerationProviderKey, Modality: generationdomain.MediaModalityImage,
			AdapterContractVersion: formalGenerationAdapterContract, ContentHash: bindingHash,
			CreatedBy: project.userID, CreatedAt: now,
		},
	}
	for _, record := range records {
		if err = create(record); err != nil {
			t.Fatalf("seed formal Generation Provider %T: %v", record, err)
		}
	}
	return profileID.String()
}

func configureFormalGenerationOwners(
	t *testing.T,
	ctx context.Context,
	database *generationtestgorm.Database,
	project compilerProjectFixture,
	modelProfileID string,
	now time.Time,
) {
	t.Helper()
	actor := costapp.Actor{UserID: project.userID.String(), TokenVersion: 1}
	costs := costapp.NewService(costgorm.New(database), costapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString})
	if _, err := costs.SetBudget(ctx, actor, costapp.SetBudgetCommand{
		ProjectID: project.projectID.String(), LimitAmount: "1000.000000", Currency: "USD",
		ExpectedRevision: 0, IdempotencyKey: "formal-generation-budget-" + project.projectID.String(),
	}); err != nil {
		t.Fatalf("configure formal Generation Cost budget: %v", err)
	}
	if _, err := costs.SetPriceQuote(ctx, actor, costapp.SetPriceQuoteCommand{
		ProjectID: project.projectID.String(), ModelProfileVersionID: modelProfileID,
		ReservationUnitAmount: "1.000000", Currency: "USD", ExpectedRevision: 0,
		IdempotencyKey: "formal-generation-price-" + project.projectID.String(),
	}); err != nil {
		t.Fatalf("configure formal Generation price quote: %v", err)
	}
	quotas := quotaapp.NewService(quotagorm.New(database), quotaapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString})
	if _, err := quotas.SetDailyPolicy(ctx, quotaapp.Actor{UserID: project.userID.String(), TokenVersion: 1}, quotaapp.SetDailyPolicyCommand{
		WorkspaceID: project.workspaceID.String(), ProjectID: project.projectID.String(),
		Metric: quotadomain.MetricGenerationImageCall, LimitUnits: 100, ExpectedRevision: 0,
		IdempotencyKey: "formal-generation-quota-" + project.projectID.String(),
	}); err != nil {
		t.Fatalf("configure formal Generation Quota: %v", err)
	}
}

type formalGenerationFacts struct {
	run                   model.WorkflowRun
	node                  model.NodeRunProjection
	intent                model.GenerationIntent
	request               model.GenerationRequest
	job                   model.GenerationProviderJob
	calls                 []model.GenerationProviderCall
	costReservation       model.CostReservation
	quotaReservation      model.QuotaReservation
	providerReceiptCount  int64
	terminalReceiptCounts map[string]int64
}

func loadFormalGenerationFacts(
	t *testing.T,
	database *generationtestgorm.Database,
	fixture formalGenerationFixture,
) formalGenerationFacts {
	t.Helper()
	var facts formalGenerationFacts
	if err := database.First(&facts.run, "id = ?", fixture.workflowRunID).Error; err != nil {
		t.Fatalf("load formal Generation Workflow run: %v", err)
	}
	if err := database.First(&facts.node, "id = ?", fixture.nodeRunID).Error; err != nil {
		t.Fatalf("load formal Generation node projection: %v", err)
	}
	if err := database.Where(
		"workflow_run_id = ? AND node_run_id = ?", fixture.workflowRunID, fixture.nodeRunID,
	).First(&facts.intent).Error; err != nil {
		t.Fatalf("load formal Generation intent: %v", err)
	}
	if err := database.Where("intent_id = ?", facts.intent.ID).First(&facts.request).Error; err != nil {
		t.Fatalf("load formal Generation request: %v", err)
	}
	if err := database.Where("intent_id = ?", facts.intent.ID).First(&facts.job).Error; err != nil {
		t.Fatalf("load formal Generation Provider job: %v", err)
	}
	if err := database.Where("job_id = ?", facts.job.ID).Order("candidate_index ASC").Find(&facts.calls).Error; err != nil {
		t.Fatalf("load formal Generation Provider calls: %v", err)
	}
	if facts.intent.CostReservationID == nil || facts.intent.QuotaReservationID == nil {
		t.Fatalf("formal Generation intent lost reservations: %#v", facts.intent)
	}
	if err := database.First(&facts.costReservation, "id = ?", *facts.intent.CostReservationID).Error; err != nil {
		t.Fatalf("load formal Generation Cost reservation: %v", err)
	}
	if err := database.First(&facts.quotaReservation, "id = ?", *facts.intent.QuotaReservationID).Error; err != nil {
		t.Fatalf("load formal Generation Quota reservation: %v", err)
	}
	if err := database.Model(&model.GenerationProviderResultReceipt{}).
		Joins("JOIN gen_provider_calls ON gen_provider_calls.id = gen_provider_result_receipts.call_id").
		Where("gen_provider_calls.job_id = ?", facts.job.ID).Count(&facts.providerReceiptCount).Error; err != nil {
		t.Fatalf("count formal Generation Provider result receipts: %v", err)
	}
	facts.terminalReceiptCounts = make(map[string]int64, 5)
	for _, operation := range []string{
		"generation.provider.terminal", "cost.reservation.settle", "cost.reservation.release",
		"quota.reservation.consume", "quota.reservation.release",
	} {
		var count int64
		if err := database.Model(&model.CommandReceipt{}).Where(
			"workspace_id = ? AND operation = ?", facts.intent.WorkspaceID, operation,
		).Count(&count).Error; err != nil {
			t.Fatalf("count formal Generation %s receipts: %v", operation, err)
		}
		facts.terminalReceiptCounts[operation] = count
	}
	return facts
}

func assertFormalGenerationSubmitted(t *testing.T, facts formalGenerationFacts) {
	t.Helper()
	if facts.run.Status != "RETRYING" || facts.node.Status != "RETRYING" || facts.node.Attempt != 1 ||
		facts.node.ActiveClaimToken != nil || len(facts.node.Output) != 0 || facts.node.OutputHash != nil {
		t.Fatalf("first formal Generation activity did not project RETRYING: run=%#v node=%#v", facts.run, facts.node)
	}
	if facts.intent.Status != generationdomain.IntentExecuting || facts.intent.GenerationRequestID == nil ||
		facts.intent.ProviderJobID == nil || *facts.intent.GenerationRequestID != facts.request.ID ||
		*facts.intent.ProviderJobID != facts.job.ID || facts.request.IntentID != facts.intent.ID ||
		facts.job.RequestID != facts.request.ID || facts.job.Status != generationdomain.ProviderJobRunning ||
		len(facts.calls) != 4 {
		t.Fatalf("first formal Generation Provider facts are incomplete: %#v", facts)
	}
	firstCall := facts.calls[0]
	if firstCall.Status != generationdomain.ProviderCallSubmitted || firstCall.RemoteRequestID == nil ||
		firstCall.RemoteJobID == nil || *firstCall.RemoteRequestID != formalGenerationRemoteRequestID ||
		*firstCall.RemoteJobID != formalGenerationRemoteJobID || firstCall.QueryDeadlineAt == nil ||
		firstCall.RemoteExpiresAt == nil || !firstCall.QueryDeadlineAt.Before(*firstCall.RemoteExpiresAt) ||
		firstCall.DispatchBoundaryEnteredAt == nil || !firstCall.DispatchBoundaryEnteredAt.Before(*firstCall.QueryDeadlineAt) {
		t.Fatalf("first formal Generation Call lost its remote identity: %#v", firstCall)
	}
	for _, call := range facts.calls[1:] {
		if call.Status != generationdomain.ProviderCallPending || call.RemoteRequestID != nil || call.RemoteJobID != nil ||
			call.DispatchBoundaryEnteredAt != nil {
			t.Fatalf("first formal Generation activity crossed a second remote boundary: %#v", call)
		}
	}
	if facts.costReservation.Status != costdomain.ReservationReserved ||
		facts.quotaReservation.Status != quotadomain.ReservationReserved {
		t.Fatalf("first formal Generation activity changed reservations: cost=%#v quota=%#v",
			facts.costReservation, facts.quotaReservation)
	}
}

func assertFormalGenerationUnknown(t *testing.T, first formalGenerationFacts, final formalGenerationFacts) {
	t.Helper()
	var failure map[string]string
	if err := json.Unmarshal(final.run.Error, &failure); err != nil {
		t.Fatalf("decode formal Generation attention projection: %v", err)
	}
	if final.run.Status != "NEEDS_ATTENTION" || final.run.NextAction == nil ||
		*final.run.NextAction != workflowdomain.ManualProviderReconciliationNextAction ||
		failure["code"] != workflowdomain.ProviderOutcomeUnknownErrorCode ||
		failure["node_id"] != formalGenerationNodeID || final.node.Status != "FAILED" ||
		final.node.Attempt != 2 || final.node.ActiveClaimToken != nil || len(final.node.Output) != 0 ||
		final.node.OutputHash != nil {
		t.Fatalf("formal Generation attention projections drifted: run=%#v node=%#v failure=%#v",
			final.run, final.node, failure)
	}
	if final.intent.Status != generationdomain.IntentOutcomeUnknown ||
		final.job.Status != generationdomain.ProviderJobOutcomeUnknown || len(final.calls) != len(first.calls) ||
		final.intent.ID != first.intent.ID || final.request.ID != first.request.ID || final.job.ID != first.job.ID ||
		final.intent.GenerationRequestID == nil || *final.intent.GenerationRequestID != first.request.ID ||
		final.intent.ProviderJobID == nil || *final.intent.ProviderJobID != first.job.ID {
		t.Fatalf("formal Generation OUTCOME_UNKNOWN aggregate drifted: first=%#v final=%#v", first, final)
	}
	for index := range final.calls {
		if final.calls[index].ID != first.calls[index].ID {
			t.Fatalf("formal Generation Call identity changed at %d: first=%#v final=%#v",
				index, first.calls[index], final.calls[index])
		}
	}
	unknownCall := final.calls[0]
	if unknownCall.Status != generationdomain.ProviderCallOutcomeUnknown || unknownCall.RemoteRequestID == nil ||
		unknownCall.RemoteJobID == nil || *unknownCall.RemoteRequestID != formalGenerationRemoteRequestID ||
		*unknownCall.RemoteJobID != formalGenerationRemoteJobID || unknownCall.QueryDeadlineAt == nil ||
		unknownCall.RemoteExpiresAt == nil || !unknownCall.QueryDeadlineAt.Equal(*first.calls[0].QueryDeadlineAt) ||
		!unknownCall.RemoteExpiresAt.Equal(*first.calls[0].RemoteExpiresAt) {
		t.Fatalf("formal Generation OUTCOME_UNKNOWN lost the original remote task: %#v", unknownCall)
	}
	for _, call := range final.calls[1:] {
		if call.Status != generationdomain.ProviderCallPending || call.DispatchBoundaryEnteredAt != nil {
			t.Fatalf("formal Generation recovery crossed a second dispatch boundary: %#v", call)
		}
	}
	if final.costReservation.ID != first.costReservation.ID || final.quotaReservation.ID != first.quotaReservation.ID ||
		final.costReservation.Status != costdomain.ReservationReserved ||
		final.quotaReservation.Status != quotadomain.ReservationReserved || final.providerReceiptCount != 0 ||
		final.intent.CostSettlementReceiptID != nil || final.intent.CostReleaseReceiptID != nil ||
		final.intent.QuotaConsumptionReceiptID != nil || final.intent.QuotaReleaseReceiptID != nil {
		t.Fatalf("formal Generation OUTCOME_UNKNOWN settled or released reservations: cost=%#v quota=%#v",
			final.costReservation, final.quotaReservation)
	}
	for operation, count := range final.terminalReceiptCounts {
		if count != 0 {
			t.Fatalf("formal Generation OUTCOME_UNKNOWN wrote %d %s receipts", count, operation)
		}
	}
}

func assertFormalGenerationCancellationFenced(
	t *testing.T,
	ctx context.Context,
	database *generationtestgorm.Database,
	fixture formalGenerationFixture,
	facts formalGenerationFacts,
) {
	t.Helper()
	now := time.Now().UTC()
	controller := &formalGenerationForbiddenController{}
	controls := workflowapp.NewControlService(
		workflowgorm.New(database), controller,
		workflowapp.ControlConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	_, workflowCancelErr := controls.Cancel(ctx, workflowapp.Actor{
		UserID: fixture.project.userID.String(), TokenVersion: 1,
	}, workflowapp.CancelCommand{
		WorkspaceID: fixture.project.workspaceID.String(), WorkflowRunID: fixture.workflowRunID,
		ExpectedRevision: facts.run.Revision,
		IdempotencyKey:   "formal-generation-workflow-cancel-after-unknown-" + facts.intent.ID.String(),
	})
	var workflowError *workflowapp.Error
	if !errors.As(workflowCancelErr, &workflowError) || workflowError.Code != "resource_conflict" || workflowError.Status != 409 {
		t.Fatalf("formal Provider attention Workflow was cancellable: %T %v", workflowCancelErr, workflowCancelErr)
	}
	if controller.calls != 0 {
		t.Fatalf("rejected formal Provider attention cancel reached Temporal %d times", controller.calls)
	}
	var controlIntentCount int64
	if err := database.Model(&model.WorkflowControlIntent{}).Where("workflow_run_id = ?", fixture.workflowRunID).
		Count(&controlIntentCount).Error; err != nil || controlIntentCount != 0 {
		t.Fatalf("rejected formal Provider attention cancel wrote ControlIntent: count=%d err=%v", controlIntentCount, err)
	}

	costConfig := costapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}
	quotaConfig := quotaapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}
	preparations := generationapp.NewPreparationService(
		generationgorm.NewPreparationStore(database, costConfig, quotaConfig),
		generationapp.PreparationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString, ClaimTTL: 5 * time.Minute},
	)
	_, err := preparations.CancelPreparedIntent(ctx, generationapp.Actor{
		UserID: fixture.project.userID.String(), TokenVersion: 1,
	}, generationapp.CancelPreparedIntentCommand{
		IntentID: facts.intent.ID.String(), IdempotencyKey: "formal-generation-cancel-after-unknown-" + facts.intent.ID.String(),
	})
	if !generationapp.IsCode(err, "state_conflict") {
		t.Fatalf("formal Generation OUTCOME_UNKNOWN was cancellable: %v", err)
	}
}

func assertFormalGenerationAudit(t *testing.T, auditPath string, facts formalGenerationFacts) {
	t.Helper()
	file, err := os.Open(auditPath)
	if err != nil {
		t.Fatalf("open formal Generation boundary audit: %v", err)
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(file)
	events := make([]formalGenerationAuditEvent, 0, 3)
	for {
		var event formalGenerationAuditEvent
		if err = decoder.Decode(&event); errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode formal Generation boundary audit: %v", err)
		}
		events = append(events, event)
	}
	counts := make(map[string]int)
	for _, event := range events {
		counts[event.Operation]++
	}
	if counts["preflight"] != 1 || counts["submit"] != 1 || counts["query"] != 1 || len(events) != 3 {
		t.Fatalf("formal Generation remote boundaries = %#v, want preflight=1 submit=1 query=1", events)
	}
	for _, event := range events {
		if event.ProviderJobID != facts.job.ID.String() || event.ProviderCallID != facts.calls[0].ID.String() {
			t.Fatalf("formal Generation boundary changed Provider identity: %#v", event)
		}
		if event.Operation != "preflight" &&
			(event.RemoteRequestID != formalGenerationRemoteRequestID || event.RemoteJobID != formalGenerationRemoteJobID) {
			t.Fatalf("formal Generation boundary lost remote identity: %#v", event)
		}
	}
}

func assertFormalGenerationHistory(
	t *testing.T,
	ctx context.Context,
	temporalClient client.Client,
	fixture formalGenerationFixture,
	firstActivityID string,
) {
	t.Helper()
	history, signals, starts, completions := loadRecoveredWorkflowHistory(t, ctx, temporalClient, fixture.workflowID)
	description, err := temporalClient.DescribeWorkflowExecution(ctx, fixture.workflowID, "")
	if err != nil || description.GetWorkflowExecutionInfo().GetStatus() != enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED {
		t.Fatalf("formal Generation Temporal execution is not COMPLETED: description=%#v err=%v", description, err)
	}
	if signals != 0 {
		t.Fatalf("formal Generation recovery received %d Human Gate signals, want 0", signals)
	}
	wantActivities := []string{
		"load-execution-plan", "execute-node:" + fixture.sourceNodeRunID,
		firstActivityID, firstActivityID + ":poll:1",
	}
	if len(starts) != len(wantActivities) || len(completions) != len(wantActivities) {
		t.Fatalf("formal Generation activity sets: starts=%#v completions=%#v", starts, completions)
	}
	for _, activityID := range wantActivities {
		if starts[activityID] != 1 || completions[activityID] != 1 {
			t.Fatalf("formal Generation activity %q counts = %d/%d, want 1/1",
				activityID, starts[activityID], completions[activityID])
		}
	}
	if starts["complete-run"] != 0 || completions["complete-run"] != 0 ||
		starts["fail-run:"+fixture.nodeRunID] != 0 || completions["fail-run:"+fixture.nodeRunID] != 0 {
		t.Fatalf("formal Generation attention called a terminal projector: starts=%#v completions=%#v", starts, completions)
	}
	controlSignals, cancellations, terminations := 0, 0, 0
	for _, event := range history.Events {
		if attributes := event.GetWorkflowExecutionSignaledEventAttributes(); attributes != nil &&
			attributes.GetSignalName() == temporaladapter.WorkflowControlSignalName {
			controlSignals++
		}
		if event.GetWorkflowExecutionCanceledEventAttributes() != nil {
			cancellations++
		}
		if event.GetWorkflowExecutionTerminatedEventAttributes() != nil {
			terminations++
		}
	}
	if controlSignals != 0 || cancellations != 0 || terminations != 0 {
		t.Fatalf("formal Generation attention history contains cancel control: signals=%d cancelled=%d terminated=%d",
			controlSignals, cancellations, terminations)
	}
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(
		temporaladapter.EpisodeProductionWorkflow,
		temporalworkflow.RegisterOptions{Name: temporaladapter.EpisodeProductionWorkflowName},
	)
	if err := replayer.ReplayWorkflowHistory(nil, history); err != nil {
		t.Fatalf("replay formal Generation production Workflow history: %v", err)
	}
}

type formalGenerationTargetBuilder struct {
	store    *generationgorm.TargetStore
	targetID string
}

func (builder *formalGenerationTargetBuilder) BuildReferenceTargets(
	ctx context.Context,
	actor generationapp.Actor,
	command generationapp.BuildReferenceTargetsCommand,
) (generationapp.BuildReferenceTargetsResult, error) {
	if builder == nil || builder.store == nil || actor.TokenVersion < 1 {
		return generationapp.BuildReferenceTargetsResult{}, errors.New("formal Generation target builder is unavailable")
	}
	target, err := builder.store.Find(ctx, builder.targetID)
	if err != nil {
		return generationapp.BuildReferenceTargetsResult{}, err
	}
	if target.CreatedBy != actor.UserID || target.SourceOwnerRef.ID != command.ApprovedIntentSetID ||
		target.SourceOwnerRef.ContentHash != command.ExpectedContentHash {
		return generationapp.BuildReferenceTargetsResult{}, errors.New("formal Generation target source drifted")
	}
	return generationapp.BuildReferenceTargetsResult{Targets: []generationdomain.GenerationTarget{target}}, nil
}

type formalGenerationForbiddenMaterializer struct{}

func (formalGenerationForbiddenMaterializer) MaterializeSucceededOutputs(
	context.Context,
	generationapp.Actor,
	generationapp.MaterializeProviderOutputsCommand,
) (generationapp.OutputMaterializationResult, error) {
	return generationapp.OutputMaterializationResult{}, errors.New("formal OUTCOME_UNKNOWN recovery reached materialization")
}

type formalGenerationForbiddenController struct{ calls int }

func (controller *formalGenerationForbiddenController) Control(
	context.Context,
	workflowdomain.ControlRequest,
) (workflowdomain.ControlObservation, error) {
	controller.calls++
	return workflowdomain.ControlObservation{}, errors.New("rejected formal Provider attention cancel reached Temporal")
}

type formalGenerationQueryFailure struct{}

func (formalGenerationQueryFailure) Error() string { return "formal remote identity is unrecoverable" }

func (formalGenerationQueryFailure) ProviderQueryFailureKind() string {
	return generationapp.ProviderQueryFailureIdentityUnrecoverable
}

type formalGenerationGateway struct {
	mode      string
	auditPath string
}

func (gateway *formalGenerationGateway) Preflight(
	_ context.Context,
	submission generationapp.ProviderSubmission,
) error {
	if err := gateway.appendAudit(formalGenerationAuditEvent{
		Operation: "preflight", ProviderJobID: submission.ProviderJobID, ProviderCallID: submission.ProviderCallID,
	}); err != nil {
		return err
	}
	if gateway.mode != formalGenerationWorkerModeSubmit {
		return errors.New("formal Generation recovery attempted a second preflight")
	}
	return nil
}

func (gateway *formalGenerationGateway) Submit(
	_ context.Context,
	submission generationapp.ProviderSubmission,
) (generationapp.ProviderOutcome, error) {
	if err := gateway.appendAudit(formalGenerationAuditEvent{
		Operation: "submit", ProviderJobID: submission.ProviderJobID, ProviderCallID: submission.ProviderCallID,
		RemoteRequestID: formalGenerationRemoteRequestID, RemoteJobID: formalGenerationRemoteJobID,
	}); err != nil {
		return generationapp.ProviderOutcome{}, err
	}
	if gateway.mode != formalGenerationWorkerModeSubmit {
		return generationapp.ProviderOutcome{}, errors.New("formal Generation recovery attempted a second Submit")
	}
	now := time.Now().UTC()
	return generationapp.ProviderOutcome{
		Status: generationapp.ProviderOutcomeAccepted, RemoteRequestID: formalGenerationRemoteRequestID,
		RemoteJobID: formalGenerationRemoteJobID, QueryDeadlineAt: now.Add(2 * time.Hour),
		RemoteExpiresAt: now.Add(26 * time.Hour),
	}, nil
}

func (gateway *formalGenerationGateway) Query(
	_ context.Context,
	submission generationapp.ProviderSubmission,
) (generationapp.ProviderOutcome, error) {
	if err := gateway.appendAudit(formalGenerationAuditEvent{
		Operation: "query", ProviderJobID: submission.ProviderJobID, ProviderCallID: submission.ProviderCallID,
		RemoteRequestID: submission.RemoteRequestID, RemoteJobID: submission.RemoteJobID,
	}); err != nil {
		return generationapp.ProviderOutcome{}, err
	}
	if gateway.mode != formalGenerationWorkerModeReconcile ||
		submission.RemoteRequestID != formalGenerationRemoteRequestID ||
		submission.RemoteJobID != formalGenerationRemoteJobID || submission.QueryDeadlineAt == nil ||
		submission.RemoteExpiresAt == nil || !submission.QueryDeadlineAt.Before(*submission.RemoteExpiresAt) {
		return generationapp.ProviderOutcome{}, errors.New("formal Generation Query did not recover the persisted remote identity")
	}
	return generationapp.ProviderOutcome{}, formalGenerationQueryFailure{}
}

func (gateway *formalGenerationGateway) appendAudit(event formalGenerationAuditEvent) error {
	file, err := os.OpenFile(gateway.auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encodeErr := json.NewEncoder(file).Encode(event)
	return errors.Join(encodeErr, file.Close())
}

func TestFormalGenerationTemporalWorkerProcessHelper(t *testing.T) {
	if os.Getenv(formalGenerationWorkerHelperFlag) != "1" {
		t.Skip("subprocess helper")
	}
	database, err := platformdatabase.Open(context.Background(), os.Getenv("LANVERSE_TEST_DATABASE_URL"), io.Discard)
	if err != nil {
		t.Fatalf("open formal Generation helper database: %v", err)
	}
	defer func() { _ = platformdatabase.Close(database) }()
	temporalRuntime, err := temporaladapter.New(temporaladapter.Config{
		Address: os.Getenv(formalGenerationWorkerAddress), Namespace: "default",
		TaskQueue: os.Getenv(formalGenerationWorkerTaskQueue),
	})
	if err != nil {
		t.Fatalf("connect formal Generation helper Temporal runtime: %v", err)
	}
	defer temporalRuntime.Close()
	now := func() time.Time { return time.Now().UTC() }
	costConfig := costapp.Config{Now: now, NewID: uuid.NewString}
	quotaConfig := quotaapp.Config{Now: now, NewID: uuid.NewString}
	preparations := generationapp.NewPreparationService(
		generationgorm.NewPreparationStore(database, costConfig, quotaConfig),
		generationapp.PreparationConfig{Now: now, NewID: uuid.NewString, ClaimTTL: 5 * time.Minute},
	)
	providers := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig),
		&formalGenerationGateway{
			mode: os.Getenv(formalGenerationWorkerMode), auditPath: os.Getenv(formalGenerationWorkerAuditPath),
		},
		generationapp.ProviderConfig{Now: now, NewID: uuid.NewString},
	)
	generationExecutor := workflowgeneration.NewNodeExecutor(
		nil,
		&formalGenerationTargetBuilder{
			store: generationgorm.NewTargetStore(database), targetID: os.Getenv(formalGenerationWorkerTargetID),
		},
		preparations,
		preparations,
		providers,
		formalGenerationForbiddenMaterializer{},
	)
	runtimeService := workflowapp.NewRuntimeService(workflowgorm.New(database), workflowapp.RuntimeConfig{
		Now: now, NewID: uuid.NewString, Executor: generationExecutor,
	})
	runtimeWorker, err := temporalRuntime.NewWorker(runtimeService)
	if err != nil {
		t.Fatalf("create formal Generation Temporal worker: %v", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		t.Fatalf("start formal Generation Temporal worker: %v", err)
	}
	defer runtimeWorker.Stop()
	select {}
}

func startFormalGenerationWorkerProcess(
	t *testing.T,
	databaseURL string,
	temporalAddress string,
	taskQueue string,
	mode string,
	auditPath string,
	targetID string,
) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	output := &bytes.Buffer{}
	command := exec.Command(os.Args[0], "-test.run=^TestFormalGenerationTemporalWorkerProcessHelper$", "-test.v")
	command.Env = append(os.Environ(),
		formalGenerationWorkerHelperFlag+"=1",
		"LANVERSE_TEST_DATABASE_URL="+databaseURL,
		formalGenerationWorkerAddress+"="+temporalAddress,
		formalGenerationWorkerTaskQueue+"="+taskQueue,
		formalGenerationWorkerMode+"="+mode,
		formalGenerationWorkerAuditPath+"="+auditPath,
		formalGenerationWorkerTargetID+"="+targetID,
	)
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		t.Fatalf("start formal Generation Temporal worker subprocess: %v", err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil && command.Process != nil {
			_ = command.Process.Kill()
			_, _ = command.Process.Wait()
		}
	})
	return command, output
}

func mustTemporalClient(t *testing.T, address string) client.Client {
	t.Helper()
	temporalClient, err := client.Dial(client.Options{HostPort: address, Namespace: "default"})
	if err != nil {
		t.Fatalf("connect formal Generation Temporal history client: %v", err)
	}
	t.Cleanup(temporalClient.Close)
	return temporalClient
}
