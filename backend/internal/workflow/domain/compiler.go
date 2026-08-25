package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
)

var (
	compilerVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	workflowTypePattern    = regexp.MustCompile(`^[a-z][a-z0-9.-]{0,119}$`)
)

func Compile(source CompilationSource, contract CompilerContract) (Compilation, error) {
	if err := validateContract(contract); err != nil {
		return Compilation{}, err
	}
	revision, catalog, err := validateSource(source)
	if err != nil {
		return Compilation{}, err
	}
	executionGraph, executions, err := compileGraph(revision.Graph, catalog)
	if err != nil {
		return Compilation{}, err
	}
	order, err := topologicalOrder(executionGraph)
	if err != nil {
		return Compilation{}, err
	}
	definition := WorkflowDefinitionVersion{
		SchemaVersion: "workflow-definition-v1", WorkspaceID: revision.WorkspaceID, ProjectID: revision.ProjectID,
		AuthoringRevisionID: revision.ID, AuthoringRevisionContentHash: revision.ContentHash,
		AuthoringRevisionExecutionHash: revision.ExecutionHash, NodeCatalogVersionID: revision.CatalogID,
		NodeCatalogKey:     catalog.Key,
		NodeCatalogVersion: catalog.Version, NodeCatalogExecutionHash: catalog.ExecutionHash,
		CompilerVersion: contract.CompilerVersion, WorkflowType: contract.WorkflowType,
		WorkflowTypeVersion: contract.WorkflowTypeVersion, RuntimeContractVersion: contract.RuntimeContractVersion,
		ExecutionGraph: executionGraph, ExecutionOrder: order, NodeExecutions: executions,
	}
	encodedDefinition, err := marshalDefinition(definition)
	if err != nil {
		return Compilation{}, fmt.Errorf("encode workflow definition: %w", err)
	}
	definition.ContentHash = sha256Hex(encodedDefinition)

	snapshot := RunInputSnapshot{
		SchemaVersion: "run-input-snapshot-v1", WorkspaceID: revision.WorkspaceID, ProjectID: revision.ProjectID,
		AuthoringRevisionID: revision.ID, AuthoringExecutionHash: revision.ExecutionHash,
		WorkflowDefinitionHash: definition.ContentHash,
		FrozenInputs:           append([]authoring.FrozenReference(nil), revision.FrozenInputs...),
	}
	encodedSnapshot, err := json.Marshal(runInputHashInput{
		SchemaVersion: snapshot.SchemaVersion, AuthoringExecutionHash: snapshot.AuthoringExecutionHash,
		WorkflowDefinitionHash: snapshot.WorkflowDefinitionHash, FrozenInputs: snapshot.FrozenInputs,
	})
	if err != nil {
		return Compilation{}, fmt.Errorf("encode run input snapshot: %w", err)
	}
	snapshot.ContentHash = sha256Hex(encodedSnapshot)
	return Compilation{Definition: definition, RunInputSnapshot: snapshot}, nil
}

func validateContract(contract CompilerContract) error {
	if !compilerVersionPattern.MatchString(contract.CompilerVersion) ||
		!compilerVersionPattern.MatchString(contract.WorkflowTypeVersion) ||
		!compilerVersionPattern.MatchString(contract.RuntimeContractVersion) ||
		!workflowTypePattern.MatchString(contract.WorkflowType) {
		return errors.New("invalid workflow compiler contract")
	}
	return nil
}

func validateSource(source CompilationSource) (authoring.Revision, authoring.Catalog, error) {
	revision, catalog := source.Revision, source.Catalog
	if _, err := uuid.Parse(revision.ID); err != nil {
		return authoring.Revision{}, authoring.Catalog{}, errors.New("invalid authoring revision identity")
	}
	if _, err := uuid.Parse(revision.WorkspaceID); err != nil {
		return authoring.Revision{}, authoring.Catalog{}, errors.New("invalid authoring workspace identity")
	}
	if _, err := uuid.Parse(revision.ProjectID); err != nil || revision.RevisionNo < 1 {
		return authoring.Revision{}, authoring.Catalog{}, errors.New("invalid authoring project or revision identity")
	}
	if revision.CatalogKey != catalog.Key || revision.CatalogVersion != catalog.Version ||
		revision.CatalogHash != catalog.ContentHash || revision.CatalogExecutionHash != catalog.ExecutionHash {
		return authoring.Revision{}, authoring.Catalog{}, errors.New("authoring revision node catalog binding has drifted")
	}
	recomputed, err := authoring.PublishSnapshot(authoring.DraftSnapshot{
		AuthoringMode: revision.AuthoringMode, Graph: revision.Graph, Layout: revision.Layout,
		FrozenInputs: revision.FrozenInputs,
	}, catalog)
	if err != nil {
		return authoring.Revision{}, authoring.Catalog{}, fmt.Errorf("validate authoring revision: %w", err)
	}
	if recomputed.ExecutionHash != revision.ExecutionHash || recomputed.ContentHash != revision.ContentHash {
		return authoring.Revision{}, authoring.Catalog{}, errors.New("authoring revision content has drifted")
	}
	revision.RevisionSnapshot = recomputed
	return revision, catalog, nil
}

