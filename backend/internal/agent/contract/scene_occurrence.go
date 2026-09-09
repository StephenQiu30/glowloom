package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"

	"github.com/google/uuid"
)

const SceneBindingFragmentCandidateSchemaVersion = "scene-binding-fragment-candidate-production"

var (
	sceneDialogueKeyPattern   = regexp.MustCompile(`^dialogue_[a-z0-9_]{1,120}$`)
	sceneBeatKeyPattern       = regexp.MustCompile(`^beat_[a-z0-9_]{1,120}$`)
	sceneOccurrenceKeyPattern = regexp.MustCompile(`^occurrence_[a-z0-9_]{1,120}$`)
)

type SceneOccurrenceBindingInput struct {
	ProductionEntityDerivationInput
	ProductionEntityCandidateRevisionID   string          `json:"production_entity_candidate_revision_id"`
	ProductionEntityCandidateRevisionHash string          `json:"production_entity_candidate_revision_hash"`
	ProductionEntityCandidate             json.RawMessage `json:"production_entity_candidate"`
}

func (value SceneOccurrenceBindingInput) Validate() error {
	if value.ProductionEntityDerivationInput.Validate() != nil {
		return errors.New("invalid Scene occurrence frozen input")
	}
	if _, err := uuid.Parse(value.ProductionEntityCandidateRevisionID); err != nil {
		return errors.New("invalid Production Entity Candidate revision identity")
	}
	if !hashPattern.MatchString(value.ProductionEntityCandidateRevisionHash) ||
		ValidateProductionEntityFragmentCandidate(
			value.ProductionEntityCandidate,
			value.ProductionEntityDerivationInput,
		) != nil {
		return errors.New("invalid frozen Production Entity Candidate")
	}
	return nil
}

type SceneDialogueFragment struct {
	DialogueKey        string              `json:"dialogue_key"`
	Order              int                 `json:"order"`
	SpeakerIdentityKey *string             `json:"speaker_identity_key"`
	SpeakerEvidence    *SourceEvidenceSpan `json:"speaker_evidence"`
	Text               string              `json:"text"`
	Evidence           SourceEvidenceSpan  `json:"evidence"`
}

type SceneBeatFragment struct {
	BeatKey  string             `json:"beat_key"`
	Order    int                `json:"order"`
	Text     string             `json:"text"`
	Evidence SourceEvidenceSpan `json:"evidence"`
}

type SceneOccurrenceFragment struct {
	OccurrenceKey  string             `json:"occurrence_key"`
	Order          int                `json:"order"`
	SubjectKind    string             `json:"subject_kind"`
	IdentityKey    string             `json:"identity_key"`
	StateKey       string             `json:"state_key"`
	OccurrenceRole string             `json:"occurrence_role"`
	Evidence       SourceEvidenceSpan `json:"evidence"`
}

type SceneBindingFragment struct {
	SceneScopeKey       string                    `json:"scene_scope_key"`
	SceneOwnerLogicalID string                    `json:"scene_owner_logical_id"`
	TemporarySceneID    string                    `json:"temporary_scene_id"`
	SourceStart         int                       `json:"source_start"`
	SourceEnd           int                       `json:"source_end"`
	Dialogues           []SceneDialogueFragment   `json:"dialogues"`
	Beats               []SceneBeatFragment       `json:"beats"`
	Occurrences         []SceneOccurrenceFragment `json:"occurrences"`
}

type SceneBindingFragmentCandidate struct {
	SourceVersionID                       string                 `json:"source_version_id"`
	SourceHash                            string                 `json:"source_hash"`
	StructureIdentitySetVersionID         string                 `json:"structure_identity_set_version_id"`
	StructureIdentitySetVersionHash       string                 `json:"structure_identity_set_version_hash"`
	SceneFactCandidateRevisionID          string                 `json:"scene_fact_candidate_revision_id"`
	SceneFactCandidateRevisionHash        string                 `json:"scene_fact_candidate_revision_hash"`
	ProductionEntityCandidateRevisionID   string                 `json:"production_entity_candidate_revision_id"`
	ProductionEntityCandidateRevisionHash string                 `json:"production_entity_candidate_revision_hash"`
	Scenes                                []SceneBindingFragment `json:"scenes"`
	ReviewIssues                          []CandidateReviewIssue `json:"review_issues"`
}

