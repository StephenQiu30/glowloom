package agent_test

import (
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestInvocationCandidateRevisionHashFreezesOriginAndContent(t *testing.T) {
	origin := &contract.InvocationCandidateOrigin{
		SourceInvocationID: "20000000-0000-0000-0000-000000000001",
		SourceResultHash:   strings.Repeat("b", 64),
	}
	material := contract.CandidateRevisionMaterial{
		StageInstanceKey: strings.Repeat("a", 64), RevisionNo: 1,
		OriginKind: "invocation", InvocationOrigin: origin,
		CandidateContentHash: strings.Repeat("b", 64),
	}
	first, err := material.Hash()
	if err != nil {
		t.Fatal(err)
	}
	material.InvocationOrigin.SourceResultHash = strings.Repeat("c", 64)
	changedOrigin, err := material.Hash()
	if err != nil || changedOrigin == first {
		t.Fatalf("origin mutation kept revision hash: first=%s changed=%s err=%v", first, changedOrigin, err)
	}
	material.InvocationOrigin.SourceResultHash = strings.Repeat("b", 64)
	material.CandidateContentHash = strings.Repeat("d", 64)
	changedContent, err := material.Hash()
	if err != nil || changedContent == first {
		t.Fatalf("content mutation kept revision hash: first=%s changed=%s err=%v", first, changedContent, err)
	}
}
