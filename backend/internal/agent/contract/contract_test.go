package contract

import (
	"encoding/json"
	"testing"
)

func TestExecutionPolicyForFreezesCurrentCandidateDefinitions(t *testing.T) {
	tests := []struct {
		kind          string
		definition    string
		prompt        string
		skillBundle   string
		outputSchema  string
		maxModelCalls int
		policyHash    string
	}{
		{"production_bible", "production-bible-harness-v1", "production-bible-prompt-v1", "production-bible-skills-v1", "production-bible-schema-v1", 3, "66f9586bc95df6f1714735b115e09d9122658bf720fb712a5ef36d2b9ba78b99"},
		{"storyboard_draft", "storyboard-harness-v1", "storyboard-prompt-v1", "storyboard-skills-v1", "storyboard-draft-schema-v1", 1, "bc6030d16816445388853a7ae4dd0f65dea8de4e57a4c969e611afd67f3ed5b8"},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			policy, err := ExecutionPolicyFor(test.kind)
			if err != nil {
				t.Fatal(err)
			}
			if policy.DefinitionKey != test.kind || policy.DefinitionVersion != test.definition || policy.PromptVersion != test.prompt || policy.SkillBundleVersion != test.skillBundle || policy.OutputSchemaVersion != test.outputSchema || policy.ModelCapability != "structured_text" || policy.MaxModelCalls != test.maxModelCalls || len(policy.AllowedTools) != 0 {
				t.Fatalf("unexpected policy: %#v", policy)
			}
			policyHash, err := policy.Hash()
			if err != nil || policyHash != test.policyHash {
				t.Fatalf("execution policy hash = %s, want %s, error = %v", policyHash, test.policyHash, err)
			}
		})
	}
	if _, err := ExecutionPolicyFor("script_structure"); err == nil {
		t.Fatal("unsupported compatibility definition remained callable")
	}
}

func TestInvocationRejectsPolicyOutsideDefinitionManifest(t *testing.T) {
	policy, err := ExecutionPolicyFor("storyboard_draft")
	if err != nil {
		t.Fatal(err)
	}
	invocation := Invocation{
		InvocationID: "00000000-0000-0000-0000-000000000001",
		Kind:         "storyboard_draft", InputHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SchemaVersion: SchemaVersion, ExecutionPolicy: policy, Payload: json.RawMessage(`{}`),
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
}

func TestAgentFailureCodesHaveFrozenStatusAndRetrySemantics(t *testing.T) {
	policy, err := ExecutionPolicyFor("storyboard_draft")
	if err != nil {
		t.Fatal(err)
	}
	invocation := Invocation{
		InvocationID: "00000000-0000-0000-0000-000000000001",
		Kind:         "storyboard_draft", InputHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SchemaVersion: SchemaVersion, ExecutionPolicy: policy, Payload: json.RawMessage(`{}`),
	}
	tests := []struct {
		code      string
		status    string
		retryable bool
	}{
		{"execution_budget_exceeded", "failed", false},
		{"tool_not_allowed", "failed", false},
		{"candidate_schema_invalid", "failed", false},
		{"runtime_unavailable", "unknown", true},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			result := Result{
				InvocationID: invocation.InvocationID, Kind: invocation.Kind, InputHash: invocation.InputHash,
				Status: test.status, SchemaVersion: SchemaVersion, Candidate: json.RawMessage(`null`),
				Executor: Executor{Name: "codex-cli", Version: "test", Model: "test"},
				Error:    &ResultError{Code: test.code, Summary: "test failure", Retryable: test.retryable},
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
