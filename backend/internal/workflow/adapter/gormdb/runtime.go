package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
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
		intent.TemporalInputHash != request.InputHash || !runtimeRerunIdentityMatches(run, request) {
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
	executionNodeIDs := append([]string(nil), compiled.Definition.ExecutionOrder...)
	expectedProjectionIDs := make(map[string]struct{}, len(executionNodeIDs))
	if run.SourceWorkflowRunID != nil {
		domainProjections := make([]domain.NodeRunProjection, 0, len(projections))
		for _, projection := range projections {
			domainProjections = append(domainProjections, nodeRunDomain(projection))
		}
		scope, scopeErr := domain.BuildRerunScope(compiled.Definition, domainProjections, *run.RerunRootNodeID)
		if scopeErr != nil {
			return domain.ExecutionPlan{}, fmt.Errorf("workflow rerun projection scope has drifted: %w", scopeErr)
		}
		if scopeErr = validateRerunProjectionSources(ctx, store.database, run, projections, scope); scopeErr != nil {
			return domain.ExecutionPlan{}, scopeErr
		}
		executionNodeIDs = scope.DirtyNodeIDs
		for _, nodeID := range scope.ReusedNodeIDs {
			expectedProjectionIDs[nodeID] = struct{}{}
		}
	} else {
		for _, projection := range projections {
			if projection.ReusedFromNodeRunID != nil {
				return domain.ExecutionPlan{}, errors.New("full workflow run contains a reused node projection")
			}
		}
	}
	for _, nodeID := range executionNodeIDs {
		expectedProjectionIDs[nodeID] = struct{}{}
	}
	if len(expectedProjectionIDs) != len(projections) {
		return domain.ExecutionPlan{}, errors.New("workflow runtime node projection set has drifted")
	}

	plan := domain.ExecutionPlan{
		WorkflowRunID: run.ID.String(), DefinitionVersionID: definition.ID.String(),
		RunInputSnapshotID: snapshot.ID.String(), DefinitionContentHash: definition.ContentHash,
		InputSnapshotHash: snapshot.ContentHash, Nodes: make([]domain.ExecutionNode, 0, len(projections)),
	}
	for _, nodeID := range executionNodeIDs {
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
			if _, _, inputErr := persistedNodeInput(node); inputErr != nil {
				return inputErr
			}
			result, resultErr := completedNodeResult(node)
			if resultErr != nil {
				return resultErr
			}
			claim = domain.NodeExecutionClaim{
				Command: command, Status: node.Status, Attempt: node.Attempt, Revision: node.Revision,
				Result: result, Replay: true,
			}
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
		resolved, resolveErr := resolveNodeExecution(transaction, run, node)
		if resolveErr != nil {
			return resolveErr
		}
		if len(node.Input) != 0 || node.InputHash != nil || node.CacheKey != nil {
			_, persistedHash, inputErr := persistedNodeInput(node)
			if inputErr != nil || persistedHash != resolved.InputHash {
				return errors.New("workflow node input projection has drifted")
			}
			if nodeCacheKeyValue(node.CacheKey) != resolved.CacheKey {
				return errors.New("workflow node cache identity has drifted")
			}
		}
		node.Status = "RUNNING"
		node.Attempt++
		node.ActiveClaimToken = &token
		node.Input = datatypes.JSON(resolved.InputJSON)
		node.InputHash = &resolved.InputHash
		node.CacheKey = nodeCacheKeyPointer(resolved.CacheKey)
		node.Revision++
		node.UpdatedAt = now.UTC()
		if updateErr := transaction.Model(&model.NodeRunProjection{}).Where("id = ?", node.ID).Updates(map[string]any{
			"status": node.Status, "attempt": node.Attempt, "active_claim_token": token,
			"input": node.Input, "input_hash": resolved.InputHash, "cache_key": node.CacheKey,
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
			Attempt: node.Attempt, Revision: node.Revision, Input: resolved.Input, InputHash: resolved.InputHash,
			OutputPorts: append([]authoring.PortDefinition(nil), resolved.Execution.OutputPorts...),
			WorkspaceID: run.WorkspaceID.String(), ProjectID: run.ProjectID.String(), InitiatorUserID: run.CreatedBy.String(),
			InitiatorTokenVersion: run.InitiatorTokenVersion,
			CachePolicy:           resolved.Execution.CachePolicy,
			CacheMaterial:         resolved.CacheMaterial, CacheKey: resolved.CacheKey,
		}
		return nil
	})
	return claim, err
}

