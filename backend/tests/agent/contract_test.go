package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestExecutionPolicyForFreezesCurrentCandidateDefinitions(t *testing.T) {
	tests := []struct {
		kind          string
		definition    string
		prompt        string
		skillBundle   string
		outputSchema  string
		maxModelCalls int
		maxSeconds    int
		policyHash    string
	}{
		{"production_bible", "production-bible-harness-v1", "production-bible-prompt-v1", "production-bible-skills-v1", "production-bible-schema-v1", 3, 900, "6f2a808344083bdcdc0d542d94861bb25511f8373a48958c3e0c02f46c3f15a2"},
		{"storyboard_draft", "storyboard-harness-v1", "storyboard-prompt-v1", "storyboard-skills-v1", "storyboard-draft-schema-v1", 1, 600, "a36be6c82351d8628536721d842316817495c8c43ff5f34662cef2516aa09a0b"},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			policy, err := contract.ExecutionPolicyFor(test.kind)
			if err != nil {
				t.Fatal(err)
			}
			if policy.DefinitionKey != test.kind || policy.DefinitionVersion != test.definition || policy.PromptVersion != test.prompt || policy.SkillBundleVersion != test.skillBundle || policy.OutputSchemaVersion != test.outputSchema || policy.ModelCapability != "structured_text" || policy.MaxModelCalls != test.maxModelCalls || policy.MaxExecutionSeconds != test.maxSeconds || len(policy.AllowedTools) != 0 {
				t.Fatalf("unexpected policy: %#v", policy)
			}
			policyHash, err := policy.Hash()
			if err != nil || policyHash != test.policyHash {
				t.Fatalf("execution policy hash = %s, want %s, error = %v", policyHash, test.policyHash, err)
			}
		})
	}
	if _, err := contract.ExecutionPolicyFor("script_structure"); err == nil {
		t.Fatal("unsupported compatibility definition remained callable")
	}
}

func TestInvocationRejectsPolicyOutsideDefinitionManifest(t *testing.T) {
	policy, err := contract.ExecutionPolicyFor("storyboard_draft")
	if err != nil {
		t.Fatal(err)
	}
	invocation := contract.Invocation{
		InvocationID: "00000000-0000-0000-0000-000000000001",
		Kind:         "storyboard_draft", InputHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SchemaVersion: contract.SchemaVersion, ExecutionPolicy: policy, Payload: json.RawMessage(`{}`),
	}
	if err = invocation.Validate(); err != nil {
		t.Fatal(err)
	}
	invocation.ExecutionPolicy.AllowedTools = []string{"shell"}
	if err = invocation.Validate(); err == nil {
		t.Fatal("invocation accepted a tool outside the definition manifest")
	}
	invocation.ExecutionPolicy = policy
	invocation.ExecutionPolicy.MaxModelCalls = policy.MaxModelCalls + 1
	if err = invocation.Validate(); err == nil {
		t.Fatal("invocation accepted a model-call budget above the definition manifest")
	}
	invocation.ExecutionPolicy = policy
	invocation.ExecutionPolicy.MaxExecutionSeconds = policy.MaxExecutionSeconds + 1
	if err = invocation.Validate(); err == nil {
		t.Fatal("invocation accepted an execution deadline above the definition manifest")
	}
}

func TestAgentFailureCodesHaveFrozenStatusAndRetrySemantics(t *testing.T) {
	policy, err := contract.ExecutionPolicyFor("storyboard_draft")
	if err != nil {
		t.Fatal(err)
	}
	invocation := contract.Invocation{
		InvocationID: "00000000-0000-0000-0000-000000000001",
		Kind:         "storyboard_draft", InputHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SchemaVersion: contract.SchemaVersion, ExecutionPolicy: policy, Payload: json.RawMessage(`{}`),
	}
	tests := []struct {
		code      string
		status    string
		retryable bool
	}{
		{"execution_budget_exceeded", "failed", false},
		{"tool_not_allowed", "failed", false},
		{"candidate_schema_invalid", "failed", false},
		{"execution_deadline_exceeded", "failed", false},
		{"runtime_unavailable", "unknown", true},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			result := contract.Result{
				InvocationID: invocation.InvocationID, Kind: invocation.Kind, InputHash: invocation.InputHash,
				Status: test.status, SchemaVersion: contract.SchemaVersion, Candidate: json.RawMessage(`null`),
				Executor: contract.Executor{Name: "codex-cli", Version: "test", Model: "test"},
				Error:    &contract.ResultError{Code: test.code, Summary: "test failure", Retryable: test.retryable},
			}
			if err := result.ValidateFor(invocation); err != nil {
				t.Fatal(err)
			}
			result.Error.Retryable = !result.Error.Retryable
			if err := result.ValidateFor(invocation); err == nil {
				t.Fatal("agent result accepted changed retry semantics")
			}
		})
	}
}
