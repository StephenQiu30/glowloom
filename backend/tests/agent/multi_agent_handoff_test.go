package agent_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/application/workers"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

func TestMultiAgentFullPipelineWithProposalYield(t *testing.T) {
	ctx := context.Background()
	g := graph.NewStateGraph()
	cp := newMockCheckpointer()

	sup := workers.NewSupervisor(workers.WithPlanApproval(false))
	scriptW := workers.NewScriptWorker()
	storyboardW := workers.NewStoryboardWorker()
	shotW := workers.NewShotWorker()
	audioW := workers.NewAudioWorker()

	// Register all nodes
	if err := g.AddNode(workers.SupervisorNodeName, sup.NodeHandler); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNode(workers.ScriptWorkerNode, scriptW.NodeHandler); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNode(workers.StoryboardWorkerNode, storyboardW.NodeHandler); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNode(workers.ShotWorkerNode, shotW.NodeHandler); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNode(workers.AudioWorkerNode, audioW.NodeHandler); err != nil {
		t.Fatal(err)
	}

	// Dynamic routing from supervisor
	if err := g.AddConditionalEdge(workers.SupervisorNodeName, sup.Router); err != nil {
		t.Fatal(err)
	}

	// Workers loop back to supervisor
	if err := g.AddEdge(workers.ScriptWorkerNode, workers.SupervisorNodeName); err != nil {
		t.Fatal(err)
	}
	if err := g.AddEdge(workers.StoryboardWorkerNode, workers.SupervisorNodeName); err != nil {
		t.Fatal(err)
	}
	if err := g.AddEdge(workers.ShotWorkerNode, workers.SupervisorNodeName); err != nil {
		t.Fatal(err)
	}
	if err := g.AddEdge(workers.AudioWorkerNode, workers.SupervisorNodeName); err != nil {
		t.Fatal(err)
	}

	if err := g.SetEntryPoint(workers.SupervisorNodeName); err != nil {
		t.Fatal(err)
	}

	cg, err := g.Compile(cp)
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}

	harness := application.NewProposalHarness(cg, cp)
	runID := uuid.New()

	initialState := &graph.AgentState{
		RunID:        runID,
		Goal:         "全流程制作短剧：雨夜剑客对决",
		MaxTurnSteps: 40,
	}

	// 1. Initial Invoke: Executes ScriptWorker -> StoryboardWorker -> ShotWorker (yields interrupt)
	state, err := cg.Invoke(ctx, initialState)
	if err != nil {
		t.Fatalf("invoke error: %v", err)
	}

	// Must be suspended for shot generation approval
	if state.Status != "suspended_for_approval" {
		t.Fatalf("expected status 'suspended_for_approval', got %q", state.Status)
	}
	if !state.Interrupted {
		t.Fatalf("expected Interrupted to be true")
	}

	// Verify that script and storyboard steps completed, but shot is in progress and audio has NOT run
	scriptStep := state.StepByID("step_script")
	if scriptStep == nil || scriptStep.Status != "completed" {
		t.Errorf("expected script step to be completed, got %+v", scriptStep)
	}

	storyboardStep := state.StepByID("step_storyboard")
	if storyboardStep == nil || storyboardStep.Status != "completed" {
		t.Errorf("expected storyboard step to be completed, got %+v", storyboardStep)
	}

	audioStep := state.StepByID("step_audio")
	if audioStep == nil || audioStep.Status != "pending" {
		t.Fatalf("audio step must still be pending before shot proposal approval! Got %+v", audioStep)
	}

	if len(state.PendingProposals()) != 1 {
		t.Fatalf("expected 1 pending proposal, got %d", len(state.PendingProposals()))
	}
	proposal := state.PendingProposals()[0]

	// 2. User reviews and approves the shot generation proposal
	resumedState, err := harness.HandleApprove(ctx, runID, proposal.ID, 10000000)
	if err != nil {
		t.Fatalf("handle approve error: %v", err)
	}

	// 3. Execution resumes: ShotWorker completes -> AudioWorker completes -> Supervisor finishes
	if resumedState.Status != "completed" {
		t.Errorf("expected final status 'completed', got %q", resumedState.Status)
	}
	if !resumedState.AllStepsCompleted() {
		t.Errorf("expected all steps to be completed")
	}

	finalShotStep := resumedState.StepByID("step_shot")
	if finalShotStep == nil || finalShotStep.Status != "completed" {
		t.Errorf("expected shot step completed, got %+v", finalShotStep)
	}

	finalAudioStep := resumedState.StepByID("step_audio")
	if finalAudioStep == nil || finalAudioStep.Status != "completed" {
		t.Errorf("expected audio step completed, got %+v", finalAudioStep)
	}
}