func (store *Store) CompleteNode(
	ctx context.Context,
	claim domain.NodeExecutionClaim,
	result domain.NodeActivityResult,
	now time.Time,
) error {
	return store.finishNode(ctx, claim, result.Status, "RUNNING", "node:"+claim.Command.NodeID+":completed", &result, now)
}

func (store *Store) CompleteNodeFromCache(
	ctx context.Context,
	claim domain.NodeExecutionClaim,
	now time.Time,
) (domain.NodeActivityResult, bool, error) {
	workspaceID, err := validateRuntimeCacheClaim(claim)
	if err != nil {
		return domain.NodeActivityResult{}, false, err
	}
	var result domain.NodeActivityResult
	found := false
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var persisted model.NodeCacheEntry
		loadErr := transaction.Where("workspace_id = ? AND cache_key = ?", workspaceID, claim.CacheKey).
			First(&persisted).Error
		if errors.Is(loadErr, gorm.ErrRecordNotFound) {
			return nil
		}
		if loadErr != nil {
			return loadErr
		}
		entry, entryErr := nodeCacheDomain(persisted)
		if entryErr != nil || entry.WorkspaceID != claim.WorkspaceID || entry.CacheKey != claim.CacheKey {
			return errors.New("workflow node cache fact has drifted")
		}
		normalized, _, outputHash, outputErr := domain.ParseNodeOutput(entry.Output)
		if outputErr != nil || outputHash != entry.OutputHash ||
			domain.ValidateNodeOutputPorts(normalized, claim.OutputPorts) != nil {
			return errors.New("workflow node cache output is invalid")
		}
		result = domain.NodeActivityResult{Status: "CACHED", Output: normalized, OutputHash: outputHash}
		if finishErr := finishNodeTransaction(
			transaction, claim, "CACHED", "RUNNING", "node:"+claim.Command.NodeID+":cached", &result, now,
		); finishErr != nil {
			return finishErr
		}
		found = true
		return nil
	})
	return result, found, err
}

func (store *Store) CompleteNodeWithCache(
	ctx context.Context,
	claim domain.NodeExecutionClaim,
	result domain.NodeActivityResult,
	entry domain.NodeCacheEntry,
	now time.Time,
) error {
	if result.Status != "SUCCEEDED" && result.Status != "SKIPPED" {
		return errors.New("workflow node cache completion status is invalid")
	}
	if _, err := validateRuntimeCacheClaim(claim); err != nil {
		return err
	}
	if entry.WorkspaceID != claim.WorkspaceID || entry.CacheKey != claim.CacheKey ||
		entry.SourceWorkflowRunID != claim.Command.WorkflowRunID || entry.SourceNodeRunID != claim.Command.NodeRunID {
		return errors.New("workflow node cache entry identity has drifted")
	}
	record, err := nodeCacheRecord(entry)
	if err != nil {
		return err
	}
	_, _, resultHash, resultErr := domain.BuildNodeOutput(result.Output)
	if resultErr != nil || result.OutputHash != resultHash || record.OutputHash != resultHash {
		return errors.New("workflow node cache completion output is invalid")
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		if finishErr := finishNodeTransaction(
			transaction, claim, result.Status, "RUNNING", "node:"+claim.Command.NodeID+":completed", &result, now,
		); finishErr != nil {
			return finishErr
		}
		persisted, cacheErr := ensureNodeCacheRecord(transaction, record)
		if cacheErr != nil {
			return cacheErr
		}
		if persisted.OutputHash != resultHash {
			return errors.New("workflow node cache output conflicts with its immutable fact")
		}
		return nil
	})
}

func (store *Store) RetryNode(ctx context.Context, claim domain.NodeExecutionClaim, now time.Time) error {
	return store.finishNode(ctx, claim, "RETRYING", "RETRYING", "node:"+claim.Command.NodeID+":retrying", nil, now)
}

