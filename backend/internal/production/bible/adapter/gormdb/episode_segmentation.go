package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
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

func (store *Store) LoadEpisodeSegmentationSeed(
	ctx context.Context,
	command application.EpisodeSegmentationCommand,
) (application.EpisodeSegmentationSeed, error) {
	return loadEpisodeSegmentationSeed(ctx, store.database, command)
}

func loadEpisodeSegmentationSeed(
	ctx context.Context,
	database *gorm.DB,
	command application.EpisodeSegmentationCommand,
) (application.EpisodeSegmentationSeed, error) {
	runID, runErr := uuid.Parse(command.WorkflowRunID)
	nodeID, nodeErr := uuid.Parse(command.NodeRunID)
	documentRevisionID, documentErr := uuid.Parse(command.DocumentRevisionID)
	aggregateID, aggregateErr := uuid.Parse(command.EvidenceCandidateRevisionID)
	bibleVersionID, bibleErr := uuid.Parse(command.BibleVersionID)
	if runErr != nil || nodeErr != nil || documentErr != nil || aggregateErr != nil || bibleErr != nil {
		return application.EpisodeSegmentationSeed{}, errors.New("invalid Episode segmentation exact identity")
	}
	var run model.WorkflowRun
	if err := database.WithContext(ctx).First(&run, "id = ?", runID).Error; err != nil {
		return application.EpisodeSegmentationSeed{}, errors.New("Episode segmentation requires an existing WorkflowRun")
	}
	var node model.NodeRunProjection
	if err := database.WithContext(ctx).First(&node, "id = ?", nodeID).Error; err != nil {
		return application.EpisodeSegmentationSeed{}, errors.New("Episode segmentation requires an existing NodeRun")
	}
	if run.WorkspaceID.String() != command.WorkspaceID || run.ProjectID.String() != command.ProjectID ||
		node.WorkspaceID != run.WorkspaceID || node.WorkflowRunID != run.ID ||
		node.Executor != "activity.episode_segmentation" ||
		node.Status == "FAILED" || node.Status == "CANCELLED" || node.Status == "SKIPPED" || node.Status == "CACHED" {
		return application.EpisodeSegmentationSeed{}, errors.New("Episode segmentation WorkflowRun or NodeRun has drifted")
	}
	var project model.Project
	if err := database.WithContext(ctx).First(&project, "id = ?", run.ProjectID).Error; err != nil ||
		project.WorkspaceID != run.WorkspaceID || project.TargetDurationMS < 1000 {
		return application.EpisodeSegmentationSeed{}, errors.New("Episode segmentation Project has drifted")
	}
	var revision model.DocumentRevision
	if err := database.WithContext(ctx).First(&revision, "id = ?", documentRevisionID).Error; err != nil {
		return application.EpisodeSegmentationSeed{}, errors.New("Episode segmentation DocumentRevision does not exist")
	}
	var document model.ScriptDocument
	if err := database.WithContext(ctx).First(&document, "id = ?", revision.DocumentID).Error; err != nil ||
		document.WorkspaceID != run.WorkspaceID || document.ProjectID != run.ProjectID ||
		revision.WorkspaceID != run.WorkspaceID || revision.NormalizedHash != command.DocumentRevisionHash ||
		revision.CodepointCount != len([]rune(revision.NormalizedText)) || revision.CodepointCount < 1 {
		return application.EpisodeSegmentationSeed{}, errors.New("Episode segmentation DocumentRevision has drifted")
	}

	var aggregate model.StageCandidateRevision
	if err := database.WithContext(ctx).First(&aggregate, "id = ?", aggregateID).Error; err != nil {
		return application.EpisodeSegmentationSeed{}, errors.New("Episode segmentation Evidence aggregate does not exist")
	}
	var aggregateHead model.StageCandidateHead
	if err := database.WithContext(ctx).First(&aggregateHead, "stage_instance_key = ?", aggregate.StageInstanceKey).Error; err != nil ||
		aggregate.WorkspaceID != run.WorkspaceID || aggregate.OriginKind != "aggregate" ||
		aggregate.CandidateRevisionHash != command.EvidenceCandidateRevisionHash ||
		aggregateHead.CurrentRevisionID != aggregate.ID ||
		aggregateHead.CurrentCandidateRevisionHash != aggregate.CandidateRevisionHash {
		return application.EpisodeSegmentationSeed{}, errors.New("Episode segmentation Evidence aggregate is stale")
	}
	aggregateValue, err := domain.DecodeSourceEvidenceAggregate(json.RawMessage(aggregate.Candidate))
	if err != nil {
		return application.EpisodeSegmentationSeed{}, err
	}
	manifestID, err := uuid.Parse(aggregateValue.ManifestID)
	if err != nil {
		return application.EpisodeSegmentationSeed{}, err
	}
	var sourceManifestRecord model.ShardManifest
	if err = database.WithContext(ctx).First(&sourceManifestRecord, "id = ? AND version = ?", manifestID, aggregateValue.ManifestVersion).Error; err != nil {
		return application.EpisodeSegmentationSeed{}, err
	}
	sourceManifest, err := sourceEvidenceManifestDomain(sourceManifestRecord)
	if err != nil || sourceManifest.ManifestHash != aggregateValue.ManifestHash ||
		sourceManifest.CoverageHash != aggregateValue.CoverageHash || sourceManifest.RootInputHash != revision.NormalizedHash {
		return application.EpisodeSegmentationSeed{}, errors.New("Episode segmentation source Evidence manifest has drifted")
	}

	var version model.ProductionBibleVersion
	if err = database.WithContext(ctx).First(&version, "id = ?", bibleVersionID).Error; err != nil ||
		version.WorkspaceID != run.WorkspaceID || version.ProjectID != run.ProjectID ||
		version.DocumentRevisionID != revision.ID || version.Version != command.BibleVersion {
		return application.EpisodeSegmentationSeed{}, errors.New("Episode segmentation Production Bible Version has drifted")
	}
	actualMaterialization, err := loadExactMaterialization(ctx, database, run, version)
	if err != nil || actualMaterialization.ContentHash != command.MaterializationHash {
		return application.EpisodeSegmentationSeed{}, errors.New("Episode segmentation Production Bible materialization has drifted")
	}

	seed := application.EpisodeSegmentationSeed{
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		DocumentLogicalID: document.ID.String(), DocumentRevisionID: revision.ID.String(),
		DocumentRevision: int64(revision.VersionNo), NormalizedText: revision.NormalizedText,
		NormalizedHash: revision.NormalizedHash, TargetDurationMS: project.TargetDurationMS,
		BibleVersionID: version.ID.String(), BibleVersion: version.Version, BibleContentHash: version.ContentHash,
		MaterializationHash:           actualMaterialization.ContentHash,
		EvidenceAggregateRevisionID:   aggregate.ID.String(),
		EvidenceAggregateRevisionHash: aggregate.CandidateRevisionHash,
		Evidence:                      make([]application.EpisodeSegmentationEvidenceSeed, 0, len(aggregateValue.Fragments)),
		Markers:                       []domain.EpisodeSegmentationMarker{},
	}
	for _, fragment := range aggregateValue.Fragments {
		leafID, parseErr := uuid.Parse(fragment.CandidateRevisionID)
		if parseErr != nil {
			return application.EpisodeSegmentationSeed{}, parseErr
		}
		var leaf model.StageCandidateRevision
		if err = database.WithContext(ctx).First(&leaf, "id = ?", leafID).Error; err != nil {
			return application.EpisodeSegmentationSeed{}, err
		}
		var leafHead model.StageCandidateHead
		if err = database.WithContext(ctx).First(&leafHead, "stage_instance_key = ?", leaf.StageInstanceKey).Error; err != nil ||
			leaf.WorkspaceID != run.WorkspaceID || leaf.OriginKind != "invocation" ||
			leaf.SourceInvocationID == nil || leaf.SourceResultHash == nil ||
			leaf.CandidateRevisionHash != fragment.CandidateRevisionHash || leafHead.CurrentRevisionID != leaf.ID ||
			leafHead.CurrentCandidateRevisionHash != leaf.CandidateRevisionHash {
			return application.EpisodeSegmentationSeed{}, errors.New("Episode segmentation Evidence leaf is stale")
		}
		var invocation model.AgentInvocation
		if err = database.WithContext(ctx).First(&invocation, "id = ?", *leaf.SourceInvocationID).Error; err != nil {
			return application.EpisodeSegmentationSeed{}, err
		}
		request, requestErr := agentgorm.StageInvocation(invocation)
		if requestErr != nil || invocation.Stage != domain.SourceEvidenceStage || invocation.Status != "succeeded" ||
			invocation.ResultHash == nil || *invocation.ResultHash != *leaf.SourceResultHash ||
			len(request.Payload.SourceRefs) != 1 || request.Payload.SourceRefs[0].OwnerVersionID != revision.ID.String() ||
			request.Payload.SourceRefs[0].ContentHash != revision.NormalizedHash || request.Payload.ShardKey != fragment.ShardKey {
			return application.EpisodeSegmentationSeed{}, errors.New("Episode segmentation Evidence leaf provenance is incomplete")
		}
		candidateJSON, marshalErr := json.Marshal(fragment.Candidate)
		if marshalErr != nil {
			return application.EpisodeSegmentationSeed{}, marshalErr
		}
		candidate, candidateErr := strictSourceEvidenceCandidate(candidateJSON)
		if candidateErr != nil {
			return application.EpisodeSegmentationSeed{}, candidateErr
		}
		seed.Evidence = append(seed.Evidence, application.EpisodeSegmentationEvidenceSeed{
			ShardKey: fragment.ShardKey, CandidateRevisionID: leaf.ID.String(),
			CandidateRevisionHash: leaf.CandidateRevisionHash,
			SourceInvocationID:    invocation.ID.String(), SourceResultHash: *invocation.ResultHash,
			LogicalStart: fragment.LogicalStart, LogicalEnd: fragment.LogicalEnd, Candidate: candidate,
		})
	}
	for _, shard := range sourceManifest.Shards {
		if shard.Status != "active" {
			continue
		}
		for _, marker := range shard.EpisodeMarkerHints {
			if marker.AbsoluteStart < shard.LogicalStart || marker.AbsoluteStart >= shard.LogicalEnd ||
				marker.AbsoluteEnd > len([]rune(revision.NormalizedText)) {
				continue
			}
			anchor := string([]rune(revision.NormalizedText)[marker.AbsoluteStart:marker.AbsoluteEnd])
			episodeNumber := marker.EpisodeNumber
			seed.Markers = append(seed.Markers, domain.EpisodeSegmentationMarker{
				EpisodeNumber: marker.EpisodeNumber, Label: marker.Label,
				Evidence: domain.Evidence{
					SourceStart: marker.AbsoluteStart, SourceEnd: marker.AbsoluteEnd,
					TextHash: domain.SourceTextHash(anchor), ExactAnchor: anchor, EpisodeNumber: &episodeNumber,
				},
			})
		}
	}
	slices.SortFunc(seed.Evidence, func(left, right application.EpisodeSegmentationEvidenceSeed) int {
		return strings.Compare(left.ShardKey, right.ShardKey)
	})
	slices.SortFunc(seed.Markers, func(left, right domain.EpisodeSegmentationMarker) int {
		return left.Evidence.SourceStart - right.Evidence.SourceStart
	})
	return seed, nil
}

