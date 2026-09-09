package domain

import (
	"errors"
	"slices"
	"strings"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
)

const (
	ProductionWorldRepairReviseEntity      = "revise_production_entity"
	ProductionWorldRepairRebindOccurrence  = "rebind_scene_occurrence"
	ProductionWorldRepairReviseInteraction = "revise_interaction"
	ProductionWorldRepairReviseContinuity  = "revise_continuity"
)

type ProductionWorldRepairSelection struct {
	Operation  string   `json:"operation"`
	TargetKeys []string `json:"target_keys"`
}

type ProductionWorldRepairTargetSet struct {
	Operation  string   `json:"operation"`
	TargetKeys []string `json:"target_keys"`
}

type ProductionWorldRepairChange struct {
	Operation         string   `json:"operation"`
	TargetKeys        []string `json:"target_keys"`
	AffectedScopeKeys []string `json:"affected_scope_keys"`
}

type ProductionWorldChangeRequest struct {
	IssueRefs    []string                    `json:"issue_refs"`
	EvidenceRefs []HumanGateEvidenceRef      `json:"evidence_refs"`
	ChangeSpec   ProductionWorldRepairChange `json:"change_spec"`
	ReasonCode   string                      `json:"reason_code"`
	UserNote     *string                     `json:"user_note,omitempty"`
}

func (value ProductionWorldChangeRequest) Clone() ProductionWorldChangeRequest {
	result := value
	result.IssueRefs = append(make([]string, 0, len(value.IssueRefs)), value.IssueRefs...)
	result.EvidenceRefs = append(make([]HumanGateEvidenceRef, 0, len(value.EvidenceRefs)), value.EvidenceRefs...)
	result.ChangeSpec.TargetKeys = append(make([]string, 0, len(value.ChangeSpec.TargetKeys)), value.ChangeSpec.TargetKeys...)
	result.ChangeSpec.AffectedScopeKeys = append(make([]string, 0, len(value.ChangeSpec.AffectedScopeKeys)), value.ChangeSpec.AffectedScopeKeys...)
	if value.UserNote != nil {
		note := *value.UserNote
		result.UserNote = &note
	}
	return result
}

type ProductionWorldRepairClosure struct {
	Operation       string   `json:"operation"`
	TargetKeys      []string `json:"target_keys"`
	SceneScopeKeys  []string `json:"scene_scope_keys"`
	EntityKeys      []string `json:"entity_keys"`
	StateKeys       []string `json:"state_keys"`
	OccurrenceKeys  []string `json:"occurrence_keys"`
	InteractionKeys []string `json:"interaction_keys"`
	ContinuityKeys  []string `json:"continuity_keys"`
	LedgerKeys      []string `json:"ledger_keys"`
}

func (value ProductionWorldRepairClosure) AllKeys() []string {
	result := make([]string, 0, len(value.SceneScopeKeys)+len(value.EntityKeys)+len(value.StateKeys)+
		len(value.OccurrenceKeys)+len(value.InteractionKeys)+len(value.ContinuityKeys)+len(value.LedgerKeys))
	result = append(result, value.SceneScopeKeys...)
	result = append(result, value.EntityKeys...)
	result = append(result, value.StateKeys...)
	result = append(result, value.OccurrenceKeys...)
	result = append(result, value.InteractionKeys...)
	result = append(result, value.ContinuityKeys...)
	result = append(result, value.LedgerKeys...)
	slices.Sort(result)
	return result
}

type productionWorldOccurrenceRef struct {
	sceneScopeKey string
	value         agentcontract.SceneOccurrenceFragment
}

type productionWorldRepairIndex struct {
	entities     map[string]ProductionWorldEntityReviewItem
	stateOwners  map[string]string
	scenes       map[string]ProductionWorldSceneOccurrenceReviewItem
	occurrences  map[string]productionWorldOccurrenceRef
	interactions map[string]agentcontract.InteractionFragment
	continuity   map[string]agentcontract.ContinuityFragment
	ledger       map[string]agentcontract.ContinuityLedgerEntry
}

type stringSet map[string]struct{}

type productionWorldRepairSets struct {
	scenes, entities, states, occurrences, interactions, continuity, ledger stringSet
}

