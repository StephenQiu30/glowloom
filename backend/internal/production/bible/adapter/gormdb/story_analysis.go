package gormdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func (store *Store) LoadStoryAnalysisSeed(
	ctx context.Context,
	command application.StoryAnalysisCommand,
) (application.StoryAnalysisSeed, error) {
	return loadStoryAnalysisSeed(ctx, store.database, command)
}

func loadStoryAnalysisSeed(
	ctx context.Context,
	database *gorm.DB,
	command application.StoryAnalysisCommand,
) (application.StoryAnalysisSeed, error) {
	runID, err := uuid.Parse(command.WorkflowRunID)
	if err != nil {
		return application.StoryAnalysisSeed{}, errors.New("invalid Story analysis WorkflowRun")
	}
	nodeID, err := uuid.Parse(command.NodeRunID)
	if err != nil {
		return application.StoryAnalysisSeed{}, errors.New("invalid Story analysis NodeRun")
	}
	var run model.WorkflowRun
	if err = database.WithContext(ctx).First(&run, "id = ?", runID).Error; err != nil {
		return application.StoryAnalysisSeed{}, errors.New("Story analysis requires an existing WorkflowRun")
	}
	var node model.NodeRunProjection
	if err = database.WithContext(ctx).First(&node, "id = ?", nodeID).Error; err != nil {
		return application.StoryAnalysisSeed{}, errors.New("Story analysis requires an existing NodeRun")
	}
	if run.WorkspaceID.String() != command.WorkspaceID || run.ProjectID.String() != command.ProjectID ||
		node.WorkspaceID != run.WorkspaceID || node.WorkflowRunID != run.ID || node.Executor != "activity.story_analysis" ||
		node.Status == "FAILED" || node.Status == "CANCELLED" ||
		node.Status == "SKIPPED" || node.Status == "CACHED" {
		return application.StoryAnalysisSeed{}, errors.New("Story analysis WorkflowRun or NodeRun has drifted")
	}
	aggregateID, err := uuid.Parse(command.EvidenceCandidateRevisionID)
	if err != nil {
		return application.StoryAnalysisSeed{}, errors.New("invalid source Evidence aggregate revision")
	}
	var aggregate model.StageCandidateRevision
	if err = database.WithContext(ctx).First(&aggregate, "id = ?", aggregateID).Error; err != nil {
		return application.StoryAnalysisSeed{}, errors.New("source Evidence aggregate revision does not exist")
	}
	var aggregateHead model.StageCandidateHead
	if err = database.WithContext(ctx).First(&aggregateHead, "stage_instance_key = ?", aggregate.StageInstanceKey).Error; err != nil {
		return application.StoryAnalysisSeed{}, err
	}
	if aggregate.WorkspaceID != run.WorkspaceID || aggregate.OriginKind != "aggregate" ||
		aggregate.CandidateRevisionHash != command.EvidenceCandidateRevisionHash ||
		aggregateHead.CurrentRevisionID != aggregate.ID ||
		aggregateHead.CurrentCandidateRevisionHash != aggregate.CandidateRevisionHash {
		return application.StoryAnalysisSeed{}, application.ErrStoryAnalysisUpstreamStale
	}
	value, err := domain.DecodeSourceEvidenceAggregate(json.RawMessage(aggregate.Candidate))
	if err != nil {
		return application.StoryAnalysisSeed{}, err
	}
	seed := application.StoryAnalysisSeed{
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		RootInputHash: command.EvidenceCandidateRevisionHash,
		Evidence:      make([]application.StoryAnalysisEvidenceSeed, 0, len(value.Fragments)),
	}
	for _, fragment := range value.Fragments {
		revisionID, parseErr := uuid.Parse(fragment.CandidateRevisionID)
		if parseErr != nil {
			return application.StoryAnalysisSeed{}, parseErr
		}
		var revision model.StageCandidateRevision
		if err = database.WithContext(ctx).First(&revision, "id = ?", revisionID).Error; err != nil {
			return application.StoryAnalysisSeed{}, err
		}
		var head model.StageCandidateHead
		if err = database.WithContext(ctx).First(&head, "stage_instance_key = ?", revision.StageInstanceKey).Error; err != nil {
			return application.StoryAnalysisSeed{}, err
		}
		if revision.WorkspaceID != run.WorkspaceID || revision.OriginKind != "invocation" ||
			revision.SourceInvocationID == nil || revision.SourceResultHash == nil ||
			revision.ID.String() != fragment.CandidateRevisionID ||
			revision.CandidateRevisionHash != fragment.CandidateRevisionHash ||
			head.CurrentRevisionID != revision.ID || head.CurrentCandidateRevisionHash != revision.CandidateRevisionHash {
			return application.StoryAnalysisSeed{}, application.ErrStoryAnalysisUpstreamStale
		}
		var invocation model.AgentInvocation
		if err = database.WithContext(ctx).First(&invocation, "id = ?", *revision.SourceInvocationID).Error; err != nil {
			return application.StoryAnalysisSeed{}, err
		}
		request, requestErr := agentgorm.StageInvocation(invocation)
		if requestErr != nil || invocation.Stage != domain.SourceEvidenceStage || invocation.Status != "succeeded" ||
			invocation.ResultHash == nil || *invocation.ResultHash != *revision.SourceResultHash ||
			len(request.Payload.SourceRefs) != 1 || request.Payload.ShardKey != fragment.ShardKey {
			return application.StoryAnalysisSeed{}, errors.New("source Evidence leaf provenance is incomplete")
		}
		fragmentJSON, marshalErr := json.Marshal(fragment.Candidate)
		if marshalErr != nil {
			return application.StoryAnalysisSeed{}, marshalErr
		}
		candidate, candidateErr := strictSourceEvidenceCandidate(fragmentJSON)
		if candidateErr != nil {
			return application.StoryAnalysisSeed{}, candidateErr
		}
		seed.Evidence = append(seed.Evidence, application.StoryAnalysisEvidenceSeed{
			Fragment: domain.StoryAnalysisEvidenceFragment{
				ShardKey: fragment.ShardKey, LogicalStart: fragment.LogicalStart, LogicalEnd: fragment.LogicalEnd,
				CandidateRevisionID:   fragment.CandidateRevisionID,
				CandidateRevisionHash: fragment.CandidateRevisionHash,
			},
			Candidate: candidate, SourceRef: request.Payload.SourceRefs[0],
			Upstream: agentcontract.StageUpstreamCandidateRef{
				Stage: domain.SourceEvidenceStage, ShardKey: fragment.ShardKey,
				CandidateRevisionID:   fragment.CandidateRevisionID,
				CandidateRevisionHash: fragment.CandidateRevisionHash,
				SourceInvocationID:    invocation.ID.String(), SourceResultHash: *invocation.ResultHash,
			},
		})
	}
	return seed, nil
}

