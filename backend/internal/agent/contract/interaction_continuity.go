package contract

import (
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const InteractionContinuityCandidateSchemaVersion = "continuity-fragment-candidate-production"

var (
	interactionKeyPattern       = regexp.MustCompile(`^interaction_[a-z0-9_]{1,120}$`)
	continuityKeyPattern        = regexp.MustCompile(`^continuity_[a-z0-9_]{1,120}$`)
	interactionSeriesKeyPattern = regexp.MustCompile(`^interaction_series_[a-z0-9_]{1,120}$`)
	continuitySeriesKeyPattern  = regexp.MustCompile(`^continuity_series_[a-z0-9_]{1,120}$`)
	continuityLedgerKeyPattern  = regexp.MustCompile(`^ledger_[a-z0-9_]{1,120}$`)
	storyTimeKeyPattern         = regexp.MustCompile(`^storytime:[a-z0-9][a-z0-9_.:-]{0,127}$`)
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

type PositiveRational struct {
	Numerator   int64 `json:"numerator"`
	Denominator int64 `json:"denominator"`
}

type SceneStoryTimeFragment struct {
	SceneScopeKey string `json:"scene_scope_key"`
	StoryTimeKey  string `json:"story_time_key"`
}

type InteractionGeometryEvidence struct {
	Hand          *SourceEvidenceSpan `json:"hand"`
	GripType      *SourceEvidenceSpan `json:"grip_type"`
	ContactPoint  *SourceEvidenceSpan `json:"contact_point"`
	Direction     *SourceEvidenceSpan `json:"direction"`
	RelativeScale *SourceEvidenceSpan `json:"relative_scale"`
}

type InteractionFragment struct {
	InteractionKey            string                      `json:"interaction_key"`
	ClaimSeriesKey            string                      `json:"claim_series_key"`
	ClaimRevision             int                         `json:"claim_revision"`
	SupersedesInteractionKey  *string                     `json:"supersedes_interaction_key"`
	SceneScopeKey             string                      `json:"scene_scope_key"`
	BeatKey                   *string                     `json:"beat_key"`
	StoryTimeKey              string                      `json:"story_time_key"`
	Predicate                 string                      `json:"predicate"`
	ActorOccurrenceKey        string                      `json:"actor_occurrence_key"`
	PropOccurrenceKey         string                      `json:"prop_occurrence_key"`
	CounterpartyOccurrenceKey *string                     `json:"counterparty_occurrence_key"`
	HolderBeforeIdentityKey   *string                     `json:"holder_before_identity_key"`
	HolderAfterIdentityKey    *string                     `json:"holder_after_identity_key"`
	PropStateBeforeKey        string                      `json:"prop_state_before_key"`
	PropStateAfterKey         string                      `json:"prop_state_after_key"`
	StateDelta                *string                     `json:"state_delta"`
	Hand                      string                      `json:"hand"`
	GripType                  *string                     `json:"grip_type"`
	ContactPoint              *string                     `json:"contact_point"`
	Direction                 *string                     `json:"direction"`
	RelativeScale             *PositiveRational           `json:"relative_scale"`
	GeometryEvidence          InteractionGeometryEvidence `json:"geometry_evidence"`
	Evidence                  SourceEvidenceSpan          `json:"evidence"`
}

type ContinuityFragment struct {
	ContinuityKey           string               `json:"continuity_key"`
	ClaimSeriesKey          string               `json:"claim_series_key"`
	ClaimRevision           int                  `json:"claim_revision"`
	SupersedesContinuityKey *string              `json:"supersedes_continuity_key"`
	SubjectKind             string               `json:"subject_kind"`
	IdentityKey             string               `json:"identity_key"`
	FromSceneScopeKey       string               `json:"from_scene_scope_key"`
	ToSceneScopeKey         string               `json:"to_scene_scope_key"`
	StoryTimeStart          string               `json:"story_time_start"`
	StoryTimeEnd            string               `json:"story_time_end"`
	BeforeStateKey          string               `json:"before_state_key"`
	AfterStateKey           string               `json:"after_state_key"`
	Transition              string               `json:"transition"`
	Delta                   *string              `json:"delta"`
	Evidence                []SourceEvidenceSpan `json:"evidence"`
}

type ContinuityLedgerEntry struct {
	LedgerKey                string               `json:"ledger_key"`
	SubjectKind              string               `json:"subject_kind"`
	IdentityKey              string               `json:"identity_key"`
	SceneScopeKey            string               `json:"scene_scope_key"`
	StoryTimeKey             string               `json:"story_time_key"`
	StateKey                 string               `json:"state_key"`
	HolderIdentityKey        *string              `json:"holder_identity_key"`
	LocationIdentityKey      *string              `json:"location_identity_key"`
	TransitionInteractionKey *string              `json:"transition_interaction_key"`
	Evidence                 []SourceEvidenceSpan `json:"evidence"`
}

type InteractionContinuityCandidate struct {
	SourceVersionID                       string                   `json:"source_version_id"`
	SourceHash                            string                   `json:"source_hash"`
	StructureIdentitySetVersionID         string                   `json:"structure_identity_set_version_id"`
	StructureIdentitySetVersionHash       string                   `json:"structure_identity_set_version_hash"`
	SceneFactCandidateRevisionID          string                   `json:"scene_fact_candidate_revision_id"`
	SceneFactCandidateRevisionHash        string                   `json:"scene_fact_candidate_revision_hash"`
	ProductionEntityCandidateRevisionID   string                   `json:"production_entity_candidate_revision_id"`
	ProductionEntityCandidateRevisionHash string                   `json:"production_entity_candidate_revision_hash"`
	SceneBindingCandidateRevisionID       string                   `json:"scene_binding_candidate_revision_id"`
	SceneBindingCandidateRevisionHash     string                   `json:"scene_binding_candidate_revision_hash"`
	SceneStoryTimes                       []SceneStoryTimeFragment `json:"scene_story_times"`
	Interactions                          []InteractionFragment    `json:"interactions"`
	ContinuityLedger                      []ContinuityLedgerEntry  `json:"continuity_ledger"`
	Continuity                            []ContinuityFragment     `json:"continuity"`
	ReviewIssues                          []CandidateReviewIssue   `json:"review_issues"`
}

type occurrenceBinding struct {
	sceneKey   string
	occurrence SceneOccurrenceFragment
}

type productionStateIdentity struct {
	identityKey string
	kind        string
}

type propLedgerState struct {
	holder       string
	stateKey     string
	storyTimeKey string
}

type continuityLedgerState struct {
	storyTimeEnd string
	stateKey     string
}

type ledgerOccurrence struct {
	sceneKey   string
	occurrence SceneOccurrenceFragment
}

func ValidateInteractionContinuityCandidate(
	raw json.RawMessage,
	input InteractionContinuityInput,
) error {
	if err := input.Validate(); err != nil {
		return err
	}
	var value InteractionContinuityCandidate
	if decodeStrict(raw, &value) != nil || value.SceneStoryTimes == nil || value.Interactions == nil ||
		value.ContinuityLedger == nil || value.Continuity == nil ||
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
	occurrences := make(map[string]occurrenceBinding)
	for _, scene := range bindings.Scenes {
		scenes[scene.SceneScopeKey] = scene
		for _, occurrence := range scene.Occurrences {
			occurrences[occurrence.OccurrenceKey] = occurrenceBinding{scene.SceneScopeKey, occurrence}
		}
	}
	storyTimeByScene := make(map[string]string, len(value.SceneStoryTimes))
	previousStoryTime := ""
	for _, anchor := range value.SceneStoryTimes {
		if _, exists := scenes[anchor.SceneScopeKey]; !exists || !storyTimeKeyPattern.MatchString(anchor.StoryTimeKey) ||
			anchor.StoryTimeKey <= previousStoryTime {
			return errors.New("story-time anchors must cover every Scene once in chronological order")
		}
		if _, duplicate := storyTimeByScene[anchor.SceneScopeKey]; duplicate {
			return errors.New("story-time anchors must cover every Scene once in chronological order")
		}
		storyTimeByScene[anchor.SceneScopeKey] = anchor.StoryTimeKey
		previousStoryTime = anchor.StoryTimeKey
	}
	if len(storyTimeByScene) != len(scenes) {
		return errors.New("story-time anchors must cover every Scene once in chronological order")
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
	interactionSeries := make(map[string]struct{}, len(value.Interactions))
	propLedger := make(map[string]propLedgerState)
	if !sort.SliceIsSorted(value.Interactions, func(left, right int) bool {
		if value.Interactions[left].StoryTimeKey != value.Interactions[right].StoryTimeKey {
			return value.Interactions[left].StoryTimeKey < value.Interactions[right].StoryTimeKey
		}
		return value.Interactions[left].InteractionKey < value.Interactions[right].InteractionKey
	}) {
		return errors.New("Interactions are not in canonical story-time order")
	}
	for _, interaction := range value.Interactions {
		propIdentity, err := validateInteraction(
			interaction,
			scenes,
			occurrences,
			states,
			actionEvidence,
			storyTimeByScene,
		)
		if err != nil {
			return err
		}
		if _, duplicate := interactionKeys[interaction.InteractionKey]; duplicate {
			return errors.New("Interaction key is duplicated")
		}
		interactionKeys[interaction.InteractionKey] = struct{}{}
		if _, duplicate := interactionSeries[interaction.ClaimSeriesKey]; duplicate {
			return errors.New("Interaction claim series is duplicated")
		}
		interactionSeries[interaction.ClaimSeriesKey] = struct{}{}
		if previous, exists := propLedger[propIdentity]; exists &&
			(previous.storyTimeKey == interaction.StoryTimeKey ||
				previous.holder != stringPointerValue(interaction.HolderBeforeIdentityKey) ||
				previous.stateKey != interaction.PropStateBeforeKey) {
			return errors.New("Prop ledger has a duplicate or discontinuous transition")
		}
		propLedger[propIdentity] = propLedgerState{
			holder:   stringPointerValue(interaction.HolderAfterIdentityKey),
			stateKey: interaction.PropStateAfterKey, storyTimeKey: interaction.StoryTimeKey,
		}
	}

	evidenceUniverse := productionEntityEvidenceUniverse(input.SceneFactCandidate)
	characterLedger, err := validateContinuityLedger(
		value.ContinuityLedger,
		value.Interactions,
		occurrences,
		states,
		storyTimeByScene,
		evidenceUniverse,
	)
	if err != nil {
		return err
	}
	continuityKeys := make(map[string]struct{}, len(value.Continuity))
	continuitySeries := make(map[string]struct{}, len(value.Continuity))
	continuityLedger := make(map[string]continuityLedgerState)
	if !sort.SliceIsSorted(value.Continuity, func(left, right int) bool {
		if value.Continuity[left].StoryTimeStart != value.Continuity[right].StoryTimeStart {
			return value.Continuity[left].StoryTimeStart < value.Continuity[right].StoryTimeStart
		}
		if value.Continuity[left].StoryTimeEnd != value.Continuity[right].StoryTimeEnd {
			return value.Continuity[left].StoryTimeEnd < value.Continuity[right].StoryTimeEnd
		}
		return value.Continuity[left].ContinuityKey < value.Continuity[right].ContinuityKey
	}) {
		return errors.New("Continuity is not in canonical story-time order")
	}
	for _, continuity := range value.Continuity {
		if err := validateContinuity(
			continuity,
			storyTimeByScene,
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
		if _, duplicate := continuitySeries[continuity.ClaimSeriesKey]; duplicate {
			return errors.New("Continuity claim series is duplicated")
		}
		continuitySeries[continuity.ClaimSeriesKey] = struct{}{}
		if previous, exists := continuityLedger[continuity.IdentityKey]; exists &&
			(previous.storyTimeEnd > continuity.StoryTimeStart || previous.stateKey != continuity.BeforeStateKey) {
			return errors.New("Continuity ledger overlaps or contains an unexplained state jump")
		}
		continuityLedger[continuity.IdentityKey] = continuityLedgerState{
			storyTimeEnd: continuity.StoryTimeEnd, stateKey: continuity.AfterStateKey,
		}
	}
	continuityLinks := make(map[string]struct{})
	for _, continuity := range value.Continuity {
		if continuity.SubjectKind == "character" {
			continuityLinks[continuityLinkKey(
				continuity.IdentityKey,
				continuity.FromSceneScopeKey,
				continuity.ToSceneScopeKey,
				continuity.BeforeStateKey,
				continuity.AfterStateKey,
			)] = struct{}{}
		}
	}
	for _, entries := range characterLedger {
		for index := 1; index < len(entries); index++ {
			before, after := entries[index-1], entries[index]
			if _, exists := continuityLinks[continuityLinkKey(
				before.IdentityKey,
				before.SceneScopeKey,
				after.SceneScopeKey,
				before.StateKey,
				after.StateKey,
			)]; !exists {
				return errors.New("Character ledger boundary lacks an exact Continuity claim")
			}
		}
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
	storyTimeByScene map[string]string,
) (string, error) {
	scene, sceneExists := scenes[value.SceneScopeKey]
	actor, actorExists := occurrences[value.ActorOccurrenceKey]
	prop, propExists := occurrences[value.PropOccurrenceKey]
	_, evidenceExists := actionEvidence[value.SceneScopeKey][sourceEvidenceKey(value.Evidence)]
	geometryEvidenceExists := true
	for _, evidence := range interactionGeometryEvidence(value.GeometryEvidence) {
		if _, exists := actionEvidence[value.SceneScopeKey][sourceEvidenceKey(*evidence)]; !exists {
			geometryEvidenceExists = false
		}
	}
	before := states[value.PropStateBeforeKey]
	after := states[value.PropStateAfterKey]
	if !interactionKeyPattern.MatchString(value.InteractionKey) ||
		!interactionSeriesKeyPattern.MatchString(value.ClaimSeriesKey) ||
		(value.ClaimRevision == 1) != (value.SupersedesInteractionKey == nil) ||
		(value.SupersedesInteractionKey != nil && !interactionKeyPattern.MatchString(*value.SupersedesInteractionKey)) ||
		value.ClaimRevision < 1 || value.ClaimRevision > 9_007_199_254_740_991 ||
		!sceneExists || !actorExists ||
		!propExists || actor.sceneKey != value.SceneScopeKey || prop.sceneKey != value.SceneScopeKey ||
		actor.occurrence.SubjectKind != "character" || prop.occurrence.SubjectKind != "prop" ||
		actor.occurrence.OccurrenceRole != "actual" || prop.occurrence.OccurrenceRole != "actual" ||
		before != (productionStateIdentity{prop.occurrence.IdentityKey, "prop"}) ||
		after != (productionStateIdentity{prop.occurrence.IdentityKey, "prop"}) || !evidenceExists ||
		!geometryEvidenceExists ||
		value.StoryTimeKey != storyTimeByScene[value.SceneScopeKey] ||
		!interactionValueIn(value.Hand, "left", "right", "both", "unspecified") ||
		!validInteractionDescriptors(value) ||
		!interactionValueIn(value.Predicate, "hold", "carry", "wear", "use", "give", "receive", "place", "drop", "open", "break") {
		return "", errors.New("Interaction does not bind exact actual occurrences and Prop states")
	}
	if value.BeatKey != nil {
		found := false
		for _, beat := range scene.Beats {
			found = found || beat.BeatKey == *value.BeatKey
		}
		if !found {
			return "", errors.New("Interaction references an unknown Scene Beat")
		}
	}
	participants := map[string]struct{}{actor.occurrence.IdentityKey: {}}
	var counterparty occurrenceBinding
	if interactionValueIn(value.Predicate, "give", "receive") {
		if value.CounterpartyOccurrenceKey == nil {
			return "", errors.New("transfer Interaction requires a counterparty")
		}
		var exists bool
		counterparty, exists = occurrences[*value.CounterpartyOccurrenceKey]
		if !exists || counterparty.sceneKey != value.SceneScopeKey ||
			counterparty.occurrence.SubjectKind != "character" ||
			counterparty.occurrence.OccurrenceRole != "actual" ||
			counterparty.occurrence.IdentityKey == actor.occurrence.IdentityKey {
			return "", errors.New("transfer Interaction counterparty is invalid")
		}
		participants[counterparty.occurrence.IdentityKey] = struct{}{}
	} else if value.CounterpartyOccurrenceKey != nil {
		return "", errors.New("non-transfer Interaction cannot add a counterparty")
	}
	for _, holder := range []*string{value.HolderBeforeIdentityKey, value.HolderAfterIdentityKey} {
		if holder != nil {
			if _, exists := participants[*holder]; !exists {
				return "", errors.New("Interaction holder is not a participant")
			}
		}
	}
	actorIdentity := actor.occurrence.IdentityKey
	counterpartyIdentity := counterparty.occurrence.IdentityKey
	beforeHolder := stringPointerValue(value.HolderBeforeIdentityKey)
	afterHolder := stringPointerValue(value.HolderAfterIdentityKey)
	switch value.Predicate {
	case "hold":
		if (beforeHolder != "" && beforeHolder != actorIdentity) || afterHolder != actorIdentity {
			return "", errors.New("hold Interaction transition is invalid")
		}
	case "carry":
		if beforeHolder != actorIdentity || afterHolder != actorIdentity {
			return "", errors.New("carry Interaction transition is invalid")
		}
	case "wear":
		if (beforeHolder != "" && beforeHolder != actorIdentity) || afterHolder != actorIdentity ||
			(beforeHolder == "" && value.StateDelta == nil) {
			return "", errors.New("wear Interaction transition is invalid")
		}
	case "use":
		if beforeHolder != afterHolder {
			return "", errors.New("use Interaction cannot implicitly transfer a holder")
		}
	case "give":
		if beforeHolder != actorIdentity || afterHolder != counterpartyIdentity {
			return "", errors.New("give Interaction transition is invalid")
		}
	case "receive":
		if beforeHolder != counterpartyIdentity || afterHolder != actorIdentity {
			return "", errors.New("receive Interaction transition is invalid")
		}
	case "place", "drop":
		if beforeHolder != actorIdentity || afterHolder != "" || value.StateDelta == nil {
			return "", errors.New("release Interaction transition is invalid")
		}
	case "open", "break":
		if beforeHolder != afterHolder || value.StateDelta == nil {
			return "", errors.New("Prop mutation Interaction transition is invalid")
		}
	}
	return prop.occurrence.IdentityKey, nil
}

func validateContinuity(
	value ContinuityFragment,
	storyTimeByScene map[string]string,
	occurrences map[string]occurrenceBinding,
	states map[string]productionStateIdentity,
	evidenceUniverse map[string]struct{},
) error {
	fromStoryTime, fromExists := storyTimeByScene[value.FromSceneScopeKey]
	toStoryTime, toExists := storyTimeByScene[value.ToSceneScopeKey]
	if !continuityKeyPattern.MatchString(value.ContinuityKey) ||
		!continuitySeriesKeyPattern.MatchString(value.ClaimSeriesKey) ||
		value.ClaimRevision < 1 || value.ClaimRevision > 9_007_199_254_740_991 ||
		(value.ClaimRevision == 1) != (value.SupersedesContinuityKey == nil) ||
		(value.SupersedesContinuityKey != nil && !continuityKeyPattern.MatchString(*value.SupersedesContinuityKey)) ||
		!interactionValueIn(value.SubjectKind, "character", "location", "prop") || !fromExists || !toExists ||
		value.StoryTimeStart != fromStoryTime || value.StoryTimeEnd != toStoryTime ||
		value.StoryTimeStart >= value.StoryTimeEnd || states[value.BeforeStateKey] != (productionStateIdentity{
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

func validateContinuityLedger(
	entries []ContinuityLedgerEntry,
	interactions []InteractionFragment,
	occurrences map[string]occurrenceBinding,
	states map[string]productionStateIdentity,
	storyTimeByScene map[string]string,
	evidenceUniverse map[string]struct{},
) (map[string][]ContinuityLedgerEntry, error) {
	if len(entries) == 0 || !sort.SliceIsSorted(entries, func(left, right int) bool {
		if entries[left].StoryTimeKey != entries[right].StoryTimeKey {
			return entries[left].StoryTimeKey < entries[right].StoryTimeKey
		}
		if entries[left].SubjectKind != entries[right].SubjectKind {
			return entries[left].SubjectKind < entries[right].SubjectKind
		}
		return entries[left].IdentityKey < entries[right].IdentityKey
	}) {
		return nil, errors.New("Continuity ledger is not in canonical story-time order")
	}

	actualSubjects := make(map[string]occurrenceBinding)
	actualLocations := make(map[string]map[string]SceneOccurrenceFragment)
	for _, occurrence := range occurrences {
		if occurrence.occurrence.OccurrenceRole != "actual" {
			continue
		}
		if interactionValueIn(occurrence.occurrence.SubjectKind, "character", "prop") {
			key := ledgerSubjectKey(occurrence.sceneKey, occurrence.occurrence.IdentityKey)
			if previous, exists := actualSubjects[key]; exists &&
				previous.occurrence.StateKey != occurrence.occurrence.StateKey {
				return nil, errors.New("one Scene cannot bind two ledger states for one identity")
			}
			actualSubjects[key] = occurrence
			continue
		}
		if occurrence.occurrence.SubjectKind == "location" {
			if actualLocations[occurrence.sceneKey] == nil {
				actualLocations[occurrence.sceneKey] = make(map[string]SceneOccurrenceFragment)
			}
			actualLocations[occurrence.sceneKey][occurrence.occurrence.IdentityKey] = occurrence.occurrence
		}
	}

	interactionByKey := make(map[string]InteractionFragment, len(interactions))
	for _, interaction := range interactions {
		interactionByKey[interaction.InteractionKey] = interaction
	}
	seenLedgerKeys := make(map[string]struct{}, len(entries))
	suppliedSubjects := make(map[string]struct{}, len(entries))
	usedInteractions := make(map[string]struct{}, len(interactions))
	propEntries := make(map[string][]ContinuityLedgerEntry)
	characterEntries := make(map[string][]ContinuityLedgerEntry)
	for _, entry := range entries {
		subjectKey := ledgerSubjectKey(entry.SceneScopeKey, entry.IdentityKey)
		occurrence, subjectExists := actualSubjects[subjectKey]
		if !continuityLedgerKeyPattern.MatchString(entry.LedgerKey) ||
			!interactionValueIn(entry.SubjectKind, "character", "prop") ||
			!subjectExists || occurrence.occurrence.SubjectKind != entry.SubjectKind ||
			occurrence.occurrence.StateKey != entry.StateKey ||
			states[entry.StateKey] != (productionStateIdentity{
				identityKey: entry.IdentityKey, kind: entry.SubjectKind,
			}) || entry.StoryTimeKey != storyTimeByScene[entry.SceneScopeKey] ||
			len(entry.Evidence) == 0 {
			return nil, errors.New("Continuity ledger does not bind an exact subject and state")
		}
		if _, duplicate := seenLedgerKeys[entry.LedgerKey]; duplicate {
			return nil, errors.New("Continuity ledger key is duplicated")
		}
		seenLedgerKeys[entry.LedgerKey] = struct{}{}
		if _, duplicate := suppliedSubjects[subjectKey]; duplicate {
			return nil, errors.New("Continuity ledger subject is duplicated")
		}
		suppliedSubjects[subjectKey] = struct{}{}

		sceneLocations := actualLocations[entry.SceneScopeKey]
		var location SceneOccurrenceFragment
		locationExists := false
		if entry.LocationIdentityKey != nil {
			location, locationExists = sceneLocations[*entry.LocationIdentityKey]
		}
		if (len(sceneLocations) > 0) != locationExists {
			return nil, errors.New("Continuity ledger does not bind the exact Scene location")
		}
		evidenceKeys := make(map[string]struct{}, len(entry.Evidence))
		for _, evidence := range entry.Evidence {
			key := productionEvidenceKey(evidence)
			if _, duplicate := evidenceKeys[key]; duplicate {
				return nil, errors.New("Continuity ledger Evidence is duplicated")
			}
			if _, exists := evidenceUniverse[key]; !exists {
				return nil, errors.New("Continuity ledger Evidence is outside SceneFacts")
			}
			evidenceKeys[key] = struct{}{}
		}
		if _, exists := evidenceKeys[productionEvidenceKey(occurrence.occurrence.Evidence)]; !exists {
			return nil, errors.New("Continuity ledger lacks exact subject Evidence")
		}
		if locationExists {
			if _, exists := evidenceKeys[productionEvidenceKey(location.Evidence)]; !exists {
				return nil, errors.New("Continuity ledger lacks exact location Evidence")
			}
		}

		if entry.SubjectKind == "character" {
			if entry.HolderIdentityKey != nil || entry.TransitionInteractionKey != nil {
				return nil, errors.New("Character ledger cannot carry holder or Prop transition")
			}
			characterEntries[entry.IdentityKey] = append(characterEntries[entry.IdentityKey], entry)
			continue
		}
		if entry.HolderIdentityKey != nil {
			holder, exists := actualSubjects[ledgerSubjectKey(entry.SceneScopeKey, *entry.HolderIdentityKey)]
			if !exists || holder.occurrence.SubjectKind != "character" {
				return nil, errors.New("Prop ledger holder must be an actual Character")
			}
		}
		propEntries[entry.IdentityKey] = append(propEntries[entry.IdentityKey], entry)
		if entry.TransitionInteractionKey == nil {
			continue
		}
		transition, exists := interactionByKey[*entry.TransitionInteractionKey]
		if !exists {
			return nil, errors.New("Continuity ledger references an unknown Interaction")
		}
		if _, duplicate := usedInteractions[*entry.TransitionInteractionKey]; duplicate {
			return nil, errors.New("Interaction is applied to the Continuity ledger twice")
		}
		propOccurrence := occurrences[transition.PropOccurrenceKey].occurrence
		if transition.SceneScopeKey != entry.SceneScopeKey ||
			transition.StoryTimeKey != entry.StoryTimeKey ||
			propOccurrence.IdentityKey != entry.IdentityKey ||
			transition.PropStateAfterKey != entry.StateKey ||
			stringPointerValue(transition.HolderAfterIdentityKey) != stringPointerValue(entry.HolderIdentityKey) {
			return nil, errors.New("Continuity ledger transition does not match its Interaction exit state")
		}
		usedInteractions[*entry.TransitionInteractionKey] = struct{}{}
	}
	if len(suppliedSubjects) != len(actualSubjects) {
		return nil, errors.New("Continuity ledger must cover every actual Character and Prop once")
	}
	if len(usedInteractions) != len(interactionByKey) {
		return nil, errors.New("every Interaction must produce one Continuity ledger transition")
	}

	for _, values := range propEntries {
		for index := 1; index < len(values); index++ {
			before, after := values[index-1], values[index]
			if after.TransitionInteractionKey == nil {
				if after.StateKey != before.StateKey ||
					stringPointerValue(after.HolderIdentityKey) != stringPointerValue(before.HolderIdentityKey) ||
					stringPointerValue(after.LocationIdentityKey) != stringPointerValue(before.LocationIdentityKey) {
					return nil, errors.New("Prop ledger contains an unexplained state or teleport")
				}
				continue
			}
			transition := interactionByKey[*after.TransitionInteractionKey]
			if transition.PropStateBeforeKey != before.StateKey ||
				stringPointerValue(transition.HolderBeforeIdentityKey) != stringPointerValue(before.HolderIdentityKey) ||
				(stringPointerValue(after.LocationIdentityKey) != stringPointerValue(before.LocationIdentityKey) &&
					!interactionValueIn(transition.Predicate, "carry", "give", "receive")) {
				return nil, errors.New("Prop ledger transition does not explain its boundary change")
			}
		}
	}
	return characterEntries, nil
}

func ledgerSubjectKey(sceneKey, identityKey string) string {
	return sceneKey + "\x00" + identityKey
}

func continuityLinkKey(identityKey, fromScene, toScene, beforeState, afterState string) string {
	return strings.Join([]string{identityKey, fromScene, toScene, beforeState, afterState}, "\x00")
}

func interactionValueIn(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func validInteractionDescriptors(value InteractionFragment) bool {
	changed := value.PropStateBeforeKey != value.PropStateAfterKey
	if changed != (value.StateDelta != nil) {
		return false
	}
	for _, descriptor := range []*string{value.StateDelta, value.GripType, value.ContactPoint, value.Direction} {
		if descriptor != nil && (*descriptor == "" || strings.TrimSpace(*descriptor) != *descriptor) {
			return false
		}
	}
	if value.RelativeScale != nil {
		if value.RelativeScale.Numerator < 1 || value.RelativeScale.Denominator < 1 ||
			value.RelativeScale.Numerator > 9_007_199_254_740_991 ||
			value.RelativeScale.Denominator > 9_007_199_254_740_991 ||
			greatestCommonDivisor(value.RelativeScale.Numerator, value.RelativeScale.Denominator) != 1 {
			return false
		}
	}
	if (value.Hand != "unspecified") != (value.GeometryEvidence.Hand != nil) ||
		(value.GripType != nil) != (value.GeometryEvidence.GripType != nil) ||
		(value.ContactPoint != nil) != (value.GeometryEvidence.ContactPoint != nil) ||
		(value.Direction != nil) != (value.GeometryEvidence.Direction != nil) ||
		(value.RelativeScale != nil) != (value.GeometryEvidence.RelativeScale != nil) {
		return false
	}
	return true
}

func interactionGeometryEvidence(value InteractionGeometryEvidence) []*SourceEvidenceSpan {
	candidates := []*SourceEvidenceSpan{
		value.Hand, value.GripType, value.ContactPoint, value.Direction, value.RelativeScale,
	}
	result := make([]*SourceEvidenceSpan, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil {
			result = append(result, candidate)
		}
	}
	return result
}

func greatestCommonDivisor(left, right int64) int64 {
	for right != 0 {
		left, right = right, left%right
	}
	return left
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
