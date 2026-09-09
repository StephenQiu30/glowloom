package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func NewProductionWorldRepository(database *gorm.DB) application.ProductionWorldBibleRepository {
	return &repository{database: database}
}

func (repo *repository) GetProductionWorldBibleHead(ctx context.Context, workspaceID, projectID string, lock bool) (domain.ProductionWorldBibleHead, domain.ProductionWorldBibleVersion, error) {
	workspaceUUID, workspaceErr := uuid.Parse(workspaceID)
	projectUUID, projectErr := uuid.Parse(projectID)
	if workspaceErr != nil || projectErr != nil {
		return domain.ProductionWorldBibleHead{}, domain.ProductionWorldBibleVersion{}, application.ErrProductionWorldBibleHeadNotFound
	}
	query := repo.database.WithContext(ctx).Where("project_id = ?", projectUUID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var headRecord model.ProductionWorldBibleScopeHead
	if err := query.First(&headRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ProductionWorldBibleHead{}, domain.ProductionWorldBibleVersion{}, application.ErrProductionWorldBibleHeadNotFound
		}
		return domain.ProductionWorldBibleHead{}, domain.ProductionWorldBibleVersion{}, err
	}
	if headRecord.WorkspaceID != workspaceUUID {
		return domain.ProductionWorldBibleHead{}, domain.ProductionWorldBibleVersion{}, errors.New("Production World Bible Head workspace has drifted")
	}
	var versionRecord model.ProductionWorldBibleVersion
	if err := repo.database.WithContext(ctx).Where("id = ?", headRecord.CurrentVersionID).First(&versionRecord).Error; err != nil {
		return domain.ProductionWorldBibleHead{}, domain.ProductionWorldBibleVersion{}, err
	}
	version, err := productionWorldBibleVersionDomain(versionRecord)
	if err != nil {
		return domain.ProductionWorldBibleHead{}, domain.ProductionWorldBibleVersion{}, err
	}
	if err = repo.verifyProductionWorldBibleRefs(ctx, version); err != nil {
		return domain.ProductionWorldBibleHead{}, domain.ProductionWorldBibleVersion{}, err
	}
	head, err := domain.NewProductionWorldBibleHead(workspaceID, projectID, headRecord.HeadRevision, version, headRecord.UpdatedAt)
	if err != nil || head.CurrentVersionID != headRecord.CurrentVersionID.String() || head.VersionContentHash != headRecord.VersionContentHash || head.HeadContentHash != headRecord.HeadContentHash {
		return domain.ProductionWorldBibleHead{}, domain.ProductionWorldBibleVersion{}, errors.New("Production World Bible Head has drifted")
	}
	return head, version, nil
}

func (repo *repository) ListProductionWorldEvidence(ctx context.Context, projectID, subjectKey string, lock bool) ([]domain.ProductionWorldEvidence, error) {
	var records []model.ProductionWorldEvidence
	query := repo.database.WithContext(ctx).Where("project_id = ? AND subject_key = ?", projectID, subjectKey).Order("revision")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.ProductionWorldEvidence, len(records))
	for i, record := range records {
		value, err := productionWorldEvidenceDomain(record)
		if err != nil {
			return nil, err
		}
		result[i] = value
	}
	return result, nil
}

func (repo *repository) ListProductionWorldSpecifications(ctx context.Context, projectID, key string, lock bool) ([]domain.ProductionWorldSpecification, error) {
	var records []model.ProductionWorldSpecification
	query := repo.database.WithContext(ctx).Where("project_id = ? AND specification_key = ?", projectID, key).Order("revision")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.ProductionWorldSpecification, len(records))
	for i, record := range records {
		var evidence model.ProductionWorldEvidence
		if err := repo.database.WithContext(ctx).Where("id = ?", record.EvidenceID).First(&evidence).Error; err != nil {
			return nil, err
		}
		value, err := productionWorldSpecificationDomain(record, evidence)
		if err != nil {
			return nil, err
		}
		var asset model.Asset
		if err = repo.database.WithContext(ctx).Where("id = ?", record.AssetID).First(&asset).Error; err != nil ||
			asset.ProjectID != record.ProjectID || asset.IdentityKey != record.IdentityKey || asset.Kind != record.Kind || asset.ContentHash != record.AssetContentHash {
			return nil, errors.New("Production World Specification Asset has drifted")
		}
		result[i] = value
	}
	return result, nil
}

