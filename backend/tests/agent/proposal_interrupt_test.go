package agent_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

func TestProposalYieldAndApprovalFlow(t *testing.T) {
	ctx := context.Background()
	g := graph.NewStateGraph()
	cp := newMockCheckpointer()

	proposalID := uuid.New()
	downstreamExecuted := false

	// Node 1: Worker that generates a Proposal (e.g. shot video generation proposal)
	err := g.AddNode("shot_worker", func(ctx context.Context, state *graph.AgentState) (*graph.AgentState, error) {
		// Worker generates a proposal costing 5_000_000 micros ($5)
		state.AddProposal(graph.Proposal{
			ID:                  proposalID,
			Action:              "shot.video.generate",
			Summary:             "生成第1镜头超清视频",
			EstimatedCostMicros: 5000000,
			Data:                json.RawMessage(`{"prompt":"雨夜街头霓虹特写","duration_seconds":5}`),
		})
		return state, nil
	})
	if err != nil {
		t.Fatalf("add shot_worker node: %v", err)
	}

	// Node 2: Downstream generative executor that should ONLY run if approved
	err = g.AddNode("generation_executor", func(ctx context.Context, state *graph.AgentState) (*graph.AgentState, error) {
		downstreamExecuted = true
		for _, p := range state.Proposals {
			if p.ID == proposalID {
				if !p.Approved {
					t.Errorf("generation_executor called with unapproved proposal!")
				}
			}
		}
		return state, nil
	})
	if err != nil {
		t.Fatalf("add generation_executor: %v", err)
	}

	// Router after shot_worker: inspect proposal decision
	err = g.AddConditionalEdge("shot_worker", func(ctx context.Context, state *graph.AgentState) (string, error) {
		for _, p := range state.Proposals {
			if p.ID == proposalID {
				if p.Approved {
					return "generation_executor", nil
				}
				if p.Status == "rejected" {
					return graph.FinishPoint, nil
				}
			}
		}
		return graph.FinishPoint, nil
	})
	if err != nil {
		t.Fatalf("add conditional edge: %v", err)
	}

	if err := g.AddEdge("generation_executor", graph.FinishPoint); err != nil {
		t.Fatalf("add edge: %v", err)
	}

	if err := g.SetEntryPoint("shot_worker"); err != nil {
		t.Fatalf("set entry point: %v", err)
	}

	cg, err := g.Compile(cp)
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}

	harness := application.NewProposalHarness(cg, cp)
	runID := uuid.New()

	initialState := &graph.AgentState{
		RunID:        runID,
		MaxTurnSteps: 40,
	}

	// Step 1: Initial invocation triggers proposal yield interrupt
	state, err := cg.Invoke(ctx, initialState)
	if err != nil {
		t.Fatalf("invoke error: %v", err)
	}

	if state.Status != "suspended_for_approval" {
		t.Fatalf("expected status 'suspended_for_approval', got %q", state.Status)
	}
	if !state.Interrupted {
		t.Fatalf("expected interrupted flag to be true")
	}
	if downstreamExecuted {
		t.Fatalf("CRITICAL: Downstream generation executor executed before approval!")
	}
	if len(state.PendingProposals()) != 1 {
		t.Fatalf("expected 1 pending proposal, got %d", len(state.PendingProposals()))
	}

	// Step 2: Test approval rejection when budget is insufficient
	_, err = harness.HandleApprove(ctx, runID, proposalID, 1000000) // 1_000_000 < 5_000_000
	if err != domain.ErrBudget {
		t.Fatalf("expected ErrBudget, got %v", err)
	}
	if downstreamExecuted {
		t.Fatalf("downstream executed after rejected budget!")
	}

	// Step 3: Test valid approval resumption
	resumedState, err := harness.HandleApprove(ctx, runID, proposalID, 10000000)
	if err != nil {
		t.Fatalf("handle approve error: %v", err)
	}

	if !downstreamExecuted {
		t.Fatalf("downstream generation_executor should have executed after approval")
	}
	if resumedState.Status != "completed" {
		t.Errorf("expected final status 'completed', got %q", resumedState.Status)
	}
	for _, p := range resumedState.Proposals {
		if p.ID == proposalID && !p.Approved {
			t.Errorf("expected proposal to be approved in final state")
		}
	}
}

func TestProposalYieldAndRejectionFlow(t *testing.T) {
	ctx := context.Background()
	g := graph.NewStateGraph()
	cp := newMockCheckpointer()

	proposalID := uuid.New()
	generationExecuted := false

	err := g.AddNode("shot_worker", func(ctx context.Context, state *graph.AgentState) (*graph.AgentState, error) {
		state.AddProposal(graph.Proposal{
			ID:                  proposalID,
			Action:              "shot.video.generate",
			Summary:             "生成昂贵的高清视频镜头",
			EstimatedCostMicros: 8000000,
			CreatedAt:           time.Now().UTC(),
		})
		return state, nil
	})
	if err != nil {
		t.Fatalf("add shot_worker node: %v", err)
	}

	err = g.AddNode("generation_executor", func(ctx context.Context, state *graph.AgentState) (*graph.AgentState, error) {
		generationExecuted = true
		return state, nil
	})
	if err != nil {
		t.Fatalf("add generation_executor: %v", err)
	}

	err = g.AddConditionalEdge("shot_worker", func(ctx context.Context, state *graph.AgentState) (string, error) {
		for _, p := range state.Proposals {
			if p.ID == proposalID {
				if p.Approved {
					return "generation_executor", nil
				}
				if p.Status == "rejected" {
					return graph.FinishPoint, nil
				}
			}
		}
		return graph.FinishPoint, nil
	})
	if err != nil {
		t.Fatalf("add conditional edge: %v", err)
	}

	if err := g.AddEdge("generation_executor", graph.FinishPoint); err != nil {
		t.Fatalf("add edge: %v", err)
	}

	if err := g.SetEntryPoint("shot_worker"); err != nil {
		t.Fatalf("set entry point: %v", err)
	}

	cg, err := g.Compile(cp)
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}

	harness := application.NewProposalHarness(cg, cp)
	runID := uuid.New()

	// Initial invoke suspends at proposal yield
	state, err := cg.Invoke(ctx, &graph.AgentState{RunID: runID, MaxTurnSteps: 40})
	if err != nil {
		t.Fatalf("invoke error: %v", err)
	}
	if state.Status != "suspended_for_approval" {
		t.Fatalf("expected suspended status, got %s", state.Status)
	}

	// User rejects proposal
	rejectedState, err := harness.HandleReject(ctx, runID, proposalID, "太贵了，重新规划")
	if err != nil {
		t.Fatalf("reject error: %v", err)
	}

	if generationExecuted {
		t.Fatalf("CRITICAL: Generation executed despite proposal being rejected!")
	}
	if rejectedState.Status != "completed" {
		t.Errorf("expected status 'completed', got %s", rejectedState.Status)
	}
	for _, p := range rejectedState.Proposals {
		if p.ID == proposalID {
			if p.Status != "rejected" || p.Approved {
				t.Errorf("expected proposal status 'rejected' and Approved=false, got status=%s, approved=%v", p.Status, p.Approved)
			}
		}
	}
}
