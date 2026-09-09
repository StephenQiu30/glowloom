package review

import (
	"context"
	"errors"

	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	reviewdomain "github.com/StephenQiu30/lanverse/backend/internal/review/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type DecisionReader struct {
	reviews *reviewapp.Service
}

func NewDecisionReader(reviews *reviewapp.Service) *DecisionReader {
	return &DecisionReader{reviews: reviews}
}

func (reader *DecisionReader) GetHumanGateDecision(
	ctx context.Context,
	actor workflowapp.Actor,
	decisionID string,
) (domain.HumanGateReviewDecision, error) {
	if reader == nil || reader.reviews == nil {
		return domain.HumanGateReviewDecision{}, workflowapp.ErrNotFound
	}
	result, err := reader.reviews.GetDecision(ctx, reviewapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, decisionID)
	if err != nil {
		return domain.HumanGateReviewDecision{}, mapDecisionError(err)
	}
	if result.Decision.HumanTaskID != result.Task.ID || result.Decision.SubjectRevision != result.Task.SubjectRevision ||
		result.Decision.SubjectHash != result.Task.SubjectHash || len(result.Decision.DecisionPayloadHash) != 64 {
		return domain.HumanGateReviewDecision{}, errors.New("review decision and human task have drifted")
	}
	decision := domain.HumanGateReviewDecision{
		WorkspaceID: result.Task.WorkspaceID, ProjectID: result.Task.ProjectID,
		WorkflowRunID: result.Task.WorkflowRunID, NodeRunID: result.Task.NodeRunID,
		HumanTaskID: result.Task.ID, ReviewDecisionID: result.Decision.ID,
		SubjectType:     result.Task.SubjectType,
		SubjectRevision: result.Decision.SubjectRevision, SubjectHash: result.Decision.SubjectHash,
		Decision: result.Decision.Decision, DecisionPayloadHash: result.Decision.DecisionPayloadHash,
	}
	if decision.SubjectType == "production_world_gate_input" {
		decision.ProductionWorldChangeRequest = productionWorldChangeRequest(result.Decision.ChangeRequest)
	} else {
		decision.ChangeRequest = structureIdentityChangeRequest(result.Decision.ChangeRequest)
	}
	return decision, nil
}

func productionWorldChangeRequest(value *reviewdomain.ChangeRequest) *domain.ProductionWorldChangeRequest {
	if value == nil {
		return nil
	}
	result := &domain.ProductionWorldChangeRequest{
		IssueRefs: append([]string(nil), value.IssueRefs...),
		ChangeSpec: domain.ProductionWorldRepairChange{
			Operation: value.ChangeSpec.Operation, TargetKeys: append([]string(nil), value.ChangeSpec.TargetKeys...),
			AffectedScopeKeys: append([]string(nil), value.ChangeSpec.AffectedScopeKeys...),
		},
		ReasonCode: value.ReasonCode,
	}
	result.EvidenceRefs = make([]domain.HumanGateEvidenceRef, len(value.EvidenceRefs))
	for index, evidence := range value.EvidenceRefs {
		result.EvidenceRefs[index] = domain.HumanGateEvidenceRef{
			SourceVersionID: evidence.SourceVersionID, SourceStart: evidence.SourceStart,
			SourceEnd: evidence.SourceEnd, TextHash: evidence.TextHash,
		}
	}
	if value.UserNote != nil {
		note := *value.UserNote
		result.UserNote = &note
	}
	return result
}

func structureIdentityChangeRequest(value *reviewdomain.ChangeRequest) *domain.StructureIdentityChangeRequest {
	if value == nil {
		return nil
	}
	result := &domain.StructureIdentityChangeRequest{
		IssueRefs: append([]string(nil), value.IssueRefs...),
		ChangeSpec: domain.StructureIdentityAllowedChange{
			Operation: value.ChangeSpec.Operation, TargetKeys: append([]string(nil), value.ChangeSpec.TargetKeys...),
			AffectedScopeKeys: append([]string(nil), value.ChangeSpec.AffectedScopeKeys...),
		},
		ReasonCode: value.ReasonCode,
	}
	result.EvidenceRefs = make([]domain.HumanGateEvidenceRef, len(value.EvidenceRefs))
	for index, evidence := range value.EvidenceRefs {
		result.EvidenceRefs[index] = domain.HumanGateEvidenceRef{
			SourceVersionID: evidence.SourceVersionID, SourceStart: evidence.SourceStart,
			SourceEnd: evidence.SourceEnd, TextHash: evidence.TextHash,
		}
	}
	if value.UserNote != nil {
		note := *value.UserNote
		result.UserNote = &note
	}
	return result
}

func mapDecisionError(err error) error {
	if errors.Is(err, reviewapp.ErrNotFound) {
		return workflowapp.ErrNotFound
	}
	var typed *reviewapp.Error
	if errors.As(err, &typed) {
		return &workflowapp.Error{Code: typed.Code, Message: typed.Message, Status: typed.Status}
	}
	return err
}

var _ workflowapp.HumanGateDecisionReader = (*DecisionReader)(nil)
