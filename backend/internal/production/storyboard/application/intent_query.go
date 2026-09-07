package application

import (
	"context"
	"encoding/json"

	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
	"github.com/google/uuid"
)

func (service *Service) GetDraftSet(ctx context.Context, actor Actor, setID string) (domain.DraftSet, error) {
	if _, err := uuid.Parse(setID); err != nil {
		return domain.DraftSet{}, invalid("Invalid storyboard draft set identity")
	}
	var result domain.DraftSet
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		result, err = repo.GetSet(ctx, actor, setID, false)
		return err
	})
	return result, normalizeError(err)
}

// GetApprovedIntents reads the committed owner receipt without rerunning a workflow
// or requiring an old candidate to remain the current draft head.
func (service *Service) GetApprovedIntents(ctx context.Context, actor Actor, setID string) (FreezeIntentSetResult, error) {
	if _, err := uuid.Parse(setID); err != nil {
		return FreezeIntentSetResult{}, invalid("Invalid storyboard draft set identity")
	}
	var result FreezeIntentSetResult
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		set, err := repo.GetSet(ctx, actor, setID, false)
		if err != nil {
			return err
		}
		if set.Status != "intent_frozen" {
			return conflict("Storyboard text intents have not been accepted")
		}
		receipt, err := repo.GetIntentReceipt(ctx, set.ID)
		if err != nil {
			return err
		}
		var approved domain.ApprovedIntentSet
		if err = json.Unmarshal(receipt.Result, &approved); err != nil {
			return conflict("Storyboard Intent receipt is invalid")
		}
		contentHash, err := domain.ApprovedIntentSetContentHash(approved)
		if err != nil {
			return err
		}
		visualHash, err := domain.ApprovedIntentVisualRequirementsHash(approved.Scenes)
		if err != nil {
			return err
		}
		if receipt.Operation != freezeIntentSetOperation || receipt.ResourceID != set.ID || receipt.WorkspaceID != set.WorkspaceID ||
			approved.SchemaVersion != "approved-storyboard-intents" || approved.ID != receipt.ID || approved.DraftSetID != set.ID ||
			approved.ProjectID != set.ProjectID || approved.WorkspaceID != set.WorkspaceID || approved.DraftSetRevision+1 != set.Revision ||
			set.ResultHash == nil || approved.ContentHash != *set.ResultHash || approved.ContentHash != contentHash || approved.VisualRequirementsHash != visualHash {
			return conflict("Storyboard Intent receipt has drifted")
		}
		result = FreezeIntentSetResult{Set: set, Approved: approved, Receipt: receipt}
		return nil
	})
	return result, normalizeError(err)
}
