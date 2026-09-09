package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"
)

const (
	ProductionWorldRepairReviseEntity      = "revise_production_entity"
	ProductionWorldRepairRebindOccurrence  = "rebind_scene_occurrence"
	ProductionWorldRepairReviseInteraction = "revise_interaction"
	ProductionWorldRepairReviseContinuity  = "revise_continuity"
)

type ProductionWorldRepairChange struct {
	Operation  string   `json:"operation"`
	TargetKeys []string `json:"target_keys"`
}

type ProductionWorldRepairClosure struct {
	SceneScopeKeys  []string `json:"scene_scope_keys"`
	EntityKeys      []string `json:"entity_keys"`
	StateKeys       []string `json:"state_keys"`
	OccurrenceKeys  []string `json:"occurrence_keys"`
	InteractionKeys []string `json:"interaction_keys"`
	ContinuityKeys  []string `json:"continuity_keys"`
	LedgerKeys      []string `json:"ledger_keys"`
}

type ProductionWorldRepairBaseCandidate struct {
	Identity             SceneAnalysisCandidateRevisionIdentity `json:"identity"`
	CandidateType        string                                 `json:"candidate_type"`
	CandidateContentHash string                                 `json:"candidate_content_hash"`
	Candidate            json.RawMessage                        `json:"candidate"`
}

type ProductionWorldRepairDirective struct {
	ReviewDecisionID    string                             `json:"review_decision_id"`
	DecisionPayloadHash string                             `json:"decision_payload_hash"`
	IssueRefs           []string                           `json:"issue_refs"`
	EvidenceRefs        []StructureIdentityRepairEvidence  `json:"evidence_refs"`
	ChangeSpec          ProductionWorldRepairChange        `json:"change_spec"`
	Closure             ProductionWorldRepairClosure       `json:"closure"`
	ReasonCode          string                             `json:"reason_code"`
	BaseCandidate       ProductionWorldRepairBaseCandidate `json:"base_candidate"`
}

func (value ProductionWorldRepairDirective) ValidateFor(stageKey string) error {
	if _, err := uuid.Parse(value.ReviewDecisionID); err != nil ||
		!hashPattern.MatchString(value.DecisionPayloadHash) || value.IssueRefs == nil ||
		len(value.EvidenceRefs) == 0 || !validSortedProductionWorldKeys(value.ChangeSpec.TargetKeys, false) ||
		!validProductionWorldRepairReason(value.ChangeSpec.Operation, value.ReasonCode) {
		return errors.New("invalid Production World repair directive")
	}
	for index, issue := range value.IssueRefs {
		if strings.TrimSpace(issue) == "" || (index > 0 && value.IssueRefs[index-1] >= issue) {
			return errors.New("invalid Production World repair issue")
		}
	}
	for index, evidence := range value.EvidenceRefs {
		if _, err := uuid.Parse(evidence.SourceVersionID); err != nil || evidence.SourceStart < 0 ||
			evidence.SourceEnd <= evidence.SourceStart || !hashPattern.MatchString(evidence.TextHash) ||
			(index > 0 && compareStructureIdentityRepairEvidence(value.EvidenceRefs[index-1], evidence) >= 0) {
			return errors.New("invalid Production World repair evidence")
		}
	}
	if err := value.Closure.validate(); err != nil || !productionWorldTargetsInsideClosure(value.ChangeSpec, value.Closure) {
		return errors.New("invalid Production World repair closure")
	}
	rootStage := productionWorldRepairRootStage(value.ChangeSpec.Operation)
	if rootStage == "" || !productionWorldRepairStageContains(stageKey, rootStage) {
		return errors.New("Production World repair operation targets another stage")
	}
	if err := value.BaseCandidate.validateFor(stageKey); err != nil {
		return err
	}
	return nil
}

