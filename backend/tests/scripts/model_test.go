package scripts_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
	. "github.com/stephenqiu30/lanverse/backend/src/scripts"
)

func TestAnalyzeScriptBuildsEpisodesScenesAndAssetEpisodeMatrix(t *testing.T) {
	content := "第1集 归途\n场景：海边码头\n人物：林夏、顾远\n道具：旧怀表\n服装：雨衣\n林夏：我们必须马上离开。\n\n第2集 回声\n场景：旧仓库\n人物：林夏\n道具：旧怀表\n林夏：怀表还在这里。\n"

	analysis, err := AnalyzeScript(content)
	if err != nil {
		t.Fatalf("AnalyzeScript() error = %v", err)
	}
	if len(analysis.Episodes) != 2 {
		t.Fatalf("episodes = %d, want 2", len(analysis.Episodes))
	}
	if analysis.Episodes[0].Number != 1 || analysis.Episodes[1].Number != 2 {
		t.Fatalf("episode numbers = %#v, want [1 2]", []int{analysis.Episodes[0].Number, analysis.Episodes[1].Number})
	}
	if len(analysis.Episodes[0].Scenes) != 1 || len(analysis.Episodes[0].Scenes[0].Narratives) != 1 {
		t.Fatalf("episode 1 scene/narrative shape is incomplete: %#v", analysis.Episodes[0])
	}
	if got := analysis.Episodes[0].Scenes[0].Narratives[0].Speaker; got != "林夏" {
		t.Fatalf("dialogue speaker = %q, want 林夏", got)
	}
	for _, asset := range analysis.Characters {
		if asset.Name == "林夏" && !containsEpisode(asset.EpisodeNumbers, 1) || asset.Name == "林夏" && !containsEpisode(asset.EpisodeNumbers, 2) {
			t.Fatalf("林夏 episode matrix = %#v, want [1 2]", asset.EpisodeNumbers)
		}
	}
	var foundProp bool
	for _, asset := range analysis.Props {
		if asset.Name == "旧怀表" {
			foundProp = true
			if len(asset.Evidence) != 2 || len(asset.EpisodeNumbers) != 2 {
				t.Fatalf("旧怀表 evidence/episode matrix = %#v/%#v, want two entries", asset.Evidence, asset.EpisodeNumbers)
			}
		}
	}
	if !foundProp {
		t.Fatal("旧怀表 was not extracted")
	}
}

