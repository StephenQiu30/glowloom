// Package domain defines the canvas graph and atomic layout command rules.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

var (
	// ErrInvalidCommand rejects malformed or out-of-document edits.
	ErrInvalidCommand = errors.New("invalid canvas command")
	// ErrUnsupportedCommand rejects business mutation and generation capabilities.
	ErrUnsupportedCommand = errors.New("unsupported canvas command")
)

// NodeConfig contains only user-editable, type-specific presentation data.
type NodeConfig struct {
	Container     *ContainerConfig        `json:"container,omitempty"`
	CinemaKit     *promptdomain.CinemaKit `json:"cinema_kit,omitempty"`
	Subject       *SubjectConfig          `json:"subject,omitempty"`
	Editor        *EditorConfig           `json:"editor,omitempty"`
	RichText      json.RawMessage         `json:"rich_text,omitempty" swaggertype:"object"`
	Composer      *promptdomain.Draft     `json:"composer,omitempty"`
	Format        string                  `json:"format,omitempty"`
	FormatVersion int                     `json:"format_version,omitempty"`
	Draft         *Draft                  `json:"draft,omitempty"`
	Text          *string                 `json:"text,omitempty"`
	Collapsed     *bool                   `json:"collapsed,omitempty"`
	BatchTable    *BatchTableConfig       `json:"batch_table,omitempty"`
	Timeline      *TimelineConfig         `json:"timeline,omitempty"`
	Director      *DirectorConfig         `json:"director,omitempty"`
	Generation    *GenerationConfig       `json:"generation,omitempty"`
	unknown       json.RawMessage
}

// Viewport stores the document camera in world coordinates.
type Viewport struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

// Node is a resource or a protected business projection. Media identity is never a URL.
type Node struct {
	InputRevision   int64      `json:"input_revision,omitempty"`
	ContentVersion  int64      `json:"content_version,omitempty,string" swaggertype:"string"`
	ID              uuid.UUID  `json:"id"`
	Title           string     `json:"title"`
	NodeType        string     `json:"node_type"`
	NodeAction      string     `json:"node_action"`
	Config          NodeConfig `json:"config"`
	X               float64    `json:"x"`
	Y               float64    `json:"y"`
	Width           *float64   `json:"width,omitempty"`
	Height          *float64   `json:"height,omitempty"`
	RefType         string     `json:"ref_type,omitempty"`
	RefID           *uuid.UUID `json:"ref_id,omitempty"`
	ParentID        *uuid.UUID `json:"parent_id,omitempty"`
	LastOperationID *uuid.UUID `json:"last_operation_id,omitempty"`
	ZIndex          int32      `json:"z_index"`
}

// Edge is a visual annotation or protected business relation.
type Edge struct {
	ID           uuid.UUID       `json:"id"`
	EdgeType     string          `json:"edge_type"`
	SourceNodeID uuid.UUID       `json:"source_node_id"`
	TargetNodeID uuid.UUID       `json:"target_node_id"`
	Role         string          `json:"role,omitempty"`
	Binding      json.RawMessage `json:"binding,omitempty" swaggertype:"object"`
}

// Document is the authoritative current graph.
type Document struct {
	AssetCollection  string          `json:"asset_collection,omitempty"`
	Kind             string          `json:"kind,omitempty"`
	SchemaVersion    int             `json:"schema_version,omitempty"`
	Writable         bool            `json:"-"`
	OperationVersion int             `json:"operation_version,omitempty"`
	ReadOnlyReason   string          `json:"read_only_reason,omitempty"`
	ID               uuid.UUID       `json:"id"`
	ProjectID        uuid.UUID       `json:"project_id"`
	Name             string          `json:"name"`
	Scope            json.RawMessage `json:"scope" swaggertype:"object"`
	Revision         int64           `json:"revision"`
	Viewport         Viewport        `json:"viewport"`
	Nodes            []Node          `json:"nodes"`
	Edges            []Edge          `json:"edges"`
}

