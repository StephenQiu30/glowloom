package workflow_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestStructureIdentityGateInputFreezesExactSubjectAndEffectPlan(t *testing.T) {
	draft := structureIdentityGateInputDraft()
	value, encoded, err := workflow.NewStructureIdentityGateInput(draft)
	if err != nil {
		t.Fatal(err)
	}
	if value.SchemaVersion != workflow.StructureIdentityGateInputSchemaVersion ||
		value.GateKey != workflow.StructureIdentityGateKey || len(value.SubjectHash) != 64 ||
		len(value.EffectPlanHash) != 64 || len(value.InputHash) != 64 {
		t.Fatalf("invalid structure identity Gate input identity: %#v", value)
	}
	if value.GateInstanceKey != "structure_identity:"+draft.ProjectID+":"+draft.Subject.ReviewCandidate.CandidateRevisionHash {
		t.Fatalf("gate instance key = %q", value.GateInstanceKey)
	}
	if !slices.Equal(value.AllowedDecisions, []string{"approved", "changes_requested", "rejected"}) {
		t.Fatalf("allowed decisions = %#v", value.AllowedDecisions)
	}
	if len(value.EffectPlan.Steps) != 2 ||
		value.EffectPlan.Steps[0].StepKey != "confirm_project_episode_lifecycle" ||
		value.EffectPlan.Steps[0].OwnerKind != "production/project" ||
		len(value.EffectPlan.Steps[0].DependsOnStepKeys) != 0 ||
		value.EffectPlan.Steps[1].StepKey != "confirm_structure_identity_set" ||
		value.EffectPlan.Steps[1].OwnerKind != "production/bible" ||
		!slices.Equal(value.EffectPlan.Steps[1].DependsOnStepKeys, []string{"confirm_project_episode_lifecycle"}) {
		t.Fatalf("effect plan = %#v", value.EffectPlan)
	}
	decoded, canonical, err := workflow.DecodeStructureIdentityGateInput(encoded)
	if err != nil || decoded.InputHash != value.InputHash || string(canonical) != string(encoded) {
		t.Fatalf("decode Gate input: decoded=%#v err=%v", decoded, err)
	}
}

func TestStructureIdentityGateInputCanonicalizesEvidenceImpactAndDecisions(t *testing.T) {
	left := structureIdentityGateInputDraft()
	right := structureIdentityGateInputDraft()
	slices.Reverse(right.EvidenceRefs)
	slices.Reverse(right.Impact.AffectedScopeKeys)
	slices.Reverse(right.Impact.PreservedFamilies)
	slices.Reverse(right.Impact.InvalidatedFamilies)
	slices.Reverse(right.AllowedDecisions)

	leftValue, leftJSON, err := workflow.NewStructureIdentityGateInput(left)
	if err != nil {
		t.Fatal(err)
	}
	rightValue, rightJSON, err := workflow.NewStructureIdentityGateInput(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftValue.InputHash != rightValue.InputHash || string(leftJSON) != string(rightJSON) {
		t.Fatalf("canonical Gate input drifted: left=%s right=%s", leftJSON, rightJSON)
	}
	if !slices.IsSortedFunc(rightValue.EvidenceRefs, func(left, right workflow.HumanGateEvidenceRef) int {
		return left.SourceStart - right.SourceStart
	}) {
		t.Fatalf("evidence refs are not sorted: %#v", rightValue.EvidenceRefs)
	}
}

func TestStructureIdentityGateInputRejectsDriftAndUnknownFields(t *testing.T) {
	tests := map[string]func(*workflow.StructureIdentityGateInputDraft){
		"wrong source owner": func(value *workflow.StructureIdentityGateInputDraft) {
			value.Subject.SourceVersion.OwnerKind = "production/bible"
		},
		"wrong span stage": func(value *workflow.StructureIdentityGateInputDraft) {
			value.Subject.SpanCandidate.StageKey = "extract_scene_facts"
		},
		"wrong review stage": func(value *workflow.StructureIdentityGateInputDraft) {
			value.Subject.ReviewCandidate.StageKey = "resolve_identities"
		},
		"duplicate candidate": func(value *workflow.StructureIdentityGateInputDraft) {
			value.Subject.ReviewCandidate.CandidateRevisionID = value.Subject.IdentityCandidate.CandidateRevisionID
		},
		"cross source evidence": func(value *workflow.StructureIdentityGateInputDraft) {
			value.EvidenceRefs[0].SourceVersionID = "30000000-0000-0000-0000-000000000099"
		},
		"duplicate evidence": func(value *workflow.StructureIdentityGateInputDraft) {
			value.EvidenceRefs[1] = value.EvidenceRefs[0]
		},
		"free form decision": func(value *workflow.StructureIdentityGateInputDraft) {
			value.AllowedDecisions = append(value.AllowedDecisions, "approve_with_note")
		},
		"missing negative branch": func(value *workflow.StructureIdentityGateInputDraft) {
			value.AllowedDecisions = []string{"approved"}
		},
		"invalid expected head": func(value *workflow.StructureIdentityGateInputDraft) {
			value.ExpectedBibleHead.Revision = 2
			value.ExpectedBibleHead.ContentHash = ""
		},
		"free form impact scope": func(value *workflow.StructureIdentityGateInputDraft) {
			value.Impact.AffectedScopeKeys[0] = "whole project"
		},
		"unknown impact family": func(value *workflow.StructureIdentityGateInputDraft) {
			value.Impact.InvalidatedFamilies[0] = "everything"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			draft := structureIdentityGateInputDraft()
			mutate(&draft)
			if _, _, err := workflow.NewStructureIdentityGateInput(draft); err == nil {
				t.Fatal("invalid Gate input was accepted")
			}
		})
	}

	_, encoded, err := workflow.NewStructureIdentityGateInput(structureIdentityGateInputDraft())
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &root); err != nil {
		t.Fatal(err)
	}
	root["unexpected"] = json.RawMessage(`true`)
	withUnknown, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = workflow.DecodeStructureIdentityGateInput(withUnknown); err == nil {
		t.Fatal("unknown Gate input field was accepted")
	}

	drifted := append([]byte(nil), encoded...)
	drifted = []byte(strings.Replace(string(drifted), strings.Repeat("e", 64), strings.Repeat("a", 64), 1))
	if _, _, err = workflow.DecodeStructureIdentityGateInput(drifted); err == nil {
		t.Fatal("mutated Gate input hash was accepted")
	}
}

