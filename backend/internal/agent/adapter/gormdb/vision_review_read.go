package gormdb

import (
	"context"
	"encoding/json"
	"errors"

	app "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReadAcceptedVisionReview joins an Owner transaction and reuses the same full
// dispatch/result integrity checks. No caller-supplied payload is trusted.
func (store *VisionReviewStore) ReadAcceptedVisionReview(ctx context.Context, tx *gorm.DB, reference gen.GenerationRevisionRef) (app.VisionReviewExecutionState, error) {
	if tx == nil || tx.Statement == nil || !reference.Valid() || reference.Revision != 1 {
		return app.VisionReviewExecutionState{}, errors.New("invalid accepted Vision Review read")
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return app.VisionReviewExecutionState{}, errors.New("accepted Vision Review requires an Owner transaction")
	}
	var candidate model.SceneAnalysisCandidateRevision
	var invocation model.SceneAnalysisInvocationRecord
	if err := tx.WithContext(ctx).First(&candidate, "id = ?", reference.ID).Error; err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	if err := tx.WithContext(ctx).First(&invocation, "id = ?", candidate.SourceInvocationID).Error; err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	var control model.SceneAnalysisControlHead
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&control, "release_id = ?", invocation.ReleaseID).Error; err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	if control.Status != "approved" || control.ControlRecordID != invocation.ControlRecordID || control.ControlRevision != invocation.ControlRevision || control.ControlHash != invocation.ControlHash || control.ReleaseFence != invocation.ReleaseFence {
		return app.VisionReviewExecutionState{}, &app.Error{Code: "release_not_executable", Message: "Vision Review release control changed"}
	}
	var run model.WorkflowRun
	if err := tx.WithContext(ctx).First(&run, "id = ?", invocation.WorkflowRunID).Error; err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	var payload contract.VisionReviewPayload
	encoded, err := canonical.JSON(json.RawMessage(invocation.Payload))
	if err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	if err := canonical.Decode(encoded, &payload); err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	repo := &visionReviewRepository{sceneAnalysisRepository: &sceneAnalysisRepository{database: tx}, validator: store.validator}
	if err := repo.ValidateVisionReviewInput(ctx, app.ExecuteVisionReviewCommand{WorkflowRunID: run.ID.String(), NodeRunID: invocation.NodeRunID.String(), UserID: run.CreatedBy.String(), TokenVersion: run.InitiatorTokenVersion, Input: payload.StageInput}); err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	state, err := repo.FindVisionReviewExecution(ctx, run.ID.String(), invocation.NodeRunID.String())
	if err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	if state.Status != "accepted" || state.Candidate.ID != reference.ID || state.Candidate.Revision != reference.Revision || state.Candidate.CandidateRevisionHash != reference.ContentHash {
		return app.VisionReviewExecutionState{}, &app.Error{Code: "candidate_conflict", Message: "Vision Review is not the exact accepted revision"}
	}
	return state, nil
}
