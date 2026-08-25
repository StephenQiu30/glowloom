package production

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	scriptdomain "github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const scriptRevisionExecutor = "workflow.input.script_revision"

type ScriptSource interface {
	GetRevision(context.Context, scriptapp.Actor, string) (scriptdomain.Analysis, error)
}

type NodeExecutor struct{ scripts ScriptSource }

func NewNodeExecutor(scripts ScriptSource) *NodeExecutor { return &NodeExecutor{scripts: scripts} }

func (executor *NodeExecutor) Execute(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor == nil || executor.scripts == nil || command.Executor != scriptRevisionExecutor ||
		strings.TrimSpace(command.WorkspaceID) == "" || strings.TrimSpace(command.ProjectID) == "" ||
		strings.TrimSpace(command.InitiatorUserID) == "" || command.InitiatorTokenVersion < 1 ||
		strings.TrimSpace(command.IdempotencyKey) == "" {
		return domain.NodeExecutorResult{}, errors.New("unsupported production workflow node execution")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 0 || len(input.FrozenInputs) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "script" ||
		command.OutputPorts[0].ValueType != "script_revision" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid script revision node contract")
	}
	reference := input.FrozenInputs[0]
	var config struct {
		DocumentRevisionID string `json:"document_revision_id"`
	}
	if reference.Kind != "script_revision" || json.Unmarshal(input.Config, &config) != nil ||
		config.DocumentRevisionID != reference.ID {
		return domain.NodeExecutorResult{}, errors.New("script revision input does not match node config")
	}
	analysis, err := executor.scripts.GetRevision(ctx, scriptapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, reference.ID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if analysis.Document.WorkspaceID != command.WorkspaceID || analysis.Document.ProjectID != command.ProjectID ||
		analysis.Revision.WorkspaceID != command.WorkspaceID || analysis.Revision.DocumentID != analysis.Document.ID ||
		analysis.Revision.ID != reference.ID || strconv.Itoa(analysis.Revision.VersionNo) != reference.Version ||
		analysis.Revision.NormalizedHash != reference.Hash {
		return domain.NodeExecutorResult{}, errors.New("script revision changed before workflow node execution")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "script", ValueType: "script_revision", ReferenceID: analysis.Revision.ID,
			ReferenceVersion: strconv.Itoa(analysis.Revision.VersionNo), ContentHash: analysis.Revision.NormalizedHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

var _ workflowapp.NodeExecutor = (*NodeExecutor)(nil)
