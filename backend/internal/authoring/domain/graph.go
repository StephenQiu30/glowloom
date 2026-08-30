package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func ValidateGraph(graph Graph, catalog Catalog) (Graph, error) {
	if len(graph.Nodes) == 0 || len(graph.Nodes) > 2000 || len(graph.Edges) > 10000 {
		return Graph{}, errors.New("authoring graph is empty or exceeds limits")
	}
	if len(catalog.compiled) == 0 || len(catalog.ContentHash) != 64 || len(catalog.ExecutionHash) != 64 {
		return Graph{}, errors.New("node catalog is not compiled")
	}

	normalized := Graph{
		Nodes: append([]Node(nil), graph.Nodes...), Edges: append([]Edge(nil), graph.Edges...),
		Variables: make(map[string]json.RawMessage, len(graph.Variables)), Bindings: append([]Binding(nil), graph.Bindings...),
	}
	nodes := make(map[string]NodeDefinition, len(normalized.Nodes))
	for index := range normalized.Nodes {
		node := &normalized.Nodes[index]
		if !keyPattern.MatchString(node.ID) {
			return Graph{}, fmt.Errorf("invalid node id %q", node.ID)
		}
		if _, exists := nodes[node.ID]; exists {
			return Graph{}, fmt.Errorf("duplicate node id %s", node.ID)
		}
		compiled, exists := catalog.compiled[definitionIdentity(node.DefinitionKey, node.DefinitionVersion)]
		if !exists {
			return Graph{}, fmt.Errorf("unknown node definition %s", definitionIdentity(node.DefinitionKey, node.DefinitionVersion))
		}
		config, err := canonicalJSON(node.Config, true)
		if err != nil {
			return Graph{}, fmt.Errorf("node %s config: %w", node.ID, err)
		}
		instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(config))
		if err != nil {
			return Graph{}, fmt.Errorf("node %s config: %w", node.ID, err)
		}
		if err = compiled.schema.Validate(instance); err != nil {
			return Graph{}, fmt.Errorf("node %s config does not match schema: %w", node.ID, err)
		}
		node.Config = config
		nodes[node.ID] = compiled.definition
	}
	for key, value := range graph.Variables {
		if !keyPattern.MatchString(key) {
			return Graph{}, fmt.Errorf("invalid variable %q", key)
		}
		canonical, err := canonicalJSON(value, false)
		if err != nil {
			return Graph{}, fmt.Errorf("variable %s: %w", key, err)
		}
		normalized.Variables[key] = canonical
	}
	if len(normalized.Variables) == 0 {
		normalized.Variables = nil
	}

	incoming := make(map[string]int)
	boundInputs, err := validateBindings(normalized.Bindings, nodes, normalized.Variables)
	if err != nil {
		return Graph{}, err
	}
	for _, inputKey := range boundInputs {
		incoming[inputKey] = 1
	}
	adjacency := make(map[string][]string, len(nodes))
	indegree := make(map[string]int, len(nodes))
	edgeIDs := make(map[string]struct{}, len(normalized.Edges))
	for nodeID := range nodes {
		indegree[nodeID] = 0
	}
	for _, edge := range normalized.Edges {
		if !keyPattern.MatchString(edge.ID) {
			return Graph{}, fmt.Errorf("invalid edge id %q", edge.ID)
		}
		if _, exists := edgeIDs[edge.ID]; exists {
			return Graph{}, fmt.Errorf("duplicate edge id %s", edge.ID)
		}
		edgeIDs[edge.ID] = struct{}{}
		from, fromFound := nodes[edge.FromNodeID]
		to, toFound := nodes[edge.ToNodeID]
		if !fromFound || !toFound {
			return Graph{}, fmt.Errorf("edge %s references an unknown node", edge.ID)
		}
		fromPort, fromFound := findPort(from.OutputPorts, edge.FromPort)
		toPort, toFound := findPort(to.InputPorts, edge.ToPort)
		if !fromFound || !toFound || fromPort.ValueType != toPort.ValueType {
			return Graph{}, fmt.Errorf("edge %s has incompatible ports", edge.ID)
		}
		inputKey := edge.ToNodeID + ":" + edge.ToPort
		incoming[inputKey]++
		if incoming[inputKey] > 1 {
			return Graph{}, fmt.Errorf("input %s has multiple sources", inputKey)
		}
		adjacency[edge.FromNodeID] = append(adjacency[edge.FromNodeID], edge.ToNodeID)
		indegree[edge.ToNodeID]++
	}
	for nodeID, definition := range nodes {
		for _, port := range definition.InputPorts {
			if port.Required && incoming[nodeID+":"+port.Key] != 1 {
				return Graph{}, fmt.Errorf("node %s is missing required input %s", nodeID, port.Key)
			}
		}
	}
	if hasCycle(adjacency, indegree) {
		return Graph{}, errors.New("general graph cycles are not allowed")
	}

	slices.SortFunc(normalized.Nodes, func(left, right Node) int { return strings.Compare(left.ID, right.ID) })
	slices.SortFunc(normalized.Edges, func(left, right Edge) int { return strings.Compare(left.ID, right.ID) })
	slices.SortFunc(normalized.Bindings, func(left, right Binding) int {
		return strings.Compare(left.NodeID+":"+left.Port+":"+left.Variable, right.NodeID+":"+right.Port+":"+right.Variable)
	})
	return normalized, nil
}