func (repo *repository) ListProductionWorldClaims(ctx context.Context, projectID, key string, lock bool) ([]domain.ProductionWorldClaim, error) {
	var records []model.ProductionWorldClaim
	query := repo.database.WithContext(ctx).Where("project_id = ? AND claim_key = ?", projectID, key).Order("revision")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.ProductionWorldClaim, len(records))
	for i, record := range records {
		var evidence model.ProductionWorldEvidence
		if err := repo.database.WithContext(ctx).Where("id = ?", record.EvidenceID).First(&evidence).Error; err != nil {
			return nil, err
		}
		value, err := productionWorldClaimDomain(record, evidence)
		if err != nil {
			return nil, err
		}
		for _, subject := range value.Subjects {
			var asset model.Asset
			if err = repo.database.WithContext(ctx).Where("id = ?", subject.AssetID).First(&asset).Error; err != nil ||
				asset.ProjectID != record.ProjectID || asset.IdentityKey != subject.IdentityKey || asset.ContentHash != subject.AssetContentHash {
				return nil, errors.New("Production World Claim subject has drifted")
			}
		}
		result[i] = value
	}
	return result, nil
}

func (repo *repository) ListProductionWorldBindings(ctx context.Context, projectID, key string, lock bool) ([]domain.ProductionWorldBinding, error) {
	var records []model.ProductionWorldBinding
	query := repo.database.WithContext(ctx).Where("project_id = ? AND identity_key = ?", projectID, key).Order("revision")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.ProductionWorldBinding, len(records))
	for i, record := range records {
		var specification model.ProductionWorldSpecification
		if err := repo.database.WithContext(ctx).Where("id = ?", record.SpecificationID).First(&specification).Error; err != nil {
			return nil, err
		}
		var links []model.ProductionWorldBindingState
		if err := repo.database.WithContext(ctx).Where("binding_id = ?", record.ID).Order("position").Find(&links).Error; err != nil {
			return nil, err
		}
		for _, link := range links {
			var state model.AssetState
			if err := repo.database.WithContext(ctx).Where("id = ?", link.AssetStateID).First(&state).Error; err != nil ||
				state.ProjectID != record.ProjectID || state.AssetID != record.AssetID || state.StateKey != link.StateKey ||
				state.Revision != link.Revision || state.ContentHash != link.ContentHash {
				return nil, errors.New("Production World Binding AssetState has drifted")
			}
		}
		value, err := productionWorldBindingDomain(record, specification, links)
		if err != nil {
			return nil, err
		}
		result[i] = value
	}
	return result, nil
}

func (repo *repository) CreateProductionWorldEvidence(ctx context.Context, values []domain.ProductionWorldEvidence) error {
	records := make([]model.ProductionWorldEvidence, len(values))
	for i, value := range values {
		record, err := productionWorldEvidenceRecord(value)
		if err != nil {
			return err
		}
		records[i] = record
	}
	return createProductionWorldRecords(ctx, repo.database, records)
}
func (repo *repository) CreateProductionWorldSpecifications(ctx context.Context, values []domain.ProductionWorldSpecification) error {
	records := make([]model.ProductionWorldSpecification, len(values))
	for i, value := range values {
		record, err := productionWorldSpecificationRecord(value)
		if err != nil {
			return err
		}
		records[i] = record
	}
	return createProductionWorldRecords(ctx, repo.database, records)
}
func (repo *repository) CreateProductionWorldClaims(ctx context.Context, values []domain.ProductionWorldClaim) error {
	records := make([]model.ProductionWorldClaim, len(values))
	for i, value := range values {
		record, err := productionWorldClaimRecord(value)
		if err != nil {
			return err
		}
		records[i] = record
	}
	return createProductionWorldRecords(ctx, repo.database, records)
}

func (repo *repository) CreateProductionWorldBindings(ctx context.Context, values []domain.ProductionWorldBinding) error {
	for _, value := range values {
		record, links, err := productionWorldBindingRecord(value)
		if err != nil {
			return err
		}
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
			return err
		}
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&links).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repo *repository) CreateProductionWorldBibleVersion(ctx context.Context, value domain.ProductionWorldBibleVersion) error {
	record, err := productionWorldBibleVersionRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) SaveProductionWorldBibleHead(ctx context.Context, value domain.ProductionWorldBibleHead, expectedRevision int64, expectedHash string) error {
	record, err := productionWorldBibleHeadRecord(value)
	if err != nil {
		return err
	}
	if expectedRevision == 0 {
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return application.ErrProductionWorldBibleConflict
			}
			return err
		}
		return nil
	}
	updated := repo.database.WithContext(ctx).Model(&model.ProductionWorldBibleScopeHead{}).Where("project_id = ? AND workspace_id = ? AND head_revision = ? AND head_content_hash = ?", record.ProjectID, record.WorkspaceID, expectedRevision, expectedHash).Select("current_version_id", "head_revision", "version_content_hash", "head_content_hash", "updated_at").Updates(&record)
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return application.ErrProductionWorldBibleConflict
	}
	return nil
}

