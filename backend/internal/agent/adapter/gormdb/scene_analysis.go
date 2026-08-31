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

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

type SceneAnalysisStore struct{ database *gorm.DB }
type sceneAnalysisRepository struct{ database *gorm.DB }

func NewSceneAnalysisStore(database *gorm.DB) *SceneAnalysisStore {
	return &SceneAnalysisStore{database: database}
}

func (store *SceneAnalysisStore) WithinSceneAnalysisTransaction(
	ctx context.Context,
	operation func(agentapp.Repository) error,
) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&sceneAnalysisRepository{database: transaction})
	})
}

func (repo *sceneAnalysisRepository) EnsureRelease(
	ctx context.Context,
	value agentapp.ReleaseRecord,
) (contract.SceneAnalysisControlProof, error) {
	record, err := sceneAnalysisReleaseRecord(value)
	if err != nil {
		return contract.SceneAnalysisControlProof{}, err
	}
	var existing model.SceneAnalysisRelease
	err = repo.database.WithContext(ctx).First(&existing, "id = ?", record.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
			return contract.SceneAnalysisControlProof{}, err
		}
		control, controlErr := sceneAnalysisControlRecord(record.ID, value.InitialControl, value.CreatedAt)
		if controlErr != nil {
			return contract.SceneAnalysisControlProof{}, controlErr
		}
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&control).Error; err != nil {
			return contract.SceneAnalysisControlProof{}, err
		}
	} else if err != nil {
		return contract.SceneAnalysisControlProof{}, err
	} else if existing.StageKey != record.StageKey || existing.ProfileKey != record.ProfileKey ||
		existing.SkillReleaseID != record.SkillReleaseID || existing.SkillReleaseHash != record.SkillReleaseHash ||
		existing.StageReleaseHash != record.StageReleaseHash || existing.BundleContentHash != record.BundleContentHash ||
		existing.AgentImageDigest != record.AgentImageDigest || existing.ModelCapability != record.ModelCapability ||
		!sameCanonicalJSON(existing.LoadedResourcePaths, record.LoadedResourcePaths) {
		return contract.SceneAnalysisControlProof{}, &agentapp.Error{
			Code: "release_identity_conflict", Message: "Scene Analysis release identity conflicts with persisted bytes",
		}
	}
	var control model.SceneAnalysisControlHead
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&control, "release_id = ?", record.ID).Error; err != nil {
		return contract.SceneAnalysisControlProof{}, err
	}
	return controlProof(control), nil
}

func (repo *sceneAnalysisRepository) EnsureManifest(ctx context.Context, value agentapp.ManifestRecord) error {
	record, err := sceneAnalysisManifestRecord(value)
	if err != nil {
		return err
	}
	var existing model.ShardManifest
	err = repo.database.WithContext(ctx).First(&existing, "id = ? AND version = 1", record.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
	}
	if err != nil {
		return err
	}
	if existing.WorkspaceID != record.WorkspaceID || existing.WorkflowRunID != record.WorkflowRunID ||
		existing.NodeRunID != record.NodeRunID || existing.Stage != record.Stage ||
		existing.RootInputHash != record.RootInputHash || !sameCanonicalJSON(existing.Shards, record.Shards) ||
		existing.CoverageHash != record.CoverageHash || existing.ManifestHash != record.ManifestHash {
		return &agentapp.Error{Code: "manifest_identity_conflict", Message: "Scene Analysis manifest conflicts with persisted bytes"}
	}
	return nil
}

func sameCanonicalJSON(left, right []byte) bool {
	leftHash, leftErr := platformcanonical.Hash(left)
	rightHash, rightErr := platformcanonical.Hash(right)
	return leftErr == nil && rightErr == nil && leftHash == rightHash
}

func (repo *sceneAnalysisRepository) FindInvocation(
	ctx context.Context,
	workflowRunID, nodeRunID, stageKey, inputHash string,
) (agentapp.InvocationRecord, error) {
	workflowID, err := uuid.Parse(workflowRunID)
	if err != nil {
		return agentapp.InvocationRecord{}, agentapp.ErrNotFound
	}
	nodeID, err := uuid.Parse(nodeRunID)
	if err != nil {
		return agentapp.InvocationRecord{}, agentapp.ErrNotFound
	}
	var record model.SceneAnalysisInvocationRecord
	if err = repo.database.WithContext(ctx).
		Where("workflow_run_id = ? AND node_run_id = ? AND stage_key = ? AND input_hash = ?", workflowID, nodeID, stageKey, inputHash).
		First(&record).Error; err != nil {
		return agentapp.InvocationRecord{}, normalizeSceneAnalysisNotFound(err)
	}
	return repo.invocationDomain(ctx, record)
}