func structureIdentityGateInputDraft() workflow.StructureIdentityGateInputDraft {
	createdAt := time.Date(2026, time.September, 9, 8, 0, 0, 0, time.UTC)
	return workflow.StructureIdentityGateInputDraft{
		WorkspaceID:   "10000000-0000-0000-0000-000000000001",
		ProjectID:     "20000000-0000-0000-0000-000000000001",
		WorkflowRunID: "30000000-0000-0000-0000-000000000001",
		NodeRunID:     "40000000-0000-0000-0000-000000000001",
		Subject: workflow.StructureIdentityGateSubject{
			SourceVersion: agentcontract.ScriptSourceVersionIdentity{
				OwnerKind: "production/script", LogicalID: "50000000-0000-0000-0000-000000000001",
				VersionID: "60000000-0000-0000-0000-000000000001", Revision: 3,
				ContentHash: strings.Repeat("a", 64), CreatedAt: createdAt,
			},
			SpanCandidate: structureIdentityCandidateRef(
				"propose_script_spans", "70000000-0000-0000-0000-000000000001", "b",
			),
			SceneFactCandidate: structureIdentityCandidateRef(
				"extract_scene_facts", "70000000-0000-0000-0000-000000000002", "c",
			),
			IdentityCandidate: structureIdentityCandidateRef(
				"resolve_identities", "70000000-0000-0000-0000-000000000003", "d",
			),
			ReviewCandidate: structureIdentityCandidateRef(
				"review_candidate", "70000000-0000-0000-0000-000000000004", "e",
			),
		},
		EvidenceRefs: []workflow.HumanGateEvidenceRef{
			{SourceVersionID: "60000000-0000-0000-0000-000000000001", SourceStart: 12, SourceEnd: 18, TextHash: strings.Repeat("2", 64)},
			{SourceVersionID: "60000000-0000-0000-0000-000000000001", SourceStart: 0, SourceEnd: 8, TextHash: strings.Repeat("1", 64)},
		},
		Impact: workflow.HumanGateImpactSummary{
			AffectedScopeKeys: []string{
				"scene:90000000-0000-0000-0000-000000000002",
				"scene:90000000-0000-0000-0000-000000000001",
			},
			PreservedFamilies:   []string{"script_source", "scene_facts"},
			InvalidatedFamilies: []string{"storyboard", "production_world"},
		},
		AllowedDecisions: []string{"rejected", "approved", "changes_requested"},
		ExpectedProjectHead: workflow.HumanGateExpectedHead{
			OwnerKind: "production/project", LogicalID: "20000000-0000-0000-0000-000000000001",
			Revision: 4, ContentHash: strings.Repeat("f", 64),
		},
		ExpectedBibleHead: workflow.HumanGateExpectedHead{
			OwnerKind: "production/bible", LogicalID: "20000000-0000-0000-0000-000000000001",
			Revision: 0,
		},
	}
}

func structureIdentityCandidateRef(
	stageKey, candidateRevisionID, hashCharacter string,
) agentcontract.SceneAnalysisCandidateRevisionIdentity {
	return agentcontract.SceneAnalysisCandidateRevisionIdentity{
		StageKey: stageKey, ShardKey: "script:full", CandidateRevisionID: candidateRevisionID,
		CandidateRevisionHash: strings.Repeat(hashCharacter, 64),
		SourceInvocationID:    strings.Replace(candidateRevisionID, "70000000", "80000000", 1),
		SourceResultHash:      strings.Repeat(hashCharacter, 64),
	}
}
