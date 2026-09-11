package gormdb

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

type VisualFoundationInputValidator func(context.Context, *gorm.DB, contract.VisualFoundationInput) error

type VisualFoundationStore struct {
	database  *gorm.DB
	validator VisualFoundationInputValidator
}

func NewVisualFoundationStore(
	database *gorm.DB,
	validator VisualFoundationInputValidator,
) (*VisualFoundationStore, error) {
	if database == nil || validator == nil {
		return nil, errors.New("Visual Foundation GORM dependencies are required")
	}
	return &VisualFoundationStore{database: database, validator: validator}, nil
}

func (store *VisualFoundationStore) WithinVisualFoundationTransaction(
	ctx context.Context,
	operation func(agentapp.VisualFoundationRepository) error,
) error {
	return platformdatabase.WithinSerializableTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&sceneAnalysisRepository{
			database: transaction, visualFoundationValidator: store.validator,
		})
	})
}

func (repo *sceneAnalysisRepository) ValidateVisualFoundationInput(
	ctx context.Context,
	input contract.VisualFoundationInput,
) error {
	if input.Validate() != nil {
		return &agentapp.Error{Code: "invalid_visual_foundation_input", Message: "Visual Foundation input is invalid"}
	}
	if repo.visualFoundationValidator == nil {
		return errors.New("Visual Foundation input validator is unavailable")
	}
	return repo.visualFoundationValidator(ctx, repo.database, input)
}

func (repo *sceneAnalysisRepository) EnsureVisualFoundationRelease(
	ctx context.Context,
	value agentapp.ReleaseRecord,
) (contract.SceneAnalysisControlProof, error) {
	if value.Variant.StageKey != contract.VisualFoundationStageKey || value.ModelCapability != "vision" {
		return contract.SceneAnalysisControlProof{}, errors.New("invalid Visual Foundation release")
	}
	return repo.EnsureRelease(ctx, value)
}

func (repo *sceneAnalysisRepository) EnsureVisualFoundationManifest(
	ctx context.Context,
	value agentapp.ManifestRecord,
) error {
	if value.StageKey != contract.VisualFoundationStageKey {
		return errors.New("invalid Visual Foundation manifest")
	}
	return repo.EnsureManifest(ctx, value)
}

func (repo *sceneAnalysisRepository) FindVisualFoundationInvocation(
	ctx context.Context,
	workflowRunID string,
	nodeRunID string,
	inputHash string,
) (agentapp.VisualFoundationInvocationRecord, error) {
	workflowID, workflowErr := uuid.Parse(workflowRunID)
	nodeID, nodeErr := uuid.Parse(nodeRunID)
	if workflowErr != nil || nodeErr != nil {
		return agentapp.VisualFoundationInvocationRecord{}, agentapp.ErrNotFound
	}
	var record model.SceneAnalysisInvocationRecord
	if err := repo.database.WithContext(ctx).Where(
		"workflow_run_id = ? AND node_run_id = ? AND stage_key = ? AND input_hash = ?",
		workflowID,
		nodeID,
		contract.VisualFoundationStageKey,
		inputHash,
	).First(&record).Error; err != nil {
		return agentapp.VisualFoundationInvocationRecord{}, normalizeSceneAnalysisNotFound(err)
	}
	return repo.visualFoundationInvocationDomain(ctx, record)
}

func (repo *sceneAnalysisRepository) CreateVisualFoundationInvocation(
	ctx context.Context,
	value agentapp.VisualFoundationInvocationRecord,
) error {
	record, err := visualFoundationInvocationRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *sceneAnalysisRepository) FindVisualFoundationCandidateByInvocation(
	ctx context.Context,
	invocationID string,
) (agentapp.Candidate, error) {
	return repo.FindCandidateByInvocation(ctx, invocationID)
}

func (repo *sceneAnalysisRepository) CountVisualFoundationAttempts(
	ctx context.Context,
	invocationID string,
) (int64, error) {
	return repo.CountAttempts(ctx, invocationID)
}

