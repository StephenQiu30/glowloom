package reference_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformowner "github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	referencedomain "github.com/StephenQiu30/lanverse/backend/internal/production/reference/domain"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestReferenceBriefCompilerFreezesApprovedFactsAndDependencySelection(t *testing.T) {
	plan := referenceBriefApprovedPlan(t)
	appearance := referenceBriefTargetByKind(t, plan, "character_appearance")
	anchor := referenceBriefTargetByKind(t, plan, "character_identity_anchor")
	facts := referencedomain.ReferenceBriefCompilationFacts{
		ApprovedPlan: plan.Version,
		Target:       appearance,
		VisualFoundationVersionRef: referenceBriefPresetRef(
			appearance.WorkspaceID, appearance.ProjectID, appearance.Revision, "visual-foundation", "c",
		),
		DependencyFacts: []referencedomain.ReferenceBriefDependencyFact{{
			Target: anchor,
			SelectedAssetVersionRef: platformowner.VersionRef{
				WorkspaceID: anchor.WorkspaceID, ProjectID: anchor.ProjectID,
				OwnerKind: "asset", VersionFamily: "asset_base_reference_set",
				OwnerLogicalID: "asset-version:character-anchor", OwnerVersionID: uuid.NewString(),
				OwnerRevision: 1, OwnerContentHash: strings.Repeat("d", 64),
			},
		}},
		StageRelease: agentcontract.ReferenceBriefStageRelease{
			StageKey: "compile_reference_brief", StageReleaseHash: strings.Repeat("e", 64),
		},
	}

	compiled, err := referencedomain.CompileReferenceBriefInput(facts)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := referencedomain.CompileReferenceBriefInput(facts)
	if err != nil || !reflect.DeepEqual(replayed, compiled) {
		t.Fatalf("Reference Brief compilation is not deterministic: got=%#v want=%#v err=%v", replayed, compiled, err)
	}
	if compiled.ApprovedReferencePlanVersionRef.OwnerVersionID != plan.Version.ID ||
		compiled.ReferencePlanTargetRef.OwnerVersionID != appearance.ID ||
		compiled.TargetBusinessKey != appearance.TargetBusinessKey || compiled.TargetKind != appearance.TargetKind ||
		compiled.TargetFulfillment != appearance.Fulfillment ||
		compiled.VisualFoundationVersionRef.OwnerVersionID != facts.VisualFoundationVersionRef.OwnerVersionID ||
		len(compiled.DependencySelections) != 1 ||
		compiled.DependencySelections[0].TargetVersionRef.OwnerVersionID != anchor.ID ||
		compiled.DependencySelections[0].SelectedAssetVersionRef.OwnerVersionID != facts.DependencyFacts[0].SelectedAssetVersionRef.OwnerVersionID ||
		compiled.TypedReadSetRoot == "" || !reflect.DeepEqual(compiled.SourceRefs, appearance.OwnerRefs) ||
		!reflect.DeepEqual(compiled.DesignFocus, appearance.Constraints.DesignFocus) ||
		!reflect.DeepEqual(compiled.ForbiddenChanges, appearance.Constraints.ForbiddenChanges) {
		t.Fatalf("Reference Brief Input omitted an exact approved fact: %#v", compiled)
	}
	changedSelection := facts
	changedSelection.DependencyFacts = append(
		[]referencedomain.ReferenceBriefDependencyFact(nil), facts.DependencyFacts...,
	)
	changedSelection.DependencyFacts[0].SelectedAssetVersionRef.OwnerVersionID = uuid.NewString()
	changed, err := referencedomain.CompileReferenceBriefInput(changedSelection)
	if err != nil {
		t.Fatal(err)
	}
	if changed.TypedReadSetRoot == compiled.TypedReadSetRoot {
		t.Fatal("Reference Brief read set did not bind the selected Asset Version")
	}
	baseFacts := facts
	baseFacts.Target = anchor
	baseFacts.DependencyFacts = nil
	baseCompiled, err := referencedomain.CompileReferenceBriefInput(baseFacts)
	if err != nil {
		t.Fatal(err)
	}
	if baseCompiled.DependencySelections == nil || len(baseCompiled.DependencySelections) != 0 {
		t.Fatalf("base Reference Brief must freeze an explicit empty dependency set: %#v", baseCompiled)
	}
}

