package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
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

func (store *Store) EnsureSourceEvidence(
	ctx context.Context,
	preparation application.SourceEvidencePreparation,
) (application.SourceEvidenceState, error) {
	if err := domain.ValidateSourceEvidenceManifest(preparation.Manifest, preparation.NormalizedText); err != nil {
		return application.SourceEvidenceState{}, err
	}
	var state application.SourceEvidenceState
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		if err := validateSourceEvidenceOwners(ctx, transaction, preparation); err != nil {
			return err
		}
		var existing model.ShardManifest
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("node_run_id = ? AND stage = ?", preparation.Manifest.NodeRunID, domain.SourceEvidenceStage).
			Order("version DESC").First(&existing).Error
		if err == nil {
			manifest, decodeErr := sourceEvidenceManifestDomain(existing)
			if decodeErr != nil {
				return decodeErr
			}
			if manifest.RootInputHash != preparation.Manifest.RootInputHash ||
				manifest.WorkflowRunID != preparation.Manifest.WorkflowRunID ||
				manifest.NodeRunID != preparation.Manifest.NodeRunID {
				return errors.New("source Evidence manifest changed for the existing NodeRun")
			}
			if manifest.Version == 1 && manifest.ManifestHash != preparation.Manifest.ManifestHash {
				return errors.New("source Evidence initial manifest changed for the existing NodeRun")
			}
			if decodeErr = domain.ValidateSourceEvidenceManifest(manifest, preparation.NormalizedText); decodeErr != nil {
				return decodeErr
			}
			state, err = sourceEvidenceState(transaction, manifest)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		record, err := sourceEvidenceManifestRecord(preparation.Manifest, preparation.CreatedAt)
		if err != nil {
			return err
		}
		if len(preparation.Invocations) == 0 {
			return errors.New("source Evidence preparation has no invocations")
		}
		if err = transaction.Omit(clause.Associations).Create(&record).Error; err != nil {
			return err
		}
		for _, invocation := range preparation.Invocations {
			if invocation.WorkflowRunID != preparation.Manifest.WorkflowRunID ||
				invocation.NodeRunID != preparation.Manifest.NodeRunID ||
				invocation.ManifestID != preparation.Manifest.ManifestID ||
				invocation.ManifestVersion != preparation.Manifest.Version ||
				invocation.ManifestHash != preparation.Manifest.ManifestHash {
				return errors.New("source Evidence invocation does not belong to the manifest")
			}
			invocationRecord, recordErr := invocationRecord(invocation)
			if recordErr != nil {
				return recordErr
			}
			if recordErr = transaction.Omit(clause.Associations).Create(&invocationRecord).Error; recordErr != nil {
				return recordErr
			}
		}
		state, err = sourceEvidenceState(transaction, preparation.Manifest)
		return err
	})
	return state, err
}

func (store *Store) ClaimNextSourceEvidence(
	ctx context.Context,
	now time.Time,
	leaseExpiresAt time.Time,
) (domain.Invocation, bool, error) {
	if !leaseExpiresAt.After(now) {
		return domain.Invocation{}, false, errors.New("source Evidence invocation lease must expire after claim time")
	}
	var result domain.Invocation
	found := false
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		latestManifestVersion := transaction.Model(&model.ShardManifest{}).
			Select("MAX(version)").
			Where("node_run_id = agt_invocations.node_run_id").
			Where("stage = agt_invocations.stage")
		var record model.AgentInvocation
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("kind = ? AND stage = ?", "storygraph_stage", domain.SourceEvidenceStage).
			Where("workflow_run_id IS NOT NULL AND node_run_id IS NOT NULL AND shard_manifest_id IS NOT NULL").
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
		result = invocationDomain(record)
		found = true
		return nil
	})
	return result, found, err
}

