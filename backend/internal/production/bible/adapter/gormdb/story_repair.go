package gormdb

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	commandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
)

func (store *Store) LoadStoryCandidateRepair(
	ctx context.Context,
	actor application.Actor,
	command application.StoryCandidateRepairCommand,
) (application.StoryCandidateRepairSeed, error) {
	return loadStoryCandidateRepair(ctx, store.database, actor, command)
}

func (store *Store) ApplyStoryCandidateRepair(
	ctx context.Context,
	actor application.Actor,
	preparation application.StoryCandidateRepairPreparation,
) (application.StoryCandidateRepairResult, error) {
	workspaceID, err := uuid.Parse(preparation.Command.WorkspaceID)
	if err != nil {
		return application.StoryCandidateRepairResult{}, application.ErrNotFound
	}
	expectedRevisionID, err := uuid.Parse(preparation.Command.ExpectedRevisionID)
	if err != nil {
		return application.StoryCandidateRepairResult{}, application.ErrNotFound
	}
	repairInvocationID, err := uuid.Parse(preparation.Command.RepairInvocationID)
	if err != nil {
		return application.StoryCandidateRepairResult{}, application.ErrNotFound
	}
	receiptID, err := uuid.Parse(preparation.ReceiptID)
	if err != nil || preparation.CreatedAt.IsZero() || !candidateRepairHash(preparation.InputHash) {
		return application.StoryCandidateRepairResult{}, errors.New("invalid Story candidate repair preparation")
	}

	var result application.StoryCandidateRepairResult
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		seed, loadErr := loadStoryCandidateRepair(ctx, transaction, actor, preparation.Command)
		if loadErr != nil {
			return loadErr
		}
		expectedCandidate, applyErr := application.ApplyStoryCandidateRepairPatch(
			seed.ParentCandidate,
			seed.RepairInput,
			seed.RepairPatch,
		)
		if applyErr != nil {
			return applyErr
		}
		expectedCandidateHash, hashErr := agentcontract.CanonicalHash(expectedCandidate)
		if hashErr != nil {
			return hashErr
		}
		preparedCandidateHash, hashErr := agentcontract.CanonicalHash(preparation.Candidate)
		if hashErr != nil || preparedCandidateHash != expectedCandidateHash {
			return errors.New("Story candidate repair preparation has drifted")
		}

		var head model.StageCandidateHead
		if loadErr = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND stage_instance_key = ?", workspaceID, preparation.Command.StageInstanceKey).
			First(&head).Error; loadErr != nil {
			if errors.Is(loadErr, gorm.ErrRecordNotFound) {
				return storyCandidateRepairConflict("Story candidate Head does not exist")
			}
			return loadErr
		}

		receipt, receiptErr := commandgorm.Find(
			ctx,
			transaction,
			preparation.Command.WorkspaceID,
			application.StoryCandidateRepairOperation,
			preparation.Command.IdempotencyKey,
		)
		if receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[application.StoryCandidateRepairResult](
				receipt,
				preparation.InputHash,
			)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return storyCandidateRepairConflict("Idempotency key was already used with different Candidate repair input")
			}
			if replayErr != nil {
				return replayErr
			}
			if replayed.ReceiptID != receipt.ID || replayed.CandidateRevisionID != receipt.ResourceID {
				return errors.New("Story candidate repair Receipt has drifted")
			}
			result = replayed
			return nil
		}
		if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if head.CurrentRevisionID != expectedRevisionID ||
			head.CurrentCandidateRevisionHash != preparation.Command.ExpectedCandidateRevisionHash ||
			head.Revision != preparation.Command.ExpectedHeadRevision {
			return storyCandidateRepairConflict("Story candidate Head changed before repair")
		}

		created, advanceErr := agentgorm.AdvanceCandidateHeadWithRepair(
			transaction,
			agentgorm.CandidateRepairAdvance{
				WorkspaceID: workspaceID, StageInstanceKey: preparation.Command.StageInstanceKey,
				ExpectedRevisionID:            expectedRevisionID,
				ExpectedCandidateRevisionHash: preparation.Command.ExpectedCandidateRevisionHash,
				ExpectedHeadRevision:          preparation.Command.ExpectedHeadRevision,
				RepairInvocationID:            repairInvocationID, RepairResultHash: seed.RepairResultHash,
				Candidate: preparation.Candidate, CreatedAt: preparation.CreatedAt,
			},
		)
		if errors.Is(advanceErr, agentgorm.ErrCandidateHeadConflict) {
			return storyCandidateRepairConflict("Story candidate Head changed before repair")
		}
		if advanceErr != nil {
			return advanceErr
		}

		closure, closureErr := storyCandidateStaleClosure(
			transaction,
			workspaceID,
			application.CandidateRevisionRef{
				ID: expectedRevisionID.String(), Hash: preparation.Command.ExpectedCandidateRevisionHash,
			},
			repairInvocationID.String(),
		)
		if closureErr != nil {
			return closureErr
		}
		staleKeys := make([]string, 0, len(closure))
		for _, stale := range closure {
			var invocationID *uuid.UUID
			if stale.InvocationID != "" {
				parsed, parseErr := uuid.Parse(stale.InvocationID)
				if parseErr != nil {
					return parseErr
				}
				invocationID = &parsed
			}
			causeID, parseErr := uuid.Parse(stale.CauseCandidateRevisionID)
			if parseErr != nil {
				return parseErr
			}
			record := model.StageInstanceStaleness{
				ID: uuid.New(), WorkspaceID: workspaceID, InvocationID: invocationID,
				StageInstanceKey:           stale.StageInstanceKey,
				CauseCandidateRevisionID:   causeID,
				CauseCandidateRevisionHash: stale.CauseCandidateRevisionHash,
				CreatedAt:                  preparation.CreatedAt,
			}
			if createErr := transaction.Omit(clause.Associations).Create(&record).Error; createErr != nil {
				return createErr
			}
			staleKeys = append(staleKeys, stale.StageInstanceKey)
		}

		result = application.StoryCandidateRepairResult{
			ReceiptID: receiptID.String(), CandidateRevisionID: created.ID.String(),
			CandidateRevisionHash:  created.CandidateRevisionHash,
			CandidateRevisionNo:    created.RevisionNo,
			StaleStageInstanceKeys: staleKeys,
		}
		encodedResult, encodeErr := platformcommand.Result(result)
		if encodeErr != nil {
			return encodeErr
		}
		_, receiptErr = commandgorm.Ensure(ctx, transaction, platformcommand.Receipt{
			ID: receiptID.String(), WorkspaceID: workspaceID.String(),
			Operation:      application.StoryCandidateRepairOperation,
			IdempotencyKey: preparation.Command.IdempotencyKey,
			InputHash:      preparation.InputHash, ResourceID: created.ID.String(), Result: encodedResult,
			CreatedBy: actor.UserID, CreatedAt: preparation.CreatedAt,
		})
		if errors.Is(receiptErr, platformcommand.ErrInputMismatch) {
			return storyCandidateRepairConflict("Idempotency key was already used with different Candidate repair input")
		}
		return receiptErr
	})
	return result, err
}

