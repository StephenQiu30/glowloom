package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	app "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	database "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type VisionReviewInputValidator func(context.Context, *gorm.DB, app.ExecuteVisionReviewCommand) error

type VisionReviewStore struct {
	database  *gorm.DB
	validator VisionReviewInputValidator
}

func NewVisionReviewStore(db *gorm.DB, validator VisionReviewInputValidator) (*VisionReviewStore, error) {
	if db == nil || validator == nil {
		return nil, errors.New("Vision Review database and current input validator are required")
	}
	return &VisionReviewStore{database: db, validator: validator}, nil
}

func (store *VisionReviewStore) WithinVisionReviewTransaction(ctx context.Context, operation func(app.VisionReviewRepository) error) error {
	if _, nested := store.database.Statement.ConnPool.(gorm.TxCommitter); nested {
		return errors.New("Vision Review execution requires its own commit boundary")
	}
	return database.WithinSerializableTransaction(ctx, store.database, func(tx *gorm.DB) error {
		return operation(&visionReviewRepository{sceneAnalysisRepository: &sceneAnalysisRepository{database: tx}, validator: store.validator})
	})
}

type visionReviewRepository struct {
	*sceneAnalysisRepository
	validator VisionReviewInputValidator
}

func (repo *visionReviewRepository) ValidateVisionReviewInput(ctx context.Context, command app.ExecuteVisionReviewCommand) error {
	if command.Input.Validate() != nil {
		return errors.New("invalid Vision Review input")
	}
	if err := repo.validator(ctx, repo.database, command); err != nil {
		return &app.Error{Code: "stale_vision_review_input", Message: "Vision Review authorization or current facts changed"}
	}
	return nil
}

func (repo *visionReviewRepository) CreateVisionReviewExecution(ctx context.Context, value app.VisionReviewInvocationRecord, auth app.DispatchAuthorizationRecord) error {
	var node model.NodeRunProjection
	if err := repo.database.WithContext(ctx).First(&node, "id = ? AND workflow_run_id = ?", value.NodeRunID, value.WorkflowRunID).Error; err != nil {
		return err
	}
	if node.Status != "RUNNING" && node.Status != "RETRYING" {
		return errors.New("completed Vision Review node cannot dispatch")
	}
	inv := value.Invocation
	if inv.Validate() != nil || auth.AttemptID != inv.AttemptID {
		return errors.New("invalid Vision Review dispatch record")
	}
	payload, err := json.Marshal(inv.Payload)
	if err != nil {
		return err
	}
	budget, err := json.Marshal(inv.Budget)
	if err != nil {
		return err
	}
	ids := []string{inv.InvocationID, inv.Payload.Scope.WorkspaceID, inv.Payload.Scope.ProjectID, value.WorkflowRunID, value.NodeRunID, value.ReleaseID, inv.Control.ControlRecordID, value.Manifest.ID}
	parsed := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		parsed[i], err = uuid.Parse(id)
		if err != nil || parsed[i] == uuid.Nil {
			return errors.New("invalid Vision Review record identity")
		}
	}
	row := model.SceneAnalysisInvocationRecord{
		ID: parsed[0], WorkspaceID: parsed[1], ProjectID: parsed[2], WorkflowRunID: parsed[3], NodeRunID: parsed[4], ReleaseID: parsed[5], ControlRecordID: parsed[6],
		ControlRevision: inv.Control.ControlRevision, ControlHash: inv.Control.ControlHash, ReleaseFence: inv.Control.ReleaseFence,
		WireSchemaID: inv.WireSchemaVersion, StageKey: contract.VisionReviewStageKey, ProfileKey: "default", StageInstanceKey: inv.StageInstanceKey(), InputHash: inv.InputHash,
		SourceHash: inv.Payload.StageInput.Subject.InputHash, ShardManifestID: parsed[7], ShardManifestHash: value.Manifest.ManifestHash,
		ShardKey: inv.Payload.Shard.ShardKey, Payload: datatypes.JSON(payload), Budget: datatypes.JSON(budget), Status: "queued", CreatedAt: value.CreatedAt, UpdatedAt: value.CreatedAt,
	}
	if err := repo.database.WithContext(ctx).Omit(clause.Associations).Create(&row).Error; err != nil {
		return err
	}
	if err := repo.CreateAttempt(ctx, app.AttemptRecord{ID: inv.AttemptID, InvocationID: inv.InvocationID, ClaimVersion: 1, ControlHash: inv.Control.ControlHash,
		ReleaseFence: inv.Control.ReleaseFence, AgentImageDigest: inv.StageRelease.AgentImageDigest, DispatchedAt: value.CreatedAt}); err != nil {
		return err
	}
	return repo.CreateDispatchAuthorization(ctx, auth)
}

