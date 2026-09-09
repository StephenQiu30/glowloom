package application

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
)

var (
	ErrProductionWorldPlanningHeadNotFound = errors.New("Production World Planning Episode Head not found")
	ErrProductionWorldPlanningConflict     = errors.New("Production World Planning Episode Head conflict")
)

type ExpectedProductionWorldPlanningHead struct {
	EpisodeID, ContentHash string
	Revision               int64
}

type ApplyProductionWorldPlanningCommand struct {
	WorkspaceID, ProjectID, ActorID string
	ExpectedBusinessKeyRoot         string
	ExpectedHeads                   []ExpectedProductionWorldPlanningHead
	EpisodeScopes                   []worlddomain.ProductionWorldPlanningEpisodeScope
	Planning                        worlddomain.ProductionWorldPlanningPartition
	Assets                          []assetdomain.Asset
	States                          []assetdomain.AssetState
	Specifications                  []bibledomain.ProductionWorldSpecification
	Bindings                        []bibledomain.ProductionWorldBinding
}

type ApplyProductionWorldPlanningResult struct {
	Heads []domain.ProductionWorldPlanningEpisodeHead
	Facts []domain.ProductionWorldPlanningFact
}

type ProductionWorldPlanningRepository interface {
	GetProductionWorldPlanningHead(context.Context, string, string, string, bool) (domain.ProductionWorldPlanningEpisodeHead, error)
	ListProductionWorldPlanningFacts(context.Context, string, string, string, bool) ([]domain.ProductionWorldPlanningFact, error)
	CreateProductionWorldPlanningFacts(context.Context, []domain.ProductionWorldPlanningFact) error
	CreateProductionWorldPlanningMemberships(context.Context, domain.ProductionWorldPlanningEpisodeHead) error
	SaveProductionWorldPlanningHead(context.Context, domain.ProductionWorldPlanningEpisodeHead, int64, string) error
}

type ProductionWorldPlanningOwner struct {
	now   func() time.Time
	newID func() string
}

func NewProductionWorldPlanningOwner(now func() time.Time, newID func() string) *ProductionWorldPlanningOwner {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if newID == nil {
		newID = uuid.NewString
	}
	return &ProductionWorldPlanningOwner{now: now, newID: newID}
}

func (owner *ProductionWorldPlanningOwner) ApplyProductionWorldPlanning(
	ctx context.Context,
	repository ProductionWorldPlanningRepository,
	command ApplyProductionWorldPlanningCommand,
) (ApplyProductionWorldPlanningResult, error) {
	if owner == nil || repository == nil {
		return ApplyProductionWorldPlanningResult{}, errors.New("Production World Planning owner is unavailable")
	}
	material, err := validateProductionWorldPlanningCommand(command)
	if err != nil {
		return ApplyProductionWorldPlanningResult{}, err
	}
	currentHeads := make(map[string]domain.ProductionWorldPlanningEpisodeHead, len(command.EpisodeScopes))
	for _, scope := range command.EpisodeScopes {
		expected := material.expectedHeads[scope.EpisodeID]
		head, headErr := repository.GetProductionWorldPlanningHead(ctx, command.WorkspaceID, command.ProjectID, scope.EpisodeID, true)
		if expected.Revision == 0 {
			if headErr == nil || !errors.Is(headErr, ErrProductionWorldPlanningHeadNotFound) {
				return ApplyProductionWorldPlanningResult{}, ErrProductionWorldPlanningConflict
			}
			continue
		}
		if headErr != nil || head.HeadRevision != expected.Revision || head.HeadContentHash != expected.ContentHash {
			return ApplyProductionWorldPlanningResult{}, ErrProductionWorldPlanningConflict
		}
		currentHeads[scope.EpisodeID] = head
	}

	now := owner.now().UTC()
	build := planningFactBuilder{ctx: ctx, repository: repository, owner: owner, command: command, now: now}
	factsByEpisode, err := build.build(material)
	if err != nil {
		return ApplyProductionWorldPlanningResult{}, err
	}
	if err = repository.CreateProductionWorldPlanningFacts(ctx, build.created); err != nil {
		return ApplyProductionWorldPlanningResult{}, err
	}

	result := ApplyProductionWorldPlanningResult{Heads: make([]domain.ProductionWorldPlanningEpisodeHead, 0, len(command.EpisodeScopes))}
	for _, scope := range command.EpisodeScopes {
		current, exists := currentHeads[scope.EpisodeID]
		revision := int64(1)
		if exists {
			revision = current.HeadRevision
		}
		head, headErr := domain.NewProductionWorldPlanningEpisodeHead(
			command.WorkspaceID, command.ProjectID, scope.EpisodeID,
			revision, factsByEpisode[scope.EpisodeID], now,
		)
		if headErr != nil {
			return ApplyProductionWorldPlanningResult{}, headErr
		}
		if exists && reflect.DeepEqual(head.Members, current.Members) {
			result.Heads = append(result.Heads, current)
			continue
		}
		if exists {
			head, headErr = domain.NewProductionWorldPlanningEpisodeHead(
				command.WorkspaceID, command.ProjectID, scope.EpisodeID,
				current.HeadRevision+1, factsByEpisode[scope.EpisodeID], now,
			)
			if headErr != nil {
				return ApplyProductionWorldPlanningResult{}, headErr
			}
		}
		if err = repository.CreateProductionWorldPlanningMemberships(ctx, head); err != nil {
			return ApplyProductionWorldPlanningResult{}, err
		}
		expected := material.expectedHeads[scope.EpisodeID]
		if err = repository.SaveProductionWorldPlanningHead(ctx, head, expected.Revision, expected.ContentHash); err != nil {
			return ApplyProductionWorldPlanningResult{}, err
		}
		result.Heads = append(result.Heads, head)
	}
	for _, head := range result.Heads {
		for _, member := range head.Members {
			result.Facts = append(result.Facts, build.byID[member.Fact.ID])
		}
	}
	return result, nil
}

