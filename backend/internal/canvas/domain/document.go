// Package domain defines the editable canvas document and atomic command rules.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	// ErrInvalidCommand rejects malformed or out-of-document edits.
	ErrInvalidCommand = errors.New("invalid canvas command")
	// ErrUnsupportedCommand rejects unopened business and generation capabilities.
	ErrUnsupportedCommand = errors.New("unsupported canvas command")
)

// TextConfig is the complete editable configuration of a note.
type TextConfig struct {
	Text string `json:"text"`
}

// Viewport stores document camera position independently of node positions.
type Viewport struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

// Node records a canvas item; references remain protected business bindings.
type Node struct {
	ID              uuid.UUID  `json:"id"`
	NodeType        string     `json:"node_type"`
	NodeAction      string     `json:"node_action"`
	Config          TextConfig `json:"config"`
	X               float64    `json:"x"`
	Y               float64    `json:"y"`
	Width           *float64   `json:"width,omitempty"`
	Height          *float64   `json:"height,omitempty"`
	RefType         string     `json:"ref_type,omitempty"`
	RefID           *uuid.UUID `json:"ref_id,omitempty"`
	ParentID        *uuid.UUID `json:"parent_id,omitempty"`
	LastOperationID *uuid.UUID `json:"last_operation_id,omitempty"`
	ZIndex          int32      `json:"z_index,omitempty"`
}

// Edge stores an annotation or a protected business connection.
type Edge struct {
	ID           uuid.UUID       `json:"id"`
	EdgeType     string          `json:"edge_type"`
	SourceNodeID uuid.UUID       `json:"source_node_id"`
	TargetNodeID uuid.UUID       `json:"target_node_id"`
	Role         string          `json:"role,omitempty"`
	Binding      json.RawMessage `json:"binding,omitempty" swaggertype:"object"`
}

// Document is the current authoritative graph snapshot.
type Document struct {
	ID        uuid.UUID       `json:"id"`
	ProjectID uuid.UUID       `json:"project_id"`
	Name      string          `json:"name"`
	Scope     json.RawMessage `json:"scope" swaggertype:"object"`
	Revision  int64           `json:"revision"`
	Viewport  Viewport        `json:"viewport"`
	Nodes     []Node          `json:"nodes"`
	Edges     []Edge          `json:"edges"`
}

// Move changes a node's position.
type Move struct {
	ID uuid.UUID `json:"id"`
	X  float64   `json:"x"`
	Y  float64   `json:"y"`
}

// Command is the closed union of supported note, annotation and camera edits.
type Command struct {
	Type     string      `json:"type"`
	Nodes    []Node      `json:"nodes,omitempty"`
	Moves    []Move      `json:"moves,omitempty"`
	ID       uuid.UUID   `json:"id,omitempty"`
	IDs      []uuid.UUID `json:"ids,omitempty"`
	Config   *TextConfig `json:"config,omitempty"`
	Edge     *Edge       `json:"edge,omitempty"`
	Viewport *Viewport   `json:"viewport,omitempty"`
}

// CommandError reports the zero-based command index while preserving its classified cause.
type CommandError struct {
	Index int
	Cause error
}

func (e *CommandError) Error() string { return fmt.Sprintf("canvas command %d: %v", e.Index, e.Cause) }
func (e *CommandError) Unwrap() error { return e.Cause }

// Apply validates every command on a private copy; callers retain their original on any failure.
func Apply(document Document, commands []Command) (Document, error) {
	if len(commands) < 1 || len(commands) > 100 {
		return Document{}, ErrInvalidCommand
	}
	doc := document
	doc.Nodes = slices.Clone(document.Nodes)
	doc.Edges = slices.Clone(document.Edges)
	for index, command := range commands {
		if err := applyOne(&doc, command); err != nil {
			return Document{}, &CommandError{Index: index, Cause: err}
		}
		if len(doc.Nodes) > 2000 {
			return Document{}, &CommandError{Index: index, Cause: ErrInvalidCommand}
		}
	}
	return doc, nil
}

