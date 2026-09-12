package gormdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	referenceapp "github.com/StephenQiu30/lanverse/backend/internal/production/reference/application"
	referencedomain "github.com/StephenQiu30/lanverse/backend/internal/production/reference/domain"
)

func (store *Store) ReadCurrentReferenceCoverage(
	ctx context.Context,
	workspaceID string,
	projectID string,
) (referenceapp.ReferenceCoverageFacts, error) {
	if store == nil || store.database == nil {
		return referenceapp.ReferenceCoverageFacts{}, errors.New("Reference Coverage store is unavailable")
	}
	workspace, workspaceErr := uuid.Parse(workspaceID)
	project, projectErr := uuid.Parse(projectID)
	if workspaceErr != nil || projectErr != nil || workspace == uuid.Nil || project == uuid.Nil {
		return referenceapp.ReferenceCoverageFacts{}, errors.New("invalid Reference Coverage scope")
	}

	var facts referenceapp.ReferenceCoverageFacts
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		plan, err := loadCurrentReferenceBriefPlan(ctx, transaction, workspace, project)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return referenceapp.ErrReferenceCoverageNotFound
		}
		if err != nil {
			return err
		}
		facts.Plan = plan
		targets := make(map[string]referencedomain.ReferencePlanTargetVersion, len(plan.TargetVersionRefs))
		facts.Targets = make([]referencedomain.ReferencePlanTargetVersion, len(plan.TargetVersionRefs))
		for index, targetRef := range plan.TargetVersionRefs {
			target, loadErr := loadReferenceBriefTarget(
				ctx, transaction, workspace, project, plan, targetRef.OwnerLogicalID,
			)
			if loadErr != nil {
				return loadErr
			}
			if target.ID != targetRef.OwnerVersionID || target.Revision != targetRef.OwnerRevision ||
				target.ContentHash != targetRef.OwnerContentHash {
				return referenceCoverageDrift("Reference Coverage Target ref has drifted")
			}
			facts.Targets[index] = target
			targets[target.TargetBusinessKey] = target
		}

		var invocations []model.SceneAnalysisInvocationRecord
		if err = transaction.WithContext(ctx).Where(
			"workspace_id = ? AND project_id = ? AND stage_key = ?",
			workspace, project, agentcontract.ReferenceBriefStageKey,
		).Order("updated_at DESC").Order("id DESC").Find(&invocations).Error; err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(targets))
		for _, invocation := range invocations {
			payload, decodeErr := decodeStoredReferenceBriefFact[agentcontract.ReferenceBriefPayload](invocation.Payload)
			if decodeErr != nil || payload.Validate() != nil || invocation.InputHash == "" ||
				invocation.StageInstanceKey == "" || invocation.ShardKey != payload.Shard.ShardKey ||
				invocation.ShardManifestHash != payload.Shard.ManifestHash {
				return referenceCoverageDrift("stored Reference Brief invocation has drifted")
			}
			input := payload.StageInput
			if !sameReferenceCoveragePlanInput(input, plan) {
				continue
			}
			target, exists := targets[input.TargetBusinessKey]
			if !exists || !sameReferenceCoverageTargetInput(input, target) {
				continue
			}
			if _, exists = seen[target.TargetBusinessKey]; exists {
				continue
			}
			if target.Fulfillment == "not_generated" || len(target.DependsOnTargetBusinessKeys) != 0 {
				return referenceCoverageDrift("Reference Brief execution exists before Target dependencies are ready")
			}
			execution := referenceapp.ReferenceBriefExecutionFact{
				TargetBusinessKey: target.TargetBusinessKey, InvocationID: invocation.ID.String(),
				InputHash: invocation.InputHash, Status: invocation.Status, UpdatedAt: invocation.UpdatedAt,
			}
			if invocation.Status == "accepted" {
				candidate, candidateErr := loadCurrentReferenceBriefCandidate(
					ctx, transaction, invocation, input,
				)
				if candidateErr != nil {
					return candidateErr
				}
				execution.Candidate = &candidate
			}
			facts.BriefExecutions = append(facts.BriefExecutions, execution)
			seen[target.TargetBusinessKey] = struct{}{}
		}
		return nil
	})
	return facts, err
}

