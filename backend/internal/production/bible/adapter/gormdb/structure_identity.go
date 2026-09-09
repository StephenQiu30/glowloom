package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	eventingdomain "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

func (store *Store) WithinStructureIdentityTransaction(
	ctx context.Context,
	operation func(application.StructureIdentityRepository) error,
) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) AuthorizeStructureIdentity(ctx context.Context, actor application.Actor, projectID string) error {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return application.ErrNotFound
	}
	return authorizeProject(ctx, repo.database, actor, id, true)
}

func (repo *repository) FindStructureIdentityCommandReceipt(
	ctx context.Context,
	workspaceID, key string,
) (platformcommand.Receipt, error) {
	return repo.FindReceipt(ctx, workspaceID, "production_bible.confirm_structure_identity_set", key)
}

func (repo *repository) GetStructureIdentitySource(
	ctx context.Context,
	projectID, revisionID string,
	lock bool,
) (application.StructureIdentitySource, error) {
	project, err := uuid.Parse(projectID)
	if err != nil {
		return application.StructureIdentitySource{}, application.ErrNotFound
	}
	revision, err := uuid.Parse(revisionID)
	if err != nil {
		return application.StructureIdentitySource{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var head model.ScriptSourceScopeHead
	if err = query.First(&head, "project_id = ?", project).Error; err != nil {
		return application.StructureIdentitySource{}, normalizeNotFound(err)
	}
	if head.CurrentDocumentRevisionID != revision {
		return application.StructureIdentitySource{}, errors.New("Production Bible source Head has changed")
	}
	var document model.DocumentRevision
	if err = repo.database.WithContext(ctx).First(&document, "id = ?", revision).Error; err != nil {
		return application.StructureIdentitySource{}, normalizeNotFound(err)
	}
	var spanIndex model.SourceSpanIndexVersion
	if err = repo.database.WithContext(ctx).First(&spanIndex, "id = ?", head.CurrentSpanIndexID).Error; err != nil {
		return application.StructureIdentitySource{}, normalizeNotFound(err)
	}
	if document.WorkspaceID != head.WorkspaceID || spanIndex.WorkspaceID != head.WorkspaceID ||
		spanIndex.ProjectID != project || spanIndex.DocumentRevisionID != revision || spanIndex.SourceHash != document.NormalizedHash {
		return application.StructureIdentitySource{}, errors.New("Production Bible source facts have drifted")
	}
	return application.StructureIdentitySource{
		WorkspaceID: head.WorkspaceID.String(), ProjectID: project.String(),
		DocumentRevisionID: document.ID.String(), DocumentRevisionHash: document.NormalizedHash,
		SpanIndexID: spanIndex.ID.String(), SpanIndexHash: spanIndex.ContentHash,
		CodepointCount: spanIndex.CodepointCount,
	}, nil
}

func (repo *repository) GetEpisodeLifecycleReceipt(
	ctx context.Context,
	receiptID, workspaceID, projectID string,
) (application.StructureIdentityEpisodeCheckpoint, error) {
	id, err := uuid.Parse(receiptID)
	if err != nil {
		return application.StructureIdentityEpisodeCheckpoint{}, application.ErrNotFound
	}
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return application.StructureIdentityEpisodeCheckpoint{}, application.ErrNotFound
	}
	project, err := uuid.Parse(projectID)
	if err != nil {
		return application.StructureIdentityEpisodeCheckpoint{}, application.ErrNotFound
	}
	var receipt model.CommandReceipt
	if err = repo.database.WithContext(ctx).First(&receipt, "id = ?", id).Error; err != nil {
		return application.StructureIdentityEpisodeCheckpoint{}, normalizeNotFound(err)
	}
	if receipt.WorkspaceID != workspace || receipt.ResourceID != project || receipt.Operation != "project.confirm_episode_lifecycle" {
		return application.StructureIdentityEpisodeCheckpoint{}, errors.New("Project Episode lifecycle receipt is not the required checkpoint")
	}
	var result projectdomain.EpisodeLifecycleSet
	if err = json.Unmarshal(receipt.Result, &result); err != nil || result.ID != receiptID ||
		result.WorkspaceID != workspaceID || result.ProjectID != projectID ||
		result.SchemaVersion != projectdomain.EpisodeLifecycleSetSchemaVersion || len(result.Episodes) == 0 {
		return application.StructureIdentityEpisodeCheckpoint{}, errors.New("Project Episode lifecycle receipt is invalid")
	}
	var activeEpisodes []model.Episode
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("project_id = ? AND status = ?", project, "active").Order("position").Order("id").Find(&activeEpisodes).Error; err != nil {
		return application.StructureIdentityEpisodeCheckpoint{}, err
	}
	if len(activeEpisodes) != len(result.Episodes) {
		return application.StructureIdentityEpisodeCheckpoint{}, errors.New("Project Episode lifecycle active set has drifted")
	}
	refs := make([]domain.EpisodeLifecycleRef, len(result.Episodes))
	previousEnd := 0
	for index, value := range result.Episodes {
		if value.Position != index+1 || value.SourceStart != previousEnd || value.SourceEnd <= value.SourceStart ||
			activeEpisodes[index].ID.String() != value.EpisodeID {
			return application.StructureIdentityEpisodeCheckpoint{}, errors.New("Project Episode lifecycle coverage has drifted")
		}
		episodeID, parseErr := uuid.Parse(value.EpisodeID)
		if parseErr != nil {
			return application.StructureIdentityEpisodeCheckpoint{}, errors.New("Project Episode lifecycle identity is invalid")
		}
		var episode model.Episode
		if parseErr = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&episode, "id = ?", episodeID).Error; parseErr != nil {
			return application.StructureIdentityEpisodeCheckpoint{}, normalizeNotFound(parseErr)
		}
		scriptID, parseErr := uuid.Parse(value.ScriptVersionID)
		if parseErr != nil {
			return application.StructureIdentityEpisodeCheckpoint{}, errors.New("Project Episode Script Version identity is invalid")
		}
		var script model.EpisodeScriptVersion
		if parseErr = repo.database.WithContext(ctx).First(&script, "id = ?", scriptID).Error; parseErr != nil {
			return application.StructureIdentityEpisodeCheckpoint{}, normalizeNotFound(parseErr)
		}
		if episode.WorkspaceID != workspace || episode.ProjectID != project || episode.Status != "active" ||
			episode.Revision != value.EpisodeRevision || episode.Position != value.Position || episode.CurrentScriptVersionID == nil ||
			*episode.CurrentScriptVersionID != script.ID || script.WorkspaceID != workspace || script.ProjectID != project ||
			script.EpisodeID != episode.ID || script.VersionNo != value.ScriptVersion || script.SourceStart != value.SourceStart ||
			script.SourceEnd != value.SourceEnd || script.DocumentRevisionID.String() != result.SourceVersionID ||
			script.ContentHash != value.ContentHash || script.Status != "published" {
			return application.StructureIdentityEpisodeCheckpoint{}, errors.New("Project Episode lifecycle checkpoint has drifted")
		}
		refs[index] = domain.EpisodeLifecycleRef{
			TemporaryEpisodeID: value.TemporaryEpisodeID, EpisodeID: value.EpisodeID,
			EpisodeRevision: value.EpisodeRevision, Position: value.Position,
			ScriptVersionID: value.ScriptVersionID, ScriptVersion: value.ScriptVersion,
			SourceStart: value.SourceStart, SourceEnd: value.SourceEnd, ContentHash: value.ContentHash,
		}
		previousEnd = value.SourceEnd
	}
	recomputedRoot, err := platformcommand.InputHash(struct {
		SchemaVersion   string                       `json:"schema_version"`
		SourceVersionID string                       `json:"source_version_id"`
		SourceHash      string                       `json:"source_hash"`
		Episodes        []domain.EpisodeLifecycleRef `json:"episodes"`
	}{projectdomain.EpisodeLifecycleSetSchemaVersion, result.SourceVersionID, result.SourceHash, refs})
	if err != nil || recomputedRoot != result.CollectionRootHash {
		return application.StructureIdentityEpisodeCheckpoint{}, errors.New("Project Episode lifecycle collection root has drifted")
	}
	return application.StructureIdentityEpisodeCheckpoint{
		GateInputID: result.GateInputID, GateInputHash: result.GateInputHash,
		ReviewDecisionID: result.ReviewDecisionID, SourceVersionID: result.SourceVersionID,
		SourceHash: result.SourceHash, CollectionRootHash: result.CollectionRootHash, Episodes: refs,
	}, nil
}

