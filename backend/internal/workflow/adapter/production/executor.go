package production

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	scriptdomain "github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	scriptRevisionExecutor  = "workflow.input.script_revision"
	productionBibleExecutor = "activity.production_bible"
)

type ScriptSource interface {
	GetRevision(context.Context, scriptapp.Actor, string) (scriptdomain.Analysis, error)
}

type BibleCandidateOwner interface {
	Create(context.Context, bibleapp.Actor, bibleapp.CreateCommand) (bibledomain.Bible, error)
}

type NodeExecutor struct {
	scripts ScriptSource
	bibles  BibleCandidateOwner
}

func NewNodeExecutor(scripts ScriptSource, bibles BibleCandidateOwner) *NodeExecutor {
	return &NodeExecutor{scripts: scripts, bibles: bibles}
}

func (executor *NodeExecutor) Execute(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor == nil || strings.TrimSpace(command.WorkspaceID) == "" || strings.TrimSpace(command.ProjectID) == "" ||
		strings.TrimSpace(command.InitiatorUserID) == "" || command.InitiatorTokenVersion < 1 ||
		strings.TrimSpace(command.IdempotencyKey) == "" {
		return domain.NodeExecutorResult{}, errors.New("unsupported production workflow node execution")
	}
	switch command.Executor {
	case scriptRevisionExecutor:
		return executor.executeScriptRevision(ctx, command)
	case productionBibleExecutor:
		return executor.executeProductionBible(ctx, command)
	default:
		return domain.NodeExecutorResult{}, errors.New("unsupported production workflow node execution")
	}
}

func (executor *NodeExecutor) executeScriptRevision(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.scripts == nil {
		return domain.NodeExecutorResult{}, errors.New("script revision workflow owner is unavailable")
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

func (executor *NodeExecutor) executeProductionBible(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor.bibles == nil {
		return domain.NodeExecutorResult{}, errors.New("production bible workflow owner is unavailable")
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "candidate" ||
		command.OutputPorts[0].ValueType != "production_bible_candidate" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid production bible node contract")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 0 {
		return domain.NodeExecutorResult{}, errors.New("invalid production bible node config")
	}
	binding := input.Bindings[0]
	if binding.Port != "script" || binding.ValueType != "script_revision" ||
		binding.SourceKind != domain.NodeInputSourceNodeOutput || binding.SourcePort != "script" ||
		strings.TrimSpace(binding.SourceNodeID) == "" || !frozenScriptMatches(input, binding) {
		return domain.NodeExecutorResult{}, errors.New("production bible script input has drifted")
	}
	bible, err := executor.bibles.Create(ctx, bibleapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, bibleapp.CreateCommand{RevisionID: binding.ReferenceID, IdempotencyKey: command.IdempotencyKey})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if bible.WorkspaceID != command.WorkspaceID || bible.ProjectID != command.ProjectID ||
		bible.DocumentRevisionID != binding.ReferenceID || bible.InputHash != binding.ContentHash ||
		bible.CreatedBy != command.InitiatorUserID || bible.Revision < 1 {
		return domain.NodeExecutorResult{}, errors.New("production bible owner result does not match workflow input")
	}
	switch bible.Status {
	case "queued", "running":
		return domain.NodeExecutorResult{Status: "RETRYING"}, nil
	case "needs_review":
		if bible.ResultHash == nil || len(*bible.ResultHash) != 64 {
			return domain.NodeExecutorResult{}, errors.New("production bible candidate result is incomplete")
		}
	default:
		return domain.NodeExecutorResult{}, errors.New("production bible candidate is unavailable")
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "candidate", ValueType: "production_bible_candidate", ReferenceID: bible.ID,
			ReferenceVersion: strconv.Itoa(bible.Revision), ContentHash: *bible.ResultHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func frozenScriptMatches(input domain.NodeInputSnapshot, binding domain.NodeInputBinding) bool {
	for _, reference := range input.FrozenInputs {
		if reference.Kind == "script_revision" && reference.ID == binding.ReferenceID &&
			reference.Version == binding.ReferenceVersion && reference.Hash == binding.ContentHash {
			return true
		}
	}
	return false
}

var _ workflowapp.NodeExecutor = (*NodeExecutor)(nil)
