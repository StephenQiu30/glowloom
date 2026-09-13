package workflow_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	temporaladapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	flow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"
)

func assertVisionReviewWorkflow(t *testing.T, ctx context.Context, db *generationtestgorm.Database, actor genapp.Actor, execution gen.ReferenceExecution, bundles gen.ReferenceBundleInputCollection, objects *referenceExecutionHTTPObjects) {
	t.Helper()
	index := -1
	for i, bundle := range bundles.Bundles {
		if bundle.Admission.InternalReviewReady {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("Vision Review requires a complete fixture group")
	}
	for _, outcome := range []string{"accepted", "outcome_unknown", "result_commit"} {
		t.Run("vision_workflow_"+outcome, func(t *testing.T) {
			store := generationgorm.New(db)
			reader, err := genapp.NewVisionReviewMediaReader(store, objects, gen.ReferenceObjectStoreRef{Profile: "minio", Bucket: "lanverse"})
			if err != nil {
				t.Fatal(err)
			}
			persistence, err := agentgorm.NewVisionReviewStore(db, generationgorm.ValidateCurrentVisionReviewInput)
			if err != nil {
				t.Fatal(err)
			}
			signer, err := grant.NewSigner("synthetic-vision-workflow-test-not-a-credential", time.Now)
			if err != nil {
				t.Fatal(err)
			}
			invoker := &persistedVisionInvoker{db: db, unknown: outcome == "outcome_unknown"}
			var offset atomic.Int64
			now := func() time.Time { return time.Now().Add(time.Duration(offset.Load())) }
			var transactions agentapp.VisionReviewTransactions = persistence
			failedCommit := &visionResultRollback{VisionReviewTransactions: persistence, db: db, offset: &offset}
			if outcome == "result_commit" {
				transactions = failedCommit
			}
			image := "sha256:" + strings.Repeat("8", 64)
			service, err := agentapp.NewVisionReviewExecutionService(transactions, reader, invoker, signer, agentapp.VisionReviewExecutionConfig{Now: now, NewID: uuid.NewString, AgentImageDigest: image})
			if err != nil {
				t.Fatal(err)
			}
			release, err := agentapp.BuildStageReleaseRecord(contract.VisionReviewStageKey, image, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			executor := workflowproduction.NewNodeExecutor(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				workflowproduction.SceneAnalysisDependencies{VisionReview: &workflowproduction.VisionReviewDependencies{Inputs: store, Execution: service, StageReleaseHash: release.Identity.StageReleaseHash}})
			replyLoss := &referenceCallNodeReplyLoss{NodeExecutor: executor}
			workflowStore := workflowgorm.New(db)
			activities := &referenceCallTemporalActivities{RuntimeService: workflowapp.NewRuntimeService(workflowStore, workflowapp.RuntimeConfig{Now: time.Now, NewID: uuid.NewString, Executor: replyLoss}), dropResponse: true}
			runtime, err := temporaladapter.New(temporaladapter.Config{Address: os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS"), Namespace: "default", TaskQueue: "lanverse-vision-review-" + uuid.NewString()})
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Close()
			w, err := runtime.NewWorker(activities)
			if err != nil {
				t.Fatal(err)
			}
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			defer w.Stop()
			catalog, err := authoring.SystemCatalog()
			if err != nil {
				t.Fatal(err)
			}
			authorStore := authoringgorm.New(db)
			if _, err := authorStore.EnsureCatalog(ctx, catalog, time.Now(), uuid.NewString); err != nil {
				t.Fatal(err)
			}
			authors := authoringapp.NewService(authorStore, authoringapp.Config{Now: time.Now, NewID: uuid.NewString})
			config, _ := json.Marshal(map[string]any{"execution_ref": bundles.ExecutionRef, "bundle_index": index})
			key := "vision-review-" + outcome
			author := authoringapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}
			draft, err := authors.Create(ctx, author, authoringapp.CreateCommand{ProjectID: execution.ProjectID, AuthoringMode: "GUIDED",
				Graph: authoring.Graph{Nodes: []authoring.Node{{ID: "vision-review", DefinitionKey: "agent.vision_review", DefinitionVersion: "1.0.0", Config: config}}}, Layout: json.RawMessage(`{"guided":{"step":1}}`),
				FrozenInputs: []authoring.FrozenReference{{Kind: "reference_execution", ID: execution.ID, Version: "1", Hash: execution.ContentHash}}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: key})
			if err != nil {
				t.Fatal(err)
			}
			published, err := authors.Publish(ctx, author, authoringapp.PublishCommand{DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: key + "-publish"})
			if err != nil {
				t.Fatal(err)
			}
			compiler := workflowapp.NewService(workflowauthoring.New(authors), workflowStore, workflowapp.Config{Now: time.Now, NewID: uuid.NewString})
			starter := workflowapp.NewStartService(compiler, workflowStore, runtime, workflowapp.StartConfig{Now: time.Now, NewID: uuid.NewString})
			run, err := starter.Start(ctx, workflowapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, workflowapp.StartCommand{AuthoringRevisionID: published.ID, IdempotencyKey: key + "-start"})
			if err != nil {
				t.Fatal(err)
			}
			historyClient, err := client.Dial(client.Options{HostPort: os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS"), Namespace: "default"})
			if err != nil {
				t.Fatal(err)
			}
			defer historyClient.Close()
			finished := false
			defer func() {
				if !finished {
					cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = historyClient.CancelWorkflow(cleanup, run.TemporalWorkflowID, "")
				}
			}()
			var result temporaladapter.RunResult
			if err := historyClient.GetWorkflow(ctx, run.TemporalWorkflowID, "").Get(ctx, &result); err != nil {
				t.Fatal(err)
			}
			finished = true
			want := "SUCCEEDED"
			if outcome != "accepted" {
				want = flow.NodeActivityNeedsAttention
			}
			if result.Status != want || invoker.calls.Load() != 1 {
				t.Fatalf("Vision Workflow outcome=%s calls=%d", result.Status, invoker.calls.Load())
			}
			var node model.NodeRunProjection
			var invocation model.SceneAnalysisInvocationRecord
			if err := db.WithContext(ctx).Where("workflow_run_id = ?", run.ID).First(&node).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.WithContext(ctx).Where("workflow_run_id = ? AND node_run_id = ?", run.ID, node.ID).First(&invocation).Error; err != nil {
				t.Fatal(err)
			}
			expectedOutcome := outcome
			if outcome == "result_commit" {
				expectedOutcome = "outcome_unknown"
				if !failedCommit.verified.Load() {
					t.Fatal("Candidate and Result rollback was not verified")
				}
			}
			if invocation.Status != expectedOutcome {
				t.Fatal("Vision result did not persist")
			}
			var attempts int64
			if err := db.WithContext(ctx).Model(&model.SceneAnalysisAttempt{}).Where("invocation_id = ?", invocation.ID).Count(&attempts).Error; err != nil || attempts != 1 {
				t.Fatalf("Vision Review created multiple attempts: %v", err)
			}
			if outcome == "accepted" {
				if !replyLoss.lost.Load() || !activities.lost.Load() || activities.activityCalls.Load() != 3 {
					t.Fatal("Vision Review did not exercise both lost commit responses")
				}
				input, err := store.CompileBaseVisionReviewInput(ctx, actor, execution.ProjectID, execution.ID, index, release.Identity.StageReleaseHash)
				if err != nil {
					t.Fatal(err)
				}
				command := agentapp.ExecuteVisionReviewCommand{WorkflowRunID: run.ID, NodeRunID: node.ID.String(), UserID: actor.UserID, TokenVersion: actor.TokenVersion, Input: input}
				state, err := service.Execute(ctx, command)
				if err != nil || state.Candidate.ID == "" || invoker.calls.Load() != 1 {
					t.Fatalf("durable Vision Candidate replay: %v", err)
				}
				output, _, _, err := flow.ParseNodeOutput(json.RawMessage(node.Output))
				if err != nil || len(output.Bindings) != 1 || output.Bindings[0].ReferenceID != state.Candidate.ID || output.Bindings[0].ContentHash != state.Candidate.CandidateRevisionHash {
					t.Fatal("Workflow did not output its persisted Candidate")
				}
				// Invalid current authorization fails even when a Candidate is cached.
				command.TokenVersion++
				if _, err := service.Execute(ctx, command); err == nil {
					t.Fatal("Vision replay ignored revoked actor")
				}
				command.TokenVersion--
				// Control is checked in the result transaction, not only at dispatch.
				var control model.SceneAnalysisControlHead
				if err := db.WithContext(ctx).First(&control, "release_id = ?", release.ID).Error; err != nil {
					t.Fatal(err)
				}
				originalStatus := control.Status
				if err := db.WithContext(ctx).Model(&control).UpdateColumn("status", "revoked").Error; err != nil {
					t.Fatal(err)
				}
				blocked := persistence.WithinVisionReviewTransaction(ctx, func(repo agentapp.VisionReviewRepository) error {
					_, err := repo.CompleteVisionReviewExecution(ctx, agentapp.VisionReviewResultAcceptance{Command: command, Record: state.Record, Result: *state.Result, AcceptedAt: time.Now()})
					return err
				})
				if err := db.WithContext(ctx).Model(&control).UpdateColumn("status", originalStatus).Error; err != nil {
					t.Fatal(err)
				}
				if blocked == nil {
					t.Fatal("revoked Control accepted existing result")
				}
				assertVisionReviewPersistedIntegrity(t, ctx, db, service, command, state)
			}
			history, _, _, _ := loadRecoveredWorkflowHistory(t, ctx, historyClient, run.TemporalWorkflowID)
			for _, event := range history.Events {
				encoded := event.String()
				for _, forbidden := range []string{"staging/reference/", "visual_grammar", "dispatch_authorization", "iVBORw0", "fidelity_invariants"} {
					if strings.Contains(encoded, forbidden) {
						t.Fatalf("Vision Review History leaked %s", forbidden)
					}
				}
			}
			replayer := worker.NewWorkflowReplayer()
			replayer.RegisterWorkflowWithOptions(temporaladapter.EpisodeProductionWorkflow, temporalworkflow.RegisterOptions{Name: temporaladapter.EpisodeProductionWorkflowName})
			if err := replayer.ReplayWorkflowHistory(nil, history); err != nil {
				t.Fatalf("Vision Review Replay: %v", err)
			}
		})
	}
}

func assertVisionReviewPersistedIntegrity(t *testing.T, ctx context.Context, db *generationtestgorm.Database, service *agentapp.VisionReviewExecutionService, command agentapp.ExecuteVisionReviewCommand, state agentapp.VisionReviewExecutionState) {
	t.Helper()
	var invocation model.SceneAnalysisInvocationRecord
	var manifest model.ShardManifest
	var candidate model.SceneAnalysisCandidateRevision
	if err := db.First(&invocation, "id = ?", state.Record.Invocation.InvocationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&manifest, "id = ?", state.Record.Manifest.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&candidate, "id = ?", state.Candidate.ID).Error; err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(invocation.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	var stage map[string]json.RawMessage
	var subject map[string]json.RawMessage
	if json.Unmarshal(payload["stage_input"], &stage) != nil || json.Unmarshal(stage["subject"], &subject) != nil {
		t.Fatal("invalid stored subject")
	}
	delete(subject, "candidate_bundle_index")
	stage["subject"], _ = json.Marshal(subject)
	payload["stage_input"], _ = json.Marshal(stage)
	missing, _ := json.Marshal(payload)
	for _, fault := range []struct {
		name              string
		target            any
		id                string
		column            string
		original, mutated any
	}{
		{"candidate_hash", &model.SceneAnalysisCandidateRevision{}, candidate.ID.String(), "candidate_revision_hash", candidate.CandidateRevisionHash, strings.Repeat("d", 64)},
		{"candidate_content", &model.SceneAnalysisCandidateRevision{}, candidate.ID.String(), "candidate", candidate.Candidate, json.RawMessage(`{}`)},
		{"manifest_content", &model.ShardManifest{}, manifest.ID.String(), "shards", manifest.Shards, json.RawMessage(`[]`)},
		{"missing_field", &model.SceneAnalysisInvocationRecord{}, invocation.ID.String(), "payload", invocation.Payload, json.RawMessage(missing)},
	} {
		t.Run("vision_persisted_"+fault.name, func(t *testing.T) {
			defer func() {
				if err := db.Model(fault.target).Where("id = ?", fault.id).UpdateColumn(fault.column, fault.original).Error; err != nil {
					t.Error(err)
				}
			}()
			if err := db.WithContext(ctx).Model(fault.target).Where("id = ?", fault.id).UpdateColumn(fault.column, fault.mutated).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := service.Execute(ctx, command); err == nil {
				t.Fatal("Vision Review replay accepted corrupted persisted facts")
			}
		})
	}
}

type persistedVisionInvoker struct {
	db      *generationtestgorm.Database
	calls   atomic.Int64
	unknown bool
}

type visionResultRollback struct {
	agentapp.VisionReviewTransactions
	db               *generationtestgorm.Database
	offset           *atomic.Int64
	failed, verified atomic.Bool
}
type visionResultRollbackRepository struct {
	agentapp.VisionReviewRepository
	owner      *visionResultRollback
	rolledBack *agentapp.VisionReviewExecutionState
}

func (r *visionResultRollbackRepository) CompleteVisionReviewExecution(ctx context.Context, value agentapp.VisionReviewResultAcceptance) (agentapp.VisionReviewExecutionState, error) {
	state, err := r.VisionReviewRepository.CompleteVisionReviewExecution(ctx, value)
	if err == nil && state.Status == "accepted" && r.owner.failed.CompareAndSwap(false, true) {
		*r.rolledBack = state
		return agentapp.VisionReviewExecutionState{}, errors.New("injected failure after Result/Candidate/Head writes")
	}
	return state, err
}
func (s *visionResultRollback) WithinVisionReviewTransaction(ctx context.Context, operation func(agentapp.VisionReviewRepository) error) error {
	var state agentapp.VisionReviewExecutionState
	err := s.VisionReviewTransactions.WithinVisionReviewTransaction(ctx, func(repo agentapp.VisionReviewRepository) error {
		return operation(&visionResultRollbackRepository{VisionReviewRepository: repo, owner: s, rolledBack: &state})
	})
	if state.Status == "accepted" {
		for _, q := range []struct {
			model any
			key   string
			value any
		}{
			{&model.SceneAnalysisResult{}, "attempt_id", state.Record.Invocation.AttemptID},
			{&model.SceneAnalysisCandidateRevision{}, "source_invocation_id", state.Record.Invocation.InvocationID},
			{&model.SceneAnalysisCandidateHead{}, "stage_instance_key", state.Record.Invocation.StageInstanceKey()},
		} {
			var count int64
			if checkErr := s.db.WithContext(ctx).Model(q.model).Where(q.key+" = ?", q.value).Count(&count).Error; checkErr != nil || count != 0 {
				return errors.New("Vision rollback left partial accepted facts")
			}
		}
		s.verified.Store(true)
		s.offset.Store(int64(3 * time.Minute))
	}
	return err
}

func (i *persistedVisionInvoker) InvokeVisionReview(ctx context.Context, invocation contract.VisionReviewInvocation, auth contract.SceneAnalysisDispatchAuthorization, images [][]byte) (contract.VisionReviewAttemptResult, error) {
	i.calls.Add(1)
	check, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var persisted model.SceneAnalysisDispatchAuthorization
	if err := i.db.WithContext(check).First(&persisted, "attempt_id = ?", invocation.AttemptID).Error; err != nil || persisted.AuthorizationHash != auth.Hash {
		return contract.VisionReviewAttemptResult{}, errors.New("Vision Review called before authorization committed")
	}
	// This write from another connection must not wait on a dispatch transaction.
	if err := i.db.WithContext(check).Model(&model.SceneAnalysisInvocationRecord{}).Where("id = ? AND status = ?", invocation.InvocationID, "running").UpdateColumn("updated_at", time.Now()).Error; err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	if len(images) != len(invocation.Payload.StageInput.Attachments) {
		return contract.VisionReviewAttemptResult{}, errors.New("incomplete image group")
	}
	for n, image := range images {
		digest := sha256.Sum256(image)
		if hex.EncodeToString(digest[:]) != invocation.Payload.StageInput.Attachments[n].Slot.SHA256 {
			return contract.VisionReviewAttemptResult{}, errors.New("wrong image bytes")
		}
	}
	if i.unknown {
		return contract.VisionReviewAttemptResult{}, errors.New("injected model response loss")
	}
	candidate := contract.VisionReviewCandidate{ContractID: contract.VisionReviewCandidateContractID, Subject: invocation.Payload.StageInput.Subject}
	for _, category := range []string{"identity", "interaction_geometry", "state", "style_fidelity", "view_role"} {
		check := contract.VisionReviewCheck{Category: category, Status: "pass", IssueCode: "none", ConfidenceBPS: 9000, Summary: "受控视觉运行夹具，不代表真实模型判断", Evidence: []contract.VisionReviewEvidence{}}
		for _, slot := range candidate.Subject.Slots {
			check.Evidence = append(check.Evidence, contract.VisionReviewEvidence{SlotKey: slot.SlotKey, Region: contract.VisionReviewRegion{Width: 10000, Height: 10000}})
		}
		candidate.Checks = append(candidate.Checks, check)
	}
	raw, err := json.Marshal(candidate)
	if err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	hash, err := contract.ProductionCanonicalHash(raw)
	if err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	diagnosticHash, _ := contract.ProductionCanonicalHash(json.RawMessage(`[]`))
	result := contract.VisionReviewAttemptResult{InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID, Kind: invocation.Kind, WireSchemaVersion: invocation.WireSchemaVersion,
		Variant: invocation.Payload.Variant, StageRelease: invocation.StageRelease, Control: invocation.Control, ClaimVersion: 1, DispatchAuthorizationHash: auth.Hash,
		Status: "accepted", CandidateType: "vision_review_candidate", Candidate: raw, InputHash: invocation.InputHash, OutputHash: &hash, Diagnostics: []contract.SceneAnalysisDiagnostic{}, DiagnosticHash: diagnosticHash,
		CompletedAt: time.Now().UTC().Truncate(time.Microsecond), Executor: contract.VisionReviewExecutor{RuntimeClass: "vision", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest, HarnessVersion: "vision-review-harness", Model: "controlled-test-model"}}
	result.ResultHash, err = result.ComputeResultHash()
	if err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	return result, result.ValidateFor(invocation, 1, auth.Hash)
}
