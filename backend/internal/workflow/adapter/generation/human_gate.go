package generation

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"

	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	generationImageReviewExecutor = "gate.generation_image_review"
	generationSelectionOperation  = "generation.candidate.select"
)

type SelectionOwner interface {
	ApplySelection(context.Context, generationapp.Actor, generationapp.ApplySelectionCommand) (generationapp.ApplySelectionResult, error)
}

type HumanGateApplier struct{ selections SelectionOwner }

func NewHumanGateApplier(selections SelectionOwner) *HumanGateApplier {
	return &HumanGateApplier{selections: selections}
}

func (applier *HumanGateApplier) ApplyHumanGateDecision(
	ctx context.Context,
	actor workflowapp.Actor,
	application domain.HumanGateOwnerApplication,
) (domain.HumanGateOwnerResult, error) {
	if applier == nil || applier.selections == nil || application.Executor != generationImageReviewExecutor ||
		application.Decision != "selected" || application.Candidate.Port != "candidates" ||
		application.Candidate.ValueType != "generation_candidate_set" ||
		application.Candidate.SourceKind != domain.NodeInputSourceNodeOutput ||
		application.Candidate.ReferenceVersion != "1" || len(application.Candidate.ContentHash) != 64 ||
		application.OutputPort != "selection" || application.OutputValueType != "generation_candidate_selection" ||
		actor.TokenVersion < 1 || !validUUID(actor.UserID) {
		return domain.HumanGateOwnerResult{}, errors.New("unsupported Generation Workflow Human Gate application")
	}
	result, err := applier.selections.ApplySelection(ctx, generationapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, generationapp.ApplySelectionCommand{
		ReviewDecisionID: application.ReviewDecisionID,
		IdempotencyKey:   "workflow-review:" + application.ReviewDecisionID,
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	selection := result.Selection
	if !validUUID(selection.ID) || selection.WorkspaceID != application.WorkspaceID ||
		selection.ProjectID != application.ProjectID || selection.WorkflowRunID != application.WorkflowRunID ||
		selection.NodeRunID != application.NodeRunID || selection.HumanTaskID != application.HumanTaskID ||
		selection.ReviewDecisionID != application.ReviewDecisionID ||
		selection.SubjectType != "generation_candidate_selection" || selection.SubjectID != application.NodeRunID ||
		selection.SubjectRevision != application.SubjectRevision ||
		selection.CandidateSetHash != application.Candidate.ContentHash || !validUUID(selection.SelectedCandidateID) ||
		len(selection.ContentHash) != 64 || selection.Revision != 1 || selection.CreatedBy != actor.UserID ||
		result.Receipt.ID == "" || result.Receipt.WorkspaceID != application.WorkspaceID ||
		result.Receipt.Operation != generationSelectionOperation || result.Receipt.ResourceID != selection.ID ||
		result.Receipt.CreatedBy != actor.UserID {
		return domain.HumanGateOwnerResult{}, errors.New("Generation selection owner result does not match Workflow Human Gate")
	}
	output, _, outputHash, err := domain.BuildNodeOutput(domain.NodeOutputSnapshot{
		SchemaVersion: domain.NodeOutputSchemaVersion,
		Bindings: []domain.NodeOutputBinding{{
			Port: application.OutputPort, ValueType: application.OutputValueType,
			ReferenceID: selection.ID, ReferenceVersion: strconv.Itoa(selection.Revision), ContentHash: selection.ContentHash,
		}},
	})
	if err != nil {
		return domain.HumanGateOwnerResult{}, err
	}
	return domain.HumanGateOwnerResult{
		ReceiptID: strings.TrimSpace(result.Receipt.ID), Operation: result.Receipt.Operation,
		Output: output, OutputHash: outputHash,
	}, nil
}

func validUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

var _ workflowapp.HumanGateOwnerApplier = (*HumanGateApplier)(nil)