func (repo *sceneAnalysisRepository) CreateInvocation(ctx context.Context, value agentapp.InvocationRecord) error {
	record, err := sceneAnalysisInvocationRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *sceneAnalysisRepository) FindCandidateByInvocation(ctx context.Context, invocationID string) (agentapp.Candidate, error) {
	parsedID, err := uuid.Parse(invocationID)
	if err != nil {
		return agentapp.Candidate{}, agentapp.ErrNotFound
	}
	var record model.SceneAnalysisCandidateRevision
	if err = repo.database.WithContext(ctx).First(&record, "source_invocation_id = ?", parsedID).Error; err != nil {
		return agentapp.Candidate{}, normalizeSceneAnalysisNotFound(err)
	}
	return repo.candidateDomain(ctx, record)
}

func (repo *sceneAnalysisRepository) CountAttempts(ctx context.Context, invocationID string) (int64, error) {
	parsedID, err := uuid.Parse(invocationID)
	if err != nil {
		return 0, agentapp.ErrNotFound
	}
	var count int64
	err = repo.database.WithContext(ctx).Model(&model.SceneAnalysisAttempt{}).
		Where("invocation_id = ?", parsedID).Count(&count).Error
	return count, err
}

func (repo *sceneAnalysisRepository) CreateAttempt(ctx context.Context, value agentapp.AttemptRecord) error {
	record, err := sceneAnalysisAttemptRecord(value)
	if err != nil {
		return err
	}
	var invocation model.SceneAnalysisInvocationRecord
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&invocation, "id = ?", record.InvocationID).Error; err != nil {
		return normalizeSceneAnalysisNotFound(err)
	}
	var control model.SceneAnalysisControlHead
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&control, "release_id = ?", invocation.ReleaseID).Error; err != nil {
		return err
	}
	if control.Status != "approved" || control.ControlHash != value.ControlHash ||
		control.ReleaseFence != value.ReleaseFence || invocation.ControlHash != value.ControlHash ||
		invocation.ReleaseFence != value.ReleaseFence {
		return &agentapp.Error{Code: "dispatch_fence_rejected", Message: "Scene Analysis dispatch fence was rejected"}
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Model(&model.SceneAnalysisInvocationRecord{}).
		Where("id = ? AND status IN ?", invocation.ID, []string{"queued", "outcome_unknown", "running"}).
		Updates(map[string]any{"status": "running", "updated_at": value.DispatchedAt}).Error
}

