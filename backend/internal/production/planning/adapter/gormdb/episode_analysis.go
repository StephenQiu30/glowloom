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
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

func (store *Store) LoadEpisodeAnalysisSeed(
	ctx context.Context,
	command application.EpisodeAnalysisCommand,
) (application.EpisodeAnalysisSeed, error) {
	return loadEpisodeAnalysisSeed(ctx, store.database, command)
}

func loadEpisodeAnalysisSeed(
	ctx context.Context,
	database *gorm.DB,
	command application.EpisodeAnalysisCommand,
) (application.EpisodeAnalysisSeed, error) {
	runID, err := uuid.Parse(command.WorkflowRunID)
	if err != nil {
		return application.EpisodeAnalysisSeed{}, errors.New("invalid Episode analysis WorkflowRun")
	}
	nodeID, err := uuid.Parse(command.NodeRunID)
	if err != nil {
		return application.EpisodeAnalysisSeed{}, errors.New("invalid Episode analysis NodeRun")
	}
	var run model.WorkflowRun
	if err = database.WithContext(ctx).First(&run, "id = ?", runID).Error; err != nil {
		return application.EpisodeAnalysisSeed{}, errors.New("Episode analysis requires an existing WorkflowRun")
	}
	var node model.NodeRunProjection
	if err = database.WithContext(ctx).First(&node, "id = ?", nodeID).Error; err != nil {
		return application.EpisodeAnalysisSeed{}, errors.New("Episode analysis requires an existing NodeRun")
	}
	if run.WorkspaceID.String() != command.WorkspaceID || run.ProjectID.String() != command.ProjectID ||
		node.WorkspaceID != run.WorkspaceID || node.WorkflowRunID != run.ID ||
		node.Executor != "activity.episode_analysis" ||
		node.Status == "FAILED" || node.Status == "CANCELLED" || node.Status == "SKIPPED" || node.Status == "CACHED" {
		return application.EpisodeAnalysisSeed{}, errors.New("Episode analysis WorkflowRun or NodeRun has drifted")
	}
	setID, err := uuid.Parse(command.EpisodeSetID)
	if err != nil {
		return application.EpisodeAnalysisSeed{}, errors.New("invalid Episode set receipt")
	}
	var setReceipt model.CommandReceipt
	if err = database.WithContext(ctx).First(&setReceipt, "id = ?", setID).Error; err != nil {
		return application.EpisodeAnalysisSeed{}, errors.New("Episode analysis requires an Episode set receipt")
	}
	var set application.EpisodeSetReference
	if err = json.Unmarshal(setReceipt.Result, &set); err != nil {
		return application.EpisodeAnalysisSeed{}, errors.New("Episode set receipt is invalid")
	}
	setHash, err := platformcommand.InputHash(set.Episodes)
	if err != nil {
		return application.EpisodeAnalysisSeed{}, err
	}
	if setReceipt.WorkspaceID != run.WorkspaceID || setReceipt.Operation != "episode_plan.apply" ||
		set.ID != setReceipt.ID.String() || set.WorkspaceID != command.WorkspaceID || set.ProjectID != command.ProjectID ||
		set.ContentHash != setHash || set.ContentHash != command.EpisodeSetHash || len(set.Episodes) == 0 {
		return application.EpisodeAnalysisSeed{}, application.ErrEpisodeAnalysisUpstreamStale
	}
	bibleID, err := uuid.Parse(command.BibleVersionID)
	if err != nil {
		return application.EpisodeAnalysisSeed{}, errors.New("invalid Production Bible Version")
	}
	var bible model.ProductionBibleVersion
	if err = database.WithContext(ctx).First(&bible, "id = ?", bibleID).Error; err != nil {
		return application.EpisodeAnalysisSeed{}, errors.New("Episode analysis requires a Production Bible Version")
	}
	if bible.WorkspaceID != run.WorkspaceID || bible.ProjectID != run.ProjectID || bible.Version != command.BibleVersion ||
		bible.DocumentRevisionID.String() != set.DocumentRevisionID ||
		bible.DocumentRevisionHash != set.DocumentRevisionHash {
		return application.EpisodeAnalysisSeed{}, application.ErrEpisodeAnalysisUpstreamStale
	}
	var materializationReceipt model.CommandReceipt
	if err = database.WithContext(ctx).
		Where("workspace_id = ? AND operation = ? AND resource_id = ?", run.WorkspaceID, "production_bible.materialize_confirmed", bible.ID).
		First(&materializationReceipt).Error; err != nil {
		return application.EpisodeAnalysisSeed{}, errors.New("Episode analysis requires a Bible Materialization receipt")
	}
	var materializationResult struct {
		Materialization bibledomain.Materialization `json:"materialization"`
	}
	if err = json.Unmarshal(materializationReceipt.Result, &materializationResult); err != nil {
		return application.EpisodeAnalysisSeed{}, errors.New("Bible Materialization receipt is invalid")
	}
	materialization := materializationResult.Materialization
	verifiedMaterialization, err := bibledomain.NewMaterialization(
		materialization.BibleVersionID,
		materialization.BibleVersionHash,
		materialization.Assets,
		materialization.Specifications,
		materialization.States,
		materialization.Bindings,
	)
	if err != nil || verifiedMaterialization.ContentHash != materialization.ContentHash ||
		materialization.BibleVersionID != bible.ID.String() || materialization.BibleVersionHash != bible.ContentHash ||
		materialization.ContentHash != command.MaterializationHash {
		return application.EpisodeAnalysisSeed{}, application.ErrEpisodeAnalysisUpstreamStale
	}
	knownIdentities, err := episodeKnownIdentities(materialization)
	if err != nil {
		return application.EpisodeAnalysisSeed{}, err
	}
	documentID, err := uuid.Parse(set.DocumentRevisionID)
	if err != nil {
		return application.EpisodeAnalysisSeed{}, application.ErrEpisodeAnalysisUpstreamStale
	}
	var revision model.DocumentRevision
	if err = database.WithContext(ctx).First(&revision, "id = ?", documentID).Error; err != nil {
		return application.EpisodeAnalysisSeed{}, err
	}
	if revision.WorkspaceID != run.WorkspaceID || revision.NormalizedHash != set.DocumentRevisionHash {
		return application.EpisodeAnalysisSeed{}, application.ErrEpisodeAnalysisUpstreamStale
	}
	var blocks []domain.Block
	if err = json.Unmarshal(revision.Blocks, &blocks); err != nil {
		return application.EpisodeAnalysisSeed{}, err
	}
	episodeSeeds := make([]application.EpisodeAnalysisEpisodeSeed, len(set.Episodes))
	previousEnd := -1
	for index, reference := range set.Episodes {
		episodeID, parseErr := uuid.Parse(reference.EpisodeID)
		if parseErr != nil {
			return application.EpisodeAnalysisSeed{}, application.ErrEpisodeAnalysisUpstreamStale
		}
		versionID, parseErr := uuid.Parse(reference.ScriptVersionID)
		if parseErr != nil {
			return application.EpisodeAnalysisSeed{}, application.ErrEpisodeAnalysisUpstreamStale
		}
		var episode model.Episode
		if err = database.WithContext(ctx).First(&episode, "id = ?", episodeID).Error; err != nil {
			return application.EpisodeAnalysisSeed{}, err
		}
		var version model.EpisodeScriptVersion
		if err = database.WithContext(ctx).First(&version, "id = ?", versionID).Error; err != nil {
			return application.EpisodeAnalysisSeed{}, err
		}
		if reference.Position != index+1 || episode.WorkspaceID != run.WorkspaceID || episode.ProjectID != run.ProjectID ||
			episode.Position != reference.Position || episode.Status != "active" || episode.CurrentScriptVersionID == nil ||
			*episode.CurrentScriptVersionID != version.ID || version.WorkspaceID != run.WorkspaceID ||
			version.ProjectID != run.ProjectID || version.EpisodeID != episode.ID || version.Status != "published" ||
			version.DocumentRevisionID != revision.ID || version.ContentHash != reference.ContentHash ||
			version.ContentHash != bibledomain.SourceTextHash(version.Content) || version.SourceStart != reference.SourceStart ||
			version.SourceEnd != reference.SourceEnd || previousEnd >= 0 && version.SourceStart != previousEnd {
			return application.EpisodeAnalysisSeed{}, application.ErrEpisodeAnalysisUpstreamStale
		}
		markers := episodeSceneMarkers(version, blocks)
		episodeSeeds[index] = application.EpisodeAnalysisEpisodeSeed{
			Source: domain.EpisodeAnalysisSource{
				EpisodeID: episode.ID.String(), EpisodePosition: episode.Position,
				ScriptVersionID: version.ID.String(), ContentHash: version.ContentHash,
				SourceStart: version.SourceStart, SourceEnd: version.SourceEnd,
				Content: version.Content, SceneMarkers: markers,
			},
			ScriptVersionNo: version.VersionNo, DocumentRevisionID: version.DocumentRevisionID.String(),
		}
		previousEnd = version.SourceEnd
	}
	return application.EpisodeAnalysisSeed{
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		EpisodeSetID: set.ID, EpisodeSetHash: set.ContentHash, Episodes: episodeSeeds,
		BibleVersionID: bible.ID.String(), BibleVersion: bible.Version,
		BibleContentHash: bible.ContentHash, BibleSnapshotHash: bible.CandidateContentHash,
		BibleSnapshot:       append(json.RawMessage(nil), bible.Snapshot...),
		MaterializationHash: materialization.ContentHash, KnownIdentities: knownIdentities,
	}, nil
}

