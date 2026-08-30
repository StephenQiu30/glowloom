package agent_test

import (
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
)

func TestStoryGraphExecutionGrantBindsAttemptFencingInputAndPolicy(t *testing.T) {
	invocation := fixtureStageInvocation(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	clock := now
	signer, err := grant.NewSigner("a-strong-agent-execution-secret-123", func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	value, err := signer.Issue(invocation, 2, 7)
	if err != nil {
		t.Fatal(err)
	}
	if err = signer.Verify(value, invocation, 2, 7); err != nil {
		t.Fatal(err)
	}
	if err = signer.Verify(value, invocation, 1, 7); err == nil {
		t.Fatal("grant authorized a different attempt")
	}
	if err = signer.Verify(value, invocation, 2, 8); err == nil {
		t.Fatal("grant authorized a different fencing token")
	}
	changed := invocation
	changed.ExecutionPolicy.MaxModelCalls--
	changed.InputHash, err = changed.ComputeInputHash()
	if err != nil {
		t.Fatal(err)
	}
	if err = signer.Verify(value, changed, 2, 7); err == nil {
		t.Fatal("grant authorized a changed execution policy and input")
	}
	clock = now.Add(grant.TTL)
	if err = signer.Verify(value, invocation, 2, 7); err == nil {
		t.Fatal("grant remained valid at its expiry")
	}
}

func fixtureStageInvocation(t *testing.T) contract.StageInvocation {
	t.Helper()
	fixture := loadStoryGraphWireFixture(t)
	invocation, err := contract.DecodeStageInvocation(fixture.ValidInvocation)
	if err != nil {
		t.Fatal(err)
	}
	return invocation
}

func TestSceneAnalysisExecutionGrantBindsAttemptInputReleaseAndImage(t *testing.T) {
	fixture := loadStoryGraphSceneAnalysisWireFixture(t)
	invocation, err := contract.DecodeSceneAnalysisInvocation(fixture.ValidInvocation)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0).UTC()
	clock := now
	signer, err := grant.NewSigner("a-strong-agent-execution-secret-123", func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	value, err := signer.IssueSceneAnalysis(invocation)
	if err != nil {
		t.Fatal(err)
	}
	if err = signer.VerifySceneAnalysis(value, invocation); err != nil {
		t.Fatal(err)
	}
	changedAttempt := invocation
	changedAttempt.AttemptID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	if err = signer.VerifySceneAnalysis(value, changedAttempt); err == nil {
		t.Fatal("Scene Analysis grant authorized a different attempt")
	}
	changedImage := invocation
	changedImage.StageRelease.AgentImageDigest = "sha256:" + strings.Repeat("9", 64)
	changedImage.InputHash, err = changedImage.ComputeInputHash()
	if err != nil {
		t.Fatal(err)
	}
	if err = signer.VerifySceneAnalysis(value, changedImage); err == nil {
		t.Fatal("Scene Analysis grant authorized a different runtime image")
	}
	clock = now.Add(grant.TTL)
	if err = signer.VerifySceneAnalysis(value, invocation); err == nil {
		t.Fatal("Scene Analysis grant remained valid at its expiry")
	}
}