func (repo *sceneAnalysisRepository) CreateDispatchAuthorization(
	ctx context.Context,
	value agentapp.DispatchAuthorizationRecord,
) error {
	record, err := sceneAnalysisDispatchAuthorizationRecord(value)
	if err != nil {
		return err
	}
	var attempt model.SceneAnalysisAttempt
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&attempt, "id = ? AND status = ?", record.AttemptID, "dispatched").Error; err != nil {
		return normalizeSceneAnalysisNotFound(err)
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *sceneAnalysisRepository) AcceptResult(
	ctx context.Context,
	value agentapp.ResultAcceptance,
) (agentapp.Candidate, error) {
	if value.Result.Status != "accepted" || value.Result.OutputHash == nil ||
		value.Result.ValidateFor(
			value.Invocation,
			value.Result.ClaimVersion,
			value.Result.DispatchAuthorizationHash,
		) != nil {
		return agentapp.Candidate{}, errors.New("only an accepted Scene Analysis result can create a Candidate")
	}
	invocation, attempt, err := repo.lockResultFence(ctx, value)
	if err != nil {
		return agentapp.Candidate{}, err
	}
	if err = repo.validateResultReadSet(ctx, value.Invocation, invocation); err != nil {
		return agentapp.Candidate{}, err
	}
	var existing model.SceneAnalysisResult
	err = repo.database.WithContext(ctx).First(&existing, "attempt_id = ?", attempt.ID).Error
	if err == nil {
		persisted, decodeErr := contract.DecodeSceneAnalysisAttemptResult(existing.Result)
		candidate, candidateErr := repo.FindCandidateByInvocation(ctx, invocation.ID.String())
		if decodeErr != nil || persisted.ValidateFor(
			value.Invocation,
			value.Result.ClaimVersion,
			value.Result.DispatchAuthorizationHash,
		) != nil || candidateErr != nil || existing.OutputHash == nil ||
			*existing.OutputHash != *value.Result.OutputHash || persisted.ResultHash != value.Result.ResultHash {
			return agentapp.Candidate{}, &agentapp.Error{Code: "result_conflict", Message: "Scene Analysis result conflicts with persisted bytes"}
		}
		return candidate, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return agentapp.Candidate{}, err
	}
	resultRecord, err := sceneAnalysisResultRecord(value)
	if err != nil {
		return agentapp.Candidate{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&resultRecord).Error; err != nil {
		return agentapp.Candidate{}, err
	}
	candidateRecord, err := sceneAnalysisCandidateRecord(value, invocation, resultRecord)
	if err != nil {
		return agentapp.Candidate{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&candidateRecord).Error; err != nil {
		return agentapp.Candidate{}, err
	}
	head := model.SceneAnalysisCandidateHead{
		StageInstanceKey: candidateRecord.StageInstanceKey, WorkspaceID: candidateRecord.WorkspaceID,
		ProjectID: candidateRecord.ProjectID, CurrentRevisionID: candidateRecord.ID,
		CurrentCandidateRevisionHash: candidateRecord.CandidateRevisionHash, Revision: 1, UpdatedAt: value.AcceptedAt,
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&head).Error; err != nil {
		return agentapp.Candidate{}, err
	}
	if err = repo.finishAttemptAndInvocation(ctx, attempt.ID, invocation.ID, "accepted", value.AcceptedAt); err != nil {
		return agentapp.Candidate{}, err
	}
	return repo.candidateDomain(ctx, candidateRecord)
}

func (repo *sceneAnalysisRepository) validateResultReadSet(
	ctx context.Context,
	request contract.SceneAnalysisInvocation,
	invocation model.SceneAnalysisInvocationRecord,
) error {
	if len(request.Payload.SourceRefs) != 1 {
		return staleSceneAnalysisReadSet()
	}
	source := request.Payload.SourceRefs[0]
	sourceVersionID, versionErr := uuid.Parse(source.VersionID)
	documentID, documentErr := uuid.Parse(source.LogicalID)
	if versionErr != nil || documentErr != nil || invocation.SourceVersionID != sourceVersionID ||
		invocation.SourceHash != source.ContentHash {
		return staleSceneAnalysisReadSet()
	}
	var revision model.DocumentRevision
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&revision, "id = ?", sourceVersionID).Error; err != nil {
		return normalizeSceneAnalysisReadSet(err)
	}
	var document model.ScriptDocument
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&document, "id = ?", documentID).Error; err != nil {
		return normalizeSceneAnalysisReadSet(err)
	}
	if source.OwnerKind != "production/script" || revision.WorkspaceID != invocation.WorkspaceID ||
		revision.DocumentID != documentID || int64(revision.VersionNo) != source.Revision ||
		revision.NormalizedHash != source.ContentHash || !revision.CreatedAt.Equal(source.CreatedAt) ||
		document.WorkspaceID != invocation.WorkspaceID || document.ProjectID != invocation.ProjectID {
		return staleSceneAnalysisReadSet()
	}
	return repo.validateUpstreamReadSet(ctx, request.Payload.UpstreamCandidates, invocation)
}

