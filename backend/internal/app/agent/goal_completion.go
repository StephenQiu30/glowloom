package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

func deliverySpec(in ShotDeliveryInput) domain.GoalDeliverySpec {
	return domain.GoalDeliverySpec{DurationMS: in.DurationMS, SourceStartMS: in.SourceStartMS, AspectRatio: in.AspectRatio, FPS: in.FPS, ProjectFormat: in.ProjectFormat}
}

// Completion counts unique original receipts matching the admitted source and
// optional output specification. An adopted timeline revision is not the source
// revision, and another successful delivery cannot satisfy this goal.
func goalCompletion(goal domain.GoalContract, deliveries []map[string]any) (domain.GoalCompletion, error) {
	out := domain.GoalCompletion{ContractHash: domain.ContentHash(goal), Kind: goal.Kind, Status: domain.DeliveryCompleted, RequiredDeliveries: goal.RequiredDeliveries, Missing: []string{}}
	if goal.Kind == domain.GoalChat {
		return out, nil
	}
	seen := map[uuid.UUID]bool{}
	matched := 0
	for _, item := range deliveries {
		raw, err := json.Marshal(item)
		if err != nil {
			return out, err
		}
		var fact struct {
			Approved   bool                     `json:"approved"`
			DeliveryID uuid.UUID                `json:"delivery_id"`
			Source     *operation.CanvasSource  `json:"source"`
			Delivery   *domain.GoalDeliverySpec `json:"delivery"`
			Status     domain.DeliveryStatus    `json:"status"`
		}
		if json.Unmarshal(raw, &fact) != nil || fact.DeliveryID == uuid.Nil || fact.Source == nil || fact.Delivery == nil {
			return out, domain.ErrConflict
		}
		if seen[fact.DeliveryID] {
			continue
		}
		seen[fact.DeliveryID] = true
		if !fact.Approved || goal.Source != nil && domain.ContentHash(fact.Source) != domain.ContentHash(goal.Source) || goal.Delivery != nil && domain.ContentHash(fact.Delivery) != domain.ContentHash(goal.Delivery) {
			continue
		}
		matched++
		if fact.Status == domain.DeliveryCompleted {
			out.CompletedDeliveries++
			continue
		}
		out.Missing = append(out.Missing, fmt.Sprintf("delivery:%s:%s", fact.DeliveryID, fact.Status))
		switch fact.Status {
		case domain.DeliveryIncomplete:
			out.Status = domain.DeliveryIncomplete
		case domain.DeliveryAwaitingReview:
			if out.Status != domain.DeliveryIncomplete {
				out.Status = domain.DeliveryAwaitingReview
			}
		case domain.DeliveryPending:
			if out.Status == domain.DeliveryCompleted {
				out.Status = domain.DeliveryPending
			}
		default:
			return out, domain.ErrConflict
		}
	}
	if matched == 0 && goal.Source == nil {
		out.Missing = append(out.Missing, "goal_source_unplanned")
	}
	if matched < goal.RequiredDeliveries {
		out.Status = domain.DeliveryIncomplete
		out.Missing = append(out.Missing, "required_delivery_missing")
	}
	if out.CompletedDeliveries >= goal.RequiredDeliveries {
		// Additional optional attempts do not prevent an already fulfilled goal.
		out.Status = domain.DeliveryCompleted
		out.Missing = []string{}
	}
	return out, nil
}

// productionOperationsSnapshot exposes only formal operations bound to this
// root's receipts. The operation owner supplies its exact lifecycle state;
// model text and unbound project tasks cannot enter the durable wait set.
func productionOperationsSnapshot(ctx context.Context, tx *gorm.DB, g domain.Grant) ([]map[string]any, error) {
	var ids []uuid.UUID
	if err := tx.Table("agent_business.operation_commitment AS binding").Select("binding.operation_id").Joins("JOIN agent_business.call_receipt c ON c.receipt_id=binding.receipt_id").Where("c.root_run_id=? AND c.kind=?", g.RootRunID, domain.CallKindTool).Order("binding.operation_id").Limit(domain.MaxProductionOperations + 1).Scan(&ids).Error; err != nil {
		return nil, err
	}
	if len(ids) > domain.MaxProductionOperations {
		return nil, domain.ErrBudget
	}
	result := []map[string]any{}
	query := operationapp.NewPublicQuery(operationStore(tx))
	for _, id := range ids {
		task, err := query.Task(ctx, g.Scope.Principal(), g.Scope.ProjectID, id)
		if err != nil {
			return nil, businessError(err)
		}
		if g.Scope.CanvasID != nil && (task.Source == nil || task.Source.CanvasID != *g.Scope.CanvasID) {
			return nil, domain.ErrNotFound
		}
		result = append(result, map[string]any{"operation_id": task.ID, "status": task.Status})
	}
	return result, nil
}
