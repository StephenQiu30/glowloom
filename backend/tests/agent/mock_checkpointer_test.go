package agent_test

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

type memoryCheckpointer struct {
	mu          sync.Mutex
	checkpoints map[uuid.UUID][]*graph.Checkpoint
}

func newMockCheckpointer() graph.Checkpointer {
	return &memoryCheckpointer{
		checkpoints: make(map[uuid.UUID][]*graph.Checkpoint),
	}
}

func (m *memoryCheckpointer) SaveCheckpoint(ctx context.Context, runID, checkpointID uuid.UUID, nodeName string, state *graph.AgentState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Deep clone state via JSON
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	var cloned graph.AgentState
	if err := json.Unmarshal(data, &cloned); err != nil {
		return err
	}

	cp := &graph.Checkpoint{
		RunID:        runID,
		CheckpointID: checkpointID,
		NodeName:     nodeName,
		State:        &cloned,
		CreatedAt:    time.Now().UTC(),
	}

	m.checkpoints[runID] = append(m.checkpoints[runID], cp)
	return nil
}

func (m *memoryCheckpointer) GetLatestCheckpoint(ctx context.Context, runID uuid.UUID) (*graph.Checkpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	list, ok := m.checkpoints[runID]
	if !ok || len(list) == 0 {
		return nil, nil
	}

	latest := list[len(list)-1]
	// Deep clone returned state
	data, err := json.Marshal(latest.State)
	if err != nil {
		return nil, err
	}
	var cloned graph.AgentState
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, err
	}

	return &graph.Checkpoint{
		RunID:        latest.RunID,
		CheckpointID: latest.CheckpointID,
		NodeName:     latest.NodeName,
		State:        &cloned,
		CreatedAt:    latest.CreatedAt,
	}, nil
}
