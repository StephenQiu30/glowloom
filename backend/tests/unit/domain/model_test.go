package domain_test

import (
	"strings"
	"testing"

	"github.com/stephenqiu30/lanverse/backend/internal/modules/scripts/domain"
)

func TestAnalyzeScriptBuildsEpisodesScenesAndAssets(t *testing.T) {
	script := strings.Join([]string{
		"第1集 归途",
		"场景：海边码头",
		"人物：林夏、顾远",
		"道具：旧怀表",
		"服装：雨衣",
		"林夏：我们必须马上离开。",
		"顾远：怀表还在这里。",
		"第2集 回声",
		"场景：旧仓库",
		"林夏：你换了雨衣。",
	}, "\n")

	analysis, err := domain.AnalyzeScript(script)
	if err != nil {
		t.Fatalf("AnalyzeScript() error = %v", err)
	}
	if len(analysis.Episodes) != 2 {
		t.Fatalf("episodes = %d, want 2", len(analysis.Episodes))
	}
	if analysis.Episodes[0].Title != "归途" || analysis.Episodes[1].Title != "回声" {
		t.Fatalf("unexpected episode titles: %#v", analysis.Episodes)
	}
	if len(analysis.Characters) != 2 {
		t.Fatalf("characters = %#v", analysis.Characters)
	}
	if len(analysis.Locations) != 2 || len(analysis.Props) != 1 || len(analysis.Costumes) != 1 {
		t.Fatalf("assets = locations:%d props:%d costumes:%d", len(analysis.Locations), len(analysis.Props), len(analysis.Costumes))
	}
	if len(analysis.Episodes[0].Scenes[0].Narratives) < 2 {
		t.Fatalf("narratives = %#v", analysis.Episodes[0].Scenes[0].Narratives)
	}
	if analysis.SourceHash == "" {
		t.Fatal("source hash is empty")
	}
}

func TestAnalyzeScriptRejectsEmptyContent(t *testing.T) {
	if _, err := domain.AnalyzeScript("  \n"); err == nil {
		t.Fatal("AnalyzeScript() accepted empty content")
	}
}