func (store *Store) finishNode(
	ctx context.Context,
	claim domain.NodeExecutionClaim,
	nodeStatus string,
	runStatus string,
	progressStage string,
	result *domain.NodeActivityResult,
	now time.Time,
) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return finishNodeTransaction(transaction, claim, nodeStatus, runStatus, progressStage, result, now)
	})
}

func finishNodeTransaction(
	transaction *gorm.DB,
	claim domain.NodeExecutionClaim,
	nodeStatus string,
	runStatus string,
	progressStage string,
	result *domain.NodeActivityResult,
	now time.Time,
) error {
	runID, nodeID, token, err := runtimeNodeIdentities(claim.Command, claim.ClaimToken)
	if err != nil {
		return application.ErrNotFound
	}
	var run model.WorkflowRun
	if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", runID).Error; loadErr != nil {
		return normalizeNotFound(loadErr)
	}
	var node model.NodeRunProjection
	if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "id = ?", nodeID).Error; loadErr != nil {
		return normalizeNotFound(loadErr)
	}
	if node.WorkflowRunID != run.ID || node.NodeID != claim.Command.NodeID || node.Executor != claim.Command.Executor ||
		node.Status != "RUNNING" || node.ActiveClaimToken == nil || *node.ActiveClaimToken != token || node.Revision != claim.Revision ||
		run.WorkspaceID.String() != claim.WorkspaceID || nodeCacheKeyValue(node.CacheKey) != claim.CacheKey {
		return &application.Error{Code: "resource_conflict", Message: "Workflow node claim is stale", Status: 409}
	}
	stopped, stopErr := stoppingControlExists(transaction, run.ID)
	if stopErr != nil {
		return stopErr
	}
	updates := map[string]any{
		"status": nodeStatus, "active_claim_token": nil,
		"revision": node.Revision + 1, "updated_at": now.UTC(),
	}
	if result != nil {
		normalized, output, outputHash, outputErr := domain.BuildNodeOutput(result.Output)
		if outputErr != nil || result.Status != nodeStatus || result.OutputHash != outputHash {
			return errors.New("workflow node completion output is invalid")
		}
		result.Output = normalized
		node.Output = datatypes.JSON(output)
		node.OutputHash = &outputHash
		updates["output"] = node.Output
		updates["output_hash"] = outputHash
	}
	node.Status = nodeStatus
	node.ActiveClaimToken = nil
	node.Revision++
	node.UpdatedAt = now.UTC()
	if updateErr := transaction.Model(&model.NodeRunProjection{}).Where("id = ?", node.ID).Updates(updates).Error; updateErr != nil {
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
}

