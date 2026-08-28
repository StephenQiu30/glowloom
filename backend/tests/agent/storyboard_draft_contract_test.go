package agent_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	storyboard "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

func TestDraftStoryboardInvocationRejectsEmptyFormalInput(t *testing.T) {
	graphID := uuid.NewString()
	_, err := contract.NewStageInvocation(uuid.NewString(), contract.StoryGraphDefinition().ExecutionPolicy(), contract.StageInvocationPayload{
		Stage: "draft_storyboard", ShardKey: "scene:scene-1",
		WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(),
		SourceRefs: []contract.StageSourceRef{{
			OwnerKind: "production/storygraph", OwnerLogicalID: uuid.NewString(), OwnerVersionID: graphID,
			Revision: 1, ContentHash: strings.Repeat("a", 64),
		}},
		BaseStoryGraphVersionID: graphID, BaseStoryGraphHash: strings.Repeat("a", 64),
		UpstreamCandidates: []contract.StageUpstreamCandidateRef{},
		ShardManifestRef: contract.ShardManifestRef{
			ManifestID: uuid.NewString(), Version: 1, Hash: strings.Repeat("b", 64),
		},
		Shard:      contract.InvocationShard{Kind: "story_scene", Key: "scene:scene-1", TreePath: "scene/0001"},
		StageInput: json.RawMessage(`{}`),
	})
	if err == nil {
		t.Fatal("empty Storyboard Draft formal input must be rejected")
	}
}

func TestDraftStoryboardCandidateAcceptsStrictNeedsAssetIntent(t *testing.T) {
	sceneNodeKey := "sgn_" + strings.Repeat("1", 64)
	beatNodeKey := "sgn_" + strings.Repeat("2", 64)
	occurrenceNodeKey := "sgn_" + strings.Repeat("3", 64)
	identityNodeKey := "sgn_" + strings.Repeat("4", 64)
	specificationNodeKey := "sgn_" + strings.Repeat("5", 64)
	stateNodeKey := "sgn_" + strings.Repeat("6", 64)
	styleHash := strings.Repeat("7", 64)
	evidenceHash := strings.Repeat("8", 64)
	documentRevisionID := uuid.NewString()
	ownerVersionID := uuid.NewString()
	assetID := uuid.NewString()
	specificationID := uuid.NewString()
	stateID := uuid.NewString()

	payload := json.RawMessage(`{
		"graph_version_no":1,
		"scene":{"story_node_key":"` + sceneNodeKey + `","owner_version_id":"` + ownerVersionID + `","owner_revision":1,"owner_hash":"` + strings.Repeat("9", 64) + `","episode_id":"` + uuid.NewString() + `","episode_position":1,"scene_position":1,"heading":"内景 客厅 日","evidence":[{"document_revision_id":"` + documentRevisionID + `","absolute_start":10,"absolute_end":14,"text_hash":"` + evidenceHash + `"}]},
		"beats":[{"story_node_key":"` + beatNodeKey + `","summary":"人物进入","required_for_coverage":true,"evidence":[{"document_revision_id":"` + documentRevisionID + `","absolute_start":10,"absolute_end":14,"text_hash":"` + evidenceHash + `"}]}],
		"dialogues":[],
		"occurrences":[{"story_node_key":"` + occurrenceNodeKey + `","identity_story_node_key":"` + identityNodeKey + `","specification_story_node_key":"` + specificationNodeKey + `","asset_state_story_node_key":"` + stateNodeKey + `","asset_id":"` + assetID + `","specification_version_id":"` + specificationID + `","asset_state_id":"` + stateID + `","asset_kind":"character","summary":"人物出现","evidence":[{"document_revision_id":"` + documentRevisionID + `","absolute_start":10,"absolute_end":14,"text_hash":"` + evidenceHash + `"}]}],
		"effective_style_snapshot":{"owner_version_id":"` + uuid.NewString() + `","revision":1,"content_hash":"` + styleHash + `","visual_style":"cinematic","aspect_ratio":"9:16"},
		"target_duration_ms":90000,
		"asset_versions":[]
	}`)
	candidate := json.RawMessage(`{
		"scene_story_node_key":"` + sceneNodeKey + `",
		"shot_intents":[{
			"shot_key":"shot-001","intent_order":1,
			"source_beat_story_node_keys":["` + beatNodeKey + `"],
			"source_evidence":[{"document_revision_id":"` + documentRevisionID + `","absolute_start":10,"absolute_end":14,"text_hash":"` + evidenceHash + `"}],
			"purpose":"建立人物动作","proposed_duration_ms":2500,
			"camera":{"scale":"medium","angle":"eye_level","movement":"static","composition":"centered"},
			"action_intent":"人物进入画面","dialogue_intent":null,"sound_intent":"环境声","performance_intent":"克制",
			"continuity_in":"承接入场","continuity_out":"保持视线方向",
			"frame_intent":{"first":"空镜","key":"人物入画","last":"人物停步"},
			"visual_requirements":[{"occurrence_story_node_key":"` + occurrenceNodeKey + `","identity_story_node_key":"` + identityNodeKey + `","specification_story_node_key":"` + specificationNodeKey + `","asset_state_story_node_key":"` + stateNodeKey + `","asset_id":"` + assetID + `","specification_version_id":"` + specificationID + `","asset_state_id":"` + stateID + `","asset_role":"subject","required_view_roles":["front","profile","back"],"asset_readiness":"needs_asset","asset_version_ref":null}],
			"risk_codes":["reference_asset_missing"],"review_issues":[]
		}],
		"asset_readiness":"needs_asset"
	}`)
	if _, err := storyboard.DecodeAndValidateCandidate(candidate, payload); err != nil {
		t.Fatalf("strict needs_asset Storyboard intent must be valid: %v", err)
	}
}
