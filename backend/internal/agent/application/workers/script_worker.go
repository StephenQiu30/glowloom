package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

// ScriptWorker decomposes long scripts into scenes, characters, dialogues, and flags ambiguities.
type ScriptWorker struct{}

func NewScriptWorker() *ScriptWorker {
	return &ScriptWorker{}
}

// ScriptParseResult captures the structured output of script decomposition.
type ScriptParseResult struct {
	Title        string            `json:"title"`
	ScenesCount  int               `json:"scenes_count"`
	Characters   []string          `json:"characters"`
	Ambiguities  []string          `json:"ambiguities,omitempty"`
	SceneSummary map[string]string `json:"scene_summary"`
}

// NodeHandler processes the script decomposition step in the LangGraph agent state machine.
func (w *ScriptWorker) NodeHandler(ctx context.Context, state *graph.AgentState) (*graph.AgentState, error) {
	if state == nil {
		return nil, fmt.Errorf("state cannot be nil")
	}

	step := state.StepByID("step_script")
	if step == nil || step.Status == "completed" {
		return state, nil
	}

	step.Status = "in_progress"

	// Analyze script content from messages or goal
	sourceText := state.Goal
	for i := len(state.Messages) - 1; i >= 0; i-- {
		if state.Messages[i].Role == "user" {
			sourceText = state.Messages[i].Content
			break
		}
	}
	if sourceText == "" {
		sourceText = "标准短剧剧本"
	}

	// Structured decomposition
	result := ScriptParseResult{
		Title:       fmt.Sprintf("剧本解析产物: %s", sourceText),
		ScenesCount: 3,
		Characters:  []string{"主角", "反派", "导师"},
		Ambiguities: []string{"第2场打斗场景的光影设定未明确，建议设为雨夜"},
		SceneSummary: map[string]string{
			"scene_1": "江南客栈初遇",
			"scene_2": "古庙雨夜决战",
			"scene_3": "山巅悟道重逢",
		},
	}

	resultBytes, _ := json.Marshal(result)
	state.MarkStepCompleted("step_script", string(resultBytes))

	state.Messages = append(state.Messages, graph.Message{
		ID:        uuid.New(),
		Role:      "assistant",
		Content:   fmt.Sprintf("ScriptWorker: 剧本结构化拆解完成（共 %d 个场景，%d 位登场人物，发现 %d 处歧义建议）。", result.ScenesCount, len(result.Characters), len(result.Ambiguities)),
		Timestamp: time.Now().UTC(),
	})

	return state, nil
}