type planningMaterial struct {
	expectedHeads     map[string]ExpectedProductionWorldPlanningHead
	episodeByScene    map[string]string
	storyTimeByScene  map[string]string
	assetByIdentity   map[string]assetdomain.Asset
	stateByKey        map[string]assetdomain.AssetState
	specByIdentity    map[string]bibledomain.ProductionWorldSpecification
	bindingByIdentity map[string]bibledomain.ProductionWorldBinding
}

func validateProductionWorldPlanningCommand(command ApplyProductionWorldPlanningCommand) (planningMaterial, error) {
	for _, id := range []string{command.WorkspaceID, command.ProjectID, command.ActorID} {
		if _, err := uuid.Parse(id); err != nil {
			return planningMaterial{}, errors.New("invalid Production World Planning owner identity")
		}
	}
	if len(command.EpisodeScopes) == 0 || len(command.Planning.Scenes) == 0 || len(command.ExpectedHeads) != len(command.EpisodeScopes) {
		return planningMaterial{}, errors.New("Production World Planning scope is incomplete")
	}
	keys := productionWorldPlanningBusinessKeys(command.Planning)
	root, err := domain.PlanningBusinessKeyRoot(keys)
	if err != nil || root != command.ExpectedBusinessKeyRoot {
		return planningMaterial{}, errors.New("Production World Planning expected business keys drifted")
	}
	result := planningMaterial{
		expectedHeads:     make(map[string]ExpectedProductionWorldPlanningHead, len(command.ExpectedHeads)),
		episodeByScene:    make(map[string]string, len(command.Planning.Scenes)),
		storyTimeByScene:  make(map[string]string, len(command.Planning.SceneStoryTimes)),
		assetByIdentity:   make(map[string]assetdomain.Asset, len(command.Assets)),
		stateByKey:        make(map[string]assetdomain.AssetState, len(command.States)),
		specByIdentity:    make(map[string]bibledomain.ProductionWorldSpecification, len(command.Specifications)),
		bindingByIdentity: make(map[string]bibledomain.ProductionWorldBinding, len(command.Bindings)),
	}
	for _, expected := range command.ExpectedHeads {
		if _, err = uuid.Parse(expected.EpisodeID); err != nil || expected.Revision < 0 ||
			(expected.Revision == 0) != (expected.ContentHash == "") ||
			(expected.Revision > 0 && len(expected.ContentHash) != 64) {
			return planningMaterial{}, errors.New("invalid Production World Planning expected Head")
		}
		if _, duplicate := result.expectedHeads[expected.EpisodeID]; duplicate {
			return planningMaterial{}, errors.New("duplicated Production World Planning expected Head")
		}
		result.expectedHeads[expected.EpisodeID] = expected
	}
	for _, scope := range command.EpisodeScopes {
		if scope.ScopeKey != "episode:"+scope.EpisodeID || len(scope.SceneScopeKeys) == 0 || !slices.IsSorted(scope.SceneScopeKeys) {
			return planningMaterial{}, errors.New("invalid Production World Planning Episode scope")
		}
		if _, exists := result.expectedHeads[scope.EpisodeID]; !exists {
			return planningMaterial{}, errors.New("missing Production World Planning expected Head")
		}
		for _, sceneKey := range scope.SceneScopeKeys {
			if _, duplicate := result.episodeByScene[sceneKey]; duplicate {
				return planningMaterial{}, errors.New("Production World Planning Scene belongs to multiple Episodes")
			}
			result.episodeByScene[sceneKey] = scope.EpisodeID
		}
	}
	for _, scene := range command.Planning.Scenes {
		if result.episodeByScene[scene.SceneScopeKey] == "" || strings.TrimSpace(scene.SceneOwnerLogicalID) == "" {
			return planningMaterial{}, errors.New("Production World Planning Scene scope drifted")
		}
	}
	if len(result.episodeByScene) != len(command.Planning.Scenes) || len(command.Planning.SceneStoryTimes) != len(command.Planning.Scenes) {
		return planningMaterial{}, errors.New("Production World Planning Scene coverage is incomplete")
	}
	for _, anchor := range command.Planning.SceneStoryTimes {
		if result.episodeByScene[anchor.SceneScopeKey] == "" || strings.TrimSpace(anchor.StoryTimeKey) == "" || result.storyTimeByScene[anchor.SceneScopeKey] != "" {
			return planningMaterial{}, errors.New("Production World Planning story-time coverage drifted")
		}
		result.storyTimeByScene[anchor.SceneScopeKey] = anchor.StoryTimeKey
	}
	for _, asset := range command.Assets {
		if assetdomain.ValidateAsset(asset) != nil || asset.WorkspaceID != command.WorkspaceID || asset.ProjectID != command.ProjectID || result.assetByIdentity[asset.IdentityKey].ID != "" {
			return planningMaterial{}, errors.New("invalid Production World Planning Asset set")
		}
		result.assetByIdentity[asset.IdentityKey] = asset
	}
	for _, state := range command.States {
		if assetdomain.ValidateAssetState(state) != nil || state.WorkspaceID != command.WorkspaceID || state.ProjectID != command.ProjectID || result.stateByKey[state.StateKey].ID != "" {
			return planningMaterial{}, errors.New("invalid Production World Planning AssetState set")
		}
		result.stateByKey[state.StateKey] = state
	}
	for _, specification := range command.Specifications {
		asset := result.assetByIdentity[specification.IdentityKey]
		if asset.ID == "" || specification.WorkspaceID != command.WorkspaceID || specification.ProjectID != command.ProjectID ||
			specification.Asset.ID != asset.ID || specification.Asset.ContentHash != asset.ContentHash || result.specByIdentity[specification.IdentityKey].ID != "" {
			return planningMaterial{}, errors.New("invalid Production World Planning Specification set")
		}
		result.specByIdentity[specification.IdentityKey] = specification
	}
	for _, binding := range command.Bindings {
		asset := result.assetByIdentity[binding.IdentityKey]
		specification := result.specByIdentity[binding.IdentityKey]
		if asset.ID == "" || specification.ID == "" || binding.WorkspaceID != command.WorkspaceID || binding.ProjectID != command.ProjectID ||
			binding.Asset.ID != asset.ID || binding.Asset.ContentHash != asset.ContentHash ||
			binding.Specification.ID != specification.ID || binding.Specification.ContentHash != specification.ContentHash ||
			result.bindingByIdentity[binding.IdentityKey].ID != "" {
			return planningMaterial{}, errors.New("invalid Production World Planning ProductionBinding set")
		}
		result.bindingByIdentity[binding.IdentityKey] = binding
	}
	return result, nil
}

