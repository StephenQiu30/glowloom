package domain

import (
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ErrStoryboardConflict indicates that an existing board or shot revision changed.
var ErrStoryboardConflict = errors.New("storyboard revision conflict")

// Storyboard is the ordered content of the ordinary canvas storyboard node.
type Storyboard struct {
	Shots []StoryboardShot `json:"shots"`
}

// StoryboardDraft contains editable content supplied by the editor or Agent.
type StoryboardDraft struct {
	Title      string `json:"title"`
	Action     string `json:"action"`
	DurationMS int64  `json:"duration_ms"`
	Dialogue   string `json:"dialogue,omitempty"`
	Camera     string `json:"camera,omitempty"`
	Sound      string `json:"sound,omitempty"`
}

// StoryboardShot keeps identity and content revision separate from order.
type StoryboardShot struct {
	StoryboardDraft
	ID       uuid.UUID       `json:"id"`
	Revision int64           `json:"revision"`
	Adopted  *StoryboardTake `json:"adopted,omitempty"`
}

// StoryboardTake references the adopted formal operation output and asset.
type StoryboardTake struct {
	OperationID  uuid.UUID `json:"operation_id"`
	OutputID     uuid.UUID `json:"output_id"`
	AssetID      uuid.UUID `json:"asset_id"`
	AssetVersion int64     `json:"asset_version"`
	ShotRevision int64     `json:"shot_revision"`
}

// StoryboardCommand describes a single atomic create, field edit, or reorder.
type StoryboardCommand struct {
	ExpectedRevision int64             `json:"expected_revision"`
	BoardID          *uuid.UUID        `json:"board_id,omitempty"`
	Shots            []StoryboardDraft `json:"shots,omitempty"`
	Edit             *StoryboardEdit   `json:"edit,omitempty"`
	Move             *StoryboardMove   `json:"move,omitempty"`
}

// StoryboardEdit targets one field of the observed shot revision.
type StoryboardEdit struct {
	ShotID           uuid.UUID       `json:"shot_id"`
	ExpectedRevision int64           `json:"expected_revision"`
	Field            string          `json:"field"`
	Value            json.RawMessage `json:"value" swaggertype:"object"`
}

// StoryboardMove reorders one stable shot relative to another, or to the end.
type StoryboardMove struct {
	ShotID   uuid.UUID  `json:"shot_id"`
	BeforeID *uuid.UUID `json:"before_id,omitempty"`
}

// StoryboardNode reads the ordinary board from its owning document.
func StoryboardNode(doc Document) (Node, bool) {
	for _, node := range doc.Nodes {
		if node.NodeType == NodeTypeStoryboard && node.Config.Storyboard != nil {
			return node, true
		}
	}
	return Node{}, false
}

// StoryboardContentChanged prevents a raw or generic update from changing the
// formally stored board, while allowing presentation-only node edits.
func StoryboardContentChanged(before, after Document) bool {
	old, exists := StoryboardNode(before)
	current, found := StoryboardNode(after)
	if exists != found || old.ID != current.ID {
		return true
	}
	count := 0
	for _, node := range after.Nodes {
		if node.NodeType == NodeTypeStoryboard {
			count++
		}
	}
	return count > 1 || exists && !equalJSON(effectJSON(old.Config.Storyboard), effectJSON(current.Config.Storyboard))
}

// PrepareStoryboardOperation validates a change and uses its original command
// identity to allocate stable IDs before Yjs computation.
func PrepareStoryboardOperation(doc Document, commandID uuid.UUID, c StoryboardCommand) (Operation, error) {
	if commandID == uuid.Nil || c.ExpectedRevision < 1 || doc.Kind == "short_drama" {
		return Operation{}, ErrInvalidCommand
	}
	if c.ExpectedRevision != doc.Revision {
		return Operation{}, ErrStoryboardConflict
	}
	board, found := StoryboardNode(doc)
	if c.BoardID == nil {
		if found {
			return Operation{}, ErrStoryboardConflict
		}
		if c.Edit != nil || c.Move != nil || len(c.Shots) < 1 || len(c.Shots) > 64 {
			return Operation{}, ErrInvalidCommand
		}
		boardID := uuid.NewSHA1(commandID, []byte("storyboard:"+doc.ID.String()))
		shots := make([]StoryboardShot, 0, len(c.Shots))
		for i, draft := range c.Shots {
			if !validStoryboardDraft(draft) {
				return Operation{}, ErrInvalidCommand
			}
			shots = append(shots, StoryboardShot{StoryboardDraft: draft,
				ID: uuid.NewSHA1(boardID, []byte{byte(i)}), Revision: 1})
		}
		return Operation{Type: OperationCreateNode, Node: &Node{
			ID: boardID, Title: "故事板", NodeType: NodeTypeStoryboard, NodeAction: NodeActionTool,
			Config: NodeConfig{Storyboard: &Storyboard{Shots: shots}},
		}}, nil
	}
	if !found || board.ID != *c.BoardID {
		return Operation{}, ErrStoryboardConflict
	}
	if c.Shots != nil || (c.Edit == nil) == (c.Move == nil) {
		return Operation{}, ErrInvalidCommand
	}
	shots := slices.Clone(board.Config.Storyboard.Shots)
	if c.Edit != nil {
		index := storyboardShotIndex(shots, c.Edit.ShotID)
		if index < 0 || shots[index].Revision != c.Edit.ExpectedRevision || shots[index].Revision == math.MaxInt64 {
			return Operation{}, ErrStoryboardConflict
		}
		if err := editStoryboardShot(&shots[index], *c.Edit); err != nil {
			return Operation{}, err
		}
	} else {
		index := storyboardShotIndex(shots, c.Move.ShotID)
		if index < 0 || c.Move.BeforeID != nil && *c.Move.BeforeID == c.Move.ShotID {
			return Operation{}, ErrStoryboardConflict
		}
		shot := shots[index]
		shots = slices.Delete(shots, index, index+1)
		to := len(shots)
		if c.Move.BeforeID != nil {
			to = storyboardShotIndex(shots, *c.Move.BeforeID)
			if to < 0 {
				return Operation{}, ErrStoryboardConflict
			}
		}
		shots = slices.Insert(shots, to, shot)
	}
	return storyboardReplacement(board, shots), nil
}

func storyboardShotIndex(shots []StoryboardShot, id uuid.UUID) int {
	return slices.IndexFunc(shots, func(shot StoryboardShot) bool { return shot.ID == id })
}

func storyboardReplacement(before Node, shots []StoryboardShot) Operation {
	present := true
	return Operation{Type: "set", NodeID: before.ID, Path: []string{"config", "storyboard"},
		ExpectedExists: &present, Expected: effectJSON(before.Config),
		Value: effectJSON(NodeConfig{Storyboard: &Storyboard{Shots: shots}})}
}

func editStoryboardShot(shot *StoryboardShot, edit StoryboardEdit) error {
	if len(edit.Value) == 0 || string(edit.Value) == "null" {
		return ErrInvalidCommand
	}
	draft := shot.StoryboardDraft
	var target any
	switch edit.Field {
	case "title":
		target = &draft.Title
	case "action":
		target = &draft.Action
	case "duration_ms":
		target = &draft.DurationMS
	case "dialogue":
		target = &draft.Dialogue
	case "camera":
		target = &draft.Camera
	case "sound":
		target = &draft.Sound
	default:
		return ErrInvalidCommand
	}
	if json.Unmarshal(edit.Value, target) != nil || !validStoryboardDraft(draft) {
		return ErrInvalidCommand
	}
	if draft != shot.StoryboardDraft {
		shot.StoryboardDraft = draft
		shot.Revision++
	}
	return nil
}

func validStoryboardDraft(s StoryboardDraft) bool {
	return validTitle(strings.TrimSpace(s.Title)) && strings.TrimSpace(s.Action) != "" &&
		s.DurationMS > 0 && s.DurationMS <= MaxTimelineDurationMS &&
		validStoryboardText(s.Action) && validStoryboardText(s.Dialogue) &&
		validStoryboardText(s.Camera) && validStoryboardText(s.Sound)
}

func validStoryboardText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= 2000
}

