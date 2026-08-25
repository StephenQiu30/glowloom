package production

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	planningdomain "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	scriptdomain "github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	scriptRevisionExecutor   = "workflow.input.script_revision"
	productionBibleExecutor  = "activity.production_bible"
	episodePlanExecutor      = "activity.episode_plan"
	episodeStructureExecutor = "activity.episode_structure"
)

type ScriptSource interface {
	GetRevision(context.Context, scriptapp.Actor, string) (scriptdomain.Analysis, error)
}

type BibleCandidateOwner interface {
	Create(context.Context, bibleapp.Actor, bibleapp.CreateCommand) (bibledomain.Bible, error)
	Get(context.Context, bibleapp.Actor, string) (bibledomain.Bible, error)
}

type ProjectSource interface {
	Get(context.Context, projectapp.Actor, string) (projectdomain.Project, error)
}

type EpisodePlanOwner interface {
	CreatePlan(context.Context, planningapp.Actor, planningapp.CreatePlanCommand) (planningapp.View, error)
	GetPlan(context.Context, planningapp.Actor, string) (planningapp.View, error)
	GetImportCommitForPlan(context.Context, planningapp.Actor, string) (planningdomain.ImportCommit, bool, error)
	Materialize(context.Context, planningapp.Actor, planningapp.MaterializeCommand) (planningdomain.ImportCommit, error)
	Publish(context.Context, planningapp.Actor, planningapp.PublishCommand) (planningdomain.ImportCommit, error)
	GetPublishedStructureBatch(context.Context, planningapp.Actor, string) (planningapp.PublishedStructureBatch, error)
}

type NodeExecutor struct {
	scripts  ScriptSource
	bibles   BibleCandidateOwner
	projects ProjectSource
	plans    EpisodePlanOwner
}

func NewNodeExecutor(
	scripts ScriptSource,
	bibles BibleCandidateOwner,
	projects ProjectSource,
	plans EpisodePlanOwner,
) *NodeExecutor {
	return &NodeExecutor{scripts: scripts, bibles: bibles, projects: projects, plans: plans}
}

func (executor *NodeExecutor) Execute(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor == nil || strings.TrimSpace(command.WorkspaceID) == "" || strings.TrimSpace(command.ProjectID) == "" ||
		strings.TrimSpace(command.InitiatorUserID) == "" || command.InitiatorTokenVersion < 1 ||
		strings.TrimSpace(command.IdempotencyKey) == "" {
		return domain.NodeExecutorResult{}, errors.New("unsupported production workflow node execution")
	}
	switch command.Executor {
	case scriptRevisionExecutor:
		return executor.executeScriptRevision(ctx, command)
	case productionBibleExecutor:
		return executor.executeProductionBible(ctx, command)
	case episodePlanExecutor:
		return executor.executeEpisodePlan(ctx, command)
	case episodeStructureExecutor:
		return executor.executeEpisodeStructure(ctx, command)
	default:
		return domain.NodeExecutorResult{}, errors.New("unsupported production workflow node execution")
	}
}