func (repo *sceneAnalysisRepository) validateUpstreamReadSet(
	ctx context.Context,
	upstreams []contract.ScriptSpanRevisionIdentity,
	invocation model.SceneAnalysisInvocationRecord,
) error {
	if len(upstreams) == 0 {
		if invocation.UpstreamCandidateRevisionID != nil || invocation.UpstreamCandidateRevisionHash != nil {
			return staleSceneAnalysisReadSet()
		}
		return nil
	}
	if len(upstreams) != 1 || invocation.UpstreamCandidateRevisionID == nil ||
		invocation.UpstreamCandidateRevisionHash == nil {
		return staleSceneAnalysisReadSet()
	}
	upstream := upstreams[0]
	candidateID, candidateErr := uuid.Parse(upstream.CandidateRevisionID)
	sourceInvocationID, invocationErr := uuid.Parse(upstream.SourceInvocationID)
	if candidateErr != nil || invocationErr != nil || *invocation.UpstreamCandidateRevisionID != candidateID ||
		*invocation.UpstreamCandidateRevisionHash != upstream.CandidateRevisionHash {
		return staleSceneAnalysisReadSet()
	}
	var candidate model.SceneAnalysisCandidateRevision
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&candidate, "id = ?", candidateID).Error; err != nil {
		return normalizeSceneAnalysisReadSet(err)
	}
	var sourceInvocation model.SceneAnalysisInvocationRecord
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&sourceInvocation, "id = ?", sourceInvocationID).Error; err != nil {
		return normalizeSceneAnalysisReadSet(err)
	}
	var sourceResult model.SceneAnalysisResult
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&sourceResult, "id = ?", candidate.SourceResultID).Error; err != nil {
		return normalizeSceneAnalysisReadSet(err)
	}
	var sourceAttempt model.SceneAnalysisAttempt
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).
		First(&sourceAttempt, "id = ?", sourceResult.AttemptID).Error; err != nil {
		return normalizeSceneAnalysisReadSet(err)
	}
	result, resultErr := contract.DecodeSceneAnalysisAttemptResult(sourceResult.Result)
	computedResultHash, resultHashErr := result.ComputeResultHash()
	contentHash, contentErr := contract.ProductionCanonicalHash(json.RawMessage(candidate.Candidate))
	revisionHash, revisionErr := sceneAnalysisCandidateRevisionHash(candidate)
	if upstream.StageKey != sourceInvocation.StageKey || upstream.ShardKey != sourceInvocation.ShardKey ||
		candidate.WorkspaceID != invocation.WorkspaceID || candidate.ProjectID != invocation.ProjectID ||
		candidate.CandidateType != "script_span_candidate" || candidate.SourceInvocationID != sourceInvocationID ||
		candidate.CandidateRevisionHash != upstream.CandidateRevisionHash ||
		candidate.SourceResultHash != upstream.SourceResultHash || sourceAttempt.InvocationID != sourceInvocationID ||
		sourceInvocation.Status != "accepted" || sourceAttempt.Status != "completed" || sourceResult.Status != "accepted" ||
		sourceResult.InputHash != sourceInvocation.InputHash || resultErr != nil ||
		result.InvocationID != sourceInvocationID.String() || result.AttemptID != sourceAttempt.ID.String() ||
		result.InputHash != sourceInvocation.InputHash || result.Status != "accepted" ||
		result.CandidateType != "script_span_candidate" || result.OutputHash == nil || sourceResult.OutputHash == nil ||
		*result.OutputHash != *sourceResult.OutputHash || resultHashErr != nil ||
		computedResultHash != result.ResultHash || result.ResultHash != upstream.SourceResultHash ||
		*sourceResult.OutputHash != candidate.CandidateContentHash || contentErr != nil ||
		contentHash != candidate.CandidateContentHash || revisionErr != nil ||
		revisionHash != candidate.CandidateRevisionHash {
		return staleSceneAnalysisReadSet()
	}
	return nil
}

func staleSceneAnalysisReadSet() error {
	return &agentapp.Error{Code: "stale_read_set", Message: "Scene Analysis read set changed before Candidate apply"}
}

func normalizeSceneAnalysisReadSet(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return staleSceneAnalysisReadSet()
	}
	return err
}