func createProductionWorldRecords[T any](ctx context.Context, database *gorm.DB, records []T) error {
	if len(records) == 0 {
		return nil
	}
	return database.WithContext(ctx).Omit(clause.Associations).Create(&records).Error
}

func productionWorldEvidenceRecord(value domain.ProductionWorldEvidence) (model.ProductionWorldEvidence, error) {
	rebuilt, err := domain.NewProductionWorldEvidence(value.ID, value.WorkspaceID, value.ProjectID, value.SubjectKey, value.Revision, value.Basis, value.CreatedBy, value.CreatedAt)
	if err != nil || !reflect.DeepEqual(rebuilt, value) {
		return model.ProductionWorldEvidence{}, errors.New("Production World Evidence has drifted")
	}
	ids, err := productionWorldIDs(value.ID, value.WorkspaceID, value.ProjectID, value.CreatedBy)
	if err != nil {
		return model.ProductionWorldEvidence{}, err
	}
	return model.ProductionWorldEvidence{ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], CreatedBy: ids[3], SubjectKey: value.SubjectKey, Revision: value.Revision, Basis: datatypes.JSON(value.Basis), ContentHash: value.ContentHash, CreatedAt: value.CreatedAt}, nil
}
func productionWorldEvidenceDomain(record model.ProductionWorldEvidence) (domain.ProductionWorldEvidence, error) {
	value, err := domain.NewProductionWorldEvidence(record.ID.String(), record.WorkspaceID.String(), record.ProjectID.String(), record.SubjectKey, record.Revision, json.RawMessage(record.Basis), record.CreatedBy.String(), record.CreatedAt)
	if err != nil || value.ContentHash != record.ContentHash {
		return domain.ProductionWorldEvidence{}, errors.New("persisted Production World Evidence has drifted")
	}
	return value, nil
}

func productionWorldSpecificationRecord(value domain.ProductionWorldSpecification) (model.ProductionWorldSpecification, error) {
	ids, err := productionWorldIDs(value.ID, value.WorkspaceID, value.ProjectID, value.Asset.ID, value.Evidence.ID, value.CreatedBy)
	if err != nil {
		return model.ProductionWorldSpecification{}, err
	}
	rebuilt, err := domain.NewProductionWorldSpecification(value.ID, value.WorkspaceID, value.ProjectID, value.SpecificationKey, value.IdentityKey, value.Kind, value.Revision, value.Asset, value.Evidence, value.Slots, value.CreatedBy, value.CreatedAt)
	if err != nil || !reflect.DeepEqual(rebuilt, value) {
		return model.ProductionWorldSpecification{}, errors.New("Production World Specification has drifted")
	}
	return model.ProductionWorldSpecification{ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], AssetID: ids[3], EvidenceID: ids[4], CreatedBy: ids[5], SpecificationKey: value.SpecificationKey, IdentityKey: value.IdentityKey, Kind: value.Kind, Revision: value.Revision, AssetContentHash: value.Asset.ContentHash, EvidenceHash: value.Evidence.ContentHash, Slots: datatypes.JSON(value.Slots), ContentHash: value.ContentHash, CreatedAt: value.CreatedAt}, nil
}
func productionWorldSpecificationDomain(record model.ProductionWorldSpecification, evidence model.ProductionWorldEvidence) (domain.ProductionWorldSpecification, error) {
	value, err := domain.NewProductionWorldSpecification(record.ID.String(), record.WorkspaceID.String(), record.ProjectID.String(), record.SpecificationKey, record.IdentityKey, record.Kind, record.Revision, domain.FragmentRef(record.AssetID.String(), "asset_identity", record.IdentityKey, record.AssetContentHash, 1), domain.FragmentRef(evidence.ID.String(), "source_evidence", evidence.SubjectKey, evidence.ContentHash, evidence.Revision), json.RawMessage(record.Slots), record.CreatedBy.String(), record.CreatedAt)
	if err != nil || value.ContentHash != record.ContentHash || record.EvidenceHash != evidence.ContentHash {
		return domain.ProductionWorldSpecification{}, errors.New("persisted Production World Specification has drifted")
	}
	return value, nil
}

