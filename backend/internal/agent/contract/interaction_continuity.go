package contract

import (
	"encoding/json"
	"errors"
	"regexp"

	"github.com/google/uuid"
)

const InteractionContinuityCandidateSchemaVersion = "continuity-fragment-candidate-production"

var (
	interactionKeyPattern = regexp.MustCompile(`^interaction_[a-z0-9_]{1,120}$`)
	continuityKeyPattern  = regexp.MustCompile(`^continuity_[a-z0-9_]{1,120}$`)
)

type InteractionContinuityInput struct {
	SceneOccurrenceBindingInput
	SceneBindingCandidateRevisionID   string          `json:"scene_binding_candidate_revision_id"`
	SceneBindingCandidateRevisionHash string          `json:"scene_binding_candidate_revision_hash"`
	SceneBindingCandidate             json.RawMessage `json:"scene_binding_candidate"`
}

func (value InteractionContinuityInput) Validate() error {
	if value.SceneOccurrenceBindingInput.Validate() != nil {
		return errors.New("invalid Interaction/Continuity frozen input")
	}
	if _, err := uuid.Parse(value.SceneBindingCandidateRevisionID); err != nil {
		return errors.New("invalid Scene Binding Candidate revision identity")
	}
	if !hashPattern.MatchString(value.SceneBindingCandidateRevisionHash) ||
		ValidateSceneBindingFragmentCandidate(
			value.SceneBindingCandidate,
			value.SceneOccurrenceBindingInput,
		) != nil {
		return errors.New("invalid frozen Scene Binding Candidate")
	}
	return nil
}

type InteractionFragment struct {
	InteractionKey            string             `json:"interaction_key"`
	SceneScopeKey             string             `json:"scene_scope_key"`
	BeatKey                   *string            `json:"beat_key"`
	Predicate                 string             `json:"predicate"`
	ActorOccurrenceKey        string             `json:"actor_occurrence_key"`
	PropOccurrenceKey         string             `json:"prop_occurrence_key"`
	CounterpartyOccurrenceKey *string            `json:"counterparty_occurrence_key"`
	HolderBeforeIdentityKey   *string            `json:"holder_before_identity_key"`
	HolderAfterIdentityKey    *string            `json:"holder_after_identity_key"`
	PropStateBeforeKey        string             `json:"prop_state_before_key"`
	PropStateAfterKey         string             `json:"prop_state_after_key"`
	Hand                      *string            `json:"hand"`
	ContactPoint              *string            `json:"contact_point"`
	Direction                 *string            `json:"direction"`
	RelativeScale             *string            `json:"relative_scale"`
	Evidence                  SourceEvidenceSpan `json:"evidence"`
}

type ContinuityFragment struct {
	ContinuityKey     string               `json:"continuity_key"`
	SubjectKind       string               `json:"subject_kind"`
	IdentityKey       string               `json:"identity_key"`
	FromSceneScopeKey string               `json:"from_scene_scope_key"`
	ToSceneScopeKey   string               `json:"to_scene_scope_key"`
	BeforeStateKey    string               `json:"before_state_key"`
	AfterStateKey     string               `json:"after_state_key"`
	Transition        string               `json:"transition"`
	Delta             *string              `json:"delta"`
	Evidence          []SourceEvidenceSpan `json:"evidence"`
}

type InteractionContinuityCandidate struct {
	SourceVersionID                       string                 `json:"source_version_id"`
	SourceHash                            string                 `json:"source_hash"`
	StructureIdentitySetVersionID         string                 `json:"structure_identity_set_version_id"`
	StructureIdentitySetVersionHash       string                 `json:"structure_identity_set_version_hash"`
	SceneFactCandidateRevisionID          string                 `json:"scene_fact_candidate_revision_id"`
	SceneFactCandidateRevisionHash        string                 `json:"scene_fact_candidate_revision_hash"`
	ProductionEntityCandidateRevisionID   string                 `json:"production_entity_candidate_revision_id"`
	ProductionEntityCandidateRevisionHash string                 `json:"production_entity_candidate_revision_hash"`
	SceneBindingCandidateRevisionID       string                 `json:"scene_binding_candidate_revision_id"`
	SceneBindingCandidateRevisionHash     string                 `json:"scene_binding_candidate_revision_hash"`
	Interactions                          []InteractionFragment  `json:"interactions"`
	Continuity                            []ContinuityFragment   `json:"continuity"`
	ReviewIssues                          []CandidateReviewIssue `json:"review_issues"`
}

