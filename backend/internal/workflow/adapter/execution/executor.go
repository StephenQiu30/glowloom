package execution

import (
	"context"
	"errors"

	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type NodeExecutor struct {
	production     workflowapp.NodeExecutor
	generation     workflowapp.NodeExecutor
	referenceCalls workflowapp.NodeExecutor
}

func NewNodeExecutor(production, generation, referenceCalls workflowapp.NodeExecutor) (*NodeExecutor, error) {
	if production == nil || generation == nil {
		return nil, errors.New("workflow executor owners are required")
	}
	return &NodeExecutor{production: production, generation: generation, referenceCalls: referenceCalls}, nil
}

func (executor *NodeExecutor) Execute(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor == nil || executor.production == nil || executor.generation == nil {
		return domain.NodeExecutorResult{}, errors.New("workflow executor owners are unavailable")
	}
	switch command.Executor {
	case "activity.reference_image_call":
		if executor.referenceCalls == nil {
			return domain.NodeExecutorResult{}, errors.New("Reference call workflow owner is unavailable")
		}
		return executor.referenceCalls.Execute(ctx, command)
	case "workflow.input.generation_candidate_set", "activity.reference_asset_generation":
		return executor.generation.Execute(ctx, command)
	case "workflow.input.script_revision",
		"workflow.input.script_source",
		"activity.script_span_proposal",
		"activity.scene_fact_extraction",
		"activity.identity_resolution",
		"activity.production_entity_derivation",
		"activity.scene_occurrence_binding",
		"activity.interaction_continuity_reconciliation",
		"activity.production_world_assembly",
		"activity.production_storygraph_projection",
		"activity.project_preset_selection",
		"activity.resolve_visual_foundation",
		"activity.plan_reference_assets",
		"activity.compile_reference_briefs",
		"activity.source_evidence",
		"activity.story_analysis",
		"activity.story_review",
		"activity.production_bible",
		"activity.production_bible_materialization",
		"activity.episode_analysis",
		"activity.episode_segmentation",
		"activity.episode_plan",
		"activity.episode_structure",
		"activity.storygraph_compile",
		"activity.storyboard_draft",
		"activity.storyboard_export",
		"workflow.input.production_shot",
		"activity.production_shot_image_binding",
		"workflow.input.production_shot_binding_target",
		"activity.production_shot_image_binding_at_target":
		return executor.production.Execute(ctx, command)
	default:
		return domain.NodeExecutorResult{}, errors.New("unsupported Workflow node owner")
	}
}

var _ workflowapp.NodeExecutor = (*NodeExecutor)(nil)
