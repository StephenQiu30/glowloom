package workers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

const (
	SupervisorNodeName   = "supervisor"
	ScriptWorkerNode     = "script_worker"
	StoryboardWorkerNode = "storyboard_worker"
	ShotWorkerNode       = "shot_worker"
	AudioWorkerNode      = "audio_worker"
	ToolsNodeName        = "tools_node"
)

// PlannerFunc decomposes a high-level goal into an ordered execution plan with dependencies.
type PlannerFunc func(ctx context.Context, goal string) (*graph.ExecutionPlan, error)

// Supervisor coordinates multi-agent creative workflows and routes tasks to specialized workers.
type Supervisor struct {
	requirePlanApproval bool
	planner             PlannerFunc
}

// SupervisorOption configures the Supervisor instance.
type SupervisorOption func(*Supervisor)

// WithPlanApproval requires user confirmation before starting execution of the generated plan.
func WithPlanApproval(require bool) SupervisorOption {
	return func(s *Supervisor) {
		s.requirePlanApproval = require
	}
}

// WithCustomPlanner overrides the default heuristic goal decomposition.
func WithCustomPlanner(planner PlannerFunc) SupervisorOption {
	return func(s *Supervisor) {
		s.planner = planner
	}
}

// NewSupervisor instantiates a Supervisor node handler and router.
func NewSupervisor(opts ...SupervisorOption) *Supervisor {
	s := &Supervisor{
		requirePlanApproval: false,
		planner:             DefaultDecomposeGoal,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NodeHandler implements graph.NodeHandler for the Supervisor node.
func (s *Supervisor) NodeHandler(ctx context.Context, state *graph.AgentState) (*graph.AgentState, error) {
	if state == nil {
		return nil, fmt.Errorf("agent state cannot be nil")
	}

	// 1. Initial Plan Generation
	if state.Plan == nil {
		goalText := state.Goal
		if goalText == "" && len(state.Messages) > 0 {
			// Extract goal from the latest user message
			for i := len(state.Messages) - 1; i >= 0; i-- {
				if state.Messages[i].Role == "user" {
					goalText = state.Messages[i].Content
					break
				}
			}
		}

		plan, err := s.planner(ctx, goalText)
		if err != nil {
			return state, fmt.Errorf("decompose goal failed: %w", err)
		}
		state.Plan = plan

		state.Messages = append(state.Messages, graph.Message{
			ID:        uuid.New(),
			Role:      "assistant",
			Content:   fmt.Sprintf("Supervisor: 已根据创作目标生成执行计划（共 %d 个阶段）。", len(plan.Steps)),
			Timestamp: time.Now().UTC(),
		})

		// If plan approval is mandated, interrupt execution for human review
		if s.requirePlanApproval && !state.Plan.Approved {
			state.Interrupt("plan_approval_required")
			return state, nil
		}
	}

	// 2. If resuming from a plan approval interrupt
	if s.requirePlanApproval && !state.Plan.Approved {
		state.Plan.Approved = true
	}

	// 3. If any interrupt is active (e.g. proposal interrupt yielded from downstream tool), halt
	if state.Interrupted {
		return state, nil
	}

	// 4. Progress Tracking: Inspect and prepare next ready step
	nextStep := state.NextReadyStep()
	if nextStep != nil {
		nextStep.Status = "in_progress"
		state.Messages = append(state.Messages, graph.Message{
			ID:        uuid.New(),
			Role:      "assistant",
			Content:   fmt.Sprintf("Supervisor: 调度专业智能体 [%s] 执行任务: %s", nextStep.Agent, nextStep.Title),
			Timestamp: time.Now().UTC(),
		})
	}

	return state, nil
}

// Router implements graph.ConditionalEdgeRouter to direct flow to the appropriate worker.
func (s *Supervisor) Router(ctx context.Context, state *graph.AgentState) (string, error) {
	if state == nil {
		return graph.FinishPoint, nil
	}

	if state.Interrupted {
		return graph.FinishPoint, nil
	}

	if state.AllStepsCompleted() {
		return graph.FinishPoint, nil
	}

	if state.Plan == nil {
		return graph.FinishPoint, fmt.Errorf("cannot route without an active execution plan")
	}

	// First priority: any active in_progress step
	for _, step := range state.Plan.Steps {
		if step.Status == "in_progress" {
			return step.Agent, nil
		}
	}

	// Second priority: next ready step
	next := state.NextReadyStep()
	if next != nil {
		next.Status = "in_progress"
		return next.Agent, nil
	}

	// Check if any steps failed or are deadlocked
	hasPending := false
	for _, step := range state.Plan.Steps {
		if step.Status == "pending" {
			hasPending = true
		}
		if step.Status == "failed" {
			return graph.FinishPoint, fmt.Errorf("plan step %q failed: %s", step.ID, step.Result)
		}
	}

	if hasPending {
		return graph.FinishPoint, fmt.Errorf("plan steps are deadlocked with unmet dependencies")
	}

	return graph.FinishPoint, nil
}

// DefaultDecomposeGoal splits user intent into the standard 4-phase creative pipeline.
func DefaultDecomposeGoal(ctx context.Context, goal string) (*graph.ExecutionPlan, error) {
	trimmed := strings.TrimSpace(goal)
	if trimmed == "" {
		trimmed = "全流程短剧创作"
	}

	// Check for scoped sub-requests
	if strings.Contains(trimmed, "仅剧本") || strings.Contains(trimmed, "剧本分析") {
		return &graph.ExecutionPlan{
			Revision: 1,
			Goal:     trimmed,
			Steps: []graph.PlanStepState{
				{
					ID:     "step_script",
					Agent:  ScriptWorkerNode,
					Title:  "剧本拆解与人物歧义分析",
					Status: "pending",
				},
			},
		}, nil
	}

	// Standard 4-step creative pipeline: Script -> Storyboard -> Shot -> Audio
	return &graph.ExecutionPlan{
		Revision: 1,
		Goal:     trimmed,
		Steps: []graph.PlanStepState{
			{
				ID:        "step_script",
				Agent:     ScriptWorkerNode,
				Title:     "长剧本场景人物拆解与歧义标记",
				Status:    "pending",
				DependsOn: nil,
			},
			{
				ID:        "step_storyboard",
				Agent:     StoryboardWorkerNode,
				Title:     "分镜镜头语言设计与机位编排",
				Status:    "pending",
				DependsOn: []string{"step_script"},
			},
			{
				ID:        "step_shot",
				Agent:     ShotWorkerNode,
				Title:     "镜头导演视频生成提示词与画面提案",
				Status:    "pending",
				DependsOn: []string{"step_storyboard"},
			},
			{
				ID:        "step_audio",
				Agent:     AudioWorkerNode,
				Title:     "后期剪辑配音、音效与时间线对齐",
				Status:    "pending",
				DependsOn: []string{"step_shot"},
			},
		},
	}, nil
}
