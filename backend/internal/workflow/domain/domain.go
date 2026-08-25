package domain

import (
	"encoding/json"
	"time"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
)

type CompilerContract struct {
	CompilerVersion        string `json:"compiler_version"`
	WorkflowType           string `json:"workflow_type"`
	WorkflowTypeVersion    string `json:"workflow_type_version"`
	RuntimeContractVersion string `json:"runtime_contract_version"`
}

type CompilationSource struct {
	Revision authoring.Revision
	Catalog  authoring.Catalog
}

type NodeExecution struct {
	NodeID                string `json:"node_id"`
	DefinitionKey         string `json:"definition_key"`
	DefinitionVersion     string `json:"definition_version"`
	DefinitionContentHash string `json:"definition_content_hash"`
	Executor              string `json:"executor"`
	CachePolicy           string `json:"cache_policy"`
	RiskLevel             string `json:"risk_level"`
}

type WorkflowDefinitionVersion struct {
	SchemaVersion                  string          `json:"schema_version"`
	WorkspaceID                    string          `json:"workspace_id"`
	ProjectID                      string          `json:"project_id"`
	AuthoringRevisionID            string          `json:"authoring_revision_id"`
	AuthoringRevisionContentHash   string          `json:"authoring_revision_content_hash"`
	AuthoringRevisionExecutionHash string          `json:"authoring_revision_execution_hash"`
	NodeCatalogVersionID           string          `json:"node_catalog_version_id"`
	NodeCatalogKey                 string          `json:"node_catalog_key"`
	NodeCatalogVersion             string          `json:"node_catalog_version"`
	NodeCatalogExecutionHash       string          `json:"node_catalog_execution_hash"`
	CompilerVersion                string          `json:"compiler_version"`
	WorkflowType                   string          `json:"workflow_type"`
	WorkflowTypeVersion            string          `json:"workflow_type_version"`
	RuntimeContractVersion         string          `json:"runtime_contract_version"`
	ExecutionGraph                 authoring.Graph `json:"execution_graph"`
	ExecutionOrder                 []string        `json:"execution_order"`
	NodeExecutions                 []NodeExecution `json:"node_executions"`
	ContentHash                    string          `json:"content_hash"`
}

type RunInputSnapshot struct {
	SchemaVersion          string                      `json:"schema_version"`
	WorkspaceID            string                      `json:"workspace_id"`
	ProjectID              string                      `json:"project_id"`
	AuthoringRevisionID    string                      `json:"authoring_revision_id"`
	AuthoringExecutionHash string                      `json:"authoring_execution_hash"`
	WorkflowDefinitionHash string                      `json:"workflow_definition_hash"`
	FrozenInputs           []authoring.FrozenReference `json:"frozen_inputs"`
	ContentHash            string                      `json:"content_hash"`
}

type Compilation struct {
	Definition       WorkflowDefinitionVersion `json:"definition"`
	RunInputSnapshot RunInputSnapshot          `json:"run_input_snapshot"`
}

type CompiledFacts struct {
	DefinitionID, RunInputSnapshotID string
	Compilation
	CreatedBy string
	CreatedAt time.Time
}

func SystemCompilerContract() CompilerContract {
	return CompilerContract{
		CompilerVersion: "1.0.0", WorkflowType: "lanverse.episode-production",
		WorkflowTypeVersion: "1.0.0", RuntimeContractVersion: "1.0.0",
	}
}

type definitionHashInput struct {
	SchemaVersion                  string          `json:"schema_version"`
	AuthoringRevisionExecutionHash string          `json:"authoring_revision_execution_hash"`
	NodeCatalogKey                 string          `json:"node_catalog_key"`
	NodeCatalogVersion             string          `json:"node_catalog_version"`
	NodeCatalogExecutionHash       string          `json:"node_catalog_execution_hash"`
	CompilerVersion                string          `json:"compiler_version"`
	WorkflowType                   string          `json:"workflow_type"`
	WorkflowTypeVersion            string          `json:"workflow_type_version"`
	RuntimeContractVersion         string          `json:"runtime_contract_version"`
	ExecutionGraph                 authoring.Graph `json:"execution_graph"`
	ExecutionOrder                 []string        `json:"execution_order"`
	NodeExecutions                 []NodeExecution `json:"node_executions"`
}

type runInputHashInput struct {
	SchemaVersion          string                      `json:"schema_version"`
	AuthoringExecutionHash string                      `json:"authoring_execution_hash"`
	WorkflowDefinitionHash string                      `json:"workflow_definition_hash"`
	FrozenInputs           []authoring.FrozenReference `json:"frozen_inputs"`
}

func marshalDefinition(value WorkflowDefinitionVersion) (json.RawMessage, error) {
	return json.Marshal(definitionHashInput{
		SchemaVersion: value.SchemaVersion, AuthoringRevisionExecutionHash: value.AuthoringRevisionExecutionHash,
		NodeCatalogKey: value.NodeCatalogKey, NodeCatalogVersion: value.NodeCatalogVersion,
		NodeCatalogExecutionHash: value.NodeCatalogExecutionHash, CompilerVersion: value.CompilerVersion,
		WorkflowType: value.WorkflowType, WorkflowTypeVersion: value.WorkflowTypeVersion,
		RuntimeContractVersion: value.RuntimeContractVersion, ExecutionGraph: value.ExecutionGraph,
		ExecutionOrder: value.ExecutionOrder, NodeExecutions: value.NodeExecutions,
	})
}
