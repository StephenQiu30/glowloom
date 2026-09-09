package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

func NewProductionWorldRepository(database *gorm.DB) application.ProductionWorldPlanningRepository {
	return &repository{database: database}
}

func (repo *repository) GetProductionWorldPlanningHead(
	ctx context.Context,
	workspaceID, projectID, episodeID string,
	lock bool,
) (domain.ProductionWorldPlanningEpisodeHead, error) {
	ids, err := planningIDs(workspaceID, projectID, episodeID)
	if err != nil {
		return domain.ProductionWorldPlanningEpisodeHead{}, application.ErrProductionWorldPlanningHeadNotFound
	}
	query := repo.database.WithContext(ctx).Where("episode_id = ?", ids[2])
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.ProductionWorldPlanningEpisodeHead
	if err = query.First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ProductionWorldPlanningEpisodeHead{}, application.ErrProductionWorldPlanningHeadNotFound
		}
		return domain.ProductionWorldPlanningEpisodeHead{}, err
	}
	if record.WorkspaceID != ids[0] || record.ProjectID != ids[1] {
		return domain.ProductionWorldPlanningEpisodeHead{}, errors.New("Production World Planning Head ownership drifted")
	}
	var memberships []model.ProductionWorldPlanningMembership
	if err = repo.database.WithContext(ctx).
		Where("episode_id = ? AND scope_revision = ?", record.EpisodeID, record.ScopeRevision).
		Order("position ASC").Find(&memberships).Error; err != nil {
		return domain.ProductionWorldPlanningEpisodeHead{}, err
	}
	if len(memberships) != record.MemberCount {
		return domain.ProductionWorldPlanningEpisodeHead{}, errors.New("Production World Planning membership coverage drifted")
	}
	facts := make([]domain.ProductionWorldPlanningFact, len(memberships))
	for index, membership := range memberships {
		fact, loadErr := repo.loadProductionWorldPlanningFact(ctx, membership.FactKind, membership.FactID)
		if loadErr != nil || membership.Position != index+1 || membership.FactID.String() != fact.ID ||
			membership.BusinessKey != fact.BusinessKey || membership.FactRevision != fact.Revision ||
			membership.FactContentHash != fact.ContentHash {
			return domain.ProductionWorldPlanningEpisodeHead{}, errors.New("Production World Planning membership fact drifted")
		}
		if verifyErr := repo.verifyProductionWorldPlanningFact(ctx, fact); verifyErr != nil {
			return domain.ProductionWorldPlanningEpisodeHead{}, verifyErr
		}
		member := domain.ProductionWorldPlanningMember{
			Position: membership.Position,
			Fact:     domain.PlanningFactRef(fact), MemberHash: membership.MemberHash,
		}
		if domain.ValidateProductionWorldPlanningMember(member) != nil {
			return domain.ProductionWorldPlanningEpisodeHead{}, errors.New("Production World Planning membership hash drifted")
		}
		facts[index] = fact
	}
	head, err := domain.NewProductionWorldPlanningEpisodeHead(
		workspaceID, projectID, episodeID, record.ScopeRevision, facts, record.UpdatedAt,
	)
	var rootRefs []domain.ProductionWorldPlanningFactRef
	rootRefErr := json.Unmarshal(record.CurrentRootRefs, &rootRefs)
	expectedRootRefs := make([]domain.ProductionWorldPlanningFactRef, len(head.Members))
	for index, member := range head.Members {
		expectedRootRefs[index] = member.Fact
	}
	if err != nil || head.HeadRevision != record.HeadRevision || head.MemberCount != record.MemberCount ||
		rootRefErr != nil || !reflect.DeepEqual(rootRefs, expectedRootRefs) ||
		head.ScopeContentHash != record.ScopeContentHash || head.MembersHash != record.MembersHash || head.CollectionRootHash != record.CollectionRootHash ||
		head.HeadContentHash != record.HeadContentHash {
		return domain.ProductionWorldPlanningEpisodeHead{}, errors.New("Production World Planning Head has drifted")
	}
	return head, nil
}