func TestReferenceBriefCompilerRejectsPlanTargetAndDependencyDrift(t *testing.T) {
	plan := referenceBriefApprovedPlan(t)
	appearance := referenceBriefTargetByKind(t, plan, "character_appearance")
	anchor := referenceBriefTargetByKind(t, plan, "character_identity_anchor")
	base := referencedomain.ReferenceBriefCompilationFacts{
		ApprovedPlan: plan.Version, Target: appearance,
		VisualFoundationVersionRef: referenceBriefPresetRef(
			appearance.WorkspaceID, appearance.ProjectID, appearance.Revision, "visual-foundation", "c",
		),
		DependencyFacts: []referencedomain.ReferenceBriefDependencyFact{{
			Target: anchor,
			SelectedAssetVersionRef: platformowner.VersionRef{
				WorkspaceID: anchor.WorkspaceID, ProjectID: anchor.ProjectID,
				OwnerKind: "asset", VersionFamily: "asset_base_reference_set",
				OwnerLogicalID: "asset-version:character-anchor", OwnerVersionID: uuid.NewString(),
				OwnerRevision: 1, OwnerContentHash: strings.Repeat("d", 64),
			},
		}},
		StageRelease: agentcontract.ReferenceBriefStageRelease{
			StageKey: "compile_reference_brief", StageReleaseHash: strings.Repeat("e", 64),
		},
	}

	missingDependency := base
	missingDependency.DependencyFacts = nil
	if _, err := referencedomain.CompileReferenceBriefInput(missingDependency); err == nil {
		t.Fatal("Reference Brief compiler accepted a missing selected dependency")
	}

	driftedTarget := base
	driftedTarget.Target = appearance
	driftedTarget.Target.ContentHash = strings.Repeat("f", 64)
	if _, err := referencedomain.CompileReferenceBriefInput(driftedTarget); err == nil {
		t.Fatal("Reference Brief compiler accepted a Target outside the approved Plan content")
	}

	driftedDependency := base
	driftedDependency.DependencyFacts = append([]referencedomain.ReferenceBriefDependencyFact(nil), base.DependencyFacts...)
	driftedDependency.DependencyFacts[0].Target = anchor
	driftedDependency.DependencyFacts[0].Target.PlanVersionID = uuid.NewString()
	if _, err := referencedomain.CompileReferenceBriefInput(driftedDependency); err == nil {
		t.Fatal("Reference Brief compiler accepted a dependency Target from another Plan")
	}
}

