package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CheckpointStore implements graph.Checkpointer with PostgreSQL.
type CheckpointStore struct {
	db *gorm.DB
}

// NewCheckpointStore creates a new PostgreSQL checkpoint store.
func NewCheckpointStore(db *gorm.DB) *CheckpointStore {
	return &CheckpointStore{db: db}
}

type checkpointRow struct {
	RunID        uuid.UUID `gorm:"primaryKey;column:run_id"`
	CheckpointID uuid.UUID `gorm:"primaryKey;column:checkpoint_id"`
	NodeName     string    `gorm:"column:node_name"`
	State        string    `gorm:"column:state"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (checkpointRow) TableName() string {
	return "agent.graph_checkpoint"
}

// SaveCheckpoint saves an immutable state snapshot at a node boundary.
func (s *CheckpointStore) SaveCheckpoint(ctx context.Context, runID, checkpointID uuid.UUID, nodeName string, state *graph.AgentState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state for checkpoint: %w", err)
	}

	row := checkpointRow{
		RunID:        runID,
		CheckpointID: checkpointID,
		NodeName:     nodeName,
		State:        string(raw),
		CreatedAt:    time.Now(),
	}

	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("insert checkpoint: %w", err)
	}
	return nil
}

// GetLatestCheckpoint fetches the most recent checkpoint for a run to support Resume.
func (s *CheckpointStore) GetLatestCheckpoint(ctx context.Context, runID uuid.UUID) (*graph.Checkpoint, error) {
	var row checkpointRow
	err := s.db.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("created_at DESC").
		First(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, graph.ErrNoCheckpointFound
	}
	if err != nil {
		return nil, fmt.Errorf("query latest checkpoint: %w", err)
	}

	var state graph.AgentState
	if err := json.Unmarshal([]byte(row.State), &state); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint state: %w", err)
	}

	return &graph.Checkpoint{
		RunID:        row.RunID,
		CheckpointID: row.CheckpointID,
		NodeName:     row.NodeName,
		State:        &state,
		CreatedAt:    row.CreatedAt,
	}, nil
}
