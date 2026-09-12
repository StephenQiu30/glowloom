package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/google/uuid"
	"gorm.io/gorm"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	presetgorm "github.com/StephenQiu30/lanverse/backend/internal/preset/adapter/gormdb"
	presetcatalog "github.com/StephenQiu30/lanverse/backend/internal/preset/catalog"
	storygraphgorm "github.com/StephenQiu30/lanverse/backend/internal/storygraph/adapter/gormdb"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
)

type ReferencePlanSourceStore struct{ database *gorm.DB }

func NewReferencePlanSourceStore(database *gorm.DB) *ReferencePlanSourceStore {
	return &ReferencePlanSourceStore{database: database}
}

func (store *ReferencePlanSourceStore) CurrentWorld(
	ctx context.Context,
	workspaceID string,
	projectID string,
) (storygraphdomain.ReferencePlanWorldReadSet, error) {
	return currentReferencePlanWorld(ctx, store.database, workspaceID, projectID)
}

func (store *ReferencePlanSourceStore) ExactVisualFoundationCandidate(
	ctx context.Context,
	workspaceID string,
	projectID string,
	candidateID string,
	revisionHash string,
) (workflowapp.ReferencePlanVisualFoundationCandidateRevision, error) {
	return exactReferencePlanVisualFoundationCandidate(
		ctx, store.database, workspaceID, projectID, candidateID, revisionHash,
	)
}

func ValidateCurrentReferencePlanInput(
	ctx context.Context,
	database *gorm.DB,
	input contract.ReferencePlanInput,
) error {
	world, err := currentReferencePlanWorld(ctx, database, input.WorkspaceID, input.ProjectID)
	if err != nil {
		return staleReferencePlanInput(err)
	}
	revision, err := exactReferencePlanVisualFoundationCandidate(
		ctx,
		database,
		input.WorkspaceID,
		input.ProjectID,
		input.VisualFoundationCandidateRevisionID,
		input.VisualFoundationCandidateRevisionHash,
	)
	if err != nil {
		return staleReferencePlanInput(err)
	}
	if err = ValidateCurrentVisualFoundationInput(ctx, database, revision.SourceInput); err != nil {
		return staleReferencePlanInput(err)
	}
	selection, err := presetgorm.NewProjectSelectionStore(database).Current(
		ctx, input.WorkspaceID, input.ProjectID,
	)
	if err != nil {
		return staleReferencePlanInput(err)
	}
	release, found, err := presetcatalog.FindCuratedRelease(selection.PresetRelease.Key, selection.PresetRelease.Release)
	if err != nil || !found || release.ContentHash != selection.PresetRelease.ContentHash {
		return staleReferencePlanInput(err)
	}
	rebuilt, _, err := workflowapp.CompileReferencePlanInput(workflowapp.ReferencePlanInputCommand{
		Inventory: world.Inventory, VisualFoundationRevision: revision, PresetRelease: release,
	})
	if err != nil || !reflect.DeepEqual(rebuilt, input) {
		return staleReferencePlanInput(err)
	}
	return nil
}

func currentReferencePlanWorld(
	ctx context.Context,
	database *gorm.DB,
	workspaceID string,
	projectID string,
) (storygraphdomain.ReferencePlanWorldReadSet, error) {
	workspace, workspaceErr := uuid.Parse(workspaceID)
	project, projectErr := uuid.Parse(projectID)
	if workspaceErr != nil || projectErr != nil || workspace == uuid.Nil || project == uuid.Nil {
		return storygraphdomain.ReferencePlanWorldReadSet{}, errors.New("invalid Reference Plan worker scope")
	}
	store := storygraphgorm.New(database)
	version, err := store.GetCurrentReferencePlanVersion(ctx, workspaceID, projectID)
	if err != nil {
		return storygraphdomain.ReferencePlanWorldReadSet{}, err
	}
	return storygraphdomain.BuildReferencePlanWorldReadSet(version)
}