type occurrenceBinding struct {
	sceneKey   string
	occurrence SceneOccurrenceFragment
}

type productionStateIdentity struct {
	identityKey string
	kind        string
}

func ValidateInteractionContinuityCandidate(
	raw json.RawMessage,
	input InteractionContinuityInput,
) error {
	if err := input.Validate(); err != nil {
		return err
	}
	var value InteractionContinuityCandidate
	if decodeStrict(raw, &value) != nil || value.Interactions == nil || value.Continuity == nil ||
		value.ReviewIssues == nil || value.SourceVersionID != input.SourceVersionID ||
		value.SourceHash != input.SourceHash ||
		value.StructureIdentitySetVersionID != input.StructureIdentitySetVersionID ||
		value.StructureIdentitySetVersionHash != input.StructureIdentitySetVersionHash ||
		value.SceneFactCandidateRevisionID != input.SceneFactCandidateRevisionID ||
		value.SceneFactCandidateRevisionHash != input.SceneFactCandidateRevisionHash ||
		value.ProductionEntityCandidateRevisionID != input.ProductionEntityCandidateRevisionID ||
		value.ProductionEntityCandidateRevisionHash != input.ProductionEntityCandidateRevisionHash ||
		value.SceneBindingCandidateRevisionID != input.SceneBindingCandidateRevisionID ||
		value.SceneBindingCandidateRevisionHash != input.SceneBindingCandidateRevisionHash {
		return errors.New("Interaction/Continuity Candidate lineage drifted")
	}

	var bindings SceneBindingFragmentCandidate
	var production ProductionEntityFragmentCandidate
	var facts SceneFactCandidate
	if decodeStrict(input.SceneBindingCandidate, &bindings) != nil ||
		decodeStrict(input.ProductionEntityCandidate, &production) != nil ||
		decodeStrict(input.SceneFactCandidate, &facts) != nil {
		return errors.New("invalid frozen Interaction/Continuity Candidates")
	}
	scenes := make(map[string]SceneBindingFragment, len(bindings.Scenes))
	sceneOrder := make(map[string]int, len(bindings.Scenes))
	occurrences := make(map[string]occurrenceBinding)
	for index, scene := range bindings.Scenes {
		scenes[scene.SceneScopeKey] = scene
		sceneOrder[scene.SceneScopeKey] = index
		for _, occurrence := range scene.Occurrences {
			occurrences[occurrence.OccurrenceKey] = occurrenceBinding{scene.SceneScopeKey, occurrence}
		}
	}
	states := make(map[string]productionStateIdentity)
	for _, entity := range production.Entities {
		for _, state := range entity.States {
			states[state.StateKey] = productionStateIdentity{entity.IdentityKey, entity.Kind}
		}
	}
	actionEvidence := make(map[string]map[string]struct{})
	formalSceneByTemporary := make(map[string]string, len(input.StructureIdentitySet.SceneRefs))
	for _, scene := range input.StructureIdentitySet.SceneRefs {
		formalSceneByTemporary[scene.TemporarySceneID] = scene.ScopeKey
	}
	for _, fact := range facts.Scenes {
		sceneKey := formalSceneByTemporary[fact.TemporarySceneID]
		actionEvidence[sceneKey] = make(map[string]struct{}, len(fact.Actions))
		for _, action := range fact.Actions {
			actionEvidence[sceneKey][sourceEvidenceKey(action.Evidence)] = struct{}{}
		}
	}

	interactionKeys := make(map[string]struct{}, len(value.Interactions))
	for _, interaction := range value.Interactions {
		if err := validateInteraction(
			interaction,
			scenes,
			occurrences,
			states,
			actionEvidence,
		); err != nil {
			return err
		}
		if _, duplicate := interactionKeys[interaction.InteractionKey]; duplicate {
			return errors.New("Interaction key is duplicated")
		}
		interactionKeys[interaction.InteractionKey] = struct{}{}
	}

	evidenceUniverse := productionEntityEvidenceUniverse(input.SceneFactCandidate)
	continuityKeys := make(map[string]struct{}, len(value.Continuity))
	for _, continuity := range value.Continuity {
		if err := validateContinuity(
			continuity,
			sceneOrder,
			occurrences,
			states,
			evidenceUniverse,
		); err != nil {
			return err
		}
		if _, duplicate := continuityKeys[continuity.ContinuityKey]; duplicate {
			return errors.New("Continuity key is duplicated")
		}
		continuityKeys[continuity.ContinuityKey] = struct{}{}
	}
	runes := []rune(input.NormalizedText)
	previousIssue := ""
	for _, issue := range value.ReviewIssues {
		if issue.IssueKey <= previousIssue || validateCandidateReviewIssue(issue, runes, true) != nil {
			return errors.New("invalid Interaction/Continuity ReviewIssue")
		}
		for _, evidence := range issue.Evidence {
			if _, exists := evidenceUniverse[productionEvidenceKey(evidence)]; !exists {
				return errors.New("Interaction/Continuity ReviewIssue Evidence is outside SceneFacts")
			}
		}
		previousIssue = issue.IssueKey
	}
	return nil
}

