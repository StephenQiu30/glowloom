package domain

import (
	"errors"
	"fmt"
)

type RerunScope struct {
	DirtyNodeIDs  []string
	ReusedNodeIDs []string
}

func BuildRerunScope(
	definition WorkflowDefinitionVersion,
	source []NodeRunProjection,
	rootNodeID string,
) (RerunScope, error) {
	executions, order, err := rerunExecutions(definition)
	if err != nil {
		return RerunScope{}, err
	}
	if _, exists := executions[rootNodeID]; !exists {
		return RerunScope{}, errors.New("rerun root node is not executable")
	}
	forward, reverse, err := rerunAdjacency(definition, executions)
	if err != nil {
		return RerunScope{}, err
	}
	dirty := graphClosure([]string{rootNodeID}, forward)
	required := graphClosure(mapKeys(dirty), reverse)
	for nodeID := range dirty {
		delete(required, nodeID)
	}

	sourceByNode, err := rerunSourceProjections(source, executions)
	if err != nil {
		return RerunScope{}, err
	}
	if _, exists := sourceByNode[rootNodeID]; !exists {
		return RerunScope{}, errors.New("rerun root projection is missing from source run")
	}
	for nodeID := range required {
		projection, exists := sourceByNode[nodeID]
		if !exists {
			return RerunScope{}, fmt.Errorf("required upstream projection %s is missing", nodeID)
		}
		if err = validateReusableProjection(projection, executions[nodeID]); err != nil {
			return RerunScope{}, fmt.Errorf("required upstream projection %s is invalid: %w", nodeID, err)
		}
	}

	scope := RerunScope{
		DirtyNodeIDs:  make([]string, 0, len(dirty)),
		ReusedNodeIDs: make([]string, 0, len(required)),
	}
	for _, nodeID := range order {
		if _, exists := dirty[nodeID]; exists {
			scope.DirtyNodeIDs = append(scope.DirtyNodeIDs, nodeID)
		}
		if _, exists := required[nodeID]; exists {
			scope.ReusedNodeIDs = append(scope.ReusedNodeIDs, nodeID)
		}
	}
	return scope, nil
}

func rerunExecutions(definition WorkflowDefinitionVersion) (map[string]NodeExecution, []string, error) {
	if len(definition.NodeExecutions) == 0 || len(definition.ExecutionOrder) != len(definition.NodeExecutions) {
		return nil, nil, errors.New("workflow definition execution set is invalid")
	}
	executions := make(map[string]NodeExecution, len(definition.NodeExecutions))
	for _, execution := range definition.NodeExecutions {
		if execution.NodeID == "" {
			return nil, nil, errors.New("workflow definition has an empty node identity")
		}
		if _, exists := executions[execution.NodeID]; exists {
			return nil, nil, errors.New("workflow definition has a duplicate node identity")
		}
		executions[execution.NodeID] = execution
	}
	seen := make(map[string]struct{}, len(definition.ExecutionOrder))
	for _, nodeID := range definition.ExecutionOrder {
		if _, exists := executions[nodeID]; !exists {
			return nil, nil, errors.New("workflow definition execution order has drifted")
		}
		if _, exists := seen[nodeID]; exists {
			return nil, nil, errors.New("workflow definition execution order is duplicated")
		}
		seen[nodeID] = struct{}{}
	}
	return executions, append([]string(nil), definition.ExecutionOrder...), nil
}

func rerunAdjacency(
	definition WorkflowDefinitionVersion,
	executions map[string]NodeExecution,
) (map[string][]string, map[string][]string, error) {
	forward := make(map[string][]string, len(executions))
	reverse := make(map[string][]string, len(executions))
	seen := make(map[string]struct{}, len(definition.ExecutionGraph.Edges))
	for _, edge := range definition.ExecutionGraph.Edges {
		if _, exists := executions[edge.FromNodeID]; !exists {
			return nil, nil, errors.New("workflow rerun edge source is not executable")
		}
		if _, exists := executions[edge.ToNodeID]; !exists {
			return nil, nil, errors.New("workflow rerun edge target is not executable")
		}
		identity := edge.FromNodeID + "\x00" + edge.ToNodeID
		if _, exists := seen[identity]; exists {
			continue
		}
		seen[identity] = struct{}{}
		forward[edge.FromNodeID] = append(forward[edge.FromNodeID], edge.ToNodeID)
		reverse[edge.ToNodeID] = append(reverse[edge.ToNodeID], edge.FromNodeID)
	}
	return forward, reverse, nil
}

func graphClosure(roots []string, adjacency map[string][]string) map[string]struct{} {
	closure := make(map[string]struct{}, len(roots))
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		if _, exists := closure[nodeID]; exists {
			continue
		}
		closure[nodeID] = struct{}{}
		queue = append(queue, adjacency[nodeID]...)
	}
	return closure
}

func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func rerunSourceProjections(
	source []NodeRunProjection,
	executions map[string]NodeExecution,
) (map[string]NodeRunProjection, error) {
	if len(source) == 0 {
		return nil, errors.New("source workflow run has no node projections")
	}
	byNode := make(map[string]NodeRunProjection, len(source))
	workspaceID, workflowRunID := source[0].WorkspaceID, source[0].WorkflowRunID
	for _, projection := range source {
		execution, exists := executions[projection.NodeID]
		if !exists || projection.WorkspaceID != workspaceID || projection.WorkflowRunID != workflowRunID ||
			projection.DefinitionKey != execution.DefinitionKey || projection.DefinitionVersion != execution.DefinitionVersion ||
			projection.Executor != execution.Executor || projection.RiskLevel != execution.RiskLevel {
			return nil, errors.New("source workflow node projection identity has drifted")
		}
		if _, exists = byNode[projection.NodeID]; exists {
			return nil, errors.New("source workflow node projection is duplicated")
		}
		byNode[projection.NodeID] = projection
	}
	return byNode, nil
}

func validateReusableProjection(projection NodeRunProjection, execution NodeExecution) error {
	switch projection.Status {
	case "SUCCEEDED", "CACHED", "SKIPPED":
	default:
		return errors.New("projection is not terminal and reusable")
	}
	if projection.ActiveClaimToken != nil {
		return errors.New("projection retains an active claim")
	}
	_, _, inputHash, err := ParseNodeInput(projection.Input)
	if err != nil || inputHash != projection.InputHash {
		return errors.New("input snapshot has drifted")
	}
	output, _, outputHash, err := ParseNodeOutput(projection.Output)
	if err != nil || outputHash != projection.OutputHash || ValidateNodeOutputPorts(output, execution.OutputPorts) != nil {
		return errors.New("output snapshot has drifted")
	}
	return nil
}
