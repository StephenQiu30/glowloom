package production

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	planningdomain "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	scriptdomain "github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	storyboarddomain "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	scriptRevisionExecutor         = "workflow.input.script_revision"
	sourceEvidenceExecutor         = "activity.source_evidence"
	storyAnalysisExecutor          = "activity.story_analysis"
	storyReviewExecutor            = "activity.story_review"
	productionBibleExecutor        = "activity.production_bible"
	bibleMaterializationExecutor   = "activity.production_bible_materialization"
	episodeSegmentationExecutor    = "activity.episode_segmentation"
	episodeAnalysisExecutor        = "activity.episode_analysis"
	episodePlanExecutor            = "activity.episode_plan"
	episodeStructureExecutor       = "activity.episode_structure"
	storyGraphCompileExecutor      = "activity.storygraph_compile"
	storyboardDraftExecutor        = "activity.storyboard_draft"
	storyboardExportExecutor       = "activity.storyboard_export"
	productionShotInputExecutor    = "workflow.input.production_shot"
	shotImageBindingExecutor       = "activity.production_shot_image_binding"
	productionShotTargetExecutor   = "workflow.input.production_shot_binding_target"
	shotImageBindingTargetExecutor = "activity.production_shot_image_binding_at_target"
)

var workflowContentHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ScriptSource interface {
	GetRevision(context.Context, scriptapp.Actor, string) (scriptdomain.Analysis, error)
}

type BibleCandidateOwner interface {
	Create(context.Context, bibleapp.Actor, bibleapp.CreateCommand) (bibledomain.Bible, error)
	Get(context.Context, bibleapp.Actor, string) (bibledomain.Bible, error)
	MaterializeConfirmedBible(context.Context, bibleapp.Actor, bibleapp.MaterializeCommand) (bibleapp.MaterializeResult, error)
}

type SourceEvidenceOwner interface {
	Ensure(context.Context, bibleapp.SourceEvidenceCommand) (bibleapp.SourceEvidenceState, error)
}

type StoryAnalysisOwner interface {
	Ensure(context.Context, bibleapp.StoryAnalysisCommand) (bibleapp.StoryAnalysisState, error)
}

type StoryReviewOwner interface {
	EnsureStoryReview(context.Context, bibleapp.StoryReviewCommand) (bibleapp.StoryReviewState, error)
}

type EpisodeSegmentationOwner interface {
	Ensure(context.Context, bibleapp.EpisodeSegmentationCommand) (bibleapp.EpisodeSegmentationState, error)
}

type EpisodeAnalysisOwner interface {
	Ensure(context.Context, planningapp.EpisodeAnalysisCommand) (planningapp.EpisodeAnalysisState, error)
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
	GetConfirmedStructureBatch(context.Context, planningapp.Actor, string) (planningapp.PublishedStructureBatch, error)
}

type PlanningOwnerSetSource interface {
	GetPlanningOwnerSet(context.Context, planningapp.Actor, string) (planningapp.AppliedPlanningOwnerSet, error)
}

type StoryGraphCompiler interface {
	CompileOwnerSet(context.Context, storygraphapp.Actor, storygraphapp.CompileOwnerSetCommand) (storygraphapp.CompileResult, error)
}

type StoryboardWorkflowOwner interface {
	CreateSet(context.Context, storyboardapp.Actor, storyboardapp.CreateSetCommand) (storyboarddomain.DraftSet, error)
	RefreshSet(context.Context, storyboardapp.Actor, string) (storyboarddomain.DraftSet, error)
	CreateExportSet(context.Context, storyboardapp.Actor, storyboardapp.CreateExportSetCommand) (storyboarddomain.ExportSet, error)
}

type ShotImageWorkflowOwner interface {
	RequireActiveShot(context.Context, storyboardapp.Actor, string) (storyboarddomain.Shot, error)
	RequireShotImageBindingTarget(context.Context, storyboardapp.Actor, string) (storyboardapp.ShotImageBindingTarget, error)
	BindSelectedImage(context.Context, storyboardapp.Actor, storyboardapp.BindSelectedImageCommand) (storyboardapp.BindSelectedImageResult, error)
	BindSelectedImageAtTarget(context.Context, storyboardapp.Actor, storyboardapp.BindSelectedImageAtTargetCommand) (storyboardapp.BindSelectedImageResult, error)
}

type NodeExecutor struct {
	scripts        ScriptSource
	evidence       SourceEvidenceOwner
	stories        StoryAnalysisOwner
	storyReviews   StoryReviewOwner
	bibles         BibleCandidateOwner
	projects       ProjectSource
	plans          EpisodePlanOwner
	planningOwners PlanningOwnerSetSource
	storygraphs    StoryGraphCompiler
	storyboards    StoryboardWorkflowOwner
	bindings       ShotImageWorkflowOwner
	segments       EpisodeSegmentationOwner
	episodes       EpisodeAnalysisOwner
}