func validateRuntimeCacheClaim(claim domain.NodeExecutionClaim) (uuid.UUID, error) {
	workspaceID, err := uuid.Parse(claim.WorkspaceID)
	if err != nil || claim.CachePolicy != "by_inputs" {
		return uuid.Nil, errors.New("workflow node cache claim is invalid")
	}
	_, cacheKey, err := domain.BuildNodeCacheKey(claim.CacheMaterial)
	if err != nil || cacheKey != claim.CacheKey {
		return uuid.Nil, errors.New("workflow node cache claim has drifted")
	}
	return workspaceID, nil
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

func completedNodeResult(node model.NodeRunProjection) (domain.NodeActivityResult, error) {
	if (node.Status != "SUCCEEDED" && node.Status != "CACHED" && node.Status != "SKIPPED") ||
		len(node.Output) == 0 || node.OutputHash == nil {
		return domain.NodeActivityResult{}, errors.New("completed workflow node has no output projection")
	}
	normalized, _, outputHash, err := domain.ParseNodeOutput(json.RawMessage(node.Output))
	if err != nil || outputHash != *node.OutputHash {
		return domain.NodeActivityResult{}, errors.New("completed workflow node output projection has drifted")
	}
	return domain.NodeActivityResult{Status: node.Status, Output: normalized, OutputHash: outputHash}, nil
}

func persistedNodeInput(node model.NodeRunProjection) (domain.NodeInputSnapshot, string, error) {
	if len(node.Input) == 0 || node.InputHash == nil {
		return domain.NodeInputSnapshot{}, "", errors.New("workflow node has no input projection")
	}
	normalized, _, inputHash, err := domain.ParseNodeInput(json.RawMessage(node.Input))
	if err != nil || inputHash != *node.InputHash {
		return domain.NodeInputSnapshot{}, "", errors.New("workflow node input projection has drifted")
	}
	return normalized, inputHash, nil
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
			if node.RiskLevel != "human_gate" {
				if _, _, inputErr := persistedNodeInput(node); inputErr != nil {
					return inputErr
				}
				if _, resultErr := completedNodeResult(node); resultErr != nil {
					return resultErr
				}
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

func (store *Store) FailRun(ctx context.Context, command domain.FailRunCommand, now time.Time) error {
	runID, err := uuid.Parse(command.WorkflowRunID)
	if err != nil {
		return application.ErrNotFound
	}
	nodeRunID, err := uuid.Parse(command.NodeRunID)
	if err != nil {
		return application.ErrNotFound
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var run model.WorkflowRun
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", runID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var node model.NodeRunProjection
		if loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "id = ?", nodeRunID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if node.WorkflowRunID != run.ID || node.NodeID != command.NodeID {
			return errors.New("workflow failed node identity has drifted")
		}
		if run.Status == "FAILED" && node.Status == "FAILED" {
			return nil
		}
		if (run.Status != "RUNNING" && run.Status != "RETRYING") ||
			(node.Status != "QUEUED" && node.Status != "RUNNING" && node.Status != "RETRYING") {
			return &application.Error{Code: "resource_conflict", Message: "Workflow run changed before failure projection", Status: 409}
		}
		stopped, stopErr := stoppingControlExists(transaction, run.ID)
		if stopErr != nil {
			return stopErr
		}
		if stopped {
			return errors.New("workflow failure projection is fenced by an active control")
		}
		if len(node.Output) != 0 || node.OutputHash != nil {
			return errors.New("failed workflow node already has an output projection")
		}
		node.Status = "FAILED"
		node.ActiveClaimToken = nil
		node.Revision++
		node.UpdatedAt = now.UTC()
		if updateErr := transaction.Model(&model.NodeRunProjection{}).Where("id = ?", node.ID).Updates(map[string]any{
			"status": node.Status, "active_claim_token": nil, "revision": node.Revision, "updated_at": node.UpdatedAt,
		}).Error; updateErr != nil {
			return updateErr
		}
		failure, encodeErr := json.Marshal(map[string]string{
			"code": command.FailureCode, "node_id": command.NodeID,
		})
		if encodeErr != nil {
			return encodeErr
		}
		nextAction := "rerun_failed_node"
		run.Status, run.ProgressStage = "FAILED", "node:"+node.NodeID+":failed"
		run.NextAction, run.Error = &nextAction, datatypes.JSON(failure)
		run.Revision++
		run.UpdatedAt = now.UTC()
		return transaction.Model(&model.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
			"status": run.Status, "progress_stage": run.ProgressStage, "next_action": nextAction,
			"error": run.Error, "revision": run.Revision, "updated_at": run.UpdatedAt,
		}).Error
	})
}

func runtimeRerunIdentityMatches(run model.WorkflowRun, request domain.StartRequest) bool {
	if run.SourceWorkflowRunID == nil || run.RerunRootNodeID == nil {
		return request.SourceWorkflowRunID == "" && request.RerunRootNodeID == ""
	}
	return run.SourceWorkflowRunID.String() == request.SourceWorkflowRunID &&
		*run.RerunRootNodeID == request.RerunRootNodeID
}

func validateRerunProjectionSources(
	ctx context.Context,
	database *gorm.DB,
	run model.WorkflowRun,
	projections []model.NodeRunProjection,
	scope domain.RerunScope,
) error {
	if run.SourceWorkflowRunID == nil {
		return errors.New("workflow rerun source identity is missing")
	}
	dirty := make(map[string]struct{}, len(scope.DirtyNodeIDs))
	for _, nodeID := range scope.DirtyNodeIDs {
		dirty[nodeID] = struct{}{}
	}
	reused := make(map[string]struct{}, len(scope.ReusedNodeIDs))
	for _, nodeID := range scope.ReusedNodeIDs {
		reused[nodeID] = struct{}{}
	}
	sourceIDs := make([]uuid.UUID, 0, len(reused))
	projectionBySource := make(map[uuid.UUID]model.NodeRunProjection, len(reused))
	for _, projection := range projections {
		if _, exists := dirty[projection.NodeID]; exists {
			if projection.ReusedFromNodeRunID != nil {
				return errors.New("dirty workflow node has a reuse source")
			}
			continue
		}
		if _, exists := reused[projection.NodeID]; !exists || projection.Status != "SKIPPED" ||
			projection.ReusedFromNodeRunID == nil || projection.Attempt != 0 || projection.ActiveClaimToken != nil ||
			projection.CacheKey != nil {
			return errors.New("workflow reused node projection identity has drifted")
		}
		sourceIDs = append(sourceIDs, *projection.ReusedFromNodeRunID)
		projectionBySource[*projection.ReusedFromNodeRunID] = projection
	}
	if len(sourceIDs) != len(reused) || len(projectionBySource) != len(reused) {
		return errors.New("workflow reused node projection set has drifted")
	}
	var sourceNodes []model.NodeRunProjection
	if err := database.WithContext(ctx).Where("id IN ?", sourceIDs).Find(&sourceNodes).Error; err != nil {
		return err
	}
	if len(sourceNodes) != len(sourceIDs) {
		return errors.New("workflow reused source node is missing")
	}
	for _, source := range sourceNodes {
		projection, exists := projectionBySource[source.ID]
		if !exists || source.WorkflowRunID != *run.SourceWorkflowRunID || source.WorkspaceID != run.WorkspaceID ||
			source.NodeID != projection.NodeID || source.DefinitionKey != projection.DefinitionKey ||
			source.DefinitionVersion != projection.DefinitionVersion || source.Executor != projection.Executor ||
			source.RiskLevel != projection.RiskLevel || nodeInputHashValue(source.InputHash) != nodeInputHashValue(projection.InputHash) ||
			nodeOutputHashValue(source.OutputHash) != nodeOutputHashValue(projection.OutputHash) ||
			!equalJSON(source.Input, projection.Input) || !equalJSON(source.Output, projection.Output) {
			return errors.New("workflow reused source node projection has drifted")
		}
	}
	return nil
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
		resolved, resolveErr := resolveNodeExecution(transaction, run, node)
		if resolveErr != nil {
			return resolveErr
		}
		if resolved.Execution.RiskLevel != "human_gate" || resolved.Execution.CachePolicy != "never" || resolved.CacheKey != "" {
			return errors.New("workflow human gate execution contract has drifted")
		}
		candidateIDs := humanGateCandidateIDs(resolved.Input)
		var candidateSet domain.NodeInputBinding
		if node.Executor == "gate.generation_image_review" {
			if len(resolved.Input.Bindings) != 1 || resolved.Input.Bindings[0].Port != "candidates" ||
				resolved.Input.Bindings[0].ValueType != "generation_candidate_set" ||
				resolved.Input.Bindings[0].SourceKind != domain.NodeInputSourceNodeOutput {
				return errors.New("workflow Generation Human Gate has no CandidateSet input")
			}
			candidateSet, candidateIDs = resolved.Input.Bindings[0], nil
		} else if len(candidateIDs) == 0 {
			return errors.New("workflow human gate has no candidate input")
		}
		allowedDecisions, allowedErr := humanGateAllowedDecisions(node.Executor)
		if allowedErr != nil {
			return allowedErr
		}
		if node.Status == "QUEUED" {
			node.Status = "WAITING_HUMAN"
			node.Attempt++
			node.Input = datatypes.JSON(resolved.InputJSON)
			node.InputHash = &resolved.InputHash
			node.Revision++
			node.UpdatedAt = now.UTC()
			if updateErr := transaction.Model(&model.NodeRunProjection{}).Where("id = ?", node.ID).Updates(map[string]any{
				"status": node.Status, "attempt": node.Attempt, "input": node.Input, "input_hash": resolved.InputHash,
				"revision": node.Revision, "updated_at": node.UpdatedAt,
			}).Error; updateErr != nil {
				return updateErr
			}
		} else {
			_, persistedHash, inputErr := persistedNodeInput(node)
			if inputErr != nil || persistedHash != resolved.InputHash {
				return errors.New("workflow human gate input projection has drifted")
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
			NodeRunID: node.ID.String(), Executor: node.Executor, InitiatorUserID: run.CreatedBy.String(),
			InitiatorTokenVersion: run.InitiatorTokenVersion,
			SubjectType:           "workflow_node_output", SubjectID: node.ID.String(),
			SubjectRevision: node.Revision, SubjectHash: resolved.InputHash, CandidateIDs: candidateIDs,
			CandidateSet:     candidateSet,
			RubricVersion:    node.Executor + "@" + node.DefinitionVersion,
			AllowedDecisions: allowedDecisions,
		}
		return nil
	})
	return binding, err
}

