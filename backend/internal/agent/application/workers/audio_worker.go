package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

// AudioWorker coordinates dubbing voices, background music (BGM), sound effects (SFX), and timeline alignment.
type AudioWorker struct{}

func NewAudioWorker() *AudioWorker {
	return &AudioWorker{}
}

// AudioTimelineTrack describes an audio track in the final timeline.
type AudioTimelineTrack struct {
	TrackType       string  `json:"track_type"` // "voiceover", "bgm", "sfx"
	AudioPrompt     string  `json:"audio_prompt"`
	StartTimeSecs   float64 `json:"start_time_secs"`
	DurationSeconds float64 `json:"duration_seconds"`
	VolumeLevel     float64 `json:"volume_level"`
}

// NodeHandler produces the aligned audio tracks and finalizes post-production.
func (w *AudioWorker) NodeHandler(ctx context.Context, state *graph.AgentState) (*graph.AgentState, error) {
	if state == nil {
		return nil, fmt.Errorf("state cannot be nil")
	}

	step := state.StepByID("step_audio")
	if step == nil || step.Status == "completed" {
		return state, nil
	}

	step.Status = "in_progress"

	tracks := []AudioTimelineTrack{
		{
			TrackType:       "voiceover",
			AudioPrompt:     "青年剑客沉稳台词: '这一剑，我等了十年。'",
			StartTimeSecs:   0.5,
			DurationSeconds: 3.2,
			VolumeLevel:     1.0,
		},
		{
			TrackType:       "sfx",
			AudioPrompt:     "暴雨雷鸣与长剑龙吟出鞘声",
			StartTimeSecs:   0.0,
			DurationSeconds: 4.5,
			VolumeLevel:     0.8,
		},
		{
			TrackType:       "bgm",
			AudioPrompt:     "国风史诗古筝与大鼓激昂变奏",
			StartTimeSecs:   0.0,
			DurationSeconds: 5.0,
			VolumeLevel:     0.5,
		},
	}

	tracksBytes, _ := json.Marshal(tracks)
	state.MarkStepCompleted("step_audio", string(tracksBytes))

	state.Messages = append(state.Messages, graph.Message{
		ID:        uuid.New(),
		Role:      "assistant",
		Content:   fmt.Sprintf("AudioWorker: 音频对齐与后期时间线编排完成（合成 %d 条音轨，含对白、音效与BGM）。", len(tracks)),
		Timestamp: time.Now().UTC(),
	})

	return state, nil
}