func (repo *sceneAnalysisRepository) CreateVisualFoundationAttempt(
	ctx context.Context,
	value agentapp.AttemptRecord,
) error {
	return repo.CreateAttempt(ctx, value)
}

func (repo *sceneAnalysisRepository) CreateVisualFoundationDispatchAuthorization(
	ctx context.Context,
	value agentapp.DispatchAuthorizationRecord,
) error {
	return repo.CreateDispatchAuthorization(ctx, value)
}

func (repo *sceneAnalysisRepository) AcceptVisualFoundationResult(
	ctx context.Context,
	value agentapp.VisualFoundationResultAcceptance,
) (agentapp.Candidate, error) {
	invocationContract := value.Invocation.Invocation
	if value.Result.Status != "accepted" || value.Result.OutputHash == nil ||
		value.Result.ValidateFor(
			invocationContract,
			value.Result.ClaimVersion,
			value.Result.DispatchAuthorizationHash,
		) != nil {
		return agentapp.Candidate{}, errors.New("only an accepted Visual Foundation result can create a Candidate")
	}
	invocation, attempt, err := repo.lockVisualFoundationResultFence(ctx, value)
	if err != nil {
		return agentapp.Candidate{}, err
	}
	var existing model.SceneAnalysisResult
	err = repo.database.WithContext(ctx).First(&existing, "attempt_id = ?", attempt.ID).Error
	if err == nil {
		persisted, decodeErr := contract.DecodeVisualFoundationAttemptResult(existing.Result)
		candidate, candidateErr := repo.FindCandidateByInvocation(ctx, invocation.ID.String())
		if decodeErr != nil || persisted.ValidateFor(
			invocationContract,
			value.Result.ClaimVersion,
			value.Result.DispatchAuthorizationHash,
		) != nil || candidateErr != nil || existing.OutputHash == nil ||
			*existing.OutputHash != *value.Result.OutputHash || persisted.ResultHash != value.Result.ResultHash {
			return agentapp.Candidate{}, &agentapp.Error{
				Code: "result_conflict", Message: "Visual Foundation result conflicts with persisted bytes",
			}
		}
		return candidate, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return agentapp.Candidate{}, err
	}
	resultRecord, err := visualFoundationResultRecord(value)
	if err != nil {
		return agentapp.Candidate{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&resultRecord).Error; err != nil {
		return agentapp.Candidate{}, err
	}
	candidateRecord, err := visualFoundationCandidateRecord(value, invocation, resultRecord)
	if err != nil {
		return agentapp.Candidate{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&candidateRecord).Error; err != nil {
		return agentapp.Candidate{}, err
	}
	head := model.SceneAnalysisCandidateHead{
		StageInstanceKey: candidateRecord.StageInstanceKey, WorkspaceID: candidateRecord.WorkspaceID,
		ProjectID: candidateRecord.ProjectID, CurrentRevisionID: candidateRecord.ID,
		CurrentCandidateRevisionHash: candidateRecord.CandidateRevisionHash,
		Revision:                     1, UpdatedAt: value.AcceptedAt,
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&head).Error; err != nil {
		return agentapp.Candidate{}, err
	}
	if err = repo.finishAttemptAndInvocation(ctx, attempt.ID, invocation.ID, "accepted", value.AcceptedAt); err != nil {
		return agentapp.Candidate{}, err
	}
	return repo.candidateDomain(ctx, candidateRecord)
}

func (repo *sceneAnalysisRepository) RecordFailedVisualFoundationResult(
	ctx context.Context,
	value agentapp.VisualFoundationResultAcceptance,
) error {
	invocationContract := value.Invocation.Invocation
	if value.Result.Status == "accepted" || value.Result.ValidateFor(
		invocationContract,
		value.Result.ClaimVersion,
		value.Result.DispatchAuthorizationHash,
	) != nil {
		return errors.New("invalid failed Visual Foundation result")
	}
	invocation, attempt, err := repo.lockVisualFoundationResultFence(ctx, value)
	if err != nil {
		return err
	}
	var existing model.SceneAnalysisResult
	err = repo.database.WithContext(ctx).First(&existing, "attempt_id = ?", attempt.ID).Error
	if err == nil {
		persisted, decodeErr := contract.DecodeVisualFoundationAttemptResult(existing.Result)
		if decodeErr != nil || persisted.ValidateFor(
			invocationContract,
			value.Result.ClaimVersion,
			value.Result.DispatchAuthorizationHash,
		) != nil || existing.Status != value.Result.Status ||
			existing.DiagnosticHash != value.Result.DiagnosticHash || persisted.ResultHash != value.Result.ResultHash {
			return &agentapp.Error{Code: "result_conflict", Message: "Visual Foundation result conflicts with persisted bytes"}
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	record, err := visualFoundationResultRecord(value)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return err
	}
	return repo.finishAttemptAndInvocation(ctx, attempt.ID, invocation.ID, value.Result.Status, value.AcceptedAt)
}

func (repo *sceneAnalysisRepository) lockVisualFoundationResultFence(
	ctx context.Context,
	value agentapp.VisualFoundationResultAcceptance,
) (model.SceneAnalysisInvocationRecord, model.SceneAnalysisAttempt, error) {
	invocationID, invocationErr := uuid.Parse(value.Invocation.Invocation.InvocationID)
	attemptID, attemptErr := uuid.Parse(value.Invocation.Invocation.AttemptID)
	if invocationErr != nil || attemptErr != nil {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, errors.New("invalid Visual Foundation result identity")
	}
	var release model.SceneAnalysisRelease
	if err := repo.database.WithContext(ctx).First(
		&release,
		"stage_release_hash = ?",
		value.Invocation.Invocation.StageRelease.StageReleaseHash,
	).Error; err != nil {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, err
	}
	var control model.SceneAnalysisControlHead
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&control, "release_id = ?", release.ID).Error; err != nil {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, err
	}
	contractInvocation := value.Invocation.Invocation
	if control.Status != "approved" || control.ControlHash != contractInvocation.Control.ControlHash ||
		control.ReleaseFence != contractInvocation.Control.ReleaseFence {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, visualFoundationFenceRejected()
	}
	var invocation model.SceneAnalysisInvocationRecord
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&invocation, "id = ?", invocationID).Error; err != nil {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, err
	}
	if invocation.StageKey != contract.VisualFoundationStageKey || invocation.ReleaseID != release.ID ||
		invocation.InputHash != contractInvocation.InputHash || invocation.ControlHash != control.ControlHash ||
		invocation.ReleaseFence != control.ReleaseFence {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, visualFoundationFenceRejected()
	}
	var attempt model.SceneAnalysisAttempt
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&attempt, "id = ? AND invocation_id = ?", attemptID, invocationID).Error; err != nil {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, err
	}
	if attempt.Status != "dispatched" || attempt.ControlHash != control.ControlHash ||
		attempt.ReleaseFence != control.ReleaseFence || attempt.AgentImageDigest != release.AgentImageDigest ||
		attempt.ClaimVersion != value.Result.ClaimVersion {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, visualFoundationFenceRejected()
	}
	var authorization model.SceneAnalysisDispatchAuthorization
	if err := repo.database.WithContext(ctx).First(&authorization, "attempt_id = ?", attempt.ID).Error; err != nil {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, err
	}
	if authorization.AuthorizationHash != value.Result.DispatchAuthorizationHash {
		return model.SceneAnalysisInvocationRecord{}, model.SceneAnalysisAttempt{}, visualFoundationFenceRejected()
	}
	return invocation, attempt, nil
}