// Move changes absolute world position; moving a group does not implicitly move children.
type Move struct {
	ID uuid.UUID `json:"id"`
	X  float64   `json:"x"`
	Y  float64   `json:"y"`
}

// Size changes a node's presentation dimensions.
type Size struct {
	ID     uuid.UUID `json:"id"`
	Width  float64   `json:"width"`
	Height float64   `json:"height"`
}

// Name changes the user-facing node title.
type Name struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
}

// Parent groups or unparents a node without changing its world position.
type Parent struct {
	ID       uuid.UUID  `json:"id"`
	ParentID *uuid.UUID `json:"parent_id" extensions:"x-nullable"`
}

// ZIndex changes stacking order.
type ZIndex struct {
	ID     uuid.UUID `json:"id"`
	ZIndex int32     `json:"z_index"`
}

// Command is the closed union of resource layout, annotation and camera commands.
type Command struct {
	Type     string      `json:"type"`
	Nodes    []Node      `json:"nodes,omitempty"`
	Moves    []Move      `json:"moves,omitempty"`
	Sizes    []Size      `json:"sizes,omitempty"`
	Names    []Name      `json:"names,omitempty"`
	Parents  []Parent    `json:"parents,omitempty"`
	ZIndices []ZIndex    `json:"z_indices,omitempty"`
	ID       uuid.UUID   `json:"id,omitempty"`
	IDs      []uuid.UUID `json:"ids,omitempty"`
	Config   *NodeConfig `json:"config,omitempty"`
	Edges    []Edge      `json:"edges,omitempty"`
	Viewport *Viewport   `json:"viewport,omitempty"`
}

// CommandError preserves the classified cause and zero-based failing command index.
type CommandError struct {
	Index int
	Cause error
}

func (e *CommandError) Error() string { return fmt.Sprintf("canvas command %d: %v", e.Index, e.Cause) }
func (e *CommandError) Unwrap() error { return e.Cause }

// InputSnapshot is the final immutable generation or subject input of one accepted Yjs projection.
type InputSnapshot struct {
	Subject  *SubjectConfig
	Title    string
	NodeID   uuid.UUID
	Revision int64
	Draft    Draft
}