func containsEpisode(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestAnalyzeScriptRejectsEmptyAndOversizedSource(t *testing.T) {
	if _, err := AnalyzeScript(" \n\t"); err == nil {
		t.Fatal("empty source should be rejected")
	}
	if _, err := AnalyzeScript(strings.Repeat("字", 2_000_001)); err == nil {
		t.Fatal("oversized source should be rejected")
	}
}

func TestAnalyzeScriptKeepsConflictingEpisodeHeadingsAsReviewableCandidates(t *testing.T) {
	content := "第1集 归途\n场景：码头\n林夏：出发。\n\n第1集 回声\n场景：仓库\n顾远：等等。\n"

	analysis, err := AnalyzeScript(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Episodes) != 2 {
		t.Fatalf("episodes = %d, want 2 reviewable candidates", len(analysis.Episodes))
	}
	if analysis.Episodes[0].TemporaryKey == analysis.Episodes[1].TemporaryKey || analysis.Episodes[0].TemporaryKey == "" {
		t.Fatalf("candidate keys are not stable and unique: %#v", analysis.Episodes)
	}
	if analysis.Episodes[0].Anchor.StartOffset != 0 || analysis.Episodes[0].Anchor.EndOffset != analysis.Episodes[1].Anchor.StartOffset || analysis.Episodes[1].Anchor.EndOffset != len(content) {
		t.Fatalf("episode source ranges do not conserve coverage: %#v", analysis.Episodes)
	}
	if analysis.Breakdown.Status != BreakdownStatusBlocked || len(analysis.Breakdown.Issues) == 0 || analysis.Breakdown.Issues[0].Code != "duplicate_episode_number" {
		t.Fatalf("breakdown validation = %#v, want duplicate blocker", analysis.Breakdown)
	}
}

func TestReviseEpisodeBreakdownCreatesNewRevisionAndPreservesSourceCoverage(t *testing.T) {
	content := "第1集 合集\n场景：码头\n林夏：出发。\n场景：仓库\n顾远：等等。\n"
	analysis, err := AnalyzeScript(content)
	if err != nil {
		t.Fatal(err)
	}
	boundary := analysis.Episodes[0].Scenes[1].Anchor.StartOffset
	originalKey := analysis.Episodes[0].TemporaryKey

	revised, err := ReviseEpisodeBreakdown(analysis, analysis.SourceHash, []EpisodeBreakdownOperation{{
		Type: BreakdownOperationSplit, CandidateKey: originalKey, BoundaryOffset: boundary,
		LeftKey: "episode-left", LeftTitle: "归途", RightKey: "episode-right", RightTitle: "回声",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Episodes) != 1 {
		t.Fatal("revision mutated the original draft")
	}
	if revised.Breakdown.RevisionNo != analysis.Breakdown.RevisionNo+1 || len(revised.Episodes) != 2 {
		t.Fatalf("revised breakdown = %#v", revised.Breakdown)
	}
	if revised.Episodes[0].Anchor.StartOffset != 0 || revised.Episodes[0].Anchor.EndOffset != boundary || revised.Episodes[1].Anchor.StartOffset != boundary || revised.Episodes[1].Anchor.EndOffset != len(content) {
		t.Fatalf("revised ranges = %#v", revised.Episodes)
	}
	if revised.Breakdown.Status != BreakdownStatusReady || revised.Breakdown.CoverageHash == "" || revised.Breakdown.SegmentationHash == "" {
		t.Fatalf("revised validation = %#v", revised.Breakdown)
	}
}

func TestReviseEpisodeBreakdownRejectsBoundaryInsideScene(t *testing.T) {
	analysis, err := AnalyzeScript("第1集 合集\n场景：码头\n林夏：出发。\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = ReviseEpisodeBreakdown(analysis, analysis.SourceHash, []EpisodeBreakdownOperation{{
		Type: BreakdownOperationSplit, CandidateKey: analysis.Episodes[0].TemporaryKey,
		BoundaryOffset: analysis.Episodes[0].Scenes[0].Anchor.StartOffset + 1,
		LeftKey:        "episode-left", LeftTitle: "归途", RightKey: "episode-right", RightTitle: "回声",
	}})
	if err == nil {
		t.Fatal("split inside a scene must be rejected")
	}
}

func TestReviseEpisodeBreakdownKeepsNamedIgnoredRangeWithoutPublishingEpisode(t *testing.T) {
	analysis, err := AnalyzeScript("片头说明\n场景：说明页\n这段不属于正片。\n\n第1集 归途\n场景：码头\n林夏：出发。\n")
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Breakdown.Status != BreakdownStatusBlocked {
		t.Fatalf("initial breakdown = %#v, want blocked unlabeled range", analysis.Breakdown)
	}

	revised, err := ReviseEpisodeBreakdown(analysis, analysis.SourceHash, []EpisodeBreakdownOperation{{
		Type: BreakdownOperationIgnore, CandidateKey: analysis.Episodes[0].TemporaryKey, Title: "片头说明，不属于正片",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if revised.Breakdown.Status != BreakdownStatusReady {
		t.Fatalf("revised breakdown = %#v, want ready with named ignore", revised.Breakdown)
	}
	if revised.Episodes[0].Decision != "ignored" || revised.Episodes[0].Title != "片头说明，不属于正片" {
		t.Fatalf("ignored candidate = %#v", revised.Episodes[0])
	}
	if revised.Episodes[0].Anchor.StartOffset != 0 || revised.Episodes[0].Anchor.EndOffset != revised.Episodes[1].Anchor.StartOffset {
		t.Fatalf("ignored range no longer conserves source coverage: %#v", revised.Episodes)
	}
}

func TestPrepareNarrativeDraftBuildsTypedNodesAndStableMentions(t *testing.T) {
	analysis, err := AnalyzeScript("第1集 归途\n场景：码头\n人物：林夏\n林夏：我们走。\n海风吹动雨衣。\n")
	if err != nil {
		t.Fatal(err)
	}
	analysis = PrepareNarrativeDraft(analysis, uuid.MustParse("11111111-1111-4111-8111-111111111111"), 1)

	if analysis.Narrative.Status != NarrativeStatusReady || analysis.Narrative.ContentHash == "" {
		t.Fatalf("narrative validation = %#v, want ready with content hash", analysis.Narrative)
	}
	nodes := analysis.Episodes[0].Scenes[0].Narratives
	if len(nodes) != 2 || nodes[0].Kind != NarrativeNodeDialogue || nodes[1].Kind != NarrativeNodeAction {
		t.Fatalf("typed narrative nodes = %#v", nodes)
	}
	if _, err := uuid.Parse(nodes[0].ID); err != nil {
		t.Fatalf("narrative node id is not stable UUID: %q", nodes[0].ID)
	}
	if len(analysis.Mentions) < 2 {
		t.Fatalf("mentions = %#v, want character and location/source mentions", analysis.Mentions)
	}
	for _, mention := range analysis.Mentions {
		if _, err := uuid.Parse(mention.ID); err != nil || mention.SceneID == "" || mention.Anchor.EndOffset <= mention.Anchor.StartOffset {
			t.Fatalf("mention is not stably anchored: %#v", mention)
		}
	}
}

func TestReviseNarrativeCreatesImmutableRevisionAndSupportsManualCommands(t *testing.T) {
	analysis, err := AnalyzeScript("第1集 归途\n场景：码头\n林夏：我们走。\n海风吹动雨衣。\n")
	if err != nil {
		t.Fatal(err)
	}
	analysis = PrepareNarrativeDraft(analysis, uuid.MustParse("11111111-1111-4111-8111-111111111111"), 1)
	scene := analysis.Episodes[0].Scenes[0]
	node := scene.Narratives[1]
	mentionID := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	revised, err := ReviseNarrative(analysis, analysis.Narrative.ContentHash, uuid.MustParse("33333333-3333-4333-8333-333333333333"), []NarrativeOperation{
		{Type: NarrativeOperationUpdateScene, SceneID: scene.ID, Heading: "海边码头"},
		{Type: NarrativeOperationUpdateNode, NodeID: node.ID, NodeKind: NarrativeNodeNarration, Text: "海风掠过空旷码头。", Anchor: node.Anchor},
		{Type: NarrativeOperationCreateMention, MentionID: mentionID.String(), SceneID: scene.ID, ElementType: "costume", SurfaceText: "雨衣", Anchor: node.Anchor},
	})
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Episodes[0].Scenes[0].Heading != "码头" || revised.Episodes[0].Scenes[0].Heading != "海边码头" {
		t.Fatal("narrative revision mutated its immutable base")
	}
	if revised.Narrative.RevisionNo != 2 || revised.Narrative.ID.String() != "33333333-3333-4333-8333-333333333333" || revised.Narrative.ContentHash == analysis.Narrative.ContentHash {
		t.Fatalf("narrative revision metadata = %#v", revised.Narrative)
	}
	if revised.Episodes[0].Scenes[0].Narratives[1].Kind != NarrativeNodeNarration {
		t.Fatalf("node was not reclassified: %#v", revised.Episodes[0].Scenes[0].Narratives[1])
	}
	if got := findMention(revised.Mentions, mentionID.String()); got == nil || got.ElementType != "costume" || got.SurfaceText != "雨衣" {
		t.Fatalf("created mention = %#v", got)
	}
}

func TestReviseNarrativeSupportsSceneSplitMergeReorderAndNamedIgnore(t *testing.T) {
	analysis, err := AnalyzeScript("第1集 合集\n场景：码头\n林夏：出发。\n海风渐强。\n场景：仓库\n顾远：等等。\n")
	if err != nil {
		t.Fatal(err)
	}
	analysis = PrepareNarrativeDraft(analysis, uuid.New(), 1)
	originalScene := analysis.Episodes[0].Scenes[0]
	leftID, rightID := uuid.New(), uuid.New()

	split, err := ReviseNarrative(analysis, analysis.Narrative.ContentHash, uuid.New(), []NarrativeOperation{{
		Type: NarrativeOperationSplitScene, SceneID: originalScene.ID, BoundaryNodeID: originalScene.Narratives[1].ID,
		LeftSceneID: leftID.String(), LeftHeading: "码头外", RightSceneID: rightID.String(), RightHeading: "码头内",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(split.Episodes[0].Scenes) != 3 || split.Episodes[0].Scenes[0].ID != leftID.String() || split.Episodes[0].Scenes[1].ID != rightID.String() {
		t.Fatalf("split scenes = %#v", split.Episodes[0].Scenes)
	}

	ignored, err := ReviseNarrative(split, split.Narrative.ContentHash, uuid.New(), []NarrativeOperation{{
		Type: NarrativeOperationIgnoreNode, NodeID: split.Episodes[0].Scenes[1].Narratives[0].ID, IgnoreReason: "环境说明不进入叙事",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if ignored.Episodes[0].Scenes[1].Narratives[0].Status != NarrativeNodeStatusIgnored {
		t.Fatalf("named ignore was not preserved: %#v", ignored.Episodes[0].Scenes[1].Narratives[0])
	}

	mergedID := uuid.New()
	merged, err := ReviseNarrative(ignored, ignored.Narrative.ContentHash, uuid.New(), []NarrativeOperation{{
		Type: NarrativeOperationMergeScenes, SceneIDs: []string{leftID.String(), rightID.String()}, TargetSceneID: mergedID.String(), Heading: "码头",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Episodes[0].Scenes) != 2 || merged.Episodes[0].Scenes[0].ID != mergedID.String() {
		t.Fatalf("merged scenes = %#v", merged.Episodes[0].Scenes)
	}

	reordered, err := ReviseNarrative(merged, merged.Narrative.ContentHash, uuid.New(), []NarrativeOperation{{
		Type: NarrativeOperationReorderScenes, EpisodeKey: merged.Episodes[0].TemporaryKey,
		OrderedSceneIDs: []string{merged.Episodes[0].Scenes[1].ID, merged.Episodes[0].Scenes[0].ID},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if reordered.Episodes[0].Scenes[0].ID != merged.Episodes[0].Scenes[1].ID {
		t.Fatalf("scene reorder failed: %#v", reordered.Episodes[0].Scenes)
	}
}

func TestReviseNarrativeCreatesDeletesNodesAndUpdatesDeletesMentions(t *testing.T) {
	analysis, err := AnalyzeScript("第1集 归途\n场景：码头\n人物：林夏\n林夏：我们走。\n")
	if err != nil {
		t.Fatal(err)
	}
	analysis = PrepareNarrativeDraft(analysis, uuid.New(), 1)
	scene := analysis.Episodes[0].Scenes[0]
	nodeID := uuid.New().String()
	mention := analysis.Mentions[0]

	created, err := ReviseNarrative(analysis, analysis.Narrative.ContentHash, uuid.New(), []NarrativeOperation{
		{Type: NarrativeOperationCreateNode, NodeID: nodeID, SceneID: scene.ID, NodeKind: NarrativeNodeBeat, Text: "林夏决定离开", Anchor: Anchor{Line: 4, StartOffset: 31, EndOffset: 32}},
		{Type: NarrativeOperationUpdateMention, MentionID: mention.ID, SceneID: scene.ID, ElementType: "character", SurfaceText: "林夏（成年）", Anchor: mention.Anchor},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, node := findNodeForTest(created, nodeID); node == nil || node.Kind != NarrativeNodeBeat {
		t.Fatalf("created node = %#v", node)
	}
	if updated := findMention(created.Mentions, mention.ID); updated == nil || updated.SurfaceText != "林夏（成年）" {
		t.Fatalf("updated mention = %#v", updated)
	}

	deleted, err := ReviseNarrative(created, created.Narrative.ContentHash, uuid.New(), []NarrativeOperation{
		{Type: NarrativeOperationDeleteNode, NodeID: nodeID},
		{Type: NarrativeOperationDeleteMention, MentionID: mention.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, node := findNodeForTest(deleted, nodeID); node != nil || findMention(deleted.Mentions, mention.ID) != nil {
		t.Fatalf("deleted members remain in draft: node=%#v mention=%#v", node, findMention(deleted.Mentions, mention.ID))
	}
}

func TestNarrativeValidationBlocksUnknownSpeakerPartialAndStaleEdits(t *testing.T) {
	analysis, err := AnalyzeScript("第1集 归途\n场景：码头\n旁白响起。\n")
	if err != nil {
		t.Fatal(err)
	}
	analysis.Episodes[0].Scenes[0].Narratives[0].Kind = NarrativeNodeDialogue
	analysis.Episodes[0].Scenes[0].Narratives[0].Speaker = ""
	analysis.ParseReport.FailedScopes = []string{"episode:1/page:2"}
	analysis = PrepareNarrativeDraft(analysis, uuid.New(), 1)

	if analysis.Narrative.Status != NarrativeStatusBlocked || !hasNarrativeIssue(analysis.Narrative.Issues, "unknown_speaker") || !hasNarrativeIssue(analysis.Narrative.Issues, "partial_source_scope") {
		t.Fatalf("narrative blockers = %#v", analysis.Narrative)
	}
	if _, err := ReviseNarrative(analysis, "stale-hash", uuid.New(), []NarrativeOperation{{Type: NarrativeOperationUpdateScene, SceneID: analysis.Episodes[0].Scenes[0].ID, Heading: "新标题"}}); err == nil {
		t.Fatal("stale narrative edit must be rejected")
	}
}

func findMention(mentions []ProductionElementMention, id string) *ProductionElementMention {
	for index := range mentions {
		if mentions[index].ID == id {
			return &mentions[index]
		}
	}
	return nil
}

func hasNarrativeIssue(issues []NarrativeIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func findNodeForTest(analysis Analysis, id string) (*Scene, *NarrativeUnit) {
	for episodeIndex := range analysis.Episodes {
		for sceneIndex := range analysis.Episodes[episodeIndex].Scenes {
			scene := &analysis.Episodes[episodeIndex].Scenes[sceneIndex]
			for nodeIndex := range scene.Narratives {
				if scene.Narratives[nodeIndex].ID == id {
					return scene, &scene.Narratives[nodeIndex]
				}
			}
		}
	}
	return nil, nil
}

func TestApproveBreakdownThenReviseAndApproveNarrativeWithGORM(t *testing.T) {
	if os.Getenv("LANVERSE_INTEGRATION") != "1" {
		t.Skip("set LANVERSE_INTEGRATION=1 to run PostgreSQL/GORM integration")
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	orm, err := database.OpenGORM(pool)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, projectID, revisionID, operationID, artifactID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	type cleanupRecord struct{ ID uuid.UUID }
	deleteBy := func(table, condition string, args ...any) {
		if result := orm.WithContext(ctx).Table(table).Where(condition, args...).Delete(&cleanupRecord{}); result.Error != nil {
			t.Logf("cleanup %s: %v", table, result.Error)
		}
	}
	t.Cleanup(func() {
		var narrativeIDs, sceneIDs, nodeIDs, orderRevisionIDs, orderUnitIDs, analysisRunIDs, breakdownIDs, contentUnitIDs, entityIDs, requirementItemIDs []uuid.UUID
		orm.Table("nar_narrative_revisions").Where("project_id = ?", projectID).Pluck("id", &narrativeIDs)
		orm.Table("nar_scenes").Where("narrative_revision_id IN ?", narrativeIDs).Pluck("id", &sceneIDs)
		orm.Table("nar_narrative_nodes").Where("scene_id IN ?", sceneIDs).Pluck("id", &nodeIDs)
		deleteBy("pk_mention_resolutions", "narrative_revision_id IN ?", narrativeIDs)
		deleteBy("nar_production_element_mentions", "narrative_revision_id IN ?", narrativeIDs)
		deleteBy("nar_narrative_nodes", "id IN ?", nodeIDs)
		deleteBy("nar_scenes", "id IN ?", sceneIDs)
		deleteBy("nar_analysis_drafts", "source_revision_id = ?", revisionID)
		deleteBy("nar_narrative_revisions", "id IN ?", narrativeIDs)
		orm.Table("nar_analysis_runs").Where("project_id = ?", projectID).Pluck("id", &analysisRunIDs)
		orm.Table("nar_episode_breakdown_revisions").Where("analysis_run_id IN ?", analysisRunIDs).Pluck("id", &breakdownIDs)
		deleteBy("nar_episode_breakdown_manifests", "breakdown_revision_id IN ?", breakdownIDs)
		deleteBy("nar_episode_candidates", "breakdown_revision_id IN ?", breakdownIDs)
		deleteBy("nar_episode_breakdown_revisions", "id IN ?", breakdownIDs)
		deleteBy("nar_analysis_runs", "id IN ?", analysisRunIDs)
		orm.Table("prj_content_order_revisions").Where("project_id = ?", projectID).Pluck("id", &orderRevisionIDs)
		deleteBy("prj_content_order_items", "order_revision_id IN ?", orderRevisionIDs)
		deleteBy("prj_content_order_revisions", "id IN ?", orderRevisionIDs)
		orm.Table("prj_content_units").Where("project_id = ?", projectID).Pluck("id", &orderUnitIDs)
		contentUnitIDs = append(contentUnitIDs, orderUnitIDs...)
		deleteBy("sht_shots", "content_unit_id IN ?", contentUnitIDs)
		deleteBy("prj_content_units", "project_id = ?", projectID)
		orm.Table("pk_production_requirement_items").Where("project_id = ?", projectID).Pluck("id", &requirementItemIDs)
		deleteBy("pk_production_requirement_revisions", "item_id IN ?", requirementItemIDs)
		deleteBy("pk_production_requirement_items", "project_id = ?", projectID)
		orm.Table("pk_entities").Where("project_id = ?", projectID).Pluck("id", &entityIDs)
		deleteBy("pk_mention_resolutions", "entity_id IN ?", entityIDs)
		deleteBy("pk_entities", "project_id = ?", projectID)
		deleteBy("nar_import_runs", "project_id = ?", projectID)
		deleteBy("nar_source_revisions", "id = ?", revisionID)
		deleteBy("media_artifact_locations", "artifact_id = ?", artifactID)
		deleteBy("media_artifacts", "id = ?", artifactID)
		deleteBy("outbox_events", "operation_id = ?", operationID)
		deleteBy("operations", "id = ?", operationID)
		deleteBy("projects", "id = ?", projectID)
		deleteBy("workspaces", "id = ?", workspaceID)
		pool.Close()
	})
	sourceHash := HashContent("source")
	for _, entry := range []struct {
		table  string
		record any
	}{
		{"workspaces", &struct {
			ID   uuid.UUID
			Name string
		}{workspaceID, "gorm integration workspace"}},
		{"projects", &struct {
			ID          uuid.UUID
			WorkspaceID uuid.UUID
			Name        string
		}{projectID, workspaceID, "gorm integration project"}},
		{"media_artifacts", map[string]any{"id": artifactID, "workspace_id": workspaceID, "project_id": projectID, "content_hash": sourceHash, "size_bytes": 6, "media_type": "text/plain", "purpose": "source", "retention_class": "standard", "status": "ready"}},
		{"media_artifact_locations", map[string]any{"id": uuid.New(), "artifact_id": artifactID, "storage_profile": "test", "bucket": "test", "object_key": uuid.NewString(), "object_version_id": uuid.NewString(), "size_bytes": 6, "content_hash": sourceHash, "status": "active"}},
		{"nar_source_revisions", map[string]any{"id": revisionID, "project_id": projectID, "artifact_id": artifactID, "name": "integration.txt", "source_type": "txt", "status": "waiting_user"}},
		{"operations", map[string]any{"id": operationID, "project_id": projectID, "type": "script_analysis", "status": "waiting_user", "progress": 35, "created_at": time.Now().UTC()}},
	} {
		if err := orm.WithContext(ctx).Table(entry.table).Create(entry.record).Error; err != nil {
			t.Fatal(entry.table, err)
		}
	}
	content := "片头说明\n场景：说明页\n这段不属于正片。\n\n第1集 归途\n场景：海边码头\n人物：林夏\n林夏：我们走。\n"
	analysis, err := AnalyzeScript(content)
	if err != nil {
		t.Fatal(err)
	}
	analysisRunID, breakdownID := uuid.New(), uuid.New()
	if err := orm.WithContext(ctx).Table("nar_analysis_runs").Create(map[string]any{
		"id": analysisRunID, "project_id": projectID, "source_revision_id": revisionID, "root_operation_id": operationID,
		"source_manifest_hash": analysis.SourceHash, "current_stage": "breakdown", "current_stage_generation": 1,
		"current_gate": "breakdown_review", "status": "waiting_user", "input_hash": analysis.SourceHash,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.WithContext(ctx).Table("nar_episode_breakdown_revisions").Create(map[string]any{
		"id": breakdownID, "analysis_run_id": analysisRunID, "revision_no": analysis.Breakdown.RevisionNo,
		"status": "draft", "segmentation_hash": analysis.Breakdown.SegmentationHash, "coverage_hash": analysis.Breakdown.CoverageHash,
	}).Error; err != nil {
		t.Fatal(err)
	}
	for _, episode := range analysis.Episodes {
		if err := orm.WithContext(ctx).Table("nar_episode_candidates").Create(map[string]any{
			"id": uuid.New(), "breakdown_revision_id": breakdownID, "temporary_key": episode.TemporaryKey,
			"ordinal": episode.Ordinal, "title": episode.Title, "rule_code": episode.BoundaryRule,
			"confidence": 1, "decision": episode.Decision, "start_offset": episode.Anchor.StartOffset, "end_offset": episode.Anchor.EndOffset,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := orm.WithContext(ctx).Table("nar_analysis_drafts").Create(map[string]any{
		"source_revision_id": revisionID, "breakdown_revision_id": breakdownID, "source_hash": analysis.SourceHash,
		"analysis": datatypes.JSON(mustJSON(analysis)), "status": "breakdown_draft",
	}).Error; err != nil {
		t.Fatal(err)
	}
	repository := NewScriptRepository(orm, nil)
	tenantContext := database.WithWorkspaceID(ctx, workspaceID)
	revised, err := repository.ReviseAnalysisDraft(tenantContext, revisionID, analysis.SourceHash, []EpisodeBreakdownOperation{
		{Type: BreakdownOperationIgnore, CandidateKey: analysis.Episodes[0].TemporaryKey, Title: "片头说明，不属于正片"},
		{Type: BreakdownOperationRename, CandidateKey: analysis.Episodes[1].TemporaryKey, Title: "归途·修订"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if revised.Breakdown.RevisionNo != 2 || revised.Episodes[0].Decision != "ignored" || revised.Episodes[1].Title != "归途·修订" {
		t.Fatalf("persisted breakdown revision = %#v", revised)
	}
	var supersededCount, draftCount int64
	if err := orm.Table("nar_episode_breakdown_revisions").Where("analysis_run_id = ? AND status = ?", analysisRunID, "superseded").Count(&supersededCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("nar_episode_breakdown_revisions").Where("analysis_run_id = ? AND status = ?", analysisRunID, "draft").Count(&draftCount).Error; err != nil {
		t.Fatal(err)
	}
	if supersededCount != 1 || draftCount != 1 {
		t.Fatalf("breakdown revision states superseded/draft = %d/%d, want 1/1", supersededCount, draftCount)
	}

	narrativeDraft, err := repository.ApproveEpisodeBreakdown(tenantContext, revisionID)
	if err != nil {
		t.Fatal(err)
	}
	if narrativeDraft.Episodes[0].ContentUnitID != uuid.Nil || narrativeDraft.Episodes[1].ContentUnitID == uuid.Nil {
		t.Fatal("breakdown approval did not persist canonical content unit id")
	}
	if narrativeDraft.Narrative.ID == uuid.Nil || narrativeDraft.Narrative.RevisionNo != 1 || narrativeDraft.Narrative.Status != NarrativeStatusReady {
		t.Fatalf("narrative draft = %#v", narrativeDraft.Narrative)
	}
	var contentUnitCount int64
	if err := orm.Table("prj_content_units").Where("project_id = ?", projectID).Count(&contentUnitCount).Error; err != nil {
		t.Fatal(err)
	}
	if contentUnitCount != 1 {
		t.Fatalf("canonical content unit count = %d, want 1 non-ignored episode", contentUnitCount)
	}
	var count int64
	if err := orm.Table("nar_scenes").Where("narrative_revision_id IN (?)", orm.Table("nar_narrative_revisions").Select("id").Where("project_id = ?", projectID)).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unapproved narrative materialized %d canonical scenes", count)
	}
	activeScene := narrativeDraft.Episodes[1].Scenes[0]
	revisedNarrative, err := repository.ReviseNarrativeDraft(tenantContext, revisionID, narrativeDraft.Narrative.ContentHash, []NarrativeOperation{{
		Type: NarrativeOperationUpdateScene, SceneID: activeScene.ID, Heading: "海边码头·人工校对",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if revisedNarrative.Narrative.RevisionNo != 2 || revisedNarrative.Episodes[1].Scenes[0].Heading != "海边码头·人工校对" {
		t.Fatalf("revised narrative = %#v", revisedNarrative.Narrative)
	}
	approved, err := repository.ApproveNarrative(tenantContext, revisionID, revisedNarrative.Narrative.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Narrative.Status != NarrativeStatusApproved {
		t.Fatalf("approved narrative = %#v", approved.Narrative)
	}
	if err := orm.Table("nar_scenes").Where("narrative_revision_id = ?", approved.Narrative.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("canonical scene count = %d, want 1", count)
	}
	var mentionCount, entityCount, requirementCount int64
	if err := orm.Table("nar_production_element_mentions").Where("narrative_revision_id IN (?)", orm.Table("nar_narrative_revisions").Select("id").Where("project_id = ?", projectID)).Count(&mentionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("pk_entities").Where("project_id = ?", projectID).Count(&entityCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("pk_production_requirement_items").Where("project_id = ?", projectID).Count(&requirementCount).Error; err != nil {
		t.Fatal(err)
	}
	if mentionCount == 0 || entityCount != 0 || requirementCount != 0 {
		t.Fatalf("approval published mentions/entities/requirements = %d/%d/%d, want >0/0/0", mentionCount, entityCount, requirementCount)
	}
	if _, err := repository.ApproveEpisodeBreakdown(tenantContext, revisionID); err != nil {
		t.Fatal("repeated breakdown approval should be idempotent:", err)
	}
	if _, err := repository.ApproveNarrative(tenantContext, revisionID, revisedNarrative.Narrative.ContentHash); err != nil {
		t.Fatal("repeated narrative approval should be idempotent:", err)
	}
	if err := orm.Table("nar_scenes").Where("narrative_revision_id IN (?)", orm.Table("nar_narrative_revisions").Select("id").Where("project_id = ?", projectID)).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("repeated approval changed canonical scene count to %d", count)
	}
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}
