package gormdb

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func (repo *repository) CandidateConfirmation(
	ctx context.Context,
	actor application.Actor,
	command application.ConfirmCommand,
	lock bool,
) (application.CandidateConfirmation, error) {
	candidateID, err := uuid.Parse(command.CandidateRevisionID)
	if err != nil {
		return application.CandidateConfirmation{}, application.ErrNotFound
	}
	documentID, err := uuid.Parse(command.DocumentRevisionID)
	if err != nil {
		return application.CandidateConfirmation{}, application.ErrNotFound
	}
	reviewDecisionID, err := uuid.Parse(command.ReviewDecisionID)
	if err != nil {
		return application.CandidateConfirmation{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx)
	var candidate model.StageCandidateRevision
	if err = query.First(&candidate, "id = ?", candidateID).Error; err != nil {
		return application.CandidateConfirmation{}, normalizeNotFound(err)
	}
	if candidate.CandidateRevisionHash != command.CandidateRevisionHash ||
		candidate.RevisionNo != command.ExpectedCandidateRevision {
		return application.CandidateConfirmation{}, confirmationConflict("Production Bible Candidate Revision has changed")
	}
	if lock {
		var head model.StageCandidateHead
		if err = query.Clauses(clause.Locking{Strength: "UPDATE"}).First(
			&head, "stage_instance_key = ?", candidate.StageInstanceKey,
		).Error; err != nil {
			return application.CandidateConfirmation{}, normalizeNotFound(err)
		}
		if head.WorkspaceID != candidate.WorkspaceID || head.CurrentRevisionID != candidate.ID ||
			head.CurrentCandidateRevisionHash != candidate.CandidateRevisionHash || head.Revision != candidate.RevisionNo {
			return application.CandidateConfirmation{}, confirmationConflict("Production Bible Candidate Head has changed")
		}
	}

	root, err := confirmationCandidateRoot(query, candidate)
	if err != nil {
		return application.CandidateConfirmation{}, err
	}
	if root.SourceInvocationID == nil || root.SourceResultHash == nil {
		return application.CandidateConfirmation{}, confirmationConflict("Production Bible Candidate provenance is incomplete")
	}
	var rootInvocation model.AgentInvocation
	if err = query.First(&rootInvocation, "id = ?", *root.SourceInvocationID).Error; err != nil {
		return application.CandidateConfirmation{}, normalizeNotFound(err)
	}
	rootRequest, err := agentgorm.StageInvocation(rootInvocation)
	if err != nil || rootInvocation.Stage != domain.ReconcileStoryStage || rootInvocation.Status != "succeeded" ||
		rootInvocation.ResultHash == nil || *rootInvocation.ResultHash != *root.SourceResultHash ||
		len(rootRequest.Payload.SourceRefs) != 1 {
		return application.CandidateConfirmation{}, confirmationConflict("Production Bible Candidate provenance has changed")
	}
	source := rootRequest.Payload.SourceRefs[0]
	if source.OwnerKind != "production/script" || source.OwnerVersionID != command.DocumentRevisionID ||
		source.ContentHash != command.DocumentRevisionHash {
		return application.CandidateConfirmation{}, confirmationConflict("Production Bible source revision has changed")
	}
	var document model.DocumentRevision
	if err = query.First(&document, "id = ?", documentID).Error; err != nil {
		return application.CandidateConfirmation{}, normalizeNotFound(err)
	}
	var script model.ScriptDocument
	if err = query.First(&script, "id = ?", document.DocumentID).Error; err != nil {
		return application.CandidateConfirmation{}, normalizeNotFound(err)
	}
	if source.OwnerLogicalID != script.ID.String() {
		return application.CandidateConfirmation{}, confirmationConflict("Production Bible source document has changed")
	}
	if err = authorizeProject(ctx, query, actor, script.ProjectID, true); err != nil {
		return application.CandidateConfirmation{}, err
	}
	if candidate.WorkspaceID != document.WorkspaceID || document.WorkspaceID != rootInvocation.WorkspaceID ||
		document.NormalizedHash != command.DocumentRevisionHash || int64(document.VersionNo) != source.Revision {
		return application.CandidateConfirmation{}, confirmationConflict("Production Bible document revision has changed")
	}

	var decoded domain.StoryReconciliationCandidate
	if err = json.Unmarshal(candidate.Candidate, &decoded); err != nil ||
		domain.ValidateStoryReconciliationCandidate(decoded, domain.StoryReconciliationCandidateEvidence(decoded)) != nil {
		return application.CandidateConfirmation{}, confirmationConflict("Production Bible Candidate is invalid")
	}
	candidateContentHash, hashErr := agentcontract.CanonicalHash(json.RawMessage(candidate.Candidate))
	if hashErr != nil || candidateContentHash != candidate.CandidateContentHash {
		return application.CandidateConfirmation{}, confirmationConflict("Production Bible Candidate content has changed")
	}
	gate, gateErr := application.EvaluateStoryReconciliationGate(candidate.ID.String(), candidate.CandidateRevisionHash, decoded)
	if gateErr != nil || len(gate.Blockers) != 0 || hasBlockingStoryIssue(decoded) {
		return application.CandidateConfirmation{}, confirmationConflict("Production Bible Candidate still has blocking issues")
	}
	if err = requireCleanStoryReview(query, candidate, document); err != nil {
		return application.CandidateConfirmation{}, err
	}
	if err = requireApprovedBibleDecision(query, reviewDecisionID, candidate, script.ProjectID); err != nil {
		return application.CandidateConfirmation{}, err
	}

	if lock {
		var project model.Project
		if err = query.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, "id = ?", script.ProjectID).Error; err != nil {
			return application.CandidateConfirmation{}, normalizeNotFound(err)
		}
		if project.WorkspaceID != candidate.WorkspaceID {
			return application.CandidateConfirmation{}, confirmationConflict("Production Bible project has changed")
		}
	}
	nextVersion := 1
	var latest model.ProductionBibleVersion
	err = query.Where("project_id = ?", script.ProjectID).Order("version DESC").First(&latest).Error
	if err == nil {
		nextVersion = latest.Version + 1
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return application.CandidateConfirmation{}, err
	}
	return application.CandidateConfirmation{
		WorkspaceID: candidate.WorkspaceID.String(), ProjectID: script.ProjectID.String(),
		DocumentRevisionID: document.ID.String(), DocumentRevisionHash: document.NormalizedHash,
		CandidateRevisionID: candidate.ID.String(), CandidateRevisionHash: candidate.CandidateRevisionHash,
		CandidateContentHash: candidate.CandidateContentHash, CandidateRevisionNo: candidate.RevisionNo,
		Snapshot: append(json.RawMessage(nil), candidate.Candidate...), NextVersion: nextVersion,
	}, nil
}

