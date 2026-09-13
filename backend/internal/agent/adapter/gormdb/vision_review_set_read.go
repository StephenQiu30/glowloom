package gormdb

import (
	"context"
	"errors"

	app "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReadAcceptedVisionReviewSet locks all Controls before any review's Owner
// inputs, so opposite caller ordering cannot invert Control/Owner lock order.
func (store *VisionReviewStore) ReadAcceptedVisionReviewSet(ctx context.Context, tx *gorm.DB, refs []gen.GenerationRevisionRef) ([]app.VisionReviewExecutionState, error) {
	if tx == nil || tx.Statement == nil || refs == nil || len(refs) > 4 {
		return nil, errors.New("invalid accepted review set")
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, errors.New("accepted review set requires an Owner transaction")
	}
	states := make([]app.VisionReviewExecutionState, 0, len(refs))
	if len(refs) == 0 {
		return states, nil
	}
	ids := make([]string, len(refs))
	seen := map[string]bool{}
	for i, ref := range refs {
		if !ref.Valid() || ref.Revision != 1 || seen[ref.ID] {
			return nil, errors.New("invalid accepted review set member")
		}
		seen[ref.ID] = true
		ids[i] = ref.ID
	}
	var candidates []model.SceneAnalysisCandidateRevision
	if err := tx.WithContext(ctx).Where("id IN ?", ids).Find(&candidates).Error; err != nil {
		return nil, err
	}
	if len(candidates) != len(refs) {
		return nil, errors.New("accepted review set member missing")
	}
	invocationIDs := make([]string, len(candidates))
	for i, candidate := range candidates {
		invocationIDs[i] = candidate.SourceInvocationID.String()
	}
	var invocations []model.SceneAnalysisInvocationRecord
	if err := tx.WithContext(ctx).Where("id IN ?", invocationIDs).Find(&invocations).Error; err != nil {
		return nil, err
	}
	if len(invocations) != len(refs) {
		return nil, errors.New("accepted review set invocation missing or shared")
	}
	releaseIDs := make([]string, len(invocations))
	for i, invocation := range invocations {
		releaseIDs[i] = invocation.ReleaseID.String()
	}
	var controls []model.SceneAnalysisControlHead
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("release_id IN ?", releaseIDs).Order("release_id").Find(&controls).Error; err != nil {
		return nil, err
	}
	for _, ref := range refs {
		state, err := store.ReadAcceptedVisionReview(ctx, tx, ref)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}