func validateInteraction(
	value InteractionFragment,
	scenes map[string]SceneBindingFragment,
	occurrences map[string]occurrenceBinding,
	states map[string]productionStateIdentity,
	actionEvidence map[string]map[string]struct{},
) error {
	scene, sceneExists := scenes[value.SceneScopeKey]
	actor, actorExists := occurrences[value.ActorOccurrenceKey]
	prop, propExists := occurrences[value.PropOccurrenceKey]
	_, evidenceExists := actionEvidence[value.SceneScopeKey][sourceEvidenceKey(value.Evidence)]
	before := states[value.PropStateBeforeKey]
	after := states[value.PropStateAfterKey]
	if !interactionKeyPattern.MatchString(value.InteractionKey) || !sceneExists || !actorExists ||
		!propExists || actor.sceneKey != value.SceneScopeKey || prop.sceneKey != value.SceneScopeKey ||
		actor.occurrence.SubjectKind != "character" || prop.occurrence.SubjectKind != "prop" ||
		actor.occurrence.OccurrenceRole != "actual" || prop.occurrence.OccurrenceRole != "actual" ||
		before != (productionStateIdentity{prop.occurrence.IdentityKey, "prop"}) ||
		after != (productionStateIdentity{prop.occurrence.IdentityKey, "prop"}) || !evidenceExists ||
		!interactionValueIn(value.Predicate, "hold", "carry", "wear", "use", "give", "receive", "place", "drop", "open", "break") {
		return errors.New("Interaction does not bind exact actual occurrences and Prop states")
	}
	if value.BeatKey != nil {
		found := false
		for _, beat := range scene.Beats {
			found = found || beat.BeatKey == *value.BeatKey
		}
		if !found {
			return errors.New("Interaction references an unknown Scene Beat")
		}
	}
	participants := map[string]struct{}{actor.occurrence.IdentityKey: {}}
	var counterparty occurrenceBinding
	if interactionValueIn(value.Predicate, "give", "receive") {
		if value.CounterpartyOccurrenceKey == nil {
			return errors.New("transfer Interaction requires a counterparty")
		}
		var exists bool
		counterparty, exists = occurrences[*value.CounterpartyOccurrenceKey]
		if !exists || counterparty.sceneKey != value.SceneScopeKey ||
			counterparty.occurrence.SubjectKind != "character" ||
			counterparty.occurrence.OccurrenceRole != "actual" ||
			counterparty.occurrence.IdentityKey == actor.occurrence.IdentityKey {
			return errors.New("transfer Interaction counterparty is invalid")
		}
		participants[counterparty.occurrence.IdentityKey] = struct{}{}
	} else if value.CounterpartyOccurrenceKey != nil {
		return errors.New("non-transfer Interaction cannot add a counterparty")
	}
	for _, holder := range []*string{value.HolderBeforeIdentityKey, value.HolderAfterIdentityKey} {
		if holder != nil {
			if _, exists := participants[*holder]; !exists {
				return errors.New("Interaction holder is not a participant")
			}
		}
	}
	if interactionValueIn(value.Predicate, "hold", "carry", "wear", "use") &&
		(value.HolderAfterIdentityKey == nil || *value.HolderAfterIdentityKey != actor.occurrence.IdentityKey) {
		return errors.New("possession Interaction must end with the actor")
	}
	if interactionValueIn(value.Predicate, "place", "drop") && value.HolderAfterIdentityKey != nil {
		return errors.New("release Interaction must end without a holder")
	}
	if interactionValueIn(value.Predicate, "give", "receive") &&
		(value.HolderBeforeIdentityKey == nil || value.HolderAfterIdentityKey == nil ||
			*value.HolderBeforeIdentityKey != actor.occurrence.IdentityKey ||
			*value.HolderAfterIdentityKey != counterparty.occurrence.IdentityKey) {
		return errors.New("transfer Interaction holder transition is invalid")
	}
	if interactionValueIn(value.Predicate, "open", "break") && value.PropStateBeforeKey == value.PropStateAfterKey {
		return errors.New("Prop mutation Interaction must change state")
	}
	return nil
}

