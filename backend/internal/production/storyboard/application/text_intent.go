package application

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
	"github.com/google/uuid"
)

type TextIntentScope struct {
	ID, RunID, ProposalID, DecisionID, WorkspaceID, ProjectID, SourceRevisionID, SourceHash, CreatedBy string
	CreatedAt                                                                                          time.Time
}
type TextIntentInput struct {
	Scope           TextIntentScope
	Candidate, Task json.RawMessage
	IDMapping       map[string]string
	RiskResolutions []domain.TextIntentRiskResolution
}

func BuildTextIntent(input TextIntentInput) (domain.TextIntentVersion, error) {
	fail := errors.New("invalid accepted text storyboard intent")
	s := input.Scope
	for _, id := range []string{s.ID, s.RunID, s.ProposalID, s.DecisionID, s.WorkspaceID, s.ProjectID, s.SourceRevisionID, s.CreatedBy} {
		if !contract.ValidTextID(id) {
			return domain.TextIntentVersion{}, fail
		}
	}
	var task contract.TextExecutionTask
	var candidate contract.TextDirection
	if s.CreatedAt.IsZero() || contract.DecodeTextWire(input.Task, &task) != nil || contract.DecodeTextWire(input.Candidate, &candidate) != nil || task.Stage != "direct_scene" || contract.ValidateTextSource(task.Source, s.SourceRevisionID, s.SourceHash) != nil || task.World == nil || task.EpisodeKey == nil || task.SceneKey == nil || candidate.EpisodeKey != *task.EpisodeKey || candidate.SceneKey != *task.SceneKey || len(candidate.Shots) == 0 || len(candidate.Shots) > 200 || strings.TrimSpace(candidate.DramaticIntent) == "" {
		return domain.TextIntentVersion{}, fail
	}
	var scene *contract.TextScene
	for _, analysis := range task.Analyses {
		if analysis.EpisodeKey == candidate.EpisodeKey {
			for i := range analysis.Scenes {
				if analysis.Scenes[i].Key == candidate.SceneKey {
					if scene != nil {
						return domain.TextIntentVersion{}, fail
					}
					scene = &analysis.Scenes[i]
				}
			}
		}
	}
	if scene == nil {
		return domain.TextIntentVersion{}, fail
	}
	evidenceIndex := contract.NewTextEvidenceIndex(task.Source)
	value := domain.TextIntentVersion{ID: s.ID, RunID: s.RunID, ProposalID: s.ProposalID, DecisionID: s.DecisionID, WorkspaceID: s.WorkspaceID, ProjectID: s.ProjectID, SourceRevisionID: s.SourceRevisionID, SourceHash: s.SourceHash, Revision: 1, CreatedBy: s.CreatedBy, CreatedAt: s.CreatedAt.UTC(), DramaticIntent: candidate.DramaticIntent, AudienceKnows: append([]string{}, candidate.AudienceKnows...), Withhold: append([]string{}, candidate.Withhold...), Blocking: []domain.TextIntentBlocking{}, Shots: []domain.TextIntentShot{}, AssetReadiness: "needs_asset", Issues: []domain.TextIntentIssue{}, RiskResolutions: append([]domain.TextIntentRiskResolution{}, input.RiskResolutions...), IDMapping: map[string]string{}}
	for k, id := range input.IDMapping {
		if !contract.ValidTextID(id) {
			return value, fail
		}
		value.IDMapping[k] = id
	}
	prefix := candidate.EpisodeKey + "/" + candidate.SceneKey
	value.EpisodeID = value.IDMapping["episode/"+candidate.EpisodeKey]
	value.StructureID = value.IDMapping["structure/"+candidate.EpisodeKey]
	value.SceneID = value.IDMapping["scene/"+prefix]
	value.WorldVersionID = value.IDMapping["world"]
	for _, id := range []string{value.EpisodeID, value.StructureID, value.SceneID, value.WorldVersionID} {
		if !contract.ValidTextID(id) {
			return value, fail
		}
	}
	intentKey := "intent/" + prefix
	if previous := value.IDMapping[intentKey]; previous != "" && previous != s.ID {
		return value, errors.New("text intent revision conflict")
	}
	value.IDMapping[intentKey] = s.ID
	beats := map[string]contract.TextBeat{}
	dialogues := map[string]contract.TextDialogue{}
	visible := map[string]contract.TextMention{}
	entities := map[string]string{}
	required := map[string]bool{}
	coveredDialogues := map[string]bool{}
	details := map[[2]int]bool{}
	for _, beat := range scene.Beats {
		if _, exists := beats[beat.Key]; exists || !contract.ValidTextKey(beat.Key) || value.IDMapping["beat/"+prefix+"/"+beat.Key] == "" {
			return value, fail
		}
		beats[beat.Key] = beat
		if beat.Required {
			required[beat.Key] = false
		}
	}
	for _, dialogue := range scene.Dialogues {
		if _, exists := dialogues[dialogue.Key]; exists || value.IDMapping["dialogue/"+prefix+"/"+dialogue.Key] == "" || dialogue.Text != dialogue.Evidence.Quote {
			return value, fail
		}
		if _, _, err := evidenceIndex.Resolve(dialogue.Evidence); err != nil {
			return value, err
		}
		dialogues[dialogue.Key] = dialogue
		coveredDialogues[dialogue.Key] = false
	}
	for _, mention := range scene.Mentions {
		if mention.Presence == "onscreen" {
			if value.IDMapping["mention/"+prefix+"/"+mention.Key] == "" {
				return value, fail
			}
			visible[mention.Key] = mention
			for _, e := range mention.VisualDetails {
				start, end, err := evidenceIndex.Resolve(e)
				if err != nil {
					return value, err
				}
				details[[2]int{start, end}] = false
			}
		}
	}
	for _, entity := range task.World.Entities {
		entityID := value.IDMapping["entity/"+entity.Key]
		if entityID == "" {
			return value, fail
		}
		for _, ref := range entity.Mentions {
			if ref.EpisodeKey == candidate.EpisodeKey && ref.SceneKey == candidate.SceneKey {
				if entities[ref.MentionKey] != "" {
					return value, fail
				}
				entities[ref.MentionKey] = entityID
			}
		}
	}
	for _, blocking := range candidate.Blocking {
		if _, exists := visible[blocking.MentionKey]; !exists || strings.TrimSpace(blocking.Position) == "" || strings.TrimSpace(blocking.Facing) == "" || strings.TrimSpace(blocking.Action) == "" {
			return value, fail
		}
		value.Blocking = append(value.Blocking, domain.TextIntentBlocking{MentionID: value.IDMapping["mention/"+prefix+"/"+blocking.MentionKey], Position: blocking.Position, Facing: blocking.Facing, Action: blocking.Action})
	}
	seenShots := map[string]bool{}
	for index, shot := range candidate.Shots {
		if !contract.ValidTextKey(shot.Key) || seenShots[shot.Key] || len(shot.BeatKeys) == 0 || shot.DurationMinMS < 100 || shot.DurationMaxMS > 120000 || shot.DurationMinMS > shot.DurationMaxMS {
			return value, fail
		}
		seenShots[shot.Key] = true
		for _, text := range []string{shot.Purpose, shot.Framing, shot.CameraMovement, shot.Action, shot.TimingBasis, shot.ScreenDirection, shot.EntryState, shot.ExitState, shot.PanelCaption} {
			if strings.TrimSpace(text) == "" {
				return value, fail
			}
		}
		key := "shot/" + prefix + "/" + shot.Key
		id := uuid.NewSHA1(uuid.MustParse(s.ID), []byte(key)).String()
		if previous := value.IDMapping[key]; previous != "" && previous != id {
			return value, errors.New("text intent shot identity conflict")
		}
		value.IDMapping[key] = id
		formal := domain.TextIntentShot{ID: id, Key: shot.Key, Position: index + 1, Purpose: shot.Purpose, Framing: shot.Framing, CameraMovement: shot.CameraMovement, Action: shot.Action, NarrativeUnitIDs: []string{}, Audio: []domain.TextIntentAudio{}, VisualRequirements: []domain.TextIntentVisual{}, DetailEvidence: []domain.EvidenceRef{}, DurationMinMS: shot.DurationMinMS, DurationMaxMS: shot.DurationMaxMS, TimingBasis: shot.TimingBasis, ScreenDirection: shot.ScreenDirection, EntryState: shot.EntryState, ExitState: shot.ExitState, PanelCaption: shot.PanelCaption, AssetReadiness: "needs_asset"}
		seen := map[string]bool{}
		for _, key := range shot.BeatKeys {
			if _, exists := beats[key]; !exists || seen[key] {
				return value, fail
			}
			seen[key] = true
			if _, exists := required[key]; exists {
				required[key] = true
			}
			formal.NarrativeUnitIDs = append(formal.NarrativeUnitIDs, value.IDMapping["beat/"+prefix+"/"+key])
		}
		seen = map[string]bool{}
		for _, cue := range shot.Audio {
			dialogue, exists := dialogues[cue.DialogueKey]
			if !exists || cue.Channel != dialogue.Channel || seen[cue.DialogueKey] {
				return value, fail
			}
			seen[cue.DialogueKey] = true
			coveredDialogues[cue.DialogueKey] = true
			speakerID := ""
			if dialogue.SpeakerMention != nil {
				speakerID = value.IDMapping["mention/"+prefix+"/"+*dialogue.SpeakerMention]
				if speakerID == "" {
					return value, fail
				}
			}
			formal.Audio = append(formal.Audio, domain.TextIntentAudio{DialogueID: value.IDMapping["dialogue/"+prefix+"/"+cue.DialogueKey], Channel: cue.Channel, Text: dialogue.Text, SpeakerMentionID: speakerID})
		}
		seen = map[string]bool{}
		for _, key := range shot.VisibleMentions {
			if _, exists := visible[key]; !exists || seen[key] {
				return value, fail
			}
			seen[key] = true
			formal.VisualRequirements = append(formal.VisualRequirements, domain.TextIntentVisual{MentionID: value.IDMapping["mention/"+prefix+"/"+key], EntityID: entities[key], AssetReadiness: "needs_asset"})
		}
		for _, e := range shot.DetailEvidence {
			if e.Block < scene.FirstBlock || e.Block > scene.LastBlock {
				return value, fail
			}
			start, end, err := evidenceIndex.Resolve(e)
			if err != nil {
				return value, err
			}
			if _, exists := details[[2]int{start, end}]; exists {
				details[[2]int{start, end}] = true
			}
			formal.DetailEvidence = append(formal.DetailEvidence, domain.EvidenceRef{DocumentRevisionID: s.SourceRevisionID, AbsoluteStart: start, AbsoluteEnd: end, TextHash: contract.TextWireHash(e.Quote)})
		}
		value.Shots = append(value.Shots, formal)
	}
	for _, covered := range required {
		if !covered {
			return value, fail
		}
	}
	for _, covered := range coveredDialogues {
		if !covered {
			return value, fail
		}
	}
	for _, covered := range details {
		if !covered {
			return value, fail
		}
	}
	blockers := map[string]bool{}
	candidate.Issues = append(candidate.Issues, contract.TextIssue{Code: "continuity_mapping_pending", Scope: *task.SceneKey, Severity: "blocker", Summary: "跨场状态与身份披露仍需平台已审阅映射；本结果仅为导演草案。"})
	for _, issue := range candidate.Issues {
		if !contract.ValidTextKey(issue.Code) || issue.Scope == "" || !slices.Contains([]string{"warning", "blocker"}, issue.Severity) {
			return value, fail
		}
		value.Issues = append(value.Issues, domain.TextIntentIssue{Code: issue.Code, Scope: issue.Scope, Severity: issue.Severity, Summary: issue.Summary})
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