func (store *Store) CompleteSourceEvidenceInvocation(
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
		request, err := agentgorm.StageInvocation(invocation)
		if err != nil {
			return err
		}
		manifest, normalizedText, shard, err := loadSourceEvidenceInvocation(ctx, transaction, invocation, request)
		if err != nil {
			return err
		}
		if _, err = domain.DecodeAndNormalizeSourceEvidenceCandidate(result.Candidate, normalizedText, shard); err != nil {
			return err
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
		if err = aggregateSourceEvidence(ctx, transaction, manifest, normalizedText, now); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

func (store *Store) FailSourceEvidenceInvocation(
	ctx context.Context,
	invocationID string,
	claimVersion int,
	outcome, code, summary string,
	retryable bool,
	now time.Time,
) (bool, error) {
	if outcome != "failed" && outcome != "unknown" {
		outcome = "unknown"
	}
	id, err := uuid.Parse(invocationID)
	if err != nil {
		return false, application.ErrNotFound
	}
	errorJSON, err := json.Marshal(map[string]any{"code": code, "summary": summary, "retryable": retryable})
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
		if err := transaction.Model(&invocation).Updates(map[string]any{
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

func (store *Store) LoadSourceEvidenceReshardSeed(
	ctx context.Context,
	invocationID string,
	claimVersion int,
	now time.Time,
) (application.SourceEvidenceReshardSeed, error) {
	id, err := uuid.Parse(invocationID)
	if err != nil {
		return application.SourceEvidenceReshardSeed{}, application.ErrNotFound
	}
	var seed application.SourceEvidenceReshardSeed
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var invocation model.AgentInvocation
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invocation, "id = ?", id).Error; err != nil {
			return normalizeNotFound(err)
		}
		if !activeInvocationClaim(invocation, claimVersion, now) {
			return errors.New("source Evidence reshard claim is stale")
		}
		request, err := agentgorm.StageInvocation(invocation)
		if err != nil {
			return err
		}
		manifest, normalizedText, shard, err := loadSourceEvidenceInvocation(ctx, transaction, invocation, request)
		if err != nil {
			return err
		}
		var latest model.ShardManifest
		if err = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("node_run_id = ? AND stage = ?", manifest.NodeRunID, domain.SourceEvidenceStage).
			Order("version DESC").First(&latest).Error; err != nil {
			return err
		}
		if latest.Version != manifest.Version || latest.ManifestHash != manifest.ManifestHash ||
			shard.Status != "active" || len(request.Payload.SourceRefs) != 1 {
			return errors.New("source Evidence reshard manifest is stale")
		}
		ref := request.Payload.SourceRefs[0]
		seed = application.SourceEvidenceReshardSeed{
			Manifest: manifest, ParentShardKey: shard.Key,
			Source: application.SourceEvidenceSource{
				ProjectID: request.Payload.ProjectID, DocumentLogicalID: ref.OwnerLogicalID,
				DocumentRevisionID: ref.OwnerVersionID, DocumentRevision: ref.Revision,
				NormalizedText: normalizedText, NormalizedHash: ref.ContentHash,
			},
		}
		return nil
	})
	return seed, err
}

func (store *Store) ApplySourceEvidenceReshard(
	ctx context.Context,
	preparation application.SourceEvidenceReshardPreparation,
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
		request, err := agentgorm.StageInvocation(invocation)
		if err != nil {
			return err
		}
		current, normalizedText, _, err := loadSourceEvidenceInvocation(ctx, transaction, invocation, request)
		if err != nil {
			return err
		}
		var latest model.ShardManifest
		if err = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("node_run_id = ? AND stage = ?", current.NodeRunID, domain.SourceEvidenceStage).
			Order("version DESC").First(&latest).Error; err != nil {
			return err
		}
		if latest.Version != current.Version || latest.ManifestHash != preparation.PreviousManifestHash ||
			preparation.Manifest.ManifestID != current.ManifestID ||
			preparation.Manifest.Version != current.Version+1 ||
			preparation.Manifest.ParentManifestHash == nil ||
			*preparation.Manifest.ParentManifestHash != current.ManifestHash {
			return errors.New("source Evidence reshard lineage has drifted")
		}
		if err = domain.ValidateSourceEvidenceManifest(preparation.Manifest, normalizedText); err != nil {
			return err
		}
		record, err := sourceEvidenceManifestRecord(preparation.Manifest, preparation.CreatedAt)
		if err != nil {
			return err
		}
		if err = transaction.Omit(clause.Associations).Create(&record).Error; err != nil {
			return err
		}
		for _, value := range preparation.Invocations {
			if value.WorkflowRunID != preparation.Manifest.WorkflowRunID ||
				value.NodeRunID != preparation.Manifest.NodeRunID ||
				value.ManifestID != preparation.Manifest.ManifestID ||
				value.ManifestVersion != preparation.Manifest.Version ||
				value.ManifestHash != preparation.Manifest.ManifestHash {
				return errors.New("source Evidence reshard invocation does not belong to the manifest")
			}
			record, recordErr := invocationRecord(value)
			if recordErr != nil {
				return recordErr
			}
			if recordErr = transaction.Omit(clause.Associations).Create(&record).Error; recordErr != nil {
				return recordErr
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
			"code": "manifest_superseded", "summary": "A newer Source Evidence manifest replaced this pending invocation", "retryable": false,
		})
		if err != nil {
			return err
		}
		if err = transaction.Model(&model.AgentInvocation{}).
			Where("node_run_id = ? AND shard_manifest_id = ? AND shard_manifest_version = ? AND status IN ?",
				invocation.NodeRunID, invocation.ShardManifestID, invocation.ShardManifestVersion, []string{"queued", "unknown"}).
			Updates(map[string]any{
				"status": "failed", "error": datatypes.JSON(supersededJSON),
				"completed_at": preparation.CreatedAt, "updated_at": preparation.CreatedAt,
			}).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

func validateSourceEvidenceOwners(
	ctx context.Context,
	database *gorm.DB,
	preparation application.SourceEvidencePreparation,
) error {
	manifest := preparation.Manifest
	var run model.WorkflowRun
	if err := database.WithContext(ctx).First(&run, "id = ?", manifest.WorkflowRunID).Error; err != nil {
		return errors.New("source Evidence requires an existing WorkflowRun")
	}
	var node model.NodeRunProjection
	if err := database.WithContext(ctx).First(&node, "id = ?", manifest.NodeRunID).Error; err != nil {
		return errors.New("source Evidence requires an existing NodeRun")
	}
	if run.WorkspaceID.String() != manifest.WorkspaceID || run.ProjectID.String() != preparation.ProjectID ||
		node.WorkspaceID != run.WorkspaceID || node.WorkflowRunID != run.ID ||
		node.Executor != "activity.source_evidence" || node.Status == "SUCCEEDED" || node.Status == "FAILED" ||
		node.Status == "CANCELLED" || node.Status == "SKIPPED" || node.Status == "CACHED" {
		return errors.New("source Evidence WorkflowRun or NodeRun has drifted")
	}
	revisionID, err := uuid.Parse(preparation.DocumentRevisionID)
	if err != nil {
		return errors.New("invalid source Evidence DocumentRevision")
	}
	var revision model.DocumentRevision
	if err = database.WithContext(ctx).First(&revision, "id = ?", revisionID).Error; err != nil {
		return errors.New("source Evidence DocumentRevision does not exist")
	}
	var document model.ScriptDocument
	if err = database.WithContext(ctx).First(&document, "id = ?", revision.DocumentID).Error; err != nil {
		return errors.New("source Evidence ScriptDocument does not exist")
	}
	if document.ID.String() != preparation.DocumentLogicalID || document.ProjectID != run.ProjectID ||
		revision.WorkspaceID != run.WorkspaceID || int64(revision.VersionNo) != preparation.DocumentRevision ||
		revision.NormalizedHash != manifest.RootInputHash || revision.NormalizedText != preparation.NormalizedText {
		return errors.New("source Evidence DocumentRevision has drifted")
	}
	return nil
}

func sourceEvidenceState(database *gorm.DB, manifest domain.SourceEvidenceManifest) (application.SourceEvidenceState, error) {
	state := application.SourceEvidenceState{
		Status: "pending", ManifestID: manifest.ManifestID, ManifestVersion: manifest.Version,
		ManifestHash: manifest.ManifestHash,
	}
	aggregateKey := domain.SourceEvidenceAggregateStageInstanceKey(manifest)
	var head model.StageCandidateHead
	if err := database.First(&head, "stage_instance_key = ?", aggregateKey).Error; err == nil {
		state.Status = "ready"
		state.CandidateRevisionID = head.CurrentRevisionID.String()
		state.CandidateRevisionHash = head.CurrentCandidateRevisionHash
		state.CandidateRevisionNo = head.Revision
		return state, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return application.SourceEvidenceState{}, err
	}
	var failed int64
	if err := database.Model(&model.AgentInvocation{}).
		Where("node_run_id = ? AND shard_manifest_id = ? AND shard_manifest_version = ? AND status = ?",
			manifest.NodeRunID, manifest.ManifestID, manifest.Version, "failed").Count(&failed).Error; err != nil {
		return application.SourceEvidenceState{}, err
	}
	if failed > 0 {
		state.Status = "failed"
	}
	return state, nil
}

func aggregateSourceEvidence(
	ctx context.Context,
	database *gorm.DB,
	manifest domain.SourceEvidenceManifest,
	normalizedText string,
	now time.Time,
) error {
	var latest model.ShardManifest
	if err := database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("node_run_id = ? AND stage = ?", manifest.NodeRunID, domain.SourceEvidenceStage).
		Order("version DESC").First(&latest).Error; err != nil {
		return err
	}
	if latest.Version != manifest.Version || latest.ManifestHash != manifest.ManifestHash {
		return nil
	}
	active := make([]domain.SourceEvidenceShard, 0, len(manifest.Shards))
	for _, shard := range manifest.Shards {
		if shard.Status == "active" {
			active = append(active, shard)
		}
	}
	slices.SortFunc(active, func(left, right domain.SourceEvidenceShard) int {
		return left.LogicalStart - right.LogicalStart
	})
	fragments := make([]domain.SourceEvidenceFragment, 0, len(active))
	leaves := make([]agentcontract.AggregateLeafCandidateRef, 0, len(active))
	for _, shard := range active {
		var invocation model.AgentInvocation
		if err := database.Where(
			"node_run_id = ? AND shard_manifest_id = ? AND shard_manifest_version = ? AND shard_key = ?",
			manifest.NodeRunID, manifest.ManifestID, manifest.Version, shard.Key,
		).First(&invocation).Error; err != nil {
			return err
		}
		if invocation.Status != "succeeded" {
			return nil
		}
		var revision model.StageCandidateRevision
		if err := database.First(&revision, "source_invocation_id = ?", invocation.ID).Error; err != nil {
			return err
		}
		candidate, err := domain.DecodeAndNormalizeSourceEvidenceCandidate(json.RawMessage(invocation.Candidate), normalizedText, shard)
		if err != nil {
			return err
		}
		fragments = append(fragments, domain.SourceEvidenceFragment{
			ShardKey: shard.Key, LogicalStart: shard.LogicalStart, LogicalEnd: shard.LogicalEnd,
			CandidateRevisionID: revision.ID.String(), CandidateRevisionHash: revision.CandidateRevisionHash,
			Candidate: candidate,
		})
		leaves = append(leaves, agentcontract.AggregateLeafCandidateRef{
			StageInstanceKey: invocation.StageInstanceKey, ShardKey: shard.Key,
			CandidateRevisionID: revision.ID.String(), CandidateRevisionHash: revision.CandidateRevisionHash,
		})
	}
	_, candidateJSON, contentHash, err := domain.BuildSourceEvidenceAggregate(manifest, fragments)
	if err != nil {
		return err
	}
	aggregateKey := domain.SourceEvidenceAggregateStageInstanceKey(manifest)
	slices.SortFunc(leaves, func(left, right agentcontract.AggregateLeafCandidateRef) int {
		if left.StageInstanceKey != right.StageInstanceKey {
			return stringsCompare(left.StageInstanceKey, right.StageInstanceKey)
		}
		return stringsCompare(left.ShardKey, right.ShardKey)
	})
	origin := agentcontract.AggregateCandidateOrigin{
		ShardManifestID: manifest.ManifestID, ManifestVersion: manifest.Version,
		ShardManifestHash: manifest.ManifestHash, LeafCandidates: leaves,
	}
	revisionHash, err := (agentcontract.CandidateRevisionMaterial{
		StageInstanceKey: aggregateKey, RevisionNo: 1, OriginKind: "aggregate",
		AggregateOrigin: &origin, CandidateContentHash: contentHash,
	}).Hash()
	if err != nil {
		return err
	}
	var existing model.StageCandidateHead
	if err = database.First(&existing, "stage_instance_key = ?", aggregateKey).Error; err == nil {
		if existing.CurrentCandidateRevisionHash != revisionHash {
			return agentgorm.ErrCandidateResultConflict
		}
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	originJSON, err := json.Marshal(origin)
	if err != nil {
		return err
	}
	revision := model.StageCandidateRevision{
		ID: uuid.New(), WorkspaceID: uuid.MustParse(manifest.WorkspaceID), StageInstanceKey: aggregateKey,
		RevisionNo: 1, OriginKind: "aggregate", AggregateOrigin: datatypes.JSON(originJSON),
		Candidate: datatypes.JSON(candidateJSON), CandidateContentHash: contentHash,
		CandidateRevisionHash: revisionHash, CreatedAt: now,
	}
	if err = database.Omit(clause.Associations).Create(&revision).Error; err != nil {
		return err
	}
	head := model.StageCandidateHead{
		WorkspaceID: revision.WorkspaceID, StageInstanceKey: aggregateKey,
		CurrentRevisionID: revision.ID, CurrentCandidateRevisionHash: revisionHash,
		Revision: 1, UpdatedAt: now,
	}
	return database.Omit(clause.Associations).Create(&head).Error
}

func loadSourceEvidenceInvocation(
	ctx context.Context,
	database *gorm.DB,
	invocation model.AgentInvocation,
	request agentcontract.StageInvocation,
) (domain.SourceEvidenceManifest, string, domain.SourceEvidenceShard, error) {
	if invocation.WorkflowRunID == nil || invocation.NodeRunID == nil || invocation.ShardManifestID == nil ||
		invocation.ShardManifestVersion == nil || len(request.Payload.SourceRefs) != 1 {
		return domain.SourceEvidenceManifest{}, "", domain.SourceEvidenceShard{}, errors.New("source Evidence invocation owner is incomplete")
	}
	var record model.ShardManifest
	if err := database.WithContext(ctx).First(&record, "id = ? AND version = ?", *invocation.ShardManifestID, *invocation.ShardManifestVersion).Error; err != nil {
		return domain.SourceEvidenceManifest{}, "", domain.SourceEvidenceShard{}, err
	}
	manifest, err := sourceEvidenceManifestDomain(record)
	if err != nil || manifest.ManifestHash != invocation.ShardManifestHash || manifest.NodeRunID != invocation.NodeRunID.String() {
		return domain.SourceEvidenceManifest{}, "", domain.SourceEvidenceShard{}, errors.New("source Evidence invocation manifest has drifted")
	}
	revisionID, err := uuid.Parse(request.Payload.SourceRefs[0].OwnerVersionID)
	if err != nil {
		return domain.SourceEvidenceManifest{}, "", domain.SourceEvidenceShard{}, err
	}
	var revision model.DocumentRevision
	if err = database.WithContext(ctx).First(&revision, "id = ?", revisionID).Error; err != nil {
		return domain.SourceEvidenceManifest{}, "", domain.SourceEvidenceShard{}, err
	}
	if revision.NormalizedHash != manifest.RootInputHash {
		return domain.SourceEvidenceManifest{}, "", domain.SourceEvidenceShard{}, errors.New("source Evidence revision hash has drifted")
	}
	for _, shard := range manifest.Shards {
		if shard.Key == invocation.ShardKey {
			return manifest, revision.NormalizedText, shard, nil
		}
	}
	return domain.SourceEvidenceManifest{}, "", domain.SourceEvidenceShard{}, errors.New("source Evidence invocation shard is absent from its manifest")
}

func sourceEvidenceManifestRecord(value domain.SourceEvidenceManifest, createdAt time.Time) (model.ShardManifest, error) {
	id, err := uuid.Parse(value.ManifestID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	runID, err := uuid.Parse(value.WorkflowRunID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	nodeID, err := uuid.Parse(value.NodeRunID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	shards, err := json.Marshal(value.Shards)
	if err != nil {
		return model.ShardManifest{}, err
	}
	return model.ShardManifest{
		ID: id, Version: value.Version, WorkspaceID: workspaceID, WorkflowRunID: runID,
		NodeRunID: nodeID, Stage: value.Stage, RootInputHash: value.RootInputHash,
		ParentManifestHash: value.ParentManifestHash, Shards: datatypes.JSON(shards),
		CoverageHash: value.CoverageHash, ManifestHash: value.ManifestHash, CreatedAt: createdAt,
	}, nil
}

func sourceEvidenceManifestDomain(record model.ShardManifest) (domain.SourceEvidenceManifest, error) {
	var shards []domain.SourceEvidenceShard
	if err := json.Unmarshal(record.Shards, &shards); err != nil {
		return domain.SourceEvidenceManifest{}, err
	}
	return domain.SourceEvidenceManifest{
		ManifestID: record.ID.String(), Version: record.Version,
		ParentManifestHash: record.ParentManifestHash, WorkspaceID: record.WorkspaceID.String(),
		WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(),
		Stage: record.Stage, RootInputHash: record.RootInputHash, Shards: shards,
		CoverageHash: record.CoverageHash, ManifestHash: record.ManifestHash,
	}, nil
}

func activeInvocationClaim(value model.AgentInvocation, claimVersion int, now time.Time) bool {
	return value.Status == "running" && value.ClaimVersion == claimVersion &&
		value.LeaseExpiresAt != nil && now.Before(*value.LeaseExpiresAt)
}

func stringsCompare(left, right string) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

var _ application.SourceEvidenceRepository = (*Store)(nil)
