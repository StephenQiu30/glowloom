package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	storyboarddomain "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
	workflowexecution "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/execution"
	workflowgeneration "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/generation"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type shotWorkflowOwner struct {
	shot    storyboarddomain.Shot
	actor   storyboardapp.Actor
	shotID  string
	loadErr error
}

func (owner *shotWorkflowOwner) RequireActiveShot(
	_ context.Context,
	actor storyboardapp.Actor,
	shotID string,
) (storyboarddomain.Shot, error) {
	owner.actor, owner.shotID = actor, shotID
	return owner.shot, owner.loadErr
}

func (owner *shotWorkflowOwner) RequireShotImageBindingTarget(
	_ context.Context,
	actor storyboardapp.Actor,
	shotID string,
) (storyboardapp.ShotImageBindingTarget, error) {
	owner.actor, owner.shotID = actor, shotID
	return storyboardapp.ShotImageBindingTarget{
		Shot: owner.shot, ExpectedCurrentRevision: 0, ContentHash: strings.Repeat("f", 64),
	}, owner.loadErr
}

func (*shotWorkflowOwner) BindSelectedImage(
	context.Context,
	storyboardapp.Actor,
	storyboardapp.BindSelectedImageCommand,
) (storyboardapp.BindSelectedImageResult, error) {
	return storyboardapp.BindSelectedImageResult{}, errors.New("not used")
}

func (*shotWorkflowOwner) BindSelectedImageAtTarget(
	context.Context,
	storyboardapp.Actor,
	storyboardapp.BindSelectedImageAtTargetCommand,
) (storyboardapp.BindSelectedImageResult, error) {
	return storyboardapp.BindSelectedImageResult{}, errors.New("not used")
}

type shotCandidateSetSource struct {
	set   generationdomain.CandidateSet
	actor generationapp.Actor
	id    string
	err   error
}

func (source *shotCandidateSetSource) RequireCandidateSet(
	_ context.Context,
	actor generationapp.Actor,
	id string,
) (generationdomain.CandidateSet, error) {
	source.actor, source.id = actor, id
	return source.set, source.err
}

