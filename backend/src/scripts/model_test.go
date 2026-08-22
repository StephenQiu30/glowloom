package scripts

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
		if asset.Name == "林夏" && !containsInt(asset.EpisodeNumbers, 1) || asset.Name == "林夏" && !containsInt(asset.EpisodeNumbers, 2) {
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

func TestAnalyzeScriptRejectsEmptyAndOversizedSource(t *testing.T) {
	if _, err := AnalyzeScript(" \n\t"); err == nil {
		t.Fatal("empty source should be rejected")
	}
	if _, err := AnalyzeScript(strings.Repeat("字", 2_000_001)); err == nil {
		t.Fatal("oversized source should be rejected")
	}
}

func TestApproveAnalysisMaterializesCanonicalWithGORM(t *testing.T) {
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
	workspaceID, projectID, revisionID, operationID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	type cleanupRecord struct{ ID uuid.UUID }
	deleteBy := func(table, condition string, args ...any) {
		if result := orm.WithContext(ctx).Table(table).Where(condition, args...).Delete(&cleanupRecord{}); result.Error != nil {
			t.Logf("cleanup %s: %v", table, result.Error)
		}
	}
	t.Cleanup(func() {
		var narrativeIDs, sceneIDs, beatIDs, orderRevisionIDs, orderUnitIDs, analysisRunIDs, breakdownIDs, contentUnitIDs, entityIDs, requirementItemIDs []uuid.UUID
		orm.Table("nar_narrative_revisions").Where("project_id = ?", projectID).Pluck("id", &narrativeIDs)
		orm.Table("nar_scenes").Where("narrative_revision_id IN ?", narrativeIDs).Pluck("id", &sceneIDs)
		orm.Table("nar_beats").Where("scene_id IN ?", sceneIDs).Pluck("id", &beatIDs)
		deleteBy("pk_mention_resolutions", "narrative_revision_id IN ?", narrativeIDs)
		deleteBy("nar_production_element_mentions", "narrative_revision_id IN ?", narrativeIDs)
		deleteBy("nar_beats", "id IN ?", beatIDs)
		deleteBy("nar_scenes", "id IN ?", sceneIDs)
		deleteBy("nar_narrative_revisions", "id IN ?", narrativeIDs)
		orm.Table("nar_analysis_runs").Where("project_id = ?", projectID).Pluck("id", &analysisRunIDs)
		orm.Table("nar_episode_breakdown_revisions").Where("analysis_run_id IN ?", analysisRunIDs).Pluck("id", &breakdownIDs)
		deleteBy("nar_episode_candidates", "breakdown_revision_id IN ?", breakdownIDs)
		deleteBy("nar_episode_breakdown_revisions", "id IN ?", breakdownIDs)
		deleteBy("nar_analysis_runs", "id IN ?", analysisRunIDs)
		orm.Table("prj_content_order_revisions").Where("project_id = ?", projectID).Pluck("id", &orderRevisionIDs)
		deleteBy("prj_content_order_items", "order_revision_id IN ?", orderRevisionIDs)
		deleteBy("prj_content_order_revisions", "id IN ?", orderRevisionIDs)
		orm.Table("prj_content_units").Where("project_id = ?", projectID).Pluck("id", &orderUnitIDs)
		contentUnitIDs = append(contentUnitIDs, orderUnitIDs...)
		deleteBy("sht_shots", "content_unit_id IN ?", contentUnitIDs)
		deleteBy("nar_source_revisions", "project_id = ?", projectID)
		deleteBy("prj_content_units", "project_id = ?", projectID)
		var projectionContentIDs []uuid.UUID
		orm.Table("content_units").Where("project_id = ?", projectID).Pluck("id", &projectionContentIDs)
		deleteBy("narrative_units", "content_unit_id IN ?", projectionContentIDs)
		deleteBy("content_units", "project_id = ?", projectID)
		orm.Table("pk_production_requirement_items").Where("project_id = ?", projectID).Pluck("id", &requirementItemIDs)
		deleteBy("pk_production_requirement_revisions", "item_id IN ?", requirementItemIDs)
		deleteBy("pk_production_requirement_items", "project_id = ?", projectID)
		orm.Table("pk_entities").Where("project_id = ?", projectID).Pluck("id", &entityIDs)
		deleteBy("pk_mention_resolutions", "entity_id IN ?", entityIDs)
		deleteBy("production_requirements", "project_id = ?", projectID)
		deleteBy("entities", "project_id = ?", projectID)
		deleteBy("pk_entities", "project_id = ?", projectID)
		deleteBy("nar_import_runs", "project_id = ?", projectID)
		deleteBy("script_analysis_drafts", "script_revision_id = ?", revisionID)
		deleteBy("outbox_events", "operation_id = ?", operationID)
		deleteBy("operations", "id = ?", operationID)
		deleteBy("script_revisions", "id = ?", revisionID)
		deleteBy("projects", "id = ?", projectID)
		deleteBy("workspaces", "id = ?", workspaceID)
		pool.Close()
	})
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
		{"script_revisions", &struct {
			ID            uuid.UUID
			ProjectID     uuid.UUID
			Name          string
			ObjectKey     string
			ContentHash   string
			ContentLength int
			Status        string
		}{revisionID, projectID, "integration.txt", "integration/" + revisionID.String() + ".txt", HashContent("source"), 6, "uploaded"}},
		{"operations", &operationRecord{ID: operationID, ProjectID: projectID, Type: "script_analysis", Status: "succeeded", Progress: 100, CreatedAt: time.Now().UTC()}},
	} {
		if err := orm.WithContext(ctx).Table(entry.table).Create(entry.record).Error; err != nil {
			t.Fatal(entry.table, err)
		}
	}
	content := "第1集 归途\n场景：海边码头\n人物：林夏\n林夏：我们走。\n"
	analysis, err := AnalyzeScript(content)
	if err != nil {
		t.Fatal(err)
	}
	if err := orm.WithContext(ctx).Create(&scriptAnalysisDraftRecord{ScriptRevisionID: revisionID, SourceHash: analysis.SourceHash, Analysis: datatypes.JSON(mustJSON(analysis)), Status: "draft"}).Error; err != nil {
		t.Fatal(err)
	}
	repository := NewScriptRepository(orm, nil)
	approved, err := repository.ApproveAnalysis(ctx, revisionID)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Episodes[0].ContentUnitID == uuid.Nil {
		t.Fatal("approval did not persist canonical content unit id")
	}
	var count int64
	if err := orm.Table("nar_scenes").Where("narrative_revision_id IN (?)", orm.Table("nar_narrative_revisions").Select("id").Where("project_id = ?", projectID)).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("canonical scene count = %d, want 1", count)
	}
	if _, err := repository.ApproveAnalysis(ctx, revisionID); err != nil {
		t.Fatal("repeated approval should be idempotent:", err)
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