func productionWorldRepairTargetSets(views ProductionWorldReviewViews) ([]ProductionWorldRepairTargetSet, error) {
	index, err := newProductionWorldRepairIndex(views)
	if err != nil {
		return nil, err
	}
	entityTargets := make(stringSet)
	for key := range index.entities {
		entityTargets.add(key)
	}
	for key := range index.stateOwners {
		entityTargets.add(key)
	}
	occurrenceTargets := make(stringSet)
	for key := range index.scenes {
		occurrenceTargets.add(key)
	}
	for key := range index.occurrences {
		occurrenceTargets.add(key)
	}
	interactionTargets := make(stringSet)
	for key := range index.interactions {
		interactionTargets.add(key)
	}
	continuityTargets := make(stringSet)
	for key := range index.continuity {
		continuityTargets.add(key)
	}
	return []ProductionWorldRepairTargetSet{
		{Operation: ProductionWorldRepairReviseEntity, TargetKeys: sortedSet(entityTargets)},
		{Operation: ProductionWorldRepairRebindOccurrence, TargetKeys: sortedSet(occurrenceTargets)},
		{Operation: ProductionWorldRepairReviseInteraction, TargetKeys: sortedSet(interactionTargets)},
		{Operation: ProductionWorldRepairReviseContinuity, TargetKeys: sortedSet(continuityTargets)},
	}, nil
}

func validateProductionWorldRepairTargetSets(values []ProductionWorldRepairTargetSet) error {
	expectedOperations := []string{
		ProductionWorldRepairReviseEntity,
		ProductionWorldRepairRebindOccurrence,
		ProductionWorldRepairReviseInteraction,
		ProductionWorldRepairReviseContinuity,
	}
	if len(values) != len(expectedOperations) {
		return errors.New("Production World repair target inventory is incomplete")
	}
	for index, value := range values {
		if value.Operation != expectedOperations[index] || value.TargetKeys == nil || !slices.IsSorted(value.TargetKeys) {
			return errors.New("Production World repair target inventory is invalid")
		}
		for targetIndex, target := range value.TargetKeys {
			if target == "" || target != strings.TrimSpace(target) ||
				(targetIndex > 0 && value.TargetKeys[targetIndex-1] == target) {
				return errors.New("Production World repair target inventory is invalid")
			}
		}
	}
	return nil
}

