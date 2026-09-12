package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

func (store *ReferenceBriefStore) ReadAcceptedReferenceBrief(
	ctx context.Context, workspaceID, projectID, revisionID, revisionHash string,
) (agentapp.AcceptedReferenceBrief, error) {
	if store == nil || store.database == nil || store.validator == nil {
		return agentapp.AcceptedReferenceBrief{}, agentapp.ErrAcceptedReferenceBriefUnavailable
	}
	for _, id := range []string{workspaceID, projectID, revisionID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil {
			return agentapp.AcceptedReferenceBrief{}, agentapp.ErrAcceptedReferenceBriefUnavailable
		}
	}
	var accepted agentapp.AcceptedReferenceBrief
	err := platformdatabase.WithinTransaction(ctx, store.database, func(tx *gorm.DB) error {
		var candidate model.SceneAnalysisCandidateRevision
		if err := tx.WithContext(ctx).Where("id = ? AND workspace_id = ? AND project_id = ?", revisionID, workspaceID, projectID).First(&candidate).Error; err != nil {
			return err
		}
		computed, err := referenceBriefCandidateRevisionHash(candidate)
		if err != nil || computed != revisionHash || candidate.CandidateRevisionHash != revisionHash ||
			candidate.CandidateType != "reference_brief_candidate" || candidate.RevisionNo != 1 {
			return agentapp.ErrAcceptedReferenceBriefUnavailable
		}
		var invocation model.SceneAnalysisInvocationRecord
		if err = tx.WithContext(ctx).First(&invocation, "id = ?", candidate.SourceInvocationID).Error; err != nil {
			return err
		}
		var payload contract.ReferenceBriefPayload
		var budget contract.SceneAnalysisExecutionBudget
		if canonical.Decode(invocation.Payload, &payload) != nil || payload.Validate() != nil || canonical.Decode(invocation.Budget, &budget) != nil ||
			invocation.WorkspaceID != candidate.WorkspaceID || invocation.ProjectID != candidate.ProjectID ||
			payload.Scope.WorkspaceID != workspaceID || payload.Scope.ProjectID != projectID {
			return agentapp.ErrAcceptedReferenceBriefUnavailable
		}
		// Lock current source heads before Agent heads, matching dispatch/accept order.
		if err = store.validator(ctx, tx, payload.StageInput); err != nil {
			return err
		}
		var release model.SceneAnalysisRelease
		if err = tx.WithContext(ctx).First(&release, "id = ?", invocation.ReleaseID).Error; err != nil {
			return err
		}
		var control model.SceneAnalysisControlHead
		if err = tx.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&control, "release_id = ?", release.ID).Error; err != nil {
			return err
		}
		if release.StageKey != contract.ReferenceBriefStageKey || release.ProfileKey != "default" || release.ModelCapability != "structured_text" ||
			control.Status != "approved" || control.ControlRecordID != invocation.ControlRecordID ||
			control.ControlRevision != invocation.ControlRevision || control.ControlHash != invocation.ControlHash || control.ReleaseFence != invocation.ReleaseFence {
			return agentapp.ErrAcceptedReferenceBriefUnavailable
		}
		// Re-read the mutable invocation while holding its shared lock.
		if err = tx.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&invocation, "id = ?", candidate.SourceInvocationID).Error; err != nil {
			return err
		}
		var head model.SceneAnalysisCandidateHead
		if err = tx.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&head, "stage_instance_key = ?", candidate.StageInstanceKey).Error; err != nil {
			return err
		}
		if head.WorkspaceID != candidate.WorkspaceID || head.ProjectID != candidate.ProjectID || head.CurrentRevisionID != candidate.ID ||
			head.Revision != candidate.RevisionNo || head.CurrentCandidateRevisionHash != revisionHash ||
			invocation.Status != "accepted" || invocation.StageKey != contract.ReferenceBriefStageKey || invocation.ProfileKey != "default" ||
			invocation.StageInstanceKey != candidate.StageInstanceKey || invocation.SourceVersionID != nil ||
			invocation.SourceHash != payload.StageInput.TypedReadSetRoot || invocation.ShardKey != payload.Shard.ShardKey ||
			invocation.ShardManifestID.String() != payload.Shard.ManifestID || invocation.ShardManifestHash != payload.Shard.ManifestHash {
			return agentapp.ErrAcceptedReferenceBriefUnavailable
		}
		var result model.SceneAnalysisResult
		if err = tx.WithContext(ctx).First(&result, "id = ?", candidate.SourceResultID).Error; err != nil {
			return err
		}
		var attempt model.SceneAnalysisAttempt
		if err = tx.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&attempt, "id = ?", result.AttemptID).Error; err != nil {
			return err
		}
		if attempt.InvocationID != invocation.ID || attempt.Status != "completed" || attempt.CompletedAt == nil ||
			attempt.ControlHash != control.ControlHash || attempt.ReleaseFence != control.ReleaseFence || attempt.AgentImageDigest != release.AgentImageDigest {
			return agentapp.ErrAcceptedReferenceBriefUnavailable
		}
		wire, err := contract.NewReferenceBriefInvocation(invocation.ID.String(), attempt.ID.String(), contract.SceneAnalysisReleaseIdentity{
			SkillReleaseID: release.SkillReleaseID.String(), SkillReleaseHash: release.SkillReleaseHash, StageReleaseHash: release.StageReleaseHash,
			BundleContentHash: release.BundleContentHash, AgentImageDigest: release.AgentImageDigest,
		}, controlProof(control), budget, payload)
		if err != nil || wire.InputHash != invocation.InputHash || wire.StageInstanceKey() != invocation.StageInstanceKey || wire.WireSchemaVersion != invocation.WireSchemaID {
			return agentapp.ErrAcceptedReferenceBriefUnavailable
		}
		var authorization model.SceneAnalysisDispatchAuthorization
		if err = tx.WithContext(ctx).First(&authorization, "attempt_id = ?", attempt.ID).Error; err != nil {
			return err
		}
		decodedResult, err := contract.DecodeReferenceBriefAttemptResult(result.Result)
		if err != nil || decodedResult.ValidateFor(wire, attempt.ClaimVersion, authorization.AuthorizationHash) != nil ||
			decodedResult.Status != "accepted" || result.Status != "accepted" || result.InputHash != wire.InputHash ||
			result.OutputHash == nil || decodedResult.OutputHash == nil || *result.OutputHash != *decodedResult.OutputHash ||
			result.DiagnosticHash != decodedResult.DiagnosticHash || !result.CompletedAt.Equal(decodedResult.CompletedAt) ||
			candidate.SourceResultHash != decodedResult.ResultHash || candidate.CandidateContentHash != *result.OutputHash ||
			!sameCanonicalJSON(candidate.Candidate, decodedResult.Candidate) {
			return agentapp.ErrAcceptedReferenceBriefUnavailable
		}
		brief, _, err := contract.DecodeReferenceBriefCandidate(json.RawMessage(candidate.Candidate))
		if err != nil || brief.ValidateFor(payload.StageInput) != nil {
			return agentapp.ErrAcceptedReferenceBriefUnavailable
		}
		// Payload is immutable in normal execution; reject unexpected concurrent drift.
		var lockedPayload contract.ReferenceBriefPayload
		var lockedBudget contract.SceneAnalysisExecutionBudget
		if canonical.Decode(invocation.Payload, &lockedPayload) != nil || canonical.Decode(invocation.Budget, &lockedBudget) != nil ||
			!reflect.DeepEqual(payload, lockedPayload) || !reflect.DeepEqual(budget, lockedBudget) ||
			invocation.ControlRecordID != control.ControlRecordID || invocation.ControlHash != control.ControlHash ||
			invocation.ControlRevision != control.ControlRevision || invocation.ReleaseFence != control.ReleaseFence || invocation.ReleaseID != release.ID ||
			invocation.WorkspaceID != candidate.WorkspaceID || invocation.ProjectID != candidate.ProjectID {
			return agentapp.ErrAcceptedReferenceBriefUnavailable
		}
		accepted = agentapp.AcceptedReferenceBrief{RevisionID: candidate.ID.String(), Revision: candidate.RevisionNo, RevisionHash: revisionHash,
			ContentHash: candidate.CandidateContentHash, Input: payload.StageInput, Candidate: brief}
		return nil
	})
	if err != nil {
		return agentapp.AcceptedReferenceBrief{}, errors.Join(agentapp.ErrAcceptedReferenceBriefUnavailable, err)
	}
	return accepted, nil
}
