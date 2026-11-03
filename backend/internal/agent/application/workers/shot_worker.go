package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

// ShotWorker generates video prompts, camera parameters, and submits generation proposals (Proposal Yield).
type ShotWorker struct{}

func NewShotWorker() *ShotWorker {
	return &ShotWorker{}
}

// ShotProposalData contains the payload for video generation approval.
type ShotProposalData struct {
	ShotID       string `json:"shot_id"`
	Prompt       string `json:"prompt"`
	Negative     string `json:"negative"`
	ModelKey     string `json:"model_key"`
	DurationSecs int    `json:"duration_secs"`
}

// NodeHandler produces prompts and yields a generation proposal if not yet approved.
func (w *ShotWorker) NodeHandler(ctx context.Context, state *graph.AgentState) (*graph.AgentState, error) {
	if state == nil {
		return nil, fmt.Errorf("state cannot be nil")
	}

	step := state.StepByID("step_shot")
	if step == nil || step.Status == "completed" {
		return state, nil
	}

	step.Status = "in_progress"

	// Check if we already have an approved proposal for this step
	var proposal *graph.Proposal
	for i := range state.Proposals {
		if state.Proposals[i].Action == "shot.video.generate" {
			proposal = &state.Proposals[i]
			break
		}
	}

	// 1. If no proposal exists, create one and yield interrupt for human approval (Proposal Yield)
	if proposal == nil {
		proposalID := uuid.New()
		data := ShotProposalData{
			ShotID:       "shot_01",
			Prompt:       "雨夜古庙，江南剑客拔剑特写，闪电寒光倒影，电影级质感，8K超清",
			Negative:     "模糊，失真，多余手指，低画质",
			ModelKey:     "doubao-video-pro",
			DurationSecs: 5,
		}
		rawBytes, _ := json.Marshal(data)

		state.AddProposal(graph.Proposal{
			ID:                  proposalID,
			Action:              "shot.video.generate",
			Summary:             "生成第1镜头高品质视频（5秒）",
			EstimatedCostMicros: 4500000, // 4.5 CNY
			Data:                rawBytes,
		})

		state.Messages = append(state.Messages, graph.Message{
			ID:        uuid.New(),
			Role:      "assistant",
			Content:   "ShotWorker: 已生成视频生成参数提案，等待创作者审核批准以开始渲染。",
			Timestamp: time.Now().UTC(),
		})

		return state, nil
	}

	// 2. If proposal was approved, complete this step
	if proposal.Approved {
		state.MarkStepCompleted("step_shot", fmt.Sprintf("已成功根据批准参数生成视频素材: %s", proposal.Summary))
		state.Messages = append(state.Messages, graph.Message{
			ID:        uuid.New(),
			Role:      "assistant",
			Content:   "ShotWorker: 视频镜头已根据批准参数生成完成，移交后期剪辑与音频对齐。",
			Timestamp: time.Now().UTC(),
		})
	} else if proposal.Status == "rejected" {
		state.MarkStepFailed("step_shot", "用户驳回了视频生成提案")
	}

	return state, nil
}