func NewProductionWorldRepairClosure(
	views ProductionWorldReviewViews,
	selection ProductionWorldRepairSelection,
) (ProductionWorldRepairClosure, error) {
	if !validProductionWorldRepairSelection(selection) {
		return ProductionWorldRepairClosure{}, errors.New("invalid Production World repair selection")
	}
	index, err := newProductionWorldRepairIndex(views)
	if err != nil {
		return ProductionWorldRepairClosure{}, err
	}
	sets := newProductionWorldRepairSets()
	seedScenes := make(stringSet)
	wholeSceneSeeds := make(stringSet)

	switch selection.Operation {
	case ProductionWorldRepairReviseEntity:
		for _, target := range selection.TargetKeys {
			if entity, ok := index.entities[target]; ok {
				sets.entities.add(target)
				for _, state := range entity.States {
					sets.states.add(state.StateKey)
				}
				continue
			}
			owner, ok := index.stateOwners[target]
			if !ok {
				return ProductionWorldRepairClosure{}, errors.New("Production World entity repair target is outside the frozen review")
			}
			sets.states.add(target)
			sets.entities.add(owner)
		}
		for key, occurrence := range index.occurrences {
			if sets.entities.has(occurrence.value.IdentityKey) || sets.states.has(occurrence.value.StateKey) {
				addProductionWorldOccurrence(sets, seedScenes, key, occurrence)
			}
		}
	case ProductionWorldRepairRebindOccurrence:
		for _, target := range selection.TargetKeys {
			if scene, ok := index.scenes[target]; ok {
				sets.scenes.add(target)
				seedScenes.add(target)
				wholeSceneSeeds.add(target)
				for _, occurrence := range scene.Occurrences {
					addProductionWorldOccurrence(sets, seedScenes, occurrence.OccurrenceKey, index.occurrences[occurrence.OccurrenceKey])
				}
				continue
			}
			occurrence, ok := index.occurrences[target]
			if !ok {
				return ProductionWorldRepairClosure{}, errors.New("Production World occurrence repair target is outside the frozen review")
			}
			addProductionWorldOccurrence(sets, seedScenes, target, occurrence)
		}
	case ProductionWorldRepairReviseInteraction:
		for _, target := range selection.TargetKeys {
			interaction, ok := index.interactions[target]
			if !ok {
				return ProductionWorldRepairClosure{}, errors.New("Production World interaction repair target is outside the frozen review")
			}
			addProductionWorldInteraction(sets, seedScenes, index, interaction)
		}
	case ProductionWorldRepairReviseContinuity:
		for _, target := range selection.TargetKeys {
			claim, ok := index.continuity[target]
			if !ok {
				return ProductionWorldRepairClosure{}, errors.New("Production World continuity repair target is outside the frozen review")
			}
			addProductionWorldContinuity(sets, index, claim)
		}
	}

	if selection.Operation != ProductionWorldRepairReviseContinuity {
		for _, interaction := range views.Interactions {
			if sets.interactions.has(interaction.InteractionKey) ||
				wholeSceneSeeds.has(interaction.SceneScopeKey) ||
				sets.occurrences.has(interaction.ActorOccurrenceKey) ||
				sets.occurrences.has(interaction.PropOccurrenceKey) ||
				(interaction.CounterpartyOccurrenceKey != nil && sets.occurrences.has(*interaction.CounterpartyOccurrenceKey)) {
				addProductionWorldInteraction(sets, seedScenes, index, interaction)
			}
		}
		for _, claim := range views.Continuity.Claims {
			if productionWorldContinuityTouchesSeeds(claim, selection.Operation, seedScenes, sets) {
				addProductionWorldContinuity(sets, index, claim)
			}
		}
	}

	for _, interaction := range views.Interactions {
		if sets.interactions.has(interaction.InteractionKey) {
			continue
		}
		if sets.scenes.has(interaction.SceneScopeKey) &&
			(sets.occurrences.has(interaction.ActorOccurrenceKey) ||
				sets.occurrences.has(interaction.PropOccurrenceKey) ||
				sets.states.has(interaction.PropStateBeforeKey) || sets.states.has(interaction.PropStateAfterKey)) {
			addProductionWorldInteraction(sets, seedScenes, index, interaction)
		}
	}
	for _, entry := range views.Continuity.Ledger {
		if !sets.scenes.has(entry.SceneScopeKey) {
			continue
		}
		if sets.entities.has(entry.IdentityKey) || sets.states.has(entry.StateKey) ||
			(entry.TransitionInteractionKey != nil && sets.interactions.has(*entry.TransitionInteractionKey)) {
			sets.ledger.add(entry.LedgerKey)
		}
	}

	return productionWorldRepairClosureFromSets(selection, sets), nil
}

func ValidateProductionWorldChangeRequest(
	gate ProductionWorldGateInput,
	detail ProductionWorldReviewDetail,
	request ProductionWorldChangeRequest,
) error {
	if detail.InputHash != gate.InputHash || detail.CandidateRevision != gate.Subject.ProductionWorldCandidate ||
		!sameProductionWorldRepairTargetSets(detail.RepairTargets, gate.Subject.RepairTargets) {
		return errors.New("Production World change request review binding has drifted")
	}
	if !validOptionalProductionWorldIssueRefs(request.IssueRefs) ||
		!validProductionWorldRepairEvidenceRefs(request.EvidenceRefs) ||
		!validProductionWorldRepairReason(request.ChangeSpec.Operation, request.ReasonCode) {
		return errors.New("invalid Production World change request")
	}
	if request.UserNote != nil {
		note := strings.TrimSpace(*request.UserNote)
		if note == "" || note != *request.UserNote || len([]rune(note)) > 1000 {
			return errors.New("invalid Production World change request note")
		}
	}
	if len(request.IssueRefs) == 0 && len(request.EvidenceRefs) == 0 && request.UserNote == nil {
		return errors.New("Production World change request has no review basis")
	}
	selection := ProductionWorldRepairSelection{
		Operation: request.ChangeSpec.Operation, TargetKeys: append([]string(nil), request.ChangeSpec.TargetKeys...),
	}
	if !productionWorldSelectionWithinTargets(selection, gate.Subject.RepairTargets) {
		return errors.New("Production World change request is outside the frozen repair targets")
	}
	closure, err := NewProductionWorldRepairClosure(detail.Views, selection)
	if err != nil || !slices.Equal(request.ChangeSpec.AffectedScopeKeys, closure.AllKeys()) {
		return errors.New("Production World change request closure has drifted")
	}
	if !productionWorldIssueRefsWithinDetail(request.IssueRefs, detail.ReviewIssues) {
		return errors.New("Production World change request issue has drifted")
	}
	allowedEvidence := productionWorldRepairEvidence(detail.Views, closure, gate.Subject.SourceVersion.VersionID)
	for _, evidence := range request.EvidenceRefs {
		if _, exists := allowedEvidence[evidence]; !exists {
			return errors.New("Production World change request evidence is outside the affected closure")
		}
	}
	return nil
}