func (executor *NodeExecutor) executeEpisodeStructure(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.plans == nil {
		return domain.NodeExecutorResult{}, errors.New("episode structure workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "candidate" ||
		command.OutputPorts[0].ValueType != "episode_structure_candidate" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid episode structure node contract")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 0 {
		return domain.NodeExecutorResult{}, errors.New("invalid episode structure node config")
	}
	binding := input.Bindings[0]
	expectedPlanRevision, err := strconv.Atoi(binding.ReferenceVersion)
	if err != nil || expectedPlanRevision < 1 || binding.Port != "episodes" || binding.ValueType != "episode_plan" ||
		binding.SourceKind != domain.NodeInputSourceNodeOutput || binding.SourcePort != "episodes" ||
		strings.TrimSpace(binding.SourceNodeID) == "" || len(binding.ContentHash) != 64 {
		return domain.NodeExecutorResult{}, errors.New("episode structure plan input has drifted")
	}
	actor := planningapp.Actor{UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion}
	view, err := executor.plans.GetPlan(ctx, actor, binding.ReferenceID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	plan := view.Plan
	if plan.ID != binding.ReferenceID || plan.WorkspaceID != command.WorkspaceID || plan.ProjectID != command.ProjectID ||
		plan.InputHash != binding.ContentHash || plan.CreatedBy != command.InitiatorUserID || len(plan.Proposals) == 0 ||
		!frozenScriptMatchesPlan(input, plan) {
		return domain.NodeExecutorResult{}, errors.New("episode structure plan does not match workflow input")
	}
	commit, found, err := executor.plans.GetImportCommitForPlan(ctx, actor, plan.ID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if !found {
		if plan.Status != "confirmed" || plan.Revision != expectedPlanRevision || view.Impact.ProjectRevision < 1 ||
			len(view.Impact.ActiveOrderHash) != 64 || !view.Impact.Allowed || len(view.Impact.Blockers) != 0 {
			return domain.NodeExecutorResult{}, errors.New("confirmed episode plan is not materializable")
		}
		commit, err = executor.plans.Materialize(ctx, actor, planningapp.MaterializeCommand{
			PlanID: plan.ID, Mode: "append_new", ExpectedPlanRevision: expectedPlanRevision,
			ExpectedProjectRevision: view.Impact.ProjectRevision, ExpectedActiveOrderHash: view.Impact.ActiveOrderHash,
			IdempotencyKey: command.IdempotencyKey + ":materialize",
		})
	}
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if commit.ID == "" || commit.WorkspaceID != command.WorkspaceID || commit.ProjectID != command.ProjectID ||
		commit.PlanID != plan.ID || commit.CreatedBy != command.InitiatorUserID || len(commit.InputHash) != 64 ||
		len(commit.Segments) != len(plan.Proposals) {
		return domain.NodeExecutorResult{}, errors.New("episode materialization does not match workflow input")
	}
	switch commit.Status {
	case "materialized":
		if commit.Revision != 1 {
			return domain.NodeExecutorResult{}, errors.New("episode materialization revision has drifted")
		}
		commit, err = executor.plans.Publish(ctx, actor, planningapp.PublishCommand{
			CommitID: commit.ID, ExpectedRevision: commit.Revision,
			IdempotencyKey: command.IdempotencyKey + ":publish",
		})
		if err != nil {
			return domain.NodeExecutorResult{}, err
		}
	case "published":
	default:
		return domain.NodeExecutorResult{}, errors.New("episode materialization is not publishable")
	}
	if commit.Status != "published" || commit.Revision != 2 {
		return domain.NodeExecutorResult{}, errors.New("episode publication did not reach its terminal revision")
	}
	batch, err := executor.plans.GetPublishedStructureBatch(ctx, actor, plan.ID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if batch.Commit.ID != commit.ID || batch.Commit.Status != "published" || batch.Commit.Revision != commit.Revision ||
		len(batch.Structures) != len(commit.Segments) || len(batch.ContentHash) != 64 {
		return domain.NodeExecutorResult{}, errors.New("published episode structure batch has drifted")
	}
	for _, structure := range batch.Structures {
		if structure.WorkspaceID != command.WorkspaceID || structure.ProjectID != command.ProjectID ||
			structure.Status != "needs_review" || structure.Revision != 1 || len(structure.ResultHash) != 64 {
			return domain.NodeExecutorResult{}, errors.New("published episode structure candidate is invalid")
		}
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "candidate", ValueType: "episode_structure_candidate", ReferenceID: commit.ID,
			ReferenceVersion: strconv.Itoa(commit.Revision), ContentHash: batch.ContentHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeEpisodePlan(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.bibles == nil || executor.projects == nil || executor.plans == nil {
		return domain.NodeExecutorResult{}, errors.New("episode plan workflow owners are unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 2 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "candidate" ||
		command.OutputPorts[0].ValueType != "episode_plan_candidate" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid episode plan node contract")
	}
	var config map[string]json.RawMessage
	var episodeCount int
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 1 ||
		json.Unmarshal(config["episode_count"], &episodeCount) != nil || episodeCount < 1 || episodeCount > 100 {
		return domain.NodeExecutorResult{}, errors.New("invalid episode plan node config")
	}
	bindings := make(map[string]domain.NodeInputBinding, len(input.Bindings))
	for _, binding := range input.Bindings {
		bindings[binding.Port] = binding
	}
	script := bindings["script"]
	bibleBinding := bindings["bible"]
	if script.ValueType != "script_revision" || script.SourceKind != domain.NodeInputSourceNodeOutput ||
		script.SourcePort != "script" || strings.TrimSpace(script.SourceNodeID) == "" || !frozenScriptMatches(input, script) ||
		bibleBinding.ValueType != "production_bible" || bibleBinding.SourceKind != domain.NodeInputSourceNodeOutput ||
		bibleBinding.SourcePort != "bible" || strings.TrimSpace(bibleBinding.SourceNodeID) == "" {
		return domain.NodeExecutorResult{}, errors.New("episode plan inputs have drifted")
	}
	actor := projectapp.Actor{UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion}
	project, err := executor.projects.Get(ctx, actor, command.ProjectID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if project.ID != command.ProjectID || project.WorkspaceID != command.WorkspaceID ||
		project.Status != projectdomain.StatusActive || project.TargetDurationMS < 15_000 || project.TargetDurationMS > 600_000 {
		return domain.NodeExecutorResult{}, errors.New("episode plan project does not match workflow input")
	}
	bibleRevision, err := strconv.Atoi(bibleBinding.ReferenceVersion)
	if err != nil || bibleRevision < 1 {
		return domain.NodeExecutorResult{}, errors.New("invalid confirmed production bible reference")
	}
	bible, err := executor.bibles.Get(ctx, bibleapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, bibleBinding.ReferenceID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if bible.ID != bibleBinding.ReferenceID || bible.WorkspaceID != command.WorkspaceID ||
		bible.ProjectID != command.ProjectID || bible.DocumentRevisionID != script.ReferenceID ||
		bible.Status != "confirmed" || bible.Revision != bibleRevision || bible.InputHash != script.ContentHash ||
		bible.ResultHash == nil || *bible.ResultHash != bibleBinding.ContentHash {
		return domain.NodeExecutorResult{}, errors.New("confirmed production bible does not match episode plan input")
	}
	view, err := executor.plans.CreatePlan(ctx, planningapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, planningapp.CreatePlanCommand{
		RevisionID: script.ReferenceID, Strategy: "explicit_markers", TargetDurationMS: project.TargetDurationMS,
		RequestedEpisodeCount: &episodeCount, IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	plan := view.Plan
	if plan.ID == "" || plan.WorkspaceID != command.WorkspaceID || plan.ProjectID != command.ProjectID ||
		plan.DocumentRevisionID != script.ReferenceID || plan.Status != "review_ready" || plan.Revision != 1 ||
		plan.TargetDurationMS != project.TargetDurationMS || plan.RequestedEpisodeCount == nil ||
		*plan.RequestedEpisodeCount != episodeCount || len(plan.Proposals) != episodeCount || len(plan.InputHash) != 64 {
		return domain.NodeExecutorResult{}, errors.New("episode plan owner result does not match workflow input")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "candidate", ValueType: "episode_plan_candidate", ReferenceID: plan.ID,
			ReferenceVersion: strconv.Itoa(plan.Revision), ContentHash: plan.InputHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeScriptRevision(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.scripts == nil {
		return domain.NodeExecutorResult{}, errors.New("script revision workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 0 || len(input.FrozenInputs) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "script" ||
		command.OutputPorts[0].ValueType != "script_revision" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid script revision node contract")
	}
	reference := input.FrozenInputs[0]
	var config struct {
		DocumentRevisionID string `json:"document_revision_id"`
	}
	if reference.Kind != "script_revision" || json.Unmarshal(input.Config, &config) != nil ||
		config.DocumentRevisionID != reference.ID {
		return domain.NodeExecutorResult{}, errors.New("script revision input does not match node config")
	}
	analysis, err := executor.scripts.GetRevision(ctx, scriptapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, reference.ID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if analysis.Document.WorkspaceID != command.WorkspaceID || analysis.Document.ProjectID != command.ProjectID ||
		analysis.Revision.WorkspaceID != command.WorkspaceID || analysis.Revision.DocumentID != analysis.Document.ID ||
		analysis.Revision.ID != reference.ID || strconv.Itoa(analysis.Revision.VersionNo) != reference.Version ||
		analysis.Revision.NormalizedHash != reference.Hash {
		return domain.NodeExecutorResult{}, errors.New("script revision changed before workflow node execution")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "script", ValueType: "script_revision", ReferenceID: analysis.Revision.ID,
			ReferenceVersion: strconv.Itoa(analysis.Revision.VersionNo), ContentHash: analysis.Revision.NormalizedHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeProductionBible(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.bibles == nil {
		return domain.NodeExecutorResult{}, errors.New("production bible workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "candidate" ||
		command.OutputPorts[0].ValueType != "production_bible_candidate" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid production bible node contract")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 0 {
		return domain.NodeExecutorResult{}, errors.New("invalid production bible node config")
	}
	binding := input.Bindings[0]
	if binding.Port != "script" || binding.ValueType != "script_revision" ||
		binding.SourceKind != domain.NodeInputSourceNodeOutput || binding.SourcePort != "script" ||
		strings.TrimSpace(binding.SourceNodeID) == "" || !frozenScriptMatches(input, binding) {
		return domain.NodeExecutorResult{}, errors.New("production bible script input has drifted")
	}
	bible, err := executor.bibles.Create(ctx, bibleapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, bibleapp.CreateCommand{RevisionID: binding.ReferenceID, IdempotencyKey: command.IdempotencyKey})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if bible.WorkspaceID != command.WorkspaceID || bible.ProjectID != command.ProjectID ||
		bible.DocumentRevisionID != binding.ReferenceID || bible.InputHash != binding.ContentHash ||
		bible.CreatedBy != command.InitiatorUserID || bible.Revision < 1 {
		return domain.NodeExecutorResult{}, errors.New("production bible owner result does not match workflow input")
	}
	switch bible.Status {
	case "queued", "running":
		return domain.NodeExecutorResult{Status: "RETRYING"}, nil
	case "needs_review":
		if bible.ResultHash == nil || len(*bible.ResultHash) != 64 {
			return domain.NodeExecutorResult{}, errors.New("production bible candidate result is incomplete")
		}
	default:
		return domain.NodeExecutorResult{}, errors.New("production bible candidate is unavailable")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "candidate", ValueType: "production_bible_candidate", ReferenceID: bible.ID,
			ReferenceVersion: strconv.Itoa(bible.Revision), ContentHash: *bible.ResultHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func frozenScriptMatches(input domain.NodeInputSnapshot, binding domain.NodeInputBinding) bool {
	for _, reference := range input.FrozenInputs {
		if reference.Kind == "script_revision" && reference.ID == binding.ReferenceID &&
			reference.Version == binding.ReferenceVersion && reference.Hash == binding.ContentHash {
			return true
		}
	}
	return false
}

func frozenScriptMatchesPlan(input domain.NodeInputSnapshot, plan planningdomain.Plan) bool {
	return len(input.FrozenInputs) == 1 && input.FrozenInputs[0].Kind == "script_revision" &&
		input.FrozenInputs[0].ID == plan.DocumentRevisionID && input.FrozenInputs[0].Hash == plan.Source.NormalizedHash
}

var _ workflowapp.NodeExecutor = (*NodeExecutor)(nil)
