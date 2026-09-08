package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	bible "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

type TextPlanningInput struct {
	Stage, WorkspaceID, ProjectID, SourceRevisionID, SourceHash, SourceText, CreatedBy string
	Candidate                                                                          json.RawMessage
	Blocks                                                                             [][2]int
	IDMapping                                                                          map[string]string
	ExpectedProjectRevision                                                            int
	SourceReceiptID                                                                    string
	TargetDurationMS                                                                   int
	CreatedAt                                                                          time.Time
	NewID                                                                              func() string
}
type TextPlanningResult struct {
	Episodes  []domain.Episode
	Versions  []Version
	Structure *domain.Structure
	IDMapping map[string]string
}
type textRange struct {
	First int `json:"first_block"`
	Last  int `json:"last_block"`
}
type textEvidence struct {
	Block      int    `json:"block"`
	Quote      string `json:"quote"`
	Occurrence *int   `json:"occurrence"`
}
type textEpisode struct {
	textRange
	Key   string `json:"key"`
	Title string `json:"title"`
}
type textMention struct {
	Key           string         `json:"key"`
	Kind          string         `json:"kind"`
	Name          string         `json:"name"`
	Presence      string         `json:"presence"`
	Evidence      textEvidence   `json:"evidence"`
	VisualDetails []textEvidence `json:"visual_details"`
}

