package gormdb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
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
			projection.DefinitionVersion != execution.DefinitionVersion || projection.Executor != execution.Executor ||
			projection.RiskLevel != execution.RiskLevel || projection.WorkflowRunID != run.ID {
			return domain.ExecutionPlan{}, fmt.Errorf("workflow runtime node %s has drifted", nodeID)
		}
		plan.Nodes = append(plan.Nodes, domain.ExecutionNode{
			NodeRunID: projection.ID.String(), NodeID: nodeID, Executor: projection.Executor, RiskLevel: projection.RiskLevel,
		})
	}
	return plan, nil
}

func (store *Store) ClaimNode(
	ctx context.Context,
	command domain.NodeActivityCommand,
	claimToken string,
	now time.Time,
) (domain.NodeExecutionClaim, error) {
	runID, nodeID, token, err := runtimeNodeIdentities(command, claimToken)
	if err != nil {
		return domain.NodeExecutionClaim{}, application.ErrNotFound
	}
	var claim domain.NodeExecutionClaim
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var run model.WorkflowRun
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", runID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var node model.NodeRunProjection
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "id = ?", nodeID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if node.WorkflowRunID != run.ID || node.NodeID != command.NodeID || node.Executor != command.Executor {
			return errors.New("workflow node execution identity has drifted")
		}
		if node.Status == "SUCCEEDED" || node.Status == "CACHED" || node.Status == "SKIPPED" {
			claim = domain.NodeExecutionClaim{Command: command, Status: node.Status, Attempt: node.Attempt, Revision: node.Revision, Replay: true}
			return nil
		}
		stopped, stopErr := stoppingControlExists(transaction, run.ID)
		if stopErr != nil {
			return stopErr
		}
		if stopped {
			return errors.New("workflow node is fenced by an active control")
		}
		if (run.Status != "RUNNING" && run.Status != "RETRYING") ||
			(node.Status != "QUEUED" && node.Status != "RUNNING" && node.Status != "RETRYING") {
			return errors.New("workflow node is not executable")
		}
		node.Status = "RUNNING"
		node.Attempt++
		node.ActiveClaimToken = &token
		node.Revision++
		node.UpdatedAt = now.UTC()
		if updateErr := transaction.Model(&model.NodeRunProjection{}).Where("id = ?", node.ID).Updates(map[string]any{
			"status": node.Status, "attempt": node.Attempt, "active_claim_token": token,
			"revision": node.Revision, "updated_at": node.UpdatedAt,
		}).Error; updateErr != nil {
			return updateErr
		}
		run.Status, run.ProgressStage = "RUNNING", "node:"+node.NodeID
		run.Revision++
		run.UpdatedAt = now.UTC()
		if updateErr := transaction.Model(&model.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
			"status": run.Status, "progress_stage": run.ProgressStage, "revision": run.Revision, "updated_at": run.UpdatedAt,
		}).Error; updateErr != nil {
			return updateErr
		}
		claim = domain.NodeExecutionClaim{
			Command: command, ClaimToken: token.String(), Status: node.Status,
			Attempt: node.Attempt, Revision: node.Revision,
		}
		return nil
	})
	return claim, err
}

func (store *Store) CompleteNode(
	ctx context.Context,
	claim domain.NodeExecutionClaim,
	status string,
	now time.Time,
) error {
	return store.finishNode(ctx, claim, status, "RUNNING", "node:"+claim.Command.NodeID+":completed", now)
}

func (store *Store) RetryNode(ctx context.Context, claim domain.NodeExecutionClaim, now time.Time) error {
	return store.finishNode(ctx, claim, "RETRYING", "RETRYING", "node:"+claim.Command.NodeID+":retrying", now)
}

func (store *Store) finishNode(
	ctx context.Context,
	claim domain.NodeExecutionClaim,
	nodeStatus string,
	runStatus string,
	progressStage string,
	now time.Time,
) error {
	runID, nodeID, token, err := runtimeNodeIdentities(claim.Command, claim.ClaimToken)
	if err != nil {
		return application.ErrNotFound
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var run model.WorkflowRun
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", runID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var node model.NodeRunProjection
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "id = ?", nodeID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if node.WorkflowRunID != run.ID || node.NodeID != claim.Command.NodeID || node.Executor != claim.Command.Executor ||
			node.Status != "RUNNING" || node.ActiveClaimToken == nil || *node.ActiveClaimToken != token || node.Revision != claim.Revision {
			return &application.Error{Code: "resource_conflict", Message: "Workflow node claim is stale", Status: 409}
		}
		stopped, stopErr := stoppingControlExists(transaction, run.ID)
		if stopErr != nil {
			return stopErr
		}
		node.Status = nodeStatus
		node.ActiveClaimToken = nil
		node.Revision++
		node.UpdatedAt = now.UTC()
		if updateErr := transaction.Model(&model.NodeRunProjection{}).Where("id = ?", node.ID).Updates(map[string]any{
			"status": node.Status, "active_claim_token": nil, "revision": node.Revision, "updated_at": node.UpdatedAt,
		}).Error; updateErr != nil {
			return updateErr
		}
		if stopped || run.Status == "PAUSED" || run.Status == "NEEDS_ATTENTION" {
			return nil
		}
		run.Status, run.ProgressStage = runStatus, progressStage
		run.Revision++
		run.UpdatedAt = now.UTC()
		return transaction.Model(&model.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
			"status": run.Status, "progress_stage": run.ProgressStage, "revision": run.Revision, "updated_at": run.UpdatedAt,
		}).Error
	})
}

