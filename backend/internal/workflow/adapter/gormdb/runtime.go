package gormdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func (store *Store) LoadExecutionPlan(ctx context.Context, request domain.StartRequest) (domain.ExecutionPlan, error) {
	runID, err := uuid.Parse(request.WorkflowRunID)
	if err != nil {
		return domain.ExecutionPlan{}, application.ErrNotFound
	}
	definitionID, err := uuid.Parse(request.DefinitionVersionID)
	if err != nil {
		return domain.ExecutionPlan{}, application.ErrNotFound
	}
	snapshotID, err := uuid.Parse(request.RunInputSnapshotID)
	if err != nil {
		return domain.ExecutionPlan{}, application.ErrNotFound
	}

	var run model.WorkflowRun
	if err = store.database.WithContext(ctx).First(&run, "id = ?", runID).Error; err != nil {
		return domain.ExecutionPlan{}, normalizeNotFound(err)
	}
	var intent model.WorkflowStartIntent
	if err = store.database.WithContext(ctx).Where("workflow_run_id = ?", runID).First(&intent).Error; err != nil {
		return domain.ExecutionPlan{}, normalizeNotFound(err)
	}
	if run.Status != "RUNNING" || intent.Status != "completed" {
		return domain.ExecutionPlan{}, errors.New("workflow start is not committed")
	}
	if run.WorkflowDefinitionVersionID != definitionID || run.RunInputSnapshotID != snapshotID ||
		run.TemporalWorkflowID != request.WorkflowID || run.StartInputHash != request.InputHash ||
		intent.TemporalInputHash != request.InputHash {
		return domain.ExecutionPlan{}, errors.New("workflow runtime start identity has drifted")
	}

	var definition model.WorkflowDefinitionVersion
	if err = store.database.WithContext(ctx).First(&definition, "id = ?", definitionID).Error; err != nil {
		return domain.ExecutionPlan{}, normalizeNotFound(err)
	}
	var snapshot model.RunInputSnapshot
	if err = store.database.WithContext(ctx).Where("id = ? AND workflow_definition_version_id = ?", snapshotID, definitionID).
		First(&snapshot).Error; err != nil {
		return domain.ExecutionPlan{}, normalizeNotFound(err)
	}
	compiled, err := compiledFacts(definition, snapshot)
	if err != nil {
		return domain.ExecutionPlan{}, err
	}
	if compiled.Definition.ContentHash != request.DefinitionContentHash ||
		compiled.RunInputSnapshot.ContentHash != request.InputSnapshotHash {
		return domain.ExecutionPlan{}, errors.New("workflow runtime compilation hash has drifted")
	}

	var projections []model.NodeRunProjection
	if err = store.database.WithContext(ctx).Where("workflow_run_id = ?", runID).Find(&projections).Error; err != nil {
		return domain.ExecutionPlan{}, err
	}
	projectionByNode := make(map[string]model.NodeRunProjection, len(projections))
	for _, projection := range projections {
		if _, exists := projectionByNode[projection.NodeID]; exists {
			return domain.ExecutionPlan{}, fmt.Errorf("workflow node projection %s is duplicated", projection.NodeID)
		}
		projectionByNode[projection.NodeID] = projection
	}
	executionByNode := make(map[string]domain.NodeExecution, len(compiled.Definition.NodeExecutions))
	for _, execution := range compiled.Definition.NodeExecutions {
		executionByNode[execution.NodeID] = execution
	}
	if len(compiled.Definition.ExecutionOrder) != len(projections) || len(executionByNode) != len(projections) {
		return domain.ExecutionPlan{}, errors.New("workflow runtime node projection set has drifted")
	}

	plan := domain.ExecutionPlan{
		WorkflowRunID: run.ID.String(), DefinitionVersionID: definition.ID.String(),
		RunInputSnapshotID: snapshot.ID.String(), DefinitionContentHash: definition.ContentHash,
		InputSnapshotHash: snapshot.ContentHash, Nodes: make([]domain.ExecutionNode, 0, len(projections)),
	}
	for _, nodeID := range compiled.Definition.ExecutionOrder {
		projection, projectionExists := projectionByNode[nodeID]
		execution, executionExists := executionByNode[nodeID]
		if !projectionExists || !executionExists || projection.DefinitionKey != execution.DefinitionKey ||
			projection.DefinitionVersion != execution.DefinitionVersion || projection.WorkflowRunID != run.ID {
			return domain.ExecutionPlan{}, fmt.Errorf("workflow runtime node %s has drifted", nodeID)
		}
		plan.Nodes = append(plan.Nodes, domain.ExecutionNode{
			NodeRunID: projection.ID.String(), NodeID: nodeID, Executor: execution.Executor, RiskLevel: execution.RiskLevel,
		})
	}
	return plan, nil
}

var _ application.RuntimeRepository = (*Store)(nil)