func validateContinuity(
	value ContinuityFragment,
	sceneOrder map[string]int,
	occurrences map[string]occurrenceBinding,
	states map[string]productionStateIdentity,
	evidenceUniverse map[string]struct{},
) error {
	fromOrder, fromExists := sceneOrder[value.FromSceneScopeKey]
	toOrder, toExists := sceneOrder[value.ToSceneScopeKey]
	if !continuityKeyPattern.MatchString(value.ContinuityKey) ||
		!interactionValueIn(value.SubjectKind, "character", "location", "prop") || !fromExists || !toExists ||
		fromOrder >= toOrder || states[value.BeforeStateKey] != (productionStateIdentity{
		identityKey: value.IdentityKey, kind: value.SubjectKind,
	}) || states[value.AfterStateKey] != (productionStateIdentity{
		identityKey: value.IdentityKey, kind: value.SubjectKind,
	}) || len(value.Evidence) == 0 {
		return errors.New("Continuity does not bind an ordered exact identity/state timeline")
	}
	seenFrom, seenTo := false, false
	for _, occurrence := range occurrences {
		if occurrence.occurrence.IdentityKey != value.IdentityKey ||
			occurrence.occurrence.OccurrenceRole != "actual" {
			continue
		}
		seenFrom = seenFrom || occurrence.sceneKey == value.FromSceneScopeKey
		seenTo = seenTo || occurrence.sceneKey == value.ToSceneScopeKey
	}
	if !seenFrom || !seenTo {
		return errors.New("Continuity requires actual occurrences at both anchors")
	}
	if value.Transition == "state_persists" {
		if value.BeforeStateKey != value.AfterStateKey || value.Delta != nil {
			return errors.New("state_persists must preserve state without a delta")
		}
	} else if value.Transition != "state_changes" || value.BeforeStateKey == value.AfterStateKey ||
		value.Delta == nil || *value.Delta == "" {
		return errors.New("state_changes must bind distinct states and a delta")
	}
	for _, evidence := range value.Evidence {
		if _, exists := evidenceUniverse[productionEvidenceKey(evidence)]; !exists {
			return errors.New("Continuity Evidence is outside frozen SceneFacts")
		}
	}
	return nil
}

func interactionValueIn(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