func (store *Store) EnsureStoryAnalysis(
	ctx context.Context,
	preparation application.StoryAnalysisPreparation,
) (application.StoryAnalysisState, error) {
	if err := domain.ValidateStoryAnalysisManifests(preparation.AnalyzeManifest, preparation.ReconcileManifest); err != nil {
		return application.StoryAnalysisState{}, err
	}
	var state application.StoryAnalysisState
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		seed, err := loadStoryAnalysisSeed(ctx, transaction, preparation.Command)
		if err != nil {
			return err
		}
		if seed.RootInputHash != preparation.AnalyzeManifest.RootInputHash || seed.NodeRunID != preparation.AnalyzeManifest.NodeRunID {
			return errors.New("Story analysis source changed before persistence")
		}
		var existing []model.ShardManifest
		if err = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("node_run_id = ? AND stage IN ?", preparation.Command.NodeRunID,
				[]string{domain.AnalyzeStoryStage, domain.ReconcileStoryStage}).
			Order("stage").Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) == 0 {
			analyzeRecord, recordErr := storyAnalysisManifestRecord(preparation.AnalyzeManifest, preparation.CreatedAt)
			if recordErr != nil {
				return recordErr
			}
			reconcileRecord, recordErr := storyReconcileManifestRecord(preparation.ReconcileManifest, preparation.CreatedAt)
			if recordErr != nil {
				return recordErr
			}
			if err = transaction.Omit(clause.Associations).Create(&analyzeRecord).Error; err != nil {
				return err
			}
			if err = transaction.Omit(clause.Associations).Create(&reconcileRecord).Error; err != nil {
				return err
			}
			if len(preparation.Invocations) != len(preparation.AnalyzeManifest.Shards) {
				return errors.New("Story analysis preparation has incomplete map invocations")
			}
			for _, invocation := range preparation.Invocations {
				if invocation.ManifestID != preparation.AnalyzeManifest.ManifestID ||
					invocation.ManifestHash != preparation.AnalyzeManifest.ManifestHash ||
					invocation.Stage != domain.AnalyzeStoryStage {
					return errors.New("Story analysis invocation does not belong to its manifest")
				}
				record, recordErr := invocationRecord(invocation)
				if recordErr != nil {
					return recordErr
				}
				if err = transaction.Omit(clause.Associations).Create(&record).Error; err != nil {
					return err
				}
			}
		} else {
			foundAnalyze, foundReconcile := false, false
			for _, record := range existing {
				if record.Version != 1 {
					continue
				}
				switch record.Stage {
				case domain.AnalyzeStoryStage:
					value, decodeErr := storyAnalysisManifestDomain(record)
					if decodeErr != nil || value.ManifestHash != preparation.AnalyzeManifest.ManifestHash {
						return errors.New("Story analysis manifest changed for the existing NodeRun")
					}
					foundAnalyze = true
				case domain.ReconcileStoryStage:
					value, decodeErr := storyReconcileManifestDomain(record)
					if decodeErr != nil || value.ManifestHash != preparation.ReconcileManifest.ManifestHash {
						return errors.New("Story reconcile manifest changed for the existing NodeRun")
					}
					foundReconcile = true
				default:
					return errors.New("unexpected Story analysis manifest stage")
				}
			}
			if !foundAnalyze || !foundReconcile {
				return errors.New("Story analysis initial manifest pair is incomplete")
			}
		}
		persistedAnalyze, persisted, err := loadLatestStoryManifestPair(transaction, preparation.Command.NodeRunID, true)
		if err != nil {
			return err
		}
		if err = domain.ValidateStoryAnalysisManifests(persistedAnalyze, persisted); err != nil {
			return err
		}
		if err = scheduleStoryReconcile(transaction, persisted, preparation.CreatedAt); err != nil {
			return err
		}
		state, err = storyAnalysisState(transaction, persisted)
		return err
	})
	return state, err
}