func sameProductionWorldRepairTargetSets(left, right []ProductionWorldRepairTargetSet) bool {
	return slices.EqualFunc(left, right, func(left, right ProductionWorldRepairTargetSet) bool {
		return left.Operation == right.Operation && slices.Equal(left.TargetKeys, right.TargetKeys)
	})
}

func validOptionalProductionWorldIssueRefs(values []string) bool {
	if values == nil || !slices.IsSorted(values) {
		return false
	}
	for index, value := range values {
		if value == "" || value != strings.TrimSpace(value) || (index > 0 && values[index-1] == value) {
			return false
		}
	}
	return true
}

func validProductionWorldRepairEvidenceRefs(values []HumanGateEvidenceRef) bool {
	if values == nil || !slices.IsSortedFunc(values, compareHumanGateEvidenceRef) {
		return false
	}
	for index, value := range values {
		if value.SourceVersionID == "" || value.SourceStart < 0 || value.SourceEnd <= value.SourceStart ||
			!nodeOutputContentHashPattern.MatchString(value.TextHash) ||
			(index > 0 && compareHumanGateEvidenceRef(values[index-1], value) == 0) {
			return false
		}
	}
	return true
}

func validProductionWorldRepairReason(operation, reason string) bool {
	expected := map[string]string{
		ProductionWorldRepairReviseEntity:      "production_entity_incorrect",
		ProductionWorldRepairRebindOccurrence:  "scene_occurrence_incorrect",
		ProductionWorldRepairReviseInteraction: "interaction_incorrect",
		ProductionWorldRepairReviseContinuity:  "continuity_incorrect",
	}
	return reason == expected[operation]
}

func productionWorldSelectionWithinTargets(
	selection ProductionWorldRepairSelection,
	values []ProductionWorldRepairTargetSet,
) bool {
	if !validProductionWorldRepairSelection(selection) {
		return false
	}
	for _, value := range values {
		if value.Operation != selection.Operation {
			continue
		}
		for _, target := range selection.TargetKeys {
			if !slices.Contains(value.TargetKeys, target) {
				return false
			}
		}
		return true
	}
	return false
}

func productionWorldIssueRefsWithinDetail(
	values []string,
	issues []worlddomain.ProductionWorldReviewIssue,
) bool {
	available := make(map[string]struct{}, len(issues))
	for _, issue := range issues {
		available[issue.SourceStage+"/"+issue.Issue.IssueKey] = struct{}{}
	}
	for _, value := range values {
		if _, exists := available[value]; !exists {
			return false
		}
	}
	return true
}