func (repo *sceneAnalysisRepository) RecordFailedResult(ctx context.Context, value agentapp.ResultAcceptance) error {
	if value.Result.Status == "accepted" || value.Result.ValidateFor(
		value.Invocation,
		value.Result.ClaimVersion,
		value.Result.DispatchAuthorizationHash,
	) != nil {
		return errors.New("invalid failed Scene Analysis result")
	}
	invocation, attempt, err := repo.lockResultFence(ctx, value)
	if err != nil {
		return err
	}
	var existing model.SceneAnalysisResult
	err = repo.database.WithContext(ctx).First(&existing, "attempt_id = ?", attempt.ID).Error
	if err == nil {
		persisted, decodeErr := contract.DecodeSceneAnalysisAttemptResult(existing.Result)
		if decodeErr != nil || persisted.ValidateFor(
			value.Invocation,
			value.Result.ClaimVersion,
			value.Result.DispatchAuthorizationHash,
		) != nil || existing.Status != value.Result.Status ||
			existing.DiagnosticHash != value.Result.DiagnosticHash || persisted.ResultHash != value.Result.ResultHash {
			return &agentapp.Error{Code: "result_conflict", Message: "Scene Analysis result conflicts with persisted bytes"}
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	record, err := sceneAnalysisResultRecord(value)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return err
	}
	return repo.finishAttemptAndInvocation(ctx, attempt.ID, invocation.ID, value.Result.Status, value.AcceptedAt)
}

func (repo *sceneAnalysisRepository) GetCandidate(ctx context.Context, projectID, candidateID string) (agentapp.Candidate, error) {
	parsedProjectID, err := uuid.Parse(projectID)
	if err != nil {
		return agentapp.Candidate{}, agentapp.ErrNotFound
	}
	parsedCandidateID, err := uuid.Parse(candidateID)
	if err != nil {
		return agentapp.Candidate{}, agentapp.ErrNotFound
	}
	var record model.SceneAnalysisCandidateRevision
	if err = repo.database.WithContext(ctx).
		Where("id = ? AND project_id = ?", parsedCandidateID, parsedProjectID).First(&record).Error; err != nil {
		return agentapp.Candidate{}, normalizeSceneAnalysisNotFound(err)
	}
	return repo.candidateDomain(ctx, record)
}

func (repo *sceneAnalysisRepository) lockResultFence(
	ctx context.Context,
	value agentapp.ResultAcceptance,
) (model.SceneAnalysisInvocationRecord, model.SceneAnalysisAttempt, error) {
	invocationID, err := uuid.Parse(value.Invocation.InvocationID)
	if err != nil {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, err
	}
	attemptID, err := uuid.Parse(value.Invocation.AttemptID)
	if err != nil {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, err
	}
	var release model.SceneAnalysisRelease
	if err = repo.database.WithContext(ctx).First(&release, "stage_release_hash = ?", value.Invocation.StageRelease.StageReleaseHash).Error; err != nil {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, err
	}
	var control model.SceneAnalysisControlHead
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&control, "release_id = ?", release.ID).Error; err != nil {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, err
	}
	if control.Status != "approved" || control.ControlHash != value.Invocation.Control.ControlHash ||
		control.ReleaseFence != value.Invocation.Control.ReleaseFence {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, &agentapp.Error{
			Code: "result_fence_rejected", Message: "Scene Analysis result fence was rejected",
		}
	}
	var invocation model.SceneAnalysisInvocationRecord
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&invocation, "id = ?", invocationID).Error; err != nil {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, err
	}
	if invocation.ReleaseID != release.ID || invocation.InputHash != value.Invocation.InputHash ||
		invocation.ControlHash != control.ControlHash || invocation.ReleaseFence != control.ReleaseFence {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, &agentapp.Error{
			Code: "result_fence_rejected", Message: "Scene Analysis result fence was rejected",
		}
	}
	var attempt model.SceneAnalysisAttempt
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&attempt, "id = ? AND invocation_id = ?", attemptID, invocationID).Error; err != nil {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, err
	}
	if attempt.ControlHash != control.ControlHash || attempt.ReleaseFence != control.ReleaseFence ||
		attempt.AgentImageDigest != release.AgentImageDigest ||
		attempt.ClaimVersion != value.Result.ClaimVersion {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, &agentapp.Error{
			Code: "result_fence_rejected", Message: "Scene Analysis result fence was rejected",
		}
	}
	var authorization model.SceneAnalysisDispatchAuthorization
	if err = repo.database.WithContext(ctx).First(&authorization, "attempt_id = ?", attempt.ID).Error; err != nil {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, err
	}
	if authorization.AuthorizationHash != value.Result.DispatchAuthorizationHash {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, &agentapp.Error{
			Code: "result_fence_rejected", Message: "Scene Analysis result fence was rejected",
		}
	}
	return invocation, attempt, nil
}

func (repo *sceneAnalysisRepository) finishAttemptAndInvocation(
	ctx context.Context,
	attemptID, invocationID uuid.UUID,
	status string,
	completedAt time.Time,
) error {
	updatedAttempt := repo.database.WithContext(ctx).Model(&model.SceneAnalysisAttempt{}).
		Where("id = ? AND invocation_id = ? AND status = ?", attemptID, invocationID, "dispatched").
		Updates(map[string]any{"status": "completed", "completed_at": completedAt})
	if updatedAttempt.Error != nil || updatedAttempt.RowsAffected != 1 {
		if updatedAttempt.Error != nil {
			return updatedAttempt.Error
		}
		return &agentapp.Error{Code: "attempt_conflict", Message: "Scene Analysis attempt was already completed"}
	}
	updatedInvocation := repo.database.WithContext(ctx).Model(&model.SceneAnalysisInvocationRecord{}).
		Where("id = ? AND status = ?", invocationID, "running").
		Updates(map[string]any{"status": status, "updated_at": completedAt})
	if updatedInvocation.Error != nil || updatedInvocation.RowsAffected != 1 {
		if updatedInvocation.Error != nil {
			return updatedInvocation.Error
		}
		return &agentapp.Error{Code: "invocation_conflict", Message: "Scene Analysis invocation state changed"}
	}
	return nil
}

