package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

func TestConsecutiveWriteChainExecution(t *testing.T) {
	ctx := context.Background()
	coord := application.NewWriteChainCoordinator()

	state := &graph.AgentState{
		RunID:           uuid.New(),
		CurrentRevision: 1,
	}

	// Simulated database table with CAS revision
	dbRevision := int64(1)

	mockCASStore := func(expectedRev int64) (int64, error) {
		if expectedRev != dbRevision {
			return 0, domain.ErrConflict
		}
		dbRevision++
		return dbRevision, nil
	}

	// 1. Tool Call 1: Create Scene Node
	rev1, err := coord.ExecuteWrite(ctx, state, dbRevision, func(ctx context.Context, expectedRev int64) (int64, error) {
		return mockCASStore(expectedRev)
	})
	if err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if rev1 != 2 || state.WriteChain.Head != 2 {
		t.Errorf("expected rev 2 and head 2, got rev=%d head=%d", rev1, state.WriteChain.Head)
	}

	// 2. Tool Call 2: Create Character Node (should automatically pick up Head = 2 as expected revision)
	rev2, err := coord.ExecuteWrite(ctx, state, 1, func(ctx context.Context, expectedRev int64) (int64, error) {
		if expectedRev != 2 {
			t.Errorf("expected tool call 2 to receive expectedRev=2, got %d", expectedRev)
		}
		return mockCASStore(expectedRev)
	})
	if err != nil {
		t.Fatalf("second write failed: %v", err)
	}
	if rev2 != 3 || state.WriteChain.Head != 3 {
		t.Errorf("expected rev 3 and head 3, got rev=%d head=%d", rev2, state.WriteChain.Head)
	}

	// 3. Tool Call 3: Link Scene and Character
	rev3, err := coord.ExecuteWrite(ctx, state, 1, func(ctx context.Context, expectedRev int64) (int64, error) {
		if expectedRev != 3 {
			t.Errorf("expected tool call 3 to receive expectedRev=3, got %d", expectedRev)
		}
		return mockCASStore(expectedRev)
	})
	if err != nil {
		t.Fatalf("third write failed: %v", err)
	}
	if rev3 != 4 || state.WriteChain.Head != 4 {
		t.Errorf("expected rev 4 and head 4, got rev=%d head=%d", rev3, state.WriteChain.Head)
	}
}

func TestWriteChainExternalCASConflict(t *testing.T) {
	ctx := context.Background()
	coord := application.NewWriteChainCoordinator()

	state := &graph.AgentState{
		RunID:           uuid.New(),
		CurrentRevision: 10,
	}

	// DB was modified externally to rev 15
	dbRevision := int64(15)

	_, err := coord.ExecuteWrite(ctx, state, 10, func(ctx context.Context, expectedRev int64) (int64, error) {
		if expectedRev != dbRevision {
			return 0, domain.ErrConflict
		}
		return dbRevision + 1, nil
	})

	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected ErrConflict on stale revision, got %v", err)
	}
}
