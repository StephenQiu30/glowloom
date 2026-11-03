package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

func TestStateGraphLinearExecution(t *testing.T) {
	sg := graph.NewStateGraph()

	err := sg.AddNode("node1", func(ctx context.Context, s *graph.AgentState) (*graph.AgentState, error) {
		s.Messages = append(s.Messages, graph.Message{Content: "step 1"})
		return s, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = sg.AddNode("node2", func(ctx context.Context, s *graph.AgentState) (*graph.AgentState, error) {
		s.Messages = append(s.Messages, graph.Message{Content: "step 2"})
		return s, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := sg.SetEntryPoint("node1"); err != nil {
		t.Fatal(err)
	}
	if err := sg.AddEdge("node1", "node2"); err != nil {
		t.Fatal(err)
	}
	if err := sg.AddEdge("node2", graph.FinishPoint); err != nil {
		t.Fatal(err)
	}

	compiled, err := sg.Compile(nil)
	if err != nil {
		t.Fatal(err)
	}

	initialState := &graph.AgentState{
		RunID: uuid.New(),
	}

	finalState, err := compiled.Invoke(t.Context(), initialState)
	if err != nil {
		t.Fatal(err)
	}

	if finalState.Status != "completed" {
		t.Fatalf("expected completed status, got %s", finalState.Status)
	}
	if len(finalState.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(finalState.Messages))
	}
	if finalState.Messages[0].Content != "step 1" || finalState.Messages[1].Content != "step 2" {
		t.Fatalf("unexpected messages content: %+v", finalState.Messages)
	}
}

func TestStateGraphProposalYieldInterrupt(t *testing.T) {
	sg := graph.NewStateGraph()

	_ = sg.AddNode("planner", func(ctx context.Context, s *graph.AgentState) (*graph.AgentState, error) {
		s.Messages = append(s.Messages, graph.Message{Content: "planning"})
		return s, nil
	})

	_ = sg.AddNode("proposer", func(ctx context.Context, s *graph.AgentState) (*graph.AgentState, error) {
		s.Proposals = append(s.Proposals, graph.Proposal{
			ID:      uuid.New(),
			Action:  "generation.submit",
			Summary: "生成雨夜短片镜头 1",
		})
		// Model yields on proposal (BeefTV Proposal Yield pattern)
		s.Interrupt("generation_proposal_approval_required")
		return s, nil
	})

	_ = sg.AddNode("executor", func(ctx context.Context, s *graph.AgentState) (*graph.AgentState, error) {
		s.Messages = append(s.Messages, graph.Message{Content: "executed after approval"})
		return s, nil
	})

	_ = sg.SetEntryPoint("planner")
	_ = sg.AddEdge("planner", "proposer")
	_ = sg.AddEdge("proposer", "executor")
	_ = sg.AddEdge("executor", graph.FinishPoint)

	compiled, err := sg.Compile(nil)
	if err != nil {
		t.Fatal(err)
	}

	initialState := &graph.AgentState{
		RunID: uuid.New(),
	}

	finalState, err := compiled.Invoke(t.Context(), initialState)
	if err != nil {
		t.Fatal(err)
	}

	// Should be interrupted and suspended, NOT completed!
	if finalState.Status != "suspended_for_approval" {
		t.Fatalf("expected suspended_for_approval, got %s", finalState.Status)
	}
	if !finalState.Interrupted {
		t.Fatal("expected Interrupted to be true")
	}
	if finalState.CurrentNode != "proposer" {
		t.Fatalf("expected current node 'proposer', got %s", finalState.CurrentNode)
	}
	// "executor" must NOT have run!
	for _, msg := range finalState.Messages {
		if msg.Content == "executed after approval" {
			t.Fatal("executor should not have executed before approval")
		}
	}
}

func TestStateGraphTurnBudgetWatchdog(t *testing.T) {
	sg := graph.NewStateGraph()

	// An infinite loop between ping and pong
	_ = sg.AddNode("ping", func(ctx context.Context, s *graph.AgentState) (*graph.AgentState, error) {
		return s, nil
	})
	_ = sg.AddNode("pong", func(ctx context.Context, s *graph.AgentState) (*graph.AgentState, error) {
		return s, nil
	})

	_ = sg.SetEntryPoint("ping")
	_ = sg.AddEdge("ping", "pong")
	_ = sg.AddEdge("pong", "ping")

	compiled, err := sg.Compile(nil)
	if err != nil {
		t.Fatal(err)
	}

	initialState := &graph.AgentState{
		RunID:        uuid.New(),
		MaxTurnSteps: 10, // Small limit for testing
	}

	finalState, err := compiled.Invoke(t.Context(), initialState)
	if !errors.Is(err, graph.ErrMaxStepsExceeded) {
		t.Fatalf("expected ErrMaxStepsExceeded, got %v", err)
	}
	if finalState.Status != "failed" {
		t.Fatalf("expected failed status, got %s", finalState.Status)
	}
	if finalState.TurnStepCount != 10 {
		t.Fatalf("expected exactly 10 turn steps, got %d", finalState.TurnStepCount)
	}
}

func TestWriteChainExtension(t *testing.T) {
	var wc graph.WriteChainState

	// First write: CAS expects revision 1, increments to 2
	if !wc.Extend(1, 2) {
		t.Fatal("expected first extend to succeed")
	}
	if wc.Start != 1 || wc.Head != 2 || wc.ExpectedRevision != 2 {
		t.Fatalf("unexpected write chain state: %+v", wc)
	}

	// Sequential in-turn write: CAS expects revision 2, increments to 3
	if !wc.Extend(2, 3) {
		t.Fatal("expected sequential extend to succeed")
	}
	if wc.Head != 3 || wc.ExpectedRevision != 3 {
		t.Fatalf("unexpected write chain head: %d", wc.Head)
	}

	// Stale write outside chain (e.g. concurrent external write conflict): expects 1 instead of 3
	if wc.Extend(1, 4) {
		t.Fatal("expected stale revision outside chain to fail")
	}
}