func sceneAnalysisReleaseRecord(value agentapp.ReleaseRecord) (model.SceneAnalysisRelease, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.SceneAnalysisRelease{}, err
	}
	skillReleaseID, err := uuid.Parse(value.Identity.SkillReleaseID)
	if err != nil {
		return model.SceneAnalysisRelease{}, err
	}
	resources, err := json.Marshal(value.LoadedResources)
	if err != nil {
		return model.SceneAnalysisRelease{}, err
	}
	return model.SceneAnalysisRelease{
		ID: id, StageKey: value.Variant.StageKey, ProfileKey: value.Variant.ProfileKey,
		SkillReleaseID: skillReleaseID, SkillReleaseHash: value.Identity.SkillReleaseHash,
		StageReleaseHash: value.Identity.StageReleaseHash, BundleContentHash: value.Identity.BundleContentHash,
		AgentImageDigest: value.Identity.AgentImageDigest, ModelCapability: "structured_text",
		LoadedResourcePaths: datatypes.JSON(resources), CreatedAt: value.CreatedAt,
	}, nil
}

func sceneAnalysisControlRecord(
	releaseID uuid.UUID,
	value contract.SceneAnalysisControlProof,
	now time.Time,
) (model.SceneAnalysisControlHead, error) {
	recordID, err := uuid.Parse(value.ControlRecordID)
	if err != nil {
		return model.SceneAnalysisControlHead{}, err
	}
	return model.SceneAnalysisControlHead{
		ReleaseID: releaseID, ControlRecordID: recordID, ControlRevision: value.ControlRevision,
		Status: value.Status, ControlHash: value.ControlHash, ReleaseFence: value.ReleaseFence, UpdatedAt: now,
	}, nil
}

func controlProof(value model.SceneAnalysisControlHead) contract.SceneAnalysisControlProof {
	return contract.SceneAnalysisControlProof{
		ControlRecordID: value.ControlRecordID.String(), ControlRevision: value.ControlRevision,
		Status: value.Status, ControlHash: value.ControlHash, ReleaseFence: value.ReleaseFence,
	}
}