func productionWorldClaimRecord(value domain.ProductionWorldClaim) (model.ProductionWorldClaim, error) {
	ids, err := productionWorldIDs(value.ID, value.WorkspaceID, value.ProjectID, value.Evidence.ID, value.CreatedBy)
	subjects, jsonErr := json.Marshal(value.Subjects)
	if err != nil || jsonErr != nil {
		return model.ProductionWorldClaim{}, errors.New("invalid Production World Claim record")
	}
	rebuilt, rebuildErr := domain.NewProductionWorldClaim(value.ID, value.WorkspaceID, value.ProjectID, value.ClaimKey, value.ClaimType, value.Statement, value.Revision, value.Subjects, value.Evidence, value.CreatedBy, value.CreatedAt)
	if rebuildErr != nil || !reflect.DeepEqual(rebuilt, value) {
		return model.ProductionWorldClaim{}, errors.New("Production World Claim has drifted")
	}
	return model.ProductionWorldClaim{ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], EvidenceID: ids[3], CreatedBy: ids[4], ClaimKey: value.ClaimKey, ClaimType: value.ClaimType, Statement: value.Statement, Revision: value.Revision, Subjects: datatypes.JSON(subjects), EvidenceHash: value.Evidence.ContentHash, ContentHash: value.ContentHash, CreatedAt: value.CreatedAt}, nil
}
func productionWorldClaimDomain(record model.ProductionWorldClaim, evidence model.ProductionWorldEvidence) (domain.ProductionWorldClaim, error) {
	var subjects []domain.ProductionWorldClaimSubject
	if err := json.Unmarshal(record.Subjects, &subjects); err != nil {
		return domain.ProductionWorldClaim{}, err
	}
	value, err := domain.NewProductionWorldClaim(record.ID.String(), record.WorkspaceID.String(), record.ProjectID.String(), record.ClaimKey, record.ClaimType, record.Statement, record.Revision, subjects, domain.FragmentRef(evidence.ID.String(), "source_evidence", evidence.SubjectKey, evidence.ContentHash, evidence.Revision), record.CreatedBy.String(), record.CreatedAt)
	if err != nil || value.ContentHash != record.ContentHash || record.EvidenceHash != evidence.ContentHash {
		return domain.ProductionWorldClaim{}, errors.New("persisted Production World Claim has drifted")
	}
	return value, nil
}

func productionWorldBindingRecord(value domain.ProductionWorldBinding) (model.ProductionWorldBinding, []model.ProductionWorldBindingState, error) {
	ids, err := productionWorldIDs(value.ID, value.WorkspaceID, value.ProjectID, value.Asset.ID, value.Specification.ID, value.CreatedBy)
	if err != nil {
		return model.ProductionWorldBinding{}, nil, err
	}
	rebuilt, err := domain.NewProductionWorldBinding(value.ID, value.WorkspaceID, value.ProjectID, value.IdentityKey, value.Revision, value.Asset, value.Specification, value.States, value.CreatedBy, value.CreatedAt)
	if err != nil || !reflect.DeepEqual(rebuilt, value) {
		return model.ProductionWorldBinding{}, nil, errors.New("Production World Binding has drifted")
	}
	record := model.ProductionWorldBinding{ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], AssetID: ids[3], SpecificationID: ids[4], CreatedBy: ids[5], IdentityKey: value.IdentityKey, Revision: value.Revision, AssetContentHash: value.Asset.ContentHash, SpecificationContentHash: value.Specification.ContentHash, ContentHash: value.ContentHash, CreatedAt: value.CreatedAt}
	links := make([]model.ProductionWorldBindingState, len(value.States))
	for i, state := range value.States {
		stateID, _ := uuid.Parse(state.ID)
		links[i] = model.ProductionWorldBindingState{BindingID: record.ID, AssetStateID: stateID, Position: i + 1, StateKey: state.StateKey, Revision: state.Revision, ContentHash: state.ContentHash}
	}
	return record, links, nil
}
func productionWorldBindingDomain(record model.ProductionWorldBinding, specification model.ProductionWorldSpecification, links []model.ProductionWorldBindingState) (domain.ProductionWorldBinding, error) {
	states := make([]domain.ProductionWorldStateRef, len(links))
	for i, link := range links {
		if link.BindingID != record.ID || link.Position != i+1 {
			return domain.ProductionWorldBinding{}, errors.New("Production World Binding state has drifted")
		}
		states[i] = domain.ProductionWorldStateRef{ID: link.AssetStateID.String(), AssetID: record.AssetID.String(), StateKey: link.StateKey, Revision: link.Revision, ContentHash: link.ContentHash}
	}
	value, err := domain.NewProductionWorldBinding(record.ID.String(), record.WorkspaceID.String(), record.ProjectID.String(), record.IdentityKey, record.Revision, domain.FragmentRef(record.AssetID.String(), "asset_identity", record.IdentityKey, record.AssetContentHash, 1), domain.FragmentRef(specification.ID.String(), "specification", specification.SpecificationKey, specification.ContentHash, specification.Revision), states, record.CreatedBy.String(), record.CreatedAt)
	if err != nil || value.ContentHash != record.ContentHash || record.SpecificationContentHash != specification.ContentHash {
		return domain.ProductionWorldBinding{}, errors.New("persisted Production World Binding has drifted")
	}
	return value, nil
}

