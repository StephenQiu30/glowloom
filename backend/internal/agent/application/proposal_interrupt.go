package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

var (
	ErrProposalNotFound   = errors.New("proposal not found")
	ErrProposalNotPending = errors.New("proposal is not in pending status")
	ErrGraphNotSuspended  = errors.New("run is not suspended for approval")
)

// ProposalHarness coordinates Proposal Yield interrupts and human approval resumption.
type ProposalHarness struct {
	graph        *graph.CompiledGraph
	checkpointer graph.Checkpointer
}

// NewProposalHarness creates a harness wrapping a compiled state graph and its PostgreSQL checkpointer.
func NewProposalHarness(g *graph.CompiledGraph, cp graph.Checkpointer) *ProposalHarness {
	return &ProposalHarness{
		graph:        g,
		checkpointer: cp,
	}
}

// HandleApprove resolves a pending proposal as approved and resumes graph execution.
func (h *ProposalHarness) HandleApprove(ctx context.Context, runID, proposalID uuid.UUID, approvedMaxMicros int64) (*graph.AgentState, error) {
	if h.checkpointer == nil {
		return nil, graph.ErrCheckpointerRequired
	}

	latestCP, err := h.checkpointer.GetLatestCheckpoint(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("load latest checkpoint: %w", err)
	}
	if latestCP == nil || latestCP.State == nil {
		return nil, graph.ErrNoCheckpointFound
	}

	state := latestCP.State
	if state.Status != "suspended_for_approval" && !state.Interrupted {
		return nil, ErrGraphNotSuspended
	}

	var target *graph.Proposal
	for i := range state.Proposals {
		if state.Proposals[i].ID == proposalID {
			target = &state.Proposals[i]
			break
		}
	}
	if target == nil {
		return nil, ErrProposalNotFound
	}
	if target.Status != "pending" || target.Approved {
		return nil, ErrProposalNotPending
	}

	if approvedMaxMicros > 0 && target.EstimatedCostMicros > approvedMaxMicros {
		return nil, domain.ErrBudget
	}

	// Resume the graph with approved resolution
	resolution := graph.ProposalResolution{
		ProposalID: proposalID,
		Approved:   true,
	}

	state.Messages = append(state.Messages, graph.Message{
		ID:        uuid.New(),
		Role:      "user",
		Content:   fmt.Sprintf("用户批准了提案 [%s]: %s (预算: %d micros)", target.Action, target.Summary, target.EstimatedCostMicros),
		Timestamp: time.Now().UTC(),
	})

	return h.graph.Resume(ctx, runID, resolution)
}

// HandleReject resolves a pending proposal as rejected and resumes the graph to adjust/skip.
func (h *ProposalHarness) HandleReject(ctx context.Context, runID, proposalID uuid.UUID, reason string) (*graph.AgentState, error) {
	if h.checkpointer == nil {
		return nil, graph.ErrCheckpointerRequired
	}

	latestCP, err := h.checkpointer.GetLatestCheckpoint(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("load latest checkpoint: %w", err)
	}
	if latestCP == nil || latestCP.State == nil {
		return nil, graph.ErrNoCheckpointFound
	}

	state := latestCP.State
	if state.Status != "suspended_for_approval" && !state.Interrupted {
		return nil, ErrGraphNotSuspended
	}

	var target *graph.Proposal
	for i := range state.Proposals {
		if state.Proposals[i].ID == proposalID {
			target = &state.Proposals[i]
			break
		}
	}
	if target == nil {
		return nil, ErrProposalNotFound
	}
	if target.Status != "pending" {
		return nil, ErrProposalNotPending
	}

	if reason == "" {
		reason = "用户驳回了提案"
	}

	resolution := graph.ProposalResolution{
		ProposalID: proposalID,
		Approved:   false,
		Reason:     reason,
	}

	state.Messages = append(state.Messages, graph.Message{
		ID:        uuid.New(),
		Role:      "user",
		Content:   fmt.Sprintf("用户驳回了提案 [%s]: %s (原因: %s)", target.Action, target.Summary, reason),
		Timestamp: time.Now().UTC(),
	})

	return h.graph.Resume(ctx, runID, resolution)
}