func ValidateSceneBindingFragmentCandidate(
	raw json.RawMessage,
	input SceneOccurrenceBindingInput,
) error {
	if err := input.Validate(); err != nil {
		return err
	}
	var value SceneBindingFragmentCandidate
	if decodeStrict(raw, &value) != nil || value.Scenes == nil || value.ReviewIssues == nil ||
		value.SourceVersionID != input.SourceVersionID || value.SourceHash != input.SourceHash ||
		value.StructureIdentitySetVersionID != input.StructureIdentitySetVersionID ||
		value.StructureIdentitySetVersionHash != input.StructureIdentitySetVersionHash ||
		value.SceneFactCandidateRevisionID != input.SceneFactCandidateRevisionID ||
		value.SceneFactCandidateRevisionHash != input.SceneFactCandidateRevisionHash ||
		value.ProductionEntityCandidateRevisionID != input.ProductionEntityCandidateRevisionID ||
		value.ProductionEntityCandidateRevisionHash != input.ProductionEntityCandidateRevisionHash ||
		len(value.Scenes) != len(input.StructureIdentitySet.SceneRefs) {
		return errors.New("Scene binding Candidate lineage or Scene coverage drifted")
	}

	var facts SceneFactCandidate
	var production ProductionEntityFragmentCandidate
	if decodeStrict(input.SceneFactCandidate, &facts) != nil ||
		decodeStrict(input.ProductionEntityCandidate, &production) != nil {
		return errors.New("invalid Scene binding frozen Candidates")
	}
	factByScene := make(map[string]SceneFact, len(facts.Scenes))
	for _, fact := range facts.Scenes {
		factByScene[fact.TemporarySceneID] = fact
	}
	entityByKey := make(map[string]ProductionEntityFragment, len(production.Entities))
	for _, entity := range production.Entities {
		entityByKey[entity.IdentityKey] = entity
	}
	mappingsByScene := make(map[string][]FrozenStructureIdentityMentionMapping)
	for _, mapping := range input.StructureIdentitySet.MentionMappings {
		if mapping.Resolution == "resolved" {
			mappingsByScene[mapping.TemporarySceneID] = append(
				mappingsByScene[mapping.TemporarySceneID],
				mapping,
			)
		}
	}
	for sceneID := range mappingsByScene {
		sort.Slice(mappingsByScene[sceneID], func(left, right int) bool {
			return sceneOccurrenceMappingKey(mappingsByScene[sceneID][left]) <
				sceneOccurrenceMappingKey(mappingsByScene[sceneID][right])
		})
	}

	for index, scene := range value.Scenes {
		formal := input.StructureIdentitySet.SceneRefs[index]
		fact, exists := factByScene[formal.TemporarySceneID]
		if !exists || scene.SceneScopeKey != formal.ScopeKey ||
			scene.SceneOwnerLogicalID != formal.SceneOwnerLogicalID ||
			scene.TemporarySceneID != formal.TemporarySceneID ||
			scene.SourceStart != formal.SourceStart || scene.SourceEnd != formal.SourceEnd ||
			scene.Dialogues == nil || scene.Beats == nil || scene.Occurrences == nil ||
			len(scene.Dialogues) != len(fact.Dialogues) || len(scene.Beats) != len(fact.Actions) {
			return errors.New("Scene binding does not match its formal Scene or SceneFacts")
		}
		if err := validateSceneDialogues(scene, fact, mappingsByScene[formal.TemporarySceneID]); err != nil {
			return err
		}
		if err := validateSceneBeats(scene, fact); err != nil {
			return err
		}
		if err := validateSceneOccurrences(
			scene,
			formal,
			mappingsByScene[formal.TemporarySceneID],
			entityByKey,
		); err != nil {
			return err
		}
	}

	runes := []rune(input.NormalizedText)
	evidenceUniverse := productionEntityEvidenceUniverse(input.SceneFactCandidate)
	previousIssue := ""
	for _, issue := range value.ReviewIssues {
		if issue.IssueKey <= previousIssue || validateCandidateReviewIssue(issue, runes, true) != nil {
			return errors.New("invalid Scene binding ReviewIssue")
		}
		for _, evidence := range issue.Evidence {
			if _, exists := evidenceUniverse[sourceEvidenceKey(evidence)]; !exists {
				return errors.New("Scene binding ReviewIssue Evidence is outside frozen SceneFacts")
			}
		}
		previousIssue = issue.IssueKey
	}
	return nil
}