func loadExactMaterialization(
	ctx context.Context,
	database *gorm.DB,
	run model.WorkflowRun,
	version model.ProductionBibleVersion,
) (domain.Materialization, error) {
	repo := &repository{database: database}
	scope, err := repo.PrepareMaterialization(ctx, application.Actor{
		UserID: run.CreatedBy.String(), TokenVersion: run.InitiatorTokenVersion,
	}, version.ID.String(), false)
	if err != nil || len(scope.Bindings) == 0 {
		return domain.Materialization{}, errors.New("Production Bible Version is not materialized")
	}
	assets := make([]domain.MaterializedAsset, 0, len(scope.Bindings))
	specifications := make([]domain.MaterializedSpecification, 0, len(scope.Bindings))
	states := make([]domain.MaterializedState, 0)
	bindings := make([]domain.MaterializedBinding, 0, len(scope.Bindings))
	for _, binding := range scope.Bindings {
		assets = append(assets, binding.Asset)
		specifications = append(specifications, binding.Specification)
		states = append(states, binding.States...)
		bindings = append(bindings, domain.MaterializedBindingRef(binding))
	}
	return domain.NewMaterialization(scope.Version.ID, scope.Version.ContentHash, assets, specifications, states, bindings)
}

func (store *Store) EnsureEpisodeSegmentation(
	ctx context.Context,
	preparation application.EpisodeSegmentationPreparation,
) (application.EpisodeSegmentationState, error) {
	if err := domain.ValidateEpisodeSegmentationManifest(preparation.Manifest); err != nil {
		return application.EpisodeSegmentationState{}, err
	}
	var state application.EpisodeSegmentationState
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		seed, err := loadEpisodeSegmentationSeed(ctx, transaction, preparation.Command)
		if err != nil {
			return err
		}
		rootHash, err := application.EpisodeSegmentationRootInputHash(seed)
		if err != nil || rootHash != preparation.Manifest.RootInputHash || seed.NodeRunID != preparation.Manifest.NodeRunID {
			return errors.New("Episode segmentation sources changed before persistence")
		}
		var existing model.ShardManifest
		err = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("node_run_id = ? AND stage = ?", preparation.Manifest.NodeRunID, domain.EpisodeSegmentationStage).
			First(&existing).Error
		if err == nil {
			manifest, decodeErr := episodeSegmentationManifestDomain(existing)
			if decodeErr != nil || manifest.ManifestHash != preparation.Manifest.ManifestHash {
				return errors.New("Episode segmentation manifest changed for the existing NodeRun")
			}
			state, err = episodeSegmentationState(transaction, manifest)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		record, err := episodeSegmentationManifestRecord(preparation.Manifest, preparation.CreatedAt)
		if err != nil {
			return err
		}
		if err = transaction.Omit(clause.Associations).Create(&record).Error; err != nil {
			return err
		}
		if preparation.Invocation.ManifestID != preparation.Manifest.ManifestID ||
			preparation.Invocation.ManifestHash != preparation.Manifest.ManifestHash ||
			preparation.Invocation.Stage != domain.EpisodeSegmentationStage {
			return errors.New("Episode segmentation invocation does not belong to its manifest")
		}
		invocation, err := invocationRecord(preparation.Invocation)
		if err != nil {
			return err
		}
		if err = transaction.Omit(clause.Associations).Create(&invocation).Error; err != nil {
			return err
		}
		state, err = episodeSegmentationState(transaction, preparation.Manifest)
		return err
	})
	return state, err
}