func sceneAnalysisManifestRecord(value agentapp.ManifestRecord) (model.ShardManifest, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	workflowRunID, err := uuid.Parse(value.WorkflowRunID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	nodeRunID, err := uuid.Parse(value.NodeRunID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	return model.ShardManifest{
		ID: id, Version: 1, WorkspaceID: workspaceID, WorkflowRunID: workflowRunID, NodeRunID: nodeRunID,
		Stage: value.StageKey, RootInputHash: value.RootInputHash, Shards: datatypes.JSON(value.Shards),
		CoverageHash: value.CoverageHash, ManifestHash: value.ManifestHash, CreatedAt: value.CreatedAt,
	}, nil
}

func sceneAnalysisInvocationRecord(value agentapp.InvocationRecord) (model.SceneAnalysisInvocationRecord, error) {
	invocation := value.Invocation
	if err := invocation.Validate(); err != nil {
		return model.SceneAnalysisInvocationRecord{}, err
	}
	identifiers := []string{
		invocation.InvocationID, value.WorkspaceID, value.ProjectID, value.WorkflowRunID, value.NodeRunID,
		value.ReleaseID, invocation.Control.ControlRecordID, value.SourceVersionID, value.Manifest.ID,
	}
	parsed := make([]uuid.UUID, len(identifiers))
	for index, identifier := range identifiers {
		value, err := uuid.Parse(identifier)
		if err != nil {
			return model.SceneAnalysisInvocationRecord{}, err
		}
		parsed[index] = value
	}
	payload, err := json.Marshal(invocation.Payload)
	if err != nil {
		return model.SceneAnalysisInvocationRecord{}, err
	}
	budget, err := json.Marshal(invocation.Budget)
	if err != nil {
		return model.SceneAnalysisInvocationRecord{}, err
	}
	upstreamID, err := optionalSceneAnalysisUUID(value.UpstreamCandidateRevisionID)
	if err != nil {
		return model.SceneAnalysisInvocationRecord{}, err
	}
	return model.SceneAnalysisInvocationRecord{
		ID: parsed[0], WorkspaceID: parsed[1], ProjectID: parsed[2], WorkflowRunID: parsed[3], NodeRunID: parsed[4],
		ReleaseID: parsed[5], ControlRecordID: parsed[6], ControlRevision: invocation.Control.ControlRevision,
		ControlHash: invocation.Control.ControlHash, ReleaseFence: invocation.Control.ReleaseFence,
		WireSchemaID: invocation.WireSchemaVersion, StageKey: invocation.Payload.Variant.StageKey,
		ProfileKey: invocation.Payload.Variant.ProfileKey, StageInstanceKey: invocation.StageInstanceKey(),
		InputHash: invocation.InputHash, SourceVersionID: parsed[7], SourceHash: value.SourceHash,
		UpstreamCandidateRevisionID: upstreamID, UpstreamCandidateRevisionHash: value.UpstreamCandidateRevisionHash,
		ShardManifestID: parsed[8], ShardManifestHash: value.Manifest.ManifestHash,
		ShardKey: invocation.Payload.Shard.ShardKey, Payload: datatypes.JSON(payload), Budget: datatypes.JSON(budget),
		Status: "queued", CreatedAt: value.CreatedAt, UpdatedAt: value.CreatedAt,
	}, nil
}

func sceneAnalysisAttemptRecord(value agentapp.AttemptRecord) (model.SceneAnalysisAttempt, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.SceneAnalysisAttempt{}, err
	}
	invocationID, err := uuid.Parse(value.InvocationID)
	if err != nil {
		return model.SceneAnalysisAttempt{}, err
	}
	return model.SceneAnalysisAttempt{
		ID: id, InvocationID: invocationID, ClaimVersion: value.ClaimVersion,
		ControlHash: value.ControlHash, ReleaseFence: value.ReleaseFence,
		AgentImageDigest: value.AgentImageDigest, Status: "dispatched", DispatchedAt: value.DispatchedAt,
	}, nil
}

func sceneAnalysisDispatchAuthorizationRecord(
	value agentapp.DispatchAuthorizationRecord,
) (model.SceneAnalysisDispatchAuthorization, error) {
	attemptID, err := uuid.Parse(value.AttemptID)
	if err != nil || len(value.AuthorizationHash) != 64 || !value.ExpiresAt.After(value.IssuedAt) {
		return model.SceneAnalysisDispatchAuthorization{}, errors.New("invalid Scene Analysis dispatch authorization record")
	}
	return model.SceneAnalysisDispatchAuthorization{
		AttemptID: attemptID, AuthorizationHash: value.AuthorizationHash,
		ExpiresAt: value.ExpiresAt, IssuedAt: value.IssuedAt,
	}, nil
}

func sceneAnalysisResultRecord(value agentapp.ResultAcceptance) (model.SceneAnalysisResult, error) {
	id, err := uuid.Parse(value.ResultID)
	if err != nil {
		return model.SceneAnalysisResult{}, err
	}
	attemptID, err := uuid.Parse(value.Invocation.AttemptID)
	if err != nil {
		return model.SceneAnalysisResult{}, err
	}
	encoded, err := json.Marshal(value.Result)
	if err != nil {
		return model.SceneAnalysisResult{}, err
	}
	return model.SceneAnalysisResult{
		ID: id, AttemptID: attemptID, Status: value.Result.Status, InputHash: value.Result.InputHash,
		OutputHash: value.Result.OutputHash, DiagnosticHash: value.Result.DiagnosticHash,
		Result: datatypes.JSON(encoded), CompletedAt: value.Result.CompletedAt,
	}, nil
}

func sceneAnalysisCandidateRecord(
	value agentapp.ResultAcceptance,
	invocation model.SceneAnalysisInvocationRecord,
	result model.SceneAnalysisResult,
) (model.SceneAnalysisCandidateRevision, error) {
	id, err := uuid.Parse(value.CandidateID)
	if err != nil {
		return model.SceneAnalysisCandidateRevision{}, err
	}
	record := model.SceneAnalysisCandidateRevision{
		ID: id, WorkspaceID: invocation.WorkspaceID, ProjectID: invocation.ProjectID,
		StageInstanceKey: invocation.StageInstanceKey, RevisionNo: 1, CandidateType: value.Result.CandidateType,
		SourceInvocationID: invocation.ID, SourceResultID: result.ID, SourceResultHash: value.Result.ResultHash,
		Candidate:            datatypes.JSON(append([]byte(nil), value.Result.Candidate...)),
		CandidateContentHash: *value.Result.OutputHash,
		CreatedAt:            value.AcceptedAt,
	}
	revisionHash, err := sceneAnalysisCandidateRevisionHash(record)
	if err != nil {
		return model.SceneAnalysisCandidateRevision{}, err
	}
	record.CandidateRevisionHash = revisionHash
	return record, nil
}

func sceneAnalysisCandidateRevisionHash(value model.SceneAnalysisCandidateRevision) (string, error) {
	material, err := json.Marshal(map[string]any{
		"contract_id": "scene-analysis-candidate-revision", "stage_instance_key": value.StageInstanceKey,
		"revision": value.RevisionNo, "candidate_type": value.CandidateType,
		"source_invocation_id": value.SourceInvocationID.String(), "source_result_id": value.SourceResultID.String(),
		"source_result_hash": value.SourceResultHash, "candidate_content_hash": value.CandidateContentHash,
	})
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(material)
}

func (repo *sceneAnalysisRepository) invocationDomain(
	ctx context.Context,
	record model.SceneAnalysisInvocationRecord,
) (agentapp.InvocationRecord, error) {
	var release model.SceneAnalysisRelease
	if err := repo.database.WithContext(ctx).First(&release, "id = ?", record.ReleaseID).Error; err != nil {
		return agentapp.InvocationRecord{}, err
	}
	var attempt model.SceneAnalysisAttempt
	if err := repo.database.WithContext(ctx).Where("invocation_id = ?", record.ID).
		Order("claim_version DESC").First(&attempt).Error; err != nil {
		return agentapp.InvocationRecord{}, normalizeSceneAnalysisNotFound(err)
	}
	var payload contract.SceneAnalysisPayload
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return agentapp.InvocationRecord{}, err
	}
	var budget contract.SceneAnalysisExecutionBudget
	if err := json.Unmarshal(record.Budget, &budget); err != nil {
		return agentapp.InvocationRecord{}, err
	}
	invocation, err := contract.NewSceneAnalysisInvocation(
		record.ID.String(), attempt.ID.String(),
		contract.SceneAnalysisReleaseIdentity{
			SkillReleaseID: release.SkillReleaseID.String(), SkillReleaseHash: release.SkillReleaseHash,
			StageReleaseHash: release.StageReleaseHash, BundleContentHash: release.BundleContentHash,
			AgentImageDigest: release.AgentImageDigest,
		},
		contract.SceneAnalysisControlProof{
			ControlRecordID: record.ControlRecordID.String(), ControlRevision: record.ControlRevision,
			Status: "approved", ControlHash: record.ControlHash, ReleaseFence: record.ReleaseFence,
		},
		budget, payload,
	)
	if err != nil || invocation.InputHash != record.InputHash || invocation.StageInstanceKey() != record.StageInstanceKey {
		return agentapp.InvocationRecord{}, errors.New("persisted Scene Analysis invocation hash drifted")
	}
	manifest := agentapp.ManifestRecord{
		ID: record.ShardManifestID.String(), WorkspaceID: record.WorkspaceID.String(),
		WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(),
		StageKey: record.StageKey, ManifestHash: record.ShardManifestHash,
	}
	return agentapp.InvocationRecord{
		Invocation: invocation, WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(), ReleaseID: record.ReleaseID.String(),
		SourceVersionID: record.SourceVersionID.String(), SourceHash: record.SourceHash,
		UpstreamCandidateRevisionID:   optionalSceneAnalysisString(record.UpstreamCandidateRevisionID),
		UpstreamCandidateRevisionHash: record.UpstreamCandidateRevisionHash,
		Manifest:                      manifest, CreatedAt: record.CreatedAt,
	}, nil
}