func referenceBriefApprovedPlan(t *testing.T) referencedomain.ApprovedReferencePlanSet {
	t.Helper()
	workspaceID, projectID := uuid.NewString(), uuid.NewString()
	worldHash, seedHash, candidateHash := strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)
	scopeKeys := []string{"scene:opening"}
	anchorKey := referenceBriefBusinessKey(t, "character_identity_anchor", "identity:hero")
	appearanceKey := referenceBriefBusinessKey(
		t, "character_appearance", "identity:hero", "specification:hero", "state:hero:coat",
	)
	targetKeys := []string{anchorKey, appearanceKey}
	sort.Strings(targetKeys)
	expected, err := storygraphdomain.BuildExpectedReferenceTargetSetProof(worldHash, scopeKeys, targetKeys)
	if err != nil {
		t.Fatal(err)
	}
	style := referenceBriefPresetRef(workspaceID, projectID, 1, "style", "a")
	policy := referenceBriefPresetRef(workspaceID, projectID, 1, "policy", "b")
	constraints := referencedomain.TargetConstraints{
		ProductionWorldOwnerSetHash: worldHash, ReferenceTargetSeedRoot: seedHash,
		VisualFoundationCandidateRevisionID:   uuid.NewString(),
		VisualFoundationCandidateRevisionHash: candidateHash,
		PresetReleaseContentHash:              strings.Repeat("4", 64),
		DesignFocus:                           []string{"preserve silhouette"}, ForbiddenChanges: []string{"change identity"},
	}
	ownerRefs := referenceBriefSourceRefs(workspaceID, projectID)
	createdAt := time.Date(2026, time.September, 12, 8, 0, 0, 0, time.UTC)
	plan, err := referencedomain.BuildApprovedReferencePlan(referencedomain.ApprovedReferencePlanDraft{
		PlanLogicalID: uuid.NewString(), PlanVersionID: uuid.NewString(), Revision: 1,
		WorkspaceID: workspaceID, ProjectID: projectID, CandidateRevisionID: uuid.NewString(),
		CandidateRevision: 1, CandidateRevisionHash: strings.Repeat("5", 64),
		CandidateContentHash:                  strings.Repeat("6", 64),
		VisualFoundationCandidateRevisionID:   constraints.VisualFoundationCandidateRevisionID,
		VisualFoundationCandidateRevisionHash: constraints.VisualFoundationCandidateRevisionHash,
		PresetReleaseContentHash:              constraints.PresetReleaseContentHash,
		ProductionWorldOwnerSetHash:           worldHash, ReferenceTargetSeedRoot: seedHash,
		ExpectedTargetSet: expected, EffectiveStyleSnapshot: style, EffectivePolicySnapshot: policy,
		Targets: []referencedomain.TargetDraft{
			{
				VersionID: uuid.NewString(), TargetBusinessKey: anchorKey,
				TargetKind: "character_identity_anchor", Fulfillment: "required",
				OwnerRefs: ownerRefs, CoverageScopeKeys: scopeKeys,
				DependsOnTargetBusinessKeys: []string{}, Constraints: constraints,
			},
			{
				VersionID: uuid.NewString(), TargetBusinessKey: appearanceKey,
				TargetKind: "character_appearance", Fulfillment: "required",
				OwnerRefs: ownerRefs, CoverageScopeKeys: scopeKeys,
				DependsOnTargetBusinessKeys: []string{anchorKey}, Constraints: constraints,
			},
		},
		ReviewDecisionID: uuid.NewString(), CreatedBy: uuid.NewString(), CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func referenceBriefTargetByKind(
	t *testing.T,
	plan referencedomain.ApprovedReferencePlanSet,
	targetKind string,
) referencedomain.ReferencePlanTargetVersion {
	t.Helper()
	for _, target := range plan.Targets {
		if target.TargetKind == targetKind {
			return target
		}
	}
	t.Fatalf("Reference Plan has no %s Target", targetKind)
	return referencedomain.ReferencePlanTargetVersion{}
}

func referenceBriefPresetRef(
	workspaceID, projectID string,
	revision int64,
	logicalSuffix, hashCharacter string,
) platformowner.VersionRef {
	return platformowner.VersionRef{
		WorkspaceID: workspaceID, ProjectID: projectID, OwnerKind: "preset",
		VersionFamily: "preset_effective_set", OwnerLogicalID: projectID,
		OwnerVersionID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(logicalSuffix)).String(),
		OwnerRevision:  revision, OwnerContentHash: strings.Repeat(hashCharacter, 64),
	}
}

func referenceBriefSourceRefs(workspaceID, projectID string) agentcontract.ReferencePlanTargetOwnerRefs {
	ref := func(ownerKind, family, logicalID, hashCharacter string) agentcontract.ReferencePlanOwnerRef {
		return agentcontract.ReferencePlanOwnerRef{
			WorkspaceID: workspaceID, ProjectID: projectID, OwnerKind: ownerKind,
			VersionFamily: family, OwnerLogicalID: logicalID,
			OwnerVersionID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(logicalID)).String(),
			OwnerRevision:  1, OwnerContentHash: strings.Repeat(hashCharacter, 64),
		}
	}
	return agentcontract.ReferencePlanTargetOwnerRefs{
		Identity:      []agentcontract.ReferencePlanOwnerRef{ref("asset", "asset_identity_set", "identity:hero", "7")},
		Specification: []agentcontract.ReferencePlanOwnerRef{ref("production/bible", "production_bible_set", "specification:hero", "8")},
		State:         []agentcontract.ReferencePlanOwnerRef{ref("asset", "asset_state_set", "state:hero:coat", "9")},
		Scene:         []agentcontract.ReferencePlanOwnerRef{ref("production/planning", "production_planning_set", "scene:opening", "a")},
		Occurrence:    []agentcontract.ReferencePlanOwnerRef{ref("production/planning", "production_planning_set", "occurrence:hero:opening", "b")},
		Interaction:   []agentcontract.ReferencePlanOwnerRef{},
	}
}

func referenceBriefBusinessKey(t *testing.T, kind string, logicalIDs ...string) string {
	t.Helper()
	parts := []any{kind}
	for _, logicalID := range logicalIDs {
		parts = append(parts, []string{"production", "owner_set", logicalID, ""})
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return string(canonical)
}
