package agents

import (
	"context"

	"github.com/google/uuid"

	platformagent "github.com/stephenqiu30/lanverse/backend/src/platform/agent"
)

type AgentRunStore interface {
	CreateRun(context.Context, AgentRun, []ProposalItem) error
	GetRun(context.Context, uuid.UUID) (AgentRun, []ProposalItem, error)
	UpdateRun(context.Context, uuid.UUID, string, string) error
}

type AgentExecutor interface {
	Start(context.Context, platformagent.AgentRunRequest) (platformagent.AgentRunResponse, error)
	Get(context.Context, uuid.UUID) (platformagent.AgentRunResponse, error)
	Cancel(context.Context, uuid.UUID) (platformagent.AgentRunResponse, error)
}
