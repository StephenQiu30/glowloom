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

func TestAggregateCandidateRevisionHashFreezesManifestLeavesAndContent(t *testing.T) {
	origin := &contract.AggregateCandidateOrigin{
		ShardManifestID: "20000000-0000-0000-0000-000000000010", ManifestVersion: 1,
		ShardManifestHash: strings.Repeat("b", 64),
		LeafCandidates: []contract.AggregateLeafCandidateRef{{
			StageInstanceKey: strings.Repeat("c", 64), ShardKey: "leaf:0001",
			CandidateRevisionID:   "20000000-0000-0000-0000-000000000011",
			CandidateRevisionHash: strings.Repeat("d", 64),
		}},
	}
	material := contract.CandidateRevisionMaterial{
		StageInstanceKey: strings.Repeat("a", 64), RevisionNo: 1,
		OriginKind: "aggregate", AggregateOrigin: origin,
		CandidateContentHash: strings.Repeat("e", 64),
	}
	first, err := material.Hash()
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(){
		func() { material.AggregateOrigin.ManifestVersion = 2 },
		func() { material.AggregateOrigin.ShardManifestHash = strings.Repeat("f", 64) },
		func() { material.AggregateOrigin.LeafCandidates[0].CandidateRevisionHash = strings.Repeat("9", 64) },
		func() { material.CandidateContentHash = strings.Repeat("8", 64) },
	}
	for index, mutate := range mutations {
		material.AggregateOrigin.ManifestVersion = 1
		material.AggregateOrigin.ShardManifestHash = strings.Repeat("b", 64)
		material.AggregateOrigin.LeafCandidates[0].CandidateRevisionHash = strings.Repeat("d", 64)
		material.CandidateContentHash = strings.Repeat("e", 64)
		mutate()
		changed, hashErr := material.Hash()
		if hashErr != nil || changed == first {
			t.Fatalf("aggregate mutation %d kept revision hash: first=%s changed=%s err=%v", index, first, changed, hashErr)
		}
	}
}

func TestRepairCandidateRevisionHashFreezesParentOriginAndContent(t *testing.T) {
	origin := &contract.RepairCandidateOrigin{
		RepairInvocationID: "20000000-0000-0000-0000-000000000002",
		RepairResultHash:   strings.Repeat("c", 64),
	}
	parentHash := strings.Repeat("b", 64)
	material := contract.CandidateRevisionMaterial{
		StageInstanceKey: strings.Repeat("a", 64), RevisionNo: 2,
		ParentCandidateRevisionHash: &parentHash,
		OriginKind:                  "repair", RepairOrigin: origin,
		CandidateContentHash: strings.Repeat("d", 64),
	}
	first, err := material.Hash()
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(){
		func() { changed := strings.Repeat("e", 64); material.ParentCandidateRevisionHash = &changed },
		func() { material.RepairOrigin.RepairInvocationID = "20000000-0000-0000-0000-000000000003" },
		func() { material.RepairOrigin.RepairResultHash = strings.Repeat("f", 64) },
		func() { material.CandidateContentHash = strings.Repeat("9", 64) },
	}
	for index, mutate := range mutations {
		parentHash = strings.Repeat("b", 64)
		material.ParentCandidateRevisionHash = &parentHash
		material.RepairOrigin.RepairInvocationID = "20000000-0000-0000-0000-000000000002"
		material.RepairOrigin.RepairResultHash = strings.Repeat("c", 64)
		material.CandidateContentHash = strings.Repeat("d", 64)
		mutate()
		changed, hashErr := material.Hash()
		if hashErr != nil || changed == first {
			t.Fatalf("repair mutation %d kept revision hash: first=%s changed=%s err=%v", index, first, changed, hashErr)
		}
	}
}

func TestCandidateRevisionHashRejectsMixedOrIncompleteOrigins(t *testing.T) {
	parentHash := strings.Repeat("b", 64)
	validRepair := contract.CandidateRevisionMaterial{
		StageInstanceKey: strings.Repeat("a", 64), RevisionNo: 2,
		ParentCandidateRevisionHash: &parentHash,
		OriginKind:                  "repair", RepairOrigin: &contract.RepairCandidateOrigin{
			RepairInvocationID: "20000000-0000-0000-0000-000000000002",
			RepairResultHash:   strings.Repeat("c", 64),
		},
		CandidateContentHash: strings.Repeat("d", 64),
	}
	invalid := []contract.CandidateRevisionMaterial{
		func() contract.CandidateRevisionMaterial {
			value := validRepair
			value.ParentCandidateRevisionHash = nil
			return value
		}(),
		func() contract.CandidateRevisionMaterial {
			value := validRepair
			value.InvocationOrigin = &contract.InvocationCandidateOrigin{
				SourceInvocationID: "20000000-0000-0000-0000-000000000001",
				SourceResultHash:   strings.Repeat("e", 64),
			}
			return value
		}(),
		func() contract.CandidateRevisionMaterial {
			value := validRepair
			value.RevisionNo = 1
			return value
		}(),
	}
	for index, value := range invalid {
		if _, err := value.Hash(); err == nil {
			t.Fatalf("invalid candidate origin %d was accepted", index)
		}
	}
}