// The caller holds the release Control and current Owner locks before the
// Invocation/Attempt locks. A node owns exactly one dispatch, regardless of input.
func (repo *visionReviewRepository) FindVisionReviewExecution(ctx context.Context, runID, nodeID string) (app.VisionReviewExecutionState, error) {
	var rows []model.SceneAnalysisInvocationRecord
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("workflow_run_id = ? AND node_run_id = ? AND stage_key = ?", runID, nodeID, contract.VisionReviewStageKey).Find(&rows).Error; err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	if len(rows) == 0 {
		return app.VisionReviewExecutionState{}, app.ErrNotFound
	}
	if len(rows) != 1 {
		return app.VisionReviewExecutionState{}, errors.New("Vision Review node has multiple invocations")
	}
	row := rows[0]
	var attempts []model.SceneAnalysisAttempt
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("invocation_id = ?", row.ID).Find(&attempts).Error; err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	if len(attempts) != 1 {
		return app.VisionReviewExecutionState{}, errors.New("Vision Review requires exactly one attempt")
	}
	attempt := attempts[0]
	var release model.SceneAnalysisRelease
	var manifest model.ShardManifest
	var auth model.SceneAnalysisDispatchAuthorization
	for _, query := range []struct {
		out   any
		key   string
		value any
	}{
		{&release, "id", row.ReleaseID}, {&manifest, "id", row.ShardManifestID}, {&auth, "attempt_id", attempt.ID},
	} {
		if err := repo.database.WithContext(ctx).Where(query.key+" = ?", query.value).First(query.out).Error; err != nil {
			return app.VisionReviewExecutionState{}, err
		}
	}
	var payload contract.VisionReviewPayload
	var budget contract.SceneAnalysisExecutionBudget
	if canonical.Decode(row.Payload, &payload) != nil || canonical.Decode(row.Budget, &budget) != nil {
		return app.VisionReviewExecutionState{}, errors.New("invalid persisted Vision Review payload")
	}
	inv, err := contract.NewVisionReviewInvocation(row.ID.String(), attempt.ID.String(), contract.SceneAnalysisReleaseIdentity{
		SkillReleaseID: release.SkillReleaseID.String(), SkillReleaseHash: release.SkillReleaseHash, StageReleaseHash: release.StageReleaseHash,
		BundleContentHash: release.BundleContentHash, AgentImageDigest: release.AgentImageDigest,
	}, contract.SceneAnalysisControlProof{ControlRecordID: row.ControlRecordID.String(), ControlRevision: row.ControlRevision, Status: "approved", ControlHash: row.ControlHash, ReleaseFence: row.ReleaseFence}, budget, payload)
	if err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	// Validate original JSONB field presence, not a reserialized typed payload
	// which would silently restore a missing zero-valued field.
	envelope, err := json.Marshal(inv)
	if err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(envelope, &wire); err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	wire["payload"], wire["budget"] = json.RawMessage(row.Payload), json.RawMessage(row.Budget)
	envelope, err = json.Marshal(wire)
	if err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	envelope, err = canonical.JSON(envelope)
	if err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	inv, err = contract.DecodeVisionReviewInvocation(envelope)
	if err != nil || row.SourceVersionID != nil || row.StageKey != contract.VisionReviewStageKey || release.StageKey != row.StageKey || release.ModelCapability != "vision" ||
		row.ProfileKey != "default" || release.ProfileKey != row.ProfileKey || row.WireSchemaID != inv.WireSchemaVersion ||
		row.WorkspaceID.String() != payload.Scope.WorkspaceID || row.ProjectID.String() != payload.Scope.ProjectID ||
		row.InputHash != inv.InputHash || row.StageInstanceKey != inv.StageInstanceKey() || row.SourceHash != payload.StageInput.Subject.InputHash ||
		row.ShardManifestID.String() != payload.Shard.ManifestID || row.ShardManifestHash != payload.Shard.ManifestHash || row.ShardKey != payload.Shard.ShardKey ||
		attempt.ClaimVersion != 1 || attempt.ControlHash != row.ControlHash || attempt.ReleaseFence != row.ReleaseFence || attempt.AgentImageDigest != release.AgentImageDigest ||
		!attempt.DispatchedAt.Equal(row.CreatedAt) || !auth.IssuedAt.Equal(attempt.DispatchedAt) || !auth.ExpiresAt.After(auth.IssuedAt) || len(auth.AuthorizationHash) != 64 ||
		manifest.WorkspaceID != row.WorkspaceID || manifest.WorkflowRunID != row.WorkflowRunID || manifest.NodeRunID != row.NodeRunID ||
		manifest.Stage != row.StageKey || manifest.Version != 1 || manifest.ParentManifestHash != nil || manifest.RootInputHash != row.SourceHash || manifest.ManifestHash != row.ShardManifestHash {
		return app.VisionReviewExecutionState{}, errors.New("persisted Vision Review execution identity drifted")
	}
	state := app.VisionReviewExecutionState{
		Record: app.VisionReviewInvocationRecord{Invocation: inv, WorkflowRunID: runID, NodeRunID: nodeID, ReleaseID: row.ReleaseID.String(), CreatedAt: row.CreatedAt,
			Manifest: app.ManifestRecord{ID: manifest.ID.String(), WorkspaceID: manifest.WorkspaceID.String(), WorkflowRunID: runID, NodeRunID: nodeID, StageKey: manifest.Stage, RootInputHash: manifest.RootInputHash, Shards: json.RawMessage(manifest.Shards), CoverageHash: manifest.CoverageHash, ManifestHash: manifest.ManifestHash, CreatedAt: manifest.CreatedAt}},
		Authorization: app.DispatchAuthorizationRecord{AttemptID: attempt.ID.String(), AuthorizationHash: auth.AuthorizationHash, ExpiresAt: auth.ExpiresAt, IssuedAt: auth.IssuedAt}, Status: row.Status,
	}
	coverage, coverageErr := contract.ProductionCanonicalHash(json.RawMessage(manifest.Shards))
	material, materialErr := json.Marshal(map[string]any{"contract_id": "vision-review-shard-manifest-production", "manifest_id": manifest.ID.String(), "workflow_run_id": runID, "node_run_id": nodeID,
		"stage_key": manifest.Stage, "root_input_hash": manifest.RootInputHash, "shards": json.RawMessage(manifest.Shards), "coverage_hash": manifest.CoverageHash})
	manifestHash, manifestErr := contract.ProductionCanonicalHash(material)
	if coverageErr != nil || materialErr != nil || manifestErr != nil || coverage != manifest.CoverageHash || manifestHash != manifest.ManifestHash {
		return app.VisionReviewExecutionState{}, errors.New("Vision Review manifest content drifted")
	}
	var results []model.SceneAnalysisResult
	if err := repo.database.WithContext(ctx).Where("attempt_id = ?", attempt.ID).Find(&results).Error; err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	var candidates []model.SceneAnalysisCandidateRevision
	if err := repo.database.WithContext(ctx).Where("source_invocation_id = ?", row.ID).Find(&candidates).Error; err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	if row.Status == "running" {
		if attempt.Status != "dispatched" || attempt.CompletedAt != nil || len(results) != 0 || len(candidates) != 0 {
			return app.VisionReviewExecutionState{}, errors.New("Vision Review dispatch state drifted")
		}
		return state, nil
	}
	if attempt.Status != "completed" || attempt.CompletedAt == nil || len(results) != 1 {
		return app.VisionReviewExecutionState{}, errors.New("Vision Review result is incomplete")
	}
	resultRow := results[0]
	result, err := contract.DecodeVisionReviewAttemptResult(resultRow.Result)
	if err != nil || result.ValidateFor(inv, 1, auth.AuthorizationHash) != nil || result.Status != row.Status || resultRow.Status != result.Status ||
		resultRow.InputHash != result.InputHash || !reflect.DeepEqual(resultRow.OutputHash, result.OutputHash) || resultRow.DiagnosticHash != result.DiagnosticHash ||
		!resultRow.CompletedAt.Equal(result.CompletedAt.Truncate(time.Microsecond)) {
		return app.VisionReviewExecutionState{}, errors.New("persisted Vision Review result drifted")
	}
	state.Result = &result
	if result.Status != "accepted" {
		if len(candidates) != 0 {
			return app.VisionReviewExecutionState{}, errors.New("unaccepted Vision Review has a candidate")
		}
		return state, nil
	}
	if len(candidates) != 1 {
		return app.VisionReviewExecutionState{}, errors.New("accepted Vision Review candidate is missing")
	}
	candidate := candidates[0]
	hash, hashErr := contract.ProductionCanonicalHash(json.RawMessage(candidate.Candidate))
	revisionHash, revisionErr := visionReviewCandidateRevisionHash(candidate)
	var head model.SceneAnalysisCandidateHead
	if err := repo.database.WithContext(ctx).First(&head, "stage_instance_key = ?", row.StageInstanceKey).Error; err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	if hashErr != nil || revisionErr != nil || candidate.CandidateType != "vision_review_candidate" || candidate.RevisionNo != 1 ||
		candidate.WorkspaceID != row.WorkspaceID || candidate.ProjectID != row.ProjectID || candidate.StageInstanceKey != row.StageInstanceKey ||
		candidate.SourceResultID != resultRow.ID || candidate.SourceResultHash != result.ResultHash || candidate.CandidateContentHash != hash || hash != *result.OutputHash ||
		candidate.CandidateRevisionHash != revisionHash || head.CurrentRevisionID != candidate.ID || head.CurrentCandidateRevisionHash != revisionHash ||
		head.WorkspaceID != row.WorkspaceID || head.ProjectID != row.ProjectID || head.Revision != 1 {
		return app.VisionReviewExecutionState{}, errors.New("persisted Vision Review candidate drifted")
	}
	state.Candidate, err = repo.candidateDomain(ctx, candidate)
	return state, err
}