func loadStoryCandidateRepair(
	ctx context.Context,
	database *gorm.DB,
	actor application.Actor,
	command application.StoryCandidateRepairCommand,
) (application.StoryCandidateRepairSeed, error) {
	workspaceID, err := uuid.Parse(command.WorkspaceID)
	if err != nil {
		return application.StoryCandidateRepairSeed{}, application.ErrNotFound
	}
	revisionID, err := uuid.Parse(command.ExpectedRevisionID)
	if err != nil {
		return application.StoryCandidateRepairSeed{}, application.ErrNotFound
	}
	invocationID, err := uuid.Parse(command.RepairInvocationID)
	if err != nil {
		return application.StoryCandidateRepairSeed{}, application.ErrNotFound
	}

	var invocation model.AgentInvocation
	if err = database.WithContext(ctx).First(&invocation, "id = ?", invocationID).Error; err != nil {
		return application.StoryCandidateRepairSeed{}, normalizeNotFound(err)
	}
	request, err := agentgorm.StageInvocation(invocation)
	if err != nil || request.Payload.Stage != "repair_candidate" || invocation.Stage != "repair_candidate" ||
		invocation.WorkspaceID != workspaceID ||
		invocation.Status != "succeeded" || invocation.ResultHash == nil || invocation.CandidateType == nil ||
		*invocation.CandidateType != "candidate_repair_patch" {
		return application.StoryCandidateRepairSeed{}, errors.New("Story candidate repair requires one successful Patch Invocation")
	}
	projectID, err := uuid.Parse(request.Payload.ProjectID)
	if err != nil {
		return application.StoryCandidateRepairSeed{}, err
	}
	if err = authorizeProject(ctx, database, actor, projectID, true); err != nil {
		return application.StoryCandidateRepairSeed{}, err
	}
	if request.Payload.WorkspaceID != command.WorkspaceID {
		return application.StoryCandidateRepairSeed{}, errors.New("Story candidate repair workspace has drifted")
	}

	var revision model.StageCandidateRevision
	if err = database.WithContext(ctx).First(&revision, "id = ?", revisionID).Error; err != nil {
		return application.StoryCandidateRepairSeed{}, normalizeNotFound(err)
	}
	if revision.WorkspaceID != workspaceID || revision.StageInstanceKey != command.StageInstanceKey ||
		revision.RevisionNo != command.ExpectedHeadRevision ||
		revision.CandidateRevisionHash != command.ExpectedCandidateRevisionHash {
		return application.StoryCandidateRepairSeed{}, storyCandidateRepairConflict("Story candidate repair target has drifted")
	}
	parentContentHash, err := agentcontract.CanonicalHash(json.RawMessage(revision.Candidate))
	if err != nil || parentContentHash != revision.CandidateContentHash {
		return application.StoryCandidateRepairSeed{}, errors.New("Story candidate parent content hash has drifted")
	}

	var repairInput agentcontract.StoryGraphRepairStageInput
	if err = json.Unmarshal(request.Payload.StageInput, &repairInput); err != nil || repairInput.Validate() != nil ||
		repairInput.TargetCandidateRevisionID != command.ExpectedRevisionID ||
		repairInput.TargetCandidateRevisionHash != command.ExpectedCandidateRevisionHash {
		return application.StoryCandidateRepairSeed{}, errors.New("Story candidate repair input does not bind the expected parent")
	}
	reviewRevisionID, err := uuid.Parse(repairInput.ReviewCandidateRevisionID)
	if err != nil {
		return application.StoryCandidateRepairSeed{}, err
	}
	var reviewRevision model.StageCandidateRevision
	if err = database.WithContext(ctx).First(&reviewRevision, "id = ?", reviewRevisionID).Error; err != nil {
		return application.StoryCandidateRepairSeed{}, normalizeNotFound(err)
	}
	if reviewRevision.WorkspaceID != workspaceID ||
		reviewRevision.CandidateRevisionHash != repairInput.ReviewCandidateRevisionHash ||
		reviewRevision.SourceInvocationID == nil || reviewRevision.SourceResultHash == nil {
		return application.StoryCandidateRepairSeed{}, errors.New("Story candidate Review revision has drifted")
	}
	reviewContentHash, err := agentcontract.CanonicalHash(json.RawMessage(reviewRevision.Candidate))
	if err != nil || reviewContentHash != reviewRevision.CandidateContentHash ||
		reviewContentHash != *reviewRevision.SourceResultHash {
		return application.StoryCandidateRepairSeed{}, errors.New("Story candidate Review content hash has drifted")
	}
	reviewBound := false
	for _, upstream := range request.Payload.UpstreamCandidates {
		if upstream.Stage == "review_storygraph" && upstream.CandidateRevisionID == reviewRevision.ID.String() &&
			upstream.CandidateRevisionHash == reviewRevision.CandidateRevisionHash &&
			upstream.SourceInvocationID == reviewRevision.SourceInvocationID.String() &&
			upstream.SourceResultHash == *reviewRevision.SourceResultHash {
			reviewBound = true
			break
		}
	}
	if !reviewBound {
		return application.StoryCandidateRepairSeed{}, errors.New("Story candidate repair lost its exact Review revision")
	}
	repairPatch, err := agentcontract.DecodeCandidateRepairPatch(invocation.Candidate)
	if err != nil || agentcontract.ValidateCandidateRepairPatch(repairInput, repairPatch) != nil {
		return application.StoryCandidateRepairSeed{}, errors.New("Story candidate repair Patch is invalid")
	}
	resultHash, err := agentcontract.CanonicalHash(json.RawMessage(invocation.Candidate))
	if err != nil || resultHash != *invocation.ResultHash {
		return application.StoryCandidateRepairSeed{}, errors.New("Story candidate repair Result hash has drifted")
	}
	return application.StoryCandidateRepairSeed{
		ParentCandidate: append(json.RawMessage(nil), revision.Candidate...),
		RepairInput:     repairInput, RepairPatch: repairPatch, RepairResultHash: resultHash,
	}, nil
}