func (repo *repository) ListProductionWorldPlanningFacts(
	ctx context.Context,
	projectID, kind, businessKey string,
	lock bool,
) ([]domain.ProductionWorldPlanningFact, error) {
	query := repo.database.WithContext(ctx).Where("project_id = ? AND business_key = ?", projectID, businessKey).Order("revision ASC")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	result := []domain.ProductionWorldPlanningFact{}
	switch kind {
	case "scene":
		var records []model.ProductionWorldPlanningScene
		if err := query.Find(&records).Error; err != nil {
			return nil, err
		}
		for _, record := range records {
			result = append(result, planningFactFromRecord(record.ID, record.WorkspaceID, record.ProjectID, record.EpisodeID, kind, record.BusinessKey, record.Revision, record.Payload, record.ContentHash, record.CreatedBy, record.CreatedAt))
		}
	case "dialogue":
		var records []model.ProductionWorldPlanningDialogue
		if err := query.Find(&records).Error; err != nil {
			return nil, err
		}
		for _, record := range records {
			result = append(result, planningFactFromRecord(record.ID, record.WorkspaceID, record.ProjectID, record.EpisodeID, kind, record.BusinessKey, record.Revision, record.Payload, record.ContentHash, record.CreatedBy, record.CreatedAt))
		}
	case "narrative_beat":
		var records []model.ProductionWorldPlanningBeat
		if err := query.Find(&records).Error; err != nil {
			return nil, err
		}
		for _, record := range records {
			result = append(result, planningFactFromRecord(record.ID, record.WorkspaceID, record.ProjectID, record.EpisodeID, kind, record.BusinessKey, record.Revision, record.Payload, record.ContentHash, record.CreatedBy, record.CreatedAt))
		}
	case "occurrence":
		var records []model.ProductionWorldPlanningOccurrence
		if err := query.Find(&records).Error; err != nil {
			return nil, err
		}
		for _, record := range records {
			result = append(result, planningFactFromRecord(record.ID, record.WorkspaceID, record.ProjectID, record.EpisodeID, kind, record.BusinessKey, record.Revision, record.Payload, record.ContentHash, record.CreatedBy, record.CreatedAt))
		}
	case "continuity_claim":
		var records []model.ProductionWorldPlanningClaim
		if err := query.Find(&records).Error; err != nil {
			return nil, err
		}
		for _, record := range records {
			result = append(result, planningFactFromRecord(record.ID, record.WorkspaceID, record.ProjectID, record.EpisodeID, kind, record.BusinessKey, record.Revision, record.Payload, record.ContentHash, record.CreatedBy, record.CreatedAt))
		}
	default:
		return nil, errors.New("unknown Production World Planning fact kind")
	}
	for _, fact := range result {
		if domain.ValidateProductionWorldPlanningFact(fact) != nil || repo.verifyProductionWorldPlanningFact(ctx, fact) != nil {
			return nil, errors.New("persisted Production World Planning fact has drifted")
		}
	}
	return result, nil
}

func (repo *repository) CreateProductionWorldPlanningFacts(ctx context.Context, values []domain.ProductionWorldPlanningFact) error {
	for _, value := range values {
		if domain.ValidateProductionWorldPlanningFact(value) != nil {
			return errors.New("Production World Planning fact has drifted")
		}
		if err := repo.createProductionWorldPlanningFact(ctx, value); err != nil {
			return err
		}
	}
	return nil
}

func (repo *repository) CreateProductionWorldPlanningMemberships(ctx context.Context, head domain.ProductionWorldPlanningEpisodeHead) error {
	ids, err := planningIDs(head.WorkspaceID, head.ProjectID, head.EpisodeID)
	if err != nil || len(head.Members) == 0 {
		return errors.New("invalid Production World Planning memberships")
	}
	records := make([]model.ProductionWorldPlanningMembership, len(head.Members))
	for index, member := range head.Members {
		if domain.ValidateProductionWorldPlanningMember(member) != nil {
			return errors.New("invalid Production World Planning member")
		}
		factID, parseErr := uuid.Parse(member.Fact.ID)
		if parseErr != nil {
			return parseErr
		}
		records[index] = model.ProductionWorldPlanningMembership{
			ID: uuid.New(), WorkspaceID: ids[0], ProjectID: ids[1], EpisodeID: ids[2],
			ScopeRevision: head.ScopeRevision, Position: member.Position, FactID: factID,
			FactKind: member.Fact.Kind, BusinessKey: member.Fact.BusinessKey,
			FactRevision: member.Fact.Revision, FactContentHash: member.Fact.ContentHash,
			MemberHash: member.MemberHash, CreatedAt: head.UpdatedAt,
		}
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&records).Error
}