func NewNodeExecutor(
	scripts ScriptSource,
	evidence SourceEvidenceOwner,
	stories StoryAnalysisOwner,
	storyReviews StoryReviewOwner,
	bibles BibleCandidateOwner,
	projects ProjectSource,
	plans EpisodePlanOwner,
	planningOwners PlanningOwnerSetSource,
	storygraphs StoryGraphCompiler,
	storyboards StoryboardWorkflowOwner,
	bindings ShotImageWorkflowOwner,
	segments EpisodeSegmentationOwner,
	episodes EpisodeAnalysisOwner,
) *NodeExecutor {
	return &NodeExecutor{
		scripts: scripts, evidence: evidence, stories: stories, storyReviews: storyReviews, bibles: bibles, projects: projects, plans: plans,
		planningOwners: planningOwners, storygraphs: storygraphs,
		storyboards: storyboards, bindings: bindings, segments: segments, episodes: episodes,
	}
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
	case sourceEvidenceExecutor:
		return executor.executeSourceEvidence(ctx, command)
	case storyAnalysisExecutor:
		return executor.executeStoryAnalysis(ctx, command)
	case storyReviewExecutor:
		return executor.executeStoryReview(ctx, command)
	case productionBibleExecutor:
		return executor.executeProductionBible(ctx, command)
	case bibleMaterializationExecutor:
		return executor.executeBibleMaterialization(ctx, command)
	case episodeSegmentationExecutor:
		return executor.executeEpisodeSegmentation(ctx, command)
	case episodeAnalysisExecutor:
		return executor.executeEpisodeAnalysis(ctx, command)
	case episodePlanExecutor:
		return executor.executeEpisodePlan(ctx, command)
	case episodeStructureExecutor:
		return executor.executeEpisodeStructure(ctx, command)
	case storyGraphCompileExecutor:
		return executor.executeStoryGraphCompile(ctx, command)
	case storyboardDraftExecutor:
		return executor.executeStoryboardDraft(ctx, command)
	case storyboardExportExecutor:
		return executor.executeStoryboardExport(ctx, command)
	case productionShotInputExecutor:
		return executor.executeProductionShotInput(ctx, command)
	case shotImageBindingExecutor:
		return executor.executeShotImageBinding(ctx, command)
	case productionShotTargetExecutor:
		return executor.executeProductionShotBindingTarget(ctx, command)
	case shotImageBindingTargetExecutor:
		return executor.executeShotImageBindingAtTarget(ctx, command)
	default:
		return domain.NodeExecutorResult{}, errors.New("unsupported production workflow node execution")
	}
}