func storyCandidateStaleClosure(
	database *gorm.DB,
	workspaceID uuid.UUID,
	root application.CandidateRevisionRef,
	appliedRepairInvocationID string,
) ([]application.CandidateStageStaleness, error) {
	var invocations []model.AgentInvocation
	if err := database.Where("workspace_id = ?", workspaceID).Order("stage_instance_key").Find(&invocations).Error; err != nil {
		return nil, err
	}
	var revisions []model.StageCandidateRevision
	if err := database.Where("workspace_id = ?", workspaceID).Order("stage_instance_key").Order("revision_no").Find(&revisions).Error; err != nil {
		return nil, err
	}
	byStage := make(map[string][]application.CandidateRevisionRef)
	for _, revision := range revisions {
		byStage[revision.StageInstanceKey] = append(byStage[revision.StageInstanceKey], application.CandidateRevisionRef{
			ID: revision.ID.String(), Hash: revision.CandidateRevisionHash,
		})
	}
	dependencies := make([]application.CandidateStageDependency, 0, len(invocations))
	dependencyStages := make(map[string]struct{}, len(invocations))
	for _, invocation := range invocations {
		var minimal struct {
			UpstreamCandidates []agentcontract.StageUpstreamCandidateRef `json:"upstream_candidates"`
		}
		if err := json.Unmarshal(invocation.Payload, &minimal); err != nil {
			return nil, errors.New("invalid persisted StoryGraph stage dependency")
		}
		if len(minimal.UpstreamCandidates) == 0 {
			continue
		}
		request, err := agentgorm.StageInvocation(invocation)
		if err != nil {
			return nil, errors.New("invalid persisted StoryGraph stage dependency")
		}
		dependencies = append(dependencies, application.CandidateStageDependency{
			InvocationID: invocation.ID.String(), StageInstanceKey: invocation.StageInstanceKey,
			UpstreamCandidates: request.Payload.UpstreamCandidates,
			CandidateRevisions: append([]application.CandidateRevisionRef(nil), byStage[invocation.StageInstanceKey]...),
		})
		dependencyStages[invocation.StageInstanceKey] = struct{}{}
	}
	for _, revision := range revisions {
		if revision.OriginKind != "aggregate" {
			continue
		}
		if _, exists := dependencyStages[revision.StageInstanceKey]; exists {
			return nil, errors.New("Candidate aggregate stage conflicts with an Invocation dependency")
		}
		var origin agentcontract.AggregateCandidateOrigin
		if err := json.Unmarshal(revision.AggregateOrigin, &origin); err != nil {
			return nil, errors.New("invalid persisted Candidate aggregate dependency")
		}
		computedHash, err := (agentcontract.CandidateRevisionMaterial{
			StageInstanceKey: revision.StageInstanceKey, RevisionNo: revision.RevisionNo,
			OriginKind: "aggregate", AggregateOrigin: &origin,
			CandidateContentHash: revision.CandidateContentHash,
		}).Hash()
		if err != nil || computedHash != revision.CandidateRevisionHash {
			return nil, errors.New("invalid persisted Candidate aggregate dependency")
		}
		upstreams := make([]agentcontract.StageUpstreamCandidateRef, len(origin.LeafCandidates))
		for index, leaf := range origin.LeafCandidates {
			upstreams[index] = agentcontract.StageUpstreamCandidateRef{
				CandidateRevisionID:   leaf.CandidateRevisionID,
				CandidateRevisionHash: leaf.CandidateRevisionHash,
			}
		}
		dependencies = append(dependencies, application.CandidateStageDependency{
			StageInstanceKey: revision.StageInstanceKey, UpstreamCandidates: upstreams,
			CandidateRevisions: append([]application.CandidateRevisionRef(nil), byStage[revision.StageInstanceKey]...),
		})
		dependencyStages[revision.StageInstanceKey] = struct{}{}
	}
	var existing []model.StageInstanceStaleness
	if err := database.Where("workspace_id = ?", workspaceID).Order("stage_instance_key").Find(&existing).Error; err != nil {
		return nil, err
	}
	existingKeys := make([]string, len(existing))
	for index := range existing {
		existingKeys[index] = existing[index].StageInstanceKey
	}
	return application.StoryCandidateStaleClosure(root, appliedRepairInvocationID, existingKeys, dependencies)
}

func storyCandidateRepairConflict(message string) error {
	return &application.Error{Code: "resource_conflict", Message: message, Status: 409}
}

func candidateRepairHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