func (store *Store) RecoverStoryAnalysis(
	ctx context.Context,
	actor application.Actor,
	preparation application.StoryAnalysisRecoveryPreparation,
) (application.StoryAnalysisRecovery, error) {
	runID, err := uuid.Parse(preparation.Command.WorkflowRunID)
	if err != nil {
		return application.StoryAnalysisRecovery{}, application.ErrNotFound
	}
	nodeRunID, err := uuid.Parse(preparation.Command.NodeRunID)
	if err != nil {
		return application.StoryAnalysisRecovery{}, application.ErrNotFound
	}
	var recovered application.StoryAnalysisRecovery
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var run model.WorkflowRun
		if loadErr := transaction.First(&run, "id = ?", runID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var node model.NodeRunProjection
		if loadErr := transaction.First(&node, "id = ?", nodeRunID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if node.WorkflowRunID != run.ID || node.WorkspaceID != run.WorkspaceID ||
			node.Executor != "activity.story_analysis" {
			return application.ErrNotFound
		}
		if err = authorizeProject(ctx, transaction, actor, run.ProjectID, true); err != nil {
			return err
		}
		repository := &repository{database: transaction}
		receipt, receiptErr := repository.FindReceipt(
			ctx, run.WorkspaceID.String(), application.StoryAnalysisRecoveryOperation,
			preparation.Command.IdempotencyKey,
		)
		if receiptErr == nil {
			replayed, replayErr := platformcommand.Replay[application.StoryAnalysisRecovery](receipt, preparation.InputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return &application.Error{
					Code: "resource_conflict", Message: "Idempotency key was already used with different Story analysis recovery input", Status: 409,
				}
			}
			if replayErr != nil {
				return replayErr
			}
			recovered = replayed
			return nil
		}
		if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		analyze, reconcile, loadErr := loadLatestStoryManifestPair(transaction, node.ID.String(), true)
		if loadErr != nil {
			return loadErr
		}
		if validateErr := domain.ValidateStoryAnalysisManifests(analyze, reconcile); validateErr != nil {
			return validateErr
		}
		if loadErr = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", runID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if loadErr = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "id = ?", nodeRunID).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		if node.WorkflowRunID != run.ID || node.WorkspaceID != run.WorkspaceID ||
			node.Executor != "activity.story_analysis" {
			return application.ErrNotFound
		}
		if (run.Status != "RUNNING" && run.Status != "RETRYING") ||
			(node.Status != "RUNNING" && node.Status != "RETRYING") {
			return &application.Error{
				Code: "resource_conflict", Message: "Story analysis NodeRun is not recoverable", Status: 409,
			}
		}
		var failures []model.AgentInvocation
		if loadErr = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("node_run_id = ? AND stage IN ? AND status = ?", node.ID,
				[]string{domain.AnalyzeStoryStage, domain.ReconcileStoryStage}, "failed").
			Order("updated_at").Order("id").Find(&failures).Error; loadErr != nil {
			return loadErr
		}
		var target *model.AgentInvocation
		for index := range failures {
			invocation := &failures[index]
			if !recoverableCurrentStoryInvocation(*invocation, analyze, reconcile) {
				continue
			}
			var failure struct {
				Code string `json:"code"`
			}
			if json.Unmarshal(invocation.Error, &failure) != nil || failure.Code != "execution_deadline_exceeded" {
				continue
			}
			if target != nil {
				return &application.Error{
					Code: "resource_conflict", Message: "Story analysis NodeRun has multiple recoverable deadline failures", Status: 409,
				}
			}
			target = invocation
		}
		if target == nil {
			return &application.Error{
				Code: "resource_conflict", Message: "Story analysis NodeRun has no recoverable deadline failure", Status: 409,
			}
		}
		recovered = application.StoryAnalysisRecovery{
			ReceiptID: preparation.ReceiptID, WorkflowRunID: run.ID.String(), NodeRunID: node.ID.String(),
			InvocationID: target.ID.String(), Stage: target.Stage, ShardKey: target.ShardKey,
			Status: "queued", FailureCode: "execution_deadline_exceeded",
			PreviousClaimVersion: target.ClaimVersion,
		}
		updated := transaction.Model(&model.AgentInvocation{}).
			Where("id = ? AND status = ? AND claim_version = ?", target.ID, "failed", target.ClaimVersion).
			Updates(map[string]any{
				"status": "queued", "error": nil, "lease_expires_at": nil,
				"started_at": nil, "completed_at": nil, "updated_at": preparation.CreatedAt,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return &application.Error{
				Code: "resource_conflict", Message: "Story analysis failure changed before recovery", Status: 409,
			}
		}
		result, resultErr := platformcommand.Result(recovered)
		if resultErr != nil {
			return resultErr
		}
		return repository.CreateReceipt(ctx, platformcommand.Receipt{
			ID: preparation.ReceiptID, WorkspaceID: run.WorkspaceID.String(),
			Operation:      application.StoryAnalysisRecoveryOperation,
			IdempotencyKey: preparation.Command.IdempotencyKey, InputHash: preparation.InputHash,
			ResourceID: target.ID.String(), Result: result,
			CreatedBy: actor.UserID, CreatedAt: preparation.CreatedAt,
		})
	})
	return recovered, err
}

func recoverableCurrentStoryInvocation(
	invocation model.AgentInvocation,
	analyze domain.StoryAnalysisManifest,
	reconcile domain.StoryReconcileManifest,
) bool {
	if invocation.ShardManifestID == nil || invocation.ShardManifestVersion == nil {
		return false
	}
	switch invocation.Stage {
	case domain.AnalyzeStoryStage:
		if invocation.ShardManifestID.String() != analyze.ManifestID ||
			*invocation.ShardManifestVersion != analyze.Version || invocation.ShardManifestHash != analyze.ManifestHash {
			return false
		}
		for _, shard := range analyze.Shards {
			if shard.Key == invocation.ShardKey {
				return shard.Status == "active"
			}
		}
	case domain.ReconcileStoryStage:
		if invocation.ShardManifestID.String() != reconcile.ManifestID ||
			*invocation.ShardManifestVersion != reconcile.Version || invocation.ShardManifestHash != reconcile.ManifestHash {
			return false
		}
		for _, shard := range reconcile.Shards {
			if shard.Key == invocation.ShardKey {
				return shard.Status == "active"
			}
		}
	}
	return false
}

func (store *Store) ClaimNextStoryAnalysis(
	ctx context.Context,
	now time.Time,
	leaseExpiresAt time.Time,
) (domain.Invocation, bool, error) {
	if !leaseExpiresAt.After(now) {
		return domain.Invocation{}, false, errors.New("Story analysis invocation lease must expire after claim time")
	}
	var result domain.Invocation
	found := false
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		latestManifestVersion := transaction.Model(&model.ShardManifest{}).
			Select("MAX(version)").Where("node_run_id = agt_invocations.node_run_id").Where("stage = agt_invocations.stage")
		var record model.AgentInvocation
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("request_type IN ?", []string{"story_analysis_shard", "story_reconcile_shard"}).
			Where("kind = ? AND stage IN ?", "storygraph_stage", []string{domain.AnalyzeStoryStage, domain.ReconcileStoryStage}).
			Where("shard_manifest_version = (?)", latestManifestVersion).
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

func (store *Store) ValidateStoryAnalysisInvocation(
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
		return errors.New("Story analysis invocation claim is stale")
	}
	if err = validateCurrentStoryManifest(store.database.WithContext(ctx), invocation, false); err != nil {
		return err
	}
	request, err := agentgorm.StageInvocation(invocation)
	if err != nil {
		return err
	}
	if err = agentcontract.ValidateStoryAnalysisInvocation(request); err != nil {
		return err
	}
	return validateStoryUpstreams(store.database.WithContext(ctx), request)
}

func (store *Store) CompleteStoryAnalysisInvocation(
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
		if err := validateCurrentStoryManifest(transaction, invocation, true); err != nil {
			if errors.Is(err, application.ErrStoryAnalysisManifestStale) {
				applied = true
				return failStoryAnalysisInvocation(
					transaction, invocation, "failed", "manifest_superseded",
					"A newer Story analysis manifest replaced this invocation", false, now,
				)
			}
			return err
		}
		request, err := agentgorm.StageInvocation(invocation)
		if err != nil {
			return err
		}
		if err = agentcontract.ValidateStoryAnalysisInvocation(request); err != nil {
			return err
		}
		if err = validateStoryUpstreams(transaction, request); err != nil {
			if errors.Is(err, application.ErrStoryAnalysisUpstreamStale) {
				applied = true
				return failStoryAnalysisInvocation(transaction, invocation, "failed", "upstream_candidate_stale", err.Error(), false, now)
			}
			return err
		}
		material, err := loadStoryInvocationMaterial(request)
		if err != nil {
			return err
		}
		switch invocation.Stage {
		case domain.AnalyzeStoryStage:
			_, err = domain.DecodeStoryAnalysisCandidate(result.Candidate, material.Evidence)
		case domain.ReconcileStoryStage:
			var candidate domain.StoryReconciliationCandidate
			candidate, err = domain.DecodeStoryReconciliationCandidate(result.Candidate, material.Evidence)
			if err == nil {
				err = domain.ValidateStoryReconciliationConservation(
					candidate, material.AnalysisCandidates, material.ReconciliationCandidates,
				)
			}
		default:
			err = errors.New("unsupported Story analysis stage")
		}
		if err != nil {
			code := "candidate_schema_invalid"
			if strings.Contains(err.Error(), "Evidence") {
				code = "evidence_invalid"
			}
			applied = true
			return failStoryAnalysisInvocation(transaction, invocation, "failed", code, err.Error(), false, now)
		}
		if _, err = agentgorm.AcceptInvocationCandidate(transaction, invocation, request, result, now); err != nil {
			return err
		}
		if err = transaction.Model(&invocation).Updates(map[string]any{
			"status": "succeeded", "result_hash": result.ResultHash,
			"candidate_type": result.CandidateType, "candidate": datatypes.JSON(result.Candidate),
			"executor": datatypes.JSON(executorJSON), "error": nil, "lease_expires_at": nil,
			"completed_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if invocation.NodeRunID == nil {
			return errors.New("Story analysis invocation has no NodeRun")
		}
		manifest, err := loadStoryReconcileManifest(transaction, invocation.NodeRunID.String())
		if err != nil {
			return err
		}
		if err = scheduleStoryReconcile(transaction, manifest, now); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

func (store *Store) LoadStoryAnalysisReshardSeed(
	ctx context.Context,
	invocationID string,
	claimVersion int,
	now time.Time,
) (application.StoryAnalysisReshardSeed, error) {
	id, err := uuid.Parse(invocationID)
	if err != nil {
		return application.StoryAnalysisReshardSeed{}, application.ErrNotFound
	}
	var seed application.StoryAnalysisReshardSeed
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var invocation model.AgentInvocation
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invocation, "id = ?", id).Error; err != nil {
			return normalizeNotFound(err)
		}
		if !activeInvocationClaim(invocation, claimVersion, now) {
			return errors.New("Story analysis reshard claim is stale")
		}
		if invocation.NodeRunID == nil || invocation.WorkflowRunID == nil {
			return errors.New("Story analysis reshard invocation has no workflow owner")
		}
		request, err := agentgorm.StageInvocation(invocation)
		if err != nil {
			return err
		}
		analyze, reconcile, err := loadLatestStoryManifestPair(transaction, invocation.NodeRunID.String(), true)
		if err != nil {
			return err
		}
		if err = domain.ValidateStoryAnalysisManifests(analyze, reconcile); err != nil {
			return err
		}
		if err = validateCurrentStoryManifest(transaction, invocation, false); err != nil {
			return err
		}
		active := false
		if invocation.Stage == domain.AnalyzeStoryStage {
			for _, shard := range analyze.Shards {
				active = active || shard.Key == invocation.ShardKey && shard.Status == "active"
			}
		} else if invocation.Stage == domain.ReconcileStoryStage {
			for _, shard := range reconcile.Shards {
				active = active || shard.Key == invocation.ShardKey && shard.Status == "active"
			}
		}
		if !active || request.Payload.ShardManifestRef.Hash != invocation.ShardManifestHash {
			return application.ErrStoryAnalysisManifestStale
		}
		var run model.WorkflowRun
		if err = transaction.First(&run, "id = ?", invocation.WorkflowRunID).Error; err != nil {
			return err
		}
		seed = application.StoryAnalysisReshardSeed{
			Stage: invocation.Stage, ShardKey: invocation.ShardKey, ProjectID: run.ProjectID.String(),
			AnalyzeManifest: analyze, ReconcileManifest: reconcile,
		}
		if invocation.Stage == domain.AnalyzeStoryStage {
			var aggregate model.StageCandidateRevision
			if err = transaction.Where("workspace_id = ? AND origin_kind = ? AND candidate_revision_hash = ?",
				invocation.WorkspaceID, "aggregate", analyze.RootInputHash).First(&aggregate).Error; err != nil {
				return err
			}
			loaded, loadErr := loadStoryAnalysisSeed(ctx, transaction, application.StoryAnalysisCommand{
				WorkspaceID: invocation.WorkspaceID.String(), ProjectID: run.ProjectID.String(),
				WorkflowRunID: invocation.WorkflowRunID.String(), NodeRunID: invocation.NodeRunID.String(),
				EvidenceCandidateRevisionID:   aggregate.ID.String(),
				EvidenceCandidateRevisionHash: aggregate.CandidateRevisionHash,
			})
			if loadErr != nil {
				return loadErr
			}
			seed.Evidence = loaded.Evidence
			return nil
		}
		var input agentcontract.StoryReconciliationStageInput
		if err = json.Unmarshal(request.Payload.StageInput, &input); err != nil {
			return err
		}
		seed.ReconcileCandidateSizes = make([]domain.StoryReconcileCandidateSize, len(input.Candidates))
		for index, child := range input.Candidates {
			revisionID, parseErr := uuid.Parse(child.CandidateRevisionID)
			if parseErr != nil {
				return parseErr
			}
			var revision model.StageCandidateRevision
			if err = transaction.First(&revision, "id = ?", revisionID).Error; err != nil {
				return err
			}
			if revision.CandidateRevisionHash != child.CandidateRevisionHash {
				return application.ErrStoryAnalysisUpstreamStale
			}
			itemCount := 0
			switch input.CandidateType {
			case "story_analysis_candidate":
				var candidate domain.StoryAnalysisCandidate
				if err = json.Unmarshal(revision.Candidate, &candidate); err != nil {
					return err
				}
				itemCount = domain.StoryAnalysisCandidateItemCount(candidate)
			case "story_reconciliation_candidate":
				var candidate domain.StoryReconciliationCandidate
				if err = json.Unmarshal(revision.Candidate, &candidate); err != nil {
					return err
				}
				itemCount = domain.StoryReconciliationCandidateItemCount(candidate)
			default:
				return errors.New("unsupported Story reconcile candidate type")
			}
			seed.ReconcileCandidateSizes[index] = domain.StoryReconcileCandidateSize{
				Stage:    request.Payload.UpstreamCandidates[index].Stage,
				ShardKey: child.ShardKey, ItemCount: itemCount,
			}
		}
		return nil
	})
	return seed, err
}