func confirmationCandidateRoot(database *gorm.DB, candidate model.StageCandidateRevision) (model.StageCandidateRevision, error) {
	current := candidate
	for current.ParentCandidateRevisionID != nil {
		var parent model.StageCandidateRevision
		if err := database.First(&parent, "id = ?", *current.ParentCandidateRevisionID).Error; err != nil {
			return model.StageCandidateRevision{}, err
		}
		if current.ParentCandidateRevisionHash == nil || *current.ParentCandidateRevisionHash != parent.CandidateRevisionHash ||
			parent.WorkspaceID != candidate.WorkspaceID || parent.StageInstanceKey != candidate.StageInstanceKey ||
			parent.RevisionNo+1 != current.RevisionNo {
			return model.StageCandidateRevision{}, confirmationConflict("Production Bible Candidate lineage has changed")
		}
		current = parent
	}
	if current.OriginKind != "invocation" || current.RevisionNo != 1 {
		return model.StageCandidateRevision{}, confirmationConflict("Production Bible Candidate root is invalid")
	}
	return current, nil
}

func requireCleanStoryReview(
	database *gorm.DB,
	candidate model.StageCandidateRevision,
	document model.DocumentRevision,
) error {
	var invocations []model.AgentInvocation
	if err := database.Where(
		"workspace_id = ? AND stage = ? AND status = ?", candidate.WorkspaceID, domain.ReviewStoryGraphStage, "succeeded",
	).Order("completed_at DESC").Order("id DESC").Find(&invocations).Error; err != nil {
		return err
	}
	for _, invocation := range invocations {
		request, err := agentgorm.StageInvocation(invocation)
		if err != nil {
			continue
		}
		var input agentcontract.StoryGraphReviewStageInput
		if json.Unmarshal(request.Payload.StageInput, &input) != nil || input.Validate() != nil ||
			input.TargetCandidateRevisionID != candidate.ID.String() ||
			input.TargetCandidateRevisionHash != candidate.CandidateRevisionHash || len(input.DeterministicGate.Blockers) != 0 ||
			len(request.Payload.SourceRefs) != 1 || request.Payload.SourceRefs[0].OwnerVersionID != document.ID.String() ||
			request.Payload.SourceRefs[0].ContentHash != document.NormalizedHash {
			continue
		}
		review, err := agentcontract.DecodeStoryGraphReviewCandidate(invocation.Candidate)
		if err != nil || agentcontract.ValidateStoryGraphReviewCandidate(input, review) != nil {
			continue
		}
		blocking := false
		for _, issue := range review.ReviewIssues {
			blocking = blocking || issue.Severity == "blocking"
		}
		if blocking {
			return confirmationConflict("Production Bible Story review still has blocking issues")
		}
		var resultRevision model.StageCandidateRevision
		if err = database.Where("source_invocation_id = ?", invocation.ID).First(&resultRevision).Error; err != nil ||
			invocation.ResultHash == nil || resultRevision.SourceResultHash == nil ||
			*resultRevision.SourceResultHash != *invocation.ResultHash {
			return confirmationConflict("Production Bible Story review evidence has changed")
		}
		return nil
	}
	return confirmationConflict("Production Bible Candidate has no successful exact Story review")
}