func visualFoundationFenceRejected() error {
	return &agentapp.Error{Code: "result_fence_rejected", Message: "Visual Foundation result fence was rejected"}
}

func visualFoundationInvocationRecord(
	value agentapp.VisualFoundationInvocationRecord,
) (model.SceneAnalysisInvocationRecord, error) {
	invocation := value.Invocation
	if err := invocation.Validate(); err != nil {
		return model.SceneAnalysisInvocationRecord{}, err
	}
	identifiers := []string{
		invocation.InvocationID, value.WorkspaceID, value.ProjectID, value.WorkflowRunID,
		value.NodeRunID, value.ReleaseID, invocation.Control.ControlRecordID, value.Manifest.ID,
	}
	parsed := make([]uuid.UUID, len(identifiers))
	for index, identifier := range identifiers {
		parsedID, err := uuid.Parse(identifier)
		if err != nil {
			return model.SceneAnalysisInvocationRecord{}, err
		}
		parsed[index] = parsedID
	}
	payload, err := json.Marshal(invocation.Payload)
	if err != nil {
		return model.SceneAnalysisInvocationRecord{}, err
	}
	budget, err := json.Marshal(invocation.Budget)
	if err != nil {
		return model.SceneAnalysisInvocationRecord{}, err
	}
	return model.SceneAnalysisInvocationRecord{
		ID: parsed[0], WorkspaceID: parsed[1], ProjectID: parsed[2], WorkflowRunID: parsed[3], NodeRunID: parsed[4],
		ReleaseID: parsed[5], ControlRecordID: parsed[6], ControlRevision: invocation.Control.ControlRevision,
		ControlHash: invocation.Control.ControlHash, ReleaseFence: invocation.Control.ReleaseFence,
		WireSchemaID: invocation.WireSchemaVersion, StageKey: contract.VisualFoundationStageKey,
		ProfileKey: "default", StageInstanceKey: invocation.StageInstanceKey(), InputHash: invocation.InputHash,
		SourceVersionID: nil, SourceHash: invocation.Payload.StageInput.ProductionWorldOwnerSetHash,
		ShardManifestID: parsed[7], ShardManifestHash: value.Manifest.ManifestHash,
		ShardKey: invocation.Payload.Shard.ShardKey, Payload: datatypes.JSON(payload), Budget: datatypes.JSON(budget),
		Status: "queued", CreatedAt: value.CreatedAt, UpdatedAt: value.CreatedAt,
	}, nil
}

