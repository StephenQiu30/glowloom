package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func (store *Store) LoadStoryReview(
	ctx context.Context,
	command application.StoryReviewCommand,
) (application.StoryReviewSeed, error) {
	return loadStoryReview(ctx, store.database, command)
}

func loadStoryReview(
	ctx context.Context,
	database *gorm.DB,
	command application.StoryReviewCommand,
) (application.StoryReviewSeed, error) {
	runID, err := uuid.Parse(command.WorkflowRunID)
	if err != nil {
		return application.StoryReviewSeed{}, errors.New("invalid Story review WorkflowRun")
	}
	nodeID, err := uuid.Parse(command.NodeRunID)
	if err != nil {
		return application.StoryReviewSeed{}, errors.New("invalid Story review NodeRun")
	}
	rootRevisionID, err := uuid.Parse(command.CandidateRevisionID)
	if err != nil {
		return application.StoryReviewSeed{}, errors.New("invalid Story review Candidate Revision")
	}
	var run model.WorkflowRun
	if err = database.WithContext(ctx).First(&run, "id = ?", runID).Error; err != nil {
		return application.StoryReviewSeed{}, errors.New("Story review requires an existing WorkflowRun")
	}
	var node model.NodeRunProjection
	if err = database.WithContext(ctx).First(&node, "id = ?", nodeID).Error; err != nil {
		return application.StoryReviewSeed{}, errors.New("Story review requires an existing NodeRun")
	}
	if run.WorkspaceID.String() != command.WorkspaceID || run.ProjectID.String() != command.ProjectID ||
		node.WorkspaceID != run.WorkspaceID || node.WorkflowRunID != run.ID || node.Executor != "activity.story_review" ||
		node.Status == "FAILED" || node.Status == "CANCELLED" || node.Status == "SKIPPED" || node.Status == "CACHED" {
		return application.StoryReviewSeed{}, errors.New("Story review WorkflowRun or NodeRun has drifted")
	}
	if err = authorizeProject(ctx, database, command.Actor, run.ProjectID, true); err != nil {
		return application.StoryReviewSeed{}, err
	}

	var root model.StageCandidateRevision
	if err = database.WithContext(ctx).First(&root, "id = ?", rootRevisionID).Error; err != nil {
		return application.StoryReviewSeed{}, normalizeNotFound(err)
	}
	if root.WorkspaceID != run.WorkspaceID || root.CandidateRevisionHash != command.CandidateRevisionHash {
		return application.StoryReviewSeed{}, errors.New("Story review root Candidate Revision has drifted")
	}
	var head model.StageCandidateHead
	if err = database.WithContext(ctx).First(&head, "stage_instance_key = ?", root.StageInstanceKey).Error; err != nil {
		return application.StoryReviewSeed{}, err
	}
	var current model.StageCandidateRevision
	if err = database.WithContext(ctx).First(&current, "id = ?", head.CurrentRevisionID).Error; err != nil {
		return application.StoryReviewSeed{}, err
	}
	if current.WorkspaceID != run.WorkspaceID || current.StageInstanceKey != root.StageInstanceKey ||
		current.CandidateRevisionHash != head.CurrentCandidateRevisionHash || current.RevisionNo != head.Revision {
		return application.StoryReviewSeed{}, errors.New("Story review Candidate Head has drifted")
	}
	ancestor := current
	for ancestor.ID != root.ID {
		if ancestor.ParentCandidateRevisionID == nil {
			return application.StoryReviewSeed{}, errors.New("Story review Candidate Head is outside the frozen root lineage")
		}
		var parent model.StageCandidateRevision
		if err = database.WithContext(ctx).First(&parent, "id = ?", *ancestor.ParentCandidateRevisionID).Error; err != nil {
			return application.StoryReviewSeed{}, err
		}
		ancestor = parent
	}

	var candidate domain.StoryReconciliationCandidate
	if err = json.Unmarshal(current.Candidate, &candidate); err != nil ||
		domain.ValidateStoryReconciliationCandidate(candidate, domain.StoryReconciliationCandidateEvidence(candidate)) != nil {
		return application.StoryReviewSeed{}, errors.New("Story review current Candidate is invalid")
	}
	if root.SourceInvocationID == nil || root.SourceResultHash == nil {
		return application.StoryReviewSeed{}, errors.New("Story review root Candidate provenance is incomplete")
	}
	var rootInvocation model.AgentInvocation
	if err = database.WithContext(ctx).First(&rootInvocation, "id = ?", *root.SourceInvocationID).Error; err != nil {
		return application.StoryReviewSeed{}, err
	}
	rootRequest, err := agentgorm.StageInvocation(rootInvocation)
	if err != nil || rootInvocation.Stage != domain.ReconcileStoryStage || rootInvocation.Status != "succeeded" ||
		rootInvocation.ResultHash == nil || *rootInvocation.ResultHash != *root.SourceResultHash ||
		len(rootRequest.Payload.SourceRefs) != 1 {
		return application.StoryReviewSeed{}, errors.New("Story review root Candidate provenance has drifted")
	}
	provenanceInvocation, provenanceHash, err := storyReviewCandidateProvenance(database.WithContext(ctx), current)
	if err != nil {
		return application.StoryReviewSeed{}, err
	}
	seed := application.StoryReviewSeed{
		CurrentCandidateRevisionID: current.ID.String(), CurrentCandidateRevisionHash: current.CandidateRevisionHash,
		CurrentCandidateRevisionNo: current.RevisionNo, CurrentStageInstanceKey: current.StageInstanceKey,
		CurrentCandidate: append(json.RawMessage(nil), current.Candidate...), SourceRef: rootRequest.Payload.SourceRefs[0],
		CurrentUpstream: agentcontract.StageUpstreamCandidateRef{
			Stage: domain.ReconcileStoryStage, ShardKey: rootInvocation.ShardKey,
			CandidateRevisionID: current.ID.String(), CandidateRevisionHash: current.CandidateRevisionHash,
			SourceInvocationID: provenanceInvocation.String(), SourceResultHash: provenanceHash,
		},
	}

	var latest model.ShardManifest
	err = database.WithContext(ctx).Where("node_run_id = ? AND stage = ?", nodeID, domain.ReviewStoryGraphStage).
		Order("version DESC").First(&latest).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return application.StoryReviewSeed{}, err
	}
	if err == nil {
		manifest, decodeErr := storyReviewManifestDomain(latest)
		if decodeErr != nil {
			return application.StoryReviewSeed{}, decodeErr
		}
		seed.LatestManifest = &manifest
		if manifest.RootInputHash == current.CandidateRevisionHash {
			var invocations []model.AgentInvocation
			if err = database.WithContext(ctx).
				Where("node_run_id = ? AND shard_manifest_id = ? AND shard_manifest_version = ? AND stage IN ?",
					nodeID, latest.ID, latest.Version, []string{domain.ReviewStoryGraphStage, "repair_candidate"}).
				Order("stage").Find(&invocations).Error; err != nil {
				return application.StoryReviewSeed{}, err
			}
			for _, invocation := range invocations {
				state, stateErr := storyReviewInvocationState(database.WithContext(ctx), invocation)
				if stateErr != nil {
					return application.StoryReviewSeed{}, stateErr
				}
				switch invocation.Stage {
				case domain.ReviewStoryGraphStage:
					if seed.Review != nil {
						return application.StoryReviewSeed{}, errors.New("Story review manifest has duplicate review invocations")
					}
					seed.Review = &state
				case "repair_candidate":
					if seed.Repair != nil {
						return application.StoryReviewSeed{}, errors.New("Story review manifest has duplicate repair invocations")
					}
					seed.Repair = &state
				}
			}
		}
	}
	var repairsUsed int64
	if err = database.WithContext(ctx).Model(&model.AgentInvocation{}).
		Where("node_run_id = ? AND stage = ?", nodeID, "repair_candidate").Count(&repairsUsed).Error; err != nil {
		return application.StoryReviewSeed{}, err
	}
	seed.RepairsUsed = int(repairsUsed)
	return seed, nil
}