func (repo *repository) SaveProductionWorldPlanningHead(ctx context.Context, head domain.ProductionWorldPlanningEpisodeHead, expectedRevision int64, expectedHash string) error {
	ids, err := planningIDs(head.WorkspaceID, head.ProjectID, head.EpisodeID)
	if err != nil {
		return err
	}
	rootRefs := make([]domain.ProductionWorldPlanningFactRef, len(head.Members))
	for index, member := range head.Members {
		rootRefs[index] = member.Fact
	}
	refs, err := json.Marshal(rootRefs)
	if err != nil {
		return err
	}
	record := model.ProductionWorldPlanningEpisodeHead{
		EpisodeID: ids[2], WorkspaceID: ids[0], ProjectID: ids[1], ScopeRevision: head.ScopeRevision,
		HeadRevision: head.HeadRevision, MemberCount: head.MemberCount, ScopeContentHash: head.ScopeContentHash, MembersHash: head.MembersHash,
		CollectionRootHash: head.CollectionRootHash, HeadContentHash: head.HeadContentHash,
		CurrentRootRefs: datatypes.JSON(refs), UpdatedAt: head.UpdatedAt,
	}
	if expectedRevision == 0 {
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return application.ErrProductionWorldPlanningConflict
			}
			return err
		}
		return nil
	}
	updated := repo.database.WithContext(ctx).Model(&model.ProductionWorldPlanningEpisodeHead{}).
		Where("episode_id = ? AND workspace_id = ? AND project_id = ? AND head_revision = ? AND head_content_hash = ?", ids[2], ids[0], ids[1], expectedRevision, expectedHash).
		Select("scope_revision", "head_revision", "member_count", "scope_content_hash", "members_hash", "collection_root_hash", "head_content_hash", "current_root_refs", "updated_at").Updates(&record)
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return application.ErrProductionWorldPlanningConflict
	}
	return nil
}

func (repo *repository) createProductionWorldPlanningFact(ctx context.Context, value domain.ProductionWorldPlanningFact) error {
	ids, err := planningIDs(value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, value.CreatedBy)
	if err != nil {
		return err
	}
	base := func() (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
		return ids[0], ids[1], ids[2], ids[3], ids[4]
	}
	switch value.Kind {
	case "scene":
		var payload domain.SceneFactPayload
		if json.Unmarshal(value.Payload, &payload) != nil {
			return errors.New("invalid Planning Scene payload")
		}
		id, workspaceID, projectID, episodeID, createdBy := base()
		return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&model.ProductionWorldPlanningScene{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID, CreatedBy: createdBy, BusinessKey: value.BusinessKey, SceneScopeKey: payload.SceneScopeKey, SceneOwnerLogicalID: payload.SceneOwnerLogicalID, StoryTimeKey: payload.StoryTimeKey, Revision: value.Revision, Payload: datatypes.JSON(value.Payload), ContentHash: value.ContentHash, CreatedAt: value.CreatedAt}).Error
	case "dialogue":
		var payload domain.DialogueFactPayload
		if json.Unmarshal(value.Payload, &payload) != nil {
			return errors.New("invalid Planning Dialogue payload")
		}
		sceneID, parseErr := uuid.Parse(payload.Scene.ID)
		if parseErr != nil {
			return parseErr
		}
		id, workspaceID, projectID, episodeID, createdBy := base()
		return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&model.ProductionWorldPlanningDialogue{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID, SceneID: sceneID, CreatedBy: createdBy, BusinessKey: value.BusinessKey, Revision: value.Revision, SequenceKey: payload.SequenceKey, Payload: datatypes.JSON(value.Payload), ContentHash: value.ContentHash, CreatedAt: value.CreatedAt}).Error
	case "narrative_beat":
		var payload domain.NarrativeBeatFactPayload
		if json.Unmarshal(value.Payload, &payload) != nil {
			return errors.New("invalid Planning Beat payload")
		}
		sceneID, parseErr := uuid.Parse(payload.Scene.ID)
		if parseErr != nil {
			return parseErr
		}
		id, workspaceID, projectID, episodeID, createdBy := base()
		return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&model.ProductionWorldPlanningBeat{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID, SceneID: sceneID, CreatedBy: createdBy, BusinessKey: value.BusinessKey, Revision: value.Revision, SequenceKey: payload.SequenceKey, Payload: datatypes.JSON(value.Payload), ContentHash: value.ContentHash, CreatedAt: value.CreatedAt}).Error
	case "occurrence":
		var payload domain.OccurrenceFactPayload
		if json.Unmarshal(value.Payload, &payload) != nil {
			return errors.New("invalid Planning Occurrence payload")
		}
		refs, parseErr := planningIDs(payload.Scene.ID, payload.Asset.ID, payload.State.ID, payload.Specification.ID, payload.Binding.ID)
		if parseErr != nil {
			return parseErr
		}
		id, workspaceID, projectID, episodeID, createdBy := base()
		return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&model.ProductionWorldPlanningOccurrence{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID, SceneID: refs[0], AssetID: refs[1], AssetStateID: refs[2], SpecificationID: refs[3], ProductionBindingID: refs[4], CreatedBy: createdBy, BusinessKey: value.BusinessKey, Revision: value.Revision, SequenceKey: payload.SequenceKey, AssetContentHash: payload.Asset.ContentHash, StateContentHash: payload.State.ContentHash, SpecificationHash: payload.Specification.ContentHash, BindingHash: payload.Binding.ContentHash, Payload: datatypes.JSON(value.Payload), ContentHash: value.ContentHash, CreatedAt: value.CreatedAt}).Error
	case "continuity_claim":
		var payload domain.PlanningClaimFactPayload
		if json.Unmarshal(value.Payload, &payload) != nil {
			return errors.New("invalid Planning Claim payload")
		}
		refs, parseErr := planningIDs(payload.SourceScene.ID, payload.TargetScene.ID)
		if parseErr != nil {
			return parseErr
		}
		id, workspaceID, projectID, episodeID, createdBy := base()
		return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&model.ProductionWorldPlanningClaim{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID, SourceSceneID: refs[0], TargetSceneID: refs[1], CreatedBy: createdBy, BusinessKey: value.BusinessKey, ClaimType: payload.ClaimType, Revision: value.Revision, StoryTimeKey: payload.StoryTimeKey, Payload: datatypes.JSON(value.Payload), ContentHash: value.ContentHash, CreatedAt: value.CreatedAt}).Error
	default:
		return errors.New("unknown Production World Planning fact kind")
	}
}

