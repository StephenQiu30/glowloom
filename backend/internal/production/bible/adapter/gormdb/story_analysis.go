package gormdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
			if len(existing) != 2 {
				return errors.New("Story analysis manifest pair is incomplete")
			}
			for _, record := range existing {
				switch record.Stage {
				case domain.AnalyzeStoryStage:
					value, decodeErr := storyAnalysisManifestDomain(record)
					if decodeErr != nil || value.ManifestHash != preparation.AnalyzeManifest.ManifestHash {
						return errors.New("Story analysis manifest changed for the existing NodeRun")
					}
				case domain.ReconcileStoryStage:
					value, decodeErr := storyReconcileManifestDomain(record)
					if decodeErr != nil || value.ManifestHash != preparation.ReconcileManifest.ManifestHash {
						return errors.New("Story reconcile manifest changed for the existing NodeRun")
					}
				default:
					return errors.New("unexpected Story analysis manifest stage")
				}
			}
		}
		persisted, err := loadStoryReconcileManifest(transaction, preparation.Command.NodeRunID)
		if err != nil {
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
		allowed, err := storyInvocationEvidence(request)
		if err != nil {
			return err
		}
		switch invocation.Stage {
		case domain.AnalyzeStoryStage:
			_, err = domain.DecodeStoryAnalysisCandidate(result.Candidate, allowed)
		case domain.ReconcileStoryStage:
			_, err = domain.DecodeStoryReconciliationCandidate(result.Candidate, allowed)
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

func storyInvocationEvidence(request agentcontract.StageInvocation) ([]domain.Evidence, error) {
	switch request.Payload.Stage {
	case domain.AnalyzeStoryStage:
		var input agentcontract.StoryAnalysisStageInput
		if err := json.Unmarshal(request.Payload.StageInput, &input); err != nil {
			return nil, err
		}
		candidate, err := strictSourceEvidenceCandidate(input.EvidenceCandidate)
		if err != nil {
			return nil, err
		}
		return domain.SourceEvidenceCandidateEvidence(candidate), nil
	case domain.ReconcileStoryStage:
		var input agentcontract.StoryReconciliationStageInput
		if err := json.Unmarshal(request.Payload.StageInput, &input); err != nil {
			return nil, err
		}
		result := []domain.Evidence{}
		for _, child := range input.Candidates {
			switch input.CandidateType {
			case "story_analysis_candidate":
				var candidate domain.StoryAnalysisCandidate
				if err := json.Unmarshal(child.Candidate, &candidate); err != nil {
					return nil, err
				}
				validated, err := domain.DecodeStoryAnalysisCandidate(child.Candidate, domain.StoryAnalysisCandidateEvidence(candidate))
				if err != nil {
					return nil, err
				}
				result = append(result, domain.StoryAnalysisCandidateEvidence(validated)...)
			case "story_reconciliation_candidate":
				var candidate domain.StoryReconciliationCandidate
				if err := json.Unmarshal(child.Candidate, &candidate); err != nil {
					return nil, err
				}
				validated, err := domain.DecodeStoryReconciliationCandidate(child.Candidate, domain.StoryReconciliationCandidateEvidence(candidate))
				if err != nil {
					return nil, err
				}
				result = append(result, domain.StoryReconciliationCandidateEvidence(validated)...)
			default:
				return nil, errors.New("invalid Story reconcile candidate type")
			}
		}
		return result, nil
	default:
		return nil, errors.New("unsupported Story analysis stage")
	}
}

func scheduleStoryReconcile(database *gorm.DB, manifest domain.StoryReconcileManifest, now time.Time) error {
	shards := append([]domain.StoryReconcileShard(nil), manifest.Shards...)
	slices.SortFunc(shards, func(left, right domain.StoryReconcileShard) int {
		if left.Level != right.Level {
			return left.Level - right.Level
		}
		return stringsCompare(left.Key, right.Key)
	})
	for _, shard := range shards {
		var existing model.AgentInvocation
		err := database.Where("node_run_id = ? AND stage = ? AND shard_key = ?", manifest.NodeRunID, domain.ReconcileStoryStage, shard.Key).
			First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		children := make([]storyReconcileReadyChild, 0, len(shard.Children))
		ready := true
		for _, child := range shard.Children {
			var invocation model.AgentInvocation
			if err = database.Where("node_run_id = ? AND stage = ? AND shard_key = ?", manifest.NodeRunID, child.Stage, child.ShardKey).
				First(&invocation).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				ready = false
				break
			} else if err != nil {
				return err
			}
			if invocation.Status != "succeeded" || invocation.ResultHash == nil {
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
		if err = database.Omit(clause.Associations).Create(&record).Error; err != nil {
			return err
		}
	}
	return nil
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
		if shard.Children[index].Stage != child.Invocation.Stage || shard.Children[index].ShardKey != child.Invocation.ShardKey ||
			child.SourceRef != sourceRef || child.ProjectID != children[0].ProjectID || child.Invocation.ResultHash == nil {
			return model.AgentInvocation{}, errors.New("Story reconcile child ordering or source has drifted")
		}
		inputs[index] = agentcontract.StoryReconciliationInputCandidate{
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
	invocationID := uuid.NewSHA1(manifestID, []byte("story-reconcile-v1\x00"+shard.Key))
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
	err := database.Where("node_run_id = ? AND stage = ? AND shard_key = ?", manifest.NodeRunID, domain.ReconcileStoryStage, manifest.RootShardKey).
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
	var failed int64
	if err = database.Model(&model.AgentInvocation{}).
		Where("node_run_id = ? AND stage IN ? AND status = ?", manifest.NodeRunID,
			[]string{domain.AnalyzeStoryStage, domain.ReconcileStoryStage}, "failed").Count(&failed).Error; err != nil {
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
		value.NodeRunID, value.Stage, value.RootInputHash, value.CoverageHash, value.ManifestHash, shards, createdAt)
}

func storyReconcileManifestRecord(value domain.StoryReconcileManifest, createdAt time.Time) (model.ShardManifest, error) {
	shards, err := json.Marshal(value.Shards)
	if err != nil {
		return model.ShardManifest{}, err
	}
	return storyManifestRecord(value.ManifestID, value.Version, value.WorkspaceID, value.WorkflowRunID,
		value.NodeRunID, value.Stage, value.RootInputHash, value.CoverageHash, value.ManifestHash, shards, createdAt)
}

func storyManifestRecord(manifestID string, version int64, workspaceID, runID, nodeID, stage, rootHash, coverageHash, manifestHash string, shards []byte, createdAt time.Time) (model.ShardManifest, error) {
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
		Stage: stage, RootInputHash: rootHash, Shards: datatypes.JSON(shards),
		CoverageHash: coverageHash, ManifestHash: manifestHash, CreatedAt: createdAt,
	}, nil
}

func storyAnalysisManifestDomain(record model.ShardManifest) (domain.StoryAnalysisManifest, error) {
	var shards []domain.StoryAnalysisShard
	if err := json.Unmarshal(record.Shards, &shards); err != nil {
		return domain.StoryAnalysisManifest{}, err
	}
	return domain.StoryAnalysisManifest{
		ManifestID: record.ID.String(), Version: record.Version, WorkspaceID: record.WorkspaceID.String(),
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
	root := ""
	for _, shard := range shards {
		if shard.ParentKey == "" {
			root = shard.Key
		}
	}
	return domain.StoryReconcileManifest{
		ManifestID: record.ID.String(), Version: record.Version, WorkspaceID: record.WorkspaceID.String(),
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
