package gormdb

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

var ErrCandidateResultConflict = errors.New("agent candidate result conflicts with the accepted revision")

func StageInvocation(record model.AgentInvocation) (contract.StageInvocation, error) {
	var policy contract.StageExecutionPolicy
	if err := json.Unmarshal(record.ExecutionPolicy, &policy); err != nil {
		return contract.StageInvocation{}, err
	}
	var payload contract.StageInvocationPayload
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return contract.StageInvocation{}, err
	}
	value := contract.StageInvocation{
		InvocationID: record.ID.String(), Kind: record.Kind, WireSchemaVersion: record.WireSchemaVersion,
		InputHash: record.InputHash, ExecutionPolicy: policy, Payload: payload,
	}
	if err := value.Validate(); err != nil {
		return contract.StageInvocation{}, err
	}
	return value, nil
}

func AcceptInvocationCandidate(
	database *gorm.DB,
	invocation model.AgentInvocation,
	request contract.StageInvocation,
	result contract.StageResult,
	now time.Time,
) (model.StageCandidateRevision, error) {
	if err := request.Validate(); err != nil {
		return model.StageCandidateRevision{}, err
	}
	if err := result.ValidateFor(request); err != nil || result.Status != "succeeded" || result.ResultHash == nil {
		return model.StageCandidateRevision{}, errors.New("only a valid successful Agent result can create a candidate revision")
	}
	stageInstanceKey, err := request.StageInstanceKey()
	if err != nil || invocation.StageInstanceKey != stageInstanceKey || invocation.ID.String() != request.InvocationID {
		return model.StageCandidateRevision{}, errors.New("Agent invocation identity does not match the persisted stage")
	}

	var existing model.StageCandidateRevision
	err = database.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source_invocation_id = ?", invocation.ID).First(&existing).Error
	if err == nil {
		if existing.SourceResultHash == nil || *existing.SourceResultHash != *result.ResultHash || existing.CandidateContentHash != *result.ResultHash {
			return model.StageCandidateRevision{}, ErrCandidateResultConflict
		}
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.StageCandidateRevision{}, err
	}

	origin := contract.InvocationCandidateOrigin{SourceInvocationID: request.InvocationID, SourceResultHash: *result.ResultHash}
	originJSON, err := json.Marshal(origin)
	if err != nil {
		return model.StageCandidateRevision{}, err
	}
	revisionHash, err := (contract.CandidateRevisionMaterial{
		StageInstanceKey: stageInstanceKey, RevisionNo: 1, OriginKind: "invocation",
		InvocationOrigin: &origin, CandidateContentHash: *result.ResultHash,
	}).Hash()
	if err != nil {
		return model.StageCandidateRevision{}, err
	}
	revision := model.StageCandidateRevision{
		ID: uuid.New(), WorkspaceID: invocation.WorkspaceID, StageInstanceKey: stageInstanceKey,
		RevisionNo: 1, OriginKind: "invocation", InvocationOrigin: datatypes.JSON(originJSON),
		SourceInvocationID: &invocation.ID, SourceResultHash: result.ResultHash,
		Candidate:            datatypes.JSON(append([]byte(nil), result.Candidate...)),
		CandidateContentHash: *result.ResultHash, CandidateRevisionHash: revisionHash, CreatedAt: now,
	}
	if err = database.Omit(clause.Associations).Create(&revision).Error; err != nil {
		return model.StageCandidateRevision{}, err
	}
	head := model.StageCandidateHead{
		WorkspaceID: invocation.WorkspaceID, StageInstanceKey: stageInstanceKey,
		CurrentRevisionID: revision.ID, CurrentCandidateRevisionHash: revisionHash,
		Revision: 1, UpdatedAt: now,
	}
	if err = database.Omit(clause.Associations).Create(&head).Error; err != nil {
		return model.StageCandidateRevision{}, err
	}
	return revision, nil
}