func episodeSceneMarkers(
	version model.EpisodeScriptVersion,
	blocks []domain.Block,
) []domain.EpisodeSceneMarker {
	content := []rune(version.Content)
	result := make([]domain.EpisodeSceneMarker, 0)
	for _, block := range blocks {
		if block.Kind != "scene_heading" || block.SourceStart < version.SourceStart || block.SourceEnd > version.SourceEnd ||
			block.SourceEnd <= block.SourceStart {
			continue
		}
		result = append(result, domain.EpisodeSceneMarker{
			Label:         string(content[block.SourceStart-version.SourceStart : block.SourceEnd-version.SourceStart]),
			AbsoluteStart: block.SourceStart, AbsoluteEnd: block.SourceEnd,
		})
	}
	slices.SortFunc(result, func(left, right domain.EpisodeSceneMarker) int {
		return left.AbsoluteStart - right.AbsoluteStart
	})
	return result
}

func episodeKnownIdentities(
	materialization bibledomain.Materialization,
) ([]agentcontract.EpisodeKnownIdentity, error) {
	assets := make(map[string]bibledomain.MaterializedAsset, len(materialization.Assets))
	for _, asset := range materialization.Assets {
		assets[asset.ID] = asset
	}
	specifications := make(map[string]bibledomain.MaterializedSpecification, len(materialization.Specifications))
	for _, specification := range materialization.Specifications {
		specifications[specification.ID] = specification
	}
	states := make(map[string][]agentcontract.EpisodeKnownState)
	for _, state := range materialization.States {
		states[state.AssetID] = append(states[state.AssetID], agentcontract.EpisodeKnownState{
			StateKey: state.StateKey, AssetStateID: state.ID, ContentHash: state.ContentHash,
		})
	}
	result := make([]agentcontract.EpisodeKnownIdentity, len(materialization.Bindings))
	for index, binding := range materialization.Bindings {
		asset, assetExists := assets[binding.AssetID]
		specification, specificationExists := specifications[binding.SpecificationVersionID]
		if !assetExists || !specificationExists || len(states[binding.AssetID]) == 0 {
			return nil, errors.New("Bible Materialization identity index is incomplete")
		}
		slices.SortFunc(states[binding.AssetID], func(left, right agentcontract.EpisodeKnownState) int {
			return strings.Compare(left.StateKey, right.StateKey)
		})
		result[index] = agentcontract.EpisodeKnownIdentity{
			EntityKey: binding.EntityKey, Kind: asset.Kind, AssetID: asset.ID,
			SpecificationVersionID: specification.ID, SpecificationHash: specification.ContentHash,
			States: states[binding.AssetID],
		}
	}
	slices.SortFunc(result, func(left, right agentcontract.EpisodeKnownIdentity) int {
		return strings.Compare(left.EntityKey, right.EntityKey)
	})
	return result, nil
}

