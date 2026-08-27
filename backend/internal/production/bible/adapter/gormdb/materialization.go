package gormdb

import (
	"context"
	"errors"
	"reflect"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func (repo *repository) PrepareMaterialization(
	ctx context.Context,
	actor application.Actor,
	versionID string,
	lock bool,
) (application.MaterializationScope, error) {
	id, err := uuid.Parse(versionID)
	if err != nil {
		return application.MaterializationScope{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx)
	versionQuery := query.Where(&model.ProductionBibleVersion{ID: id})
	if lock {
		versionQuery = versionQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var versionRecord model.ProductionBibleVersion
	if err = versionQuery.First(&versionRecord).Error; err != nil {
		return application.MaterializationScope{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, versionRecord.ProjectID, lock); err != nil {
		return application.MaterializationScope{}, err
	}
	if lock {
		var project model.Project
		if err = query.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(&model.Project{ID: versionRecord.ProjectID}).First(&project).Error; err != nil {
			return application.MaterializationScope{}, normalizeNotFound(err)
		}
		if project.WorkspaceID != versionRecord.WorkspaceID {
			return application.MaterializationScope{}, confirmationConflict("Production Bible project has changed")
		}
	}
	version, err := productionBibleVersionDomain(versionRecord)
	if err != nil {
		return application.MaterializationScope{}, err
	}

	scope := application.MaterializationScope{
		Version:                 version,
		AssetsByEntityKey:       map[string]assetdomain.Asset{},
		SpecificationsByAssetID: map[string][]domain.SpecificationVersion{},
		StatesByAssetID:         map[string][]assetdomain.AssetState{},
		Bindings:                []domain.ProductionBinding{},
	}
	assetsByID := map[uuid.UUID]assetdomain.Asset{}
	var assetRecords []model.Asset
	if err = query.Where(&model.Asset{ProjectID: versionRecord.ProjectID}).Order("identity_key").Find(&assetRecords).Error; err != nil {
		return application.MaterializationScope{}, err
	}
	for _, record := range assetRecords {
		value, convertErr := assetDomain(record)
		if convertErr != nil || record.WorkspaceID != versionRecord.WorkspaceID {
			return application.MaterializationScope{}, confirmationConflict("Production Bible Asset identity has drifted")
		}
		if _, exists := scope.AssetsByEntityKey[value.IdentityKey]; exists {
			return application.MaterializationScope{}, confirmationConflict("Production Bible Asset identity is duplicated")
		}
		scope.AssetsByEntityKey[value.IdentityKey] = value
		assetsByID[record.ID] = value
	}

	specificationsByID := map[uuid.UUID]domain.SpecificationVersion{}
	var specificationRecords []model.ProductionBibleSpecificationVersion
	if err = query.Where(&model.ProductionBibleSpecificationVersion{ProjectID: versionRecord.ProjectID}).
		Order("asset_id, version").Find(&specificationRecords).Error; err != nil {
		return application.MaterializationScope{}, err
	}
	for _, record := range specificationRecords {
		value, convertErr := specificationDomain(record)
		asset, assetExists := assetsByID[record.AssetID]
		if convertErr != nil || !assetExists || asset.ProjectID != value.ProjectID || asset.WorkspaceID != value.WorkspaceID ||
			record.WorkspaceID != versionRecord.WorkspaceID {
			return application.MaterializationScope{}, confirmationConflict("Production Bible SpecificationVersion has drifted")
		}
		scope.SpecificationsByAssetID[value.AssetID] = append(scope.SpecificationsByAssetID[value.AssetID], value)
		specificationsByID[record.ID] = value
	}

	statesByID := map[uuid.UUID]assetdomain.AssetState{}
	var stateRecords []model.AssetState
	if err = query.Where(&model.AssetState{ProjectID: versionRecord.ProjectID}).
		Order("asset_id, state_key, revision").Find(&stateRecords).Error; err != nil {
		return application.MaterializationScope{}, err
	}
	for _, record := range stateRecords {
		value, convertErr := assetStateDomain(record)
		asset, assetExists := assetsByID[record.AssetID]
		if convertErr != nil || !assetExists || asset.ProjectID != value.ProjectID || asset.WorkspaceID != value.WorkspaceID ||
			record.WorkspaceID != versionRecord.WorkspaceID {
			return application.MaterializationScope{}, confirmationConflict("Production Bible AssetState has drifted")
		}
		scope.StatesByAssetID[value.AssetID] = append(scope.StatesByAssetID[value.AssetID], value)
		statesByID[record.ID] = value
	}

	var bindingRecords []model.ProductionBinding
	if err = query.Where(&model.ProductionBinding{BibleVersionID: versionRecord.ID}).Order("entity_key").
		Find(&bindingRecords).Error; err != nil {
		return application.MaterializationScope{}, err
	}
	for _, record := range bindingRecords {
		var stateLinks []model.ProductionBindingState
		if err = query.Where(&model.ProductionBindingState{ProductionBindingID: record.ID}).
			Order("position").Find(&stateLinks).Error; err != nil {
			return application.MaterializationScope{}, err
		}
		value, convertErr := productionBindingDomain(record, stateLinks, assetsByID, specificationsByID, statesByID)
		if convertErr != nil || value.WorkspaceID != version.WorkspaceID || value.ProjectID != version.ProjectID ||
			value.BibleVersionHash != version.ContentHash {
			return application.MaterializationScope{}, confirmationConflict("Production Bible binding has drifted")
		}
		scope.Bindings = append(scope.Bindings, value)
	}
	return scope, nil
}

func (repo *repository) CreateMaterialization(ctx context.Context, write application.MaterializationWrite) error {
	if err := validateMaterializationWrite(write); err != nil {
		return err
	}
	assetRecords := make([]model.Asset, 0, len(write.NewAssets))
	for _, value := range write.NewAssets {
		record, err := assetRecord(value)
		if err != nil {
			return err
		}
		assetRecords = append(assetRecords, record)
	}
	if len(assetRecords) > 0 {
		if err := repo.database.WithContext(ctx).Omit(clause.Associations).Create(&assetRecords).Error; err != nil {
			return normalizeMaterializationWriteError(err)
		}
	}

	specificationRecords := make([]model.ProductionBibleSpecificationVersion, 0, len(write.NewSpecifications))
	for _, value := range write.NewSpecifications {
		record, err := specificationRecord(value)
		if err != nil {
			return err
		}
		specificationRecords = append(specificationRecords, record)
	}
	if len(specificationRecords) > 0 {
		if err := repo.database.WithContext(ctx).Omit(clause.Associations).Create(&specificationRecords).Error; err != nil {
			return normalizeMaterializationWriteError(err)
		}
	}

	stateRecords := make([]model.AssetState, 0, len(write.NewStates))
	for _, value := range write.NewStates {
		record, err := assetStateRecord(value)
		if err != nil {
			return err
		}
		stateRecords = append(stateRecords, record)
	}
	if len(stateRecords) > 0 {
		if err := repo.database.WithContext(ctx).Omit(clause.Associations).Create(&stateRecords).Error; err != nil {
			return normalizeMaterializationWriteError(err)
		}
	}

	for _, value := range write.NewBindings {
		record, err := productionBindingRecord(value)
		if err != nil {
			return err
		}
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
			return normalizeMaterializationWriteError(err)
		}
		stateLinks := make([]model.ProductionBindingState, 0, len(value.States))
		for index, state := range value.States {
			stateID, parseErr := uuid.Parse(state.ID)
			if parseErr != nil {
				return parseErr
			}
			stateLinks = append(stateLinks, model.ProductionBindingState{
				ProductionBindingID: record.ID,
				AssetStateID:        stateID,
				Position:            index + 1,
			})
		}
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&stateLinks).Error; err != nil {
			return normalizeMaterializationWriteError(err)
		}
	}
	return nil
}

func (repo *repository) VerifyMaterialization(
	ctx context.Context,
	actor application.Actor,
	expected domain.Materialization,
) error {
	scope, err := repo.PrepareMaterialization(ctx, actor, expected.BibleVersionID, false)
	if err != nil {
		return err
	}
	assets := make([]domain.MaterializedAsset, 0, len(scope.Bindings))
	specifications := make([]domain.MaterializedSpecification, 0, len(scope.Bindings))
	states := make([]domain.MaterializedState, 0)
	bindings := make([]domain.MaterializedBinding, 0, len(scope.Bindings))
	for _, binding := range scope.Bindings {
		assets = append(assets, binding.Asset)
		specifications = append(specifications, binding.Specification)
		states = append(states, binding.States...)
		bindings = append(bindings, domain.MaterializedBindingRef(binding))
	}
	actual, err := domain.NewMaterialization(
		scope.Version.ID, scope.Version.ContentHash, assets, specifications, states, bindings,
	)
	if err != nil || !reflect.DeepEqual(actual, expected) {
		return confirmationConflict("Production Bible materialization has drifted")
	}
	return nil
}

func validateMaterializationWrite(write application.MaterializationWrite) error {
	rebuilt, err := domain.NewMaterialization(
		write.Materialization.BibleVersionID,
		write.Materialization.BibleVersionHash,
		write.Materialization.Assets,
		write.Materialization.Specifications,
		write.Materialization.States,
		write.Materialization.Bindings,
	)
	if err != nil || !reflect.DeepEqual(rebuilt, write.Materialization) {
		return confirmationConflict("Production Bible materialization write is invalid")
	}
	for _, value := range write.NewAssets {
		if assetdomain.ValidateAsset(value) != nil {
			return confirmationConflict("Production Bible Asset write is invalid")
		}
	}
	for _, value := range write.NewSpecifications {
		if domain.ValidateSpecificationVersion(value) != nil {
			return confirmationConflict("Production Bible SpecificationVersion write is invalid")
		}
	}
	for _, value := range write.NewStates {
		if assetdomain.ValidateAssetState(value) != nil {
			return confirmationConflict("Production Bible AssetState write is invalid")
		}
	}
	for _, value := range write.NewBindings {
		if domain.ValidateProductionBinding(value) != nil {
			return confirmationConflict("Production Bible binding write is invalid")
		}
	}
	return nil
}

func assetRecord(value assetdomain.Asset) (model.Asset, error) {
	if err := assetdomain.ValidateAsset(value); err != nil {
		return model.Asset{}, err
	}
	id, workspaceID, projectID, createdBy, err := materializationIDs(value.ID, value.WorkspaceID, value.ProjectID, value.CreatedBy)
	if err != nil {
		return model.Asset{}, err
	}
	return model.Asset{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID,
		Kind: value.Kind, IdentityKey: value.IdentityKey, Revision: value.Revision,
		ContentHash: value.ContentHash, CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}

func assetDomain(record model.Asset) (assetdomain.Asset, error) {
	value, err := assetdomain.NewAsset(assetdomain.AssetInput{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		Kind: record.Kind, IdentityKey: record.IdentityKey, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
	})
	if err != nil || value.Revision != record.Revision || value.ContentHash != record.ContentHash {
		return assetdomain.Asset{}, errors.New("persisted Asset has drifted")
	}
	return value, nil
}

func assetStateRecord(value assetdomain.AssetState) (model.AssetState, error) {
	if err := assetdomain.ValidateAssetState(value); err != nil {
		return model.AssetState{}, err
	}
	id, workspaceID, projectID, createdBy, err := materializationIDs(value.ID, value.WorkspaceID, value.ProjectID, value.CreatedBy)
	if err != nil {
		return model.AssetState{}, err
	}
	assetID, err := uuid.Parse(value.AssetID)
	if err != nil {
		return model.AssetState{}, err
	}
	return model.AssetState{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, AssetID: assetID,
		StateKey: value.StateKey, Label: value.Label, Revision: value.Revision,
		Snapshot: datatypes.JSON(value.Snapshot), ContentHash: value.ContentHash,
		CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}

func assetStateDomain(record model.AssetState) (assetdomain.AssetState, error) {
	value, err := assetdomain.NewAssetState(assetdomain.AssetStateInput{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		AssetID: record.AssetID.String(), StateKey: record.StateKey, Label: record.Label, Revision: record.Revision,
		Snapshot: append([]byte(nil), record.Snapshot...), CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
	})
	if err != nil || value.ContentHash != record.ContentHash {
		return assetdomain.AssetState{}, errors.New("persisted AssetState has drifted")
	}
	return value, nil
}

func specificationRecord(value domain.SpecificationVersion) (model.ProductionBibleSpecificationVersion, error) {
	if err := domain.ValidateSpecificationVersion(value); err != nil {
		return model.ProductionBibleSpecificationVersion{}, err
	}
	id, workspaceID, projectID, createdBy, err := materializationIDs(value.ID, value.WorkspaceID, value.ProjectID, value.CreatedBy)
	if err != nil {
		return model.ProductionBibleSpecificationVersion{}, err
	}
	assetID, err := uuid.Parse(value.AssetID)
	if err != nil {
		return model.ProductionBibleSpecificationVersion{}, err
	}
	sourceID, err := uuid.Parse(value.SourceBibleVersionID)
	if err != nil {
		return model.ProductionBibleSpecificationVersion{}, err
	}
	return model.ProductionBibleSpecificationVersion{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, AssetID: assetID,
		Kind: value.Kind, EntityKey: value.EntityKey, Version: value.Version, SourceBibleVersionID: sourceID,
		Snapshot: datatypes.JSON(value.Snapshot), ContentHash: value.ContentHash,
		CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}

func specificationDomain(record model.ProductionBibleSpecificationVersion) (domain.SpecificationVersion, error) {
	value, err := domain.NewSpecificationVersion(domain.SpecificationVersionInput{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		AssetID: record.AssetID.String(), Kind: record.Kind, EntityKey: record.EntityKey, Version: record.Version,
		SourceBibleVersionID: record.SourceBibleVersionID.String(), Snapshot: append([]byte(nil), record.Snapshot...),
		CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
	})
	if err != nil || value.ContentHash != record.ContentHash {
		return domain.SpecificationVersion{}, errors.New("persisted Production Bible SpecificationVersion has drifted")
	}
	return value, nil
}

func productionBindingRecord(value domain.ProductionBinding) (model.ProductionBinding, error) {
	if err := domain.ValidateProductionBinding(value); err != nil {
		return model.ProductionBinding{}, err
	}
	id, workspaceID, projectID, createdBy, err := materializationIDs(value.ID, value.WorkspaceID, value.ProjectID, value.CreatedBy)
	if err != nil {
		return model.ProductionBinding{}, err
	}
	bibleVersionID, err := uuid.Parse(value.BibleVersionID)
	if err != nil {
		return model.ProductionBinding{}, err
	}
	assetID, err := uuid.Parse(value.Asset.ID)
	if err != nil {
		return model.ProductionBinding{}, err
	}
	specificationID, err := uuid.Parse(value.Specification.ID)
	if err != nil {
		return model.ProductionBinding{}, err
	}
	return model.ProductionBinding{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID,
		BibleVersionID: bibleVersionID, BibleVersionHash: value.BibleVersionHash, EntityKey: value.EntityKey,
		AssetID: assetID, SpecificationVersionID: specificationID, Revision: value.Revision,
		ContentHash: value.ContentHash, CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}

func productionBindingDomain(
	record model.ProductionBinding,
	stateLinks []model.ProductionBindingState,
	assets map[uuid.UUID]assetdomain.Asset,
	specifications map[uuid.UUID]domain.SpecificationVersion,
	states map[uuid.UUID]assetdomain.AssetState,
) (domain.ProductionBinding, error) {
	asset, assetExists := assets[record.AssetID]
	specification, specificationExists := specifications[record.SpecificationVersionID]
	if !assetExists || !specificationExists || len(stateLinks) == 0 {
		return domain.ProductionBinding{}, errors.New("persisted ProductionBinding references are incomplete")
	}
	stateRefs := make([]domain.MaterializedState, 0, len(stateLinks))
	for index, link := range stateLinks {
		state, exists := states[link.AssetStateID]
		if !exists || link.Position != index+1 || link.ProductionBindingID != record.ID {
			return domain.ProductionBinding{}, errors.New("persisted ProductionBinding State references have drifted")
		}
		stateRefs = append(stateRefs, domain.MaterializedStateRef(state))
	}
	value, err := domain.NewProductionBinding(domain.ProductionBindingInput{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		BibleVersionID: record.BibleVersionID.String(), BibleVersionHash: record.BibleVersionHash,
		EntityKey: record.EntityKey, Asset: domain.MaterializedAssetRef(asset),
		Specification: domain.MaterializedSpecificationRef(specification), States: stateRefs,
		CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
	})
	if err != nil || value.Revision != record.Revision || value.ContentHash != record.ContentHash {
		return domain.ProductionBinding{}, errors.New("persisted ProductionBinding has drifted")
	}
	return value, nil
}

func materializationIDs(idValue, workspaceValue, projectValue, actorValue string) (
	id uuid.UUID,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	actorID uuid.UUID,
	err error,
) {
	if id, err = uuid.Parse(idValue); err != nil {
		return
	}
	if workspaceID, err = uuid.Parse(workspaceValue); err != nil {
		return
	}
	if projectID, err = uuid.Parse(projectValue); err != nil {
		return
	}
	actorID, err = uuid.Parse(actorValue)
	return
}

func normalizeMaterializationWriteError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return confirmationConflict("Production Bible materialization already exists")
	}
	return err
}