func (repo *sceneAnalysisRepository) visualFoundationInvocationDomain(
	ctx context.Context,
	record model.SceneAnalysisInvocationRecord,
) (agentapp.VisualFoundationInvocationRecord, error) {
	if record.StageKey != contract.VisualFoundationStageKey || record.SourceVersionID != nil {
		return agentapp.VisualFoundationInvocationRecord{}, errors.New("persisted Visual Foundation invocation has invalid source ownership")
	}
	var release model.SceneAnalysisRelease
	if err := repo.database.WithContext(ctx).First(&release, "id = ?", record.ReleaseID).Error; err != nil {
		return agentapp.VisualFoundationInvocationRecord{}, err
	}
	var attempt model.SceneAnalysisAttempt
	if err := repo.database.WithContext(ctx).Where("invocation_id = ?", record.ID).
		Order("claim_version DESC").First(&attempt).Error; err != nil {
		return agentapp.VisualFoundationInvocationRecord{}, normalizeSceneAnalysisNotFound(err)
	}
	var payload contract.VisualFoundationPayload
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return agentapp.VisualFoundationInvocationRecord{}, err
	}
	var budget contract.SceneAnalysisExecutionBudget
	if err := json.Unmarshal(record.Budget, &budget); err != nil {
		return agentapp.VisualFoundationInvocationRecord{}, err
	}
	invocation, err := contract.NewVisualFoundationInvocation(
		record.ID.String(),
		attempt.ID.String(),
		contract.SceneAnalysisReleaseIdentity{
			SkillReleaseID: release.SkillReleaseID.String(), SkillReleaseHash: release.SkillReleaseHash,
			StageReleaseHash: release.StageReleaseHash, BundleContentHash: release.BundleContentHash,
			AgentImageDigest: release.AgentImageDigest,
		},
		contract.SceneAnalysisControlProof{
			ControlRecordID: record.ControlRecordID.String(), ControlRevision: record.ControlRevision,
			Status: "approved", ControlHash: record.ControlHash, ReleaseFence: record.ReleaseFence,
		},
		budget,
		payload,
	)
	if err != nil || invocation.InputHash != record.InputHash || invocation.StageInstanceKey() != record.StageInstanceKey ||
		record.SourceHash != invocation.Payload.StageInput.ProductionWorldOwnerSetHash {
		return agentapp.VisualFoundationInvocationRecord{}, errors.New("persisted Visual Foundation invocation hash drifted")
	}
	return agentapp.VisualFoundationInvocationRecord{
		Invocation: invocation, WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(),
		ReleaseID: record.ReleaseID.String(),
		Manifest: agentapp.ManifestRecord{
			ID: record.ShardManifestID.String(), WorkspaceID: record.WorkspaceID.String(),
			WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(),
			StageKey: record.StageKey, ManifestHash: record.ShardManifestHash,
		},
		CreatedAt: record.CreatedAt,
	}, nil
}