func (repo *repository) loadProductionWorldPlanningFact(ctx context.Context, kind string, id uuid.UUID) (domain.ProductionWorldPlanningFact, error) {
	switch kind {
	case "scene":
		var value model.ProductionWorldPlanningScene
		if err := repo.database.WithContext(ctx).First(&value, "id = ?", id).Error; err != nil {
			return domain.ProductionWorldPlanningFact{}, err
		}
		return planningFactFromRecord(value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, kind, value.BusinessKey, value.Revision, value.Payload, value.ContentHash, value.CreatedBy, value.CreatedAt), nil
	case "dialogue":
		var value model.ProductionWorldPlanningDialogue
		if err := repo.database.WithContext(ctx).First(&value, "id = ?", id).Error; err != nil {
			return domain.ProductionWorldPlanningFact{}, err
		}
		return planningFactFromRecord(value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, kind, value.BusinessKey, value.Revision, value.Payload, value.ContentHash, value.CreatedBy, value.CreatedAt), nil
	case "narrative_beat":
		var value model.ProductionWorldPlanningBeat
		if err := repo.database.WithContext(ctx).First(&value, "id = ?", id).Error; err != nil {
			return domain.ProductionWorldPlanningFact{}, err
		}
		return planningFactFromRecord(value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, kind, value.BusinessKey, value.Revision, value.Payload, value.ContentHash, value.CreatedBy, value.CreatedAt), nil
	case "occurrence":
		var value model.ProductionWorldPlanningOccurrence
		if err := repo.database.WithContext(ctx).First(&value, "id = ?", id).Error; err != nil {
			return domain.ProductionWorldPlanningFact{}, err
		}
		return planningFactFromRecord(value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, kind, value.BusinessKey, value.Revision, value.Payload, value.ContentHash, value.CreatedBy, value.CreatedAt), nil
	case "continuity_claim":
		var value model.ProductionWorldPlanningClaim
		if err := repo.database.WithContext(ctx).First(&value, "id = ?", id).Error; err != nil {
			return domain.ProductionWorldPlanningFact{}, err
		}
		return planningFactFromRecord(value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, kind, value.BusinessKey, value.Revision, value.Payload, value.ContentHash, value.CreatedBy, value.CreatedAt), nil
	default:
		return domain.ProductionWorldPlanningFact{}, errors.New("unknown Production World Planning fact kind")
	}
}