func (store *Store) EnsureStoryReviewInvocation(
	ctx context.Context,
	preparation application.StoryReviewPreparation,
) error {
	if err := domain.ValidateStoryReviewManifest(preparation.Manifest); err != nil {
		return err
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		seed, err := loadStoryReview(ctx, transaction, preparation.Command)
		if err != nil {
			return err
		}
		var latest model.ShardManifest
		err = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("node_run_id = ? AND stage = ?", preparation.Command.NodeRunID, domain.ReviewStoryGraphStage).
			Order("version DESC").First(&latest).Error
		if err == nil && latest.RootInputHash == seed.CurrentCandidateRevisionHash {
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if preparation.Manifest.Version != 1 || preparation.Manifest.ParentManifestHash != nil {
				return errors.New("Story review initial manifest lineage has drifted")
			}
		} else if preparation.Manifest.ManifestID != latest.ID.String() ||
			preparation.Manifest.Version != latest.Version+1 || preparation.Manifest.ParentManifestHash == nil ||
			*preparation.Manifest.ParentManifestHash != latest.ManifestHash {
			return errors.New("Story review manifest lineage has drifted")
		}
		if preparation.Manifest.RootInputHash != seed.CurrentCandidateRevisionHash ||
			preparation.Manifest.WorkspaceID != preparation.Command.WorkspaceID ||
			preparation.Manifest.WorkflowRunID != preparation.Command.WorkflowRunID ||
			preparation.Manifest.NodeRunID != preparation.Command.NodeRunID ||
			preparation.Invocation.Stage != domain.ReviewStoryGraphStage ||
			preparation.Invocation.ManifestHash != preparation.Manifest.ManifestHash {
			return errors.New("Story review preparation has drifted")
		}
		record, err := storyReviewManifestRecord(preparation.Manifest, preparation.CreatedAt)
		if err != nil {
			return err
		}
		if err = transaction.Omit(clause.Associations).Create(&record).Error; err != nil {
			return err
		}
		invocation, err := invocationRecord(preparation.Invocation)
		if err != nil {
			return err
		}
		return transaction.Omit(clause.Associations).Create(&invocation).Error
	})
}