func (repo *repository) GetStructureIdentityHead(
	ctx context.Context,
	projectID string,
	lock bool,
) (application.StructureIdentityHead, bool, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return application.StructureIdentityHead{}, false, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var head model.StructureIdentityScopeHead
	err = query.First(&head, "project_id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.StructureIdentityHead{}, false, nil
	}
	if err != nil {
		return application.StructureIdentityHead{}, false, err
	}
	return application.StructureIdentityHead{
		CurrentVersionID: head.CurrentVersionID.String(), HeadRevision: head.HeadRevision, HeadHash: head.HeadHash,
	}, true, nil
}

func (repo *repository) GetStructureIdentityVersion(
	ctx context.Context,
	versionID string,
) (domain.StructureIdentitySetVersion, error) {
	id, err := uuid.Parse(versionID)
	if err != nil {
		return domain.StructureIdentitySetVersion{}, application.ErrNotFound
	}
	var record model.StructureIdentitySetVersion
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return domain.StructureIdentitySetVersion{}, normalizeNotFound(err)
	}
	return structureIdentityVersionDomain(record)
}

func (repo *repository) CreateStructureIdentityVersion(
	ctx context.Context,
	value domain.StructureIdentitySetVersion,
) error {
	record, err := structureIdentityVersionRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) SaveStructureIdentityHead(
	ctx context.Context,
	workspaceID, projectID string,
	head application.StructureIdentityHead,
	now time.Time,
) error {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return err
	}
	project, err := uuid.Parse(projectID)
	if err != nil {
		return err
	}
	version, err := uuid.Parse(head.CurrentVersionID)
	if err != nil {
		return err
	}
	record := model.StructureIdentityScopeHead{
		ProjectID: project, WorkspaceID: workspace, CurrentVersionID: version,
		HeadRevision: head.HeadRevision, HeadHash: head.HeadHash, UpdatedAt: now,
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Save(&record).Error
}

