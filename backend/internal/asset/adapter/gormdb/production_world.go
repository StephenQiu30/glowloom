package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	"github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

func NewProductionWorldRepository(database *gorm.DB) application.ProductionWorldAssetRepository {
	return &repository{database: database}
}

func (repo *repository) GetIdentityStateHead(
	ctx context.Context,
	workspaceID, projectID string,
	lock bool,
) (domain.IdentityStateCollectionHead, error) {
	workspaceUUID, workspaceErr := uuid.Parse(workspaceID)
	projectUUID, projectErr := uuid.Parse(projectID)
	if workspaceErr != nil || projectErr != nil {
		return domain.IdentityStateCollectionHead{}, application.ErrIdentityStateHeadNotFound
	}
	query := repo.database.WithContext(ctx).Where("project_id = ?", projectUUID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.AssetIdentityStateScopeHead
	if err := query.First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.IdentityStateCollectionHead{}, application.ErrIdentityStateHeadNotFound
		}
		return domain.IdentityStateCollectionHead{}, err
	}
	if record.WorkspaceID != workspaceUUID {
		return domain.IdentityStateCollectionHead{}, errors.New("Asset identity-state Head workspace has drifted")
	}
	var cachedMembers []domain.IdentityStateMember
	if err := json.Unmarshal(record.CurrentRootRefs, &cachedMembers); err != nil {
		return domain.IdentityStateCollectionHead{}, errors.New("Asset identity-state Head members have drifted")
	}
	var membershipRecords []model.AssetIdentityStateMembership
	if err := repo.database.WithContext(ctx).
		Where("project_id = ? AND scope_revision = ?", projectUUID, record.ScopeRevision).
		Order("position ASC").Find(&membershipRecords).Error; err != nil {
		return domain.IdentityStateCollectionHead{}, err
	}
	members := make([]domain.IdentityStateMember, len(membershipRecords))
	for index, membership := range membershipRecords {
		if membership.WorkspaceID != workspaceUUID || membership.Position != index+1 {
			return domain.IdentityStateCollectionHead{}, errors.New("Asset identity-state membership has drifted")
		}
		members[index] = domain.IdentityStateMember{
			AssetID: membership.AssetID.String(), AssetStateID: membership.AssetStateID.String(),
			IdentityKey: membership.IdentityKey, StateKey: membership.StateKey,
			AssetContentHash: membership.AssetContentHash, StateContentHash: membership.StateContentHash,
			MemberContentHash: membership.MemberContentHash,
		}
		if domain.ValidateIdentityStateMember(members[index]) != nil {
			return domain.IdentityStateCollectionHead{}, errors.New("Asset identity-state membership has drifted")
		}
	}
	if !reflect.DeepEqual(cachedMembers, members) {
		return domain.IdentityStateCollectionHead{}, errors.New("Asset identity-state Head cache has drifted")
	}
	head, err := domain.NewIdentityStateCollectionHead(
		record.WorkspaceID.String(), record.ProjectID.String(), record.ScopeRevision, members, record.UpdatedAt,
	)
	if err != nil || head.ScopeContentHash != record.ScopeContentHash || head.MemberCount != record.MemberCount ||
		head.MembersHash != record.MembersHash || head.CollectionRootHash != record.CollectionRootHash ||
		head.HeadRevision != record.HeadRevision || head.HeadContentHash != record.HeadContentHash {
		return domain.IdentityStateCollectionHead{}, errors.New("Asset identity-state Head has drifted")
	}
	return head, nil
}

func (repo *repository) GetAssetByIdentityKey(
	ctx context.Context,
	projectID, identityKey string,
	lock bool,
) (domain.Asset, error) {
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return domain.Asset{}, application.ErrProductionWorldAssetNotFound
	}
	query := repo.database.WithContext(ctx).Where("project_id = ? AND identity_key = ?", projectUUID, identityKey)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.Asset
	if err = query.First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Asset{}, application.ErrProductionWorldAssetNotFound
		}
		return domain.Asset{}, err
	}
	return productionWorldAssetDomain(record), nil
}

func (repo *repository) CreateProductionWorldAsset(ctx context.Context, value domain.Asset) error {
	record, err := productionWorldAssetRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) GetLatestAssetState(
	ctx context.Context,
	assetID, stateKey string,
	lock bool,
) (domain.AssetState, error) {
	assetUUID, err := uuid.Parse(assetID)
	if err != nil {
		return domain.AssetState{}, application.ErrProductionWorldAssetStateNotFound
	}
	query := repo.database.WithContext(ctx).
		Where("asset_id = ? AND state_key = ?", assetUUID, stateKey).Order("revision DESC")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.AssetState
	if err = query.First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.AssetState{}, application.ErrProductionWorldAssetStateNotFound
		}
		return domain.AssetState{}, err
	}
	return productionWorldAssetStateDomain(record)
}