func visualFoundationResultRecord(
	value agentapp.VisualFoundationResultAcceptance,
) (model.SceneAnalysisResult, error) {
	id, err := uuid.Parse(value.ResultID)
	if err != nil {
		return model.SceneAnalysisResult{}, err
	}
	attemptID, err := uuid.Parse(value.Invocation.Invocation.AttemptID)
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

func visualFoundationCandidateRecord(
	value agentapp.VisualFoundationResultAcceptance,
	invocation model.SceneAnalysisInvocationRecord,
	result model.SceneAnalysisResult,
) (model.SceneAnalysisCandidateRevision, error) {
	id, err := uuid.Parse(value.CandidateID)
	if err != nil {
		return model.SceneAnalysisCandidateRevision{}, err
	}
	record := model.SceneAnalysisCandidateRevision{
		ID: id, WorkspaceID: invocation.WorkspaceID, ProjectID: invocation.ProjectID,
		StageInstanceKey: invocation.StageInstanceKey, RevisionNo: 1,
		CandidateType: "visual_foundation_candidate", SourceInvocationID: invocation.ID,
		SourceResultID: result.ID, SourceResultHash: value.Result.ResultHash,
		Candidate:            datatypes.JSON(append([]byte(nil), value.Result.Candidate...)),
		CandidateContentHash: *value.Result.OutputHash, CreatedAt: value.AcceptedAt,
	}
	revisionHash, err := visualFoundationCandidateRevisionHash(record)
	if err != nil {
		return model.SceneAnalysisCandidateRevision{}, err
	}
	record.CandidateRevisionHash = revisionHash
	return record, nil
}

func visualFoundationCandidateRevisionHash(value model.SceneAnalysisCandidateRevision) (string, error) {
	material, err := json.Marshal(map[string]any{
		"contract_id":        "visual-foundation-candidate-revision-production",
		"stage_instance_key": value.StageInstanceKey, "revision": value.RevisionNo,
		"candidate_type": value.CandidateType, "source_invocation_id": value.SourceInvocationID.String(),
		"source_result_id": value.SourceResultID.String(), "source_result_hash": value.SourceResultHash,
		"candidate_content_hash": value.CandidateContentHash,
	})
	if err != nil {
		return "", err
	}
	return contract.ProductionCanonicalHash(material)
}

var _ agentapp.VisualFoundationTransactions = (*VisualFoundationStore)(nil)
var _ agentapp.VisualFoundationRepository = (*sceneAnalysisRepository)(nil)
