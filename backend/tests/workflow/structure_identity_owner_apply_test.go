package workflow_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestStructureIdentityOwnerAppliesProjectBeforeBible(t *testing.T) {
	gateDraft := structureIdentityGateInputDraft()
	gate, _, err := workflow.NewStructureIdentityGateInput(gateDraft)
	if err != nil {
		t.Fatal(err)
	}
	span := agentcontract.ScriptSpanCandidate{
		SourceVersionID: gate.Subject.SourceVersion.VersionID, SourceHash: gate.Subject.SourceVersion.ContentHash,
		CodepointCount: 8,
		Episodes: []agentcontract.ScriptEpisodeSpan{{
			TemporaryEpisodeID: "episode_001", Position: 1, CodepointStart: 0, CodepointEnd: 8,
			SceneSpanIDs: []string{"span_scene_001"},
		}},
		Spans: []agentcontract.ScriptSceneSpan{{
			TemporarySpanID: "span_scene_001", EpisodeSpanID: "episode_001", Kind: "scene",
			CodepointStart: 0, CodepointEnd: 8, Heading: "第一场",
			Evidence: agentcontract.SourceEvidenceSpan{SourceStart: 0, SourceEnd: 2, TextHash: strings.Repeat("1", 64), ExactAnchor: "第一"},
		}},
	}
	mention := agentcontract.IdentityMentionRef{
		Kind: "character", OccurrenceRole: "actual",
		TemporarySceneID: "scene_001", SourceStart: 2, SourceEnd: 4,
		TextHash: strings.Repeat("2", 64), ExactAnchor: "阿青",
	}
	facts := agentcontract.SceneFactCandidate{
		SourceVersionID: gate.Subject.SourceVersion.VersionID, SourceHash: gate.Subject.SourceVersion.ContentHash,
		Scenes: []agentcontract.SceneFact{{TemporarySceneID: "scene_001", SpanID: "span_scene_001", SourceStart: 0, SourceEnd: 8}},
	}
	identities := agentcontract.IdentityResolutionCandidate{
		SourceVersionID: gate.Subject.SourceVersion.VersionID, SourceHash: gate.Subject.SourceVersion.ContentHash,
		ResolvedClusters: []agentcontract.IdentityCluster{{
			TemporaryIdentityKey: "identity_character_aqing", Kind: "character", Resolution: "new",
			CanonicalName: "阿青", Aliases: []string{"阿青"}, MentionRefs: []agentcontract.IdentityMentionRef{mention},
		}},
		AmbiguousMentions: []agentcontract.AmbiguousIdentityMention{}, RejectedMentions: []agentcontract.RejectedIdentityMention{},
		Coverage: agentcontract.IdentityResolutionCoverage{MentionCount: 1, ResolvedCount: 1},
	}
	review := agentcontract.StructureIdentityReviewCandidate{
		ProfileKey: "structure_identity", SourceVersionID: gate.Subject.SourceVersion.VersionID,
		SourceHash: gate.Subject.SourceVersion.ContentHash, ReviewIssues: []agentcontract.CandidateReviewIssue{},
		Suggestions: []agentcontract.StructureIdentityReviewSuggestion{},
	}
	rawCandidates := map[string]any{
		"propose_script_spans": span, "extract_scene_facts": facts,
		"resolve_identities": identities, "review_candidate": review,
	}
	refs := []agentcontract.SceneAnalysisCandidateRevisionIdentity{
		gate.Subject.SpanCandidate, gate.Subject.SceneFactCandidate,
		gate.Subject.IdentityCandidate, gate.Subject.ReviewCandidate,
	}
	ownerCandidates := make([]workflow.StructureIdentityOwnerCandidate, len(refs))
	for index, ref := range refs {
		candidate, marshalErr := json.Marshal(rawCandidates[ref.StageKey])
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		ownerCandidates[index] = workflow.StructureIdentityOwnerCandidate{
			Identity: ref,
			Release: agentcontract.SceneAnalysisReleaseIdentity{
				SkillReleaseID: uuid.NewString(), SkillReleaseHash: strings.Repeat("3", 64),
				StageReleaseHash: strings.Repeat("4", 64), BundleContentHash: agentcontract.SceneAnalysisSkillBundleHash,
				AgentImageDigest: "sha256:" + strings.Repeat("5", 64),
			},
			Candidate: candidate,
		}
	}
	material, err := json.Marshal(workflow.StructureIdentityOwnerMaterial{
		SchemaVersion: workflow.StructureIdentityOwnerMaterialSchema, GateInputID: uuid.NewString(), GateInput: gate,
		SpanIndexID: uuid.NewString(), SpanIndexHash: strings.Repeat("6", 64), Candidates: ownerCandidates,
	})
	if err != nil {
		t.Fatal(err)
	}
	order := []string{}
	projectOwner := &structureIdentityProjectOwner{order: &order}
	bibleOwner := &structureIdentityBibleOwner{order: &order}
	applier := workflowproduction.New(nil, bibleOwner, projectOwner, nil, nil, nil, nil)
	result, err := applier.ApplyHumanGateDecision(context.Background(), workflowapp.Actor{
		UserID: uuid.NewString(), TokenVersion: 1,
	}, workflow.HumanGateOwnerApplication{
		WorkspaceID: gate.WorkspaceID, ProjectID: gate.ProjectID, WorkflowRunID: gate.WorkflowRunID, NodeRunID: gate.NodeRunID,
		HumanTaskID: uuid.NewString(), ReviewDecisionID: uuid.NewString(), SubjectRevision: 1,
		Decision: "approved", Executor: "gate.structure_identity_review",
		Candidate: workflow.NodeInputBinding{
			Port: "review", ValueType: "structure_identity_review_candidate", SourceKind: workflow.NodeInputSourceNodeOutput,
			ReferenceID: gate.Subject.ReviewCandidate.CandidateRevisionID, ReferenceVersion: "1",
			ContentHash: gate.Subject.ReviewCandidate.CandidateRevisionHash,
		},
		OutputPort: "identities", OutputValueType: "structure_identity_set_version", OwnerMaterial: material,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slicesEqualStrings(order, []string{"project", "bible"}) || result.Operation != bibledomain.StructureIdentityCommandOperation ||
		result.ReceiptID == "" || len(result.Output.Bindings) != 1 ||
		result.Output.Bindings[0].ValueType != "structure_identity_set_version" {
		t.Fatalf("order=%v result=%#v", order, result)
	}
	if projectOwner.command.GateInputID == "" || bibleOwner.command.ProjectEpisodeReceiptID != projectOwner.result.ID ||
		len(bibleOwner.command.SceneRefs) != 1 || len(bibleOwner.command.Identities) != 1 ||
		len(bibleOwner.command.MentionMappings) != 1 || bibleOwner.command.Coverage.ResolvedCount != 1 {
		t.Fatalf("project=%#v bible=%#v", projectOwner.command, bibleOwner.command)
	}
}

type structureIdentityProjectOwner struct {
	order   *[]string
	command projectapp.ConfirmEpisodeLifecycleCommand
	result  projectdomain.EpisodeLifecycleSet
}

func (owner *structureIdentityProjectOwner) ConfirmEpisodeLifecycle(
	_ context.Context,
	_ projectapp.Actor,
	command projectapp.ConfirmEpisodeLifecycleCommand,
) (projectdomain.EpisodeLifecycleSet, error) {
	*owner.order = append(*owner.order, "project")
	owner.command = command
	owner.result = projectdomain.EpisodeLifecycleSet{
		SchemaVersion: projectdomain.EpisodeLifecycleSetSchemaVersion, ID: uuid.NewString(),
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		GateInputID: command.GateInputID, GateInputHash: command.GateInputHash,
		ReviewDecisionID: command.ReviewDecisionID, SourceVersionID: command.SourceVersionID, SourceHash: command.SourceHash,
		Episodes: []projectdomain.EpisodeLifecycleEpisodeRef{{
			TemporaryEpisodeID: "episode_001", EpisodeID: uuid.NewString(), EpisodeRevision: 1,
			Position: 1, ScriptVersionID: uuid.NewString(), ScriptVersion: 1,
			SourceStart: 0, SourceEnd: 8, ContentHash: strings.Repeat("7", 64),
		}},
		CollectionRootHash: strings.Repeat("8", 64), CreatedAt: time.Now().UTC(),
	}
	return owner.result, nil
}

type structureIdentityBibleOwner struct {
	order   *[]string
	command bibleapp.ConfirmStructureIdentitySetCommand
}

func (owner *structureIdentityBibleOwner) ConfirmStructureIdentitySet(
	_ context.Context,
	actor bibleapp.Actor,
	command bibleapp.ConfirmStructureIdentitySetCommand,
) (bibledomain.ConfirmStructureIdentitySetResult, error) {
	*owner.order = append(*owner.order, "bible")
	owner.command = command
	versionID, commandReceiptID := uuid.NewString(), uuid.NewString()
	version := bibledomain.StructureIdentitySetVersion{
		SchemaVersion: bibledomain.StructureIdentitySetSchemaVersion, ID: versionID,
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, Version: 1,
		GateInputID: command.GateInputID, GateInputHash: command.GateInputHash,
		ReviewDecisionID: command.ReviewDecisionID, ProjectEpisodeReceiptID: command.ProjectEpisodeReceiptID,
		ContentHash: strings.Repeat("9", 64), CreatedBy: actor.UserID, CreatedAt: time.Now().UTC(),
	}
	collection, err := bibledomain.BuildStructureIdentityCollection(version)
	if err != nil {
		return bibledomain.ConfirmStructureIdentitySetResult{}, err
	}
	receipt, err := bibledomain.NewStructureIdentityCollectionReceipt(
		uuid.NewString(), uuid.NewString(), "workflow-structure-identity", command.ReviewDecisionID,
		collection, []string{"scene:owner-apply"}, time.Now().UTC(), actor.UserID,
	)
	if err != nil {
		return bibledomain.ConfirmStructureIdentitySetResult{}, err
	}
	return bibledomain.ConfirmStructureIdentitySetResult{
		Version:          version,
		Receipt:          receipt,
		CommandReceiptID: commandReceiptID, CommandOperation: bibledomain.StructureIdentityCommandOperation,
	}, nil
}

func slicesEqualStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