func sameReferenceCoveragePlanInput(
	input agentcontract.ReferenceBriefInput,
	plan referencedomain.ApprovedReferencePlanVersion,
) bool {
	ref := input.ApprovedReferencePlanVersionRef
	return input.WorkspaceID == plan.WorkspaceID && input.ProjectID == plan.ProjectID &&
		ref.WorkspaceID == plan.WorkspaceID && ref.ProjectID == plan.ProjectID &&
		ref.OwnerKind == "production/reference" && ref.VersionFamily == "reference_plan_set" &&
		ref.OwnerLogicalID == plan.LogicalID && ref.OwnerVersionID == plan.ID &&
		ref.OwnerRevision == plan.Revision && ref.OwnerContentHash == plan.ContentHash
}

func sameReferenceCoverageTargetInput(
	input agentcontract.ReferenceBriefInput,
	target referencedomain.ReferencePlanTargetVersion,
) bool {
	ref := input.ReferencePlanTargetRef
	return input.WorkspaceID == target.WorkspaceID && input.ProjectID == target.ProjectID &&
		input.TargetBusinessKey == target.TargetBusinessKey && input.TargetKind == target.TargetKind &&
		ref.WorkspaceID == target.WorkspaceID && ref.ProjectID == target.ProjectID &&
		ref.OwnerKind == "production/reference" && ref.VersionFamily == "reference_plan_set" &&
		ref.OwnerLogicalID == target.TargetBusinessKey && ref.OwnerVersionID == target.ID &&
		ref.OwnerRevision == target.Revision && ref.OwnerContentHash == target.ContentHash
}

func loadCurrentReferenceBriefCandidate(
	ctx context.Context,
	database *gorm.DB,
	invocation model.SceneAnalysisInvocationRecord,
	input agentcontract.ReferenceBriefInput,
) (referenceapp.ReferenceBriefCandidateRef, error) {
	var head model.SceneAnalysisCandidateHead
	if err := database.WithContext(ctx).Where(
		"stage_instance_key = ? AND workspace_id = ? AND project_id = ?",
		invocation.StageInstanceKey, invocation.WorkspaceID, invocation.ProjectID,
	).First(&head).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return referenceapp.ReferenceBriefCandidateRef{}, referenceCoverageDrift("accepted Reference Brief Candidate Head is missing")
		}
		return referenceapp.ReferenceBriefCandidateRef{}, err
	}
	reader, err := agentgorm.NewReferenceBriefStore(database, ValidateCurrentReferenceBriefInput)
	if err != nil {
		return referenceapp.ReferenceBriefCandidateRef{}, err
	}
	candidate, err := reader.ReadAcceptedReferenceBrief(ctx, invocation.WorkspaceID.String(), invocation.ProjectID.String(), head.CurrentRevisionID.String(), head.CurrentCandidateRevisionHash)
	if err != nil || candidate.Candidate.ValidateFor(input) != nil {
		return referenceapp.ReferenceBriefCandidateRef{}, errors.Join(referenceCoverageDrift("accepted Reference Brief provenance has drifted"), err)
	}
	return referenceapp.ReferenceBriefCandidateRef{
		RevisionID: candidate.RevisionID, Revision: candidate.Revision,
		RevisionHash: candidate.RevisionHash, ContentHash: candidate.ContentHash,
	}, nil
}

func referenceCoverageDrift(message string) error {
	return fmt.Errorf("%s: %w", message, referenceapp.ErrReferenceCoverageDrift)
}

var _ referenceapp.ReferenceCoverageReader = (*Store)(nil)