var textKey = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func BuildTextPlanning(input TextPlanningInput) (TextPlanningResult, error) {
	result := TextPlanningResult{Episodes: []domain.Episode{}, Versions: []Version{}, IDMapping: map[string]string{}}
	if input.NewID == nil || input.CreatedAt.IsZero() || textHash(input.SourceText) != input.SourceHash || len(input.Blocks) == 0 {
		return result, errors.New("invalid frozen text planning input")
	}
	if input.Stage == "map_manuscript" {
		return buildTextEpisodes(input, result)
	}
	if input.Stage == "analyze_episode" {
		return buildTextStructure(input, result)
	}
	return result, errors.New("unsupported text planning stage")
}
func buildTextEpisodes(input TextPlanningInput, result TextPlanningResult) (TextPlanningResult, error) {
	var candidate struct {
		Episodes []textEpisode `json:"episodes"`
		Excluded []textRange   `json:"excluded"`
	}
	if json.Unmarshal(input.Candidate, &candidate) != nil || len(candidate.Episodes) == 0 || len(candidate.Episodes) > 200 {
		return result, errors.New("invalid episode map")
	}
	coverage := make([]bool, len(input.Blocks))
	seen := map[string]bool{}
	previous := -1
	for i, episode := range candidate.Episodes {
		if !textKey.MatchString(episode.Key) || seen[episode.Key] || strings.TrimSpace(episode.Title) == "" || utf8.RuneCountInString(episode.Title) > 120 || episode.First <= previous {
			return result, errors.New("invalid episode identity or order")
		}
		seen[episode.Key] = true
		previous = episode.Last
		start, end, err := coverTextRange(episode.textRange, input.Blocks, coverage)
		if err != nil {
			return result, err
		}
		episodeID, versionID := input.NewID(), input.NewID()
		content := string([]rune(input.SourceText)[start:end])
		if input.TargetDurationMS <= 0 {
			return result, errors.New("episode target duration must come from the project")
		}
		result.Episodes = append(result.Episodes, domain.Episode{ID: episodeID, WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID, Name: episode.Title, Position: i + 1, TargetDurationMS: input.TargetDurationMS, Status: "active", Revision: 1, CurrentScriptVersionID: &versionID, CreatedAt: input.CreatedAt, UpdatedAt: input.CreatedAt})
		result.Versions = append(result.Versions, Version{ID: versionID, WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID, EpisodeID: episodeID, DocumentRevisionID: input.SourceRevisionID, VersionNo: 1, SourceStart: start, SourceEnd: end, Content: content, ContentHash: textHash(content), Status: "published", CreatedBy: input.CreatedBy, CreatedAt: input.CreatedAt})
		result.IDMapping["episode/"+episode.Key] = episodeID
		result.IDMapping["script/"+episode.Key] = versionID
	}
	for _, excluded := range candidate.Excluded {
		if _, _, err := coverTextRange(excluded, input.Blocks, coverage); err != nil {
			return result, err
		}
	}
	for _, covered := range coverage {
		if !covered {
			return result, errors.New("episode map leaves uncovered source")
		}
	}
	return result, nil
}
func buildTextStructure(input TextPlanningInput, result TextPlanningResult) (TextPlanningResult, error) {
	var candidate struct {
		EpisodeKey string `json:"episode_key"`
		Scenes     []struct {
			textRange
			Key          string `json:"key"`
			Title        string `json:"title"`
			Summary      string `json:"summary"`
			TimeLabel    string `json:"time_label"`
			TimeBranch   string `json:"time_branch"`
			Presentation string `json:"presentation"`
			Beats        []struct {
				Key      string         `json:"key"`
				Action   string         `json:"action"`
				Evidence []textEvidence `json:"evidence"`
				Required bool           `json:"required"`
				Origin   string         `json:"origin"`
			} `json:"beats"`
			Dialogues []struct {
				Key            string       `json:"key"`
				SpeakerMention *string      `json:"speaker_mention"`
				Text           string       `json:"text"`
				Evidence       textEvidence `json:"evidence"`
				Channel        string       `json:"channel"`
			} `json:"dialogues"`
			Mentions []textMention `json:"mentions"`
		} `json:"scenes"`
	}
	if json.Unmarshal(input.Candidate, &candidate) != nil || len(candidate.Scenes) == 0 || len(candidate.Scenes) > 200 || !textKey.MatchString(candidate.EpisodeKey) {
		return result, errors.New("invalid episode analysis")
	}
	epID, scriptID := input.IDMapping["episode/"+candidate.EpisodeKey], input.IDMapping["script/"+candidate.EpisodeKey]
	if epID == "" || scriptID == "" {
		return result, errors.New("episode map has not been adopted")
	}
	structure := domain.Structure{ID: input.NewID(), WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID, EpisodeID: epID, ScriptVersionID: scriptID, Status: "confirmed", Revision: 1, ConfirmedBy: &input.CreatedBy, ConfirmedAt: &input.CreatedAt, CreatedBy: input.CreatedBy, CreatedAt: input.CreatedAt, UpdatedAt: input.CreatedAt, Scenes: []domain.Scene{}}
	result.IDMapping["structure/"+candidate.EpisodeKey] = structure.ID
	seen := map[string]bool{}
	coverage := make([]bool, len(input.Blocks))
	for index, s := range candidate.Scenes {
		if !textKey.MatchString(s.Key) || seen[s.Key] || strings.TrimSpace(s.Title) == "" {
			return result, errors.New("invalid scene identity")
		}
		seen[s.Key] = true
		start, end, err := coverTextRange(s.textRange, input.Blocks, coverage)
		if err != nil {
			return result, err
		}
		scene := domain.Scene{ID: input.NewID(), TemporaryKey: s.Key, Heading: s.Title, Position: index + 1, SourceStart: start, SourceEnd: end, Dialogues: []domain.Dialogue{}, NarrativeUnits: []domain.NarrativeUnit{}, Tasks: []domain.ProductionTask{}, TextFacts: &domain.TextSceneFacts{Summary: s.Summary, TimeLabel: s.TimeLabel, TimeBranch: s.TimeBranch, Presentation: s.Presentation, Mentions: []domain.TextMention{}}}
		prefix := candidate.EpisodeKey + "/" + s.Key
		result.IDMapping["scene/"+prefix] = scene.ID
		mentions := map[string]domain.TextMention{}
		for _, m := range s.Mentions {
			if !textKey.MatchString(m.Key) {
				return result, errors.New("invalid mention key")
			}
			if _, ok := mentions[m.Key]; ok {
				return result, errors.New("duplicate mention")
			}
			e, err := resolveTextEvidence(input, m.Evidence, start, end)
			if err != nil {
				return result, err
			}
			visualDetails := make([]bible.Evidence, 0, len(m.VisualDetails))
			for _, detail := range m.VisualDetails {
				resolved, detailErr := resolveTextEvidence(input, detail, start, end)
				if detailErr != nil {
					return result, detailErr
				}
				visualDetails = append(visualDetails, resolved)
			}
			value := domain.TextMention{ID: input.NewID(), TemporaryKey: m.Key, Kind: m.Kind, Name: m.Name, Presence: m.Presence, Evidence: e, VisualDetails: visualDetails}
			mentions[m.Key] = value
			scene.TextFacts.Mentions = append(scene.TextFacts.Mentions, value)
			result.IDMapping["mention/"+prefix+"/"+m.Key] = value.ID
		}
		beats := map[string]bool{}
		for _, b := range s.Beats {
			if !textKey.MatchString(b.Key) || beats[b.Key] || len(b.Evidence) == 0 || strings.TrimSpace(b.Action) == "" {
				return result, errors.New("invalid beat")
			}
			beats[b.Key] = true
			ev := []bible.Evidence{}
			for _, e := range b.Evidence {
				value, err := resolveTextEvidence(input, e, start, end)
				if err != nil {
					return result, err
				}
				ev = append(ev, value)
			}
			required := b.Required
			unit := domain.NarrativeUnit{ID: input.NewID(), TemporaryKey: b.Key, Kind: "action", Text: b.Action, SourceStart: ev[0].SourceStart, SourceEnd: ev[0].SourceEnd, Evidence: ev, Required: &required, Origin: b.Origin}
			scene.NarrativeUnits = append(scene.NarrativeUnits, unit)
			result.IDMapping["beat/"+prefix+"/"+b.Key] = unit.ID
		}
		if len(scene.NarrativeUnits) == 0 {
			return result, errors.New("scene has no narrative units")
		}
		dialogues := map[string]bool{}
		for _, d := range s.Dialogues {
			if !textKey.MatchString(d.Key) || dialogues[d.Key] {
				return result, errors.New("invalid dialogue key")
			}
			dialogues[d.Key] = true
			e, err := resolveTextEvidence(input, d.Evidence, start, end)
			if err != nil || d.Text != d.Evidence.Quote {
				return result, errors.New("dialogue does not match frozen source")
			}
			speaker, mentionID := "未知说话人", ""
			if d.SpeakerMention != nil {
				m, ok := mentions[*d.SpeakerMention]
				if !ok {
					return result, errors.New("unknown dialogue speaker mention")
				}
				speaker, mentionID = m.Name, m.ID
			}
			dialogue := domain.Dialogue{ID: input.NewID(), TemporaryKey: d.Key, Speaker: speaker, Text: d.Text, SourceStart: e.SourceStart, SourceEnd: e.SourceEnd, Evidence: []bible.Evidence{e}, Channel: d.Channel, SpeakerMentionID: mentionID}
			scene.Dialogues = append(scene.Dialogues, dialogue)
			result.IDMapping["dialogue/"+prefix+"/"+d.Key] = dialogue.ID
		}
		structure.Scenes = append(structure.Scenes, scene)
	}
	raw, err := json.Marshal(structure.Scenes)
	if err != nil {
		return result, err
	}
	structure.ResultHash, err = canonical.Hash(raw)
	if err != nil {
		return result, err
	}
	result.Structure = &structure
	return result, nil
}
func coverTextRange(value textRange, blocks [][2]int, coverage []bool) (int, int, error) {
	if value.First < 0 || value.Last < value.First || value.Last >= len(blocks) {
		return 0, 0, errors.New("source range outside frozen blocks")
	}
	for i := value.First; i <= value.Last; i++ {
		if coverage[i] {
			return 0, 0, errors.New("overlapping source ranges")
		}
		coverage[i] = true
	}
	return blocks[value.First][0], blocks[value.Last][1], nil
}
func resolveTextEvidence(input TextPlanningInput, e textEvidence, start, end int) (bible.Evidence, error) {
	fail := errors.New("evidence is outside the exact source scene")
	if e.Block < 0 || e.Block >= len(input.Blocks) || e.Quote == "" {
		return bible.Evidence{}, fail
	}
	b := input.Blocks[e.Block]
	text := string([]rune(input.SourceText)[b[0]:b[1]])
	offset := strings.Index(text, e.Quote)
	if offset < 0 {
		return bible.Evidence{}, fail
	}
	if e.Occurrence != nil {
		if *e.Occurrence < 0 {
			return bible.Evidence{}, fail
		}
		for i := 0; i < *e.Occurrence; i++ {
			next := strings.Index(text[offset+1:], e.Quote)
			if next < 0 {
				return bible.Evidence{}, fail
			}
			offset += 1 + next
		}
	} else if strings.Contains(text[offset+1:], e.Quote) {
		return bible.Evidence{}, fail
	}
	absolute := b[0] + utf8.RuneCountInString(text[:offset])
	last := absolute + utf8.RuneCountInString(e.Quote)
	if absolute < start || last > end {
		return bible.Evidence{}, fail
	}
	return bible.Evidence{SourceStart: absolute, SourceEnd: last, TextHash: textHash(e.Quote), ExactAnchor: e.Quote}, nil
}
func textHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// TextPublicationEvents uses the same publication contract as other script owners.
func TextPublicationEvents(input TextPlanningInput, versions []Version) ([]domain.OutboxEvent, error) {
	return episodeSetEvents(input.NewID, input.CreatedAt, input.SourceReceiptID,
		EpisodeSegmentationSource{WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID}, versions)
}