func (executor *NodeExecutor) executeStoryReview(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.storyReviews == nil {
		return domain.NodeExecutorResult{}, errors.New("Story review workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "candidate" ||
		command.OutputPorts[0].ValueType != "story_reconciliation_candidate" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid Story review node contract")
	}
	var config struct {
		MaxRepairRounds int `json:"max_repair_rounds"`
	}
	var rawConfig map[string]json.RawMessage
	if json.Unmarshal(input.Config, &rawConfig) != nil || len(rawConfig) != 1 ||
		json.Unmarshal(input.Config, &config) != nil || config.MaxRepairRounds < 1 || config.MaxRepairRounds > 3 {
		return domain.NodeExecutorResult{}, errors.New("invalid Story review node config")
	}
	binding := input.Bindings[0]
	if binding.Port != "candidate" || binding.ValueType != "story_reconciliation_candidate" ||
		binding.SourceKind != domain.NodeInputSourceNodeOutput || binding.SourcePort != "candidate" ||
		strings.TrimSpace(binding.SourceNodeID) == "" {
		return domain.NodeExecutorResult{}, errors.New("Story review Candidate input has drifted")
	}
	state, err := executor.storyReviews.EnsureStoryReview(ctx, bibleapp.StoryReviewCommand{
		Actor:       bibleapp.Actor{UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion},
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		CandidateRevisionID: binding.ReferenceID, CandidateRevisionHash: binding.ContentHash,
		MaxRepairRounds: config.MaxRepairRounds,
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	switch state.Status {
	case "pending":
		return domain.NodeExecutorResult{Status: "RETRYING"}, nil
	case "needs_review":
		return domain.NodeExecutorResult{}, errors.New("Story review stopped: " + state.FailureCode)
	case "ready":
		if _, parseErr := uuid.Parse(state.CandidateRevisionID); parseErr != nil ||
			state.CandidateRevisionNo < 1 || !workflowContentHashPattern.MatchString(state.CandidateRevisionHash) {
			return domain.NodeExecutorResult{}, errors.New("Story review candidate is incomplete")
		}
	default:
		return domain.NodeExecutorResult{}, errors.New("Story review returned an invalid status")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "candidate", ValueType: "story_reconciliation_candidate",
			ReferenceID: state.CandidateRevisionID, ReferenceVersion: strconv.FormatInt(state.CandidateRevisionNo, 10),
			ContentHash: state.CandidateRevisionHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeSourceEvidence(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.scripts == nil || executor.evidence == nil {
		return domain.NodeExecutorResult{}, errors.New("source Evidence workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "evidence" ||
		command.OutputPorts[0].ValueType != "source_evidence_candidate" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid source Evidence node contract")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 0 {
		return domain.NodeExecutorResult{}, errors.New("invalid source Evidence node config")
	}
	binding := input.Bindings[0]
	if binding.Port != "script" || binding.ValueType != "script_revision" ||
		binding.SourceKind != domain.NodeInputSourceNodeOutput || binding.SourcePort != "script" ||
		strings.TrimSpace(binding.SourceNodeID) == "" || !frozenScriptMatches(input, binding) {
		return domain.NodeExecutorResult{}, errors.New("source Evidence script input has drifted")
	}
	analysis, err := executor.scripts.GetRevision(ctx, scriptapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, binding.ReferenceID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if analysis.Document.WorkspaceID != command.WorkspaceID || analysis.Document.ProjectID != command.ProjectID ||
		analysis.Revision.ID != binding.ReferenceID || analysis.Revision.DocumentID != analysis.Document.ID ||
		strconv.Itoa(analysis.Revision.VersionNo) != binding.ReferenceVersion ||
		analysis.Revision.NormalizedHash != binding.ContentHash {
		return domain.NodeExecutorResult{}, errors.New("source Evidence DocumentRevision has drifted")
	}
	state, err := executor.evidence.Ensure(ctx, bibleapp.SourceEvidenceCommand{
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		DocumentLogicalID: analysis.Document.ID, DocumentRevisionID: analysis.Revision.ID,
		DocumentRevision: int64(analysis.Revision.VersionNo),
		NormalizedText:   analysis.Revision.NormalizedText, NormalizedHash: analysis.Revision.NormalizedHash,
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	switch state.Status {
	case "pending":
		return domain.NodeExecutorResult{Status: "RETRYING"}, nil
	case "failed":
		return domain.NodeExecutorResult{}, errors.New("source Evidence extraction has a failed active shard")
	case "ready":
		if _, parseErr := uuid.Parse(state.CandidateRevisionID); parseErr != nil ||
			state.CandidateRevisionNo < 1 || !workflowContentHashPattern.MatchString(state.CandidateRevisionHash) {
			return domain.NodeExecutorResult{}, errors.New("source Evidence aggregate is incomplete")
		}
	default:
		return domain.NodeExecutorResult{}, errors.New("source Evidence extraction returned an invalid status")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "evidence", ValueType: "source_evidence_candidate",
			ReferenceID:      state.CandidateRevisionID,
			ReferenceVersion: strconv.FormatInt(state.CandidateRevisionNo, 10),
			ContentHash:      state.CandidateRevisionHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeStoryAnalysis(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.stories == nil {
		return domain.NodeExecutorResult{}, errors.New("Story analysis workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "candidate" ||
		command.OutputPorts[0].ValueType != "story_reconciliation_candidate" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid Story analysis node contract")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 0 {
		return domain.NodeExecutorResult{}, errors.New("invalid Story analysis node config")
	}
	binding := input.Bindings[0]
	if binding.Port != "evidence" || binding.ValueType != "source_evidence_candidate" ||
		binding.SourceKind != domain.NodeInputSourceNodeOutput || binding.SourcePort != "evidence" ||
		strings.TrimSpace(binding.SourceNodeID) == "" {
		return domain.NodeExecutorResult{}, errors.New("Story analysis Evidence input has drifted")
	}
	state, err := executor.stories.Ensure(ctx, bibleapp.StoryAnalysisCommand{
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		EvidenceCandidateRevisionID:   binding.ReferenceID,
		EvidenceCandidateRevisionHash: binding.ContentHash,
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	switch state.Status {
	case "pending":
		return domain.NodeExecutorResult{Status: "RETRYING"}, nil
	case "failed":
		return domain.NodeExecutorResult{Status: "RETRYING"}, nil
	case "ready":
		if _, parseErr := uuid.Parse(state.CandidateRevisionID); parseErr != nil ||
			state.CandidateRevisionNo < 1 || !workflowContentHashPattern.MatchString(state.CandidateRevisionHash) {
			return domain.NodeExecutorResult{}, errors.New("Story reconciliation candidate is incomplete")
		}
	default:
		return domain.NodeExecutorResult{}, errors.New("Story analysis returned an invalid status")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "candidate", ValueType: "story_reconciliation_candidate",
			ReferenceID:      state.CandidateRevisionID,
			ReferenceVersion: strconv.FormatInt(state.CandidateRevisionNo, 10),
			ContentHash:      state.CandidateRevisionHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeProductionShotInput(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.bindings == nil {
		return domain.NodeExecutorResult{}, errors.New("Production Shot workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 0 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "shot" ||
		command.OutputPorts[0].ValueType != "production_shot" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid Production Shot source contract")
	}
	var config map[string]json.RawMessage
	var shotID string
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 1 ||
		json.Unmarshal(config["shot_id"], &shotID) != nil {
		return domain.NodeExecutorResult{}, errors.New("invalid Production Shot source config")
	}
	shotID = strings.TrimSpace(shotID)
	if _, parseErr := uuid.Parse(shotID); parseErr != nil {
		return domain.NodeExecutorResult{}, errors.New("invalid Production Shot source config")
	}
	shot, err := executor.bindings.RequireActiveShot(ctx, storyboardapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, shotID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	for _, identifier := range []string{
		shot.ID, shot.WorkspaceID, shot.ProjectID, shot.EpisodeID, shot.BatchID, shot.CreatedBy,
	} {
		if _, parseErr := uuid.Parse(identifier); parseErr != nil {
			return domain.NodeExecutorResult{}, errors.New("Production Shot source returned an invalid identifier")
		}
	}
	if shot.ID != shotID || shot.WorkspaceID != command.WorkspaceID || shot.ProjectID != command.ProjectID ||
		shot.Status != "active" || shot.Position < 1 || shot.Revision < 1 ||
		strings.TrimSpace(shot.ProposalKey) == "" || strings.TrimSpace(shot.Title) == "" ||
		!workflowContentHashPattern.MatchString(shot.ContentHash) || shot.CreatedAt.IsZero() {
		return domain.NodeExecutorResult{}, errors.New("Production Shot source has drifted")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "shot", ValueType: "production_shot", ReferenceID: shot.ID,
			ReferenceVersion: strconv.Itoa(shot.Revision), ContentHash: shot.ContentHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeProductionShotBindingTarget(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.bindings == nil {
		return domain.NodeExecutorResult{}, errors.New("Production Shot workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 0 || len(command.OutputPorts) != 2 {
		return domain.NodeExecutorResult{}, errors.New("invalid Production Shot binding target source contract")
	}
	var hasShotPort, hasTargetPort bool
	for _, port := range command.OutputPorts {
		switch {
		case port.Key == "shot" && port.ValueType == "production_shot" && port.Required:
			hasShotPort = true
		case port.Key == "binding_target" && port.ValueType == "production_shot_image_binding_target" && port.Required:
			hasTargetPort = true
		default:
			return domain.NodeExecutorResult{}, errors.New("invalid Production Shot binding target source contract")
		}
	}
	if !hasShotPort || !hasTargetPort {
		return domain.NodeExecutorResult{}, errors.New("invalid Production Shot binding target source contract")
	}
	var config map[string]json.RawMessage
	var shotID string
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 1 ||
		json.Unmarshal(config["shot_id"], &shotID) != nil {
		return domain.NodeExecutorResult{}, errors.New("invalid Production Shot binding target source config")
	}
	shotID = strings.TrimSpace(shotID)
	if _, parseErr := uuid.Parse(shotID); parseErr != nil {
		return domain.NodeExecutorResult{}, errors.New("invalid Production Shot binding target source config")
	}
	target, err := executor.bindings.RequireShotImageBindingTarget(ctx, storyboardapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, shotID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	shot := target.Shot
	for _, identifier := range []string{
		shot.ID, shot.WorkspaceID, shot.ProjectID, shot.EpisodeID, shot.BatchID, shot.CreatedBy,
	} {
		if _, parseErr := uuid.Parse(identifier); parseErr != nil {
			return domain.NodeExecutorResult{}, errors.New("Production Shot binding target source returned an invalid identifier")
		}
	}
	if shot.ID != shotID || shot.WorkspaceID != command.WorkspaceID || shot.ProjectID != command.ProjectID ||
		shot.Status != "active" || shot.Position < 1 || shot.Revision < 1 ||
		strings.TrimSpace(shot.ProposalKey) == "" || strings.TrimSpace(shot.Title) == "" ||
		!workflowContentHashPattern.MatchString(shot.ContentHash) || shot.CreatedAt.IsZero() ||
		target.ExpectedCurrentRevision < 0 || !workflowContentHashPattern.MatchString(target.ContentHash) {
		return domain.NodeExecutorResult{}, errors.New("Production Shot binding target source has drifted")
	}
	if target.ExpectedCurrentRevision == 0 {
		if target.CurrentBindingID != "" || target.CurrentBindingContentHash != "" {
			return domain.NodeExecutorResult{}, errors.New("Production Shot binding target source has drifted")
		}
	} else if _, parseErr := uuid.Parse(target.CurrentBindingID); parseErr != nil ||
		!workflowContentHashPattern.MatchString(target.CurrentBindingContentHash) {
		return domain.NodeExecutorResult{}, errors.New("Production Shot binding target source has drifted")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{
			{
				Port: "shot", ValueType: "production_shot", ReferenceID: shot.ID,
				ReferenceVersion: strconv.Itoa(shot.Revision), ContentHash: shot.ContentHash,
			},
			{
				Port: "binding_target", ValueType: "production_shot_image_binding_target", ReferenceID: shot.ID,
				ReferenceVersion: strconv.Itoa(target.ExpectedCurrentRevision + 1), ContentHash: target.ContentHash,
			},
		},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeShotImageBinding(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.bindings == nil {
		return domain.NodeExecutorResult{}, errors.New("Shot image binding workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 2 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "binding" ||
		command.OutputPorts[0].ValueType != "production_shot_image_binding" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid Shot image binding node contract")
	}
	var config map[string]json.RawMessage
	var expectedCurrentRevision int
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 1 ||
		json.Unmarshal(config["expected_current_revision"], &expectedCurrentRevision) != nil || expectedCurrentRevision < 0 {
		return domain.NodeExecutorResult{}, errors.New("invalid Shot image binding node config")
	}
	bindings := make(map[string]domain.NodeInputBinding, len(input.Bindings))
	for _, binding := range input.Bindings {
		bindings[binding.Port] = binding
	}
	shot, selection := bindings["shot"], bindings["selection"]
	shotRevision, shotRevisionErr := strconv.Atoi(shot.ReferenceVersion)
	selectionRevision, selectionRevisionErr := strconv.Atoi(selection.ReferenceVersion)
	if shotRevisionErr != nil || shotRevision < 1 || selectionRevisionErr != nil || selectionRevision != 1 ||
		shot.ValueType != "production_shot" || selection.ValueType != "generation_candidate_selection" ||
		shot.SourceKind != domain.NodeInputSourceNodeOutput || selection.SourceKind != domain.NodeInputSourceNodeOutput ||
		shot.SourceNodeID == "" || shot.SourcePort == "" || selection.SourceNodeID == "" || selection.SourcePort == "" ||
		len(shot.ContentHash) != 64 || len(selection.ContentHash) != 64 {
		return domain.NodeExecutorResult{}, errors.New("Shot image binding inputs have drifted")
	}
	result, err := executor.bindings.BindSelectedImage(ctx, storyboardapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, storyboardapp.BindSelectedImageCommand{
		ShotID: shot.ReferenceID, ExpectedShotRevision: shotRevision, ExpectedShotContentHash: shot.ContentHash,
		CandidateSelectionID: selection.ReferenceID, ExpectedCurrentRevision: expectedCurrentRevision,
		IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	binding := result.Binding
	if binding.WorkspaceID != command.WorkspaceID || binding.ProjectID != command.ProjectID ||
		binding.ShotID != shot.ReferenceID || binding.ShotRevision != shotRevision ||
		binding.ShotContentHash != shot.ContentHash || binding.CandidateSelectionID != selection.ReferenceID ||
		binding.CandidateSelectionRevision != selectionRevision ||
		binding.CandidateSelectionContentHash != selection.ContentHash || binding.Revision != expectedCurrentRevision+1 ||
		len(binding.ContentHash) != 64 || binding.CreatedBy != command.InitiatorUserID ||
		result.Receipt.Operation != "storyboard.shot.bind_selected_image" ||
		result.Receipt.ResourceID != binding.ID || result.Receipt.CreatedBy != command.InitiatorUserID {
		return domain.NodeExecutorResult{}, errors.New("Shot image binding does not match workflow input")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "binding", ValueType: "production_shot_image_binding", ReferenceID: binding.ID,
			ReferenceVersion: strconv.Itoa(binding.Revision), ContentHash: binding.ContentHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeShotImageBindingAtTarget(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.bindings == nil {
		return domain.NodeExecutorResult{}, errors.New("Shot image binding workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 3 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "binding" ||
		command.OutputPorts[0].ValueType != "production_shot_image_binding" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid Shot image binding target node contract")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 0 {
		return domain.NodeExecutorResult{}, errors.New("invalid Shot image binding target node config")
	}
	bindings := make(map[string]domain.NodeInputBinding, len(input.Bindings))
	for _, binding := range input.Bindings {
		bindings[binding.Port] = binding
	}
	shot, selection, target := bindings["shot"], bindings["selection"], bindings["binding_target"]
	shotRevision, shotRevisionErr := strconv.Atoi(shot.ReferenceVersion)
	selectionRevision, selectionRevisionErr := strconv.Atoi(selection.ReferenceVersion)
	targetRevision, targetRevisionErr := strconv.Atoi(target.ReferenceVersion)
	if shotRevisionErr != nil || shotRevision < 1 || selectionRevisionErr != nil || selectionRevision != 1 ||
		targetRevisionErr != nil || targetRevision < 1 ||
		shot.ValueType != "production_shot" || selection.ValueType != "generation_candidate_selection" ||
		target.ValueType != "production_shot_image_binding_target" || target.ReferenceID != shot.ReferenceID ||
		shot.SourceKind != domain.NodeInputSourceNodeOutput || selection.SourceKind != domain.NodeInputSourceNodeOutput ||
		target.SourceKind != domain.NodeInputSourceNodeOutput || shot.SourceNodeID == "" || selection.SourceNodeID == "" ||
		target.SourceNodeID == "" || target.SourceNodeID != shot.SourceNodeID || shot.SourcePort != "shot" ||
		target.SourcePort != "binding_target" || selection.SourcePort == "" ||
		!workflowContentHashPattern.MatchString(shot.ContentHash) ||
		!workflowContentHashPattern.MatchString(selection.ContentHash) ||
		!workflowContentHashPattern.MatchString(target.ContentHash) {
		return domain.NodeExecutorResult{}, errors.New("Shot image binding target inputs have drifted")
	}
	result, err := executor.bindings.BindSelectedImageAtTarget(ctx, storyboardapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, storyboardapp.BindSelectedImageAtTargetCommand{
		ShotID: shot.ReferenceID, ExpectedShotRevision: shotRevision,
		ExpectedShotContentHash: shot.ContentHash, CandidateSelectionID: selection.ReferenceID,
		ExpectedCurrentRevision: targetRevision - 1, ExpectedBindingTargetHash: target.ContentHash,
		IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	binding := result.Binding
	if binding.WorkspaceID != command.WorkspaceID || binding.ProjectID != command.ProjectID ||
		binding.ShotID != shot.ReferenceID || binding.ShotRevision != shotRevision ||
		binding.ShotContentHash != shot.ContentHash || binding.CandidateSelectionID != selection.ReferenceID ||
		binding.CandidateSelectionRevision != selectionRevision ||
		binding.CandidateSelectionContentHash != selection.ContentHash || binding.Revision != targetRevision ||
		!workflowContentHashPattern.MatchString(binding.ContentHash) || binding.CreatedBy != command.InitiatorUserID ||
		result.Receipt.Operation != "storyboard.shot.bind_selected_image" ||
		result.Receipt.ResourceID != binding.ID || result.Receipt.CreatedBy != command.InitiatorUserID {
		return domain.NodeExecutorResult{}, errors.New("Shot image binding does not match frozen target input")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "binding", ValueType: "production_shot_image_binding", ReferenceID: binding.ID,
			ReferenceVersion: strconv.Itoa(binding.Revision), ContentHash: binding.ContentHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeStoryboardExport(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.storyboards == nil {
		return domain.NodeExecutorResult{}, errors.New("storyboard export workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "export" ||
		command.OutputPorts[0].ValueType != "storyboard_export" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid storyboard export node contract")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 0 {
		return domain.NodeExecutorResult{}, errors.New("invalid storyboard export node config")
	}
	binding := input.Bindings[0]
	expectedRevision, err := strconv.Atoi(binding.ReferenceVersion)
	if err != nil || expectedRevision < 1 || binding.Port != "storyboards" || binding.ValueType != "storyboards" ||
		binding.SourceKind != domain.NodeInputSourceNodeOutput || binding.SourcePort != "storyboards" ||
		strings.TrimSpace(binding.SourceNodeID) == "" || len(binding.ContentHash) != 64 {
		return domain.NodeExecutorResult{}, errors.New("applied storyboard input has drifted")
	}
	exportSet, err := executor.storyboards.CreateExportSet(ctx, storyboardapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, storyboardapp.CreateExportSetCommand{
		SetID: binding.ReferenceID, ExpectedRevision: expectedRevision,
		ExpectedResultHash: binding.ContentHash, IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if exportSet.WorkspaceID != command.WorkspaceID || exportSet.ProjectID != command.ProjectID ||
		exportSet.DraftSetID != binding.ReferenceID || exportSet.DraftSetRevision != expectedRevision ||
		exportSet.Status != "succeeded" || exportSet.Revision != 1 || len(exportSet.Exports) == 0 ||
		len(exportSet.ContentHash) != 64 || exportSet.CreatedBy != command.InitiatorUserID {
		return domain.NodeExecutorResult{}, errors.New("Storyboard Export Set does not match workflow input")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "export", ValueType: "storyboard_export", ReferenceID: exportSet.ID,
			ReferenceVersion: strconv.Itoa(exportSet.Revision), ContentHash: exportSet.ContentHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeStoryGraphCompile(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.planningOwners == nil || executor.storygraphs == nil {
		return domain.NodeExecutorResult{}, errors.New("StoryGraph workflow owners are unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "storygraph" ||
		command.OutputPorts[0].ValueType != "storygraph_version" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid StoryGraph compiler node contract")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 0 {
		return domain.NodeExecutorResult{}, errors.New("invalid StoryGraph compiler node config")
	}
	binding := input.Bindings[0]
	if binding.Port != "structures" || binding.ValueType != "planning_owner_set" ||
		binding.SourceKind != domain.NodeInputSourceNodeOutput || binding.SourcePort != "structures" ||
		strings.TrimSpace(binding.SourceNodeID) == "" || binding.ReferenceVersion != "1" ||
		!workflowContentHashPattern.MatchString(binding.ContentHash) {
		return domain.NodeExecutorResult{}, errors.New("Planning owner set input has drifted")
	}
	actor := planningapp.Actor{UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion}
	owners, err := executor.planningOwners.GetPlanningOwnerSet(ctx, actor, binding.ReferenceID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if owners.Set.ID != binding.ReferenceID || owners.Set.WorkspaceID != command.WorkspaceID ||
		owners.Set.ProjectID != command.ProjectID || owners.Set.ContentHash != binding.ContentHash ||
		len(owners.Set.Structures) == 0 || len(owners.Set.Structures) != len(owners.Structures) ||
		!workflowContentHashPattern.MatchString(owners.Set.BibleContentHash) {
		return domain.NodeExecutorResult{}, errors.New("Planning owner set does not match workflow input")
	}
	required := make([]storygraph.OwnerHeadRef, len(owners.Set.Structures))
	for index, reference := range owners.Set.Structures {
		if reference.Revision < 1 || !workflowContentHashPattern.MatchString(reference.ResultHash) {
			return domain.NodeExecutorResult{}, errors.New("Planning owner set contains an invalid Structure")
		}
		required[index] = storygraph.OwnerHeadRef{
			OwnerKind: "production/planning", OwnerLogicalID: reference.EpisodeID,
			OwnerVersionID: reference.StructureID, OwnerRevision: int64(reference.Revision), ContentHash: reference.ResultHash,
		}
	}
	compiled, err := executor.storygraphs.CompileOwnerSet(ctx, storygraphapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, storygraphapp.CompileOwnerSetCommand{
		ProjectID: command.ProjectID, OwnerSetID: owners.Set.ID, OwnerSetHash: owners.Set.ContentHash,
		RequiredBibleVersionID: owners.Set.BibleVersionID, RequiredBibleHash: owners.Set.BibleContentHash,
		RequiredOwners: required, IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if compiled.Version.WorkspaceID != command.WorkspaceID || compiled.Version.ProjectID != command.ProjectID ||
		compiled.Version.Status != "published" || compiled.Version.VersionNo < 1 ||
		compiled.Head.CurrentVersionID != compiled.Version.ID || compiled.Head.CurrentContentHash != compiled.Version.ContentHash ||
		compiled.Receipt.ResourceID != compiled.Version.ID || compiled.Receipt.CreatedBy != command.InitiatorUserID {
		return domain.NodeExecutorResult{}, errors.New("StoryGraph publication does not match workflow input")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "storygraph", ValueType: "storygraph_version", ReferenceID: compiled.Version.ID,
			ReferenceVersion: strconv.FormatInt(compiled.Version.VersionNo, 10), ContentHash: compiled.Version.ContentHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeStoryboardDraft(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.storyboards == nil {
		return domain.NodeExecutorResult{}, errors.New("storyboard workflow owners are unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "candidate" ||
		command.OutputPorts[0].ValueType != "storyboard_intent_candidate_set" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid storyboard draft node contract")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 0 {
		return domain.NodeExecutorResult{}, errors.New("invalid storyboard draft node config")
	}
	binding := input.Bindings[0]
	expectedVersion, err := strconv.ParseInt(binding.ReferenceVersion, 10, 64)
	if err != nil || expectedVersion < 1 || binding.Port != "storygraph" || binding.ValueType != "storygraph_version" ||
		binding.SourceKind != domain.NodeInputSourceNodeOutput || binding.SourcePort != "storygraph" ||
		strings.TrimSpace(binding.SourceNodeID) == "" || !workflowContentHashPattern.MatchString(binding.ContentHash) {
		return domain.NodeExecutorResult{}, errors.New("Storyboard Draft StoryGraph input has drifted")
	}
	set, err := executor.storyboards.CreateSet(ctx, storyboardapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, storyboardapp.CreateSetCommand{
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		GraphVersionID: binding.ReferenceID, GraphVersionNo: expectedVersion,
		GraphContentHash: binding.ContentHash, IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	set, err = executor.storyboards.RefreshSet(ctx, storyboardapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, set.ID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if set.WorkspaceID != command.WorkspaceID || set.ProjectID != command.ProjectID ||
		set.WorkflowRunID != command.WorkflowRunID || set.NodeRunID != command.NodeRunID ||
		set.GraphVersionID != binding.ReferenceID || set.GraphVersionNo != expectedVersion ||
		set.GraphContentHash != binding.ContentHash || set.CreatedBy != command.InitiatorUserID || len(set.Batches) == 0 {
		return domain.NodeExecutorResult{}, errors.New("storyboard draft set does not match workflow input")
	}
	switch set.Status {
	case "queued":
		return domain.NodeExecutorResult{Status: "RETRYING"}, nil
	case "needs_asset":
		if set.ResultHash == nil || len(*set.ResultHash) != 64 || set.Revision != 2 ||
			set.CandidateRevisionID == nil || set.CandidateRevisionHash == nil ||
			*set.ResultHash != *set.CandidateRevisionHash {
			return domain.NodeExecutorResult{}, errors.New("storyboard draft set candidate is incomplete")
		}
	default:
		return domain.NodeExecutorResult{}, errors.New("storyboard draft set candidate is unavailable")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "candidate", ValueType: "storyboard_intent_candidate_set", ReferenceID: *set.CandidateRevisionID,
			ReferenceVersion: "1", ContentHash: *set.CandidateRevisionHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
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

func (executor *NodeExecutor) executeBibleMaterialization(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.bibles == nil {
		return domain.NodeExecutorResult{}, errors.New("Production Bible materialization owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "materialization" ||
		command.OutputPorts[0].ValueType != "production_bible_materialization" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid Production Bible materialization node contract")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 0 {
		return domain.NodeExecutorResult{}, errors.New("invalid Production Bible materialization node config")
	}
	binding := input.Bindings[0]
	version, parseErr := strconv.Atoi(binding.ReferenceVersion)
	if parseErr != nil || version < 1 || binding.Port != "bible" || binding.ValueType != "production_bible_version" ||
		binding.SourceKind != domain.NodeInputSourceNodeOutput || binding.SourcePort != "bible" ||
		strings.TrimSpace(binding.SourceNodeID) == "" || !workflowContentHashPattern.MatchString(binding.ContentHash) {
		return domain.NodeExecutorResult{}, errors.New("Production Bible Version input has drifted")
	}
	result, err := executor.bibles.MaterializeConfirmedBible(ctx, bibleapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, bibleapp.MaterializeCommand{
		BibleVersionID: binding.ReferenceID, ExpectedVersion: version,
		ExpectedContentHash: binding.ContentHash, IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if result.Materialization.BibleVersionID != binding.ReferenceID ||
		result.Materialization.BibleVersionHash != binding.ContentHash ||
		!workflowContentHashPattern.MatchString(result.Materialization.ContentHash) ||
		result.Receipt.ResourceID != binding.ReferenceID || result.Receipt.Operation != "production_bible.materialize_confirmed" {
		return domain.NodeExecutorResult{}, errors.New("Production Bible materialization result does not match workflow input")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "materialization", ValueType: "production_bible_materialization",
			ReferenceID: binding.ReferenceID, ReferenceVersion: binding.ReferenceVersion,
			ContentHash: result.Materialization.ContentHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeEpisodeSegmentation(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.segments == nil {
		return domain.NodeExecutorResult{}, errors.New("Episode segmentation workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 2 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "candidate" ||
		command.OutputPorts[0].ValueType != "episode_segmentation_candidate" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid Episode segmentation node contract")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 0 {
		return domain.NodeExecutorResult{}, errors.New("invalid Episode segmentation node config")
	}
	bindings := make(map[string]domain.NodeInputBinding, len(input.Bindings))
	for _, binding := range input.Bindings {
		bindings[binding.Port] = binding
	}
	evidence, materialization := bindings["evidence"], bindings["materialization"]
	bibleVersion, versionErr := strconv.Atoi(materialization.ReferenceVersion)
	if versionErr != nil || bibleVersion < 1 ||
		evidence.ValueType != "source_evidence_candidate" || evidence.SourceKind != domain.NodeInputSourceNodeOutput ||
		evidence.SourcePort != "evidence" || strings.TrimSpace(evidence.SourceNodeID) == "" ||
		materialization.ValueType != "production_bible_materialization" ||
		materialization.SourceKind != domain.NodeInputSourceNodeOutput ||
		materialization.SourcePort != "materialization" || strings.TrimSpace(materialization.SourceNodeID) == "" ||
		!workflowContentHashPattern.MatchString(evidence.ContentHash) ||
		!workflowContentHashPattern.MatchString(materialization.ContentHash) {
		return domain.NodeExecutorResult{}, errors.New("Episode segmentation exact inputs have drifted")
	}
	var scriptID, scriptHash string
	foundScript := false
	for _, reference := range input.FrozenInputs {
		if reference.Kind == "script_revision" {
			if foundScript {
				return domain.NodeExecutorResult{}, errors.New("Episode segmentation has multiple frozen scripts")
			}
			scriptID, scriptHash, foundScript = reference.ID, reference.Hash, true
		}
	}
	if !foundScript || !workflowContentHashPattern.MatchString(scriptHash) {
		return domain.NodeExecutorResult{}, errors.New("Episode segmentation frozen script is missing")
	}
	state, err := executor.segments.Ensure(ctx, bibleapp.EpisodeSegmentationCommand{
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		DocumentRevisionID: scriptID, DocumentRevisionHash: scriptHash,
		EvidenceCandidateRevisionID:   evidence.ReferenceID,
		EvidenceCandidateRevisionHash: evidence.ContentHash,
		BibleVersionID:                materialization.ReferenceID, BibleVersion: bibleVersion,
		MaterializationHash: materialization.ContentHash,
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	switch state.Status {
	case "pending":
		return domain.NodeExecutorResult{Status: "RETRYING"}, nil
	case "failed":
		return domain.NodeExecutorResult{}, errors.New("Episode segmentation candidate generation failed")
	case "ready":
		if _, parseErr := uuid.Parse(state.CandidateRevisionID); parseErr != nil ||
			state.CandidateRevisionNo < 1 || !workflowContentHashPattern.MatchString(state.CandidateRevisionHash) {
			return domain.NodeExecutorResult{}, errors.New("Episode segmentation candidate is incomplete")
		}
	default:
		return domain.NodeExecutorResult{}, errors.New("Episode segmentation returned an invalid status")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "candidate", ValueType: "episode_segmentation_candidate",
			ReferenceID: state.CandidateRevisionID, ReferenceVersion: strconv.FormatInt(state.CandidateRevisionNo, 10),
			ContentHash: state.CandidateRevisionHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func (executor *NodeExecutor) executeEpisodeAnalysis(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.episodes == nil {
		return domain.NodeExecutorResult{}, errors.New("Episode analysis workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 2 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "candidate" ||
		command.OutputPorts[0].ValueType != "episode_planning_candidate_set" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid Episode analysis node contract")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 0 {
		return domain.NodeExecutorResult{}, errors.New("invalid Episode analysis node config")
	}
	bindings := make(map[string]domain.NodeInputBinding, len(input.Bindings))
	for _, binding := range input.Bindings {
		bindings[binding.Port] = binding
	}
	episodes, materialization := bindings["episodes"], bindings["materialization"]
	bibleVersion, versionErr := strconv.Atoi(materialization.ReferenceVersion)
	if versionErr != nil || bibleVersion < 1 ||
		episodes.Port != "episodes" || episodes.ValueType != "episode_set" ||
		episodes.SourceKind != domain.NodeInputSourceNodeOutput || episodes.SourcePort != "episodes" ||
		strings.TrimSpace(episodes.SourceNodeID) == "" ||
		materialization.Port != "materialization" || materialization.ValueType != "production_bible_materialization" ||
		materialization.SourceKind != domain.NodeInputSourceNodeOutput || materialization.SourcePort != "materialization" ||
		strings.TrimSpace(materialization.SourceNodeID) == "" ||
		!workflowContentHashPattern.MatchString(episodes.ContentHash) ||
		!workflowContentHashPattern.MatchString(materialization.ContentHash) {
		return domain.NodeExecutorResult{}, errors.New("Episode analysis exact inputs have drifted")
	}
	state, err := executor.episodes.Ensure(ctx, planningapp.EpisodeAnalysisCommand{
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		EpisodeSetID: episodes.ReferenceID, EpisodeSetHash: episodes.ContentHash,
		BibleVersionID: materialization.ReferenceID, BibleVersion: bibleVersion,
		MaterializationHash: materialization.ContentHash,
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	switch state.Status {
	case "pending":
		return domain.NodeExecutorResult{Status: "RETRYING"}, nil
	case "failed":
		return domain.NodeExecutorResult{}, errors.New("Episode analysis candidate generation failed")
	case "ready":
		if _, parseErr := uuid.Parse(state.CandidateRevisionID); parseErr != nil ||
			state.CandidateRevisionNo < 1 || !workflowContentHashPattern.MatchString(state.CandidateRevisionHash) {
			return domain.NodeExecutorResult{}, errors.New("Episode analysis candidate set is incomplete")
		}
	default:
		return domain.NodeExecutorResult{}, errors.New("Episode analysis returned an invalid status")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "candidate", ValueType: "episode_planning_candidate_set",
			ReferenceID: state.CandidateRevisionID, ReferenceVersion: strconv.FormatInt(state.CandidateRevisionNo, 10),
			ContentHash: state.CandidateRevisionHash,
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