func productionWorldRepairEvidence(
	views ProductionWorldReviewViews,
	closure ProductionWorldRepairClosure,
	sourceVersionID string,
) map[HumanGateEvidenceRef]struct{} {
	result := make(map[HumanGateEvidenceRef]struct{})
	add := func(value agentcontract.SourceEvidenceSpan) {
		result[HumanGateEvidenceRef{
			SourceVersionID: sourceVersionID, SourceStart: value.SourceStart,
			SourceEnd: value.SourceEnd, TextHash: value.TextHash,
		}] = struct{}{}
	}
	for _, collection := range [][]ProductionWorldEntityReviewItem{views.CharacterAppearances, views.Locations, views.PropStates} {
		for _, entity := range collection {
			if !slices.Contains(closure.EntityKeys, entity.IdentityKey) {
				continue
			}
			for _, evidence := range entity.Basis.Evidence {
				add(evidence)
			}
			for _, state := range entity.States {
				if !slices.Contains(closure.StateKeys, state.StateKey) {
					continue
				}
				for _, evidence := range state.Basis.Evidence {
					add(evidence)
				}
			}
		}
	}
	for _, scene := range views.SceneOccurrences {
		for _, occurrence := range scene.Occurrences {
			if slices.Contains(closure.OccurrenceKeys, occurrence.OccurrenceKey) {
				add(occurrence.Evidence)
			}
		}
	}
	for _, interaction := range views.Interactions {
		if !slices.Contains(closure.InteractionKeys, interaction.InteractionKey) {
			continue
		}
		add(interaction.Evidence)
		for _, evidence := range []*agentcontract.SourceEvidenceSpan{
			interaction.GeometryEvidence.Hand, interaction.GeometryEvidence.GripType,
			interaction.GeometryEvidence.ContactPoint, interaction.GeometryEvidence.Direction,
			interaction.GeometryEvidence.RelativeScale,
		} {
			if evidence != nil {
				add(*evidence)
			}
		}
	}
	for _, claim := range views.Continuity.Claims {
		if slices.Contains(closure.ContinuityKeys, claim.ContinuityKey) {
			for _, evidence := range claim.Evidence {
				add(evidence)
			}
		}
	}
	for _, entry := range views.Continuity.Ledger {
		if slices.Contains(closure.LedgerKeys, entry.LedgerKey) {
			for _, evidence := range entry.Evidence {
				add(evidence)
			}
		}
	}
	return result
}

func validProductionWorldRepairSelection(value ProductionWorldRepairSelection) bool {
	if !slices.Contains([]string{
		ProductionWorldRepairReviseEntity,
		ProductionWorldRepairRebindOccurrence,
		ProductionWorldRepairReviseInteraction,
		ProductionWorldRepairReviseContinuity,
	}, value.Operation) || len(value.TargetKeys) == 0 || !slices.IsSorted(value.TargetKeys) {
		return false
	}
	for index, key := range value.TargetKeys {
		if strings.TrimSpace(key) == "" || key != strings.TrimSpace(key) || (index > 0 && value.TargetKeys[index-1] == key) {
			return false
		}
	}
	return true
}