func productionWorldBibleVersionRecord(value domain.ProductionWorldBibleVersion) (model.ProductionWorldBibleVersion, error) {
	rebuilt, err := domain.NewProductionWorldBibleVersion(value)
	if err != nil || !reflect.DeepEqual(rebuilt, value) {
		return model.ProductionWorldBibleVersion{}, errors.New("Production World Bible has drifted")
	}
	ids, err := productionWorldIDs(value.ID, value.WorkspaceID, value.ProjectID, value.StructureIdentitySet.VersionID, value.Candidate.VersionID, value.ReviewDecisionID, value.CreatedBy)
	if err != nil {
		return model.ProductionWorldBibleVersion{}, err
	}
	evidence, _ := json.Marshal(value.Evidence)
	specifications, _ := json.Marshal(value.Specifications)
	claims, _ := json.Marshal(value.Claims)
	bindings, _ := json.Marshal(value.Bindings)
	return model.ProductionWorldBibleVersion{ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], StructureIdentitySetVersionID: ids[3], CandidateRevisionID: ids[4], ReviewDecisionID: ids[5], CreatedBy: ids[6], Revision: value.Revision, StructureIdentitySetRevision: value.StructureIdentitySet.Revision, CandidateRevisionNo: value.Candidate.Revision, StructureIdentitySetHash: value.StructureIdentitySet.ContentHash, CandidateRevisionHash: value.Candidate.ContentHash, PartitionHash: value.PartitionHash, BusinessKeyRoot: value.BusinessKeyRoot, EvidenceRefs: datatypes.JSON(evidence), SpecificationRefs: datatypes.JSON(specifications), ClaimRefs: datatypes.JSON(claims), BindingRefs: datatypes.JSON(bindings), ContentHash: value.ContentHash, CreatedAt: value.CreatedAt}, nil
}
func productionWorldBibleVersionDomain(record model.ProductionWorldBibleVersion) (domain.ProductionWorldBibleVersion, error) {
	var evidence, specifications, claims, bindings []domain.ProductionWorldFragmentRef
	if json.Unmarshal(record.EvidenceRefs, &evidence) != nil || json.Unmarshal(record.SpecificationRefs, &specifications) != nil || json.Unmarshal(record.ClaimRefs, &claims) != nil || json.Unmarshal(record.BindingRefs, &bindings) != nil {
		return domain.ProductionWorldBibleVersion{}, errors.New("Production World Bible refs have drifted")
	}
	value, err := domain.NewProductionWorldBibleVersion(domain.ProductionWorldBibleVersion{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(), Revision: record.Revision, StructureIdentitySet: domain.ProductionWorldOwnerRef{OwnerKind: "production/bible", LogicalID: record.ProjectID.String(), VersionID: record.StructureIdentitySetVersionID.String(), Revision: record.StructureIdentitySetRevision, ContentHash: record.StructureIdentitySetHash}, Candidate: domain.ProductionWorldOwnerRef{OwnerKind: "agent", LogicalID: record.ProjectID.String(), VersionID: record.CandidateRevisionID.String(), Revision: record.CandidateRevisionNo, ContentHash: record.CandidateRevisionHash}, ReviewDecisionID: record.ReviewDecisionID.String(), PartitionHash: record.PartitionHash, BusinessKeyRoot: record.BusinessKeyRoot, Evidence: evidence, Specifications: specifications, Claims: claims, Bindings: bindings, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt})
	if err != nil || value.ContentHash != record.ContentHash {
		return domain.ProductionWorldBibleVersion{}, errors.New("persisted Production World Bible has drifted")
	}
	return value, nil
}
func productionWorldBibleHeadRecord(value domain.ProductionWorldBibleHead) (model.ProductionWorldBibleScopeHead, error) {
	ids, err := productionWorldIDs(value.ProjectID, value.WorkspaceID, value.CurrentVersionID)
	if err != nil {
		return model.ProductionWorldBibleScopeHead{}, err
	}
	return model.ProductionWorldBibleScopeHead{ProjectID: ids[0], WorkspaceID: ids[1], CurrentVersionID: ids[2], HeadRevision: value.HeadRevision, VersionContentHash: value.VersionContentHash, HeadContentHash: value.HeadContentHash, UpdatedAt: value.UpdatedAt}, nil
}
func productionWorldIDs(values ...string) ([]uuid.UUID, error) {
	result := make([]uuid.UUID, len(values))
	for i, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, err
		}
		result[i] = id
	}
	return result, nil
}

