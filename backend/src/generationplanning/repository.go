package generationplanning

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GenerationPlanRepository struct{ orm *gorm.DB }

func NewGenerationPlanRepository(orm *gorm.DB) *GenerationPlanRepository {
	return &GenerationPlanRepository{orm: orm}
}

type planRecord struct {
	ID                   uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID            uuid.UUID `gorm:"column:project_id;type:uuid"`
	TargetType           string    `gorm:"column:target_type"`
	TargetID             uuid.UUID `gorm:"column:target_id;type:uuid"`
	Status               string
	ExecutionDisposition *string `gorm:"column:execution_disposition"`
	InputSnapshotHash    string  `gorm:"column:input_snapshot_hash"`
	PromptHash           string  `gorm:"column:prompt_hash"`
}

func (planRecord) TableName() string { return "gen_plans" }

type planItemRecord struct {
	ID            uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	PlanID        uuid.UUID `gorm:"column:plan_id;type:uuid"`
	Ordinal       int
	CapabilityKey string `gorm:"column:capability_key"`
	Prompt        string
	Status        string
}

func (planItemRecord) TableName() string { return "gen_plan_items" }

func (r *GenerationPlanRepository) CreatePlan(ctx context.Context, plan Plan, items []Item) error {
	if r.orm == nil {
		return fmt.Errorf("generation plan repository ORM is not configured")
	}
	return r.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		planRecord := planRecord{ID: plan.ID, ProjectID: plan.ProjectID, TargetType: plan.TargetType, TargetID: plan.TargetID, Status: plan.Status, InputSnapshotHash: plan.InputSnapshotHash, PromptHash: plan.PromptHash}
		if plan.ExecutionDisposition != "" {
			planRecord.ExecutionDisposition = &plan.ExecutionDisposition
		}
		if err := tx.Create(&planRecord).Error; err != nil {
			return fmt.Errorf("create generation plan: %w", err)
		}
		for _, item := range items {
			if err := tx.Create(&planItemRecord{ID: item.ID, PlanID: item.PlanID, Ordinal: item.Ordinal, CapabilityKey: item.CapabilityKey, Prompt: item.Prompt, Status: item.Status}).Error; err != nil {
				return fmt.Errorf("create generation item: %w", err)
			}
		}
		return nil
	})
}

func (r *GenerationPlanRepository) GetPlan(ctx context.Context, id uuid.UUID) (Plan, []Item, error) {
	if r.orm == nil {
		return Plan{}, nil, fmt.Errorf("generation plan repository ORM is not configured")
	}
	var planRecord planRecord
	if err := r.orm.WithContext(ctx).First(&planRecord, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Plan{}, nil, httpapi.NotFound("生成计划")
		}
		return Plan{}, nil, err
	}
	var itemRecords []planItemRecord
	if err := r.orm.WithContext(ctx).Where("plan_id = ?", id).Order("ordinal").Find(&itemRecords).Error; err != nil {
		return Plan{}, nil, err
	}
	disposition := ""
	if planRecord.ExecutionDisposition != nil {
		disposition = *planRecord.ExecutionDisposition
	}
	plan := Plan{ID: planRecord.ID, ProjectID: planRecord.ProjectID, TargetType: planRecord.TargetType, TargetID: planRecord.TargetID, Status: planRecord.Status, ExecutionDisposition: disposition, InputSnapshotHash: planRecord.InputSnapshotHash, PromptHash: planRecord.PromptHash}
	items := make([]Item, 0, len(itemRecords))
	for _, itemRecord := range itemRecords {
		items = append(items, Item{ID: itemRecord.ID, PlanID: itemRecord.PlanID, Ordinal: itemRecord.Ordinal, CapabilityKey: itemRecord.CapabilityKey, Prompt: itemRecord.Prompt, Status: itemRecord.Status})
	}
	return plan, items, nil
}

func (r *GenerationPlanRepository) Preflight(ctx context.Context, id uuid.UUID) (Plan, []Item, error) {
	if r.orm == nil {
		return Plan{}, nil, fmt.Errorf("generation plan repository ORM is not configured")
	}
	if err := r.orm.WithContext(ctx).Model(&planRecord{}).Where("id = ? AND status IN ?", id, []string{"draft", "blocked"}).Update("status", "preflight_ready").Error; err != nil {
		return Plan{}, nil, err
	}
	return r.GetPlan(ctx, id)
}

func (r *GenerationPlanRepository) Approve(ctx context.Context, id uuid.UUID, disposition string, selected []uuid.UUID) (Plan, []Item, error) {
	if r.orm == nil {
		return Plan{}, nil, fmt.Errorf("generation plan repository ORM is not configured")
	}
	if err := r.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var planRecord planRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&planRecord, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpapi.NotFound("生成计划")
			}
			return err
		}
		if planRecord.Status != "preflight_ready" && planRecord.Status != "approved" {
			return httpapi.Validation("生成计划尚未完成预检", "先完成预检再批准生成计划")
		}
		var itemRecords []planItemRecord
		if err := tx.Where("plan_id = ?", id).Find(&itemRecords).Error; err != nil {
			return err
		}
		selectedSet := make(map[uuid.UUID]struct{}, len(selected))
		for _, itemID := range selected {
			selectedSet[itemID] = struct{}{}
		}
		for _, itemRecord := range itemRecords {
			status := "excluded"
			if _, ok := selectedSet[itemRecord.ID]; ok {
				status = "selected"
			}
			if err := tx.Model(&itemRecord).Update("status", status).Error; err != nil {
				return err
			}
		}
		return tx.Model(&planRecord).Updates(map[string]any{"status": "approved", "execution_disposition": disposition}).Error
	}); err != nil {
		return Plan{}, nil, err
	}
	return r.GetPlan(ctx, id)
}
