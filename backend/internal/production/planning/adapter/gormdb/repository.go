package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

type Store struct{ database *gorm.DB }
type repository struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }
func (store *Store) WithinTransaction(ctx context.Context, operation func(application.Repository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error { return operation(&repository{database: transaction}) })
}

func (repo *repository) RevisionSource(ctx context.Context, actor application.Actor, revisionID string, write bool) (domain.Source, string, string, error) {
	id, err := uuid.Parse(revisionID)
	if err != nil {
		return domain.Source{}, "", "", application.ErrNotFound
	}
	var revision model.DocumentRevision
	if err = repo.database.WithContext(ctx).First(&revision, "id = ?", id).Error; err != nil {
		return domain.Source{}, "", "", normalizeNotFound(err)
	}
	var document model.ScriptDocument
	if err = repo.database.WithContext(ctx).First(&document, "id = ?", revision.DocumentID).Error; err != nil {
		return domain.Source{}, "", "", normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, document.ProjectID, write); err != nil {
		return domain.Source{}, "", "", err
	}
	source, err := sourceDomain(revision)
	return source, revision.WorkspaceID.String(), document.ProjectID.String(), err
}

func (repo *repository) FindReceipt(ctx context.Context, workspaceID, operation, key string) (platformcommand.Receipt, error) {
	id, err := uuid.Parse(workspaceID)
	if err != nil {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	var record model.CommandReceipt
	if err = repo.database.WithContext(ctx).Where("workspace_id = ? AND operation = ? AND idempotency_key = ?", id, operation, key).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
		}
		return platformcommand.Receipt{}, err
	}
	return platformcommand.Receipt{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), Operation: record.Operation, IdempotencyKey: record.IdempotencyKey, InputHash: record.InputHash, ResourceID: record.ResourceID.String(), Result: append([]byte(nil), record.Result...), CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt}, nil
}
func (repo *repository) CreateReceipt(ctx context.Context, value platformcommand.Receipt) error {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return err
	}
	resourceID, err := uuid.Parse(value.ResourceID)
	if err != nil {
		return err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return err
	}
	record := model.CommandReceipt{ID: id, WorkspaceID: workspaceID, Operation: value.Operation, IdempotencyKey: value.IdempotencyKey, InputHash: value.InputHash, ResourceID: resourceID, Result: datatypes.JSON(value.Result), CreatedBy: createdBy, CreatedAt: value.CreatedAt}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return &application.Error{Code: "resource_conflict", Message: "Idempotency key is already in use", Status: 409}
		}
		return err
	}
	return nil
}

func (repo *repository) CreatePlan(ctx context.Context, value domain.Plan) error {
	record, err := planRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}