func (repo *repository) CreateStructureIdentityCollectionReceipt(
	ctx context.Context,
	value domain.StructureIdentityCollectionReceipt,
	workspaceID, projectID string,
	now time.Time,
) error {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return err
	}
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return err
	}
	project, err := uuid.Parse(projectID)
	if err != nil {
		return err
	}
	version, err := uuid.Parse(value.VersionID)
	if err != nil {
		return err
	}
	review, err := uuid.Parse(value.ReviewDecisionID)
	if err != nil {
		return err
	}
	scopes, err := json.Marshal(value.CoveredScopeKeys)
	if err != nil {
		return err
	}
	members, err := json.Marshal([]map[string]string{{"version_id": value.VersionID, "content_hash": value.VersionContentHash}})
	if err != nil {
		return err
	}
	record := model.StructureIdentityCollectionReceipt{
		ID: id, WorkspaceID: workspace, ProjectID: project, VersionID: version, ReviewDecisionID: review,
		CheckpointKey: value.CheckpointKey, CollectionFamily: value.CollectionFamily,
		CoveredScopeKeys: datatypes.JSON(scopes), Members: datatypes.JSON(members),
		CollectionRootHash: value.CollectionRootHash, ReceiptContentHash: value.ReceiptContentHash,
		CreatedAt: now,
	}
	// The user who committed the command owns the receipt; ReviewDecision is provenance only.
	var versionRecord model.StructureIdentitySetVersion
	if err = repo.database.WithContext(ctx).Select("created_by").First(&versionRecord, "id = ?", version).Error; err != nil {
		return err
	}
	record.CreatedBy = versionRecord.CreatedBy
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) CreateStructureIdentityCommandReceipt(ctx context.Context, receipt platformcommand.Receipt) error {
	return repo.CreateReceipt(ctx, receipt)
}

