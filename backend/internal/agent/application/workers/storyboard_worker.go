package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

// StoryboardWorker plans cinematic shots, camera angles, and visual pacing based on script output.
type StoryboardWorker struct{}

func NewStoryboardWorker() *StoryboardWorker {
	return &StoryboardWorker{}
}

// ShotDesign describes a cinematic shot planned by the storyboard worker.
type ShotDesign struct {
	ShotID          string `json:"shot_id"`
	CameraAngle     string `json:"camera_angle"`  // "特写", "中景", "全景", "俯瞰"
	CameraMotion    string `json:"camera_motion"` // "推镜头", "固定机位", "环绕摇移"
	DurationSeconds int    `json:"duration_seconds"`
	VisualAction    string `json:"visual_action"`
}

// NodeHandler executes storyboard layout and registers planned shots.
func (w *StoryboardWorker) NodeHandler(ctx context.Context, state *graph.AgentState) (*graph.AgentState, error) {
	if state == nil {
		return nil, fmt.Errorf("state cannot be nil")
	}

	step := state.StepByID("step_storyboard")
	if step == nil || step.Status == "completed" {
		return state, nil
	}

	step.Status = "in_progress"

	// Plan storyboard shots
	shots := []ShotDesign{
		{
			ShotID:          "shot_01",
			CameraAngle:     "特写",
			CameraMotion:    "缓慢推进",
			DurationSeconds: 4,
			VisualAction:    "主角拔剑出鞘，剑刃映照雨夜闪电寒光",
		},
		{
			ShotID:          "shot_02",
			CameraAngle:     "全景",
			CameraMotion:    "低机位仰拍摇镜头",
			DurationSeconds: 5,
			VisualAction:    "古庙屋檐下暴雨倾盆，二人对峙拔剑相向",
		},
	}

	shotsBytes, _ := json.Marshal(shots)
	state.MarkStepCompleted("step_storyboard", string(shotsBytes))

	state.Messages = append(state.Messages, graph.Message{
		ID:        uuid.New(),
		Role:      "assistant",
		Content:   fmt.Sprintf("StoryboardWorker: 分镜设计完成，共排布 %d 个镜头机位与运动轨迹。", len(shots)),
		Timestamp: time.Now().UTC(),
	})

	return state, nil
}