func compileGraph(graph authoring.Graph, catalog authoring.Catalog) (authoring.Graph, []NodeExecution, error) {
	definitions := make(map[string]authoring.NodeDefinition, len(catalog.Definitions))
	for _, definition := range catalog.Definitions {
		definitions[definition.Key+"@"+definition.Version] = definition
	}
	executableIDs := make(map[string]struct{}, len(graph.Nodes))
	executionGraph := authoring.Graph{Variables: graph.Variables}
	executions := make([]NodeExecution, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		definition, exists := definitions[node.DefinitionKey+"@"+node.DefinitionVersion]
		if !exists {
			return authoring.Graph{}, nil, fmt.Errorf("unknown node definition %s@%s", node.DefinitionKey, node.DefinitionVersion)
		}
		if !definition.Executable {
			continue
		}
		executableIDs[node.ID] = struct{}{}
		executionGraph.Nodes = append(executionGraph.Nodes, node)
		executions = append(executions, NodeExecution{
			NodeID: node.ID, DefinitionKey: definition.Key, DefinitionVersion: definition.Version,
			DefinitionContentHash: definition.ContentHash, Executor: definition.Executor,
			InputPorts:  append([]authoring.PortDefinition(nil), definition.InputPorts...),
			OutputPorts: append([]authoring.PortDefinition(nil), definition.OutputPorts...),
			CachePolicy: definition.CachePolicy, RiskLevel: definition.RiskLevel,
		})
	}
	if len(executionGraph.Nodes) == 0 {
		return authoring.Graph{}, nil, errors.New("workflow definition has no executable nodes")
	}
	for _, edge := range graph.Edges {
		_, fromExecutable := executableIDs[edge.FromNodeID]
		_, toExecutable := executableIDs[edge.ToNodeID]
		if fromExecutable != toExecutable {
			return authoring.Graph{}, nil, fmt.Errorf("visual node cannot participate in executable edge %s", edge.ID)
		}
		if fromExecutable && toExecutable {
			executionGraph.Edges = append(executionGraph.Edges, edge)
		}
	}
	for _, binding := range graph.Bindings {
		if _, executable := executableIDs[binding.NodeID]; executable {
			executionGraph.Bindings = append(executionGraph.Bindings, binding)
		}
	}
	slices.SortFunc(executionGraph.Nodes, func(left, right authoring.Node) int { return strings.Compare(left.ID, right.ID) })
	slices.SortFunc(executionGraph.Edges, func(left, right authoring.Edge) int { return strings.Compare(left.ID, right.ID) })
	slices.SortFunc(executionGraph.Bindings, func(left, right authoring.Binding) int {
		return strings.Compare(left.NodeID+":"+left.Port+":"+left.Variable, right.NodeID+":"+right.Port+":"+right.Variable)
	})
	slices.SortFunc(executions, func(left, right NodeExecution) int { return strings.Compare(left.NodeID, right.NodeID) })
	return executionGraph, executions, nil
}

func topologicalOrder(graph authoring.Graph) ([]string, error) {
	indegree := make(map[string]int, len(graph.Nodes))
	adjacency := make(map[string][]string, len(graph.Nodes))
	for _, node := range graph.Nodes {
		indegree[node.ID] = 0
	}
	for _, edge := range graph.Edges {
		if _, found := indegree[edge.FromNodeID]; !found {
			return nil, errors.New("workflow edge source is not executable")
		}
		if _, found := indegree[edge.ToNodeID]; !found {
			return nil, errors.New("workflow edge target is not executable")
		}
		adjacency[edge.FromNodeID] = append(adjacency[edge.FromNodeID], edge.ToNodeID)
		indegree[edge.ToNodeID]++
	}
	ready := make([]string, 0, len(indegree))
	for nodeID, count := range indegree {
		if count == 0 {
			ready = append(ready, nodeID)
		}
	}
	slices.Sort(ready)
	order := make([]string, 0, len(indegree))
	for len(ready) > 0 {
		nodeID := ready[0]
		ready = ready[1:]
		order = append(order, nodeID)
		slices.Sort(adjacency[nodeID])
		for _, next := range adjacency[nodeID] {
			indegree[next]--
			if indegree[next] == 0 {
				ready = append(ready, next)
			}
		}
		slices.Sort(ready)
	}
	if len(order) != len(indegree) {
		return nil, errors.New("workflow definition contains a cycle")
	}
	return order, nil
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