func (store *Store) EnsureStoryRepairInvocation(
	ctx context.Context,
	preparation application.StoryRepairPreparation,
) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		seed, err := loadStoryReview(ctx, transaction, preparation.Command)
		if err != nil {
			return err
		}
		if seed.LatestManifest == nil || seed.LatestManifest.ManifestHash != preparation.Manifest.ManifestHash ||
			seed.LatestManifest.RootInputHash != seed.CurrentCandidateRevisionHash || seed.Review == nil ||
			seed.Review.Status != "succeeded" {
			return errors.New("Story repair prerequisites have drifted")
		}
		if seed.Repair != nil {
			return nil
		}
		if seed.RepairsUsed >= preparation.Command.MaxRepairRounds || preparation.Invocation.Stage != "repair_candidate" ||
			preparation.Invocation.ManifestHash != preparation.Manifest.ManifestHash {
			return errors.New("Story repair budget or manifest has drifted")
		}
		record, err := invocationRecord(preparation.Invocation)
		if err != nil {
			return err
		}
		return transaction.Omit(clause.Associations).Create(&record).Error
	})
}

func storyReviewCandidateProvenance(
	database *gorm.DB,
	revision model.StageCandidateRevision,
) (uuid.UUID, string, error) {
	var invocationID uuid.UUID
	var resultHash string
	switch revision.OriginKind {
	case "invocation":
		if revision.SourceInvocationID == nil || revision.SourceResultHash == nil {
			return uuid.Nil, "", errors.New("Story review Candidate invocation provenance is incomplete")
		}
		invocationID, resultHash = *revision.SourceInvocationID, *revision.SourceResultHash
	case "repair":
		var origin agentcontract.RepairCandidateOrigin
		if json.Unmarshal(revision.RepairOrigin, &origin) != nil {
			return uuid.Nil, "", errors.New("Story review Candidate repair provenance is invalid")
		}
		var err error
		invocationID, err = uuid.Parse(origin.RepairInvocationID)
		if err != nil {
			return uuid.Nil, "", err
		}
		resultHash = origin.RepairResultHash
	default:
		return uuid.Nil, "", errors.New("Story review Candidate origin is unsupported")
	}
	var invocation model.AgentInvocation
	if err := database.First(&invocation, "id = ?", invocationID).Error; err != nil {
		return uuid.Nil, "", err
	}
	if invocation.Status != "succeeded" || invocation.ResultHash == nil || *invocation.ResultHash != resultHash {
		return uuid.Nil, "", errors.New("Story review Candidate provenance has drifted")
	}
	return invocationID, resultHash, nil
}

