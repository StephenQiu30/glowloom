package agent_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/application/workers"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

func TestSupervisorDecomposeGoal(t *testing.T) {
	ctx := context.Background()

	t.Run("StandardCreativeGoal", func(t *testing.T) {
		plan, err := workers.DefaultDecomposeGoal(ctx, "创作短剧：龙王归来第1集")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(plan.Steps) != 4 {
			t.Fatalf("expected 4 steps, got %d", len(plan.Steps))
		}
		if plan.Steps[0].Agent != workers.ScriptWorkerNode {
			t.Errorf("expected first step agent %s, got %s", workers.ScriptWorkerNode, plan.Steps[0].Agent)
		}
		if plan.Steps[1].Agent != workers.StoryboardWorkerNode {
			t.Errorf("expected second step agent %s, got %s", workers.StoryboardWorkerNode, plan.Steps[1].Agent)
		}
		if plan.Steps[2].Agent != workers.ShotWorkerNode {
			t.Errorf("expected third step agent %s, got %s", workers.ShotWorkerNode, plan.Steps[2].Agent)
		}
		if plan.Steps[3].Agent != workers.AudioWorkerNode {
			t.Errorf("expected fourth step agent %s, got %s", workers.AudioWorkerNode, plan.Steps[3].Agent)
		}
	})

	t.Run("ScopedScriptOnlyGoal", func(t *testing.T) {
		plan, err := workers.DefaultDecomposeGoal(ctx, "仅剧本分析与人物拆解")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(plan.Steps) != 1 {
			t.Fatalf("expected 1 step, got %d", len(plan.Steps))
		}
		if plan.Steps[0].Agent != workers.ScriptWorkerNode {
			t.Errorf("expected %s, got %s", workers.ScriptWorkerNode, plan.Steps[0].Agent)
		}
	})
}

func TestSupervisorWorkflowOrdering(t *testing.T) {
	ctx := context.Background()
	g := graph.NewStateGraph()
	sup := workers.NewSupervisor(workers.WithPlanApproval(false))

	// Register supervisor node
	if err := g.AddNode(workers.SupervisorNodeName, sup.NodeHandler); err != nil {
		t.Fatalf("add supervisor node: %v", err)
	}

	executionTrace := make([]string, 0)

	// Mock workers
	workersList := []string{
		workers.ScriptWorkerNode,
		workers.StoryboardWorkerNode,
		workers.ShotWorkerNode,
		workers.AudioWorkerNode,
	}

	for _, wName := range workersList {
		workerName := wName
		err := g.AddNode(workerName, func(ctx context.Context, state *graph.AgentState) (*graph.AgentState, error) {
			executionTrace = append(executionTrace, workerName)
			// Mark active step completed
			for i := range state.Plan.Steps {
				if state.Plan.Steps[i].Agent == workerName && state.Plan.Steps[i].Status == "in_progress" {
					state.Plan.Steps[i].Status = "completed"
					state.Plan.Steps[i].Result = "output from " + workerName
					break
				}
			}
			return state, nil
		})
		if err != nil {
			t.Fatalf("add worker node %s: %v", workerName, err)
		}
		// Each worker routes back to supervisor
		if err := g.AddEdge(workerName, workers.SupervisorNodeName); err != nil {
			t.Fatalf("add edge from %s to supervisor: %v", workerName, err)
		}
	}

	// Supervisor dynamically routes to next ready worker or finish
	if err := g.AddConditionalEdge(workers.SupervisorNodeName, sup.Router); err != nil {
		t.Fatalf("add conditional edge: %v", err)
	}

	if err := g.SetEntryPoint(workers.SupervisorNodeName); err != nil {
		t.Fatalf("set entry point: %v", err)
	}

	cg, err := g.Compile(nil)
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}

	initialState := &graph.AgentState{
		RunID:        uuid.New(),
		Goal:         "全流程剧本到成片制作",
		MaxTurnSteps: 40,
	}

	finalState, err := cg.Invoke(ctx, initialState)
	if err != nil {
		t.Fatalf("invoke graph: %v", err)
	}

	if finalState.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", finalState.Status)
	}

	if !finalState.AllStepsCompleted() {
		t.Errorf("expected all steps to be completed")
	}

	expectedTrace := []string{
		workers.ScriptWorkerNode,
		workers.StoryboardWorkerNode,
		workers.ShotWorkerNode,
		workers.AudioWorkerNode,
	}

	if len(executionTrace) != len(expectedTrace) {
		t.Fatalf("expected trace len %d, got %d: %v", len(expectedTrace), len(executionTrace), executionTrace)
	}

	for i := range expectedTrace {
		if executionTrace[i] != expectedTrace[i] {
			t.Errorf("step %d: expected %s, got %s", i, expectedTrace[i], executionTrace[i])
		}
	}
}

func TestSupervisorPlanApprovalInterrupt(t *testing.T) {
	ctx := context.Background()
	g := graph.NewStateGraph()
	sup := workers.NewSupervisor(workers.WithPlanApproval(true))
	cp := newMockCheckpointer()

	if err := g.AddNode(workers.SupervisorNodeName, sup.NodeHandler); err != nil {
		t.Fatalf("add supervisor node: %v", err)
	}

	workerCalled := false
	if err := g.AddNode(workers.ScriptWorkerNode, func(ctx context.Context, state *graph.AgentState) (*graph.AgentState, error) {
		workerCalled = true
		state.MarkStepCompleted("step_script", "done")
		return state, nil
	}); err != nil {
		t.Fatalf("add script node: %v", err)
	}

	if err := g.AddEdge(workers.ScriptWorkerNode, graph.FinishPoint); err != nil {
		t.Fatalf("add edge: %v", err)
	}

	if err := g.AddConditionalEdge(workers.SupervisorNodeName, sup.Router); err != nil {
		t.Fatalf("add conditional edge: %v", err)
	}

	if err := g.SetEntryPoint(workers.SupervisorNodeName); err != nil {
		t.Fatalf("set entry point: %v", err)
	}

	cg, err := g.Compile(cp)
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}

	runID := uuid.New()
	initialState := &graph.AgentState{
		RunID:        runID,
		Goal:         "仅剧本分析与人物拆解",
		MaxTurnSteps: 40,
	}

	// 1. Initial execution must interrupt for plan approval
	suspendedState, err := cg.Invoke(ctx, initialState)
	if err != nil {
		t.Fatalf("invoke error: %v", err)
	}

	if suspendedState.Status != "suspended_for_approval" {
		t.Fatalf("expected status 'suspended_for_approval', got %q", suspendedState.Status)
	}
	if suspendedState.InterruptReason != "plan_approval_required" {
		t.Errorf("expected reason 'plan_approval_required', got %q", suspendedState.InterruptReason)
	}
	if workerCalled {
		t.Fatalf("worker must NOT be called before plan approval!")
	}

	// 2. Resume graph with approval
	resumedState, err := cg.Resume(ctx, runID, func(s *graph.AgentState) {
		if s.Plan != nil {
			s.Plan.Approved = true
		}
	})
	if err != nil {
		t.Fatalf("resume error: %v", err)
	}

	if resumedState.Status != "completed" {
		t.Errorf("expected final status 'completed', got %q", resumedState.Status)
	}
	if !workerCalled {
		t.Fatalf("expected worker to be called after plan approval")
	}
}