func humanGateAllowedDecisions(executor string) ([]string, error) {
	switch executor {
	case "gate.generation_image_review":
		return []string{"changes_requested", "rejected", "selected"}, nil
	case "gate.production_bible_review", "gate.episode_plan_review", "gate.episode_structure_review", "gate.storyboard_review":
		return []string{"approved", "changes_requested", "rejected"}, nil
	default:
		return nil, errors.New("unsupported workflow human gate rubric")
	}
}

func humanGateCandidateIDs(input domain.NodeInputSnapshot) []string {
	candidates := make([]string, 0, len(input.Bindings))
	for _, binding := range input.Bindings {
		if binding.SourceKind == domain.NodeInputSourceNodeOutput && binding.ReferenceID != "" {
			candidates = append(candidates, binding.ReferenceID)
		}
	}
	slices.Sort(candidates)
	return slices.Compact(candidates)
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
		applyEvidence := signalApplyDomain(apply)
		if applyEvidence.OwnerReceiptID != command.OwnerReceiptID || applyEvidence.OutputHash != command.OutputHash ||
			applyEvidence.Output.SchemaVersion != command.Output.SchemaVersion ||
			!slices.Equal(applyEvidence.Output.Bindings, command.Output.Bindings) {
			return errors.New("workflow human gate owner output has drifted before application")
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
			if targetNodeStatus != "SUCCEEDED" {
				return nil
			}
			completed, completedErr := completedNodeResult(node)
			if completedErr == nil && completed.OutputHash == applyEvidence.OutputHash &&
				slices.Equal(completed.Output.Bindings, applyEvidence.Output.Bindings) {
				return nil
			}
			return errors.New("completed workflow human gate output has drifted")
		}
		if node.Status != "WAITING_HUMAN" || node.Revision != apply.SubjectRevision || run.Status != "WAITING_HUMAN" {
			return &application.Error{Code: "resource_conflict", Message: "Workflow human gate changed before decision application", Status: 409}
		}
		node.Status = targetNodeStatus
		updates := map[string]any{"status": targetNodeStatus}
		if targetNodeStatus == "SUCCEEDED" {
			node.Output, node.OutputHash = datatypes.JSON(apply.Output), apply.OutputHash
			updates["output"], updates["output_hash"] = node.Output, node.OutputHash
		}
		node.Revision++
		node.UpdatedAt = now.UTC()
		updates["revision"], updates["updated_at"] = node.Revision, node.UpdatedAt
		if updateErr := transaction.Model(&model.NodeRunProjection{}).Where("id = ?", node.ID).Updates(updates).Error; updateErr != nil {
			return updateErr
		}
		run.Status, run.ProgressStage = targetRunStatus, progressStage
		run.Revision++
		run.UpdatedAt = now.UTC()
		runUpdates := map[string]any{
			"status": run.Status, "progress_stage": run.ProgressStage,
			"revision": run.Revision, "updated_at": run.UpdatedAt,
		}
		if nextAction == "" {
			runUpdates["next_action"] = nil
			runUpdates["error"] = nil
		} else {
			runUpdates["next_action"] = nextAction
		}
		return transaction.Model(&model.WorkflowRun{}).Where("id = ?", run.ID).Updates(runUpdates).Error
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
var _ application.RunFailureRepository = (*Store)(nil)
var _ application.HumanGateRepository = (*Store)(nil)
var _ application.HumanGateApplyRepository = (*Store)(nil)
