package application

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	"github.com/google/uuid"
)

type TextWorldScope struct {
	ID, RunID, ProposalID, DecisionID, WorkspaceID, ProjectID, SourceRevisionID, SourceHash, CreatedBy string
	CreatedAt                                                                                          time.Time
}
type TextWorldInput struct {
	Scope           TextWorldScope
	Candidate, Task json.RawMessage
	IDMapping       map[string]string
	RiskResolutions []domain.TextWorldRiskResolution
}

// BuildTextWorld converts reviewed mentions into stable formal entity identities.
// It deliberately does not create visual assets or infer an undisclosed identity.
func BuildTextWorld(input TextWorldInput) (domain.TextWorldVersion, error) {
	fail := errors.New("invalid accepted text world")
	s := input.Scope
	for _, id := range []string{s.ID, s.RunID, s.ProposalID, s.DecisionID, s.WorkspaceID, s.ProjectID, s.SourceRevisionID, s.CreatedBy} {
		if !contract.ValidTextID(id) {
			return domain.TextWorldVersion{}, fail
		}
	}
	var task contract.TextExecutionTask
	var candidate contract.TextWorld
	if s.CreatedAt.IsZero() || contract.DecodeTextWire(input.Task, &task) != nil || contract.DecodeTextWire(input.Candidate, &candidate) != nil || task.Stage != "build_world" || contract.ValidateTextSource(task.Source, s.SourceRevisionID, s.SourceHash) != nil || len(task.Analyses) == 0 || len(candidate.Entities) > 2000 {
		return domain.TextWorldVersion{}, fail
	}
	evidenceIndex := contract.NewTextEvidenceIndex(task.Source)
	value := domain.TextWorldVersion{ID: s.ID, RunID: s.RunID, ProposalID: s.ProposalID, DecisionID: s.DecisionID, WorkspaceID: s.WorkspaceID, ProjectID: s.ProjectID, SourceRevisionID: s.SourceRevisionID, SourceHash: s.SourceHash, Revision: 1, CreatedBy: s.CreatedBy, CreatedAt: s.CreatedAt.UTC(), Entities: []domain.TextWorldEntity{}, UnresolvedMentionIDs: []string{}, Relations: []domain.TextWorldRelation{}, StateEvents: []domain.TextWorldState{}, AssetNeeds: []domain.TextWorldAssetNeed{}, Issues: []domain.TextWorldIssue{}, RiskResolutions: append([]domain.TextWorldRiskResolution{}, input.RiskResolutions...), IDMapping: map[string]string{}}
	for k, id := range input.IDMapping {
		if !contract.ValidTextID(id) {
			return value, fail
		}
		value.IDMapping[k] = id
	}
	if previous := value.IDMapping["world"]; previous != "" && previous != s.ID {
		return value, errors.New("text world revision conflict")
	}
	value.IDMapping["world"] = s.ID
	mentions := map[string]contract.TextMention{}
	scenes := map[string]contract.TextScene{}
	for _, analysis := range task.Analyses {
		if !contract.ValidTextKey(analysis.EpisodeKey) || value.IDMapping["episode/"+analysis.EpisodeKey] == "" || value.IDMapping["structure/"+analysis.EpisodeKey] == "" {
			return value, fail
		}
		for _, scene := range analysis.Scenes {
			prefix := analysis.EpisodeKey + "/" + scene.Key
			if !contract.ValidTextKey(scene.Key) || value.IDMapping["scene/"+prefix] == "" {
				return value, fail
			}
			if _, exists := scenes[prefix]; exists {
				return value, fail
			}
			scenes[prefix] = scene
			for _, mention := range scene.Mentions {
				key := prefix + "/" + mention.Key
				if !contract.ValidTextKey(mention.Key) || value.IDMapping["mention/"+key] == "" {
					return value, fail
				}
				if _, exists := mentions[key]; exists {
					return value, fail
				}
				mentions[key] = mention
			}
		}
	}
	covered := map[string]bool{}
	entityIDs := map[string]string{}
	identityEvidence := map[[2]int]bool{}
	for _, entity := range candidate.Entities {
		if !contract.ValidTextKey(entity.Key) || strings.TrimSpace(entity.Label) == "" || len(entity.Mentions) == 0 || !slices.Contains([]string{"cast", "place", "prop"}, entity.Kind) || !slices.Contains([]string{"explicit", "inferred", "uncertain"}, entity.IdentityBasis) || entityIDs[entity.Key] != "" {
			return value, fail
		}
		if entity.IdentityBasis != "explicit" && (entity.Uncertainty == nil || strings.TrimSpace(*entity.Uncertainty) == "") {
			return value, fail
		}
		if entity.IdentityBasis != "explicit" {
			candidate.Issues = append(candidate.Issues, contract.TextIssue{Code: "identity_requires_review", Scope: entity.Key, Severity: "blocker", Summary: *entity.Uncertainty})
		}
		id := uuid.NewSHA1(uuid.MustParse(s.ID), []byte("entity/"+entity.Key)).String()
		if previous := value.IDMapping["entity/"+entity.Key]; previous != "" && previous != id {
			return value, fail
		}
		value.IDMapping["entity/"+entity.Key] = id
		entityIDs[entity.Key] = id
		evidence, err := worldEvidence(evidenceIndex, entity.Evidence)
		if err != nil {
			return value, err
		}
		for _, item := range evidence {
			identityEvidence[[2]int{item.SourceStart, item.SourceEnd}] = true
		}
		formal := domain.TextWorldEntity{ID: id, Key: entity.Key, Kind: entity.Kind, Label: entity.Label, IdentityBasis: entity.IdentityBasis, Uncertainty: entity.Uncertainty, MentionIDs: []string{}, Evidence: evidence}
		for _, ref := range entity.Mentions {
			key := ref.EpisodeKey + "/" + ref.SceneKey + "/" + ref.MentionKey
			mention, exists := mentions[key]
			if !exists || covered[key] || mention.Kind != entity.Kind {
				return value, fail
			}
			covered[key] = true
			formal.MentionIDs = append(formal.MentionIDs, value.IDMapping["mention/"+key])
		}
		value.Entities = append(value.Entities, formal)
	}
	for _, ref := range candidate.UnresolvedMentions {
		key := ref.EpisodeKey + "/" + ref.SceneKey + "/" + ref.MentionKey
		if _, exists := mentions[key]; !exists || covered[key] {
			return value, fail
		}
		covered[key] = true
		value.UnresolvedMentionIDs = append(value.UnresolvedMentionIDs, value.IDMapping["mention/"+key])
	}
	if len(covered) != len(mentions) {
		return value, fail
	}
	if len(candidate.UnresolvedMentions) > 0 {
		candidate.Issues = append(candidate.Issues, contract.TextIssue{Code: "unresolved_identity", Scope: "world", Severity: "blocker", Summary: "仍有未归并的场次提及。"})
	}
	for _, relation := range candidate.Relations {
		if entityIDs[relation.Subject] == "" || entityIDs[relation.Target] == "" || strings.TrimSpace(relation.Predicate) == "" || !slices.Contains([]string{"extracted", "inferred", "proposed"}, relation.Origin) || !slices.Contains([]string{"narration", "claim", "unknown"}, relation.Basis) {
			return value, fail
		}
		evidence, err := worldEvidence(evidenceIndex, relation.Evidence)
		if err != nil {
			return value, err
		}
		value.Relations = append(value.Relations, domain.TextWorldRelation{SubjectID: entityIDs[relation.Subject], Predicate: relation.Predicate, TargetID: entityIDs[relation.Target], Origin: relation.Origin, Basis: relation.Basis, Evidence: evidence})
	}
	for _, event := range candidate.StateEvents {
		prefix := event.EpisodeKey + "/" + event.SceneKey
		scene, exists := scenes[prefix]
		if !exists || entityIDs[event.EntityKey] == "" || event.TimeBranch != scene.TimeBranch || strings.TrimSpace(event.StoryTime) == "" || strings.TrimSpace(event.Property) == "" || !slices.Contains([]string{"known", "unknown", "conflicting"}, event.Knowledge) || !slices.Contains([]string{"narration", "claim", "unknown"}, event.Basis) || (event.Knowledge == "known" && event.Basis != "narration") {
			return value, fail
		}
		localEvidence := false
		for _, e := range event.Evidence {
			if e.Block >= scene.FirstBlock && e.Block <= scene.LastBlock {
				localEvidence = true
				continue
			}
			start, end, evidenceErr := evidenceIndex.Resolve(e)
			if evidenceErr != nil || !identityEvidence[[2]int{start, end}] {
				return value, fail
			}
		}
		if !localEvidence {
			return value, fail
		}
		evidence, err := worldEvidence(evidenceIndex, event.Evidence)
		if err != nil {
			return value, err
		}
		value.StateEvents = append(value.StateEvents, domain.TextWorldState{EntityID: entityIDs[event.EntityKey], EpisodeID: value.IDMapping["episode/"+event.EpisodeKey], SceneID: value.IDMapping["scene/"+prefix], TimeBranch: event.TimeBranch, StoryTime: event.StoryTime, Property: event.Property, Before: event.Before, After: event.After, Knowledge: event.Knowledge, Basis: event.Basis, Evidence: evidence})
	}
	for _, need := range candidate.AssetNeeds {
		if entityIDs[need.EntityKey] == "" || strings.TrimSpace(need.Description) == "" {
			return value, fail
		}
		evidence, err := worldEvidence(evidenceIndex, need.Evidence)
		if err != nil {
			return value, err
		}
		value.AssetNeeds = append(value.AssetNeeds, domain.TextWorldAssetNeed{EntityID: entityIDs[need.EntityKey], Description: need.Description, Evidence: evidence})
	}
	blockers := map[string]bool{}
	for _, issue := range candidate.Issues {
		if !contract.ValidTextKey(issue.Code) || issue.Scope == "" || !slices.Contains([]string{"warning", "blocker"}, issue.Severity) {
			return value, fail
		}
		value.Issues = append(value.Issues, domain.TextWorldIssue{Code: issue.Code, Scope: issue.Scope, Severity: issue.Severity, Summary: issue.Summary})
		if issue.Severity == "blocker" {
			blockers[issue.Code+"\x00"+issue.Scope] = false
		}
	}
	for _, resolution := range value.RiskResolutions {
		key := resolution.Code + "\x00" + resolution.Scope
		done, exists := blockers[key]
		if !exists || done || strings.TrimSpace(resolution.Reason) == "" {
			return value, fail
		}
		blockers[key] = true
	}
	for _, done := range blockers {
		if !done {
			return value, fail
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return value, err
	}
	value.ContentHash, err = canonical.Hash(raw)
	return value, err
}

func worldEvidence(index *contract.TextEvidenceIndex, items []contract.TextEvidence) ([]domain.Evidence, error) {
	if len(items) == 0 {
		return nil, errors.New("formal world requires source evidence")
	}
	result := make([]domain.Evidence, 0, len(items))
	for _, item := range items {
		start, end, err := index.Resolve(item)
		if err != nil {
			return nil, err
		}
		result = append(result, domain.Evidence{SourceStart: start, SourceEnd: end, TextHash: contract.TextWireHash(item.Quote), ExactAnchor: item.Quote})
	}
	return result, nil
}