// Apply evaluates a private graph for structural validation and project copy.
// Shared edits and immutable input snapshots are owned by ApplyOperations.
func Apply(document Document, commands []Command) (Document, error) {
	if len(commands) < 1 || len(commands) > 100 {
		return Document{}, ErrInvalidCommand
	}
	if err := ValidateGraph(document); err != nil {
		return Document{}, fmt.Errorf("%w: %w", ErrGraphReadOnly, err)
	}
	doc := document
	doc.Nodes = slices.Clone(document.Nodes)
	doc.Edges = slices.Clone(document.Edges)
	for index, command := range commands {
		if err := applyOne(&doc, command); err != nil {
			return Document{}, &CommandError{Index: index, Cause: err}
		}
		if len(doc.Nodes) > 2000 || len(doc.Edges) > 4000 {
			return Document{}, &CommandError{Index: index, Cause: ErrInvalidCommand}
		}
	}
	if err := ValidateGraph(doc); err != nil {
		return Document{}, &CommandError{Index: len(commands) - 1, Cause: err}
	}
	if err := validateTimelineRelationChanges(document, doc); err != nil {
		return Document{}, &CommandError{Index: len(commands) - 1, Cause: err}
	}
	if err := validateSubjectRelationChanges(document, doc); err != nil {
		return Document{}, &CommandError{Index: len(commands) - 1, Cause: err}
	}
	return doc, nil
}
func applyOne(doc *Document, c Command) error {
	kinds := []string{"AddNodes", "MoveNodes", "ResizeNodes", "UpdateNodeConfig", "RenameNodes", "DeleteNodes", "SetNodeParents", "SetNodeZIndex", "Connect", "Disconnect", "SetViewport"}
	if !slices.Contains(kinds, c.Type) {
		return ErrUnsupportedCommand
	}
	populated := 0
	for _, yes := range []bool{c.Nodes != nil, c.Moves != nil, c.Sizes != nil, c.Names != nil, c.Parents != nil, c.ZIndices != nil, c.ID != uuid.Nil, c.IDs != nil, c.Config != nil, c.Edges != nil, c.Viewport != nil} {
		if yes {
			populated++
		}
	}
	want := 1
	if c.Type == "UpdateNodeConfig" {
		want = 2
	}
	if populated != want {
		return ErrInvalidCommand
	}
	switch c.Type {
	case "AddNodes":
		if !batchSize(len(c.Nodes)) {
			return ErrInvalidCommand
		}
		for _, n := range c.Nodes {
			if !editable(n) || n.LastOperationID != nil {
				return ErrUnsupportedCommand
			}
			if n.InputRevision != 0 || n.ID == uuid.Nil || nodeIndex(doc, n.ID) >= 0 || !validTitle(n.Title) || !coordinate(n.X) || !coordinate(n.Y) || !dimension(n.NodeType, n.Width) || !dimension(n.NodeType, n.Height) || !validConfig(n.NodeType, n.Config) || !validZIndex(n.ZIndex) {
				return ErrInvalidCommand
			}
			if IsDraft(n) || n.NodeType == "subject" {
				n.InputRevision = 1
			}
			doc.Nodes = append(doc.Nodes, n)
		}
	case "MoveNodes":
		if !batchSize(len(c.Moves)) {
			return ErrInvalidCommand
		}
		seen := map[uuid.UUID]bool{}
		for _, m := range c.Moves {
			i, err := editableIndex(doc, m.ID, seen)
			if err != nil {
				return err
			}
			if !coordinate(m.X) || !coordinate(m.Y) {
				return ErrInvalidCommand
			}
			doc.Nodes[i].X = m.X
			doc.Nodes[i].Y = m.Y
		}
	case "ResizeNodes":
		if !batchSize(len(c.Sizes)) {
			return ErrInvalidCommand
		}
		seen := map[uuid.UUID]bool{}
		for _, s := range c.Sizes {
			i, err := editableIndex(doc, s.ID, seen)
			if err != nil {
				return err
			}
			if !dimension(doc.Nodes[i].NodeType, &s.Width) || !dimension(doc.Nodes[i].NodeType, &s.Height) {
				return ErrInvalidCommand
			}
			doc.Nodes[i].Width = &s.Width
			doc.Nodes[i].Height = &s.Height
		}
	case "RenameNodes":
		if !batchSize(len(c.Names)) {
			return ErrInvalidCommand
		}
		seen := map[uuid.UUID]bool{}
		for _, n := range c.Names {
			i, err := editableIndex(doc, n.ID, seen)
			if err != nil {
				return err
			}
			if !validTitle(n.Title) {
				return ErrInvalidCommand
			}
			doc.Nodes[i].Title = n.Title
		}
	case "SetNodeParents":
		if !batchSize(len(c.Parents)) {
			return ErrInvalidCommand
		}
		seen := map[uuid.UUID]bool{}
		for _, p := range c.Parents {
			i, err := editableIndex(doc, p.ID, seen)
			if err != nil {
				return err
			}
			doc.Nodes[i].ParentID = p.ParentID
		}
	case "SetNodeZIndex":
		if !batchSize(len(c.ZIndices)) {
			return ErrInvalidCommand
		}
		seen := map[uuid.UUID]bool{}
		for _, z := range c.ZIndices {
			i, err := editableIndex(doc, z.ID, seen)
			if err != nil {
				return err
			}
			if !validZIndex(z.ZIndex) {
				return ErrInvalidCommand
			}
			doc.Nodes[i].ZIndex = z.ZIndex
		}
	case "UpdateNodeConfig":
		i, err := editableIndex(doc, c.ID, map[uuid.UUID]bool{})
		if err != nil {
			return err
		}
		if c.Config == nil || !validConfig(doc.Nodes[i].NodeType, *c.Config) {
			return ErrInvalidCommand
		}
		if IsDraft(doc.Nodes[i]) && doc.Nodes[i].Config.Draft.Composer != nil {
			if c.Config.Draft == nil || c.Config.Draft.Composer == nil || c.Config.Draft.Composer.ExtendsLabels(doc.Nodes[i].Config.Draft.Composer.LabelState) != nil {
				return ErrInvalidCommand
			}
		}
		if doc.Nodes[i].Config.Draft == nil && c.Config.Draft != nil {
			doc.Nodes[i].InputRevision = 1
		} else if IsDraft(doc.Nodes[i]) && !SameInput(*doc.Nodes[i].Config.Draft, *c.Config.Draft) {
			if doc.Nodes[i].InputRevision < 1 || doc.Nodes[i].InputRevision >= math.MaxInt32 {
				return ErrInvalidCommand
			}
			doc.Nodes[i].InputRevision++
		}
		doc.Nodes[i].Config = *c.Config
	case "DeleteNodes":
		if !batchSize(len(c.IDs)) {
			return ErrInvalidCommand
		}
		seen := map[uuid.UUID]bool{}
		for _, id := range c.IDs {
			if index := nodeIndex(doc, id); index >= 0 && isGenerationDraft(doc.Nodes[index]) {
				return ErrUnsupportedCommand
			}
			if _, err := editableIndex(doc, id, seen); err != nil {
				return err
			}
		}
		for _, e := range doc.Edges {
			if (seen[e.SourceNodeID] || seen[e.TargetNodeID]) && !annotation(e) {
				return ErrUnsupportedCommand
			}
		}
		if protectedConsumer(doc.Nodes, seen) {
			return ErrUnsupportedCommand
		}
		clearTimelineReferences(doc.Nodes, seen)
		clearSubjectReferences(doc.Nodes, seen)
		clearDirectorReferences(doc.Nodes, seen)
		doc.Nodes = slices.DeleteFunc(doc.Nodes, func(n Node) bool { return seen[n.ID] })
		clearBatchReferences(doc.Nodes, seen)
		for i, n := range doc.Nodes {
			if n.ParentID != nil && seen[*n.ParentID] {
				if !editable(n) {
					return ErrUnsupportedCommand
				}
				doc.Nodes[i].ParentID = nil
			}
		}
		doc.Edges = slices.DeleteFunc(doc.Edges, func(e Edge) bool { return seen[e.SourceNodeID] || seen[e.TargetNodeID] })
	case "Connect":
		if !batchSize(len(c.Edges)) {
			return ErrInvalidCommand
		}
		for _, e := range c.Edges {
			if !annotation(e) {
				return ErrUnsupportedCommand
			}
			if e.ID == uuid.Nil || e.SourceNodeID == e.TargetNodeID || nodeIndex(doc, e.SourceNodeID) < 0 || nodeIndex(doc, e.TargetNodeID) < 0 || slices.ContainsFunc(doc.Edges, func(a Edge) bool { return a.ID == e.ID }) {
				return ErrInvalidCommand
			}
			if !editable(doc.Nodes[nodeIndex(doc, e.SourceNodeID)]) || !editable(doc.Nodes[nodeIndex(doc, e.TargetNodeID)]) {
				return ErrUnsupportedCommand
			}
			doc.Edges = append(doc.Edges, e)
		}
	case "Disconnect":
		if !batchSize(len(c.IDs)) {
			return ErrInvalidCommand
		}
		seen := map[uuid.UUID]bool{}
		for _, id := range c.IDs {
			i := slices.IndexFunc(doc.Edges, func(e Edge) bool { return e.ID == id })
			if i < 0 || seen[id] {
				return ErrInvalidCommand
			}
			if !annotation(doc.Edges[i]) {
				return ErrUnsupportedCommand
			}
			seen[id] = true
		}
		doc.Edges = slices.DeleteFunc(doc.Edges, func(e Edge) bool { return seen[e.ID] })
	case "SetViewport":
		if c.Viewport == nil || !coordinate(c.Viewport.X) || !coordinate(c.Viewport.Y) || math.IsNaN(c.Viewport.Zoom) || c.Viewport.Zoom < 0.05 || c.Viewport.Zoom > 8 {
			return ErrInvalidCommand
		}
		doc.Viewport = *c.Viewport
	}
	return nil
}
func editableIndex(doc *Document, id uuid.UUID, seen map[uuid.UUID]bool) (int, error) {
	i := nodeIndex(doc, id)
	if i < 0 || seen[id] {
		return -1, ErrInvalidCommand
	}
	if !editable(doc.Nodes[i]) {
		return -1, ErrUnsupportedCommand
	}
	seen[id] = true
	return i, nil
}
func editable(n Node) bool {
	if n.LastOperationID != nil {
		return false
	}
	if n.NodeType == "image_generation" || n.NodeType == "video_generation" {
		return n.NodeAction == "generate" && (n.RefType == "" && n.RefID == nil || n.Config.Editor != nil && n.RefType == "media_asset" && n.RefID != nil && *n.RefID != uuid.Nil)
	}
	if n.NodeType == "cinema_studio" || n.NodeType == "batch_table" || n.NodeType == "timeline" || n.NodeType == "director" || n.NodeType == "generation" {
		return n.NodeAction == "tool" && n.RefType == "" && n.RefID == nil
	}
	if n.NodeAction != "resource" {
		return false
	}
	switch n.NodeType {
	case "text", "group", "config", "subject":
		return n.RefType == "" && n.RefID == nil
	case "image", "video", "audio", "model":
		return n.RefType == "media_asset" && n.RefID != nil && *n.RefID != uuid.Nil || n.Config.Editor != nil && n.RefType == "" && n.RefID == nil
	default:
		return false
	}
}
func annotation(e Edge) bool {
	return e.EdgeType == "annotation" && e.Role == "" && (len(e.Binding) == 0 || validEditorConnection(e.Binding))
}
func validConfig(kind string, c NodeConfig) bool {
	if c.Container != nil && (kind != "group" || !validContainer(c.Container)) {
		return false
	}
	if c.CinemaKit != nil {
		if c.CinemaKit.Validate() != nil || len(c.CinemaKit.Sources) != 0 {
			return false
		}
		if kind == "cinema_studio" {
			return len(c.CinemaKit.Kinds()) == 1 && c.Subject == nil && c.Composer == nil && c.Draft == nil && c.Text == nil && c.Collapsed == nil && c.BatchTable == nil && c.Timeline == nil && c.Director == nil && c.Generation == nil && len(c.RichText) == 0 && c.Format == "" && c.FormatVersion == 0 && len(c.unknown) == 0 && validEditor(c.Editor)
		}
		media := kind
		if kind == "image_generation" {
			media = "image"
		}
		if kind == "video_generation" {
			media = "video"
		}
		if !c.CinemaKit.Supports(media + ".generate") {
			return false
		}
	} else if kind == "cinema_studio" {
		return false
	}
	if c.Draft != nil && c.Draft.Composer != nil && c.Draft.Composer.CinemaKit != nil {
		return false
	}

	if kind == "subject" {
		return c.Subject != nil && validSubject(*c.Subject) && validEditor(c.Editor) && len(c.unknown) == 0 && len(c.RichText) == 0 && c.Composer == nil && c.Format == "" && c.FormatVersion == 0 && c.Draft == nil && c.Text == nil && c.Collapsed == nil && c.BatchTable == nil && c.Timeline == nil && c.Director == nil && c.Generation == nil
	}
	if c.Subject != nil {
		return false
	}
	if !validEditor(c.Editor) || len(c.unknown) != 0 || (len(c.RichText) > 0 && (kind != "text" || c.Composer == nil || !validRichText(c.RichText, c.Composer))) {
		return false
	}
	if kind != "text" && (c.Composer != nil || c.Format != "" || c.FormatVersion != 0) {
		return false
	}
	if kind == "image_generation" || kind == "video_generation" {
		return c.Draft != nil && validDraft(*c.Draft) && c.Text == nil && c.Collapsed == nil && c.BatchTable == nil && c.Timeline == nil && c.Director == nil && c.Generation == nil
	}
	if c.Draft != nil {
		switch kind {
		case "text", "image", "video", "audio":
			if c.Editor == nil || c.Draft.Composer == nil || !validDraft(*c.Draft) {
				return false
			}
		default:
			return false
		}
	}
	if kind == "generation" {
		return c.Text == nil && c.Collapsed == nil && c.BatchTable == nil && c.Timeline == nil && c.Director == nil && c.Generation != nil && validGeneration(*c.Generation)
	}
	if c.Generation != nil {
		return false
	}
	if kind == "batch_table" {
		return c.Text == nil && c.Collapsed == nil && c.Timeline == nil && c.Director == nil && c.BatchTable != nil && validBatchTable(*c.BatchTable)
	}
	if kind == "timeline" {
		return c.Text == nil && c.Collapsed == nil && c.BatchTable == nil && c.Director == nil && c.Timeline != nil && validTimeline(*c.Timeline)
	}
	if kind == "director" {
		return c.Text == nil && c.Collapsed == nil && c.BatchTable == nil && c.Timeline == nil && c.Director != nil && validDirector(*c.Director)
	}
	if c.BatchTable != nil || c.Timeline != nil || c.Director != nil {
		return false
	}
	switch kind {
	case "text":
		if c.Composer != nil {
			return c.Format == "tiptap" && c.FormatVersion == 1 && c.Text == nil && c.Collapsed == nil && c.Composer.Validate() == nil && c.Composer.ModelVersion == nil && c.Composer.Mode == nil && len(c.Composer.Params) == 0
		}
		return (c.Format == "" && c.FormatVersion == 0 || (c.Format == "plain" || c.Format == "markdown") && c.FormatVersion == 1) && c.Text != nil && c.Collapsed == nil && utf8.ValidString(*c.Text) && utf8.RuneCountInString(*c.Text) <= 10000
	case "group":
		return c.Text == nil && c.Collapsed != nil
	case "config":
		return c.Editor != nil && c.Text == nil && c.Collapsed == nil
	case "image", "video", "audio", "model":
		return c.Text == nil && c.Collapsed == nil
	default:
		return false
	}
}
func validParents(nodes []Node) bool {
	byID := make(map[uuid.UUID]Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	state := make(map[uuid.UUID]uint8, len(nodes))
	var visit func(uuid.UUID) bool
	visit = func(id uuid.UUID) bool {
		switch state[id] {
		case 1:
			return false
		case 2:
			return true
		}
		state[id] = 1
		n := byID[id]
		if n.ParentID != nil {
			parent, ok := byID[*n.ParentID]
			if !ok || parent.NodeType != "group" || parent.NodeAction != "resource" || !visit(parent.ID) {
				return false
			}
		}
		state[id] = 2
		return true
	}
	for _, n := range nodes {
		if !visit(n.ID) {
			return false
		}
	}
	return true
}

func nodeIndex(doc *Document, id uuid.UUID) int {
	return slices.IndexFunc(doc.Nodes, func(n Node) bool { return n.ID == id })
}
func batchSize(n int) bool      { return n > 0 && n <= 2000 }
func coordinate(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) <= 1e6 }
func dimension(kind string, v *float64) bool {
	maximum := float64(2000)
	if kind == "group" {
		maximum = 100000
	}
	return v == nil || (!math.IsNaN(*v) && !math.IsInf(*v, 0) && *v >= 40 && *v <= maximum)
}
func validZIndex(v int32) bool { return v >= -1000000 && v <= 1000000 }
func validTitle(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= 128
}
