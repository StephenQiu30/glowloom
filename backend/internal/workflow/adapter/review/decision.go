package review

import (
	"context"
	"errors"

	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
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
		result.Decision.SubjectHash != result.Task.SubjectHash {
		return domain.HumanGateReviewDecision{}, errors.New("review decision and human task have drifted")
	}
	return domain.HumanGateReviewDecision{
		WorkspaceID: result.Task.WorkspaceID, WorkflowRunID: result.Task.WorkflowRunID, NodeRunID: result.Task.NodeRunID,
		HumanTaskID: result.Task.ID, ReviewDecisionID: result.Decision.ID,
		SubjectRevision: result.Decision.SubjectRevision, SubjectHash: result.Decision.SubjectHash,
		Decision: result.Decision.Decision,
	}, nil
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
