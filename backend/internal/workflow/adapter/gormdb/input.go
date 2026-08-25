package gormdb

import (
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type resolvedNodeExecution struct {
	Input         domain.NodeInputSnapshot
	InputJSON     json.RawMessage
	InputHash     string
	Execution     domain.NodeExecution
	CacheMaterial domain.NodeCacheKeyMaterial
	CacheKey      string
}

func resolveNodeExecution(transaction *gorm.DB, run model.WorkflowRun, node model.NodeRunProjection) (resolvedNodeExecution, error) {
	var definition model.WorkflowDefinitionVersion
	if err := transaction.First(&definition, "id = ?", run.WorkflowDefinitionVersionID).Error; err != nil {
		return resolvedNodeExecution{}, normalizeNotFound(err)
	}
	var snapshot model.RunInputSnapshot
	if err := transaction.Where(
		"id = ? AND workflow_definition_version_id = ?", run.RunInputSnapshotID, run.WorkflowDefinitionVersionID,
	).First(&snapshot).Error; err != nil {
		return resolvedNodeExecution{}, normalizeNotFound(err)
	}
	compiled, err := compiledFacts(definition, snapshot)
	if err != nil {
		return resolvedNodeExecution{}, err
	}
	if compiled.Definition.WorkspaceID != run.WorkspaceID.String() || compiled.Definition.ProjectID != run.ProjectID.String() ||
		compiled.RunInputSnapshot.WorkspaceID != run.WorkspaceID.String() ||
		compiled.RunInputSnapshot.ProjectID != run.ProjectID.String() {
		return resolvedNodeExecution{}, errors.New("workflow node input ownership has drifted")
	}

	executions := make(map[string]domain.NodeExecution, len(compiled.Definition.NodeExecutions))
	for _, execution := range compiled.Definition.NodeExecutions {
		executions[execution.NodeID] = execution
	}
	execution, exists := executions[node.NodeID]
	if !exists || execution.DefinitionKey != node.DefinitionKey || execution.DefinitionVersion != node.DefinitionVersion ||
		execution.Executor != node.Executor || execution.RiskLevel != node.RiskLevel || len(execution.OutputPorts) == 0 {
		return resolvedNodeExecution{}, errors.New("workflow node input execution descriptor has drifted")
	}
	var graphNode authoring.Node
	graphNodeFound := false
	for _, candidate := range compiled.Definition.ExecutionGraph.Nodes {
		if candidate.ID == node.NodeID {
			graphNode, graphNodeFound = candidate, true
			break
		}
	}
	if !graphNodeFound || graphNode.DefinitionKey != node.DefinitionKey || graphNode.DefinitionVersion != node.DefinitionVersion {
		return resolvedNodeExecution{}, errors.New("workflow node input graph identity has drifted")
	}

	var projections []model.NodeRunProjection
	if err = transaction.Where("workflow_run_id = ?", run.ID).Find(&projections).Error; err != nil {
		return resolvedNodeExecution{}, err
	}
	projectionByNode := make(map[string]model.NodeRunProjection, len(projections))
	for _, projection := range projections {
		if _, duplicated := projectionByNode[projection.NodeID]; duplicated {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node input source %s is duplicated", projection.NodeID)
		}
		projectionByNode[projection.NodeID] = projection
	}

	bindings := make([]domain.NodeInputBinding, 0, len(execution.InputPorts))
	for _, edge := range compiled.Definition.ExecutionGraph.Edges {
		if edge.ToNodeID != node.NodeID {
			continue
		}
		sourceProjection, sourceExists := projectionByNode[edge.FromNodeID]
		sourceExecution, executionExists := executions[edge.FromNodeID]
		if !sourceExists || !executionExists || sourceProjection.DefinitionKey != sourceExecution.DefinitionKey ||
			sourceProjection.DefinitionVersion != sourceExecution.DefinitionVersion ||
			sourceProjection.Executor != sourceExecution.Executor || sourceProjection.RiskLevel != sourceExecution.RiskLevel {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node input source %s is missing", edge.FromNodeID)
		}
		sourcePort, sourcePortExists := executionPort(sourceExecution.OutputPorts, edge.FromPort)
		targetPort, targetPortExists := executionPort(execution.InputPorts, edge.ToPort)
		if !sourcePortExists || !targetPortExists || sourcePort.ValueType != targetPort.ValueType {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node input edge %s has drifted", edge.ID)
		}
		sourceResult, resultErr := completedNodeResult(sourceProjection)
		if resultErr != nil {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node input source %s: %w", edge.FromNodeID, resultErr)
		}
		if resultErr = domain.ValidateNodeOutputPorts(sourceResult.Output, sourceExecution.OutputPorts); resultErr != nil {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node input source %s output: %w", edge.FromNodeID, resultErr)
		}
		output, outputExists := outputBinding(sourceResult.Output, edge.FromPort)
		if !outputExists || output.ValueType != targetPort.ValueType {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node input source port %s has drifted", edge.FromPort)
		}
		bindings = append(bindings, domain.NodeInputBinding{
			Port: edge.ToPort, ValueType: targetPort.ValueType, SourceKind: domain.NodeInputSourceNodeOutput,
			SourceNodeID: edge.FromNodeID, SourcePort: edge.FromPort,
			ReferenceID: output.ReferenceID, ReferenceVersion: output.ReferenceVersion, ContentHash: output.ContentHash,
		})
	}
	for _, binding := range compiled.Definition.ExecutionGraph.Bindings {
		if binding.NodeID != node.NodeID {
			continue
		}
		port, portExists := executionPort(execution.InputPorts, binding.Port)
		value, variableExists := compiled.Definition.ExecutionGraph.Variables[binding.Variable]
		if !portExists || !variableExists || port.ValueType != binding.ValueType {
			return resolvedNodeExecution{}, fmt.Errorf("workflow node variable binding %s has drifted", binding.Port)
		}
		bindings = append(bindings, domain.NodeInputBinding{
			Port: binding.Port, ValueType: binding.ValueType, SourceKind: domain.NodeInputSourceVariable,
			Variable: binding.Variable, Value: append([]byte(nil), value...),
		})
	}
	if err = validateResolvedInputPorts(bindings, execution.InputPorts); err != nil {
		return resolvedNodeExecution{}, err
	}
	input, inputJSON, inputHash, err := domain.BuildNodeInput(domain.NodeInputSnapshot{
		SchemaVersion: domain.NodeInputSchemaVersion, Config: graphNode.Config, Bindings: bindings,
		FrozenInputs: compiled.RunInputSnapshot.FrozenInputs,
	})
	if err != nil {
		return resolvedNodeExecution{}, err
	}
	cacheMaterial, cacheKey, err := domain.BuildNodeCacheMaterial(execution, input, compiled.Definition.RuntimeContractVersion)
	if err != nil {
		return resolvedNodeExecution{}, err
	}
	if execution.CachePolicy == "never" {
		cacheKey = ""
	}
	return resolvedNodeExecution{
		Input: input, InputJSON: inputJSON, InputHash: inputHash, Execution: execution,
		CacheMaterial: cacheMaterial, CacheKey: cacheKey,
	}, nil
}

func validateResolvedInputPorts(bindings []domain.NodeInputBinding, expected []authoring.PortDefinition) error {
	expectedByPort := make(map[string]authoring.PortDefinition, len(expected))
	for _, port := range expected {
		expectedByPort[port.Key] = port
	}
	observed := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		port, exists := expectedByPort[binding.Port]
		if !exists || port.ValueType != binding.ValueType {
			return errors.New("workflow node input port has drifted")
		}
		if _, duplicated := observed[binding.Port]; duplicated {
			return errors.New("workflow node input port is duplicated")
		}
		observed[binding.Port] = struct{}{}
	}
	for _, port := range expected {
		if _, exists := observed[port.Key]; port.Required && !exists {
			return errors.New("workflow node required input is missing")
		}
	}
	return nil
}

func executionPort(ports []authoring.PortDefinition, key string) (authoring.PortDefinition, bool) {
	for _, port := range ports {
		if port.Key == key {
			return port, true
		}
	}
	return authoring.PortDefinition{}, false
}

func outputBinding(output domain.NodeOutputSnapshot, port string) (domain.NodeOutputBinding, bool) {
	for _, binding := range output.Bindings {
		if binding.Port == port {
			return binding, true
		}
	}
	return domain.NodeOutputBinding{}, false
}
