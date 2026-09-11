package gormdb

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

func (store *Store) WithinEpisodeLifecycleTransaction(
	ctx context.Context,
	operation func(application.EpisodeLifecycleRepository) error,
) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) GetEpisodeLifecycleProject(
	ctx context.Context,
	projectID string,
	forUpdate bool,
) (domain.Project, error) {
	return repo.Get(ctx, projectID, forUpdate)
}

func (repo *repository) GetEpisodeLifecycleSource(
	ctx context.Context,
	versionID string,
) (domain.EpisodeLifecycleSource, error) {
	id, err := uuid.Parse(versionID)
	if err != nil {
		return domain.EpisodeLifecycleSource{}, application.ErrNotFound
	}
	var revision model.DocumentRevision
	if err = repo.database.WithContext(ctx).First(&revision, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.EpisodeLifecycleSource{}, application.ErrNotFound
		}
		return domain.EpisodeLifecycleSource{}, err
	}
	var document model.ScriptDocument
	if err = repo.database.WithContext(ctx).First(&document, "id = ?", revision.DocumentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.EpisodeLifecycleSource{}, application.ErrNotFound
		}
		return domain.EpisodeLifecycleSource{}, err
	}
	var head model.ScriptSourceScopeHead
	if err = repo.database.WithContext(ctx).First(&head, "project_id = ?", document.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.EpisodeLifecycleSource{}, application.ErrNotFound
		}
		return domain.EpisodeLifecycleSource{}, err
	}
	if head.WorkspaceID != revision.WorkspaceID || head.DocumentLogicalID != document.ID ||
		head.CurrentDocumentRevisionID != revision.ID {
		return domain.EpisodeLifecycleSource{}, errors.New("Project Episode lifecycle Script Source Head has drifted")
	}
	return domain.EpisodeLifecycleSource{
		WorkspaceID: revision.WorkspaceID.String(), ProjectID: document.ProjectID.String(),
		DocumentID: document.ID.String(), VersionID: revision.ID.String(), Revision: int64(revision.VersionNo),
		HeadRevision: head.HeadRevision, HeadHash: head.HeadHash,
		ContentHash: revision.NormalizedHash, NormalizedText: revision.NormalizedText, CreatedAt: revision.CreatedAt.UTC(),
	}, nil
}

func (repo *repository) ListEpisodeLifecycleEpisodes(
	ctx context.Context,
	projectID string,
) ([]domain.EpisodeLifecycleEpisode, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return nil, application.ErrNotFound
	}
	var records []model.Episode
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("project_id = ?", id).Order("position").Order("id").Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.EpisodeLifecycleEpisode, len(records))
	for index, record := range records {
		var current *string
		if record.CurrentScriptVersionID != nil {
			value := record.CurrentScriptVersionID.String()
			current = &value
		}
		result[index] = domain.EpisodeLifecycleEpisode{
			ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
			Name: record.Name, Position: record.Position, TargetDurationMS: record.TargetDurationMS,
			Status: record.Status, Revision: record.Revision, CurrentScriptVersionID: current,
			CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		}
	}
	return result, nil
}

func (repo *repository) FindEpisodeLifecycleScriptVersion(
	ctx context.Context,
	episodeID, documentRevisionID string,
	sourceStart, sourceEnd int,
	contentHash string,
) (domain.EpisodeLifecycleScriptVersion, bool, error) {
	episode, err := uuid.Parse(episodeID)
	if err != nil {
		return domain.EpisodeLifecycleScriptVersion{}, false, application.ErrNotFound
	}
	revision, err := uuid.Parse(documentRevisionID)
	if err != nil {
		return domain.EpisodeLifecycleScriptVersion{}, false, application.ErrNotFound
	}
	var record model.EpisodeScriptVersion
	err = repo.database.WithContext(ctx).Where(
		"episode_id = ? AND document_revision_id = ? AND source_start = ? AND source_end = ? AND content_hash = ? AND status = ?",
		episode, revision, sourceStart, sourceEnd, contentHash, "published",
	).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.EpisodeLifecycleScriptVersion{}, false, nil
	}
	if err != nil {
		return domain.EpisodeLifecycleScriptVersion{}, false, err
	}
	return episodeLifecycleScriptVersionDomain(record), true, nil
}

func (repo *repository) NextEpisodeLifecycleScriptVersion(ctx context.Context, episodeID string) (int, error) {
	id, err := uuid.Parse(episodeID)
	if err != nil {
		return 0, application.ErrNotFound
	}
	var latest model.EpisodeScriptVersion
	err = repo.database.WithContext(ctx).Where("episode_id = ?", id).Order("version_no DESC").First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return latest.VersionNo + 1, nil
}

