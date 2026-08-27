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

var (
	ErrCandidateResultConflict = errors.New("agent candidate result conflicts with the accepted revision")
	ErrCandidateHeadConflict   = errors.New("candidate head changed before repair")
	ErrCandidateRepairNoChange = errors.New("candidate repair did not change candidate content")
)

type CandidateRepairAdvance struct {
	WorkspaceID                   uuid.UUID
	StageInstanceKey              string
	ExpectedRevisionID            uuid.UUID
	ExpectedCandidateRevisionHash string
	ExpectedHeadRevision          int64
	RepairInvocationID            uuid.UUID
	RepairResultHash              string
	Candidate                     json.RawMessage
	CreatedAt                     time.Time
}

func AdvanceCandidateHeadWithRepair(
	database *gorm.DB,
	advance CandidateRepairAdvance,
) (model.StageCandidateRevision, error) {
	if database == nil || advance.WorkspaceID == uuid.Nil || advance.ExpectedRevisionID == uuid.Nil ||
		advance.RepairInvocationID == uuid.Nil || advance.ExpectedHeadRevision < 1 || advance.CreatedAt.IsZero() {
		return model.StageCandidateRevision{}, errors.New("invalid Candidate repair advance")
	}
	var candidateObject map[string]any
	if err := json.Unmarshal(advance.Candidate, &candidateObject); err != nil || candidateObject == nil {
		return model.StageCandidateRevision{}, errors.New("Candidate repair output must be an object")
	}
	candidateContentHash, err := contract.CanonicalHash(advance.Candidate)
	if err != nil {
		return model.StageCandidateRevision{}, err
	}
	var created model.StageCandidateRevision
	err = database.Transaction(func(transaction *gorm.DB) error {
		var head model.StageCandidateHead
		loadErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND stage_instance_key = ?", advance.WorkspaceID, advance.StageInstanceKey).
			First(&head).Error
		if errors.Is(loadErr, gorm.ErrRecordNotFound) {
			return ErrCandidateHeadConflict
		}
		if loadErr != nil {
			return loadErr
		}
		if head.CurrentRevisionID != advance.ExpectedRevisionID ||
			head.CurrentCandidateRevisionHash != advance.ExpectedCandidateRevisionHash ||
			head.Revision != advance.ExpectedHeadRevision {
			return ErrCandidateHeadConflict
		}

		var parent model.StageCandidateRevision
		if loadErr = transaction.First(&parent, "id = ?", advance.ExpectedRevisionID).Error; loadErr != nil {
			return loadErr
		}
		if parent.WorkspaceID != advance.WorkspaceID || parent.StageInstanceKey != advance.StageInstanceKey ||
			parent.RevisionNo != advance.ExpectedHeadRevision ||
			parent.CandidateRevisionHash != advance.ExpectedCandidateRevisionHash {
			return ErrCandidateHeadConflict
		}
		if candidateContentHash == parent.CandidateContentHash {
			return ErrCandidateRepairNoChange
		}

		var repairInvocation model.AgentInvocation
		if loadErr = transaction.First(&repairInvocation, "id = ?", advance.RepairInvocationID).Error; loadErr != nil {
			return loadErr
		}
		if repairInvocation.WorkspaceID != advance.WorkspaceID || repairInvocation.Kind != "storygraph_stage" ||
			repairInvocation.WireSchemaVersion != contract.StoryGraphWireSchemaVersion ||
			repairInvocation.Stage != "repair_candidate" || repairInvocation.Status != "succeeded" ||
			repairInvocation.ResultHash == nil || *repairInvocation.ResultHash != advance.RepairResultHash ||
			repairInvocation.CandidateType == nil || *repairInvocation.CandidateType != "candidate_repair_patch" {
			return errors.New("Candidate repair Invocation is not an accepted successful result")
		}
		persistedRepairHash, hashErr := contract.CanonicalHash(json.RawMessage(repairInvocation.Candidate))
		if hashErr != nil || persistedRepairHash != advance.RepairResultHash {
			return errors.New("Candidate repair Result hash has drifted")
		}

		parentHash := parent.CandidateRevisionHash
		repairOrigin := contract.RepairCandidateOrigin{
			RepairInvocationID: repairInvocation.ID.String(), RepairResultHash: advance.RepairResultHash,
		}
		revisionHash, hashErr := (contract.CandidateRevisionMaterial{
			StageInstanceKey: advance.StageInstanceKey, RevisionNo: parent.RevisionNo + 1,
			ParentCandidateRevisionHash: &parentHash, OriginKind: "repair", RepairOrigin: &repairOrigin,
			CandidateContentHash: candidateContentHash,
		}).Hash()
		if hashErr != nil {
			return hashErr
		}
		repairOriginJSON, marshalErr := json.Marshal(repairOrigin)
		if marshalErr != nil {
			return marshalErr
		}
		created = model.StageCandidateRevision{
			ID: uuid.New(), WorkspaceID: advance.WorkspaceID, StageInstanceKey: advance.StageInstanceKey,
			RevisionNo:                parent.RevisionNo + 1,
			ParentCandidateRevisionID: &parent.ID, ParentCandidateRevisionHash: &parentHash,
			OriginKind: "repair", RepairOrigin: datatypes.JSON(repairOriginJSON),
			Candidate:            datatypes.JSON(append([]byte(nil), advance.Candidate...)),
			CandidateContentHash: candidateContentHash, CandidateRevisionHash: revisionHash,
			CreatedAt: advance.CreatedAt.UTC(),
		}
		if createErr := transaction.Omit(clause.Associations).Create(&created).Error; createErr != nil {
			return createErr
		}
		updated := transaction.Model(&model.StageCandidateHead{}).
			Where("workspace_id = ? AND stage_instance_key = ? AND current_revision_id = ? AND current_candidate_revision_hash = ? AND revision = ?",
				advance.WorkspaceID, advance.StageInstanceKey, advance.ExpectedRevisionID,
				advance.ExpectedCandidateRevisionHash, advance.ExpectedHeadRevision).
			Updates(map[string]any{
				"current_revision_id": created.ID, "current_candidate_revision_hash": created.CandidateRevisionHash,
				"revision": created.RevisionNo, "updated_at": advance.CreatedAt.UTC(),
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrCandidateHeadConflict
		}
		return nil
	})
	return created, err
}

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