func (repo *repository) GetPlan(ctx context.Context, actor application.Actor, planID string, forUpdate bool) (domain.Plan, error) {
	id, err := uuid.Parse(planID)
	if err != nil {
		return domain.Plan{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", id)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.EpisodePlan
	if err = query.First(&record).Error; err != nil {
		return domain.Plan{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, forUpdate); err != nil {
		return domain.Plan{}, err
	}
	var revision model.DocumentRevision
	if err = repo.database.WithContext(ctx).First(&revision, "id = ?", record.DocumentRevisionID).Error; err != nil {
		return domain.Plan{}, normalizeNotFound(err)
	}
	source, err := sourceDomain(revision)
	if err != nil {
		return domain.Plan{}, err
	}
	return planDomain(record, source)
}
func (repo *repository) SavePlan(ctx context.Context, value domain.Plan) error {
	record, err := planRecord(value)
	if err != nil {
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.EpisodePlan{}).Where("id = ?", record.ID).Updates(map[string]any{"status": record.Status, "proposals": record.Proposals, "revision": record.Revision, "confirmed_by": record.ConfirmedBy, "confirmed_at": record.ConfirmedAt, "updated_at": record.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (repo *repository) ProjectImpact(ctx context.Context, actor application.Actor, projectID string, write bool) (domain.Impact, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return domain.Impact{}, application.ErrNotFound
	}
	if err = authorizeProject(ctx, repo.database, actor, id, write); err != nil {
		return domain.Impact{}, err
	}
	var project model.Project
	if err = repo.database.WithContext(ctx).First(&project, "id = ?", id).Error; err != nil {
		return domain.Impact{}, normalizeNotFound(err)
	}
	var records []model.Episode
	if err = repo.database.WithContext(ctx).Where("project_id = ? AND status = ?", id, "active").Order("position").Order("id").Find(&records).Error; err != nil {
		return domain.Impact{}, err
	}
	order := make([]struct {
		ID       string
		Position int
	}, len(records))
	for index, item := range records {
		order[index] = struct {
			ID       string
			Position int
		}{item.ID.String(), item.Position}
	}
	hash, err := platformcommand.InputHash(order)
	if err != nil {
		return domain.Impact{}, err
	}
	return domain.Impact{ProjectRevision: project.Revision, ActiveEpisodeCount: len(records), ProjectedEpisodeCount: len(records), ActiveOrderHash: hash, Allowed: true, Blockers: []domain.Blocker{}}, nil
}
func (repo *repository) HasConfirmedBible(ctx context.Context, projectID, revisionID string) (bool, error) {
	project, err := uuid.Parse(projectID)
	if err != nil {
		return false, nil
	}
	revision, err := uuid.Parse(revisionID)
	if err != nil {
		return false, nil
	}
	var count int64
	err = repo.database.WithContext(ctx).Model(&model.ProductionBible{}).Where("project_id = ? AND document_revision_id = ? AND status = ?", project, revision, "confirmed").Count(&count).Error
	return count > 0, err
}

func (repo *repository) GetEpisodeSegmentationSource(
	ctx context.Context,
	actor application.Actor,
	candidateRevisionID string,
	forUpdate bool,
) (application.EpisodeSegmentationSource, error) {
	candidateID, err := uuid.Parse(candidateRevisionID)
	if err != nil {
		return application.EpisodeSegmentationSource{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", candidateID)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var candidate model.StageCandidateRevision
	if err = query.First(&candidate).Error; err != nil {
		return application.EpisodeSegmentationSource{}, normalizeNotFound(err)
	}
	var head model.StageCandidateHead
	headQuery := repo.database.WithContext(ctx).Where("workspace_id = ? AND stage_instance_key = ?", candidate.WorkspaceID, candidate.StageInstanceKey)
	if forUpdate {
		headQuery = headQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err = headQuery.First(&head).Error; err != nil {
		return application.EpisodeSegmentationSource{}, normalizeNotFound(err)
	}
	if head.CurrentRevisionID != candidate.ID || head.CurrentCandidateRevisionHash != candidate.CandidateRevisionHash ||
		head.Revision != candidate.RevisionNo || candidate.OriginKind != "invocation" || candidate.SourceInvocationID == nil ||
		candidate.SourceResultHash == nil || *candidate.SourceResultHash != candidate.CandidateContentHash {
		return application.EpisodeSegmentationSource{}, &application.Error{
			Code: "resource_conflict", Message: "Episode segmentation Candidate head changed before approval", Status: 409,
		}
	}
	contentHash, err := agentcontract.CanonicalHash(json.RawMessage(candidate.Candidate))
	if err != nil || contentHash != candidate.CandidateContentHash {
		return application.EpisodeSegmentationSource{}, errors.New("Episode segmentation Candidate content hash has drifted")
	}
	var invocation model.AgentInvocation
	if err = repo.database.WithContext(ctx).First(&invocation, "id = ?", *candidate.SourceInvocationID).Error; err != nil {
		return application.EpisodeSegmentationSource{}, normalizeNotFound(err)
	}
	request, err := episodeSegmentationInvocation(invocation)
	if err != nil ||
		invocation.WorkspaceID != candidate.WorkspaceID || invocation.Stage != bibledomain.EpisodeSegmentationStage ||
		invocation.Status != "succeeded" || invocation.ResultHash == nil || *invocation.ResultHash != candidate.CandidateContentHash ||
		invocation.CandidateType == nil || *invocation.CandidateType != "episode_segmentation_candidate" {
		return application.EpisodeSegmentationSource{}, errors.New("Episode segmentation Candidate invocation has drifted")
	}
	if persistedHash, hashErr := agentcontract.CanonicalHash(json.RawMessage(invocation.Candidate)); hashErr != nil || persistedHash != candidate.CandidateContentHash {
		return application.EpisodeSegmentationSource{}, errors.New("Episode segmentation Candidate invocation result has drifted")
	}
	projectID, err := uuid.Parse(request.Payload.ProjectID)
	if err != nil || request.Payload.WorkspaceID != candidate.WorkspaceID.String() {
		return application.EpisodeSegmentationSource{}, errors.New("Episode segmentation Candidate owner has drifted")
	}
	if err = authorizeProject(ctx, repo.database, actor, projectID, forUpdate); err != nil {
		return application.EpisodeSegmentationSource{}, err
	}
	var input agentcontract.EpisodeSegmentationStageInput
	if err = json.Unmarshal(request.Payload.StageInput, &input); err != nil {
		return application.EpisodeSegmentationSource{}, err
	}
	documentRevisionID, err := uuid.Parse(input.DocumentRevisionID)
	if err != nil {
		return application.EpisodeSegmentationSource{}, errors.New("Episode segmentation source revision is invalid")
	}
	var revision model.DocumentRevision
	if err = repo.database.WithContext(ctx).First(&revision, "id = ?", documentRevisionID).Error; err != nil {
		return application.EpisodeSegmentationSource{}, normalizeNotFound(err)
	}
	if revision.WorkspaceID != candidate.WorkspaceID || revision.NormalizedHash != input.NormalizedHash ||
		revision.CodepointCount != input.SourceCodePoints || revision.CodepointCount != len([]rune(revision.NormalizedText)) {
		return application.EpisodeSegmentationSource{}, errors.New("Episode segmentation source revision has drifted")
	}
	allowed := make([]bibledomain.Evidence, 0, len(input.EvidenceIndex)+len(input.MarkerHints))
	for _, item := range input.EvidenceIndex {
		allowed = append(allowed, episodeSegmentationEvidence(item.Evidence))
	}
	markers := make([]bibledomain.EpisodeSegmentationMarker, 0, len(input.MarkerHints))
	for _, marker := range input.MarkerHints {
		evidence := episodeSegmentationEvidence(marker.Evidence)
		allowed = append(allowed, evidence)
		markers = append(markers, bibledomain.EpisodeSegmentationMarker{
			EpisodeNumber: marker.EpisodeNumber, Label: marker.Label, Evidence: evidence,
		})
	}
	decoded, err := bibledomain.DecodeEpisodeSegmentationCandidate(
		json.RawMessage(candidate.Candidate), revision.NormalizedText, allowed, markers,
	)
	if err != nil {
		return application.EpisodeSegmentationSource{}, fmt.Errorf("invalid persisted Episode segmentation Candidate: %w", err)
	}
	return application.EpisodeSegmentationSource{
		CandidateRevisionID: candidate.ID.String(), CandidateRevisionHash: candidate.CandidateRevisionHash,
		CandidateRevision: candidate.RevisionNo, WorkspaceID: candidate.WorkspaceID.String(), ProjectID: projectID.String(),
		DocumentRevisionID: revision.ID.String(), DocumentRevisionHash: revision.NormalizedHash,
		NormalizedText: revision.NormalizedText, TargetDurationMS: input.TargetDurationMS,
		AllowedEvidence: allowed, Markers: markers, Candidate: decoded,
	}, nil
}

func (repo *repository) GetEpisodeSet(
	ctx context.Context,
	actor application.Actor,
	reference application.EpisodeSetReference,
) ([]domain.Episode, []application.Version, error) {
	projectID, err := uuid.Parse(reference.ProjectID)
	if err != nil {
		return nil, nil, application.ErrNotFound
	}
	if err = authorizeProject(ctx, repo.database, actor, projectID, false); err != nil {
		return nil, nil, err
	}
	episodeIDs := make([]uuid.UUID, len(reference.Episodes))
	versionIDs := make([]uuid.UUID, len(reference.Episodes))
	for index, item := range reference.Episodes {
		if episodeIDs[index], err = uuid.Parse(item.EpisodeID); err != nil {
			return nil, nil, application.ErrNotFound
		}
		if versionIDs[index], err = uuid.Parse(item.ScriptVersionID); err != nil {
			return nil, nil, application.ErrNotFound
		}
	}
	var episodeRecords []model.Episode
	if err = repo.database.WithContext(ctx).Where("project_id = ? AND id IN ?", projectID, episodeIDs).Find(&episodeRecords).Error; err != nil {
		return nil, nil, err
	}
	var versionRecords []model.EpisodeScriptVersion
	if err = repo.database.WithContext(ctx).Where("project_id = ? AND id IN ?", projectID, versionIDs).Find(&versionRecords).Error; err != nil {
		return nil, nil, err
	}
	episodesByID := make(map[string]domain.Episode, len(episodeRecords))
	for _, record := range episodeRecords {
		episodesByID[record.ID.String()] = episodeDomain(record)
	}
	versionsByID := make(map[string]application.Version, len(versionRecords))
	for _, record := range versionRecords {
		versionsByID[record.ID.String()] = versionDomain(record)
	}
	episodes := make([]domain.Episode, len(reference.Episodes))
	versions := make([]application.Version, len(reference.Episodes))
	for index, item := range reference.Episodes {
		var exists bool
		if episodes[index], exists = episodesByID[item.EpisodeID]; !exists {
			return nil, nil, application.ErrNotFound
		}
		if versions[index], exists = versionsByID[item.ScriptVersionID]; !exists {
			return nil, nil, application.ErrNotFound
		}
	}
	return episodes, versions, nil
}

func (repo *repository) ApplyEpisodeSet(
	ctx context.Context,
	episodes []domain.Episode,
	versions []application.Version,
	events []domain.OutboxEvent,
) error {
	if len(episodes) == 0 || len(episodes) != len(versions) || len(episodes) != len(events) {
		return errors.New("Episode set batch is incomplete")
	}
	projectID, err := uuid.Parse(episodes[0].ProjectID)
	if err != nil {
		return err
	}
	var project model.Project
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, "id = ?", projectID).Error; err != nil {
		return normalizeNotFound(err)
	}
	var existing int64
	if err = repo.database.WithContext(ctx).Model(&model.Episode{}).Where("project_id = ?", projectID).Count(&existing).Error; err != nil {
		return err
	}
	if existing != 0 {
		return &application.Error{Code: "resource_conflict", Message: "Project Episode boundaries changed before approval", Status: 409}
	}
	episodeRecords := make([]model.Episode, len(episodes))
	for index, value := range episodes {
		if value.ProjectID != episodes[0].ProjectID || value.WorkspaceID != episodes[0].WorkspaceID {
			return errors.New("Episode set owner changed within the batch")
		}
		episodeRecords[index], err = episodeRecord(value)
		if err != nil {
			return err
		}
	}
	versionRecords := make([]model.EpisodeScriptVersion, len(versions))
	for index, value := range versions {
		versionRecords[index], err = versionRecord(value)
		if err != nil {
			return err
		}
	}
	eventRecords := make([]model.OutboxEvent, len(events))
	for index, value := range events {
		eventRecords[index], err = outboxRecord(value)
		if err != nil {
			return err
		}
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&episodeRecords).Error; err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&versionRecords).Error; err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&eventRecords).Error; err != nil {
		return err
	}
	updated := repo.database.WithContext(ctx).Model(&model.Project{}).Where("id = ? AND revision = ?", project.ID, project.Revision).
		Updates(map[string]any{"revision": project.Revision + 1, "updated_at": episodes[0].UpdatedAt})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return &application.Error{Code: "resource_conflict", Message: "Project changed before Episode Plan approval", Status: 409}
	}
	return nil
}

func episodeSegmentationEvidence(value agentcontract.EpisodeSegmentationEvidence) bibledomain.Evidence {
	return bibledomain.Evidence{
		SourceStart: value.SourceStart, SourceEnd: value.SourceEnd, TextHash: value.TextHash,
		ExactAnchor: value.ExactAnchor, EpisodeNumber: value.EpisodeNumber,
	}
}

func episodeSegmentationInvocation(record model.AgentInvocation) (agentcontract.StageInvocation, error) {
	var policy agentcontract.StageExecutionPolicy
	if err := json.Unmarshal(record.ExecutionPolicy, &policy); err != nil {
		return agentcontract.StageInvocation{}, err
	}
	var payload agentcontract.StageInvocationPayload
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return agentcontract.StageInvocation{}, err
	}
	value := agentcontract.StageInvocation{
		InvocationID: record.ID.String(), Kind: record.Kind, WireSchemaVersion: record.WireSchemaVersion,
		InputHash: record.InputHash, ExecutionPolicy: policy, Payload: payload,
	}
	if err := agentcontract.ValidateEpisodeSegmentationInvocation(value); err != nil {
		return agentcontract.StageInvocation{}, err
	}
	return value, nil
}

func (repo *repository) Materialize(ctx context.Context, plan domain.Plan, commit domain.ImportCommit, episodes []domain.Episode, versions []application.Version) error {
	planRecord, err := planRecord(plan)
	if err != nil {
		return err
	}
	commitRecord, err := commitRecord(commit)
	if err != nil {
		return err
	}
	episodeRecords := make([]model.Episode, len(episodes))
	for index, value := range episodes {
		episodeRecords[index], err = episodeRecord(value)
		if err != nil {
			return err
		}
	}
	versionRecords := make([]model.EpisodeScriptVersion, len(versions))
	for index, value := range versions {
		versionRecords[index], err = versionRecord(value)
		if err != nil {
			return err
		}
	}
	if len(episodeRecords) > 0 {
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&episodeRecords).Error; err != nil {
			return err
		}
	}
	if len(versionRecords) > 0 {
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&versionRecords).Error; err != nil {
			return err
		}
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&commitRecord).Error; err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Model(&model.EpisodePlan{}).Where("id = ?", planRecord.ID).Updates(map[string]any{"status": planRecord.Status, "revision": planRecord.Revision, "updated_at": planRecord.UpdatedAt}).Error; err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Model(&model.Project{}).Where("id = ?", planRecord.ProjectID).Updates(map[string]any{"revision": gorm.Expr("revision + 1"), "updated_at": planRecord.UpdatedAt}).Error
}

func (repo *repository) GetCommit(ctx context.Context, actor application.Actor, commitID string, forUpdate bool) (domain.ImportCommit, error) {
	id, err := uuid.Parse(commitID)
	if err != nil {
		return domain.ImportCommit{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", id)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.ImportCommit
	if err = query.First(&record).Error; err != nil {
		return domain.ImportCommit{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, forUpdate); err != nil {
		return domain.ImportCommit{}, err
	}
	return commitDomain(record)
}

func (repo *repository) GetPlanCommit(ctx context.Context, actor application.Actor, planID string) (domain.ImportCommit, error) {
	id, err := uuid.Parse(planID)
	if err != nil {
		return domain.ImportCommit{}, application.ErrNotFound
	}
	var record model.ImportCommit
	if err = repo.database.WithContext(ctx).Where("plan_id = ?", id).First(&record).Error; err != nil {
		return domain.ImportCommit{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, false); err != nil {
		return domain.ImportCommit{}, err
	}
	return commitDomain(record)
}

func (repo *repository) Publish(ctx context.Context, commit domain.ImportCommit, structures []domain.Structure, events []domain.OutboxEvent) error {
	commitRecord, err := commitRecord(commit)
	if err != nil {
		return err
	}
	for _, segment := range commit.Segments {
		versionID, parseErr := uuid.Parse(segment.DraftVersionID)
		if parseErr != nil {
			return parseErr
		}
		episodeID, parseErr := uuid.Parse(segment.EpisodeID)
		if parseErr != nil {
			return parseErr
		}
		if err = repo.database.WithContext(ctx).Model(&model.EpisodeScriptVersion{}).Where("id = ? AND episode_id = ?", versionID, episodeID).Updates(map[string]any{"status": "published", "updated_at": commit.UpdatedAt}).Error; err != nil {
			return err
		}
		if err = repo.database.WithContext(ctx).Model(&model.Episode{}).Where("id = ?", episodeID).Updates(map[string]any{"current_script_version_id": versionID, "revision": gorm.Expr("revision + 1"), "updated_at": commit.UpdatedAt}).Error; err != nil {
			return err
		}
	}
	structureRecords := make([]model.EpisodeStructure, len(structures))
	for index, value := range structures {
		structureRecords[index], err = structureRecord(value)
		if err != nil {
			return err
		}
	}
	if len(structureRecords) > 0 {
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&structureRecords).Error; err != nil {
			return err
		}
	}
	eventRecords := make([]model.OutboxEvent, len(events))
	for index, value := range events {
		eventRecords[index], err = outboxRecord(value)
		if err != nil {
			return err
		}
	}
	if len(eventRecords) > 0 {
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&eventRecords).Error; err != nil {
			return err
		}
	}
	return repo.database.WithContext(ctx).Model(&model.ImportCommit{}).Where("id = ?", commitRecord.ID).Updates(map[string]any{"status": commitRecord.Status, "segments": commitRecord.Segments, "revision": commitRecord.Revision, "updated_at": commitRecord.UpdatedAt}).Error
}

func (repo *repository) ListEpisodes(ctx context.Context, actor application.Actor, projectID string) ([]domain.Episode, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return nil, application.ErrNotFound
	}
	if err = authorizeProject(ctx, repo.database, actor, id, false); err != nil {
		return nil, err
	}
	var records []model.Episode
	if err = repo.database.WithContext(ctx).Where("project_id = ?", id).Order("position").Order("id").Find(&records).Error; err != nil {
		return nil, err
	}
	values := make([]domain.Episode, len(records))
	for index, record := range records {
		values[index] = episodeDomain(record)
	}
	return values, nil
}
func (repo *repository) GetEpisode(ctx context.Context, actor application.Actor, episodeID string, forUpdate bool) (domain.Episode, error) {
	id, err := uuid.Parse(episodeID)
	if err != nil {
		return domain.Episode{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", id)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.Episode
	if err = query.First(&record).Error; err != nil {
		return domain.Episode{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, forUpdate); err != nil {
		return domain.Episode{}, err
	}
	return episodeDomain(record), nil
}

func (repo *repository) GetStructure(ctx context.Context, actor application.Actor, structureID string, forUpdate bool) (domain.Structure, error) {
	id, err := uuid.Parse(structureID)
	if err != nil {
		return domain.Structure{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", id)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.EpisodeStructure
	if err = query.First(&record).Error; err != nil {
		return domain.Structure{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, forUpdate); err != nil {
		return domain.Structure{}, err
	}
	return structureDomain(record)
}
func (repo *repository) GetEpisodeStructure(ctx context.Context, actor application.Actor, episodeID string) (domain.Structure, error) {
	episode, err := repo.GetEpisode(ctx, actor, episodeID, false)
	if err != nil {
		return domain.Structure{}, err
	}
	id, _ := uuid.Parse(episode.ID)
	var record model.EpisodeStructure
	if err = repo.database.WithContext(ctx).Where("episode_id = ? AND status <> ?", id, "superseded").Order("created_at DESC").First(&record).Error; err != nil {
		return domain.Structure{}, normalizeNotFound(err)
	}
	return structureDomain(record)
}
func (repo *repository) SaveStructure(ctx context.Context, value domain.Structure) error {
	record, err := structureRecord(value)
	if err != nil {
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.EpisodeStructure{}).Where("id = ?", record.ID).Updates(map[string]any{"status": record.Status, "scenes": record.Scenes, "revision": record.Revision, "confirmed_by": record.ConfirmedBy, "confirmed_at": record.ConfirmedAt, "updated_at": record.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return application.ErrNotFound
	}
	return nil
}

func authorizeProject(ctx context.Context, database *gorm.DB, actor application.Actor, projectID uuid.UUID, write bool) error {
	userID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return unauthenticated()
	}
	var user model.UserAccount
	if err = database.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil || user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return unauthenticated()
	}
	var project model.Project
	if err = database.WithContext(ctx).First(&project, "id = ?", projectID).Error; err != nil {
		return application.ErrNotFound
	}
	var workspace model.Workspace
	if err = database.WithContext(ctx).First(&workspace, "id = ?", project.WorkspaceID).Error; err != nil {
		return application.ErrNotFound
	}
	var membership model.Membership
	if err = database.WithContext(ctx).Where("workspace_id = ? AND user_id = ? AND status = ?", project.WorkspaceID, userID, "active").First(&membership).Error; err != nil {
		return application.ErrNotFound
	}
	if write && (membership.Role == "viewer" || workspace.Status != "active" || project.Status != "active") {
		return &application.Error{Code: "forbidden", Message: "Action is not allowed", Status: 403}
	}
	return nil
}

func sourceDomain(record model.DocumentRevision) (domain.Source, error) {
	var blocks []domain.Block
	if err := json.Unmarshal(record.Blocks, &blocks); err != nil {
		return domain.Source{}, err
	}
	if blocks == nil {
		blocks = []domain.Block{}
	}
	return domain.Source{DocumentRevisionID: record.ID.String(), NormalizedText: record.NormalizedText, NormalizedHash: record.NormalizedHash, CodepointCount: record.CodepointCount, Blocks: blocks}, nil
}
func planRecord(value domain.Plan) (model.EpisodePlan, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.EpisodePlan{}, err
	}
	workspace, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.EpisodePlan{}, err
	}
	project, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.EpisodePlan{}, err
	}
	revision, err := uuid.Parse(value.DocumentRevisionID)
	if err != nil {
		return model.EpisodePlan{}, err
	}
	creator, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.EpisodePlan{}, err
	}
	var confirmer *uuid.UUID
	if value.ConfirmedBy != nil {
		parsed, e := uuid.Parse(*value.ConfirmedBy)
		if e != nil {
			return model.EpisodePlan{}, e
		}
		confirmer = &parsed
	}
	proposals, err := json.Marshal(value.Proposals)
	if err != nil {
		return model.EpisodePlan{}, err
	}
	return model.EpisodePlan{ID: id, WorkspaceID: workspace, ProjectID: project, DocumentRevisionID: revision, Strategy: value.Strategy, Status: value.Status, TargetDurationMS: value.TargetDurationMS, RequestedEpisodeCount: value.RequestedEpisodeCount, TotalDurationMS: value.TotalEstimatedDurationMS, InputHash: value.InputHash, EngineVersion: value.EngineVersion, ModelName: value.ModelName, PromptVersion: value.PromptVersion, SchemaVersion: value.SchemaVersion, PlanningErrorCode: value.PlanningErrorCode, Proposals: datatypes.JSON(proposals), Revision: value.Revision, ConfirmedBy: confirmer, ConfirmedAt: value.ConfirmedAt, CreatedBy: creator, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}
func planDomain(record model.EpisodePlan, source domain.Source) (domain.Plan, error) {
	var proposals []domain.Proposal
	if err := json.Unmarshal(record.Proposals, &proposals); err != nil {
		return domain.Plan{}, err
	}
	if proposals == nil {
		proposals = []domain.Proposal{}
	}
	var confirmedBy *string
	if record.ConfirmedBy != nil {
		value := record.ConfirmedBy.String()
		confirmedBy = &value
	}
	return domain.Plan{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(), DocumentRevisionID: record.DocumentRevisionID.String(), Strategy: record.Strategy, Status: record.Status, TargetDurationMS: record.TargetDurationMS, RequestedEpisodeCount: record.RequestedEpisodeCount, TotalEstimatedDurationMS: record.TotalDurationMS, InputHash: record.InputHash, EngineVersion: record.EngineVersion, ModelName: record.ModelName, PromptVersion: record.PromptVersion, SchemaVersion: record.SchemaVersion, PlanningErrorCode: record.PlanningErrorCode, Revision: record.Revision, ConfirmedBy: confirmedBy, ConfirmedAt: record.ConfirmedAt, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, Proposals: proposals, Source: source}, nil
}
func episodeRecord(value domain.Episode) (model.Episode, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.Episode{}, err
	}
	workspace, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.Episode{}, err
	}
	project, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.Episode{}, err
	}
	var currentScriptVersionID *uuid.UUID
	if value.CurrentScriptVersionID != nil {
		parsed, parseErr := uuid.Parse(*value.CurrentScriptVersionID)
		if parseErr != nil {
			return model.Episode{}, parseErr
		}
		currentScriptVersionID = &parsed
	}
	var currentTimelineVersionID *uuid.UUID
	if value.CurrentTimelineVersionID != nil {
		parsed, parseErr := uuid.Parse(*value.CurrentTimelineVersionID)
		if parseErr != nil {
			return model.Episode{}, parseErr
		}
		currentTimelineVersionID = &parsed
	}
	return model.Episode{
		ID: id, WorkspaceID: workspace, ProjectID: project, Name: value.Name,
		Position: value.Position, TargetDurationMS: value.TargetDurationMS, Status: value.Status,
		Revision: value.Revision, CurrentScriptVersionID: currentScriptVersionID,
		CurrentTimelineVersionID: currentTimelineVersionID, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}
func episodeDomain(record model.Episode) domain.Episode {
	var current *string
	if record.CurrentScriptVersionID != nil {
		value := record.CurrentScriptVersionID.String()
		current = &value
	}
	var timeline *string
	if record.CurrentTimelineVersionID != nil {
		value := record.CurrentTimelineVersionID.String()
		timeline = &value
	}
	return domain.Episode{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(), Name: record.Name, Position: record.Position, TargetDurationMS: record.TargetDurationMS, Status: record.Status, Revision: record.Revision, CurrentScriptVersionID: current, CurrentTimelineVersionID: timeline, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}
func versionRecord(value application.Version) (model.EpisodeScriptVersion, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	workspace, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	project, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	episode, err := uuid.Parse(value.EpisodeID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	revision, err := uuid.Parse(value.DocumentRevisionID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	creator, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	return model.EpisodeScriptVersion{ID: id, WorkspaceID: workspace, ProjectID: project, EpisodeID: episode, VersionNo: value.VersionNo, DocumentRevisionID: revision, SourceStart: value.SourceStart, SourceEnd: value.SourceEnd, Content: value.Content, ContentHash: value.ContentHash, Status: value.Status, CreatedBy: creator, CreatedAt: value.CreatedAt, UpdatedAt: value.CreatedAt}, nil
}

func versionDomain(value model.EpisodeScriptVersion) application.Version {
	return application.Version{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		EpisodeID: value.EpisodeID.String(), DocumentRevisionID: value.DocumentRevisionID.String(),
		VersionNo: value.VersionNo, SourceStart: value.SourceStart, SourceEnd: value.SourceEnd,
		Content: value.Content, ContentHash: value.ContentHash, Status: value.Status,
		CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt,
	}
}

func outboxRecord(value domain.OutboxEvent) (model.OutboxEvent, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.OutboxEvent{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.OutboxEvent{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.OutboxEvent{}, err
	}
	receiptID, err := uuid.Parse(value.SourceReceiptID)
	if err != nil {
		return model.OutboxEvent{}, err
	}
	return model.OutboxEvent{
		ID: id, EventType: value.EventType, EventVersion: value.EventVersion,
		WorkspaceID: workspaceID, ProjectID: projectID,
		AggregateKind: value.AggregateKind, AggregateID: value.AggregateID, AggregateRevision: value.AggregateRevision,
		SourceReceiptID: receiptID, Payload: datatypes.JSON(value.Payload), PayloadHash: value.PayloadHash,
		Status: value.Status, Attempts: value.Attempts, OccurredAt: value.OccurredAt, CreatedAt: value.CreatedAt,
	}, nil
}
func commitRecord(value domain.ImportCommit) (model.ImportCommit, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.ImportCommit{}, err
	}
	workspace, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.ImportCommit{}, err
	}
	project, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.ImportCommit{}, err
	}
	plan, err := uuid.Parse(value.PlanID)
	if err != nil {
		return model.ImportCommit{}, err
	}
	creator, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.ImportCommit{}, err
	}
	segments, err := json.Marshal(value.Segments)
	if err != nil {
		return model.ImportCommit{}, err
	}
	return model.ImportCommit{ID: id, WorkspaceID: workspace, ProjectID: project, PlanID: plan, Mode: value.Mode, Status: value.Status, InputHash: value.InputHash, ExpectedProjectRevision: value.ExpectedProjectRevision, ExpectedActiveOrderHash: value.ExpectedActiveOrderHash, ErrorCode: value.ErrorCode, Segments: datatypes.JSON(segments), Revision: value.Revision, CreatedBy: creator, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}
func commitDomain(record model.ImportCommit) (domain.ImportCommit, error) {
	var segments []domain.Segment
	if err := json.Unmarshal(record.Segments, &segments); err != nil {
		return domain.ImportCommit{}, err
	}
	if segments == nil {
		segments = []domain.Segment{}
	}
	return domain.ImportCommit{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(), PlanID: record.PlanID.String(), Mode: record.Mode, Status: record.Status, InputHash: record.InputHash, ExpectedProjectRevision: record.ExpectedProjectRevision, ExpectedActiveOrderHash: record.ExpectedActiveOrderHash, ErrorCode: record.ErrorCode, Revision: record.Revision, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, Segments: segments}, nil
}
func structureRecord(value domain.Structure) (model.EpisodeStructure, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.EpisodeStructure{}, err
	}
	workspace, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.EpisodeStructure{}, err
	}
	project, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.EpisodeStructure{}, err
	}
	episode, err := uuid.Parse(value.EpisodeID)
	if err != nil {
		return model.EpisodeStructure{}, err
	}
	version, err := uuid.Parse(value.ScriptVersionID)
	if err != nil {
		return model.EpisodeStructure{}, err
	}
	creator, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.EpisodeStructure{}, err
	}
	var confirmer *uuid.UUID
	if value.ConfirmedBy != nil {
		parsed, e := uuid.Parse(*value.ConfirmedBy)
		if e != nil {
			return model.EpisodeStructure{}, e
		}
		confirmer = &parsed
	}
	scenes, err := json.Marshal(value.Scenes)
	if err != nil {
		return model.EpisodeStructure{}, err
	}
	return model.EpisodeStructure{ID: id, WorkspaceID: workspace, ProjectID: project, EpisodeID: episode, ScriptVersionID: version, Status: value.Status, Scenes: datatypes.JSON(scenes), ResultHash: value.ResultHash, Revision: value.Revision, ConfirmedBy: confirmer, ConfirmedAt: value.ConfirmedAt, CreatedBy: creator, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}
func structureDomain(record model.EpisodeStructure) (domain.Structure, error) {
	var scenes []domain.Scene
	if err := json.Unmarshal(record.Scenes, &scenes); err != nil {
		return domain.Structure{}, err
	}
	if scenes == nil {
		scenes = []domain.Scene{}
	}
	var confirmer *string
	if record.ConfirmedBy != nil {
		value := record.ConfirmedBy.String()
		confirmer = &value
	}
	return domain.Structure{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(), EpisodeID: record.EpisodeID.String(), ScriptVersionID: record.ScriptVersionID.String(), Status: record.Status, ResultHash: record.ResultHash, Revision: record.Revision, ConfirmedBy: confirmer, ConfirmedAt: record.ConfirmedAt, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, Scenes: scenes}, nil
}
func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	return err
}
func unauthenticated() error {
	return &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login"}
}