func (store *Store) ApplyStoryAnalysisReshard(
	ctx context.Context,
	preparation application.StoryAnalysisReshardPreparation,
) (bool, error) {
	id, err := uuid.Parse(preparation.InvocationID)
	if err != nil {
		return false, application.ErrNotFound
	}
	applied := false
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var invocation model.AgentInvocation
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invocation, "id = ?", id).Error; err != nil {
			return normalizeNotFound(err)
		}
		if !activeInvocationClaim(invocation, preparation.ClaimVersion, preparation.CreatedAt) {
			return nil
		}
		currentAnalyze, currentReconcile, err := loadLatestStoryManifestPair(transaction, invocation.NodeRunID.String(), true)
		if err != nil {
			return err
		}
		if currentAnalyze.ManifestHash != preparation.PreviousAnalyzeManifestHash ||
			currentReconcile.ManifestHash != preparation.PreviousReconcileManifestHash {
			return errors.New("Story analysis reshard lineage has drifted")
		}
		nextAnalyze := currentAnalyze
		if preparation.AnalyzeManifest != nil {
			nextAnalyze = *preparation.AnalyzeManifest
			if nextAnalyze.ManifestID != currentAnalyze.ManifestID || nextAnalyze.Version != currentAnalyze.Version+1 ||
				nextAnalyze.ParentManifestHash == nil || *nextAnalyze.ParentManifestHash != currentAnalyze.ManifestHash {
				return errors.New("Story analysis map reshard lineage has drifted")
			}
		}
		nextReconcile := preparation.ReconcileManifest
		if nextReconcile.ManifestID != currentReconcile.ManifestID || nextReconcile.Version != currentReconcile.Version+1 ||
			nextReconcile.ParentManifestHash == nil || *nextReconcile.ParentManifestHash != currentReconcile.ManifestHash {
			return errors.New("Story reconcile reshard lineage has drifted")
		}
		if err = domain.ValidateStoryAnalysisManifests(nextAnalyze, nextReconcile); err != nil {
			return err
		}
		if preparation.AnalyzeManifest != nil {
			record, recordErr := storyAnalysisManifestRecord(nextAnalyze, preparation.CreatedAt)
			if recordErr != nil {
				return recordErr
			}
			if err = transaction.Omit(clause.Associations).Create(&record).Error; err != nil {
				return err
			}
		}
		reconcileRecord, err := storyReconcileManifestRecord(nextReconcile, preparation.CreatedAt)
		if err != nil {
			return err
		}
		if err = transaction.Omit(clause.Associations).Create(&reconcileRecord).Error; err != nil {
			return err
		}
		for _, value := range preparation.Invocations {
			if preparation.AnalyzeManifest == nil || value.ManifestID != nextAnalyze.ManifestID ||
				value.ManifestVersion != nextAnalyze.Version || value.ManifestHash != nextAnalyze.ManifestHash ||
				value.Stage != domain.AnalyzeStoryStage {
				return errors.New("Story analysis reshard invocation does not belong to the current map manifest")
			}
			var succeeded model.AgentInvocation
			lookupErr := transaction.Where("node_run_id = ? AND stage = ? AND shard_key = ? AND status = ?",
				nextAnalyze.NodeRunID, domain.AnalyzeStoryStage, value.ShardKey, "succeeded").First(&succeeded).Error
			if lookupErr == nil {
				continue
			}
			if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return lookupErr
			}
			record, recordErr := invocationRecord(value)
			if recordErr != nil {
				return recordErr
			}
			if err = transaction.Omit(clause.Associations).Create(&record).Error; err != nil {
				return err
			}
		}
		errorJSON, err := json.Marshal(map[string]any{
			"code": preparation.ErrorCode, "summary": preparation.ErrorSummary, "retryable": false,
		})
		if err != nil {
			return err
		}
		if err = transaction.Model(&invocation).Updates(map[string]any{
			"status": "failed", "error": datatypes.JSON(errorJSON), "lease_expires_at": nil,
			"completed_at": preparation.CreatedAt, "updated_at": preparation.CreatedAt,
		}).Error; err != nil {
			return err
		}
		supersededJSON, err := json.Marshal(map[string]any{
			"code": "manifest_superseded", "summary": "A newer Story analysis manifest replaced this pending invocation", "retryable": false,
		})
		if err != nil {
			return err
		}
		if preparation.AnalyzeManifest != nil {
			if err = transaction.Model(&model.AgentInvocation{}).
				Where("node_run_id = ? AND stage = ? AND shard_manifest_version = ? AND status IN ?",
					invocation.NodeRunID, domain.AnalyzeStoryStage, currentAnalyze.Version, []string{"queued", "unknown"}).
				Updates(map[string]any{"status": "failed", "error": datatypes.JSON(supersededJSON),
					"completed_at": preparation.CreatedAt, "updated_at": preparation.CreatedAt}).Error; err != nil {
				return err
			}
		}
		if err = transaction.Model(&model.AgentInvocation{}).
			Where("node_run_id = ? AND stage = ? AND shard_manifest_version = ? AND status IN ?",
				invocation.NodeRunID, domain.ReconcileStoryStage, currentReconcile.Version, []string{"queued", "unknown"}).
			Updates(map[string]any{"status": "failed", "error": datatypes.JSON(supersededJSON),
				"completed_at": preparation.CreatedAt, "updated_at": preparation.CreatedAt}).Error; err != nil {
			return err
		}
		if err = scheduleStoryReconcile(transaction, nextReconcile, preparation.CreatedAt); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