func ValidateProductionWorldRepairCandidate(
	directive ProductionWorldRepairDirective,
	stageKey string,
	candidate json.RawMessage,
) error {
	if err := directive.ValidateFor(stageKey); err != nil {
		return err
	}
	base, err := productionWorldRepairObject(directive.BaseCandidate.Candidate)
	if err != nil {
		return err
	}
	current, err := productionWorldRepairObject(candidate)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(base, current) {
		return errors.New("Production World repair Candidate did not change")
	}
	switch stageKey {
	case "derive_production_entities":
		err = validateProductionEntityRepairPreservation(directive, base, current)
	case "bind_scene_occurrences":
		err = validateSceneOccurrenceRepairPreservation(directive, base, current)
	case "reconcile_interaction_continuity":
		err = validateInteractionContinuityRepairPreservation(directive, base, current)
	default:
		err = errors.New("Production World repair Candidate targets another stage")
	}
	if err != nil {
		return errors.New("Production World repair Candidate changed content outside its authorized closure")
	}
	return nil
}

func validateProductionEntityRepairPreservation(
	directive ProductionWorldRepairDirective,
	base, current map[string]any,
) error {
	if directive.ChangeSpec.Operation != ProductionWorldRepairReviseEntity ||
		!productionWorldSameExcept(base, current, "entities", "design_gaps", "review_issues") {
		return errors.New("invalid Production Entity repair")
	}
	baseEntities, err := productionWorldIndexedObjects(base, "entities", "identity_key")
	if err != nil {
		return err
	}
	currentEntities, err := productionWorldIndexedObjects(current, "entities", "identity_key")
	if err != nil || !productionWorldSameObjectKeys(baseEntities, currentEntities) {
		return errors.New("Production Entity keys drifted")
	}
	targets := productionWorldStringSet(directive.ChangeSpec.TargetKeys)
	authorizedGapSubjects := make(map[string]struct{})
	for identityKey, baseEntity := range baseEntities {
		currentEntity := currentEntities[identityKey]
		baseStates, stateErr := productionWorldIndexedObjects(baseEntity, "states", "state_key")
		if stateErr != nil {
			return stateErr
		}
		currentStates, stateErr := productionWorldIndexedObjects(currentEntity, "states", "state_key")
		if stateErr != nil || !productionWorldSameObjectKeys(baseStates, currentStates) {
			return errors.New("Production State keys drifted")
		}
		if _, entityTargeted := targets[identityKey]; entityTargeted {
			if !productionWorldSameFields(baseEntity, currentEntity, "identity_key", "kind", "specification_key") {
				return errors.New("Production Entity stable identity drifted")
			}
			authorizedGapSubjects[identityKey] = struct{}{}
			if specificationKey, ok := baseEntity["specification_key"].(string); ok {
				authorizedGapSubjects[specificationKey] = struct{}{}
			}
			for stateKey := range baseStates {
				authorizedGapSubjects[stateKey] = struct{}{}
			}
			continue
		}
		if !productionWorldSameExcept(baseEntity, currentEntity, "states") {
			return errors.New("untargeted Production Entity changed")
		}
		for stateKey, baseState := range baseStates {
			if _, stateTargeted := targets[stateKey]; stateTargeted {
				authorizedGapSubjects[stateKey] = struct{}{}
				continue
			}
			if !reflect.DeepEqual(baseState, currentStates[stateKey]) {
				return errors.New("untargeted Production State changed")
			}
		}
	}
	if !productionWorldPreservesAuthorizedObjects(
		base, current, "design_gaps", "gap_key",
		func(value map[string]any) bool {
			subject, ok := value["subject_key"].(string)
			_, authorized := authorizedGapSubjects[subject]
			return ok && authorized
		},
	) || !productionWorldPreservesReviewIssues(directive, "derive_production_entities", base, current) {
		return errors.New("Production Entity supporting collections drifted")
	}
	return nil
}