func storyReviewInvocationState(
	database *gorm.DB,
	invocation model.AgentInvocation,
) (application.StoryReviewInvocationState, error) {
	state := application.StoryReviewInvocationState{
		InvocationID: invocation.ID.String(), Status: invocation.Status,
	}
	if len(invocation.Error) > 0 {
		var failure struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(invocation.Error, &failure) != nil {
			return application.StoryReviewInvocationState{}, errors.New("Story review Invocation error has drifted")
		}
		state.FailureCode = failure.Code
	}
	if invocation.Status != "succeeded" {
		return state, nil
	}
	if invocation.ResultHash == nil {
		return application.StoryReviewInvocationState{}, errors.New("Story review successful Invocation has no Result hash")
	}
	var revision model.StageCandidateRevision
	if err := database.Where("source_invocation_id = ?", invocation.ID).First(&revision).Error; err != nil {
		return application.StoryReviewInvocationState{}, err
	}
	if revision.SourceResultHash == nil || *revision.SourceResultHash != *invocation.ResultHash {
		return application.StoryReviewInvocationState{}, errors.New("Story review Candidate Revision provenance has drifted")
	}
	state.ResultHash = *invocation.ResultHash
	state.CandidateRevisionID = revision.ID.String()
	state.CandidateRevisionHash = revision.CandidateRevisionHash
	state.Candidate = append(json.RawMessage(nil), revision.Candidate...)
	return state, nil
}

func storyReviewManifestRecord(value domain.StoryReviewManifest, createdAt time.Time) (model.ShardManifest, error) {
	shards, err := json.Marshal(value.Shards)
	if err != nil {
		return model.ShardManifest{}, err
	}
	return storyManifestRecord(
		value.ManifestID, value.Version, value.WorkspaceID, value.WorkflowRunID, value.NodeRunID,
		value.Stage, value.RootInputHash, value.ParentManifestHash, value.CoverageHash, value.ManifestHash, shards, createdAt,
	)
}

func storyReviewManifestDomain(record model.ShardManifest) (domain.StoryReviewManifest, error) {
	var shards []domain.StoryReviewShard
	if err := json.Unmarshal(record.Shards, &shards); err != nil {
		return domain.StoryReviewManifest{}, err
	}
	value := domain.StoryReviewManifest{
		ManifestID: record.ID.String(), Version: record.Version, ParentManifestHash: record.ParentManifestHash,
		WorkspaceID: record.WorkspaceID.String(), WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(),
		Stage: record.Stage, RootInputHash: record.RootInputHash, Shards: shards,
		CoverageHash: record.CoverageHash, ManifestHash: record.ManifestHash,
	}
	return value, domain.ValidateStoryReviewManifest(value)
}