func (store *Store) EnsureEpisodeAnalysis(
	ctx context.Context,
	preparation application.EpisodeAnalysisPreparation,
) (application.EpisodeAnalysisState, error) {
	if err := domain.ValidateEpisodeAnalysisManifests(
		preparation.AnalyzeManifest,
		preparation.ReconcileManifest,
	); err != nil {
		return application.EpisodeAnalysisState{}, err
	}
	var state application.EpisodeAnalysisState
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		seed, err := loadEpisodeAnalysisSeed(ctx, transaction, preparation.Command)
		if err != nil {
			return err
		}
		rootHash, err := application.EpisodeAnalysisRootInputHash(seed)
		if err != nil || rootHash != preparation.AnalyzeManifest.RootInputHash {
			return application.ErrEpisodeAnalysisUpstreamStale
		}
		var existing []model.ShardManifest
		if err = transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("node_run_id = ? AND stage IN ?", preparation.Command.NodeRunID,
				[]string{domain.AnalyzeEpisodeStage, domain.ReconcileEpisodeStage}).
			Order("stage").Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) == 0 {
			analyzeRecord, recordErr := episodeAnalysisManifestRecord(preparation.AnalyzeManifest, preparation.CreatedAt)
			if recordErr != nil {
				return recordErr
			}
			reconcileRecord, recordErr := episodeReconcileManifestRecord(preparation.ReconcileManifest, preparation.CreatedAt)
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
				return errors.New("Episode analysis preparation has incomplete map invocations")
			}
			for _, invocation := range preparation.Invocations {
				if invocation.ManifestID != preparation.AnalyzeManifest.ManifestID ||
					invocation.ManifestHash != preparation.AnalyzeManifest.ManifestHash ||
					invocation.Stage != domain.AnalyzeEpisodeStage {
					return errors.New("Episode analysis invocation does not belong to its manifest")
				}
				record, recordErr := episodeInvocationRecord(invocation)
				if recordErr != nil {
					return recordErr
				}
				if err = transaction.Omit(clause.Associations).Create(&record).Error; err != nil {
					return err
				}
			}
		} else {
			if len(existing) != 2 {
				return errors.New("Episode analysis manifest pair is incomplete")
			}
			for _, record := range existing {
				switch record.Stage {
				case domain.AnalyzeEpisodeStage:
					if record.ManifestHash != preparation.AnalyzeManifest.ManifestHash {
						return errors.New("Episode analysis manifest changed for the existing NodeRun")
					}
				case domain.ReconcileEpisodeStage:
					if record.ManifestHash != preparation.ReconcileManifest.ManifestHash {
						return errors.New("Episode reconciliation manifest changed for the existing NodeRun")
					}
				default:
					return errors.New("unexpected Episode analysis manifest stage")
				}
			}
		}
		analyze, reconcile, err := loadEpisodeManifestPair(transaction, preparation.Command.NodeRunID, true)
		if err != nil {
			return err
		}
		if err = domain.ValidateEpisodeAnalysisManifests(analyze, reconcile); err != nil {
			return err
		}
		if err = scheduleEpisodeReconcile(transaction, analyze, reconcile, preparation.CreatedAt); err != nil {
			return err
		}
		if err = ensureEpisodePlanningAggregate(transaction, reconcile, preparation.CreatedAt); err != nil {
			return err
		}
		state, err = episodeAnalysisState(transaction, reconcile)
		return err
	})
	return state, err
}

func (store *Store) ClaimNextEpisodeAnalysis(
	ctx context.Context,
	now time.Time,
	leaseExpiresAt time.Time,
) (bibledomain.Invocation, bool, error) {
	if !leaseExpiresAt.After(now) {
		return bibledomain.Invocation{}, false, errors.New("Episode analysis invocation lease must expire after claim time")
	}
	var result bibledomain.Invocation
	found := false
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var record model.AgentInvocation
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("request_type IN ?", []string{"episode_analysis_shard", "episode_reconcile_shard"}).
			Where("kind = ? AND stage IN ?", "storygraph_stage", []string{domain.AnalyzeEpisodeStage, domain.ReconcileEpisodeStage}).
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
		result, found = episodeInvocationDomain(record), true
		return nil
	})
	return result, found, err
}

func (store *Store) ValidateEpisodeAnalysisInvocation(
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
	if !activeEpisodeInvocationClaim(invocation, claimVersion, now) {
		return errors.New("Episode analysis invocation claim is stale")
	}
	request, err := validateEpisodeInvocation(store.database.WithContext(ctx), invocation, false)
	if err != nil {
		return err
	}
	return validateEpisodeInvocationSources(store.database.WithContext(ctx), request)
}

