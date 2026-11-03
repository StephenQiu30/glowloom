package application

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
)

// WriteExecutor executes a state-modifying action guarded by Compare-And-Swap (CAS).
type WriteExecutor func(ctx context.Context, expectedRev int64) (newRev int64, err error)

// WriteChainCoordinator manages CAS revision advancement during multi-step tool turns (BeefTV extendWriteChain pattern).
type WriteChainCoordinator struct{}

// NewWriteChainCoordinator creates a coordinator for safe contiguous write operations.
func NewWriteChainCoordinator() *WriteChainCoordinator {
	return &WriteChainCoordinator{}
}

// ExecuteWrite guards a write operation by ensuring the tool uses the latest local head revision,
// and extends the state's WriteChain upon successful CAS commit.
func (c *WriteChainCoordinator) ExecuteWrite(
	ctx context.Context,
	state *graph.AgentState,
	baseRevision int64,
	executor WriteExecutor,
) (int64, error) {
	if state == nil {
		return 0, fmt.Errorf("agent state cannot be nil")
	}

	// Determine the expected revision: use local chain head if active, otherwise base revision
	expected := baseRevision
	if state.WriteChain.Head > 0 {
		expected = state.WriteChain.Head
	}

	newRev, err := executor(ctx, expected)
	if err != nil {
		return 0, err
	}

	// CAS commit succeeded: extend local write chain
	if !state.WriteChain.Extend(expected, newRev) {
		return 0, fmt.Errorf("%w: failed to extend write chain from rev %d to %d (head is %d)",
			domain.ErrConflict, expected, newRev, state.WriteChain.Head)
	}

	state.CurrentRevision = newRev
	return newRev, nil
}