func (store *Store) ClaimNextStoryReview(
	ctx context.Context,
	now time.Time,
	leaseExpiresAt time.Time,
) (domain.Invocation, bool, error) {
	if !leaseExpiresAt.After(now) {
		return domain.Invocation{}, false, errors.New("Story review invocation lease must expire after claim time")
	}
	var result domain.Invocation
	found := false
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		latestVersion := transaction.Model(&model.ShardManifest{}).Select("MAX(version)").
			Where("node_run_id = agt_invocations.node_run_id AND stage = ?", domain.ReviewStoryGraphStage)
		var record model.AgentInvocation
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("request_type IN ?", []string{"story_review_shard", "story_repair_shard"}).
			Where("kind = ? AND stage IN ?", "storygraph_stage", []string{domain.ReviewStoryGraphStage, "repair_candidate"}).
			Where("shard_manifest_version = (?)", latestVersion).
			Where("status = ? OR status = ? OR (status = ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?))", "queued", "unknown", "running", now).
			Order("created_at").Order("id").First(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = transaction.Model(&record).Updates(map[string]any{
			"status": "running", "attempts": gorm.Expr("attempts + 1"),
			"claim_version": gorm.Expr("claim_version + 1"), "lease_expires_at": leaseExpiresAt,
			"started_at": now, "completed_at": nil, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		record.Status, record.StartedAt, record.Attempts = "running", &now, record.Attempts+1
		record.ClaimVersion, record.LeaseExpiresAt = record.ClaimVersion+1, &leaseExpiresAt
		result, found = invocationDomain(record), true
		return nil
	})
	return result, found, err
}

func (store *Store) ValidateStoryReviewInvocation(
	ctx context.Context,
	invocationID string,
	claimVersion int,
	now time.Time,
) error {
	id, err := uuid.Parse(invocationID)
	if err != nil {
		return application.ErrNotFound
	}
	var invocation model.AgentInvocation
	if err = store.database.WithContext(ctx).First(&invocation, "id = ?", id).Error; err != nil {
		return normalizeNotFound(err)
	}
	if !activeInvocationClaim(invocation, claimVersion, now) {
		return errors.New("Story review invocation claim is stale")
	}
	return validateStoryReviewInvocation(store.database.WithContext(ctx), invocation, false)
}

func validateStoryReviewInvocation(database *gorm.DB, invocation model.AgentInvocation, lock bool) error {
	if invocation.NodeRunID == nil || invocation.ShardManifestID == nil || invocation.ShardManifestVersion == nil {
		return errors.New("Story review invocation has no workflow owner")
	}
	query := database
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var manifest model.ShardManifest
	if err := query.Where("node_run_id = ? AND stage = ?", invocation.NodeRunID, domain.ReviewStoryGraphStage).
		Order("version DESC").First(&manifest).Error; err != nil {
		return err
	}
	if manifest.ID != *invocation.ShardManifestID || manifest.Version != *invocation.ShardManifestVersion ||
		manifest.ManifestHash != invocation.ShardManifestHash {
		return errors.New("Story review manifest was superseded")
	}
	request, err := agentgorm.StageInvocation(invocation)
	if err != nil {
		return err
	}
	switch invocation.Stage {
	case domain.ReviewStoryGraphStage:
		var input agentcontract.StoryGraphReviewStageInput
		if json.Unmarshal(request.Payload.StageInput, &input) != nil || input.Validate() != nil {
			return errors.New("Story review Invocation input is invalid")
		}
		return validateStoryReviewTarget(database, input.TargetCandidateRevisionID, input.TargetCandidateRevisionHash, true)
	case "repair_candidate":
		var input agentcontract.StoryGraphRepairStageInput
		if json.Unmarshal(request.Payload.StageInput, &input) != nil || input.Validate() != nil {
			return errors.New("Story repair Invocation input is invalid")
		}
		if err = validateStoryReviewTarget(database, input.TargetCandidateRevisionID, input.TargetCandidateRevisionHash, true); err != nil {
			return err
		}
		return validateStoryReviewTarget(database, input.ReviewCandidateRevisionID, input.ReviewCandidateRevisionHash, false)
	default:
		return errors.New("unsupported Story review invocation stage")
	}
}

func validateStoryReviewTarget(database *gorm.DB, revisionID, revisionHash string, requireHead bool) error {
	id, err := uuid.Parse(revisionID)
	if err != nil {
		return err
	}
	var revision model.StageCandidateRevision
	if err = database.First(&revision, "id = ?", id).Error; err != nil {
		return err
	}
	if revision.CandidateRevisionHash != revisionHash {
		return errors.New("Story review Candidate Revision has drifted")
	}
	if requireHead {
		var head model.StageCandidateHead
		if err = database.First(&head, "stage_instance_key = ?", revision.StageInstanceKey).Error; err != nil {
			return err
		}
		if head.CurrentRevisionID != revision.ID || head.CurrentCandidateRevisionHash != revision.CandidateRevisionHash {
			return errors.New("Story review Candidate Head has changed")
		}
	}
	return nil
}

func (store *Store) CompleteStoryReviewInvocation(
	ctx context.Context,
	invocationID string,
	claimVersion int,
	result agentcontract.StageResult,
	now time.Time,
) (bool, error) {
	id, err := uuid.Parse(invocationID)
	if err != nil {
		return false, application.ErrNotFound
	}
	executorJSON, err := json.Marshal(result.Executor)
	if err != nil {
		return false, err
	}
	applied := false
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var invocation model.AgentInvocation
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invocation, "id = ?", id).Error; err != nil {
			return normalizeNotFound(err)
		}
		if !activeInvocationClaim(invocation, claimVersion, now) {
			return nil
		}
		if err := validateStoryReviewInvocation(transaction, invocation, true); err != nil {
			return err
		}
		request, err := agentgorm.StageInvocation(invocation)
		if err != nil || result.ValidateFor(request) != nil || result.Status != "succeeded" {
			return errors.New("Story review Agent result is invalid")
		}
		switch invocation.Stage {
		case domain.ReviewStoryGraphStage:
			var input agentcontract.StoryGraphReviewStageInput
			if json.Unmarshal(request.Payload.StageInput, &input) != nil {
				return errors.New("Story review input is invalid")
			}
			candidate, decodeErr := agentcontract.DecodeStoryGraphReviewCandidate(result.Candidate)
			if decodeErr != nil || agentcontract.ValidateStoryGraphReviewCandidate(input, candidate) != nil {
				return errors.New("Story review Candidate is invalid")
			}
		case "repair_candidate":
			var input agentcontract.StoryGraphRepairStageInput
			if json.Unmarshal(request.Payload.StageInput, &input) != nil {
				return errors.New("Story repair input is invalid")
			}
			patch, decodeErr := agentcontract.DecodeCandidateRepairPatch(result.Candidate)
			if decodeErr != nil || agentcontract.ValidateCandidateRepairPatch(input, patch) != nil {
				return errors.New("Story repair Patch is invalid")
			}
		}
		if _, err = agentgorm.AcceptInvocationCandidate(transaction, invocation, request, result, now); err != nil {
			return err
		}
		if err = transaction.Model(&invocation).Updates(map[string]any{
			"status": "succeeded", "result_hash": result.ResultHash, "candidate_type": result.CandidateType,
			"candidate": datatypes.JSON(result.Candidate), "executor": datatypes.JSON(executorJSON), "error": nil,
			"lease_expires_at": nil, "completed_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

func (store *Store) FailStoryReviewInvocation(
	ctx context.Context,
	invocationID string,
	claimVersion int,
	outcome, code, summary string,
	retryable bool,
	now time.Time,
) (bool, error) {
	id, err := uuid.Parse(invocationID)
	if err != nil {
		return false, application.ErrNotFound
	}
	applied := false
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var invocation model.AgentInvocation
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invocation, "id = ?", id).Error; err != nil {
			return normalizeNotFound(err)
		}
		if !activeInvocationClaim(invocation, claimVersion, now) {
			return nil
		}
		if outcome != "failed" && outcome != "unknown" {
			outcome = "unknown"
		}
		errorJSON, err := json.Marshal(map[string]any{"code": code, "summary": summary, "retryable": retryable})
		if err != nil {
			return err
		}
		if err = transaction.Model(&invocation).Updates(map[string]any{
			"status": outcome, "error": datatypes.JSON(errorJSON), "lease_expires_at": nil,
			"completed_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

var _ application.StoryReviewRepository = (*Store)(nil)