func (store *Store) FailStoryAnalysisInvocation(
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
		if err := failStoryAnalysisInvocation(transaction, invocation, outcome, code, summary, retryable, now); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

func failStoryAnalysisInvocation(database *gorm.DB, invocation model.AgentInvocation, outcome, code, summary string, retryable bool, now time.Time) error {
	if outcome != "failed" && outcome != "unknown" {
		outcome = "unknown"
	}
	errorJSON, err := json.Marshal(map[string]any{"code": code, "summary": summary, "retryable": retryable})
	if err != nil {
		return err
	}
	return database.Model(&invocation).Updates(map[string]any{
		"status": outcome, "error": datatypes.JSON(errorJSON), "lease_expires_at": nil,
		"completed_at": now, "updated_at": now,
	}).Error
}

func validateStoryUpstreams(database *gorm.DB, request agentcontract.StageInvocation) error {
	for _, upstream := range request.Payload.UpstreamCandidates {
		revisionID, err := uuid.Parse(upstream.CandidateRevisionID)
		if err != nil {
			return err
		}
		var revision model.StageCandidateRevision
		if err = database.First(&revision, "id = ?", revisionID).Error; err != nil {
			return application.ErrStoryAnalysisUpstreamStale
		}
		var head model.StageCandidateHead
		if err = database.First(&head, "stage_instance_key = ?", revision.StageInstanceKey).Error; err != nil {
			return application.ErrStoryAnalysisUpstreamStale
		}
		if revision.CandidateRevisionHash != upstream.CandidateRevisionHash ||
			revision.SourceInvocationID == nil || revision.SourceResultHash == nil ||
			revision.SourceInvocationID.String() != upstream.SourceInvocationID ||
			*revision.SourceResultHash != upstream.SourceResultHash || head.CurrentRevisionID != revision.ID ||
			head.CurrentCandidateRevisionHash != revision.CandidateRevisionHash {
			return application.ErrStoryAnalysisUpstreamStale
		}
	}
	return nil
}

type storyInvocationMaterial struct {
	Evidence                 []domain.Evidence
	AnalysisCandidates       []domain.StoryAnalysisCandidate
	ReconciliationCandidates []domain.StoryReconciliationCandidate
}

func loadStoryInvocationMaterial(request agentcontract.StageInvocation) (storyInvocationMaterial, error) {
	switch request.Payload.Stage {
	case domain.AnalyzeStoryStage:
		var input agentcontract.StoryAnalysisStageInput
		if err := json.Unmarshal(request.Payload.StageInput, &input); err != nil {
			return storyInvocationMaterial{}, err
		}
		candidate, err := strictSourceEvidenceCandidate(input.EvidenceCandidate)
		if err != nil {
			return storyInvocationMaterial{}, err
		}
		return storyInvocationMaterial{Evidence: domain.SourceEvidenceCandidateEvidence(candidate)}, nil
	case domain.ReconcileStoryStage:
		var input agentcontract.StoryReconciliationStageInput
		if err := json.Unmarshal(request.Payload.StageInput, &input); err != nil {
			return storyInvocationMaterial{}, err
		}
		result := storyInvocationMaterial{
			Evidence: []domain.Evidence{}, AnalysisCandidates: []domain.StoryAnalysisCandidate{},
			ReconciliationCandidates: []domain.StoryReconciliationCandidate{},
		}
		for _, child := range input.Candidates {
			switch input.CandidateType {
			case "story_analysis_candidate":
				var candidate domain.StoryAnalysisCandidate
				if err := json.Unmarshal(child.Candidate, &candidate); err != nil {
					return storyInvocationMaterial{}, err
				}
				validated, err := domain.DecodeStoryAnalysisCandidate(child.Candidate, domain.StoryAnalysisCandidateEvidence(candidate))
				if err != nil {
					return storyInvocationMaterial{}, err
				}
				result.AnalysisCandidates = append(result.AnalysisCandidates, validated)
				result.Evidence = append(result.Evidence, domain.StoryAnalysisCandidateEvidence(validated)...)
			case "story_reconciliation_candidate":
				var candidate domain.StoryReconciliationCandidate
				if err := json.Unmarshal(child.Candidate, &candidate); err != nil {
					return storyInvocationMaterial{}, err
				}
				validated, err := domain.DecodeStoryReconciliationCandidate(child.Candidate, domain.StoryReconciliationCandidateEvidence(candidate))
				if err != nil {
					return storyInvocationMaterial{}, err
				}
				result.ReconciliationCandidates = append(result.ReconciliationCandidates, validated)
				result.Evidence = append(result.Evidence, domain.StoryReconciliationCandidateEvidence(validated)...)
			default:
				return storyInvocationMaterial{}, errors.New("invalid Story reconcile candidate type")
			}
		}
		return result, nil
	default:
		return storyInvocationMaterial{}, errors.New("unsupported Story analysis stage")
	}
}

func scheduleStoryReconcile(database *gorm.DB, manifest domain.StoryReconcileManifest, now time.Time) error {
	manifestVersions := map[string]int64{domain.ReconcileStoryStage: manifest.Version}
	shards := append([]domain.StoryReconcileShard(nil), manifest.Shards...)
	slices.SortFunc(shards, func(left, right domain.StoryReconcileShard) int {
		if left.Level != right.Level {
			return left.Level - right.Level
		}
		return stringsCompare(left.Key, right.Key)
	})
	for _, shard := range shards {
		if shard.Status != "active" {
			continue
		}
		var existing model.AgentInvocation
		err := database.Where("node_run_id = ? AND stage = ? AND shard_key = ? AND shard_manifest_version = ?",
			manifest.NodeRunID, domain.ReconcileStoryStage, shard.Key, manifest.Version).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		err = database.Where("node_run_id = ? AND stage = ? AND shard_key = ? AND status = ?",
			manifest.NodeRunID, domain.ReconcileStoryStage, shard.Key, "succeeded").
			Order("shard_manifest_version DESC").First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		children := make([]storyReconcileReadyChild, 0, len(shard.Children))
		ready := true
		for _, child := range shard.Children {
			version, exists := manifestVersions[child.Stage]
			if !exists {
				var childManifest model.ShardManifest
				err = database.Where("node_run_id = ? AND stage = ?", manifest.NodeRunID, child.Stage).
					Order("version DESC").First(&childManifest).Error
				if err != nil {
					return err
				}
				version = childManifest.Version
				manifestVersions[child.Stage] = version
			}
			invocation, found, lookupErr := storyReconcileChildInvocation(
				database, manifest.NodeRunID, child.Stage, child.ShardKey, version,
			)
			if lookupErr != nil {
				return lookupErr
			}
			if !found {
				ready = false
				break
			}
			var revision model.StageCandidateRevision
			if err = database.First(&revision, "source_invocation_id = ?", invocation.ID).Error; err != nil {
				return err
			}
			var head model.StageCandidateHead
			if err = database.First(&head, "stage_instance_key = ?", revision.StageInstanceKey).Error; err != nil {
				return err
			}
			if head.CurrentRevisionID != revision.ID || head.CurrentCandidateRevisionHash != revision.CandidateRevisionHash {
				return application.ErrStoryAnalysisUpstreamStale
			}
			request, requestErr := agentgorm.StageInvocation(invocation)
			if requestErr != nil || len(request.Payload.SourceRefs) != 1 {
				return errors.New("Story reconcile child provenance is incomplete")
			}
			children = append(children, storyReconcileReadyChild{
				Invocation: invocation, Revision: revision, SourceRef: request.Payload.SourceRefs[0],
				ProjectID: request.Payload.ProjectID,
			})
		}
		if !ready {
			continue
		}
		record, err := storyReconcileInvocationRecord(manifest, shard, children, now)
		if err != nil {
			return err
		}
		created := database.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if err = created.Error; err != nil {
			return err
		}
		if created.RowsAffected == 0 {
			var concurrent model.AgentInvocation
			if err = database.First(&concurrent, "id = ?", record.ID).Error; err != nil {
				return err
			}
			if !sameStoryReconcileInvocationIdentity(concurrent, record) {
				return errors.New("concurrent Story reconcile invocation identity conflicts with deterministic schedule")
			}
		}
	}
	return nil
}

func storyReconcileChildInvocation(
	database *gorm.DB,
	nodeRunID, stage, shardKey string,
	currentManifestVersion int64,
) (model.AgentInvocation, bool, error) {
	var current model.AgentInvocation
	err := database.Where("node_run_id = ? AND stage = ? AND shard_key = ? AND shard_manifest_version = ?",
		nodeRunID, stage, shardKey, currentManifestVersion).First(&current).Error
	if err == nil {
		return current, current.Status == "succeeded" && current.ResultHash != nil, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AgentInvocation{}, false, err
	}
	var reusable model.AgentInvocation
	err = database.Where("node_run_id = ? AND stage = ? AND shard_key = ? AND status = ?",
		nodeRunID, stage, shardKey, "succeeded").
		Where("shard_manifest_version < ?", currentManifestVersion).
		Order("shard_manifest_version DESC").First(&reusable).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AgentInvocation{}, false, nil
	}
	if err != nil {
		return model.AgentInvocation{}, false, err
	}
	return reusable, reusable.ResultHash != nil, nil
}

