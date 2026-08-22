package generationplanning

import (
	"context"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
	"github.com/stephenqiu30/lanverse/backend/src/platform/toolkit"
)

type GenerationPlanService struct{ repository GenerationPlanStore }

func NewGenerationPlanService(repository GenerationPlanStore) *GenerationPlanService {
	return &GenerationPlanService{repository: repository}
}

type CreateInput struct {
	ProjectID  uuid.UUID
	TargetType string
	TargetID   uuid.UUID
	Prompt     string
	Capability string
	Count      int
}

func (s *GenerationPlanService) Create(ctx context.Context, input CreateInput) (Plan, []Item, error) {
	if input.ProjectID == uuid.Nil || input.TargetID == uuid.Nil || input.TargetType == "" || input.Prompt == "" || input.Capability == "" {
		return Plan{}, nil, httpapi.Validation("目标、提示词和能力不能为空", "补齐生成计划必填字段后重试")
	}
	if input.Count < 1 || input.Count > 100 {
		input.Count = 1
	}
	hash := toolkit.SHA256String(input.Prompt + "\x00" + input.Capability)
	plan := Plan{ID: uuid.New(), ProjectID: input.ProjectID, TargetType: input.TargetType, TargetID: input.TargetID, Status: "draft", InputSnapshotHash: hash, PromptHash: hash}
	items := make([]Item, 0, input.Count)
	for index := 1; index <= input.Count; index++ {
		items = append(items, Item{ID: uuid.New(), PlanID: plan.ID, Ordinal: index, CapabilityKey: input.Capability, Prompt: input.Prompt, Status: "proposed"})
	}
	if err := s.repository.CreatePlan(ctx, plan, items); err != nil {
		return Plan{}, nil, err
	}
	return plan, items, nil
}

func (s *GenerationPlanService) Get(ctx context.Context, id uuid.UUID) (Plan, []Item, error) {
	return s.repository.GetPlan(ctx, id)
}
func (s *GenerationPlanService) Preflight(ctx context.Context, id uuid.UUID) (Plan, []Item, error) {
	return s.repository.Preflight(ctx, id)
}
func (s *GenerationPlanService) Approve(ctx context.Context, id uuid.UUID, disposition string, selected []uuid.UUID) (Plan, []Item, error) {
	if disposition != "start_now" && disposition != "hold" {
		return Plan{}, nil, httpapi.Validation("执行处置必须是 start_now 或 hold", "选择有效的执行处置后重试")
	}
	return s.repository.Approve(ctx, id, disposition, selected)
}