func (repo *visionReviewRepository) CompleteVisionReviewExecution(ctx context.Context, value app.VisionReviewResultAcceptance) (app.VisionReviewExecutionState, error) {
	if value.Command.WorkflowRunID != value.Record.WorkflowRunID || value.Command.NodeRunID != value.Record.NodeRunID {
		return app.VisionReviewExecutionState{}, errors.New("Vision Review result belongs to another Workflow node")
	}
	var control model.SceneAnalysisControlHead
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&control, "release_id = ?", value.Record.ReleaseID).Error; err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	expected := value.Record.Invocation.Control
	if control.Status != "approved" || control.ControlRecordID.String() != expected.ControlRecordID || control.ControlRevision != expected.ControlRevision || control.ControlHash != expected.ControlHash || control.ReleaseFence != expected.ReleaseFence {
		return app.VisionReviewExecutionState{}, &app.Error{Code: "release_not_executable", Message: "Vision Review release control changed"}
	}
	if err := repo.ValidateVisionReviewInput(ctx, value.Command); err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	state, err := repo.FindVisionReviewExecution(ctx, value.Record.WorkflowRunID, value.Record.NodeRunID)
	if err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	if !reflect.DeepEqual(state.Record.Invocation, value.Record.Invocation) || !reflect.DeepEqual(value.Command.Input, state.Record.Invocation.Payload.StageInput) ||
		value.Result.ValidateFor(state.Record.Invocation, 1, state.Authorization.AuthorizationHash) != nil {
		return app.VisionReviewExecutionState{}, errors.New("Vision Review result does not match its dispatch")
	}
	if state.Result != nil {
		if state.Result.ResultHash != value.Result.ResultHash {
			return app.VisionReviewExecutionState{}, &app.Error{Code: "result_conflict", Message: "Vision Review result is already final"}
		}
		return state, nil
	}
	resultID, err := uuid.Parse(value.ResultID)
	if err != nil || resultID == uuid.Nil {
		return app.VisionReviewExecutionState{}, errors.New("invalid Vision Review result identity")
	}
	raw, err := json.Marshal(value.Result)
	if err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	result := model.SceneAnalysisResult{ID: resultID, AttemptID: uuid.MustParse(state.Record.Invocation.AttemptID), Status: value.Result.Status, InputHash: value.Result.InputHash, OutputHash: value.Result.OutputHash, DiagnosticHash: value.Result.DiagnosticHash, Result: datatypes.JSON(raw), CompletedAt: value.Result.CompletedAt}
	if err := repo.database.WithContext(ctx).Omit(clause.Associations).Create(&result).Error; err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	if value.Result.Status == "accepted" {
		candidateID, err := uuid.Parse(value.CandidateID)
		if err != nil || candidateID == uuid.Nil {
			return app.VisionReviewExecutionState{}, errors.New("invalid Vision Review candidate identity")
		}
		inv := state.Record.Invocation
		candidate := model.SceneAnalysisCandidateRevision{ID: candidateID, WorkspaceID: uuid.MustParse(inv.Payload.Scope.WorkspaceID), ProjectID: uuid.MustParse(inv.Payload.Scope.ProjectID),
			StageInstanceKey: inv.StageInstanceKey(), RevisionNo: 1, CandidateType: "vision_review_candidate", SourceInvocationID: uuid.MustParse(inv.InvocationID), SourceResultID: result.ID, SourceResultHash: value.Result.ResultHash,
			Candidate: datatypes.JSON(value.Result.Candidate), CandidateContentHash: *value.Result.OutputHash, CreatedAt: value.AcceptedAt}
		candidate.CandidateRevisionHash, err = visionReviewCandidateRevisionHash(candidate)
		if err != nil {
			return app.VisionReviewExecutionState{}, err
		}
		if err := repo.database.WithContext(ctx).Omit(clause.Associations).Create(&candidate).Error; err != nil {
			return app.VisionReviewExecutionState{}, err
		}
		head := model.SceneAnalysisCandidateHead{StageInstanceKey: candidate.StageInstanceKey, WorkspaceID: candidate.WorkspaceID, ProjectID: candidate.ProjectID, CurrentRevisionID: candidate.ID, CurrentCandidateRevisionHash: candidate.CandidateRevisionHash, Revision: 1, UpdatedAt: value.AcceptedAt}
		if err := repo.database.WithContext(ctx).Omit(clause.Associations).Create(&head).Error; err != nil {
			return app.VisionReviewExecutionState{}, err
		}
	}
	if err := repo.finishAttemptAndInvocation(ctx, result.AttemptID, uuid.MustParse(state.Record.Invocation.InvocationID), value.Result.Status, value.AcceptedAt); err != nil {
		return app.VisionReviewExecutionState{}, err
	}
	return repo.FindVisionReviewExecution(ctx, value.Record.WorkflowRunID, value.Record.NodeRunID)
}

func visionReviewCandidateRevisionHash(value model.SceneAnalysisCandidateRevision) (string, error) {
	raw, err := json.Marshal(map[string]any{"contract_id": "vision-review-candidate-revision-production", "stage_instance_key": value.StageInstanceKey, "revision": value.RevisionNo,
		"candidate_type": value.CandidateType, "source_invocation_id": value.SourceInvocationID.String(), "source_result_id": value.SourceResultID.String(), "source_result_hash": value.SourceResultHash, "candidate_content_hash": value.CandidateContentHash})
	if err != nil {
		return "", err
	}
	return contract.ProductionCanonicalHash(raw)
}

var _ app.VisionReviewTransactions = (*VisionReviewStore)(nil)
