package generation

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const candidateSetInputExecutor = "workflow.input.generation_candidate_set"

var candidateSetHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type CandidateSetSource interface {
	RequireCandidateSet(context.Context, generationapp.Actor, string) (generationdomain.CandidateSet, error)
}

type NodeExecutor struct {
	candidateSets CandidateSetSource
}

func NewNodeExecutor(candidateSets CandidateSetSource) *NodeExecutor {
	return &NodeExecutor{candidateSets: candidateSets}
}

func (executor *NodeExecutor) Execute(
	ctx context.Context,
	command domain.NodeExecutorCommand,
) (domain.NodeExecutorResult, error) {
	if executor == nil || executor.candidateSets == nil || command.Executor != candidateSetInputExecutor ||
		strings.TrimSpace(command.IdempotencyKey) == "" || command.InitiatorTokenVersion < 1 {
		return domain.NodeExecutorResult{}, errors.New("unsupported Generation workflow node execution")
	}
	for _, identifier := range []string{command.WorkspaceID, command.ProjectID, command.InitiatorUserID} {
		if _, err := uuid.Parse(strings.TrimSpace(identifier)); err != nil {
			return domain.NodeExecutorResult{}, errors.New("invalid Generation workflow execution boundary")
		}
	}
	input, _, inputHash, err := domain.BuildNodeInput(command.Input)
	if err != nil || inputHash != command.InputHash || len(input.Bindings) != 1 ||
		len(command.OutputPorts) != 1 || command.OutputPorts[0].Key != "candidates" ||
		command.OutputPorts[0].ValueType != "generation_candidate_set" || !command.OutputPorts[0].Required {
		return domain.NodeExecutorResult{}, errors.New("invalid Generation CandidateSet source contract")
	}
	shot := input.Bindings[0]
	shotRevision, revisionErr := strconv.Atoi(shot.ReferenceVersion)
	if shot.Port != "shot" || shot.ValueType != "production_shot" ||
		shot.SourceKind != domain.NodeInputSourceNodeOutput || strings.TrimSpace(shot.SourceNodeID) == "" ||
		shot.SourcePort != "shot" || revisionErr != nil || shotRevision < 1 ||
		!validCandidateSetUUID(shot.ReferenceID) || !candidateSetHashPattern.MatchString(shot.ContentHash) {
		return domain.NodeExecutorResult{}, errors.New("Generation CandidateSet Shot input has drifted")
	}
	var config map[string]json.RawMessage
	var providerJobID string
	if json.Unmarshal(input.Config, &config) != nil || len(config) != 1 ||
		json.Unmarshal(config["provider_job_id"], &providerJobID) != nil || !validCandidateSetUUID(providerJobID) {
		return domain.NodeExecutorResult{}, errors.New("invalid Generation CandidateSet source config")
	}
	providerJobID = strings.TrimSpace(providerJobID)
	set, err := executor.candidateSets.RequireCandidateSet(ctx, generationapp.Actor{
		UserID: command.InitiatorUserID, TokenVersion: command.InitiatorTokenVersion,
	}, providerJobID)
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	if set.ID != providerJobID || set.WorkspaceID != command.WorkspaceID || set.ProjectID != command.ProjectID ||
		set.Revision != 1 || !validCandidateSetUUID(set.ProviderReceiptID) ||
		!candidateSetHashPattern.MatchString(set.ContentHash) || len(set.Candidates) == 0 || len(set.Candidates) > 100 {
		return domain.NodeExecutorResult{}, errors.New("Generation CandidateSet source has drifted")
	}
	seenCandidates := make(map[string]struct{}, len(set.Candidates))
	for _, candidate := range set.Candidates {
		if !validCandidateSetUUID(candidate.ID) || !validCandidateSetUUID(candidate.ArtifactID) || !validCandidateSetUUID(candidate.QCReportID) ||
			candidate.Revision < 1 || candidate.ArtifactRevision < 1 ||
			!candidateSetHashPattern.MatchString(candidate.ArtifactSHA256) ||
			!candidateSetHashPattern.MatchString(candidate.QCReportHash) {
			return domain.NodeExecutorResult{}, errors.New("Generation CandidateSet contains an invalid candidate")
		}
		if _, exists := seenCandidates[candidate.ID]; exists {
			return domain.NodeExecutorResult{}, errors.New("Generation CandidateSet contains duplicate candidates")
		}
		seenCandidates[candidate.ID] = struct{}{}
	}
	output, _, _, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: "candidates", ValueType: "generation_candidate_set", ReferenceID: set.ID,
			ReferenceVersion: strconv.Itoa(set.Revision), ContentHash: set.ContentHash,
		}},
	})
	if err != nil {
		return domain.NodeExecutorResult{}, err
	}
	return domain.NodeExecutorResult{Status: "SUCCEEDED", Output: output}, nil
}

func validCandidateSetUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

var _ workflowapp.NodeExecutor = (*NodeExecutor)(nil)