func (repo *repository) verifyProductionWorldPlanningFact(ctx context.Context, fact domain.ProductionWorldPlanningFact) error {
	id, err := uuid.Parse(fact.ID)
	if err != nil {
		return errors.New("invalid Production World Planning fact identity")
	}
	sceneRef := func(ref domain.ProductionWorldPlanningFactRef) error {
		sceneID, parseErr := uuid.Parse(ref.ID)
		if parseErr != nil {
			return errors.New("Production World Planning Scene ref identity has drifted")
		}
		scene, loadErr := repo.loadProductionWorldPlanningFact(ctx, "scene", sceneID)
		if loadErr != nil || domain.ValidateProductionWorldPlanningFact(scene) != nil || domain.PlanningFactRef(scene) != ref {
			return fmt.Errorf("Production World Planning Scene ref has drifted: expected=%#v actual=%#v load_error=%v", ref, domain.PlanningFactRef(scene), loadErr)
		}
		return nil
	}
	factRef := func(ref *domain.ProductionWorldPlanningFactRef, expectedKind string) error {
		if ref == nil || ref.Kind != expectedKind {
			return errors.New("Production World Planning fact ref kind has drifted")
		}
		refID, parseErr := uuid.Parse(ref.ID)
		if parseErr != nil {
			return parseErr
		}
		value, loadErr := repo.loadProductionWorldPlanningFact(ctx, expectedKind, refID)
		if loadErr != nil || domain.ValidateProductionWorldPlanningFact(value) != nil || domain.PlanningFactRef(value) != *ref {
			return errors.New("Production World Planning fact ref has drifted")
		}
		return nil
	}
	stateRef := func(ref *domain.ExactPlanningRef) error {
		if ref == nil {
			return errors.New("Production World Planning state ref is missing")
		}
		stateID, parseErr := uuid.Parse(ref.ID)
		if parseErr != nil {
			return parseErr
		}
		var state model.AssetState
		if repo.database.WithContext(ctx).First(&state, "id = ?", stateID).Error != nil ||
			state.StateKey != ref.BusinessKey || state.Revision != ref.Revision || state.ContentHash != ref.ContentHash {
			return errors.New("Production World Planning state ref has drifted")
		}
		return nil
	}
	switch fact.Kind {
	case "scene":
		var record model.ProductionWorldPlanningScene
		var payload domain.SceneFactPayload
		if repo.database.WithContext(ctx).First(&record, "id = ?", id).Error != nil || json.Unmarshal(fact.Payload, &payload) != nil ||
			record.SceneScopeKey != payload.SceneScopeKey || record.SceneOwnerLogicalID != payload.SceneOwnerLogicalID ||
			record.StoryTimeKey != payload.StoryTimeKey {
			return errors.New("Production World Planning Scene metadata has drifted")
		}
	case "dialogue":
		var record model.ProductionWorldPlanningDialogue
		var payload domain.DialogueFactPayload
		if repo.database.WithContext(ctx).First(&record, "id = ?", id).Error != nil || json.Unmarshal(fact.Payload, &payload) != nil ||
			record.SceneID.String() != payload.Scene.ID || record.SequenceKey != payload.SequenceKey || sceneRef(payload.Scene) != nil {
			return errors.New("Production World Planning Dialogue refs have drifted")
		}
	case "narrative_beat":
		var record model.ProductionWorldPlanningBeat
		var payload domain.NarrativeBeatFactPayload
		if repo.database.WithContext(ctx).First(&record, "id = ?", id).Error != nil || json.Unmarshal(fact.Payload, &payload) != nil ||
			record.SceneID.String() != payload.Scene.ID || record.SequenceKey != payload.SequenceKey || sceneRef(payload.Scene) != nil {
			return errors.New("Production World Planning Beat refs have drifted")
		}
	case "occurrence":
		var record model.ProductionWorldPlanningOccurrence
		var payload domain.OccurrenceFactPayload
		if repo.database.WithContext(ctx).First(&record, "id = ?", id).Error != nil || json.Unmarshal(fact.Payload, &payload) != nil ||
			record.SceneID.String() != payload.Scene.ID || record.SequenceKey != payload.SequenceKey ||
			record.AssetID.String() != payload.Asset.ID || record.AssetStateID.String() != payload.State.ID ||
			record.SpecificationID.String() != payload.Specification.ID || record.ProductionBindingID.String() != payload.Binding.ID ||
			record.AssetContentHash != payload.Asset.ContentHash || record.StateContentHash != payload.State.ContentHash ||
			record.SpecificationHash != payload.Specification.ContentHash || record.BindingHash != payload.Binding.ContentHash ||
			sceneRef(payload.Scene) != nil {
			return errors.New("Production World Planning Occurrence refs have drifted")
		}
		var asset model.Asset
		var state model.AssetState
		var specification model.ProductionWorldSpecification
		var binding model.ProductionWorldBinding
		if repo.database.WithContext(ctx).First(&asset, "id = ?", record.AssetID).Error != nil ||
			repo.database.WithContext(ctx).First(&state, "id = ?", record.AssetStateID).Error != nil ||
			repo.database.WithContext(ctx).First(&specification, "id = ?", record.SpecificationID).Error != nil ||
			repo.database.WithContext(ctx).First(&binding, "id = ?", record.ProductionBindingID).Error != nil ||
			asset.IdentityKey != payload.Asset.BusinessKey || asset.Revision != payload.Asset.Revision || asset.ContentHash != payload.Asset.ContentHash ||
			state.StateKey != payload.State.BusinessKey || state.Revision != payload.State.Revision || state.ContentHash != payload.State.ContentHash ||
			specification.SpecificationKey != payload.Specification.BusinessKey || specification.Revision != payload.Specification.Revision || specification.ContentHash != payload.Specification.ContentHash ||
			binding.IdentityKey != payload.Binding.BusinessKey || binding.Revision != payload.Binding.Revision || binding.ContentHash != payload.Binding.ContentHash ||
			state.AssetID != asset.ID || specification.AssetID != asset.ID || binding.AssetID != asset.ID ||
			binding.SpecificationID != specification.ID {
			return errors.New("Production World Planning Occurrence production refs have drifted")
		}
		var bindingState model.ProductionWorldBindingState
		if repo.database.WithContext(ctx).Where("binding_id = ? AND asset_state_id = ?", binding.ID, state.ID).First(&bindingState).Error != nil ||
			bindingState.StateKey != state.StateKey || bindingState.Revision != state.Revision || bindingState.ContentHash != state.ContentHash {
			return errors.New("Production World Planning Occurrence binding state has drifted")
		}
	case "continuity_claim":
		var record model.ProductionWorldPlanningClaim
		var payload domain.PlanningClaimFactPayload
		if repo.database.WithContext(ctx).First(&record, "id = ?", id).Error != nil || json.Unmarshal(fact.Payload, &payload) != nil {
			return errors.New("Production World Planning Claim payload has drifted")
		}
		if record.SourceSceneID.String() != payload.SourceScene.ID || record.TargetSceneID.String() != payload.TargetScene.ID ||
			record.ClaimType != payload.ClaimType || record.StoryTimeKey != payload.StoryTimeKey {
			return errors.New("Production World Planning Claim metadata has drifted")
		}
		if sourceErr := sceneRef(payload.SourceScene); sourceErr != nil {
			return fmt.Errorf("Production World Planning Claim source Scene ref has drifted: %w", sourceErr)
		}
		if targetErr := sceneRef(payload.TargetScene); targetErr != nil {
			return fmt.Errorf("Production World Planning Claim target Scene ref has drifted: %w", targetErr)
		}
		if stateRef(payload.BeforeState) != nil || stateRef(payload.AfterState) != nil {
			return errors.New("Production World Planning Claim State refs have drifted")
		}
		if payload.ClaimType == "interaction" {
			if factRef(payload.ActorOccurrence, "occurrence") != nil || factRef(payload.PropOccurrence, "occurrence") != nil ||
				(payload.CounterpartyOccurrence != nil && factRef(payload.CounterpartyOccurrence, "occurrence") != nil) {
				return errors.New("Production World Planning Interaction refs have drifted")
			}
		} else if payload.ClaimType != "continuity" || payload.ActorOccurrence != nil || payload.PropOccurrence != nil || payload.CounterpartyOccurrence != nil {
			return errors.New("Production World Planning Claim branch has drifted")
		}
	default:
		return errors.New("unknown Production World Planning fact kind")
	}
	return nil
}

func planningFactFromRecord(id, workspaceID, projectID, episodeID uuid.UUID, kind, key string, revision int, payload datatypes.JSON, hash string, createdBy uuid.UUID, createdAt time.Time) domain.ProductionWorldPlanningFact {
	return domain.ProductionWorldPlanningFact{
		ID: id.String(), WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), EpisodeID: episodeID.String(),
		Kind: kind, BusinessKey: key, Revision: revision, Payload: json.RawMessage(payload), ContentHash: hash,
		CreatedBy: createdBy.String(), CreatedAt: createdAt,
	}
}

func planningIDs(values ...string) ([]uuid.UUID, error) {
	result := make([]uuid.UUID, len(values))
	for index, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, err
		}
		result[index] = id
	}
	return result, nil
}