type planningFactBuilder struct {
	ctx        context.Context
	repository ProductionWorldPlanningRepository
	owner      *ProductionWorldPlanningOwner
	command    ApplyProductionWorldPlanningCommand
	now        time.Time
	created    []domain.ProductionWorldPlanningFact
	byID       map[string]domain.ProductionWorldPlanningFact
}

func (builder *planningFactBuilder) build(material planningMaterial) (map[string][]domain.ProductionWorldPlanningFact, error) {
	builder.byID = make(map[string]domain.ProductionWorldPlanningFact)
	result := make(map[string][]domain.ProductionWorldPlanningFact, len(builder.command.EpisodeScopes))
	sceneFacts := make(map[string]domain.ProductionWorldPlanningFact, len(builder.command.Planning.Scenes))
	occurrenceFacts := make(map[string]domain.ProductionWorldPlanningFact)
	for _, scene := range builder.command.Planning.Scenes {
		episodeID := material.episodeByScene[scene.SceneScopeKey]
		payload := domain.SceneFactPayload{
			SceneScopeKey: scene.SceneScopeKey, SceneOwnerLogicalID: scene.SceneOwnerLogicalID,
			TemporarySceneID: scene.TemporarySceneID, StoryTimeKey: material.storyTimeByScene[scene.SceneScopeKey],
			SourceStart: scene.SourceStart, SourceEnd: scene.SourceEnd,
		}
		fact, err := builder.fact(episodeID, "scene", "scene:"+scene.SceneOwnerLogicalID, payload)
		if err != nil {
			return nil, err
		}
		sceneFacts[scene.SceneScopeKey] = fact
		result[episodeID] = append(result[episodeID], fact)
	}
	for _, scene := range builder.command.Planning.Scenes {
		episodeID, sceneFact := material.episodeByScene[scene.SceneScopeKey], sceneFacts[scene.SceneScopeKey]
		for _, dialogue := range scene.Dialogues {
			fact, err := builder.fact(episodeID, "dialogue", "dialogue:"+scene.SceneOwnerLogicalID+":"+dialogue.DialogueKey, domain.DialogueFactPayload{Scene: domain.PlanningFactRef(sceneFact), SequenceKey: dialogue.Order, Fragment: mustPlanningJSON(dialogue)})
			if err != nil {
				return nil, err
			}
			result[episodeID] = append(result[episodeID], fact)
		}
		for _, beat := range scene.Beats {
			fact, err := builder.fact(episodeID, "narrative_beat", "beat:"+scene.SceneOwnerLogicalID+":"+beat.BeatKey, domain.NarrativeBeatFactPayload{Scene: domain.PlanningFactRef(sceneFact), SequenceKey: beat.Order, Fragment: mustPlanningJSON(beat)})
			if err != nil {
				return nil, err
			}
			result[episodeID] = append(result[episodeID], fact)
		}
		for _, occurrence := range scene.Occurrences {
			asset, state := material.assetByIdentity[occurrence.IdentityKey], material.stateByKey[occurrence.StateKey]
			specification, binding := material.specByIdentity[occurrence.IdentityKey], material.bindingByIdentity[occurrence.IdentityKey]
			if asset.ID == "" || state.AssetID != asset.ID || specification.ID == "" || binding.ID == "" || !bindingContainsState(binding, state) {
				return nil, errors.New("Production World Planning Occurrence cannot resolve exact production refs")
			}
			payload := domain.OccurrenceFactPayload{
				Scene: domain.PlanningFactRef(sceneFact), SequenceKey: occurrence.Order, Fragment: mustPlanningJSON(occurrence),
				Asset:         planningRef(asset.ID, occurrence.IdentityKey, asset.Revision, asset.ContentHash),
				State:         planningRef(state.ID, state.StateKey, state.Revision, state.ContentHash),
				Specification: planningRef(specification.ID, specification.SpecificationKey, specification.Revision, specification.ContentHash),
				Binding:       planningRef(binding.ID, binding.IdentityKey, binding.Revision, binding.ContentHash),
			}
			fact, err := builder.fact(episodeID, "occurrence", "occurrence:"+occurrence.OccurrenceKey, payload)
			if err != nil {
				return nil, err
			}
			occurrenceFacts[occurrence.OccurrenceKey] = fact
			result[episodeID] = append(result[episodeID], fact)
		}
	}
	for _, interaction := range builder.command.Planning.Interactions {
		episodeID, sceneFact := material.episodeByScene[interaction.SceneScopeKey], sceneFacts[interaction.SceneScopeKey]
		actor, prop := occurrenceFacts[interaction.ActorOccurrenceKey], occurrenceFacts[interaction.PropOccurrenceKey]
		if actor.ID == "" || prop.ID == "" {
			return nil, errors.New("Production World Planning Interaction occurrence refs drifted")
		}
		payload := domain.PlanningClaimFactPayload{
			ClaimType: "interaction", StoryTimeKey: interaction.StoryTimeKey,
			SourceScene: domain.PlanningFactRef(sceneFact), TargetScene: domain.PlanningFactRef(sceneFact),
			ActorOccurrence: planningFactRefPointer(actor), PropOccurrence: planningFactRefPointer(prop),
			BeforeState: planningStateRefPointer(material.stateByKey[interaction.PropStateBeforeKey]),
			AfterState:  planningStateRefPointer(material.stateByKey[interaction.PropStateAfterKey]),
			Fragment:    mustPlanningJSON(interaction),
		}
		if interaction.CounterpartyOccurrenceKey != nil {
			counterparty := occurrenceFacts[*interaction.CounterpartyOccurrenceKey]
			if counterparty.ID == "" {
				return nil, errors.New("Production World Planning Interaction counterparty drifted")
			}
			payload.CounterpartyOccurrence = planningFactRefPointer(counterparty)
		}
		fact, err := builder.fact(episodeID, "continuity_claim", "interaction:"+interaction.InteractionKey, payload)
		if err != nil {
			return nil, err
		}
		result[episodeID] = append(result[episodeID], fact)
	}
	for _, continuity := range builder.command.Planning.Continuity {
		source, target := sceneFacts[continuity.FromSceneScopeKey], sceneFacts[continuity.ToSceneScopeKey]
		episodeID := material.episodeByScene[continuity.ToSceneScopeKey]
		before, after := material.stateByKey[continuity.BeforeStateKey], material.stateByKey[continuity.AfterStateKey]
		if source.ID == "" || target.ID == "" || before.ID == "" || after.ID == "" {
			return nil, errors.New("Production World Planning Continuity refs drifted")
		}
		payload := domain.PlanningClaimFactPayload{
			ClaimType: "continuity", StoryTimeKey: continuity.StoryTimeEnd,
			SourceScene: domain.PlanningFactRef(source), TargetScene: domain.PlanningFactRef(target),
			BeforeState: planningStateRefPointer(before), AfterState: planningStateRefPointer(after),
			Fragment: mustPlanningJSON(continuity),
		}
		fact, err := builder.fact(episodeID, "continuity_claim", "continuity:"+continuity.ContinuityKey, payload)
		if err != nil {
			return nil, err
		}
		result[episodeID] = append(result[episodeID], fact)
	}
	return result, nil
}