func (store *Store) ClaimNextEpisodeSegmentation(
	ctx context.Context,
	now time.Time,
	leaseExpiresAt time.Time,
) (domain.Invocation, bool, error) {
	if !leaseExpiresAt.After(now) {
		return domain.Invocation{}, false, errors.New("Episode segmentation invocation lease must expire after claim time")
	}
	var result domain.Invocation
	found := false
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var record model.AgentInvocation
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("kind = ? AND stage = ?", "storygraph_stage", domain.EpisodeSegmentationStage).
			Where("workflow_run_id IS NOT NULL AND node_run_id IS NOT NULL AND shard_manifest_id IS NOT NULL").
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

func (store *Store) ValidateEpisodeSegmentationInvocation(
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
		return errors.New("Episode segmentation invocation claim is stale")
	}
	_, _, _, err = loadEpisodeSegmentationInvocation(ctx, store.database, invocation)
	return err
}

func (store *Store) CompleteEpisodeSegmentationInvocation(
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
		request, seed, input, err := loadEpisodeSegmentationInvocation(ctx, transaction, invocation)
		if err != nil {
			return err
		}
		allowed, markers, err := episodeSegmentationValidationEvidence(input, seed.NormalizedText)
		if err != nil {
			return err
		}
		if _, err = domain.DecodeEpisodeSegmentationCandidate(result.Candidate, seed.NormalizedText, allowed, markers); err != nil {
			return fmt.Errorf("%w: %v", application.ErrEpisodeSegmentationCandidateInvalid, err)
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
		applied = true
		return nil
	})
	return applied, err
}

