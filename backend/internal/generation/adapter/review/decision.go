package review

import (
	"context"
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
)

type DecisionReader struct {
	reviews *reviewapp.Service
}

func NewDecisionReader(reviews *reviewapp.Service) *DecisionReader {
	return &DecisionReader{reviews: reviews}
}

func (reader *DecisionReader) GetSelectedDecision(
	ctx context.Context,
	actor application.Actor,
	decisionID string,
) (application.SelectionDecision, error) {
	if reader == nil || reader.reviews == nil {
		return application.SelectionDecision{}, &application.Error{
			Code: "not_found", Message: "Review decision not found", Status: 404,
		}
	}
	result, err := reader.reviews.GetDecision(ctx, reviewapp.Actor{
		UserID: actor.UserID, TokenVersion: actor.TokenVersion,
	}, decisionID)
	if err != nil {
		return application.SelectionDecision{}, mapReviewError(err)
	}
	if result.Decision.Decision != "selected" || result.Decision.SelectedCandidateID == nil {
		return application.SelectionDecision{}, &application.Error{
			Code: "state_conflict", Message: "Review decision did not select a generation candidate", Status: 409,
		}
	}
	return application.SelectionDecision{
		ID: result.Decision.ID, WorkspaceID: result.Task.WorkspaceID, ProjectID: result.Task.ProjectID,
		WorkflowRunID: result.Task.WorkflowRunID, NodeRunID: result.Task.NodeRunID,
		HumanTaskID: result.Task.ID, SubjectType: result.Task.SubjectType, SubjectID: result.Task.SubjectID,
		SubjectRevision: result.Task.SubjectRevision, CandidateIDs: append([]string(nil), result.Task.CandidateIDs...),
		SelectedCandidateID: *result.Decision.SelectedCandidateID, ReviewerID: result.Decision.CreatedBy,
	}, nil
}

func mapReviewError(err error) error {
	if errors.Is(err, reviewapp.ErrNotFound) {
		return &application.Error{Code: "not_found", Message: "Review decision not found", Status: 404}
	}
	var typed *reviewapp.Error
	if errors.As(err, &typed) {
		return &application.Error{Code: typed.Code, Message: typed.Message, Status: typed.Status}
	}
	return err
}

var _ application.ReviewDecisionReader = (*DecisionReader)(nil)