func (repo *repository) GetEpisodeLifecycleScriptVersion(
	ctx context.Context,
	versionID string,
) (domain.EpisodeLifecycleScriptVersion, error) {
	id, err := uuid.Parse(versionID)
	if err != nil {
		return domain.EpisodeLifecycleScriptVersion{}, application.ErrNotFound
	}
	var record model.EpisodeScriptVersion
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.EpisodeLifecycleScriptVersion{}, application.ErrNotFound
		}
		return domain.EpisodeLifecycleScriptVersion{}, err
	}
	return episodeLifecycleScriptVersionDomain(record), nil
}

func (repo *repository) CreateEpisodeLifecycleEpisode(
	ctx context.Context,
	value domain.EpisodeLifecycleEpisode,
) error {
	record, err := episodeLifecycleEpisodeRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) SaveEpisodeLifecycleEpisode(
	ctx context.Context,
	value domain.EpisodeLifecycleEpisode,
) error {
	record, err := episodeLifecycleEpisodeRecord(value)
	if err != nil {
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.Episode{}).Where("id = ?", record.ID).Updates(map[string]any{
		"name": record.Name, "position": record.Position, "target_duration_ms": record.TargetDurationMS,
		"status": record.Status, "revision": record.Revision,
		"current_script_version_id": record.CurrentScriptVersionID, "updated_at": record.UpdatedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (repo *repository) CreateEpisodeLifecycleScriptVersion(
	ctx context.Context,
	value domain.EpisodeLifecycleScriptVersion,
) error {
	record, err := episodeLifecycleScriptVersionRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) SaveEpisodeLifecycleProject(ctx context.Context, value domain.Project) error {
	return repo.Save(ctx, value)
}

func (repo *repository) FindEpisodeOwnerVersion(
	ctx context.Context,
	episodeID string,
) (domain.EpisodeOwnerVersion, bool, error) {
	id, err := uuid.Parse(episodeID)
	if err != nil {
		return domain.EpisodeOwnerVersion{}, false, application.ErrNotFound
	}
	var record model.ProjectEpisodeVersion
	err = repo.database.WithContext(ctx).Where("episode_id = ?", id).Order("revision DESC").First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.EpisodeOwnerVersion{}, false, nil
	}
	if err != nil {
		return domain.EpisodeOwnerVersion{}, false, err
	}
	return projectEpisodeVersionDomain(record), true, nil
}

func (repo *repository) CreateEpisodeOwnerVersion(ctx context.Context, value domain.EpisodeOwnerVersion) error {
	record, err := projectEpisodeVersionRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) CreateProjectEpisodeMemberships(
	ctx context.Context,
	values []domain.ProjectEpisodeMembership,
) error {
	records := make([]model.ProjectEpisodeMembership, len(values))
	for index, value := range values {
		record, err := projectEpisodeMembershipRecord(value)
		if err != nil {
			return err
		}
		records[index] = record
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&records).Error
}

func (repo *repository) GetProjectEpisodeScopeHead(
	ctx context.Context,
	projectID string,
	forUpdate bool,
) (domain.ProjectEpisodeScopeHead, bool, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return domain.ProjectEpisodeScopeHead{}, false, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.ProjectEpisodeScopeHead
	err = query.First(&record, "project_id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ProjectEpisodeScopeHead{}, false, nil
	}
	if err != nil {
		return domain.ProjectEpisodeScopeHead{}, false, err
	}
	var refs []ownercollection.VersionRef
	if json.Unmarshal(record.CurrentVersionRefs, &refs) != nil {
		return domain.ProjectEpisodeScopeHead{}, false, errors.New("Project Episode Scope Head refs have drifted")
	}
	return domain.ProjectEpisodeScopeHead{
		WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(), ScopeKey: record.ScopeKey,
		ScopeRevision: record.ScopeRevision, ScopeContentHash: record.ScopeContentHash,
		MemberCount: record.MemberCount, MembersHash: record.MembersHash, CollectionRootHash: record.CollectionRootHash,
		CurrentVersionRefs: refs, HeadRevision: record.HeadRevision, HeadContentHash: record.HeadContentHash,
		UpdatedAt: record.UpdatedAt.UTC(),
	}, true, nil
}

func (repo *repository) CreateProjectEpisodeScopeHead(ctx context.Context, value domain.ProjectEpisodeScopeHead) error {
	record, err := projectEpisodeScopeHeadRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) AdvanceProjectEpisodeScopeHead(
	ctx context.Context,
	value domain.ProjectEpisodeScopeHead,
	expectedRevision int64,
	expectedHash string,
) error {
	record, err := projectEpisodeScopeHeadRecord(value)
	if err != nil {
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.ProjectEpisodeScopeHead{}).
		Where("project_id = ? AND head_revision = ? AND head_content_hash = ?", record.ProjectID, expectedRevision, expectedHash).
		Updates(map[string]any{
			"scope_revision": record.ScopeRevision, "scope_content_hash": record.ScopeContentHash,
			"member_count": record.MemberCount, "members_hash": record.MembersHash,
			"collection_root_hash": record.CollectionRootHash, "current_version_refs": record.CurrentVersionRefs,
			"head_revision": record.HeadRevision, "head_content_hash": record.HeadContentHash, "updated_at": record.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("Project Episode Scope Head CAS conflict")
	}
	return nil
}

func (repo *repository) CreateProjectEpisodeCollectionReceipt(
	ctx context.Context,
	value domain.ProjectEpisodeCollectionReceipt,
) error {
	record, err := projectEpisodeCollectionReceiptRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func episodeLifecycleEpisodeRecord(value domain.EpisodeLifecycleEpisode) (model.Episode, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.Episode{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.Episode{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.Episode{}, err
	}
	var current *uuid.UUID
	if value.CurrentScriptVersionID != nil {
		parsed, parseErr := uuid.Parse(*value.CurrentScriptVersionID)
		if parseErr != nil {
			return model.Episode{}, parseErr
		}
		current = &parsed
	}
	return model.Episode{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, Name: value.Name,
		Position: value.Position, TargetDurationMS: value.TargetDurationMS,
		Status: value.Status, Revision: value.Revision, CurrentScriptVersionID: current,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func episodeLifecycleScriptVersionRecord(
	value domain.EpisodeLifecycleScriptVersion,
) (model.EpisodeScriptVersion, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	episodeID, err := uuid.Parse(value.EpisodeID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	revisionID, err := uuid.Parse(value.DocumentRevisionID)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.EpisodeScriptVersion{}, err
	}
	return model.EpisodeScriptVersion{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID,
		VersionNo: value.VersionNo, DocumentRevisionID: revisionID,
		SourceStart: value.SourceStart, SourceEnd: value.SourceEnd,
		Content: value.Content, ContentHash: value.ContentHash, Status: value.Status,
		CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func episodeLifecycleScriptVersionDomain(
	record model.EpisodeScriptVersion,
) domain.EpisodeLifecycleScriptVersion {
	return domain.EpisodeLifecycleScriptVersion{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		EpisodeID: record.EpisodeID.String(), DocumentRevisionID: record.DocumentRevisionID.String(),
		VersionNo: record.VersionNo, SourceStart: record.SourceStart, SourceEnd: record.SourceEnd,
		Content: record.Content, ContentHash: record.ContentHash, Status: record.Status,
		CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func projectEpisodeVersionRecord(value domain.EpisodeOwnerVersion) (model.ProjectEpisodeVersion, error) {
	ids, err := parseEpisodeOwnerUUIDs(
		value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID,
		value.SourceVersionID, value.ScriptVersionID, value.CreatedBy,
	)
	if err != nil {
		return model.ProjectEpisodeVersion{}, err
	}
	var parentID *uuid.UUID
	if value.ParentVersionID != nil {
		parsed, parseErr := uuid.Parse(*value.ParentVersionID)
		if parseErr != nil {
			return model.ProjectEpisodeVersion{}, parseErr
		}
		parentID = &parsed
	}
	return model.ProjectEpisodeVersion{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], EpisodeID: ids[3], Revision: value.Revision,
		ParentVersionID: parentID, ParentContentHash: value.ParentContentHash,
		Status: value.Status, Position: value.Position, SequenceKey: value.SequenceKey,
		Name: value.Name, TargetDurationMS: value.TargetDurationMS,
		SourceVersionID: ids[4], ScriptVersionID: ids[5], SourceStart: value.SourceStart, SourceEnd: value.SourceEnd,
		ScriptContentHash: value.ScriptContentHash, ContentHash: value.ContentHash,
		CreatedBy: ids[6], CreatedAt: value.CreatedAt,
	}, nil
}

func projectEpisodeVersionDomain(value model.ProjectEpisodeVersion) domain.EpisodeOwnerVersion {
	var parentID *string
	if value.ParentVersionID != nil {
		encoded := value.ParentVersionID.String()
		parentID = &encoded
	}
	return domain.EpisodeOwnerVersion{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		EpisodeID: value.EpisodeID.String(), Revision: value.Revision,
		ParentVersionID: parentID, ParentContentHash: value.ParentContentHash,
		Status: value.Status, Position: value.Position, SequenceKey: value.SequenceKey,
		Name: value.Name, TargetDurationMS: value.TargetDurationMS,
		SourceVersionID: value.SourceVersionID.String(), ScriptVersionID: value.ScriptVersionID.String(),
		SourceStart: value.SourceStart, SourceEnd: value.SourceEnd, ScriptContentHash: value.ScriptContentHash,
		ContentHash: value.ContentHash, CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt.UTC(),
	}
}

func projectEpisodeMembershipRecord(value domain.ProjectEpisodeMembership) (model.ProjectEpisodeMembership, error) {
	ids, err := parseEpisodeOwnerUUIDs(value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, value.EpisodeVersionID)
	if err != nil {
		return model.ProjectEpisodeMembership{}, err
	}
	return model.ProjectEpisodeMembership{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], ScopeRevision: value.ScopeRevision,
		Position: value.Position, EpisodeID: ids[3], EpisodeVersionID: ids[4],
		VersionContentHash: value.VersionContentHash, CreatedAt: value.CreatedAt,
	}, nil
}

func projectEpisodeScopeHeadRecord(value domain.ProjectEpisodeScopeHead) (model.ProjectEpisodeScopeHead, error) {
	ids, err := parseEpisodeOwnerUUIDs(value.ProjectID, value.WorkspaceID)
	if err != nil {
		return model.ProjectEpisodeScopeHead{}, err
	}
	refs, err := json.Marshal(value.CurrentVersionRefs)
	if err != nil {
		return model.ProjectEpisodeScopeHead{}, err
	}
	return model.ProjectEpisodeScopeHead{
		ProjectID: ids[0], WorkspaceID: ids[1], ScopeKey: value.ScopeKey,
		ScopeRevision: value.ScopeRevision, ScopeContentHash: value.ScopeContentHash,
		MemberCount: value.MemberCount, MembersHash: value.MembersHash, CollectionRootHash: value.CollectionRootHash,
		CurrentVersionRefs: refs, HeadRevision: value.HeadRevision, HeadContentHash: value.HeadContentHash,
		UpdatedAt: value.UpdatedAt,
	}, nil
}

func projectEpisodeCollectionReceiptRecord(
	value domain.ProjectEpisodeCollectionReceipt,
) (model.ProjectEpisodeCollectionReceipt, error) {
	ids, err := parseEpisodeOwnerUUIDs(
		value.ID, value.CommandID, value.WorkspaceID, value.ProjectID, value.ReviewDecisionID, value.CommittedBy,
	)
	if err != nil {
		return model.ProjectEpisodeCollectionReceipt{}, err
	}
	members, err := json.Marshal(value.Members)
	if err != nil {
		return model.ProjectEpisodeCollectionReceipt{}, err
	}
	covered, err := json.Marshal(value.CoveredScopeKeys)
	if err != nil {
		return model.ProjectEpisodeCollectionReceipt{}, err
	}
	committed, err := json.Marshal(value.CommittedOwnerVersionRefs)
	if err != nil {
		return model.ProjectEpisodeCollectionReceipt{}, err
	}
	return model.ProjectEpisodeCollectionReceipt{
		ID: ids[0], CommandID: ids[1], IdempotencyKey: value.IdempotencyKey,
		WorkspaceID: ids[2], ProjectID: ids[3], DecisionCheckpointID: domain.ProjectEpisodeCheckpoint,
		ReviewDecisionID: ids[4], OwnerKind: "production/project", VersionFamily: domain.ProjectEpisodeCollectionFamily,
		ScopeKind: "project", ScopeKey: "project:" + value.ProjectID,
		ScopeRevision: value.ScopeRevision, ScopeContentHash: value.ScopeContentHash,
		Members: members, MemberCount: value.MemberCount, MembersHash: value.MembersHash,
		CollectionRootHash: value.CollectionRootHash, CoveredScopeKeys: covered,
		CommittedOwnerVersionRefs: committed, ReceiptContentHash: value.ReceiptContentHash,
		CommittedAt: value.CommittedAt, CommittedBy: ids[5],
	}, nil
}

func parseEpisodeOwnerUUIDs(values ...string) ([]uuid.UUID, error) {
	result := make([]uuid.UUID, len(values))
	for index, value := range values {
		parsed, err := uuid.Parse(value)
		if err != nil {
			return nil, err
		}
		result[index] = parsed
	}
	return result, nil
}

var _ application.EpisodeLifecycleTransactionManager = (*Store)(nil)
var _ application.EpisodeLifecycleRepository = (*repository)(nil)