func (store *Store) FailEpisodeSegmentationInvocation(
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

func loadEpisodeSegmentationInvocation(
	ctx context.Context,
	database *gorm.DB,
	invocation model.AgentInvocation,
) (agentcontract.StageInvocation, application.EpisodeSegmentationSeed, agentcontract.EpisodeSegmentationStageInput, error) {
	request, err := agentgorm.StageInvocation(invocation)
	if err != nil || agentcontract.ValidateEpisodeSegmentationInvocation(request) != nil ||
		invocation.Stage != domain.EpisodeSegmentationStage || invocation.ShardManifestID == nil ||
		invocation.ShardManifestVersion == nil || invocation.WorkflowRunID == nil || invocation.NodeRunID == nil {
		return agentcontract.StageInvocation{}, application.EpisodeSegmentationSeed{}, agentcontract.EpisodeSegmentationStageInput{}, errors.New("invalid Episode segmentation invocation")
	}
	var input agentcontract.EpisodeSegmentationStageInput
	if err = json.Unmarshal(request.Payload.StageInput, &input); err != nil {
		return agentcontract.StageInvocation{}, application.EpisodeSegmentationSeed{}, input, err
	}
	var manifestRecord model.ShardManifest
	if err = database.WithContext(ctx).First(&manifestRecord, "id = ? AND version = ?", *invocation.ShardManifestID, *invocation.ShardManifestVersion).Error; err != nil {
		return agentcontract.StageInvocation{}, application.EpisodeSegmentationSeed{}, input, err
	}
	manifest, err := episodeSegmentationManifestDomain(manifestRecord)
	if err != nil || manifest.ManifestHash != invocation.ShardManifestHash || manifest.NodeRunID != invocation.NodeRunID.String() {
		return agentcontract.StageInvocation{}, application.EpisodeSegmentationSeed{}, input, errors.New("Episode segmentation manifest has drifted")
	}
	seed, err := loadEpisodeSegmentationSeed(ctx, database, application.EpisodeSegmentationCommand{
		WorkspaceID: request.Payload.WorkspaceID, ProjectID: request.Payload.ProjectID,
		WorkflowRunID: invocation.WorkflowRunID.String(), NodeRunID: invocation.NodeRunID.String(),
		DocumentRevisionID: input.DocumentRevisionID, DocumentRevisionHash: input.NormalizedHash,
		EvidenceCandidateRevisionID:   input.EvidenceAggregateRevisionID,
		EvidenceCandidateRevisionHash: input.EvidenceAggregateRevisionHash,
		BibleVersionID:                input.BibleVersionID, BibleVersion: input.BibleVersion,
		MaterializationHash: input.MaterializationHash,
	})
	if err != nil {
		return agentcontract.StageInvocation{}, application.EpisodeSegmentationSeed{}, input, err
	}
	rootHash, err := application.EpisodeSegmentationRootInputHash(seed)
	if err != nil || rootHash != manifest.RootInputHash {
		return agentcontract.StageInvocation{}, application.EpisodeSegmentationSeed{}, input, errors.New("Episode segmentation frozen inputs have drifted")
	}
	return request, seed, input, nil
}

func episodeSegmentationValidationEvidence(
	input agentcontract.EpisodeSegmentationStageInput,
	normalizedText string,
) ([]domain.Evidence, []domain.EpisodeSegmentationMarker, error) {
	allowed := make([]domain.Evidence, 0, len(input.EvidenceIndex)+len(input.MarkerHints))
	for _, item := range input.EvidenceIndex {
		allowed = append(allowed, episodeSegmentationDomainEvidence(item.Evidence))
	}
	markers := make([]domain.EpisodeSegmentationMarker, 0, len(input.MarkerHints))
	for _, marker := range input.MarkerHints {
		value := episodeSegmentationDomainEvidence(marker.Evidence)
		allowed = append(allowed, value)
		markers = append(markers, domain.EpisodeSegmentationMarker{EpisodeNumber: marker.EpisodeNumber, Label: marker.Label, Evidence: value})
	}
	if err := domain.ValidateEpisodeSegmentationEvidence(allowed, normalizedText); err != nil {
		return nil, nil, err
	}
	return allowed, markers, nil
}

func episodeSegmentationDomainEvidence(value agentcontract.EpisodeSegmentationEvidence) domain.Evidence {
	return domain.Evidence{
		SourceStart: value.SourceStart, SourceEnd: value.SourceEnd, TextHash: value.TextHash,
		ExactAnchor: value.ExactAnchor, EpisodeNumber: value.EpisodeNumber,
	}
}

func episodeSegmentationState(
	database *gorm.DB,
	manifest domain.EpisodeSegmentationManifest,
) (application.EpisodeSegmentationState, error) {
	state := application.EpisodeSegmentationState{
		Status: "pending", ManifestID: manifest.ManifestID, ManifestVersion: manifest.Version, ManifestHash: manifest.ManifestHash,
	}
	var invocation model.AgentInvocation
	if err := database.First(&invocation, "shard_manifest_id = ? AND shard_manifest_version = ?", manifest.ManifestID, manifest.Version).Error; err != nil {
		return application.EpisodeSegmentationState{}, err
	}
	var head model.StageCandidateHead
	if err := database.First(&head, "stage_instance_key = ?", invocation.StageInstanceKey).Error; err == nil {
		state.Status = "ready"
		state.CandidateRevisionID = head.CurrentRevisionID.String()
		state.CandidateRevisionHash = head.CurrentCandidateRevisionHash
		state.CandidateRevisionNo = head.Revision
		return state, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return application.EpisodeSegmentationState{}, err
	}
	if invocation.Status == "failed" {
		state.Status = "failed"
	}
	return state, nil
}

func episodeSegmentationManifestRecord(value domain.EpisodeSegmentationManifest, createdAt time.Time) (model.ShardManifest, error) {
	if err := domain.ValidateEpisodeSegmentationManifest(value); err != nil {
		return model.ShardManifest{}, err
	}
	id, idErr := uuid.Parse(value.ManifestID)
	workspaceID, workspaceErr := uuid.Parse(value.WorkspaceID)
	runID, runErr := uuid.Parse(value.WorkflowRunID)
	nodeID, nodeErr := uuid.Parse(value.NodeRunID)
	if idErr != nil || workspaceErr != nil || runErr != nil || nodeErr != nil {
		return model.ShardManifest{}, errors.New("invalid Episode segmentation manifest identity")
	}
	shards, err := json.Marshal([]domain.EpisodeSegmentationShard{value.Shard})
	if err != nil {
		return model.ShardManifest{}, err
	}
	return model.ShardManifest{
		ID: id, Version: value.Version, WorkspaceID: workspaceID, WorkflowRunID: runID,
		NodeRunID: nodeID, Stage: value.Stage, RootInputHash: value.RootInputHash,
		Shards: datatypes.JSON(shards), CoverageHash: value.CoverageHash,
		ManifestHash: value.ManifestHash, CreatedAt: createdAt,
	}, nil
}

func episodeSegmentationManifestDomain(record model.ShardManifest) (domain.EpisodeSegmentationManifest, error) {
	var shards []domain.EpisodeSegmentationShard
	if err := json.Unmarshal(record.Shards, &shards); err != nil || len(shards) != 1 {
		return domain.EpisodeSegmentationManifest{}, errors.New("invalid persisted Episode segmentation shard")
	}
	value := domain.EpisodeSegmentationManifest{
		ManifestID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(),
		WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(),
		Stage: record.Stage, Version: record.Version, RootInputHash: record.RootInputHash,
		CoverageHash: record.CoverageHash, ManifestHash: record.ManifestHash, Shard: shards[0],
	}
	return value, domain.ValidateEpisodeSegmentationManifest(value)
}