func (builder *planningFactBuilder) fact(episodeID, kind, key string, payload any) (domain.ProductionWorldPlanningFact, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return domain.ProductionWorldPlanningFact{}, err
	}
	existing, err := builder.repository.ListProductionWorldPlanningFacts(builder.ctx, builder.command.ProjectID, kind, key, true)
	if err != nil {
		return domain.ProductionWorldPlanningFact{}, err
	}
	revision := 1
	if len(existing) > 0 {
		revision = existing[len(existing)-1].Revision + 1
	}
	value, err := domain.NewProductionWorldPlanningFact(builder.owner.newID(), builder.command.WorkspaceID, builder.command.ProjectID, episodeID, kind, key, revision, raw, builder.command.ActorID, builder.now)
	if err != nil {
		return domain.ProductionWorldPlanningFact{}, err
	}
	if len(existing) > 0 && existing[len(existing)-1].ContentHash == value.ContentHash {
		value = existing[len(existing)-1]
	} else {
		builder.created = append(builder.created, value)
	}
	builder.byID[value.ID] = value
	return value, nil
}

func productionWorldPlanningBusinessKeys(value worlddomain.ProductionWorldPlanningPartition) []string {
	keys := make([]string, 0)
	for _, scene := range value.Scenes {
		keys = append(keys, "scene:"+scene.SceneOwnerLogicalID)
		for _, dialogue := range scene.Dialogues {
			keys = append(keys, "dialogue:"+scene.SceneOwnerLogicalID+":"+dialogue.DialogueKey)
		}
		for _, beat := range scene.Beats {
			keys = append(keys, "beat:"+scene.SceneOwnerLogicalID+":"+beat.BeatKey)
		}
		for _, occurrence := range scene.Occurrences {
			keys = append(keys, "occurrence:"+occurrence.OccurrenceKey)
		}
	}
	for _, interaction := range value.Interactions {
		keys = append(keys, "interaction:"+interaction.InteractionKey)
	}
	for _, continuity := range value.Continuity {
		keys = append(keys, "continuity:"+continuity.ContinuityKey)
	}
	return keys
}

func planningRef(id, key string, revision int, hash string) domain.ExactPlanningRef {
	return domain.ExactPlanningRef{ID: id, BusinessKey: key, Revision: revision, ContentHash: hash}
}
func planningFactRefPointer(value domain.ProductionWorldPlanningFact) *domain.ProductionWorldPlanningFactRef {
	ref := domain.PlanningFactRef(value)
	return &ref
}
func planningStateRefPointer(value assetdomain.AssetState) *domain.ExactPlanningRef {
	ref := planningRef(value.ID, value.StateKey, value.Revision, value.ContentHash)
	return &ref
}
func bindingContainsState(binding bibledomain.ProductionWorldBinding, state assetdomain.AssetState) bool {
	return slices.ContainsFunc(binding.States, func(ref bibledomain.ProductionWorldStateRef) bool {
		return ref.ID == state.ID && ref.AssetID == state.AssetID && ref.StateKey == state.StateKey && ref.Revision == state.Revision && ref.ContentHash == state.ContentHash
	})
}
func mustPlanningJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}