func validStoryboard(s Storyboard) bool {
	if len(s.Shots) < 1 || len(s.Shots) > 64 {
		return false
	}
	ids := map[uuid.UUID]bool{}
	for _, shot := range s.Shots {
		if shot.ID == uuid.Nil || ids[shot.ID] || shot.Revision < 1 || !validStoryboardDraft(shot.StoryboardDraft) {
			return false
		}
		ids[shot.ID] = true
		if take := shot.Adopted; take != nil && (take.OperationID == uuid.Nil || take.OutputID == uuid.Nil || take.AssetID == uuid.Nil ||
			take.AssetVersion < 1 || take.ShotRevision < 1 || take.ShotRevision > shot.Revision) {
			return false
		}
	}
	return len(effectJSON(s)) <= 256<<10
}

// StoryboardShotTarget pins the current board and one of its content revisions.
type StoryboardShotTarget struct {
	ExpectedRevision int64     `json:"expected_revision"`
	BoardID          uuid.UUID `json:"board_id"`
	ShotID           uuid.UUID `json:"shot_id"`
	ShotRevision     int64     `json:"shot_revision"`
}

// StoryboardAdoption is the user's explicit choice of a finished candidate.
type StoryboardAdoption struct {
	StoryboardShotTarget
	OperationID uuid.UUID `json:"operation_id"`
	OutputID    uuid.UUID `json:"output_id"`
}

// PrepareStoryboardAdoption updates the selected take without changing the shot
// content revision. Old outputs remain in the generation task history.
func PrepareStoryboardAdoption(doc Document, input StoryboardAdoption, take StoryboardTake) (Operation, error) {
	board, found := StoryboardNode(doc)
	if doc.Kind == "short_drama" || !found || board.ID != input.BoardID || doc.Revision != input.ExpectedRevision {
		return Operation{}, ErrStoryboardConflict
	}
	shots := slices.Clone(board.Config.Storyboard.Shots)
	index := storyboardShotIndex(shots, input.ShotID)
	if index < 0 || shots[index].Revision != input.ShotRevision || take.ShotRevision != input.ShotRevision {
		return Operation{}, ErrStoryboardConflict
	}
	if take.OperationID == uuid.Nil || take.OperationID != input.OperationID ||
		take.OutputID == uuid.Nil || take.OutputID != input.OutputID || take.AssetID == uuid.Nil || take.AssetVersion < 1 {
		return Operation{}, ErrInvalidCommand
	}
	shots[index].Adopted = &take
	return storyboardReplacement(board, shots), nil
}