func (store *Store) CompleteEpisodeAnalysisInvocation(
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
		if !activeEpisodeInvocationClaim(invocation, claimVersion, now) {
			return nil
		}
		request, err := validateEpisodeInvocation(transaction, invocation, true)
		if err != nil {
			if errors.Is(err, application.ErrEpisodeAnalysisManifestStale) {
				applied = true
				return failEpisodeInvocation(
					transaction, invocation, "failed", "manifest_superseded",
					"A newer Episode analysis manifest replaced this invocation", false, now,
				)
			}
			return err
		}
		if err = validateEpisodeInvocationSources(transaction, request); err != nil {
			if errors.Is(err, application.ErrEpisodeAnalysisUpstreamStale) {
				applied = true
				return failEpisodeInvocation(
					transaction, invocation, "failed", "upstream_candidate_stale", err.Error(), false, now,
				)
			}
			return err
		}
		if err = validateEpisodeCandidateResult(transaction, request, result); err != nil {
			code := "candidate_schema_invalid"
			if strings.Contains(err.Error(), "Evidence") {
				code = "evidence_invalid"
			} else if strings.Contains(err.Error(), "identity") || strings.Contains(err.Error(), "state") {
				code = "unknown_identity"
			}
			applied = true
			return failEpisodeInvocation(transaction, invocation, "failed", code, err.Error(), false, now)
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
			return errors.New("Episode analysis invocation has no NodeRun")
		}
		analyze, reconcile, err := loadEpisodeManifestPair(transaction, invocation.NodeRunID.String(), true)
		if err != nil {
			return err
		}
		if err = scheduleEpisodeReconcile(transaction, analyze, reconcile, now); err != nil {
			return err
		}
		if err = ensureEpisodePlanningAggregate(transaction, reconcile, now); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

func (store *Store) FailEpisodeAnalysisInvocation(
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
		if !activeEpisodeInvocationClaim(invocation, claimVersion, now) {
			return nil
		}
		if err := failEpisodeInvocation(transaction, invocation, outcome, code, summary, retryable, now); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

func failEpisodeInvocation(
	database *gorm.DB,
	invocation model.AgentInvocation,
	outcome, code, summary string,
	retryable bool,
	now time.Time,
) error {
	if outcome != "failed" && outcome != "unknown" {
		outcome = "failed"
	}
	if retryable {
		outcome = "unknown"
	}
	errorJSON, err := json.Marshal(map[string]any{
		"code": code, "summary": summary, "retryable": retryable,
	})
	if err != nil {
		return err
	}
	updates := map[string]any{
		"status": outcome, "error": datatypes.JSON(errorJSON), "lease_expires_at": nil,
		"updated_at": now,
	}
	if outcome == "failed" {
		updates["completed_at"] = now
	} else {
		updates["completed_at"] = nil
	}
	return database.Model(&invocation).Updates(updates).Error
}

func activeEpisodeInvocationClaim(value model.AgentInvocation, claimVersion int, now time.Time) bool {
	return value.Status == "running" && value.ClaimVersion == claimVersion &&
		value.LeaseExpiresAt != nil && value.LeaseExpiresAt.After(now)
}

func validateEpisodeInvocation(
	database *gorm.DB,
	invocation model.AgentInvocation,
	lock bool,
) (agentcontract.StageInvocation, error) {
	if invocation.NodeRunID == nil || invocation.ShardManifestID == nil || invocation.ShardManifestVersion == nil {
		return agentcontract.StageInvocation{}, application.ErrEpisodeAnalysisManifestStale
	}
	query := database
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var manifest model.ShardManifest
	if err := query.Where("node_run_id = ? AND stage = ?", invocation.NodeRunID, invocation.Stage).
		Order("version DESC").First(&manifest).Error; err != nil {
		return agentcontract.StageInvocation{}, err
	}
	if manifest.ID != *invocation.ShardManifestID || manifest.Version != *invocation.ShardManifestVersion ||
		manifest.ManifestHash != invocation.ShardManifestHash {
		return agentcontract.StageInvocation{}, application.ErrEpisodeAnalysisManifestStale
	}
	request, err := agentgorm.StageInvocation(invocation)
	if err != nil {
		return agentcontract.StageInvocation{}, err
	}
	if err = agentcontract.ValidateEpisodeAnalysisInvocation(request); err != nil {
		return agentcontract.StageInvocation{}, err
	}
	return request, nil
}

func validateEpisodeInvocationSources(
	database *gorm.DB,
	request agentcontract.StageInvocation,
) error {
	for _, ref := range request.Payload.SourceRefs {
		versionID, err := uuid.Parse(ref.OwnerVersionID)
		if err != nil {
			return application.ErrEpisodeAnalysisUpstreamStale
		}
		switch ref.OwnerKind {
		case "production/episode-script":
			var version model.EpisodeScriptVersion
			if err = database.First(&version, "id = ?", versionID).Error; err != nil {
				return application.ErrEpisodeAnalysisUpstreamStale
			}
			var episode model.Episode
			if err = database.First(&episode, "id = ?", version.EpisodeID).Error; err != nil {
				return application.ErrEpisodeAnalysisUpstreamStale
			}
			if version.EpisodeID.String() != ref.OwnerLogicalID || version.VersionNo != int(ref.Revision) ||
				version.ContentHash != ref.ContentHash || version.Status != "published" ||
				episode.CurrentScriptVersionID == nil || *episode.CurrentScriptVersionID != version.ID {
				return application.ErrEpisodeAnalysisUpstreamStale
			}
		case "production/bible-version":
			var bible model.ProductionBibleVersion
			if err = database.First(&bible, "id = ?", versionID).Error; err != nil ||
				bible.ID.String() != ref.OwnerLogicalID || bible.Version != int(ref.Revision) ||
				bible.ContentHash != ref.ContentHash {
				return application.ErrEpisodeAnalysisUpstreamStale
			}
		case "production/bible-materialization":
			var receipt model.CommandReceipt
			if err = database.Where("operation = ? AND resource_id = ?", "production_bible.materialize_confirmed", versionID).
				First(&receipt).Error; err != nil {
				return application.ErrEpisodeAnalysisUpstreamStale
			}
			var result struct {
				Materialization bibledomain.Materialization `json:"materialization"`
			}
			if err = json.Unmarshal(receipt.Result, &result); err != nil ||
				result.Materialization.BibleVersionID != ref.OwnerLogicalID ||
				result.Materialization.ContentHash != ref.ContentHash {
				return application.ErrEpisodeAnalysisUpstreamStale
			}
		default:
			return application.ErrEpisodeAnalysisUpstreamStale
		}
	}
	for _, upstream := range request.Payload.UpstreamCandidates {
		revisionID, err := uuid.Parse(upstream.CandidateRevisionID)
		if err != nil {
			return application.ErrEpisodeAnalysisUpstreamStale
		}
		var revision model.StageCandidateRevision
		if err = database.First(&revision, "id = ?", revisionID).Error; err != nil {
			return application.ErrEpisodeAnalysisUpstreamStale
		}
		var head model.StageCandidateHead
		if err = database.First(&head, "stage_instance_key = ?", revision.StageInstanceKey).Error; err != nil ||
			revision.CandidateRevisionHash != upstream.CandidateRevisionHash ||
			head.CurrentRevisionID != revision.ID || head.CurrentCandidateRevisionHash != revision.CandidateRevisionHash {
			return application.ErrEpisodeAnalysisUpstreamStale
		}
	}
	return nil
}

func validateEpisodeCandidateResult(
	database *gorm.DB,
	request agentcontract.StageInvocation,
	result agentcontract.StageResult,
) error {
	switch request.Payload.Stage {
	case domain.AnalyzeEpisodeStage:
		if result.CandidateType != "episode_analysis_candidate" {
			return errors.New("Episode analysis returned the wrong candidate type")
		}
		var input agentcontract.EpisodeAnalysisStageInput
		if err := json.Unmarshal(request.Payload.StageInput, &input); err != nil {
			return err
		}
		_, err := domain.DecodeEpisodeAnalysisCandidate(result.Candidate, episodeAnalysisScope(input))
		return err
	case domain.ReconcileEpisodeStage:
		if result.CandidateType != "episode_reconciliation_candidate" {
			return errors.New("Episode reconciliation returned the wrong candidate type")
		}
		var input agentcontract.EpisodeReconciliationStageInput
		if err := json.Unmarshal(request.Payload.StageInput, &input); err != nil {
			return err
		}
		analysisChildren := make([]domain.EpisodeAnalysisCandidate, 0, len(input.Candidates))
		reconcileChildren := make([]domain.EpisodeReconciliationCandidate, 0, len(input.Candidates))
		for _, child := range input.Candidates {
			revisionID, err := uuid.Parse(child.CandidateRevisionID)
			if err != nil {
				return err
			}
			var revision model.StageCandidateRevision
			if err = database.First(&revision, "id = ?", revisionID).Error; err != nil {
				return err
			}
			if revision.CandidateRevisionHash != child.CandidateRevisionHash ||
				!jsonEqual(revision.Candidate, child.Candidate) {
				return application.ErrEpisodeAnalysisUpstreamStale
			}
			switch input.CandidateType {
			case "episode_analysis_candidate":
				var candidate domain.EpisodeAnalysisCandidate
				if err = json.Unmarshal(revision.Candidate, &candidate); err != nil {
					return err
				}
				analysisChildren = append(analysisChildren, candidate)
			case "episode_reconciliation_candidate":
				var candidate domain.EpisodeReconciliationCandidate
				if err = json.Unmarshal(revision.Candidate, &candidate); err != nil {
					return err
				}
				reconcileChildren = append(reconcileChildren, candidate)
			default:
				return errors.New("invalid Episode reconciliation child type")
			}
		}
		var version model.EpisodeScriptVersion
		if err := database.First(&version, "id = ?", input.ScriptVersionID).Error; err != nil {
			return err
		}
		scope := episodeReconciliationScope(input, version.Content)
		_, err := domain.DecodeEpisodeReconciliationCandidate(
			result.Candidate,
			scope,
			analysisChildren,
			reconcileChildren,
		)
		return err
	default:
		return errors.New("unsupported Episode analysis stage")
	}
}

func episodeAnalysisScope(input agentcontract.EpisodeAnalysisStageInput) domain.EpisodeCandidateScope {
	return domain.EpisodeCandidateScope{
		EpisodeID: input.EpisodeID, EpisodePosition: input.EpisodePosition,
		ScriptVersionID: input.ScriptVersionID, SourceStart: input.LogicalStart, SourceEnd: input.LogicalEnd,
		ContextStart: input.ContextStart, ContextText: input.ContextText,
		KnownIdentities: episodeCandidateIdentities(input.KnownIdentities),
	}
}

func episodeReconciliationScope(
	input agentcontract.EpisodeReconciliationStageInput,
	content string,
) domain.EpisodeCandidateScope {
	return domain.EpisodeCandidateScope{
		EpisodeID: input.EpisodeID, EpisodePosition: input.EpisodePosition,
		ScriptVersionID: input.ScriptVersionID,
		SourceStart:     input.EpisodeSourceStart, SourceEnd: input.EpisodeSourceEnd,
		ContextStart: input.EpisodeSourceStart, ContextText: content,
		KnownIdentities: episodeCandidateIdentities(input.KnownIdentities),
	}
}

func episodeCandidateIdentities(
	values []agentcontract.EpisodeKnownIdentity,
) []domain.EpisodeCandidateIdentity {
	result := make([]domain.EpisodeCandidateIdentity, len(values))
	for index, value := range values {
		states := make([]string, len(value.States))
		for stateIndex, state := range value.States {
			states[stateIndex] = state.StateKey
		}
		result[index] = domain.EpisodeCandidateIdentity{EntityKey: value.EntityKey, StateKeys: states}
	}
	return result
}

func jsonEqual(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	leftJSON, leftErr := json.Marshal(leftValue)
	rightJSON, rightErr := json.Marshal(rightValue)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

type episodeReconcileReadyChild struct {
	Invocation model.AgentInvocation
	Revision   model.StageCandidateRevision
	Request    agentcontract.StageInvocation
}

func scheduleEpisodeReconcile(
	database *gorm.DB,
	analyze domain.EpisodeAnalysisManifest,
	reconcile domain.EpisodeReconcileManifest,
	now time.Time,
) error {
	shards := append([]domain.EpisodeReconcileShard(nil), reconcile.Shards...)
	slices.SortFunc(shards, func(left, right domain.EpisodeReconcileShard) int {
		if left.EpisodePosition != right.EpisodePosition {
			return left.EpisodePosition - right.EpisodePosition
		}
		if left.Level != right.Level {
			return left.Level - right.Level
		}
		return strings.Compare(left.Key, right.Key)
	})
	for _, shard := range shards {
		var existing model.AgentInvocation
		err := database.Where(
			"node_run_id = ? AND stage = ? AND shard_key = ? AND shard_manifest_id = ? AND shard_manifest_version = ?",
			reconcile.NodeRunID, domain.ReconcileEpisodeStage, shard.Key, reconcile.ManifestID, reconcile.Version,
		).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		children := make([]episodeReconcileReadyChild, 0, len(shard.Children))
		ready := true
		for _, child := range shard.Children {
			manifestID, manifestVersion := analyze.ManifestID, analyze.Version
			if child.Stage == domain.ReconcileEpisodeStage {
				manifestID, manifestVersion = reconcile.ManifestID, reconcile.Version
			}
			var invocation model.AgentInvocation
			err = database.Where(
				"node_run_id = ? AND stage = ? AND shard_key = ? AND shard_manifest_id = ? AND shard_manifest_version = ?",
				reconcile.NodeRunID, child.Stage, child.ShardKey, manifestID, manifestVersion,
			).First(&invocation).Error
			if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && (invocation.Status != "succeeded" || invocation.ResultHash == nil) {
				ready = false
				break
			}
			if err != nil {
				return err
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
				return application.ErrEpisodeAnalysisUpstreamStale
			}
			request, requestErr := agentgorm.StageInvocation(invocation)
			if requestErr != nil {
				return requestErr
			}
			children = append(children, episodeReconcileReadyChild{
				Invocation: invocation, Revision: revision, Request: request,
			})
		}
		if !ready {
			continue
		}
		record, err := episodeReconcileInvocationRecord(reconcile, shard, children, now)
		if err != nil {
			return err
		}
		created := database.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if created.Error != nil {
			return created.Error
		}
	}
	return nil
}

func episodeReconcileInvocationRecord(
	manifest domain.EpisodeReconcileManifest,
	shard domain.EpisodeReconcileShard,
	children []episodeReconcileReadyChild,
	now time.Time,
) (model.AgentInvocation, error) {
	if len(children) != len(shard.Children) || len(children) == 0 || len(children) > manifest.FanIn {
		return model.AgentInvocation{}, errors.New("Episode reconcile children are incomplete")
	}
	candidateType := "episode_analysis_candidate"
	if shard.Children[0].Stage == domain.ReconcileEpisodeStage {
		candidateType = "episode_reconciliation_candidate"
	}
	inputs := make([]agentcontract.EpisodeReconciliationInputCandidate, len(children))
	upstreams := make([]agentcontract.StageUpstreamCandidateRef, len(children))
	var frozen agentcontract.EpisodeReconciliationStageInput
	for index, child := range children {
		childSpec := shard.Children[index]
		if child.Invocation.Stage != childSpec.Stage || child.Invocation.ShardKey != childSpec.ShardKey ||
			child.Invocation.ResultHash == nil {
			return model.AgentInvocation{}, errors.New("Episode reconcile child ordering has drifted")
		}
		var childFrozen agentcontract.EpisodeReconciliationStageInput
		switch child.Invocation.Stage {
		case domain.AnalyzeEpisodeStage:
			var input agentcontract.EpisodeAnalysisStageInput
			if err := json.Unmarshal(child.Request.Payload.StageInput, &input); err != nil {
				return model.AgentInvocation{}, err
			}
			childFrozen = agentcontract.EpisodeReconciliationStageInput{
				EpisodeID: input.EpisodeID, EpisodePosition: input.EpisodePosition,
				ScriptVersionID: input.ScriptVersionID, ScriptVersionNo: input.ScriptVersionNo,
				EpisodeSourceStart: input.EpisodeSourceStart, EpisodeSourceEnd: input.EpisodeSourceEnd,
				ScriptContentHash: input.ScriptContentHash,
				BibleVersionID:    input.BibleVersionID, BibleVersion: input.BibleVersion,
				BibleContentHash: input.BibleContentHash, MaterializationHash: input.MaterializationHash,
				KnownIdentities: input.KnownIdentities,
			}
		case domain.ReconcileEpisodeStage:
			if err := json.Unmarshal(child.Request.Payload.StageInput, &childFrozen); err != nil {
				return model.AgentInvocation{}, err
			}
		default:
			return model.AgentInvocation{}, errors.New("unsupported Episode reconcile child stage")
		}
		if index == 0 {
			frozen = childFrozen
		} else if !sameEpisodeReconcileFrozenInput(frozen, childFrozen) {
			return model.AgentInvocation{}, errors.New("Episode reconcile children do not share one frozen Episode")
		}
		inputs[index] = agentcontract.EpisodeReconciliationInputCandidate{
			ShardKey: child.Invocation.ShardKey, CandidateRevisionID: child.Revision.ID.String(),
			CandidateRevisionHash: child.Revision.CandidateRevisionHash,
			Candidate:             json.RawMessage(child.Revision.Candidate),
		}
		upstreams[index] = agentcontract.StageUpstreamCandidateRef{
			Stage: child.Invocation.Stage, ShardKey: child.Invocation.ShardKey,
			CandidateRevisionID:   child.Revision.ID.String(),
			CandidateRevisionHash: child.Revision.CandidateRevisionHash,
			SourceInvocationID:    child.Invocation.ID.String(), SourceResultHash: *child.Invocation.ResultHash,
		}
	}
	frozen.Level = shard.Level + 1
	frozen.CandidateType = candidateType
	frozen.Candidates = inputs
	stageInput, err := json.Marshal(frozen)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	sourceRefs := []agentcontract.StageSourceRef{
		{OwnerKind: "production/episode-script", OwnerLogicalID: frozen.EpisodeID,
			OwnerVersionID: frozen.ScriptVersionID, Revision: int64(frozen.ScriptVersionNo),
			ContentHash: frozen.ScriptContentHash},
		{OwnerKind: "production/bible-version", OwnerLogicalID: frozen.BibleVersionID,
			OwnerVersionID: frozen.BibleVersionID, Revision: int64(frozen.BibleVersion),
			ContentHash: frozen.BibleContentHash},
		{OwnerKind: "production/bible-materialization", OwnerLogicalID: frozen.BibleVersionID,
			OwnerVersionID: frozen.BibleVersionID, Revision: int64(frozen.BibleVersion),
			ContentHash: frozen.MaterializationHash},
	}
	manifestID := uuid.MustParse(manifest.ManifestID)
	invocationID := uuid.NewSHA1(manifestID, []byte(fmt.Sprintf(
		"episode-reconcile\x00%d\x00%s\x00%s", manifest.Version, manifest.ManifestHash, shard.Key,
	)))
	request, err := agentcontract.NewStageInvocation(
		invocationID.String(),
		agentcontract.StoryGraphDefinition().ExecutionPolicy(),
		agentcontract.StageInvocationPayload{
			Stage: domain.ReconcileEpisodeStage, ShardKey: shard.Key,
			WorkspaceID: manifest.WorkspaceID, ProjectID: children[0].Request.Payload.ProjectID,
			SourceRefs: sourceRefs, UpstreamCandidates: upstreams,
			ShardManifestRef: agentcontract.ShardManifestRef{
				ManifestID: manifest.ManifestID, Version: manifest.Version, Hash: manifest.ManifestHash,
			},
			Shard: agentcontract.InvocationShard{
				Kind: shard.Kind, Key: shard.Key, TreePath: shard.TreePath, ParentKey: shard.ParentKey,
			},
			StageInput: stageInput,
		},
	)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	if err = agentcontract.ValidateEpisodeAnalysisInvocation(request); err != nil {
		return model.AgentInvocation{}, err
	}
	policyJSON, err := json.Marshal(request.ExecutionPolicy)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	payloadJSON, err := json.Marshal(request.Payload)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	stageKey, err := request.StageInstanceKey()
	if err != nil {
		return model.AgentInvocation{}, err
	}
	version := manifest.Version
	runID, nodeID, workspaceID := uuid.MustParse(manifest.WorkflowRunID), uuid.MustParse(manifest.NodeRunID), uuid.MustParse(manifest.WorkspaceID)
	return model.AgentInvocation{
		ID: invocationID, WorkspaceID: workspaceID, WorkflowRunID: &runID, NodeRunID: &nodeID,
		ShardManifestID: &manifestID, ShardManifestVersion: &version,
		RequestType: "episode_reconcile_shard", RequestID: invocationID,
		Kind: "storygraph_stage", WireSchemaVersion: agentcontract.StoryGraphWireSchemaVersion,
		Stage: domain.ReconcileEpisodeStage, ShardKey: shard.Key,
		StageInstanceKey: stageKey, ShardManifestHash: manifest.ManifestHash, InputHash: request.InputHash,
		ExecutionPolicy: datatypes.JSON(policyJSON), Payload: datatypes.JSON(payloadJSON),
		Status: "queued", CreatedAt: now, UpdatedAt: now,
	}, nil
}

func sameEpisodeReconcileFrozenInput(
	left agentcontract.EpisodeReconciliationStageInput,
	right agentcontract.EpisodeReconciliationStageInput,
) bool {
	left.Level, right.Level = 0, 0
	left.CandidateType, right.CandidateType = "", ""
	left.Candidates, right.Candidates = nil, nil
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func ensureEpisodePlanningAggregate(
	database *gorm.DB,
	manifest domain.EpisodeReconcileManifest,
	now time.Time,
) error {
	inputs := make([]domain.EpisodePlanningRootInput, 0, len(manifest.Roots))
	leaves := make([]agentcontract.AggregateLeafCandidateRef, 0, len(manifest.Roots))
	var frozen agentcontract.EpisodeReconciliationStageInput
	for index, root := range manifest.Roots {
		var invocation model.AgentInvocation
		err := database.Where(
			"node_run_id = ? AND stage = ? AND shard_key = ? AND shard_manifest_id = ? AND shard_manifest_version = ?",
			manifest.NodeRunID, domain.ReconcileEpisodeStage, root.ShardKey, manifest.ManifestID, manifest.Version,
		).First(&invocation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && (invocation.Status != "succeeded" || invocation.ResultHash == nil) {
			return nil
		}
		if err != nil {
			return err
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
			return application.ErrEpisodeAnalysisUpstreamStale
		}
		request, err := agentgorm.StageInvocation(invocation)
		if err != nil {
			return err
		}
		var childInput agentcontract.EpisodeReconciliationStageInput
		if err = json.Unmarshal(request.Payload.StageInput, &childInput); err != nil {
			return err
		}
		if index == 0 {
			frozen = childInput
		} else if childInput.BibleVersionID != frozen.BibleVersionID ||
			childInput.BibleVersion != frozen.BibleVersion ||
			childInput.BibleContentHash != frozen.BibleContentHash ||
			childInput.MaterializationHash != frozen.MaterializationHash {
			return application.ErrEpisodeAnalysisUpstreamStale
		}
		var candidate domain.EpisodeReconciliationCandidate
		if err = json.Unmarshal(revision.Candidate, &candidate); err != nil {
			return err
		}
		inputs = append(inputs, domain.EpisodePlanningRootInput{
			EpisodeID: root.EpisodeID, EpisodePosition: root.EpisodePosition,
			ScriptVersionID: candidate.ScriptVersionID, ShardKey: root.ShardKey,
			StageInstanceKey:    revision.StageInstanceKey,
			CandidateRevisionID: revision.ID.String(), CandidateRevisionHash: revision.CandidateRevisionHash,
			Candidate: candidate,
		})
		leaves = append(leaves, agentcontract.AggregateLeafCandidateRef{
			StageInstanceKey: revision.StageInstanceKey, ShardKey: root.ShardKey,
			CandidateRevisionID: revision.ID.String(), CandidateRevisionHash: revision.CandidateRevisionHash,
		})
	}
	_, candidateJSON, contentHash, stageKey, err := domain.BuildEpisodePlanningCandidateSet(
		manifest,
		frozen.BibleVersionID,
		frozen.BibleVersion,
		frozen.BibleContentHash,
		frozen.MaterializationHash,
		inputs,
	)
	if err != nil {
		return err
	}
	slices.SortFunc(leaves, func(left, right agentcontract.AggregateLeafCandidateRef) int {
		if left.StageInstanceKey != right.StageInstanceKey {
			return strings.Compare(left.StageInstanceKey, right.StageInstanceKey)
		}
		return strings.Compare(left.ShardKey, right.ShardKey)
	})
	origin := agentcontract.AggregateCandidateOrigin{
		ShardManifestID: manifest.ManifestID, ManifestVersion: manifest.Version,
		ShardManifestHash: manifest.ManifestHash, LeafCandidates: leaves,
	}
	revisionHash, err := (agentcontract.CandidateRevisionMaterial{
		StageInstanceKey: stageKey, RevisionNo: 1, OriginKind: "aggregate",
		AggregateOrigin: &origin, CandidateContentHash: contentHash,
	}).Hash()
	if err != nil {
		return err
	}
	var existing model.StageCandidateHead
	if err = database.First(&existing, "stage_instance_key = ?", stageKey).Error; err == nil {
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
		ID: uuid.New(), WorkspaceID: uuid.MustParse(manifest.WorkspaceID), StageInstanceKey: stageKey,
		RevisionNo: 1, OriginKind: "aggregate", AggregateOrigin: datatypes.JSON(originJSON),
		Candidate: datatypes.JSON(candidateJSON), CandidateContentHash: contentHash,
		CandidateRevisionHash: revisionHash, CreatedAt: now,
	}
	if err = database.Omit(clause.Associations).Create(&revision).Error; err != nil {
		return err
	}
	head := model.StageCandidateHead{
		WorkspaceID: revision.WorkspaceID, StageInstanceKey: stageKey,
		CurrentRevisionID: revision.ID, CurrentCandidateRevisionHash: revisionHash,
		Revision: 1, UpdatedAt: now,
	}
	return database.Omit(clause.Associations).Create(&head).Error
}

func episodeAnalysisState(
	database *gorm.DB,
	manifest domain.EpisodeReconcileManifest,
) (application.EpisodeAnalysisState, error) {
	stageKey, err := domain.EpisodePlanningCandidateSetStageInstanceKey(manifest)
	if err != nil {
		return application.EpisodeAnalysisState{}, err
	}
	var head model.StageCandidateHead
	err = database.First(&head, "stage_instance_key = ?", stageKey).Error
	if err == nil {
		var revision model.StageCandidateRevision
		if err = database.First(&revision, "id = ?", head.CurrentRevisionID).Error; err != nil {
			return application.EpisodeAnalysisState{}, err
		}
		return application.EpisodeAnalysisState{
			Status: "ready", CandidateRevisionID: revision.ID.String(),
			CandidateRevisionHash: revision.CandidateRevisionHash, CandidateRevisionNo: revision.RevisionNo,
		}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return application.EpisodeAnalysisState{}, err
	}
	var failed int64
	if err = database.Model(&model.AgentInvocation{}).
		Where("node_run_id = ? AND request_type IN ? AND status = ?", manifest.NodeRunID,
			[]string{"episode_analysis_shard", "episode_reconcile_shard"}, "failed").
		Count(&failed).Error; err != nil {
		return application.EpisodeAnalysisState{}, err
	}
	if failed > 0 {
		return application.EpisodeAnalysisState{Status: "failed"}, nil
	}
	return application.EpisodeAnalysisState{Status: "pending"}, nil
}

func episodeAnalysisManifestRecord(
	value domain.EpisodeAnalysisManifest,
	createdAt time.Time,
) (model.ShardManifest, error) {
	shards, err := json.Marshal(value.Shards)
	if err != nil {
		return model.ShardManifest{}, err
	}
	return episodeManifestRecord(
		value.ManifestID, value.Version, value.WorkspaceID, value.WorkflowRunID, value.NodeRunID,
		value.Stage, value.RootInputHash, value.ParentManifestHash, value.CoverageHash,
		value.ManifestHash, shards, createdAt,
	)
}

func episodeReconcileManifestRecord(
	value domain.EpisodeReconcileManifest,
	createdAt time.Time,
) (model.ShardManifest, error) {
	shards, err := json.Marshal(value.Shards)
	if err != nil {
		return model.ShardManifest{}, err
	}
	return episodeManifestRecord(
		value.ManifestID, value.Version, value.WorkspaceID, value.WorkflowRunID, value.NodeRunID,
		value.Stage, value.RootInputHash, value.ParentManifestHash, value.CoverageHash,
		value.ManifestHash, shards, createdAt,
	)
}

func episodeManifestRecord(
	manifestID string,
	version int64,
	workspaceID, runID, nodeID, stage, rootHash string,
	parentHash *string,
	coverageHash, manifestHash string,
	shards []byte,
	createdAt time.Time,
) (model.ShardManifest, error) {
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
		Stage: stage, RootInputHash: rootHash, ParentManifestHash: parentHash,
		Shards: datatypes.JSON(shards), CoverageHash: coverageHash,
		ManifestHash: manifestHash, CreatedAt: createdAt,
	}, nil
}

func episodeAnalysisManifestDomain(
	record model.ShardManifest,
) (domain.EpisodeAnalysisManifest, error) {
	var shards []domain.EpisodeAnalysisShard
	if err := json.Unmarshal(record.Shards, &shards); err != nil || len(shards) == 0 {
		return domain.EpisodeAnalysisManifest{}, errors.New("invalid persisted Episode analysis shards")
	}
	return domain.EpisodeAnalysisManifest{
		ManifestID: record.ID.String(), Version: record.Version,
		ParentManifestHash: record.ParentManifestHash, WorkspaceID: record.WorkspaceID.String(),
		WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(),
		Stage: record.Stage, RootInputHash: record.RootInputHash,
		MaxShardCodePoints: shards[0].MaxShardCodePoints,
		OverlapCodePoints:  shards[0].OverlapCodePoints,
		Shards:             shards, CoverageHash: record.CoverageHash, ManifestHash: record.ManifestHash,
	}, nil
}

func episodeReconcileManifestDomain(
	record model.ShardManifest,
) (domain.EpisodeReconcileManifest, error) {
	var shards []domain.EpisodeReconcileShard
	if err := json.Unmarshal(record.Shards, &shards); err != nil || len(shards) == 0 {
		return domain.EpisodeReconcileManifest{}, errors.New("invalid persisted Episode reconciliation shards")
	}
	referenced := make(map[string]struct{})
	for _, shard := range shards {
		for _, child := range shard.Children {
			if child.Stage == domain.ReconcileEpisodeStage {
				referenced[child.ShardKey] = struct{}{}
			}
		}
	}
	roots := make([]domain.EpisodeReconcileRoot, 0)
	for _, shard := range shards {
		if _, exists := referenced[shard.Key]; exists {
			continue
		}
		roots = append(roots, domain.EpisodeReconcileRoot{
			EpisodeID: shard.EpisodeID, EpisodePosition: shard.EpisodePosition,
			ShardKey: shard.Key, SubtreeHash: shard.SubtreeHash,
		})
	}
	slices.SortFunc(roots, func(left, right domain.EpisodeReconcileRoot) int {
		return left.EpisodePosition - right.EpisodePosition
	})
	return domain.EpisodeReconcileManifest{
		ManifestID: record.ID.String(), Version: record.Version,
		ParentManifestHash: record.ParentManifestHash, WorkspaceID: record.WorkspaceID.String(),
		WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(),
		Stage: record.Stage, RootInputHash: record.RootInputHash, FanIn: 2,
		Roots: roots, Shards: shards, CoverageHash: record.CoverageHash, ManifestHash: record.ManifestHash,
	}, nil
}

func loadEpisodeManifestPair(
	database *gorm.DB,
	nodeRunID string,
	lock bool,
) (domain.EpisodeAnalysisManifest, domain.EpisodeReconcileManifest, error) {
	query := database
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var analyzeRecord model.ShardManifest
	if err := query.Where("node_run_id = ? AND stage = ?", nodeRunID, domain.AnalyzeEpisodeStage).
		Order("version DESC").First(&analyzeRecord).Error; err != nil {
		return domain.EpisodeAnalysisManifest{}, domain.EpisodeReconcileManifest{}, err
	}
	query = database
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var reconcileRecord model.ShardManifest
	if err := query.Where("node_run_id = ? AND stage = ?", nodeRunID, domain.ReconcileEpisodeStage).
		Order("version DESC").First(&reconcileRecord).Error; err != nil {
		return domain.EpisodeAnalysisManifest{}, domain.EpisodeReconcileManifest{}, err
	}
	analyze, err := episodeAnalysisManifestDomain(analyzeRecord)
	if err != nil {
		return domain.EpisodeAnalysisManifest{}, domain.EpisodeReconcileManifest{}, err
	}
	reconcile, err := episodeReconcileManifestDomain(reconcileRecord)
	return analyze, reconcile, err
}

func episodeInvocationRecord(value bibledomain.Invocation) (model.AgentInvocation, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	workspace, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	run, err := uuid.Parse(value.WorkflowRunID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	node, err := uuid.Parse(value.NodeRunID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	manifest, err := uuid.Parse(value.ManifestID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	version := value.ManifestVersion
	return model.AgentInvocation{
		ID: id, WorkspaceID: workspace, WorkflowRunID: &run, NodeRunID: &node,
		ShardManifestID: &manifest, ShardManifestVersion: &version,
		RequestType: value.RequestType, RequestID: id,
		Kind: value.Kind, WireSchemaVersion: agentcontract.StoryGraphWireSchemaVersion,
		Stage: value.Stage, ShardKey: value.ShardKey, StageInstanceKey: value.StageInstanceKey,
		ShardManifestHash: value.ManifestHash, InputHash: value.InputHash,
		ExecutionPolicy: datatypes.JSON(value.ExecutionPolicy), Payload: datatypes.JSON(value.Payload),
		Status: value.Status, Attempts: value.Attempts, ClaimVersion: value.ClaimVersion,
		LeaseExpiresAt: value.LeaseExpiresAt, CreatedAt: value.CreatedAt, UpdatedAt: value.CreatedAt,
	}, nil
}

func episodeInvocationDomain(value model.AgentInvocation) bibledomain.Invocation {
	result := bibledomain.Invocation{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(),
		RequestType: value.RequestType, RequestID: value.RequestID.String(),
		Kind: value.Kind, Stage: value.Stage, ShardKey: value.ShardKey,
		InputHash: value.InputHash, StageInstanceKey: value.StageInstanceKey,
		ManifestHash: value.ShardManifestHash, Status: value.Status,
		ExecutionPolicy: append(json.RawMessage(nil), value.ExecutionPolicy...),
		Payload:         append(json.RawMessage(nil), value.Payload...),
		Attempts:        value.Attempts, ClaimVersion: value.ClaimVersion,
		LeaseExpiresAt: value.LeaseExpiresAt, CreatedAt: value.CreatedAt,
	}
	if value.WorkflowRunID != nil {
		result.WorkflowRunID = value.WorkflowRunID.String()
	}
	if value.NodeRunID != nil {
		result.NodeRunID = value.NodeRunID.String()
	}
	if value.ShardManifestID != nil {
		result.ManifestID = value.ShardManifestID.String()
	}
	if value.ShardManifestVersion != nil {
		result.ManifestVersion = *value.ShardManifestVersion
	}
	return result
}
