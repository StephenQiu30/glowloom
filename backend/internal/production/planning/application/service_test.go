package application

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

func TestExplicitProposalsPreserveEverySourceRune(t *testing.T) {
	text := "第一集\n《开始》\n内景·房间·日\n小兰：走吧\n\n第二集\n《结束》\n外景·街道·夜\n小兰离开。"
	firstEnd := len([]rune("第一集"))
	secondStart := len([]rune("第一集\n《开始》\n内景·房间·日\n小兰：走吧\n\n"))
	blocks := []domain.Block{
		{ID: "a", Position: 1, Kind: "episode_marker", SourceStart: 0, SourceEnd: firstEnd},
		{ID: "b", Position: 2, Kind: "action", SourceStart: firstEnd + 1, SourceEnd: firstEnd + 5},
		{ID: "c", Position: 3, Kind: "scene_heading", SourceStart: firstEnd + 6, SourceEnd: firstEnd + 13},
		{ID: "d", Position: 4, Kind: "dialogue", SourceStart: firstEnd + 14, SourceEnd: secondStart - 2},
		{ID: "e", Position: 5, Kind: "separator", SourceStart: secondStart - 1, SourceEnd: secondStart - 1},
		{ID: "f", Position: 6, Kind: "episode_marker", SourceStart: secondStart, SourceEnd: secondStart + firstEnd},
		{ID: "g", Position: 7, Kind: "action", SourceStart: secondStart + firstEnd + 1, SourceEnd: secondStart + firstEnd + 5},
		{ID: "h", Position: 8, Kind: "action", SourceStart: secondStart + firstEnd + 6, SourceEnd: len([]rune(text))},
	}
	service := &Service{config: Config{NewID: sequenceIDs()}}
	proposals, err := service.explicitProposals(domain.Source{NormalizedText: text, Blocks: blocks}, 90_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 2 || proposals[0].Title != "开始" || proposals[1].Title != "结束" {
		t.Fatalf("unexpected proposals: %#v", proposals)
	}
	if proposals[0].SourceStart != 0 || proposals[0].SourceEnd != proposals[1].SourceStart || proposals[1].SourceEnd != len([]rune(text)) {
		t.Fatalf("source is not conserved: %#v", proposals)
	}
	hash := sha256.Sum256([]byte(string([]rune(text)[proposals[0].SourceStart:proposals[0].SourceEnd])))
	if proposals[0].ContentHash != hex.EncodeToString(hash[:]) {
		t.Fatal("proposal hash does not bind its exact source range")
	}
}

func TestExtractScenesAlwaysProvidesRequiredShotBreakdown(t *testing.T) {
	service := &Service{config: Config{NewID: sequenceIDs()}}
	scenes := service.extractScenes("第一集\n《开始》\n内景·房间·日\n小兰：走吧\n她打开门。\n外景·街道·夜\n小兰离开。")
	if len(scenes) != 2 {
		t.Fatalf("scene count = %d", len(scenes))
	}
	for _, scene := range scenes {
		if len(scene.Tasks) != 1 || scene.Tasks[0].Kind != "shot_breakdown" || !scene.Tasks[0].Required {
			t.Fatalf("invalid scene tasks: %#v", scene.Tasks)
		}
	}
	if len(scenes[0].Dialogues) != 1 || scenes[0].Dialogues[0].Speaker != "小兰" {
		t.Fatalf("dialogue not extracted: %#v", scenes[0].Dialogues)
	}
}

func sequenceIDs() func() string {
	index := 0
	return func() string { index++; return fmt.Sprintf("00000000-0000-0000-0000-%012d", index) }
}
