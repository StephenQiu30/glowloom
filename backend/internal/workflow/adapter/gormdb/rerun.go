package gormdb

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func (repo *repository) LoadRerunSource(ctx context.Context, sourceWorkflowRunID string) (domain.RerunSource, error) {
	runID, err := uuid.Parse(sourceWorkflowRunID)
	if err != nil {
		return domain.RerunSource{}, application.ErrNotFound
	}
	var run model.WorkflowRun
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&run, "id = ?", runID).Error; err != nil {
		return domain.RerunSource{}, normalizeNotFound(err)
	}
	var nodes []model.NodeRunProjection
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workflow_run_id = ?", run.ID).Order("node_id ASC").Find(&nodes).Error; err != nil {
		return domain.RerunSource{}, err
	}
	result := domain.RerunSource{Run: runDomain(run), Nodes: make([]domain.NodeRunProjection, 0, len(nodes))}
	for _, node := range nodes {
		result.Nodes = append(result.Nodes, nodeRunDomain(node))
	}
	return result, nil
}
