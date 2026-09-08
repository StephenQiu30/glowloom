package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// These wire types describe the bounded text tasks. Owners convert them into
// their own accepted records; the wire payload never becomes a formal version.
type TextSource struct {
	RevisionID  string `json:"revision_id"`
	ContentHash string `json:"content_hash"`
	Text        string `json:"text"`
}
type TextEvidence struct {
	Block      int    `json:"block"`
	Quote      string `json:"quote"`
	Occurrence *int   `json:"occurrence,omitempty"`
}
type TextIssue struct {
	Code     string `json:"code"`
	Scope    string `json:"scope"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}
type TextMentionRef struct {
	EpisodeKey string `json:"episode_key"`
	SceneKey   string `json:"scene_key"`
	MentionKey string `json:"mention_key"`
}
type TextMention struct {
	Key           string         `json:"key"`
	Kind          string         `json:"kind"`
	Name          string         `json:"name"`
	Presence      string         `json:"presence"`
	Evidence      TextEvidence   `json:"evidence"`
	VisualDetails []TextEvidence `json:"visual_details"`
}
type TextBeat struct {
	Key      string         `json:"key"`
	Action   string         `json:"action"`
	Evidence []TextEvidence `json:"evidence"`
	Required bool           `json:"required"`
	Origin   string         `json:"origin"`
}
type TextDialogue struct {
	Key            string       `json:"key"`
	SpeakerMention *string      `json:"speaker_mention"`
	Text           string       `json:"text"`
	Evidence       TextEvidence `json:"evidence"`
	Channel        string       `json:"channel"`
}
type TextScene struct {
	Key          string         `json:"key"`
	Title        string         `json:"title"`
	Summary      string         `json:"summary"`
	FirstBlock   int            `json:"first_block"`
	LastBlock    int            `json:"last_block"`
	TimeLabel    string         `json:"time_label"`
	TimeBranch   string         `json:"time_branch"`
	Presentation string         `json:"presentation"`
	Beats        []TextBeat     `json:"beats"`
	Dialogues    []TextDialogue `json:"dialogues"`
	Mentions     []TextMention  `json:"mentions"`
	Issues       []TextIssue    `json:"issues"`
}
type TextAnalysis struct {
	EpisodeKey   string            `json:"episode_key"`
	Scenes       []TextScene       `json:"scenes"`
	Summary      string            `json:"summary"`
	Conflict     string            `json:"conflict"`
	TurningPoint string            `json:"turning_point"`
	EndingHook   string            `json:"ending_hook"`
	Excluded     []json.RawMessage `json:"excluded"`
	Issues       []TextIssue       `json:"issues"`
}
type TextEntity struct {
	Key           string           `json:"key"`
	Kind          string           `json:"kind"`
	Label         string           `json:"label"`
	Mentions      []TextMentionRef `json:"mentions"`
	IdentityBasis string           `json:"identity_basis"`
	Evidence      []TextEvidence   `json:"evidence"`
	Uncertainty   *string          `json:"uncertainty"`
}
type TextRelation struct {
	Subject   string         `json:"subject"`
	Predicate string         `json:"predicate"`
	Target    string         `json:"target"`
	Origin    string         `json:"origin"`
	Basis     string         `json:"basis"`
	Evidence  []TextEvidence `json:"evidence"`
}
type TextStateEvent struct {
	EntityKey  string         `json:"entity_key"`
	EpisodeKey string         `json:"episode_key"`
	SceneKey   string         `json:"scene_key"`
	TimeBranch string         `json:"time_branch"`
	StoryTime  string         `json:"story_time"`
	Property   string         `json:"property"`
	Before     *string        `json:"before"`
	After      *string        `json:"after"`
	Knowledge  string         `json:"knowledge"`
	Basis      string         `json:"basis"`
	Evidence   []TextEvidence `json:"evidence"`
}
type TextAssetNeed struct {
	EntityKey   string         `json:"entity_key"`
	Description string         `json:"description"`
	Evidence    []TextEvidence `json:"evidence"`
}
type TextWorld struct {
	Entities           []TextEntity     `json:"entities"`
	UnresolvedMentions []TextMentionRef `json:"unresolved_mentions"`
	Relations          []TextRelation   `json:"relations"`
	StateEvents        []TextStateEvent `json:"state_events"`
	AssetNeeds         []TextAssetNeed  `json:"asset_needs"`
	Issues             []TextIssue      `json:"issues"`
}
type TextBlocking struct {
	MentionKey string `json:"mention_key"`
	Position   string `json:"position"`
	Facing     string `json:"facing"`
	Action     string `json:"action"`
}
type TextAudio struct {
	DialogueKey string `json:"dialogue_key"`
	Channel     string `json:"channel"`
}
type TextDirectedShot struct {
	Key             string         `json:"key"`
	Purpose         string         `json:"purpose"`
	Framing         string         `json:"framing"`
	CameraMovement  string         `json:"camera_movement"`
	Action          string         `json:"action"`
	BeatKeys        []string       `json:"beat_keys"`
	Audio           []TextAudio    `json:"audio"`
	VisibleMentions []string       `json:"visible_mentions"`
	DetailEvidence  []TextEvidence `json:"detail_evidence"`
	DurationMinMS   int            `json:"duration_min_ms"`
	DurationMaxMS   int            `json:"duration_max_ms"`
	TimingBasis     string         `json:"timing_basis"`
	ScreenDirection string         `json:"screen_direction"`
	EntryState      string         `json:"entry_state"`
	ExitState       string         `json:"exit_state"`
	PanelCaption    string         `json:"panel_caption"`
}
type TextDirection struct {
	EpisodeKey     string             `json:"episode_key"`
	SceneKey       string             `json:"scene_key"`
	DramaticIntent string             `json:"dramatic_intent"`
	AudienceKnows  []string           `json:"audience_knows"`
	Withhold       []string           `json:"withhold"`
	Blocking       []TextBlocking     `json:"blocking"`
	Shots          []TextDirectedShot `json:"shots"`
	Issues         []TextIssue        `json:"issues"`
}
type TextExecutionTask struct {
	InvocationID   string          `json:"invocation_id"`
	Stage          string          `json:"stage"`
	Source         TextSource      `json:"source"`
	ReleaseHash    string          `json:"release_hash"`
	EpisodeMap     json.RawMessage `json:"episode_map"`
	Analyses       []TextAnalysis  `json:"analyses"`
	World          *TextWorld      `json:"world"`
	EpisodeKey     *string         `json:"episode_key"`
	SceneKey       *string         `json:"scene_key"`
	TimeoutSeconds int             `json:"timeout_seconds"`
}

var textKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func ValidTextKey(value string) bool { return textKeyPattern.MatchString(value) }
func ValidTextID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
func TextWireHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
func DecodeTextWire(raw []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("trailing text payload")
	}
	return nil
}
func ValidateTextSource(source TextSource, revisionID, hash string) error {
	if !ValidTextID(source.RevisionID) || source.RevisionID != revisionID || source.ContentHash != hash || !utf8.ValidString(source.Text) || strings.TrimSpace(source.Text) == "" || strings.Contains(source.Text, "\r") || utf8.RuneCountInString(source.Text) > 2_000_000 || TextWireHash(source.Text) != hash {
		return errors.New("text source identity or content drift")
	}
	return nil
}

type textEvidenceBlock struct {
	start int
	text  string
}

// TextEvidenceIndex indexes a task's source once, so verifying many references
// does not repeatedly scan and allocate the entire manuscript.
type TextEvidenceIndex struct{ blocks []textEvidenceBlock }

func NewTextEvidenceIndex(source TextSource) *TextEvidenceIndex {
	runes := []rune(source.Text)
	index := &TextEvidenceIndex{}
	start := 0
	for i, r := range runes {
		switch r {
		case '\n', '\v', '\f', '\x1c', '\x1d', '\x1e', '\u0085', '\u2028', '\u2029':
			index.blocks = append(index.blocks, textEvidenceBlock{start: start, text: string(runes[start : i+1])})
			start = i + 1
		}
	}
	if start < len(runes) {
		index.blocks = append(index.blocks, textEvidenceBlock{start: start, text: string(runes[start:])})
	}
	return index
}

// Resolve uses Python splitlines Unicode boundaries and code-point offsets.
// Ambiguous quotes require an explicit occurrence, including overlapping matches.
func (index *TextEvidenceIndex) Resolve(e TextEvidence) (int, int, error) {
	if e.Block < 0 || e.Block >= len(index.blocks) || e.Quote == "" || !utf8.ValidString(e.Quote) || (e.Occurrence != nil && (*e.Occurrence < 0 || *e.Occurrence > 2_000_000)) {
		return 0, 0, errors.New("invalid source evidence")
	}
	block := index.blocks[e.Block]
	offset := strings.Index(block.text, e.Quote)
	occurrence := 0
	if e.Occurrence != nil {
		occurrence = *e.Occurrence
	} else if offset >= 0 && strings.Contains(block.text[offset+1:], e.Quote) {
		return 0, 0, errors.New("ambiguous source evidence")
	}
	for count := 0; count < occurrence && offset >= 0; count++ {
		next := strings.Index(block.text[offset+1:], e.Quote)
		if next < 0 {
			offset = -1
		} else {
			offset += next + 1
		}
	}
	if offset < 0 {
		return 0, 0, errors.New("source quote not found")
	}
	start := block.start + utf8.RuneCountInString(block.text[:offset])
	return start, start + utf8.RuneCountInString(e.Quote), nil
}