func exactReferencePlanVisualFoundationCandidate(
	ctx context.Context,
	database *gorm.DB,
	workspaceID string,
	projectID string,
	candidateID string,
	revisionHash string,
) (workflowapp.ReferencePlanVisualFoundationCandidateRevision, error) {
	workspace, workspaceErr := uuid.Parse(workspaceID)
	project, projectErr := uuid.Parse(projectID)
	candidate, candidateErr := uuid.Parse(candidateID)
	if workspaceErr != nil || projectErr != nil || candidateErr != nil ||
		workspace == uuid.Nil || project == uuid.Nil || candidate == uuid.Nil {
		return workflowapp.ReferencePlanVisualFoundationCandidateRevision{}, errors.New("invalid Visual Foundation Candidate identity")
	}
	var record model.SceneAnalysisCandidateRevision
	if err := database.WithContext(ctx).Where(
		"id = ? AND workspace_id = ? AND project_id = ? AND candidate_type = ? AND candidate_revision_hash = ?",
		candidate, workspace, project, "visual_foundation_candidate", revisionHash,
	).First(&record).Error; err != nil {
		return workflowapp.ReferencePlanVisualFoundationCandidateRevision{}, err
	}
	var invocation model.SceneAnalysisInvocationRecord
	if err := database.WithContext(ctx).Where(
		"id = ? AND stage_key = ?",
		record.SourceInvocationID, contract.VisualFoundationStageKey,
	).First(&invocation).Error; err != nil {
		return workflowapp.ReferencePlanVisualFoundationCandidateRevision{}, err
	}
	var payload contract.VisualFoundationPayload
	if err := json.Unmarshal(invocation.Payload, &payload); err != nil || payload.Validate() != nil {
		return workflowapp.ReferencePlanVisualFoundationCandidateRevision{}, errors.New("persisted Visual Foundation invocation is invalid")
	}
	contentHash, err := platformcanonical.Hash(json.RawMessage(record.Candidate))
	if err != nil || contentHash != record.CandidateContentHash {
		return workflowapp.ReferencePlanVisualFoundationCandidateRevision{}, errors.New("persisted Visual Foundation Candidate content has drifted")
	}
	decodedCandidate, _, err := contract.DecodeVisualFoundationCandidate(json.RawMessage(record.Candidate))
	if err != nil || decodedCandidate.ValidateFor(payload.StageInput) != nil {
		return workflowapp.ReferencePlanVisualFoundationCandidateRevision{}, errors.New("persisted Visual Foundation Candidate lineage has drifted")
	}
	revisionMaterial, err := json.Marshal(map[string]any{
		"contract_id":        "visual-foundation-candidate-revision-production",
		"stage_instance_key": record.StageInstanceKey, "revision": record.RevisionNo,
		"candidate_type": record.CandidateType, "source_invocation_id": record.SourceInvocationID.String(),
		"source_result_id": record.SourceResultID.String(), "source_result_hash": record.SourceResultHash,
		"candidate_content_hash": record.CandidateContentHash,
	})
	if err != nil {
		return workflowapp.ReferencePlanVisualFoundationCandidateRevision{}, err
	}
	rebuiltRevisionHash, err := contract.ProductionCanonicalHash(revisionMaterial)
	if err != nil || rebuiltRevisionHash != record.CandidateRevisionHash {
		return workflowapp.ReferencePlanVisualFoundationCandidateRevision{}, errors.New("persisted Visual Foundation Candidate revision has drifted")
	}
	return workflowapp.ReferencePlanVisualFoundationCandidateRevision{
		ID: record.ID.String(), Revision: record.RevisionNo, RevisionHash: record.CandidateRevisionHash,
		CandidateContentHash: record.CandidateContentHash,
		Candidate:            append([]byte(nil), record.Candidate...), SourceInput: payload.StageInput,
	}, nil
}

func staleReferencePlanInput(cause error) error {
	message := "Reference Plan input changed before Candidate acceptance"
	if cause != nil {
		message += ": " + cause.Error()
	}
	return &agentapp.Error{Code: "stale_reference_plan_input", Message: message}
}