func runtimeNodeIdentities(command domain.NodeActivityCommand, claimToken string) (uuid.UUID, uuid.UUID, uuid.UUID, error) {
	runID, err := uuid.Parse(command.WorkflowRunID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	nodeID, err := uuid.Parse(command.NodeRunID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	token, err := uuid.Parse(claimToken)
	return runID, nodeID, token, err
}

func (store *Store) CompleteRun(ctx context.Context, command domain.CompleteRunCommand, now time.Time) error {
	runID, err := uuid.Parse(command.WorkflowRunID)
	if err != nil {
		return application.ErrNotFound
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var run model.WorkflowRun
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", runID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if run.Status == "SUCCEEDED" {
			return nil
		}
		stopped, stopErr := stoppingControlExists(transaction, run.ID)
		if stopErr != nil {
			return stopErr
		}
		if stopped {
			return errors.New("workflow completion is fenced by an active control")
		}
		if run.Status != "RUNNING" && run.Status != "RETRYING" {
			return errors.New("workflow run is not completable")
		}
		var nodes []model.NodeRunProjection
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workflow_run_id = ?", run.ID).Find(&nodes).Error; loadErr != nil {
			return loadErr
		}
		if len(nodes) == 0 {
			return errors.New("workflow run has no node projections")
		}
		for _, node := range nodes {
			if (node.Status != "SUCCEEDED" && node.Status != "CACHED" && node.Status != "SKIPPED") || node.ActiveClaimToken != nil {
				return errors.New("workflow run has incomplete node projections")
			}
		}
		run.Status, run.ProgressStage = "SUCCEEDED", "completed"
		run.NextAction, run.Error = nil, nil
		run.Revision++
		run.UpdatedAt = now.UTC()
		return transaction.Model(&model.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
			"status": run.Status, "progress_stage": run.ProgressStage, "next_action": nil, "error": nil,
			"revision": run.Revision, "updated_at": run.UpdatedAt,
		}).Error
	})
}

func (store *Store) PrepareHumanGate(
	ctx context.Context,
	command domain.NodeActivityCommand,
	now time.Time,
) (domain.HumanGateBinding, error) {
	runID, err := uuid.Parse(command.WorkflowRunID)
	if err != nil {
		return domain.HumanGateBinding{}, application.ErrNotFound
	}
	nodeID, err := uuid.Parse(command.NodeRunID)
	if err != nil {
		return domain.HumanGateBinding{}, application.ErrNotFound
	}
	var binding domain.HumanGateBinding
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var run model.WorkflowRun
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", runID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var node model.NodeRunProjection
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "id = ?", nodeID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if node.WorkflowRunID != run.ID || node.NodeID != command.NodeID || node.Executor != command.Executor ||
			node.RiskLevel != "human_gate" || (node.Status != "QUEUED" && node.Status != "WAITING_HUMAN") ||
			(run.Status != "RUNNING" && run.Status != "WAITING_HUMAN") {
			return errors.New("workflow node is not an openable human gate")
		}
		stopped, stopErr := stoppingControlExists(transaction, run.ID)
		if stopErr != nil {
			return stopErr
		}
		if stopped {
			return errors.New("workflow human gate is fenced by an active control")
		}
		if node.Status == "QUEUED" {
			node.Status = "WAITING_HUMAN"
			node.Attempt++
			node.Revision++
			node.UpdatedAt = now.UTC()
			if updateErr := transaction.Model(&model.NodeRunProjection{}).Where("id = ?", node.ID).Updates(map[string]any{
				"status": node.Status, "attempt": node.Attempt, "revision": node.Revision, "updated_at": node.UpdatedAt,
			}).Error; updateErr != nil {
				return updateErr
			}
		}
		if run.Status != "WAITING_HUMAN" || run.ProgressStage != "human_gate:"+node.NodeID {
			run.Status, run.ProgressStage = "WAITING_HUMAN", "human_gate:"+node.NodeID
			nextAction := "review_human_task"
			run.NextAction = &nextAction
			run.Revision++
			run.UpdatedAt = now.UTC()
			if updateErr := transaction.Model(&model.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
				"status": run.Status, "progress_stage": run.ProgressStage, "next_action": run.NextAction,
				"revision": run.Revision, "updated_at": run.UpdatedAt,
			}).Error; updateErr != nil {
				return updateErr
			}
		}
		binding = domain.HumanGateBinding{
			WorkspaceID: run.WorkspaceID.String(), ProjectID: run.ProjectID.String(), WorkflowRunID: run.ID.String(),
			NodeRunID: node.ID.String(), SubjectType: "workflow_node_output", SubjectID: node.ID.String(),
			SubjectRevision: node.Revision, CandidateIDs: []string{},
			RubricVersion: node.Executor + "@" + node.DefinitionVersion,
		}
		return nil
	})
	return binding, err
}