func PublishSnapshot(draft DraftSnapshot, catalog Catalog) (RevisionSnapshot, error) {
	mode := strings.ToUpper(strings.TrimSpace(draft.AuthoringMode))
	if mode != "GUIDED" && mode != "CANVAS" {
		return RevisionSnapshot{}, errors.New("authoring mode must be GUIDED or CANVAS")
	}
	graph, err := ValidateGraph(draft.Graph, catalog)
	if err != nil {
		return RevisionSnapshot{}, err
	}
	layout, err := canonicalJSON(draft.Layout, true)
	if err != nil {
		return RevisionSnapshot{}, fmt.Errorf("layout: %w", err)
	}
	inputs := append([]FrozenReference(nil), draft.FrozenInputs...)
	if len(inputs) == 0 {
		return RevisionSnapshot{}, errors.New("at least one frozen input is required")
	}
	slices.SortFunc(inputs, func(left, right FrozenReference) int {
		return strings.Compare(left.Kind+":"+left.ID+":"+left.Version, right.Kind+":"+right.ID+":"+right.Version)
	})
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		identity := input.Kind + ":" + input.ID + ":" + input.Version
		if !keyPattern.MatchString(input.Kind) || input.Version == "" || !hashPattern.MatchString(input.Hash) {
			return RevisionSnapshot{}, errors.New("invalid frozen input")
		}
		if _, err = uuid.Parse(input.ID); err != nil {
			return RevisionSnapshot{}, errors.New("invalid frozen input id")
		}
		if _, exists := seen[identity]; exists {
			return RevisionSnapshot{}, errors.New("duplicate frozen input")
		}
		seen[identity] = struct{}{}
	}

	snapshot := RevisionSnapshot{
		AuthoringMode: mode, Graph: graph, Layout: layout, FrozenInputs: inputs,
		CatalogKey: catalog.Key, CatalogVersion: catalog.Version, CatalogHash: catalog.ContentHash,
		CatalogExecutionHash: catalog.ExecutionHash,
	}
	executionGraph := executableGraph(graph, catalog)
	snapshot.ExecutionHash, err = hashValue(struct {
		SchemaVersion        string            `json:"schema_version"`
		Graph                Graph             `json:"graph"`
		FrozenInputs         []FrozenReference `json:"frozen_inputs"`
		CatalogKey           string            `json:"catalog_key"`
		CatalogVersion       string            `json:"catalog_version"`
		CatalogExecutionHash string            `json:"catalog_execution_hash"`
	}{"authoring-execution", executionGraph, inputs, catalog.Key, catalog.Version, catalog.ExecutionHash})
	if err != nil {
		return RevisionSnapshot{}, err
	}
	snapshot.ContentHash, err = hashValue(struct {
		AuthoringMode  string            `json:"authoring_mode"`
		Graph          Graph             `json:"graph"`
		Layout         json.RawMessage   `json:"layout"`
		FrozenInputs   []FrozenReference `json:"frozen_inputs"`
		CatalogKey     string            `json:"catalog_key"`
		CatalogVersion string            `json:"catalog_version"`
		CatalogHash    string            `json:"catalog_hash"`
	}{mode, graph, layout, inputs, catalog.Key, catalog.Version, catalog.ContentHash})
	return snapshot, err
}

func executableGraph(graph Graph, catalog Catalog) Graph {
	executable := make(map[string]struct{}, len(graph.Nodes))
	result := Graph{Variables: graph.Variables}
	for _, node := range graph.Nodes {
		compiled := catalog.compiled[definitionIdentity(node.DefinitionKey, node.DefinitionVersion)]
		if compiled.definition.Executable {
			executable[node.ID] = struct{}{}
			result.Nodes = append(result.Nodes, node)
		}
	}
	for _, edge := range graph.Edges {
		_, fromExecutable := executable[edge.FromNodeID]
		_, toExecutable := executable[edge.ToNodeID]
		if fromExecutable && toExecutable {
			result.Edges = append(result.Edges, edge)
		}
	}
	for _, binding := range graph.Bindings {
		if _, exists := executable[binding.NodeID]; exists {
			result.Bindings = append(result.Bindings, binding)
		}
	}
	return result
}

func validateBindings(bindings []Binding, nodes map[string]NodeDefinition, variables map[string]json.RawMessage) ([]string, error) {
	seen := make(map[string]struct{}, len(bindings))
	inputs := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		definition, nodeFound := nodes[binding.NodeID]
		_, variableFound := variables[binding.Variable]
		port, portFound := findPort(definition.InputPorts, binding.Port)
		identity := binding.NodeID + ":" + binding.Port
		if !nodeFound || !variableFound || !portFound || port.ValueType != binding.ValueType {
			return nil, fmt.Errorf("invalid binding %s", identity)
		}
		if _, exists := seen[identity]; exists {
			return nil, fmt.Errorf("duplicate binding %s", identity)
		}
		seen[identity] = struct{}{}
		inputs = append(inputs, identity)
	}
	return inputs, nil
}

func findPort(ports []PortDefinition, key string) (PortDefinition, bool) {
	for _, port := range ports {
		if port.Key == key {
			return port, true
		}
	}
	return PortDefinition{}, false
}

func hasCycle(adjacency map[string][]string, indegree map[string]int) bool {
	queue := make([]string, 0, len(indegree))
	for nodeID, count := range indegree {
		if count == 0 {
			queue = append(queue, nodeID)
		}
	}
	visited := 0
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		visited++
		for _, next := range adjacency[nodeID] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	return visited != len(indegree)
}

func hashValue(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}
