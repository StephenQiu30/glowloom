package agent_test

import (
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

func TestStudioDigestComputationAndDiffInjection(t *testing.T) {
	computer := application.NewStudioDigestComputer()

	pid := uuid.New()
	initialSnapshot := application.CanvasSnapshot{
		ProjectID: pid,
		Revision:  1,
		Elements: map[string]application.ElementInfo{
			"el_1": {
				ID:        "el_1",
				Type:      "character",
				Title:     "李逍遥",
				Content:   "主角，手持木剑",
				UpdatedAt: 100,
			},
			"el_2": {
				ID:        "el_2",
				Type:      "scene",
				Title:     "余杭客栈",
				Content:   "破旧但温馨的江南客栈",
				UpdatedAt: 100,
			},
		},
	}

	state := &graph.AgentState{
		RunID:     uuid.New(),
		ProjectID: pid,
	}

	// 1. Initial snapshot injection
	err := computer.InjectDigestDiff(state, initialSnapshot, nil)
	if err != nil {
		t.Fatalf("initial inject error: %v", err)
	}
	if state.LastStudioHash == "" {
		t.Fatal("expected non-empty LastStudioHash")
	}
	if len(state.Messages) != 1 {
		t.Fatalf("expected 1 system message for initial snapshot, got %d", len(state.Messages))
	}
	if !strings.Contains(state.Messages[0].Content, "<canvas_changes>") {
		t.Errorf("expected <canvas_changes> in message: %s", state.Messages[0].Content)
	}

	firstHash := state.LastStudioHash

	// 2. Re-running with identical snapshot should produce NO new diff
	err = computer.InjectDigestDiff(state, initialSnapshot, &initialSnapshot)
	if err != nil {
		t.Fatalf("second inject error: %v", err)
	}
	if state.LastStudioHash != firstHash {
		t.Errorf("hash should remain identical")
	}
	if len(state.Messages) != 1 {
		t.Errorf("no new message should be added when canvas is unchanged, got %d", len(state.Messages))
	}

	// 3. User modifies el_1, deletes el_2, and adds el_3
	updatedSnapshot := application.CanvasSnapshot{
		ProjectID: pid,
		Revision:  2,
		Elements: map[string]application.ElementInfo{
			"el_1": {
				ID:        "el_1",
				Type:      "character",
				Title:     "李逍遥",
				Content:   "主角，晋升为蜀山剑侠", // modified
				UpdatedAt: 200,
			},
			"el_3": {
				ID:        "el_3",
				Type:      "shot",
				Title:     "御剑飞行特写", // added
				Content:   "李逍遥御剑穿云而过",
				UpdatedAt: 200,
			},
		},
	}

	err = computer.InjectDigestDiff(state, updatedSnapshot, &initialSnapshot)
	if err != nil {
		t.Fatalf("third inject error: %v", err)
	}

	if state.LastStudioHash == firstHash {
		t.Fatal("hash should have changed after edits")
	}
	if len(state.Messages) != 2 {
		t.Fatalf("expected second diff message, got %d messages", len(state.Messages))
	}

	secondMsgContent := state.Messages[1].Content
	if !strings.Contains(secondMsgContent, "+ 新增元素 [shot] 御剑飞行特写") {
		t.Errorf("expected added element in diff: %s", secondMsgContent)
	}
	if !strings.Contains(secondMsgContent, "~ 更新元素 [character] 李逍遥") {
		t.Errorf("expected modified element in diff: %s", secondMsgContent)
	}
	if !strings.Contains(secondMsgContent, "- 删除元素 [scene] 余杭客栈") {
		t.Errorf("expected deleted element in diff: %s", secondMsgContent)
	}
}