func validateSceneOccurrenceRepairPreservation(
	directive ProductionWorldRepairDirective,
	base, current map[string]any,
) error {
	if directive.ChangeSpec.Operation != ProductionWorldRepairReviseEntity &&
		directive.ChangeSpec.Operation != ProductionWorldRepairRebindOccurrence {
		return errors.New("invalid Scene Occurrence repair")
	}
	if !productionWorldSameExcept(base, current, "scenes", "review_issues") {
		return errors.New("Scene Occurrence lineage drifted")
	}
	baseScenes, err := productionWorldIndexedObjects(base, "scenes", "scene_scope_key")
	if err != nil {
		return err
	}
	currentScenes, err := productionWorldIndexedObjects(current, "scenes", "scene_scope_key")
	if err != nil || !productionWorldSameObjectKeys(baseScenes, currentScenes) {
		return errors.New("Scene binding keys drifted")
	}
	targets := productionWorldStringSet(directive.ChangeSpec.TargetKeys)
	allowedOccurrences := productionWorldStringSet(directive.Closure.OccurrenceKeys)
	for sceneKey, baseScene := range baseScenes {
		currentScene := currentScenes[sceneKey]
		if !productionWorldSameExcept(baseScene, currentScene, "occurrences") {
			return errors.New("frozen Scene content changed")
		}
		baseOccurrences, occurrenceErr := productionWorldIndexedObjects(baseScene, "occurrences", "occurrence_key")
		if occurrenceErr != nil {
			return occurrenceErr
		}
		currentOccurrences, occurrenceErr := productionWorldIndexedObjects(currentScene, "occurrences", "occurrence_key")
		if occurrenceErr != nil || !productionWorldSameObjectKeys(baseOccurrences, currentOccurrences) {
			return errors.New("Occurrence keys drifted")
		}
		_, wholeSceneTargeted := targets[sceneKey]
		for occurrenceKey, baseOccurrence := range baseOccurrences {
			_, allowed := allowedOccurrences[occurrenceKey]
			if directive.ChangeSpec.Operation == ProductionWorldRepairRebindOccurrence {
				_, explicitlyTargeted := targets[occurrenceKey]
				allowed = wholeSceneTargeted || explicitlyTargeted
			}
			if !allowed && !reflect.DeepEqual(baseOccurrence, currentOccurrences[occurrenceKey]) {
				return errors.New("untargeted Occurrence changed")
			}
		}
	}
	if !productionWorldPreservesReviewIssues(directive, "bind_scene_occurrences", base, current) {
		return errors.New("Scene binding review issues drifted")
	}
	return nil
}

func validateInteractionContinuityRepairPreservation(
	directive ProductionWorldRepairDirective,
	base, current map[string]any,
) error {
	if !productionWorldSameExcept(
		base, current, "interactions", "continuity", "continuity_ledger", "review_issues",
	) {
		return errors.New("Interaction/Continuity lineage or story time drifted")
	}
	interactionAllowed := make(map[string]struct{})
	continuityAllowed := make(map[string]struct{})
	switch directive.ChangeSpec.Operation {
	case ProductionWorldRepairReviseEntity, ProductionWorldRepairRebindOccurrence:
		interactionAllowed = productionWorldStringSet(directive.Closure.InteractionKeys)
		continuityAllowed = productionWorldStringSet(directive.Closure.ContinuityKeys)
	case ProductionWorldRepairReviseInteraction:
		interactionAllowed = productionWorldStringSet(directive.ChangeSpec.TargetKeys)
		continuityAllowed = productionWorldStringSet(directive.Closure.ContinuityKeys)
	case ProductionWorldRepairReviseContinuity:
		continuityAllowed = productionWorldStringSet(directive.ChangeSpec.TargetKeys)
	default:
		return errors.New("invalid Interaction/Continuity repair")
	}
	if !productionWorldPreservesFixedKeys(base, current, "interactions", "interaction_key", interactionAllowed) ||
		!productionWorldPreservesFixedKeys(base, current, "continuity", "continuity_key", continuityAllowed) ||
		!productionWorldPreservesFixedKeys(
			base, current, "continuity_ledger", "ledger_key", productionWorldStringSet(directive.Closure.LedgerKeys),
		) || !productionWorldPreservesReviewIssues(directive, "reconcile_interaction_continuity", base, current) {
		return errors.New("Interaction/Continuity closure drifted")
	}
	return nil
}