func newProductionWorldRepairIndex(views ProductionWorldReviewViews) (productionWorldRepairIndex, error) {
	if views.CharacterAppearances == nil || views.Locations == nil || views.PropStates == nil ||
		views.SceneOccurrences == nil || views.Interactions == nil || views.Continuity.Claims == nil ||
		views.Continuity.Ledger == nil {
		return productionWorldRepairIndex{}, errors.New("Production World repair graph is incomplete")
	}
	index := productionWorldRepairIndex{
		entities: make(map[string]ProductionWorldEntityReviewItem), stateOwners: make(map[string]string),
		scenes: make(map[string]ProductionWorldSceneOccurrenceReviewItem), occurrences: make(map[string]productionWorldOccurrenceRef),
		interactions: make(map[string]agentcontract.InteractionFragment), continuity: make(map[string]agentcontract.ContinuityFragment),
		ledger: make(map[string]agentcontract.ContinuityLedgerEntry),
	}
	for _, collection := range [][]ProductionWorldEntityReviewItem{views.CharacterAppearances, views.Locations, views.PropStates} {
		for _, entity := range collection {
			if strings.TrimSpace(entity.IdentityKey) == "" {
				return productionWorldRepairIndex{}, errors.New("Production World repair graph has an invalid entity")
			}
			if _, duplicate := index.entities[entity.IdentityKey]; duplicate {
				return productionWorldRepairIndex{}, errors.New("Production World repair graph has duplicate entity keys")
			}
			index.entities[entity.IdentityKey] = entity
			for _, state := range entity.States {
				if strings.TrimSpace(state.StateKey) == "" {
					return productionWorldRepairIndex{}, errors.New("Production World repair graph has an invalid state")
				}
				if _, duplicate := index.stateOwners[state.StateKey]; duplicate {
					return productionWorldRepairIndex{}, errors.New("Production World repair graph has duplicate state keys")
				}
				index.stateOwners[state.StateKey] = entity.IdentityKey
			}
		}
	}
	for _, scene := range views.SceneOccurrences {
		if strings.TrimSpace(scene.SceneScopeKey) == "" {
			return productionWorldRepairIndex{}, errors.New("Production World repair graph has an invalid Scene")
		}
		if _, duplicate := index.scenes[scene.SceneScopeKey]; duplicate {
			return productionWorldRepairIndex{}, errors.New("Production World repair graph has duplicate Scene keys")
		}
		index.scenes[scene.SceneScopeKey] = scene
		for _, occurrence := range scene.Occurrences {
			if _, exists := index.entities[occurrence.IdentityKey]; !exists || index.stateOwners[occurrence.StateKey] != occurrence.IdentityKey {
				return productionWorldRepairIndex{}, errors.New("Production World repair graph has an invalid Occurrence reference")
			}
			if _, duplicate := index.occurrences[occurrence.OccurrenceKey]; duplicate || strings.TrimSpace(occurrence.OccurrenceKey) == "" {
				return productionWorldRepairIndex{}, errors.New("Production World repair graph has duplicate Occurrence keys")
			}
			index.occurrences[occurrence.OccurrenceKey] = productionWorldOccurrenceRef{sceneScopeKey: scene.SceneScopeKey, value: occurrence}
		}
	}
	for _, interaction := range views.Interactions {
		if _, duplicate := index.interactions[interaction.InteractionKey]; duplicate || strings.TrimSpace(interaction.InteractionKey) == "" ||
			index.scenes[interaction.SceneScopeKey].SceneScopeKey == "" || index.occurrences[interaction.ActorOccurrenceKey].sceneScopeKey != interaction.SceneScopeKey ||
			index.occurrences[interaction.PropOccurrenceKey].sceneScopeKey != interaction.SceneScopeKey {
			return productionWorldRepairIndex{}, errors.New("Production World repair graph has an invalid Interaction reference")
		}
		if interaction.CounterpartyOccurrenceKey != nil && index.occurrences[*interaction.CounterpartyOccurrenceKey].sceneScopeKey != interaction.SceneScopeKey {
			return productionWorldRepairIndex{}, errors.New("Production World repair graph has an invalid Interaction counterparty")
		}
		index.interactions[interaction.InteractionKey] = interaction
	}
	for _, claim := range views.Continuity.Claims {
		if _, duplicate := index.continuity[claim.ContinuityKey]; duplicate || strings.TrimSpace(claim.ContinuityKey) == "" ||
			index.scenes[claim.FromSceneScopeKey].SceneScopeKey == "" || index.scenes[claim.ToSceneScopeKey].SceneScopeKey == "" ||
			index.entities[claim.IdentityKey].IdentityKey == "" || index.stateOwners[claim.BeforeStateKey] != claim.IdentityKey ||
			index.stateOwners[claim.AfterStateKey] != claim.IdentityKey {
			return productionWorldRepairIndex{}, errors.New("Production World repair graph has an invalid Continuity reference")
		}
		index.continuity[claim.ContinuityKey] = claim
	}
	for _, entry := range views.Continuity.Ledger {
		if _, duplicate := index.ledger[entry.LedgerKey]; duplicate || strings.TrimSpace(entry.LedgerKey) == "" ||
			index.scenes[entry.SceneScopeKey].SceneScopeKey == "" || index.entities[entry.IdentityKey].IdentityKey == "" ||
			index.stateOwners[entry.StateKey] != entry.IdentityKey ||
			(entry.TransitionInteractionKey != nil && index.interactions[*entry.TransitionInteractionKey].InteractionKey == "") {
			return productionWorldRepairIndex{}, errors.New("Production World repair graph has an invalid ledger reference")
		}
		index.ledger[entry.LedgerKey] = entry
	}
	return index, nil
}

func newProductionWorldRepairSets() productionWorldRepairSets {
	return productionWorldRepairSets{
		scenes: make(stringSet), entities: make(stringSet), states: make(stringSet), occurrences: make(stringSet),
		interactions: make(stringSet), continuity: make(stringSet), ledger: make(stringSet),
	}
}