func requireApprovedBibleDecision(
	database *gorm.DB,
	decisionID uuid.UUID,
	candidate model.StageCandidateRevision,
	projectID uuid.UUID,
) error {
	var decision model.ReviewDecision
	if err := database.First(&decision, "id = ?", decisionID).Error; err != nil {
		return normalizeNotFound(err)
	}
	var task model.HumanTask
	if err := database.First(&task, "id = ?", decision.HumanTaskID).Error; err != nil {
		return normalizeNotFound(err)
	}
	if decision.Decision != "approved" || task.Status != "COMPLETED" || task.SubjectType != "story_reconciliation_candidate" ||
		task.SubjectID != candidate.ID || task.SubjectHash != candidate.CandidateRevisionHash ||
		task.WorkspaceID != candidate.WorkspaceID || task.ProjectID != projectID || decision.WorkspaceID != task.WorkspaceID ||
		decision.SubjectRevision != task.SubjectRevision || decision.SubjectHash != task.SubjectHash {
		return confirmationConflict("Production Bible ReviewDecision does not match the Candidate")
	}
	var candidateIDs []string
	if json.Unmarshal(task.CandidateIDs, &candidateIDs) != nil || len(candidateIDs) != 1 || candidateIDs[0] != candidate.ID.String() {
		return confirmationConflict("Production Bible HumanTask Candidate binding has changed")
	}
	return nil
}

func hasBlockingStoryIssue(candidate domain.StoryReconciliationCandidate) bool {
	for _, issue := range append(append([]domain.ReviewIssue(nil), candidate.Conflicts...), candidate.ReviewIssues...) {
		if issue.Severity == "blocking" {
			return true
		}
	}
	return false
}

func (repo *repository) GetBibleVersion(
	ctx context.Context,
	actor application.Actor,
	versionID string,
) (domain.ProductionBibleVersion, error) {
	id, err := uuid.Parse(versionID)
	if err != nil {
		return domain.ProductionBibleVersion{}, application.ErrNotFound
	}
	var record model.ProductionBibleVersion
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return domain.ProductionBibleVersion{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, false); err != nil {
		return domain.ProductionBibleVersion{}, err
	}
	return productionBibleVersionDomain(record)
}

func (repo *repository) CreateBibleVersion(ctx context.Context, value domain.ProductionBibleVersion) error {
	record, err := productionBibleVersionRecord(value)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return confirmationConflict("Production Bible Version already exists")
		}
		return err
	}
	return nil
}

func productionBibleVersionRecord(value domain.ProductionBibleVersion) (model.ProductionBibleVersion, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.ProductionBibleVersion{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.ProductionBibleVersion{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.ProductionBibleVersion{}, err
	}
	documentID, err := uuid.Parse(value.DocumentRevisionID)
	if err != nil {
		return model.ProductionBibleVersion{}, err
	}
	candidateID, err := uuid.Parse(value.CandidateRevisionID)
	if err != nil {
		return model.ProductionBibleVersion{}, err
	}
	decisionID, err := uuid.Parse(value.ReviewDecisionID)
	if err != nil {
		return model.ProductionBibleVersion{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.ProductionBibleVersion{}, err
	}
	return model.ProductionBibleVersion{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, DocumentRevisionID: documentID,
		DocumentRevisionHash: value.DocumentRevisionHash, CandidateRevisionID: candidateID,
		CandidateRevisionNo: value.CandidateRevisionNo, CandidateRevisionHash: value.CandidateRevisionHash,
		CandidateContentHash: value.CandidateContentHash, Version: value.Version, ReviewDecisionID: decisionID,
		Snapshot: datatypes.JSON(value.Snapshot), ContentHash: value.ContentHash,
		CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}

func productionBibleVersionDomain(record model.ProductionBibleVersion) (domain.ProductionBibleVersion, error) {
	value, err := domain.NewProductionBibleVersion(domain.ProductionBibleVersionInput{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		DocumentRevisionID: record.DocumentRevisionID.String(), DocumentRevisionHash: record.DocumentRevisionHash,
		CandidateRevisionID: record.CandidateRevisionID.String(), CandidateRevisionNo: record.CandidateRevisionNo,
		CandidateRevisionHash: record.CandidateRevisionHash, CandidateContentHash: record.CandidateContentHash,
		Version: record.Version, ReviewDecisionID: record.ReviewDecisionID.String(),
		Snapshot: append(json.RawMessage(nil), record.Snapshot...), CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
	})
	if err != nil || value.ContentHash != record.ContentHash {
		return domain.ProductionBibleVersion{}, errors.New("persisted Production Bible Version has drifted")
	}
	return value, nil
}

func confirmationConflict(message string) error {
	return &application.Error{Code: "resource_conflict", Message: message, Status: 409}
}
