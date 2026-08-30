package agent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestStoryGraphFailureCodesHaveFrozenStatusAndRetrySemantics(t *testing.T) {
	invocation := fixtureStageInvocation(t)
	var tests []struct {
		Code      string `json:"code"`
		Status    string `json:"status"`
		Retryable bool   `json:"retryable"`
	}
	_, currentFile, _, _ := runtime.Caller(0)
	encoded, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "../fixtures/agent/storygraph-errors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(encoded, &tests); err != nil || len(tests) != 11 {
		t.Fatalf("error fixture = %#v, err=%v", tests, err)
	}
	for _, test := range tests {
		t.Run(test.Code, func(t *testing.T) {
			result := contract.StageResult{
				InvocationID: invocation.InvocationID, Kind: "storygraph_stage",
				WireSchemaVersion: contract.StoryGraphWireSchemaVersion,
				Stage:             invocation.Payload.Stage, ShardKey: invocation.Payload.ShardKey,
				Status: test.Status, CandidateType: "source_evidence_candidate",
				Candidate: json.RawMessage(`null`), InputHash: invocation.InputHash,
				Issues:   []contract.StageIssue{},
				Executor: contract.Executor{Name: "codex-cli", Version: "test", Model: "test"},
				Error:    &contract.ResultError{Code: test.Code, Summary: "test", Retryable: test.Retryable},
			}
			if err := result.ValidateFor(invocation); err != nil {
				t.Fatal(err)
			}
			result.Error.Retryable = !result.Error.Retryable
			if err := result.ValidateFor(invocation); err == nil {
				t.Fatal("changed retry semantics were accepted")
			}
		})
	}
}