func addProductionWorldOccurrence(
	sets productionWorldRepairSets,
	seedScenes stringSet,
	key string,
	occurrence productionWorldOccurrenceRef,
) {
	sets.occurrences.add(key)
	sets.scenes.add(occurrence.sceneScopeKey)
	seedScenes.add(occurrence.sceneScopeKey)
	sets.entities.add(occurrence.value.IdentityKey)
	sets.states.add(occurrence.value.StateKey)
}

func addProductionWorldInteraction(
	sets productionWorldRepairSets,
	seedScenes stringSet,
	index productionWorldRepairIndex,
	value agentcontract.InteractionFragment,
) {
	sets.interactions.add(value.InteractionKey)
	sets.scenes.add(value.SceneScopeKey)
	seedScenes.add(value.SceneScopeKey)
	for _, key := range []string{value.ActorOccurrenceKey, value.PropOccurrenceKey} {
		addProductionWorldOccurrence(sets, seedScenes, key, index.occurrences[key])
	}
	if value.CounterpartyOccurrenceKey != nil {
		addProductionWorldOccurrence(sets, seedScenes, *value.CounterpartyOccurrenceKey, index.occurrences[*value.CounterpartyOccurrenceKey])
	}
	sets.states.add(value.PropStateBeforeKey)
	sets.states.add(value.PropStateAfterKey)
	if owner := index.stateOwners[value.PropStateBeforeKey]; owner != "" {
		sets.entities.add(owner)
	}
	if value.HolderBeforeIdentityKey != nil {
		sets.entities.add(*value.HolderBeforeIdentityKey)
	}
	if value.HolderAfterIdentityKey != nil {
		sets.entities.add(*value.HolderAfterIdentityKey)
	}
}

func addProductionWorldContinuity(
	sets productionWorldRepairSets,
	index productionWorldRepairIndex,
	value agentcontract.ContinuityFragment,
) {
	sets.continuity.add(value.ContinuityKey)
	sets.scenes.add(value.FromSceneScopeKey)
	sets.scenes.add(value.ToSceneScopeKey)
	sets.entities.add(value.IdentityKey)
	sets.states.add(value.BeforeStateKey)
	sets.states.add(value.AfterStateKey)
	for key, occurrence := range index.occurrences {
		if (occurrence.sceneScopeKey == value.FromSceneScopeKey || occurrence.sceneScopeKey == value.ToSceneScopeKey) &&
			occurrence.value.IdentityKey == value.IdentityKey &&
			(occurrence.value.StateKey == value.BeforeStateKey || occurrence.value.StateKey == value.AfterStateKey) {
			sets.occurrences.add(key)
		}
	}
}

func productionWorldContinuityTouchesSeeds(
	value agentcontract.ContinuityFragment,
	operation string,
	seedScenes stringSet,
	sets productionWorldRepairSets,
) bool {
	if operation == ProductionWorldRepairReviseEntity {
		return sets.entities.has(value.IdentityKey) || sets.states.has(value.BeforeStateKey) || sets.states.has(value.AfterStateKey)
	}
	if !seedScenes.has(value.FromSceneScopeKey) && !seedScenes.has(value.ToSceneScopeKey) {
		return false
	}
	return sets.entities.has(value.IdentityKey) || sets.states.has(value.BeforeStateKey) || sets.states.has(value.AfterStateKey)
}

func productionWorldRepairClosureFromSets(
	selection ProductionWorldRepairSelection,
	sets productionWorldRepairSets,
) ProductionWorldRepairClosure {
	return ProductionWorldRepairClosure{
		Operation: selection.Operation, TargetKeys: append([]string(nil), selection.TargetKeys...),
		SceneScopeKeys: sortedSet(sets.scenes), EntityKeys: sortedSet(sets.entities), StateKeys: sortedSet(sets.states),
		OccurrenceKeys: sortedSet(sets.occurrences), InteractionKeys: sortedSet(sets.interactions),
		ContinuityKeys: sortedSet(sets.continuity), LedgerKeys: sortedSet(sets.ledger),
	}
}

func (set stringSet) add(value string) {
	if value != "" {
		set[value] = struct{}{}
	}
}

func (set stringSet) has(value string) bool {
	_, ok := set[value]
	return ok
}

func sortedSet(set stringSet) []string {
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}