func TestShotWorkflowSourceExecutorsRouteOnlyToOwningApplications(t *testing.T) {
	workspaceID, projectID, userID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	shotID, providerJobID := uuid.NewString(), uuid.NewString()
	shotHash, candidateSetHash := strings.Repeat("a", 64), strings.Repeat("b", 64)
	owner := &shotWorkflowOwner{shot: storyboarddomain.Shot{
		ID: shotID, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: uuid.NewString(),
		BatchID: uuid.NewString(), ProposalKey: "shot-1", Position: 1, Title: "雨巷",
		ContentHash: shotHash, Status: "active", Revision: 2, CreatedBy: userID,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}}
	sets := &shotCandidateSetSource{set: generationdomain.CandidateSet{
		ID: providerJobID, WorkspaceID: workspaceID, ProjectID: projectID,
		ProviderReceiptID: uuid.NewString(), Revision: 1, ContentHash: candidateSetHash,
		Candidates: []generationdomain.CandidateReference{{
			ID: uuid.NewString(), Revision: 1, ArtifactID: uuid.NewString(), ArtifactRevision: 1,
			ArtifactSHA256: strings.Repeat("d", 64), QCReportID: uuid.NewString(), QCReportHash: strings.Repeat("e", 64),
		}},
	}}
	production := workflowproduction.NewNodeExecutor(nil, nil, nil, nil, nil, owner)
	generation := workflowgeneration.NewNodeExecutor(sets)
	executor, err := workflowexecution.NewNodeExecutor(production, generation)
	if err != nil {
		t.Fatalf("compose explicit Workflow executor router: %v", err)
	}

	shotResult := executeShotSource(t, executor, workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{
			WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), NodeID: "shot", Executor: "workflow.input.production_shot_binding_target", Attempt: 1,
		},
		WorkspaceID: workspaceID, ProjectID: projectID, InitiatorUserID: userID, InitiatorTokenVersion: 3,
		IdempotencyKey: "shot-source", OutputPorts: []authoring.PortDefinition{
			{Key: "shot", ValueType: "production_shot", Required: true},
			{Key: "binding_target", ValueType: "production_shot_image_binding_target", Required: true},
		},
	}, json.RawMessage(`{"shot_id":"`+shotID+`"}`))
	shotOutputs := make(map[string]workflow.NodeOutputBinding, len(shotResult.Output.Bindings))
	for _, binding := range shotResult.Output.Bindings {
		shotOutputs[binding.Port] = binding
	}
	if len(shotOutputs) != 2 || shotOutputs["shot"].ReferenceID != shotID ||
		shotOutputs["shot"].ReferenceVersion != "2" || shotOutputs["shot"].ContentHash != shotHash ||
		shotOutputs["binding_target"].ReferenceID != shotID ||
		shotOutputs["binding_target"].ReferenceVersion != "1" ||
		shotOutputs["binding_target"].ContentHash != strings.Repeat("f", 64) {
		t.Fatalf("Production Shot source output drifted: %#v", shotResult)
	}
	if owner.actor.UserID != userID || owner.actor.TokenVersion != 3 || owner.shotID != shotID {
		t.Fatalf("Production Shot owner call drifted: actor=%#v shot=%s", owner.actor, owner.shotID)
	}

	candidateResult := executeShotSource(t, executor, workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{
			WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), NodeID: "candidates", Executor: "workflow.input.generation_candidate_set", Attempt: 1,
		},
		WorkspaceID: workspaceID, ProjectID: projectID, InitiatorUserID: userID, InitiatorTokenVersion: 3,
		IdempotencyKey: "candidate-source", OutputPorts: []authoring.PortDefinition{{Key: "candidates", ValueType: "generation_candidate_set", Required: true}},
	}, json.RawMessage(`{"provider_job_id":"`+providerJobID+`"}`), domainShotBinding(shotID, shotHash))
	if len(candidateResult.Output.Bindings) != 1 || candidateResult.Output.Bindings[0].ReferenceID != providerJobID ||
		candidateResult.Output.Bindings[0].ReferenceVersion != "1" || candidateResult.Output.Bindings[0].ContentHash != candidateSetHash {
		t.Fatalf("Generation CandidateSet source output drifted: %#v", candidateResult)
	}
	if sets.actor.UserID != userID || sets.actor.TokenVersion != 3 || sets.id != providerJobID {
		t.Fatalf("Generation CandidateSet owner call drifted: actor=%#v id=%s", sets.actor, sets.id)
	}

	owner.shot.ProjectID = uuid.NewString()
	if _, err = executeShotSourceError(executor, workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{
			WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), NodeID: "shot", Executor: "workflow.input.production_shot_binding_target", Attempt: 1,
		},
		WorkspaceID: workspaceID, ProjectID: projectID, InitiatorUserID: userID, InitiatorTokenVersion: 3,
		IdempotencyKey: "shot-source-drift", OutputPorts: []authoring.PortDefinition{
			{Key: "shot", ValueType: "production_shot", Required: true},
			{Key: "binding_target", ValueType: "production_shot_image_binding_target", Required: true},
		},
	}, json.RawMessage(`{"shot_id":"`+shotID+`"}`)); err == nil {
		t.Fatal("Production Shot source accepted an Owner snapshot from another project")
	}
	if _, err = executeShotSourceError(executor, workflow.NodeExecutorCommand{
		NodeActivityCommand: workflow.NodeActivityCommand{
			WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), NodeID: "unknown", Executor: "workflow.input.unknown", Attempt: 1,
		},
		WorkspaceID: workspaceID, ProjectID: projectID, InitiatorUserID: userID, InitiatorTokenVersion: 3,
		IdempotencyKey: "unknown-source", OutputPorts: []authoring.PortDefinition{{Key: "shot", ValueType: "production_shot", Required: true}},
	}, json.RawMessage(`{}`)); err == nil {
		t.Fatal("Workflow executor router accepted an unowned executor")
	}
}

type executingNode interface {
	Execute(context.Context, workflow.NodeExecutorCommand) (workflow.NodeExecutorResult, error)
}

func executeShotSource(
	t *testing.T,
	executor executingNode,
	command workflow.NodeExecutorCommand,
	config json.RawMessage,
	bindings ...workflow.NodeInputBinding,
) workflow.NodeExecutorResult {
	t.Helper()
	result, err := executeShotSourceError(executor, command, config, bindings...)
	if err != nil {
		t.Fatalf("execute Shot Workflow source: %v", err)
	}
	return result
}

func executeShotSourceError(
	executor executingNode,
	command workflow.NodeExecutorCommand,
	config json.RawMessage,
	bindings ...workflow.NodeInputBinding,
) (workflow.NodeExecutorResult, error) {
	input, _, inputHash, err := workflow.BuildNodeInput(workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion, Config: config,
		Bindings: bindings,
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: uuid.NewString(), Version: "1", Hash: strings.Repeat("c", 64),
		}},
	})
	if err != nil {
		return workflow.NodeExecutorResult{}, err
	}
	command.Input, command.InputHash = input, inputHash
	return executor.Execute(context.Background(), command)
}

func domainShotBinding(shotID, contentHash string) workflow.NodeInputBinding {
	return workflow.NodeInputBinding{
		Port: "shot", ValueType: "production_shot", SourceKind: workflow.NodeInputSourceNodeOutput,
		SourceNodeID: "shot", SourcePort: "shot", ReferenceID: shotID, ReferenceVersion: "2", ContentHash: contentHash,
	}
}

var _ workflowproduction.ShotImageWorkflowOwner = (*shotWorkflowOwner)(nil)
