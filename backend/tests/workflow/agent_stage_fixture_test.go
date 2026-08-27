package workflow_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

func mustStageInvocationRecord(
	t *testing.T,
	id, workspaceID, requestID uuid.UUID,
	requestType, stage, status string,
	now time.Time,
) model.AgentInvocation {
	t.Helper()
	policy := contract.StoryGraphDefinition().ExecutionPolicy()
	if stage == "draft_storyboard" {
		policy.MaxModelCalls = 1
	}
	payload := contract.StageInvocationPayload{
		Stage: stage, ShardKey: "test-shard", WorkspaceID: workspaceID.String(), ProjectID: uuid.NewString(),
		SourceRefs: []contract.StageSourceRef{}, UpstreamCandidates: []contract.StageUpstreamCandidateRef{},
		ShardManifestRef: contract.ShardManifestRef{ManifestID: uuid.NewString(), Version: 1, Hash: strings.Repeat("c", 64)},
		Shard:            contract.InvocationShard{Kind: "test", Key: "test-shard", TreePath: "0"}, StageInput: json.RawMessage(`{}`),
	}
	invocation, err := contract.NewStageInvocation(id.String(), policy, payload)
	if err != nil {
		t.Fatal(err)
	}
	stageInstanceKey, err := invocation.StageInstanceKey()
	if err != nil {
		t.Fatal(err)
	}
	encodedPolicy, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return model.AgentInvocation{
		ID: id, WorkspaceID: workspaceID, RequestType: requestType, RequestID: requestID,
		Kind: "storygraph_stage", WireSchemaVersion: contract.StoryGraphWireSchemaVersion,
		Stage: stage, ShardKey: payload.ShardKey, StageInstanceKey: stageInstanceKey,
		ShardManifestHash: payload.ShardManifestRef.Hash, InputHash: invocation.InputHash,
		ExecutionPolicy: encodedPolicy, Payload: encodedPayload,
		Status: status, CreatedAt: now, UpdatedAt: now,
	}
}
