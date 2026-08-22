package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
)

type AgentRepository struct {
	orm *gorm.DB
}

func NewAgentRepository(orm *gorm.DB) *AgentRepository {
	return &AgentRepository{orm: orm}
}

type agentRunRecord struct {
	ID                uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID         uuid.UUID `gorm:"column:project_id;type:uuid"`
	OperationID       uuid.UUID `gorm:"column:operation_id;type:uuid"`
	SkillID           string    `gorm:"column:skill_id"`
	Stage             string
	StageGeneration   int    `gorm:"column:stage_generation"`
	RequestHash       string `gorm:"column:request_hash"`
	Status            string
	InputSnapshotHash string    `gorm:"column:input_snapshot_hash"`
	ResultHash        *string   `gorm:"column:result_hash"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (agentRunRecord) TableName() string { return "m06_agent_runs" }

type proposalItemRecord struct {
	ID            uuid.UUID      `gorm:"column:id;type:uuid;primaryKey"`
	AgentRunID    uuid.UUID      `gorm:"column:agent_run_id;type:uuid"`
	TargetModule  string         `gorm:"column:target_module"`
	TargetCommand string         `gorm:"column:target_command"`
	Payload       datatypes.JSON `gorm:"column:payload;type:jsonb"`
	Decision      string
	ReadSetHash   string `gorm:"column:read_set_hash"`
	WriteSetHash  string `gorm:"column:write_set_hash"`
}

func (proposalItemRecord) TableName() string { return "m06_proposal_items" }

func (r *AgentRepository) CreateRun(ctx context.Context, run AgentRun, items []ProposalItem) error {
	if r.orm == nil {
		return fmt.Errorf("agent repository ORM is not configured")
	}
	return r.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		runRecord := agentRunRecord{ID: run.ID, ProjectID: run.ProjectID, OperationID: run.OperationID, SkillID: run.Skill, Stage: run.Stage, StageGeneration: run.StageGeneration, RequestHash: run.RequestHash, Status: run.Status, InputSnapshotHash: run.InputSnapshotHash, CreatedAt: run.CreatedAt}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&runRecord).Error; err != nil {
			return fmt.Errorf("create agent run: %w", err)
		}
		for _, item := range items {
			payload, err := json.Marshal(item.Payload)
			if err != nil {
				return fmt.Errorf("encode proposal item: %w", err)
			}
			itemRecord := proposalItemRecord{ID: item.ID, AgentRunID: item.AgentRunID, TargetModule: item.TargetModule, TargetCommand: item.TargetCommand, Payload: datatypes.JSON(payload), Decision: item.Decision, ReadSetHash: item.ReadSetHash, WriteSetHash: item.WriteSetHash}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&itemRecord).Error; err != nil {
				return fmt.Errorf("create proposal item: %w", err)
			}
		}
		return nil
	})
}

func (r *AgentRepository) GetRun(ctx context.Context, id uuid.UUID) (AgentRun, []ProposalItem, error) {
	if r.orm == nil {
		return AgentRun{}, nil, fmt.Errorf("agent repository ORM is not configured")
	}
	var runRecord agentRunRecord
	if err := r.orm.WithContext(ctx).Where("id = ?", id).First(&runRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AgentRun{}, nil, httpapi.NotFound("AgentRun")
		}
		return AgentRun{}, nil, err
	}
	var itemRecords []proposalItemRecord
	if err := r.orm.WithContext(ctx).Where("agent_run_id = ?", id).Order("id").Find(&itemRecords).Error; err != nil {
		return AgentRun{}, nil, err
	}
	resultHash := ""
	if runRecord.ResultHash != nil {
		resultHash = *runRecord.ResultHash
	}
	run := AgentRun{ID: runRecord.ID, ProjectID: runRecord.ProjectID, OperationID: runRecord.OperationID, Skill: runRecord.SkillID, Stage: runRecord.Stage, StageGeneration: runRecord.StageGeneration, RequestHash: runRecord.RequestHash, Status: runRecord.Status, InputSnapshotHash: runRecord.InputSnapshotHash, ResultHash: resultHash, CreatedAt: runRecord.CreatedAt}
	items := make([]ProposalItem, 0, len(itemRecords))
	for _, itemRecord := range itemRecords {
		var payload any
		if err := json.Unmarshal(itemRecord.Payload, &payload); err != nil {
			return AgentRun{}, nil, fmt.Errorf("decode proposal item: %w", err)
		}
		items = append(items, ProposalItem{ID: itemRecord.ID, AgentRunID: itemRecord.AgentRunID, TargetModule: itemRecord.TargetModule, TargetCommand: itemRecord.TargetCommand, Payload: payload, Decision: itemRecord.Decision, ReadSetHash: itemRecord.ReadSetHash, WriteSetHash: itemRecord.WriteSetHash})
	}
	return run, items, nil
}

func (r *AgentRepository) UpdateRun(ctx context.Context, id uuid.UUID, status, resultHash string) error {
	if r.orm == nil {
		return fmt.Errorf("agent repository ORM is not configured")
	}
	updates := map[string]any{"status": status}
	if resultHash == "" {
		updates["result_hash"] = nil
	} else {
		updates["result_hash"] = resultHash
	}
	return r.orm.WithContext(ctx).Model(&agentRunRecord{}).Where("id = ?", id).Updates(updates).Error
}
