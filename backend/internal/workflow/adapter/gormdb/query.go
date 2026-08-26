package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func (store *Store) GetRun(ctx context.Context, actor application.Actor, runID string) (application.RunView, error) {
	id, err := uuid.Parse(runID)
	if err != nil {
		return application.RunView{}, application.ErrNotFound
	}
	run, err := authorizeWorkflowRun(ctx, store.database, actor, id, false, false)
	if err != nil {
		return application.RunView{}, err
	}
	var nodes []model.NodeRunProjection
	if err = store.database.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Find(&nodes).Error; err != nil {
		return application.RunView{}, err
	}
	if len(nodes) == 0 {
		return application.RunView{}, errors.New("workflow run has no node projections")
	}
	var definitionRecord model.WorkflowDefinitionVersion
	if err = store.database.WithContext(ctx).First(&definitionRecord, "id = ?", run.WorkflowDefinitionVersionID).Error; err != nil {
		return application.RunView{}, normalizeNotFound(err)
	}
	var definition domain.WorkflowDefinitionVersion
	if err = json.Unmarshal(definitionRecord.Definition, &definition); err != nil ||
		definition.ContentHash != definitionRecord.ContentHash {
		return application.RunView{}, errors.New("workflow query definition projection has drifted")
	}
	position := make(map[string]int, len(definition.ExecutionOrder))
	for index, nodeID := range definition.ExecutionOrder {
		if _, exists := position[nodeID]; exists {
			return application.RunView{}, errors.New("workflow query execution order is duplicated")
		}
		position[nodeID] = index
	}
	for _, node := range nodes {
		if _, exists := position[node.NodeID]; !exists || node.WorkflowRunID != run.ID || node.WorkspaceID != run.WorkspaceID {
			return application.RunView{}, errors.New("workflow query node projection has drifted")
		}
	}
	slices.SortFunc(nodes, func(left, right model.NodeRunProjection) int {
		return position[left.NodeID] - position[right.NodeID]
	})
	view := application.RunView{Run: runDomain(run), Nodes: make([]domain.NodeRunProjection, len(nodes))}
	for index, node := range nodes {
		view.Nodes[index] = nodeRunDomain(node)
	}
	return view, nil
}

func (store *Store) ResolveRunAccess(
	ctx context.Context,
	actor application.Actor,
	runID string,
	write bool,
) (domain.WorkflowRun, error) {
	id, err := uuid.Parse(runID)
	if err != nil {
		return domain.WorkflowRun{}, application.ErrNotFound
	}
	run, err := authorizeWorkflowRun(ctx, store.database, actor, id, write, false)
	if err != nil {
		return domain.WorkflowRun{}, err
	}
	return runDomain(run), nil
}

func authorizeWorkflowRun(
	ctx context.Context,
	database *gorm.DB,
	actor application.Actor,
	runID uuid.UUID,
	write, lock bool,
) (model.WorkflowRun, error) {
	userID, err := uuid.Parse(actor.UserID)
	if err != nil || actor.TokenVersion < 1 {
		return model.WorkflowRun{}, unauthenticatedWorkflow()
	}
	var user model.UserAccount
	if err = database.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil ||
		user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return model.WorkflowRun{}, unauthenticatedWorkflow()
	}
	query := database.WithContext(ctx)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var run model.WorkflowRun
	if err = query.First(&run, "id = ?", runID).Error; err != nil {
		return model.WorkflowRun{}, normalizeNotFound(err)
	}
	var workspace model.Workspace
	if err = database.WithContext(ctx).First(&workspace, "id = ?", run.WorkspaceID).Error; err != nil {
		return model.WorkflowRun{}, normalizeNotFound(err)
	}
	var membership model.Membership
	if err = database.WithContext(ctx).Where(
		"workspace_id = ? AND user_id = ? AND status = ?", run.WorkspaceID, userID, "active",
	).First(&membership).Error; err != nil {
		return model.WorkflowRun{}, application.ErrNotFound
	}
	if write && (workspace.Status != "active" || membership.Role == "viewer") {
		return model.WorkflowRun{}, &application.Error{Code: "forbidden", Message: "Action is not allowed", Status: 403}
	}
	return run, nil
}

func unauthenticatedWorkflow() error {
	return &application.Error{
		Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login",
	}
}

var _ application.QueryRepository = (*Store)(nil)