func (repo *repository) CreateProductionWorldAssetState(ctx context.Context, value domain.AssetState) error {
	record, err := productionWorldAssetStateRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) CreateIdentityStateMemberships(
	ctx context.Context,
	head domain.IdentityStateCollectionHead,
) error {
	records := make([]model.AssetIdentityStateMembership, len(head.Members))
	for index, member := range head.Members {
		assetID, assetErr := uuid.Parse(member.AssetID)
		stateID, stateErr := uuid.Parse(member.AssetStateID)
		workspaceID, workspaceErr := uuid.Parse(head.WorkspaceID)
		projectID, projectErr := uuid.Parse(head.ProjectID)
		if assetErr != nil || stateErr != nil || workspaceErr != nil || projectErr != nil {
			return errors.New("invalid Asset identity-state membership identity")
		}
		records[index] = model.AssetIdentityStateMembership{
			ID: uuid.NewSHA1(projectID, []byte("lanverse:asset-identity-state-member:"+
				strconv.FormatInt(head.ScopeRevision, 10)+":"+member.MemberContentHash)),
			WorkspaceID: workspaceID, ProjectID: projectID, ScopeRevision: head.ScopeRevision, Position: index + 1,
			AssetID: assetID, AssetStateID: stateID, IdentityKey: member.IdentityKey, StateKey: member.StateKey,
			AssetContentHash: member.AssetContentHash, StateContentHash: member.StateContentHash,
			MemberContentHash: member.MemberContentHash, CreatedAt: head.UpdatedAt,
		}
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&records).Error
}

func (repo *repository) SaveIdentityStateHead(
	ctx context.Context,
	head domain.IdentityStateCollectionHead,
	expectedRevision int64,
	expectedHash string,
) error {
	record, err := productionWorldIdentityStateHeadRecord(head)
	if err != nil {
		return err
	}
	if expectedRevision == 0 {
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return application.ErrIdentityStateHeadConflict
			}
			return err
		}
		return nil
	}
	updated := repo.database.WithContext(ctx).Model(&model.AssetIdentityStateScopeHead{}).
		Where("project_id = ? AND workspace_id = ? AND head_revision = ? AND head_content_hash = ?",
			record.ProjectID, record.WorkspaceID, expectedRevision, expectedHash).
		Select("scope_revision", "scope_content_hash", "member_count", "members_hash", "collection_root_hash",
			"current_root_refs", "head_revision", "head_content_hash", "updated_at").Updates(&record)
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return application.ErrIdentityStateHeadConflict
	}
	return nil
}

func productionWorldAssetRecord(value domain.Asset) (model.Asset, error) {
	if domain.ValidateAsset(value) != nil {
		return model.Asset{}, errors.New("invalid Production World Asset")
	}
	id, _ := uuid.Parse(value.ID)
	workspaceID, _ := uuid.Parse(value.WorkspaceID)
	projectID, _ := uuid.Parse(value.ProjectID)
	createdBy, _ := uuid.Parse(value.CreatedBy)
	return model.Asset{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, Kind: value.Kind, IdentityKey: value.IdentityKey,
		Revision: value.Revision, ContentHash: value.ContentHash, CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}

func productionWorldAssetDomain(value model.Asset) domain.Asset {
	return domain.Asset{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		Kind: value.Kind, IdentityKey: value.IdentityKey, Revision: value.Revision, ContentHash: value.ContentHash,
		CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt.UTC(),
	}
}

func productionWorldAssetStateRecord(value domain.AssetState) (model.AssetState, error) {
	if domain.ValidateAssetState(value) != nil {
		return model.AssetState{}, errors.New("invalid Production World AssetState")
	}
	id, _ := uuid.Parse(value.ID)
	workspaceID, _ := uuid.Parse(value.WorkspaceID)
	projectID, _ := uuid.Parse(value.ProjectID)
	assetID, _ := uuid.Parse(value.AssetID)
	createdBy, _ := uuid.Parse(value.CreatedBy)
	return model.AssetState{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, AssetID: assetID,
		StateKey: value.StateKey, Label: value.Label, Revision: value.Revision,
		Snapshot: datatypes.JSON(value.Snapshot), ContentHash: value.ContentHash,
		CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}

func productionWorldAssetStateDomain(value model.AssetState) (domain.AssetState, error) {
	state, err := domain.NewAssetState(domain.AssetStateInput{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		AssetID: value.AssetID.String(), StateKey: value.StateKey, Label: value.Label, Revision: value.Revision,
		Snapshot: json.RawMessage(value.Snapshot), CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt.UTC(),
	})
	if err != nil || state.ContentHash != value.ContentHash {
		return domain.AssetState{}, errors.New("persisted Production World AssetState has drifted")
	}
	return state, nil
}

func productionWorldIdentityStateHeadRecord(
	value domain.IdentityStateCollectionHead,
) (model.AssetIdentityStateScopeHead, error) {
	workspaceID, workspaceErr := uuid.Parse(value.WorkspaceID)
	projectID, projectErr := uuid.Parse(value.ProjectID)
	refs, refsErr := json.Marshal(value.Members)
	if workspaceErr != nil || projectErr != nil || refsErr != nil {
		return model.AssetIdentityStateScopeHead{}, errors.New("invalid Asset identity-state Head")
	}
	rebuilt, err := domain.NewIdentityStateCollectionHead(
		value.WorkspaceID, value.ProjectID, value.ScopeRevision, value.Members, value.UpdatedAt,
	)
	if err != nil || !reflect.DeepEqual(rebuilt, value) {
		return model.AssetIdentityStateScopeHead{}, errors.New("Asset identity-state Head has drifted")
	}
	return model.AssetIdentityStateScopeHead{
		ProjectID: projectID, WorkspaceID: workspaceID, ScopeRevision: value.ScopeRevision,
		ScopeContentHash: value.ScopeContentHash, MemberCount: value.MemberCount, MembersHash: value.MembersHash,
		CollectionRootHash: value.CollectionRootHash, CurrentRootRefs: datatypes.JSON(refs),
		HeadRevision: value.HeadRevision, HeadContentHash: value.HeadContentHash, UpdatedAt: value.UpdatedAt,
	}, nil
}