func productionWorldRepairObject(raw json.RawMessage) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if decoder.Decode(&value) != nil || value == nil {
		return nil, errors.New("invalid Production World repair Candidate")
	}
	return value, nil
}

func productionWorldIndexedObjects(root map[string]any, field, keyField string) (map[string]map[string]any, error) {
	values, ok := root[field].([]any)
	if !ok {
		return nil, errors.New("invalid Production World repair Candidate collection")
	}
	result := make(map[string]map[string]any, len(values))
	for _, raw := range values {
		value, objectOK := raw.(map[string]any)
		key, keyOK := value[keyField].(string)
		if !objectOK || !keyOK || strings.TrimSpace(key) == "" {
			return nil, errors.New("invalid Production World repair Candidate item")
		}
		if _, duplicate := result[key]; duplicate {
			return nil, errors.New("duplicate Production World repair Candidate item")
		}
		result[key] = value
	}
	return result, nil
}

func productionWorldSameExcept(left, right map[string]any, fields ...string) bool {
	leftCopy, rightCopy := make(map[string]any, len(left)), make(map[string]any, len(right))
	for key, value := range left {
		leftCopy[key] = value
	}
	for key, value := range right {
		rightCopy[key] = value
	}
	for _, field := range fields {
		delete(leftCopy, field)
		delete(rightCopy, field)
	}
	return reflect.DeepEqual(leftCopy, rightCopy)
}

func productionWorldSameFields(left, right map[string]any, fields ...string) bool {
	for _, field := range fields {
		if !reflect.DeepEqual(left[field], right[field]) {
			return false
		}
	}
	return true
}

func productionWorldSameObjectKeys(left, right map[string]map[string]any) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if _, exists := right[key]; !exists {
			return false
		}
	}
	return true
}

func productionWorldPreservesFixedKeys(
	base, current map[string]any,
	field, keyField string,
	allowed map[string]struct{},
) bool {
	baseItems, err := productionWorldIndexedObjects(base, field, keyField)
	if err != nil {
		return false
	}
	currentItems, err := productionWorldIndexedObjects(current, field, keyField)
	if err != nil || !productionWorldSameObjectKeys(baseItems, currentItems) {
		return false
	}
	for key, baseItem := range baseItems {
		if _, mutable := allowed[key]; !mutable && !reflect.DeepEqual(baseItem, currentItems[key]) {
			return false
		}
	}
	return true
}

func productionWorldPreservesAuthorizedObjects(
	base, current map[string]any,
	field, keyField string,
	authorized func(map[string]any) bool,
) bool {
	baseItems, err := productionWorldIndexedObjects(base, field, keyField)
	if err != nil {
		return false
	}
	currentItems, err := productionWorldIndexedObjects(current, field, keyField)
	if err != nil {
		return false
	}
	keys := make(map[string]struct{}, len(baseItems)+len(currentItems))
	for key := range baseItems {
		keys[key] = struct{}{}
	}
	for key := range currentItems {
		keys[key] = struct{}{}
	}
	for key := range keys {
		left, leftExists := baseItems[key]
		right, rightExists := currentItems[key]
		if leftExists && rightExists && reflect.DeepEqual(left, right) {
			continue
		}
		if (leftExists && !authorized(left)) || (rightExists && !authorized(right)) {
			return false
		}
	}
	return true
}

func productionWorldPreservesReviewIssues(
	directive ProductionWorldRepairDirective,
	stage string,
	base, current map[string]any,
) bool {
	allowed := make(map[string]struct{})
	prefix := stage + "/"
	for _, reference := range directive.IssueRefs {
		if strings.HasPrefix(reference, prefix) {
			allowed[strings.TrimPrefix(reference, prefix)] = struct{}{}
		}
	}
	return productionWorldPreservesAuthorizedObjects(
		base, current, "review_issues", "issue_key",
		func(value map[string]any) bool {
			key, ok := value["issue_key"].(string)
			_, exists := allowed[key]
			return ok && exists
		},
	)
}

func productionWorldStringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func (value ProductionWorldRepairClosure) validate() error {
	collections := [][]string{
		value.SceneScopeKeys, value.EntityKeys, value.StateKeys, value.OccurrenceKeys,
		value.InteractionKeys, value.ContinuityKeys, value.LedgerKeys,
	}
	for _, collection := range collections {
		if !validSortedProductionWorldKeys(collection, true) {
			return errors.New("invalid Production World repair closure keys")
		}
	}
	return nil
}

func (value ProductionWorldRepairBaseCandidate) validateFor(stageKey string) error {
	expectedType := map[string]string{
		"derive_production_entities":       "production_entity_fragment_candidate",
		"bind_scene_occurrences":           "scene_binding_fragment_candidate",
		"reconcile_interaction_continuity": "continuity_fragment_candidate",
	}[stageKey]
	if expectedType == "" || value.Identity.Validate() != nil || value.Identity.StageKey != stageKey ||
		value.Identity.ShardKey != "script:full" || value.CandidateType != expectedType ||
		!hashPattern.MatchString(value.CandidateContentHash) || len(value.Candidate) == 0 {
		return errors.New("invalid Production World repair base Candidate")
	}
	decoder := json.NewDecoder(bytes.NewReader(value.Candidate))
	decoder.UseNumber()
	var body map[string]any
	if decoder.Decode(&body) != nil || body == nil {
		return errors.New("invalid Production World repair base Candidate")
	}
	contentHash, err := CanonicalHash(value.Candidate)
	if err != nil || contentHash != value.CandidateContentHash {
		return errors.New("Production World repair base Candidate content has drifted")
	}
	return nil
}

func validSortedProductionWorldKeys(values []string, allowEmpty bool) bool {
	if values == nil || (!allowEmpty && len(values) == 0) || !slices.IsSorted(values) {
		return false
	}
	for index, value := range values {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) ||
			(index > 0 && values[index-1] == value) {
			return false
		}
	}
	return true
}

func validProductionWorldRepairReason(operation, reason string) bool {
	return map[string]string{
		ProductionWorldRepairReviseEntity:      "production_entity_incorrect",
		ProductionWorldRepairRebindOccurrence:  "scene_occurrence_incorrect",
		ProductionWorldRepairReviseInteraction: "interaction_incorrect",
		ProductionWorldRepairReviseContinuity:  "continuity_incorrect",
	}[operation] == reason
}

func productionWorldTargetsInsideClosure(
	change ProductionWorldRepairChange,
	closure ProductionWorldRepairClosure,
) bool {
	allowed := []string(nil)
	switch change.Operation {
	case ProductionWorldRepairReviseEntity:
		allowed = append(append([]string(nil), closure.EntityKeys...), closure.StateKeys...)
	case ProductionWorldRepairRebindOccurrence:
		allowed = append(append([]string(nil), closure.SceneScopeKeys...), closure.OccurrenceKeys...)
	case ProductionWorldRepairReviseInteraction:
		allowed = closure.InteractionKeys
	case ProductionWorldRepairReviseContinuity:
		allowed = closure.ContinuityKeys
	default:
		return false
	}
	for _, target := range change.TargetKeys {
		if !slices.Contains(allowed, target) {
			return false
		}
	}
	return true
}

func productionWorldRepairRootStage(operation string) string {
	switch operation {
	case ProductionWorldRepairReviseEntity:
		return "derive_production_entities"
	case ProductionWorldRepairRebindOccurrence:
		return "bind_scene_occurrences"
	case ProductionWorldRepairReviseInteraction, ProductionWorldRepairReviseContinuity:
		return "reconcile_interaction_continuity"
	default:
		return ""
	}
}

func productionWorldRepairStageContains(stageKey, rootStage string) bool {
	order := map[string]int{
		"derive_production_entities": 1, "bind_scene_occurrences": 2, "reconcile_interaction_continuity": 3,
	}
	return order[rootStage] > 0 && order[stageKey] >= order[rootStage]
}