func sameStoryReconcileInvocationIdentity(left, right model.AgentInvocation) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID &&
		sameUUIDPointer(left.WorkflowRunID, right.WorkflowRunID) && sameUUIDPointer(left.NodeRunID, right.NodeRunID) &&
		sameUUIDPointer(left.ShardManifestID, right.ShardManifestID) &&
		sameInt64Pointer(left.ShardManifestVersion, right.ShardManifestVersion) &&
		left.RequestType == right.RequestType && left.RequestID == right.RequestID && left.Kind == right.Kind &&
		left.WireSchemaVersion == right.WireSchemaVersion && left.Stage == right.Stage && left.ShardKey == right.ShardKey &&
		left.StageInstanceKey == right.StageInstanceKey && left.ShardManifestHash == right.ShardManifestHash &&
		left.InputHash == right.InputHash
}

func sameUUIDPointer(left, right *uuid.UUID) bool {
	return left != nil && right != nil && *left == *right
}

func sameInt64Pointer(left, right *int64) bool {
	return left != nil && right != nil && *left == *right
}

type storyReconcileReadyChild struct {
	Invocation model.AgentInvocation
	Revision   model.StageCandidateRevision
	SourceRef  agentcontract.StageSourceRef
	ProjectID  string
}

func storyReconcileInvocationRecord(
	manifest domain.StoryReconcileManifest,
	shard domain.StoryReconcileShard,
	children []storyReconcileReadyChild,
	now time.Time,
) (model.AgentInvocation, error) {
	if len(children) != len(shard.Children) || len(children) == 0 || len(children) > manifest.FanIn {
		return model.AgentInvocation{}, errors.New("Story reconcile children are incomplete")
	}
	candidateType := "story_analysis_candidate"
	if shard.Children[0].Stage == domain.ReconcileStoryStage {
		candidateType = "story_reconciliation_candidate"
	}
	inputs := make([]agentcontract.StoryReconciliationInputCandidate, len(children))
	upstreams := make([]agentcontract.StageUpstreamCandidateRef, len(children))
	sourceRef := children[0].SourceRef
	for index, child := range children {
		childSpec := shard.Children[index]
		if childSpec.Stage != child.Invocation.Stage || childSpec.ShardKey != child.Invocation.ShardKey ||
			child.SourceRef != sourceRef || child.ProjectID != children[0].ProjectID || child.Invocation.ResultHash == nil {
			return model.AgentInvocation{}, errors.New("Story reconcile child ordering or source has drifted")
		}
		candidateJSON := json.RawMessage(child.Revision.Candidate)
		if childSpec.CandidateItemStart != nil || childSpec.CandidateItemEnd != nil {
			if childSpec.CandidateItemStart == nil || childSpec.CandidateItemEnd == nil {
				return model.AgentInvocation{}, errors.New("Story reconcile child candidate range is incomplete")
			}
			switch candidateType {
			case "story_analysis_candidate":
				var candidate domain.StoryAnalysisCandidate
				if decodeErr := json.Unmarshal(candidateJSON, &candidate); decodeErr != nil {
					return model.AgentInvocation{}, decodeErr
				}
				sliced, sliceErr := domain.SliceStoryAnalysisCandidate(candidate, *childSpec.CandidateItemStart, *childSpec.CandidateItemEnd)
				if sliceErr != nil {
					return model.AgentInvocation{}, sliceErr
				}
				encoded, marshalErr := json.Marshal(sliced)
				if marshalErr != nil {
					return model.AgentInvocation{}, marshalErr
				}
				candidateJSON = encoded
			case "story_reconciliation_candidate":
				var candidate domain.StoryReconciliationCandidate
				if decodeErr := json.Unmarshal(candidateJSON, &candidate); decodeErr != nil {
					return model.AgentInvocation{}, decodeErr
				}
				sliced, sliceErr := domain.SliceStoryReconciliationCandidate(candidate, *childSpec.CandidateItemStart, *childSpec.CandidateItemEnd)
				if sliceErr != nil {
					return model.AgentInvocation{}, sliceErr
				}
				encoded, marshalErr := json.Marshal(sliced)
				if marshalErr != nil {
					return model.AgentInvocation{}, marshalErr
				}
				candidateJSON = encoded
			}
		}
		inputs[index] = agentcontract.StoryReconciliationInputCandidate{
			ShardKey: child.Invocation.ShardKey, CandidateRevisionID: child.Revision.ID.String(),
			CandidateRevisionHash: child.Revision.CandidateRevisionHash,
			CandidateItemStart:    childSpec.CandidateItemStart, CandidateItemEnd: childSpec.CandidateItemEnd,
			Candidate: candidateJSON,
		}
		upstreams[index] = agentcontract.StageUpstreamCandidateRef{
			Stage: child.Invocation.Stage, ShardKey: child.Invocation.ShardKey,
			CandidateRevisionID:   child.Revision.ID.String(),
			CandidateRevisionHash: child.Revision.CandidateRevisionHash,
			SourceInvocationID:    child.Invocation.ID.String(), SourceResultHash: *child.Invocation.ResultHash,
		}
	}
	stageInput, err := json.Marshal(agentcontract.StoryReconciliationStageInput{
		Level: shard.Level, CandidateType: candidateType, Candidates: inputs,
	})
	if err != nil {
		return model.AgentInvocation{}, err
	}
	manifestID, err := uuid.Parse(manifest.ManifestID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	invocationID := uuid.NewSHA1(manifestID, []byte(fmt.Sprintf(
		"story-reconcile\x00%d\x00%s\x00%s", manifest.Version, manifest.ManifestHash, shard.Key,
	)))
	request, err := agentcontract.NewStageInvocation(invocationID.String(), agentcontract.StoryGraphDefinition().ExecutionPolicy(), agentcontract.StageInvocationPayload{
		Stage: domain.ReconcileStoryStage, ShardKey: shard.Key,
		WorkspaceID: manifest.WorkspaceID, ProjectID: children[0].ProjectID,
		SourceRefs: []agentcontract.StageSourceRef{sourceRef}, UpstreamCandidates: upstreams,
		ShardManifestRef: agentcontract.ShardManifestRef{
			ManifestID: manifest.ManifestID, Version: manifest.Version, Hash: manifest.ManifestHash,
		},
		Shard: agentcontract.InvocationShard{
			Kind: shard.Kind, Key: shard.Key, TreePath: shard.TreePath, ParentKey: shard.ParentKey,
		},
		StageInput: stageInput,
	})
	if err != nil {
		return model.AgentInvocation{}, err
	}
	if err = agentcontract.ValidateStoryAnalysisInvocation(request); err != nil {
		return model.AgentInvocation{}, err
	}
	policyJSON, _ := json.Marshal(request.ExecutionPolicy)
	payloadJSON, _ := json.Marshal(request.Payload)
	stageKey, err := request.StageInstanceKey()
	if err != nil {
		return model.AgentInvocation{}, err
	}
	version := manifest.Version
	runID := uuid.MustParse(manifest.WorkflowRunID)
	nodeID := uuid.MustParse(manifest.NodeRunID)
	workspaceID := uuid.MustParse(manifest.WorkspaceID)
	return model.AgentInvocation{
		ID: invocationID, WorkspaceID: workspaceID, WorkflowRunID: &runID, NodeRunID: &nodeID,
		ShardManifestID: &manifestID, ShardManifestVersion: &version,
		RequestType: "story_reconcile_shard", RequestID: invocationID,
		Kind: "storygraph_stage", WireSchemaVersion: agentcontract.StoryGraphWireSchemaVersion,
		Stage: domain.ReconcileStoryStage, ShardKey: shard.Key, StageInstanceKey: stageKey,
		ShardManifestHash: manifest.ManifestHash, InputHash: request.InputHash,
		ExecutionPolicy: datatypes.JSON(policyJSON), Payload: datatypes.JSON(payloadJSON),
		Status: "queued", Attempts: 0, ClaimVersion: 0, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func storyAnalysisState(database *gorm.DB, manifest domain.StoryReconcileManifest) (application.StoryAnalysisState, error) {
	var root model.AgentInvocation
	err := database.Where("node_run_id = ? AND stage = ? AND shard_key = ? AND status = ?",
		manifest.NodeRunID, domain.ReconcileStoryStage, manifest.RootShardKey, "succeeded").
		First(&root).Error
	if err == nil && root.Status == "succeeded" {
		var revision model.StageCandidateRevision
		if err = database.First(&revision, "source_invocation_id = ?", root.ID).Error; err != nil {
			return application.StoryAnalysisState{}, err
		}
		var head model.StageCandidateHead
		if err = database.First(&head, "stage_instance_key = ?", revision.StageInstanceKey).Error; err != nil {
			return application.StoryAnalysisState{}, err
		}
		if head.CurrentRevisionID != revision.ID || head.CurrentCandidateRevisionHash != revision.CandidateRevisionHash {
			return application.StoryAnalysisState{}, application.ErrStoryAnalysisUpstreamStale
		}
		return application.StoryAnalysisState{
			Status: "ready", CandidateRevisionID: revision.ID.String(),
			CandidateRevisionHash: revision.CandidateRevisionHash, CandidateRevisionNo: revision.RevisionNo,
		}, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return application.StoryAnalysisState{}, err
	}
	latestManifestVersion := database.Model(&model.ShardManifest{}).
		Select("MAX(version)").Where("node_run_id = agt_invocations.node_run_id").Where("stage = agt_invocations.stage")
	var failed int64
	if err = database.Model(&model.AgentInvocation{}).
		Where("node_run_id = ? AND stage IN ? AND status = ?", manifest.NodeRunID,
			[]string{domain.AnalyzeStoryStage, domain.ReconcileStoryStage}, "failed").
		Where("shard_manifest_version = (?)", latestManifestVersion).Count(&failed).Error; err != nil {
		return application.StoryAnalysisState{}, err
	}
	if failed > 0 {
		return application.StoryAnalysisState{Status: "failed"}, nil
	}
	return application.StoryAnalysisState{Status: "pending"}, nil
}

func storyAnalysisManifestRecord(value domain.StoryAnalysisManifest, createdAt time.Time) (model.ShardManifest, error) {
	shards, err := json.Marshal(value.Shards)
	if err != nil {
		return model.ShardManifest{}, err
	}
	return storyManifestRecord(value.ManifestID, value.Version, value.WorkspaceID, value.WorkflowRunID,
		value.NodeRunID, value.Stage, value.RootInputHash, value.ParentManifestHash, value.CoverageHash, value.ManifestHash, shards, createdAt)
}

func storyReconcileManifestRecord(value domain.StoryReconcileManifest, createdAt time.Time) (model.ShardManifest, error) {
	shards, err := json.Marshal(value.Shards)
	if err != nil {
		return model.ShardManifest{}, err
	}
	return storyManifestRecord(value.ManifestID, value.Version, value.WorkspaceID, value.WorkflowRunID,
		value.NodeRunID, value.Stage, value.RootInputHash, value.ParentManifestHash, value.CoverageHash, value.ManifestHash, shards, createdAt)
}

func storyManifestRecord(manifestID string, version int64, workspaceID, runID, nodeID, stage, rootHash string, parentHash *string, coverageHash, manifestHash string, shards []byte, createdAt time.Time) (model.ShardManifest, error) {
	id, err := uuid.Parse(manifestID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	run, err := uuid.Parse(runID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	node, err := uuid.Parse(nodeID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	return model.ShardManifest{
		ID: id, Version: version, WorkspaceID: workspace, WorkflowRunID: run, NodeRunID: node,
		Stage: stage, RootInputHash: rootHash, ParentManifestHash: parentHash, Shards: datatypes.JSON(shards),
		CoverageHash: coverageHash, ManifestHash: manifestHash, CreatedAt: createdAt,
	}, nil
}

func storyAnalysisManifestDomain(record model.ShardManifest) (domain.StoryAnalysisManifest, error) {
	var shards []domain.StoryAnalysisShard
	if err := json.Unmarshal(record.Shards, &shards); err != nil {
		return domain.StoryAnalysisManifest{}, err
	}
	return domain.StoryAnalysisManifest{
		ManifestID: record.ID.String(), Version: record.Version, ParentManifestHash: record.ParentManifestHash, WorkspaceID: record.WorkspaceID.String(),
		WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(),
		Stage: record.Stage, RootInputHash: record.RootInputHash, Shards: shards,
		CoverageHash: record.CoverageHash, ManifestHash: record.ManifestHash,
	}, nil
}

func storyReconcileManifestDomain(record model.ShardManifest) (domain.StoryReconcileManifest, error) {
	var shards []domain.StoryReconcileShard
	if err := json.Unmarshal(record.Shards, &shards); err != nil {
		return domain.StoryReconcileManifest{}, err
	}
	referenced := make(map[string]struct{})
	for _, shard := range shards {
		if shard.Status != "active" {
			continue
		}
		for _, child := range shard.Children {
			if child.Stage == domain.ReconcileStoryStage {
				referenced[child.ShardKey] = struct{}{}
			}
		}
	}
	root := ""
	for _, shard := range shards {
		if shard.Status != "active" {
			continue
		}
		if _, exists := referenced[shard.Key]; !exists {
			if root != "" {
				return domain.StoryReconcileManifest{}, errors.New("Story reconcile manifest has multiple active roots")
			}
			root = shard.Key
		}
	}
	return domain.StoryReconcileManifest{
		ManifestID: record.ID.String(), Version: record.Version, ParentManifestHash: record.ParentManifestHash, WorkspaceID: record.WorkspaceID.String(),
		WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(),
		Stage: record.Stage, RootInputHash: record.RootInputHash, FanIn: 2, RootShardKey: root,
		Shards: shards, CoverageHash: record.CoverageHash, ManifestHash: record.ManifestHash,
	}, nil
}

func loadStoryReconcileManifest(database *gorm.DB, nodeRunID string) (domain.StoryReconcileManifest, error) {
	var record model.ShardManifest
	if err := database.Where("node_run_id = ? AND stage = ?", nodeRunID, domain.ReconcileStoryStage).
		Order("version DESC").First(&record).Error; err != nil {
		return domain.StoryReconcileManifest{}, err
	}
	return storyReconcileManifestDomain(record)
}

func loadLatestStoryManifestPair(
	database *gorm.DB,
	nodeRunID string,
	lock bool,
) (domain.StoryAnalysisManifest, domain.StoryReconcileManifest, error) {
	query := database
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var analyzeRecord model.ShardManifest
	if err := query.Where("node_run_id = ? AND stage = ?", nodeRunID, domain.AnalyzeStoryStage).
		Order("version DESC").First(&analyzeRecord).Error; err != nil {
		return domain.StoryAnalysisManifest{}, domain.StoryReconcileManifest{}, err
	}
	query = database
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var reconcileRecord model.ShardManifest
	if err := query.Where("node_run_id = ? AND stage = ?", nodeRunID, domain.ReconcileStoryStage).
		Order("version DESC").First(&reconcileRecord).Error; err != nil {
		return domain.StoryAnalysisManifest{}, domain.StoryReconcileManifest{}, err
	}
	analyze, err := storyAnalysisManifestDomain(analyzeRecord)
	if err != nil {
		return domain.StoryAnalysisManifest{}, domain.StoryReconcileManifest{}, err
	}
	reconcile, err := storyReconcileManifestDomain(reconcileRecord)
	return analyze, reconcile, err
}

func validateCurrentStoryManifest(database *gorm.DB, invocation model.AgentInvocation, lock bool) error {
	if invocation.NodeRunID == nil || invocation.ShardManifestVersion == nil || invocation.ShardManifestID == nil {
		return application.ErrStoryAnalysisManifestStale
	}
	var latest model.ShardManifest
	query := database
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Where("node_run_id = ? AND stage = ?", invocation.NodeRunID, invocation.Stage).
		Order("version DESC").First(&latest).Error; err != nil {
		return err
	}
	if latest.ID != *invocation.ShardManifestID || latest.Version != *invocation.ShardManifestVersion ||
		latest.ManifestHash != invocation.ShardManifestHash {
		return application.ErrStoryAnalysisManifestStale
	}
	return nil
}

func strictSourceEvidenceCandidate(raw json.RawMessage) (domain.SourceEvidenceCandidate, error) {
	var value domain.SourceEvidenceCandidate
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return domain.SourceEvidenceCandidate{}, errors.New("candidate does not match source Evidence schema")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.SourceEvidenceCandidate{}, errors.New("source Evidence candidate contains multiple JSON values")
	}
	return value, nil
}