func (repo *repository) verifyProductionWorldBibleRefs(ctx context.Context, version domain.ProductionWorldBibleVersion) error {
	checks := []struct {
		refs []domain.ProductionWorldFragmentRef
		load func(domain.ProductionWorldFragmentRef) ([]domain.ProductionWorldFragmentRef, error)
	}{
		{version.Evidence, func(ref domain.ProductionWorldFragmentRef) ([]domain.ProductionWorldFragmentRef, error) {
			values, err := repo.ListProductionWorldEvidence(ctx, version.ProjectID, ref.BusinessKey, false)
			return productionWorldEvidenceReferenceSet(values), err
		}},
		{version.Specifications, func(ref domain.ProductionWorldFragmentRef) ([]domain.ProductionWorldFragmentRef, error) {
			values, err := repo.ListProductionWorldSpecifications(ctx, version.ProjectID, ref.BusinessKey, false)
			return productionWorldSpecificationReferenceSet(values), err
		}},
		{version.Claims, func(ref domain.ProductionWorldFragmentRef) ([]domain.ProductionWorldFragmentRef, error) {
			values, err := repo.ListProductionWorldClaims(ctx, version.ProjectID, ref.BusinessKey, false)
			return productionWorldClaimReferenceSet(values), err
		}},
		{version.Bindings, func(ref domain.ProductionWorldFragmentRef) ([]domain.ProductionWorldFragmentRef, error) {
			values, err := repo.ListProductionWorldBindings(ctx, version.ProjectID, ref.BusinessKey, false)
			return productionWorldBindingReferenceSet(values), err
		}},
	}
	for _, check := range checks {
		for _, expected := range check.refs {
			actual, err := check.load(expected)
			if err != nil || !slices.Contains(actual, expected) {
				return errors.New("Production World Bible fragment refs have drifted")
			}
		}
	}
	return nil
}

func productionWorldEvidenceReferenceSet(values []domain.ProductionWorldEvidence) []domain.ProductionWorldFragmentRef {
	result := make([]domain.ProductionWorldFragmentRef, len(values))
	for index, value := range values {
		result[index] = domain.FragmentRef(value.ID, "source_evidence", value.SubjectKey, value.ContentHash, value.Revision)
	}
	return result
}
func productionWorldSpecificationReferenceSet(values []domain.ProductionWorldSpecification) []domain.ProductionWorldFragmentRef {
	result := make([]domain.ProductionWorldFragmentRef, len(values))
	for index, value := range values {
		result[index] = domain.FragmentRef(value.ID, "specification", value.SpecificationKey, value.ContentHash, value.Revision)
	}
	return result
}
func productionWorldClaimReferenceSet(values []domain.ProductionWorldClaim) []domain.ProductionWorldFragmentRef {
	result := make([]domain.ProductionWorldFragmentRef, len(values))
	for index, value := range values {
		result[index] = domain.FragmentRef(value.ID, value.ClaimType, value.ClaimKey, value.ContentHash, value.Revision)
	}
	return result
}
func productionWorldBindingReferenceSet(values []domain.ProductionWorldBinding) []domain.ProductionWorldFragmentRef {
	result := make([]domain.ProductionWorldFragmentRef, len(values))
	for index, value := range values {
		result[index] = domain.FragmentRef(value.ID, "production_binding", value.IdentityKey, value.ContentHash, value.Revision)
	}
	return result
}