func (repo *repository) AppendStructureIdentityAudit(
	ctx context.Context,
	value application.StructureIdentityAudit,
) error {
	workspace, actor, project, err := parseStructureIdentityAuditIDs(value)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(map[string]any{
		"version_id": value.VersionID, "version": value.Version,
		"review_decision_id": value.ReviewDecisionID, "collection_root_hash": value.CollectionRootHash,
	})
	if err != nil {
		return err
	}
	record := model.AuditEvent{
		ID: uuid.New(), WorkspaceID: workspace, ActorID: actor,
		Action: "production_bible.structure_identity_confirmed", TargetType: "project", TargetID: project,
		Result: "succeeded", TraceID: uuid.NewString(), Metadata: datatypes.JSON(metadata), OccurredAt: value.OccurredAt,
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) AppendStructureIdentityOutbox(
	ctx context.Context,
	value application.StructureIdentityOutbox,
) error {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return err
	}
	workspace, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return err
	}
	project, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return err
	}
	receipt, err := uuid.Parse(value.SourceReceiptID)
	if err != nil {
		return err
	}
	record := model.OutboxEvent{
		ID: id, EventType: eventingdomain.StructureIdentitySetPublished, EventVersion: value.EventVersion,
		WorkspaceID: workspace, ProjectID: project, AggregateKind: "production_bible_structure_identity",
		AggregateID: value.VersionID, AggregateRevision: int64(value.Version), SourceReceiptID: receipt,
		Payload: datatypes.JSON(value.Payload), PayloadHash: value.PayloadHash, Status: "pending",
		OccurredAt: value.OccurredAt, CreatedAt: value.OccurredAt,
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func structureIdentityVersionRecord(value domain.StructureIdentitySetVersion) (model.StructureIdentitySetVersion, error) {
	id, workspace, project, err := parseThreeUUIDs(value.ID, value.WorkspaceID, value.ProjectID)
	if err != nil {
		return model.StructureIdentitySetVersion{}, err
	}
	gate, review, episodeReceipt, err := parseThreeUUIDs(value.GateInputID, value.ReviewDecisionID, value.ProjectEpisodeReceiptID)
	if err != nil {
		return model.StructureIdentitySetVersion{}, err
	}
	document, spanIndex, creator, err := parseThreeUUIDs(value.DocumentRevisionID, value.SpanIndexID, value.CreatedBy)
	if err != nil {
		return model.StructureIdentitySetVersion{}, err
	}
	var parent *uuid.UUID
	if value.ParentVersionID != nil {
		parsed, parseErr := uuid.Parse(*value.ParentVersionID)
		if parseErr != nil {
			return model.StructureIdentitySetVersion{}, parseErr
		}
		parent = &parsed
	}
	encoded, err := marshalStructureIdentityVersionParts(value)
	if err != nil {
		return model.StructureIdentitySetVersion{}, err
	}
	return model.StructureIdentitySetVersion{
		ID: id, WorkspaceID: workspace, ProjectID: project, Version: value.Version, ParentVersionID: parent,
		GateInputID: gate, GateInputHash: value.GateInputHash, ReviewDecisionID: review,
		ProjectEpisodeReceiptID: episodeReceipt, DocumentRevisionID: document, SpanIndexID: spanIndex,
		CandidateRefs: encoded[0], EpisodeRefs: encoded[1], SceneRefs: encoded[2], Identities: encoded[3],
		MentionMappings: encoded[4], Coverage: encoded[5], ContentHash: value.ContentHash,
		CreatedBy: creator, CreatedAt: value.CreatedAt,
	}, nil
}

func structureIdentityVersionDomain(record model.StructureIdentitySetVersion) (domain.StructureIdentitySetVersion, error) {
	value := domain.StructureIdentitySetVersion{
		SchemaVersion: domain.StructureIdentitySetSchemaVersion, ID: record.ID.String(),
		WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(), Version: record.Version,
		GateInputID: record.GateInputID.String(), GateInputHash: record.GateInputHash,
		ReviewDecisionID: record.ReviewDecisionID.String(), ProjectEpisodeReceiptID: record.ProjectEpisodeReceiptID.String(),
		DocumentRevisionID: record.DocumentRevisionID.String(), SpanIndexID: record.SpanIndexID.String(),
		ContentHash: record.ContentHash, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
	}
	if record.ParentVersionID != nil {
		parent := record.ParentVersionID.String()
		value.ParentVersionID = &parent
	}
	for _, item := range []struct {
		raw datatypes.JSON
		to  any
	}{
		{record.CandidateRefs, &value.CandidateRefs}, {record.EpisodeRefs, &value.EpisodeRefs},
		{record.SceneRefs, &value.SceneRefs}, {record.Identities, &value.Identities},
		{record.MentionMappings, &value.MentionMappings}, {record.Coverage, &value.Coverage},
	} {
		if err := json.Unmarshal(item.raw, item.to); err != nil {
			return domain.StructureIdentitySetVersion{}, fmt.Errorf("decode StructureIdentitySetVersion: %w", err)
		}
	}
	return value, nil
}

func marshalStructureIdentityVersionParts(value domain.StructureIdentitySetVersion) ([6]datatypes.JSON, error) {
	parts := []any{value.CandidateRefs, value.EpisodeRefs, value.SceneRefs, value.Identities, value.MentionMappings, value.Coverage}
	var result [6]datatypes.JSON
	for index, part := range parts {
		encoded, err := json.Marshal(part)
		if err != nil {
			return result, err
		}
		result[index] = datatypes.JSON(encoded)
	}
	return result, nil
}

func parseThreeUUIDs(first, second, third string) (uuid.UUID, uuid.UUID, uuid.UUID, error) {
	one, err := uuid.Parse(first)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	two, err := uuid.Parse(second)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	three, err := uuid.Parse(third)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	return one, two, three, nil
}

func parseStructureIdentityAuditIDs(value application.StructureIdentityAudit) (uuid.UUID, uuid.UUID, uuid.UUID, error) {
	return parseThreeUUIDs(value.WorkspaceID, value.ActorID, value.ProjectID)
}

var _ application.StructureIdentityTransactionManager = (*Store)(nil)
var _ application.StructureIdentityRepository = (*repository)(nil)