func (store *Store) ApplyHumanGate(
	ctx context.Context,
	command domain.ApplyHumanGateCommand,
	now time.Time,
) error {
	runID, err := uuid.Parse(command.WorkflowRunID)
	if err != nil {
		return application.ErrNotFound
	}
	nodeID, err := uuid.Parse(command.NodeRunID)
	if err != nil {
		return application.ErrNotFound
	}
	intentID, err := uuid.Parse(command.SignalIntentID)
	if err != nil {
		return application.ErrNotFound
	}
	decision := strings.ToLower(command.Decision)
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var intent model.WorkflowSignalIntent
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&intent, "id = ?", intentID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var apply model.WorkflowHumanGateApplyReceipt
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&apply, "id = ?", intent.ApplyReceiptID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var run model.WorkflowRun
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", runID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var node model.NodeRunProjection
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "id = ?", nodeID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if intent.Status != "completed" || intent.WorkflowRunID != run.ID || intent.NodeRunID != node.ID ||
			intent.Decision != decision || apply.WorkflowRunID != run.ID || apply.NodeRunID != node.ID ||
			apply.ReviewDecisionID != intent.ReviewDecisionID || apply.Decision != decision ||
			node.WorkflowRunID != run.ID || node.NodeID != command.NodeID || node.RiskLevel != "human_gate" {
			return errors.New("workflow human gate apply identity has drifted")
		}
		stopped, stopErr := stoppingControlExists(transaction, run.ID)
		if stopErr != nil {
			return stopErr
		}
		if stopped {
			return errors.New("workflow human gate apply is fenced by an active control")
		}
		targetNodeStatus, targetRunStatus, progressStage := "FAILED", "NEEDS_ATTENTION", "human_gate:rejected"
		nextAction := "review_rejected"
		if decision == "approved" || decision == "selected" {
			targetNodeStatus, targetRunStatus, progressStage, nextAction = "SUCCEEDED", "RUNNING", "human_gate:applied", ""
		} else if decision == "changes_requested" {
			progressStage, nextAction = "human_gate:changes_requested", "revise_node_output"
		}
		if node.Status == targetNodeStatus && node.Revision == apply.SubjectRevision+1 {
			return nil
		}
		if node.Status != "WAITING_HUMAN" || node.Revision != apply.SubjectRevision || run.Status != "WAITING_HUMAN" {
			return &application.Error{Code: "resource_conflict", Message: "Workflow human gate changed before decision application", Status: 409}
		}
		node.Status = targetNodeStatus
		node.Revision++
		node.UpdatedAt = now.UTC()
		if updateErr := transaction.Model(&model.NodeRunProjection{}).Where("id = ?", node.ID).Updates(map[string]any{
			"status": node.Status, "revision": node.Revision, "updated_at": node.UpdatedAt,
		}).Error; updateErr != nil {
			return updateErr
		}
		run.Status, run.ProgressStage = targetRunStatus, progressStage
		run.Revision++
		run.UpdatedAt = now.UTC()
		updates := map[string]any{
			"status": run.Status, "progress_stage": run.ProgressStage,
			"revision": run.Revision, "updated_at": run.UpdatedAt,
		}
		if nextAction == "" {
			updates["next_action"] = nil
			updates["error"] = nil
		} else {
			updates["next_action"] = nextAction
		}
		return transaction.Model(&model.WorkflowRun{}).Where("id = ?", run.ID).Updates(updates).Error
	})
}

func stoppingControlExists(transaction *gorm.DB, workflowRunID uuid.UUID) (bool, error) {
	var count int64
	err := transaction.Model(&model.WorkflowControlIntent{}).
		Where(
			"workflow_run_id = ? AND action IN ? AND status IN ?",
			workflowRunID, []string{domain.ControlActionPause, domain.ControlActionCancel}, []string{"pending", "unknown"},
		).
		Count(&count).Error
	return count != 0, err
}

var _ application.RuntimeRepository = (*Store)(nil)
var _ application.NodeRuntimeRepository = (*Store)(nil)
var _ application.RunCompletionRepository = (*Store)(nil)
var _ application.HumanGateRepository = (*Store)(nil)
var _ application.HumanGateApplyRepository = (*Store)(nil)
