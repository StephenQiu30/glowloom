package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"

	platformagent "github.com/stephenqiu30/lanverse/backend/src/platform/agent"
	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
)

type AgentService struct {
	repository AgentRunStore
	executor   AgentExecutor
}

func NewAgentService(repository AgentRunStore, executor AgentExecutor) *AgentService {
	return &AgentService{repository: repository, executor: executor}
}

type StartInput struct {
	ProjectID   uuid.UUID
	OperationID uuid.UUID
	Skill       string
	Stage       string
	RequestHash string
	SnapshotRef string
}

func (s *AgentService) Start(ctx context.Context, input StartInput) (AgentRun, []ProposalItem, error) {
	if input.Skill != "script_analysis" || input.Stage != "manifest" && input.Stage != "narrative" && input.Stage != "knowledge" {
		return AgentRun{}, nil, httpapi.Validation("Agent skill 或 stage 不受支持", "选择当前契约允许的 Agent skill/stage")
	}
	if input.RequestHash == "" || input.SnapshotRef == "" {
		return AgentRun{}, nil, httpapi.Validation("request hash 和 snapshot reference 不能为空", "提供当前 AgentRun 输入快照后重试")
	}
	remote, err := s.executor.Start(ctx, platformagent.AgentRunRequest{Skill: input.Skill, Stage: input.Stage, RequestHash: input.RequestHash, SnapshotRef: input.SnapshotRef})
	if err != nil {
		return AgentRun{}, nil, err
	}
	inputHash := sha256.Sum256([]byte(input.SnapshotRef))
	run := AgentRun{ID: remote.RunID, ProjectID: input.ProjectID, OperationID: input.OperationID, Skill: remote.Skill, Stage: remote.Stage, StageGeneration: 1, RequestHash: remote.RequestHash, Status: remote.Status, InputSnapshotHash: hex.EncodeToString(inputHash[:])}
	items := make([]ProposalItem, 0, len(remote.Items))
	for _, item := range remote.Items {
		payloadHash := sha256.Sum256([]byte(fmt.Sprintf("%v", item.Value)))
		items = append(items, ProposalItem{ID: uuid.New(), AgentRunID: remote.RunID, TargetModule: "narrative", TargetCommand: item.Kind, Payload: item.Value, Decision: "pending", ReadSetHash: inputHashHex(inputHash), WriteSetHash: hex.EncodeToString(payloadHash[:])})
	}
	if err := s.repository.CreateRun(ctx, run, items); err != nil {
		return AgentRun{}, nil, err
	}
	return run, items, nil
}

func inputHashHex(value [32]byte) string { return hex.EncodeToString(value[:]) }

func (s *AgentService) Get(ctx context.Context, id uuid.UUID) (AgentRun, []ProposalItem, error) {
	return s.repository.GetRun(ctx, id)
}

func (s *AgentService) Cancel(ctx context.Context, id uuid.UUID) (AgentRun, []ProposalItem, error) {
	run, items, err := s.repository.GetRun(ctx, id)
	if err != nil {
		return AgentRun{}, nil, err
	}
	if run.Status == "succeeded" || run.Status == "failed" || run.Status == "cancelled" {
		return run, items, nil
	}
	if _, err := s.executor.Cancel(ctx, id); err != nil {
		return AgentRun{}, nil, err
	}
	if err := s.repository.UpdateRun(ctx, id, "cancelled", ""); err != nil {
		return AgentRun{}, nil, err
	}
	run.Status = "cancelled"
	return run, items, nil
}