func validateSceneDialogues(
	scene SceneBindingFragment,
	fact SceneFact,
	mappings []FrozenStructureIdentityMentionMapping,
) error {
	formalSpeakers := make(map[string]struct{})
	for _, mapping := range mappings {
		if mapping.Kind == "character" && mapping.IdentityKey != nil {
			formalSpeakers[*mapping.IdentityKey+"\x00"+sceneOccurrenceMappingEvidenceKey(mapping)] = struct{}{}
		}
	}
	keys := make(map[string]struct{}, len(scene.Dialogues))
	for index, dialogue := range scene.Dialogues {
		frozen := fact.Dialogues[index]
		if dialogue.Order != index+1 || !sceneDialogueKeyPattern.MatchString(dialogue.DialogueKey) ||
			dialogue.Text != frozen.Text || dialogue.Evidence != frozen.Evidence ||
			(dialogue.SpeakerIdentityKey == nil) != (dialogue.SpeakerEvidence == nil) {
			return errors.New("Scene Dialogue drifted from frozen SceneFacts")
		}
		if _, duplicate := keys[dialogue.DialogueKey]; duplicate {
			return errors.New("Scene Dialogue key is duplicated")
		}
		keys[dialogue.DialogueKey] = struct{}{}
		if dialogue.SpeakerIdentityKey != nil {
			if dialogue.SpeakerEvidence.ExactAnchor != frozen.SpeakerMention {
				return errors.New("Scene Dialogue speaker Evidence drifted")
			}
			key := *dialogue.SpeakerIdentityKey + "\x00" + sourceEvidenceKey(*dialogue.SpeakerEvidence)
			if _, exists := formalSpeakers[key]; !exists {
				return errors.New("Scene Dialogue speaker is not an exact formal identity mapping")
			}
		}
	}
	return nil
}

func validateSceneBeats(scene SceneBindingFragment, fact SceneFact) error {
	keys := make(map[string]struct{}, len(scene.Beats))
	for index, beat := range scene.Beats {
		frozen := fact.Actions[index]
		if beat.Order != index+1 || !sceneBeatKeyPattern.MatchString(beat.BeatKey) ||
			beat.Text != frozen.Text || beat.Evidence != frozen.Evidence {
			return errors.New("Scene Beat drifted from frozen SceneFacts")
		}
		if _, duplicate := keys[beat.BeatKey]; duplicate {
			return errors.New("Scene Beat key is duplicated")
		}
		keys[beat.BeatKey] = struct{}{}
	}
	return nil
}

func validateSceneOccurrences(
	scene SceneBindingFragment,
	formal FrozenStructureIdentitySceneRef,
	mappings []FrozenStructureIdentityMentionMapping,
	entities map[string]ProductionEntityFragment,
) error {
	if len(scene.Occurrences) != len(mappings) {
		return errors.New("Scene occurrences do not cover the resolved mention partition")
	}
	keys := make(map[string]struct{}, len(scene.Occurrences))
	for index, occurrence := range scene.Occurrences {
		mapping := mappings[index]
		if mapping.IdentityKey == nil || occurrence.Order != index+1 ||
			!sceneOccurrenceKeyPattern.MatchString(occurrence.OccurrenceKey) ||
			occurrence.SubjectKind != mapping.Kind || occurrence.IdentityKey != *mapping.IdentityKey ||
			occurrence.OccurrenceRole != mapping.OccurrenceRole || occurrence.Evidence != (SourceEvidenceSpan{
			SourceStart: mapping.SourceStart, SourceEnd: mapping.SourceEnd,
			TextHash: mapping.TextHash, ExactAnchor: mapping.ExactAnchor,
		}) {
			return errors.New("Scene occurrence drifted from its formal identity mapping")
		}
		if _, duplicate := keys[occurrence.OccurrenceKey]; duplicate {
			return errors.New("Scene occurrence key is duplicated")
		}
		keys[occurrence.OccurrenceKey] = struct{}{}
		entity, exists := entities[occurrence.IdentityKey]
		if !exists || entity.Kind != occurrence.SubjectKind {
			return errors.New("Scene occurrence references an unknown Production Entity")
		}
		stateFound := slices.ContainsFunc(entity.States, func(state ProductionStateFragment) bool {
			return state.StateKey == occurrence.StateKey &&
				slices.Contains(state.ApplicableSceneScopeKeys, formal.ScopeKey)
		})
		if !stateFound {
			return errors.New("Scene occurrence State does not apply to its formal Scene")
		}
	}
	return nil
}

func sceneOccurrenceMappingKey(value FrozenStructureIdentityMentionMapping) string {
	identity := ""
	if value.IdentityKey != nil {
		identity = *value.IdentityKey
	}
	return fmt.Sprintf(
		"%020d\x00%020d\x00%s\x00%s",
		value.SourceStart,
		value.SourceEnd,
		value.Kind,
		identity,
	)
}

func sceneOccurrenceMappingEvidenceKey(value FrozenStructureIdentityMentionMapping) string {
	return sourceEvidenceKey(SourceEvidenceSpan{
		SourceStart: value.SourceStart, SourceEnd: value.SourceEnd,
		TextHash: value.TextHash, ExactAnchor: value.ExactAnchor,
	})
}
