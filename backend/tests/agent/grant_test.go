package agent_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
)

func TestGrantBindsInvocationAndExpires(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	clock := now
	signer, err := grant.NewSigner("lanverse-agent-grant-test-secret-32-bytes", func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	policy, err := contract.ExecutionPolicyFor("production_bible")
	if err != nil {
		t.Fatal(err)
	}
	invocation := contract.Invocation{InvocationID: "00000000-0000-0000-0000-000000000001", Kind: "production_bible", InputHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SchemaVersion: contract.SchemaVersion, ExecutionPolicy: policy, Payload: json.RawMessage(`{}`)}
	value, err := signer.Issue(invocation)
	if err != nil {
		t.Fatal(err)
	}
	if err = signer.Verify(value, invocation); err != nil {
		t.Fatal(err)
	}

	changed := invocation
	changed.InputHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err = signer.Verify(value, changed); err == nil {
		t.Fatal("grant authorized a changed input hash")
	}
	changed = invocation
	changed.ExecutionPolicy.MaxModelCalls--
	if err = signer.Verify(value, changed); err == nil {
		t.Fatal("grant authorized a changed execution budget")
	}
	clock = now.Add(grant.TTL)
	if err = signer.Verify(value, invocation); err == nil {
		t.Fatal("grant remained valid at its expiry")
	}
}