func applyOne(doc *Document, c Command) error {
	if !slices.Contains([]string{"AddNodes", "MoveNodes", "UpdateNodeConfig", "DeleteNodes", "Connect", "Disconnect", "SetViewport"}, c.Type) {
		return ErrUnsupportedCommand
	}
	// Reject extra union fields as well as unknown command kinds.
	populated := 0
	for _, yes := range []bool{c.Nodes != nil, c.Moves != nil, c.ID != uuid.Nil, c.IDs != nil, c.Config != nil, c.Edge != nil, c.Viewport != nil} {
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
		if len(c.Nodes) < 1 || len(c.Nodes) > 100 {
			return ErrInvalidCommand
		}
		for _, n := range c.Nodes {
			if !editable(n) {
				return ErrUnsupportedCommand
			}
			if n.ID == uuid.Nil || nodeIndex(doc, n.ID) >= 0 || !coordinate(n.X) || !coordinate(n.Y) || !dimension(n.Width) || !dimension(n.Height) || !validText(n.Config.Text) {
				return ErrInvalidCommand
			}
			doc.Nodes = append(doc.Nodes, n)
		}
	case "MoveNodes":
		if len(c.Moves) < 1 || len(c.Moves) > 2000 {
			return ErrInvalidCommand
		}
		seen := make(map[uuid.UUID]bool)
		for _, m := range c.Moves {
			i := nodeIndex(doc, m.ID)
			if i < 0 || seen[m.ID] || !coordinate(m.X) || !coordinate(m.Y) {
				return ErrInvalidCommand
			}
			if !editable(doc.Nodes[i]) {
				return ErrUnsupportedCommand
			}
			seen[m.ID] = true
			doc.Nodes[i].X = m.X
			doc.Nodes[i].Y = m.Y
		}
	case "UpdateNodeConfig":
		i := nodeIndex(doc, c.ID)
		if i < 0 || c.Config == nil || !validText(c.Config.Text) {
			return ErrInvalidCommand
		}
		if !editable(doc.Nodes[i]) {
			return ErrUnsupportedCommand
		}
		doc.Nodes[i].Config = *c.Config
	case "DeleteNodes":
		if len(c.IDs) < 1 || len(c.IDs) > 2000 {
			return ErrInvalidCommand
		}
		seen := make(map[uuid.UUID]bool)
		for _, id := range c.IDs {
			i := nodeIndex(doc, id)
			if i < 0 || seen[id] {
				return ErrInvalidCommand
			}
			if !editable(doc.Nodes[i]) {
				return ErrUnsupportedCommand
			}
			seen[id] = true
		}
		for _, e := range doc.Edges {
			if (seen[e.SourceNodeID] || seen[e.TargetNodeID]) && e.EdgeType != "annotation" {
				return ErrUnsupportedCommand
			}
		}
		doc.Nodes = slices.DeleteFunc(doc.Nodes, func(n Node) bool { return seen[n.ID] })
		doc.Edges = slices.DeleteFunc(doc.Edges, func(e Edge) bool { return seen[e.SourceNodeID] || seen[e.TargetNodeID] })
	case "Connect":
		if c.Edge == nil {
			return ErrInvalidCommand
		}
		e := *c.Edge
		if e.EdgeType != "annotation" || e.Role != "" || len(e.Binding) != 0 {
			return ErrUnsupportedCommand
		}
		if e.ID == uuid.Nil || e.SourceNodeID == e.TargetNodeID || nodeIndex(doc, e.SourceNodeID) < 0 || nodeIndex(doc, e.TargetNodeID) < 0 || len(doc.Edges) >= 4000 {
			return ErrInvalidCommand
		}
		if !editable(doc.Nodes[nodeIndex(doc, e.SourceNodeID)]) || !editable(doc.Nodes[nodeIndex(doc, e.TargetNodeID)]) {
			return ErrUnsupportedCommand
		}
		for _, existing := range doc.Edges {
			if existing.ID == e.ID {
				return ErrInvalidCommand
			}
		}
		doc.Edges = append(doc.Edges, e)
	case "Disconnect":
		i := slices.IndexFunc(doc.Edges, func(e Edge) bool { return e.ID == c.ID })
		if i < 0 {
			return ErrInvalidCommand
		}
		if doc.Edges[i].EdgeType != "annotation" {
			return ErrUnsupportedCommand
		}
		doc.Edges = slices.Delete(doc.Edges, i, i+1)
	case "SetViewport":
		if c.Viewport == nil || !coordinate(c.Viewport.X) || !coordinate(c.Viewport.Y) || math.IsNaN(c.Viewport.Zoom) || c.Viewport.Zoom < 0.1 || c.Viewport.Zoom > 4 {
			return ErrInvalidCommand
		}
		doc.Viewport = *c.Viewport
	default:
		return ErrUnsupportedCommand
	}
	return nil
}
func editable(n Node) bool {
	return n.NodeType == "text" && n.NodeAction == "resource" && n.RefType == "" && n.RefID == nil && n.ParentID == nil && n.LastOperationID == nil
}
func nodeIndex(doc *Document, id uuid.UUID) int {
	return slices.IndexFunc(doc.Nodes, func(n Node) bool { return n.ID == id })
}
func coordinate(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) <= 1e6 }
func dimension(v *float64) bool { return v == nil || (!math.IsNaN(*v) && *v >= 40 && *v <= 2000) }
func validText(text string) bool {
	return utf8.ValidString(text) && utf8.RuneCountInString(text) <= 10000
}