func (repo *sceneAnalysisRepository) candidateDomain(
	ctx context.Context,
	record model.SceneAnalysisCandidateRevision,
) (agentapp.Candidate, error) {
	var invocation model.SceneAnalysisInvocationRecord
	if err := repo.database.WithContext(ctx).First(&invocation, "id = ?", record.SourceInvocationID).Error; err != nil {
		return agentapp.Candidate{}, err
	}
	return agentapp.Candidate{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		StageKey: invocation.StageKey, ProfileKey: invocation.ProfileKey, StageInstanceKey: record.StageInstanceKey,
		Revision: record.RevisionNo, CandidateType: record.CandidateType,
		Candidate: append([]byte(nil), record.Candidate...), CandidateContentHash: record.CandidateContentHash,
		CandidateRevisionHash: record.CandidateRevisionHash, SourceInvocationID: record.SourceInvocationID.String(),
		SourceResultID: record.SourceResultID.String(), SourceResultHash: record.SourceResultHash,
		CreatedAt: record.CreatedAt,
	}, nil
}

func optionalSceneAnalysisUUID(value *string) (*uuid.UUID, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := uuid.Parse(*value)
	return &parsed, err
}

func optionalSceneAnalysisString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	result := value.String()
	return &result
}

func normalizeSceneAnalysisNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return agentapp.ErrNotFound
	}
	return err
}
