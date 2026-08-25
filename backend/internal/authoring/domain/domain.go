package domain

import "encoding/json"

type PortDefinition struct {
	Key       string `json:"key"`
	ValueType string `json:"value_type"`
	Required  bool   `json:"required"`
}

type NodeDefinition struct {
	Key          string           `json:"key"`
	Version      string           `json:"version"`
	Name         string           `json:"name"`
	Category     string           `json:"category"`
	Executor     string           `json:"executor"`
	InputPorts   []PortDefinition `json:"input_ports"`
	OutputPorts  []PortDefinition `json:"output_ports"`
	ConfigSchema json.RawMessage  `json:"config_schema"`
	CachePolicy  string           `json:"cache_policy"`
	RiskLevel    string           `json:"risk_level"`
	Executable   bool             `json:"executable"`
	ContentHash  string           `json:"content_hash"`
}

type Catalog struct {
	Key           string           `json:"key"`
	Version       string           `json:"version"`
	Definitions   []NodeDefinition `json:"definitions"`
	ContentHash   string           `json:"content_hash"`
	ExecutionHash string           `json:"execution_hash"`
	compiled      map[string]compiledDefinition
}

type Node struct {
	ID                string          `json:"id"`
	DefinitionKey     string          `json:"definition_key"`
	DefinitionVersion string          `json:"definition_version"`
	Config            json.RawMessage `json:"config"`
}

type Edge struct {
	ID         string `json:"id"`
	FromNodeID string `json:"from_node_id"`
	FromPort   string `json:"from_port"`
	ToNodeID   string `json:"to_node_id"`
	ToPort     string `json:"to_port"`
}

type Binding struct {
	NodeID    string `json:"node_id"`
	Port      string `json:"port"`
	Variable  string `json:"variable"`
	ValueType string `json:"value_type"`
}

type Graph struct {
	Nodes     []Node                     `json:"nodes"`
	Edges     []Edge                     `json:"edges"`
	Variables map[string]json.RawMessage `json:"variables,omitempty"`
	Bindings  []Binding                  `json:"bindings,omitempty"`
}

type FrozenReference struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Version string `json:"version"`
	Hash    string `json:"hash"`
}

type DraftSnapshot struct {
	AuthoringMode string            `json:"authoring_mode"`
	Graph         Graph             `json:"graph"`
	Layout        json.RawMessage   `json:"layout"`
	FrozenInputs  []FrozenReference `json:"frozen_inputs"`
}

type RevisionSnapshot struct {
	AuthoringMode        string            `json:"authoring_mode"`
	Graph                Graph             `json:"graph"`
	Layout               json.RawMessage   `json:"layout"`
	FrozenInputs         []FrozenReference `json:"frozen_inputs"`
	CatalogKey           string            `json:"catalog_key"`
	CatalogVersion       string            `json:"catalog_version"`
	CatalogHash          string            `json:"catalog_hash"`
	CatalogExecutionHash string            `json:"catalog_execution_hash"`
	ExecutionHash        string            `json:"execution_hash"`
	ContentHash          string            `json:"content_hash"`
}
