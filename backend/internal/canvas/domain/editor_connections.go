package domain

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
)

// EditorConnection stores port and reference semantics for editable canvas links.
type EditorConnection struct {
	SchemaVersion int           `json:"schemaVersion"`
	FromPortID    string        `json:"fromPortId"`
	ToPortID      string        `json:"toPortId"`
	Purpose       string        `json:"purpose"`
	Order         int           `json:"order"`
	Binding       EditorBinding `json:"binding"`
}

// EditorBinding selects node content, a pinned asset or a future output.
type EditorBinding struct {
	Kind         string                 `json:"kind"`
	ReferenceKey string                 `json:"referenceKey,omitempty"`
	Selection    *EditorOutputSelection `json:"selection,omitempty"`
}

// EditorOutputSelection identifies an output item or collection.
type EditorOutputSelection struct {
	Mode  string `json:"mode"`
	Index int    `json:"index,omitempty"`
	Field string `json:"field,omitempty"`
}

func parseEditorConnection(body []byte) (EditorConnection, bool) {
	var c EditorConnection
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if len(body) > 4096 || decoder.Decode(&c) != nil || c.SchemaVersion != 1 || c.Order < 0 || c.Order > 999 {
		return c, false
	}
	purposes := map[string]string{"prompt": "PROMPT", "settings": "SETTINGS", "reference_images": "REFERENCE_IMAGE", "reference_videos": "REFERENCE_VIDEO", "reference_audio": "REFERENCE_AUDIO", "first_frame": "FIRST_FRAME", "last_frame": "LAST_FRAME", "camera": "CINEMA_CAMERA", "palette": "CINEMA_PALETTE", "style": "CINEMA_STYLE", "motion": "CINEMA_MOTION", "reference_subjects": "REFERENCE_SUBJECT"}
	if purposes[c.ToPortID] != c.Purpose || c.Purpose == "" || !slices.Contains([]string{"text", "image", "video", "audio", "settings", "last_frame", "camera", "palette", "style", "motion", "subject"}, c.FromPortID) {
		return c, false
	}
	switch c.Binding.Kind {
	case "NODE_VALUE":
		if c.Binding.ReferenceKey != "" || c.Binding.Selection != nil {
			return c, false
		}
	case "FILE_REFERENCE":
		if c.Binding.ReferenceKey == "" || len(c.Binding.ReferenceKey) > 256 || c.Binding.Selection != nil {
			return c, false
		}
	case "RUN_OUTPUT":
		if c.Binding.ReferenceKey != "" || c.Binding.Selection == nil {
			return c, false
		}
		s := c.Binding.Selection
		if (s.Mode != "ALL" && s.Mode != "ITEM") || s.Mode == "ITEM" && (s.Index < 1 || s.Index > 1000) || s.Mode == "ALL" && s.Index != 0 || len(s.Field) > 256 {
			return c, false
		}
	default:
		return c, false
	}
	return c, true
}
func validEditorConnection(body []byte) bool { _, ok := parseEditorConnection(body); return ok }
func validateEditorConnections(doc Document) error {
	nodes := map[uuid.UUID]Node{}
	adj := map[uuid.UUID][]uuid.UUID{}
	for _, n := range doc.Nodes {
		nodes[n.ID] = n
		inputSources(n, func(source uuid.UUID) { adj[source] = append(adj[source], n.ID) })
	}
	seen := map[string]bool{}
	single := map[string]bool{}
	for _, e := range doc.Edges {
		if len(e.Binding) == 0 {
			continue
		}
		c, ok := parseEditorConnection(e.Binding)
		if !ok {
			return ErrInvalidCommand
		}
		source, target := nodes[e.SourceNodeID], nodes[e.TargetNodeID]
		s, t := editorNodeType(source), editorNodeType(target)
		if s == "cinema_studio" {
			kit := source.Config.CinemaKit
			if kit == nil || len(kit.Kinds()) != 1 || c.FromPortID != kit.Kinds()[0] || c.ToPortID != c.FromPortID || c.Binding.Kind != "NODE_VALUE" || !kit.Supports(t+".generate") {
				return ErrInvalidCommand
			}
			key := e.TargetNodeID.String() + "/" + c.ToPortID
			if single[key] {
				return ErrInvalidCommand
			}
			single[key] = true
			if _, err := EffectiveCinemaKit(doc, target); err != nil {
				return err
			}
			adj[e.SourceNodeID] = append(adj[e.SourceNodeID], e.TargetNodeID)
			continue
		}

		if s == "subject" {
			if c.FromPortID != "subject" || c.ToPortID != "reference_subjects" || c.Binding.Kind != "NODE_VALUE" || !slices.Contains([]string{"image", "video", "audio"}, t) {
				return ErrInvalidCommand
			}
			key := e.SourceNodeID.String() + "/" + e.TargetNodeID.String() + "/subject"
			if seen[key] {
				return ErrInvalidCommand
			}
			seen[key] = true
			adj[e.SourceNodeID] = append(adj[e.SourceNodeID], e.TargetNodeID)
			continue
		}

		want := map[string]string{"prompt": "text", "settings": "config", "reference_images": "image", "reference_videos": "video", "reference_audio": "audio", "first_frame": "image", "last_frame": "image"}[c.ToPortID]
		sourceKind := s
		if s == "video" && c.FromPortID == "last_frame" {
			sourceKind = "image"
		}
		if sourceKind != want || !slices.Contains([]string{"text", "image", "video", "audio"}, t) || (c.ToPortID == "first_frame" || c.ToPortID == "last_frame") && t != "video" {
			return ErrInvalidCommand
		}
		from := s
		if s == "config" {
			from = "settings"
		}
		if c.FromPortID != from && (s != "video" || c.FromPortID != "last_frame") {
			return ErrInvalidCommand
		}
		if c.Binding.Kind == "NODE_VALUE" && s != "text" && s != "config" {
			return ErrInvalidCommand
		}
		if c.Binding.Kind == "FILE_REFERENCE" {
			found := false
			if source.Config.Editor != nil {
				for _, ref := range source.Config.Editor.Files {
					purpose := "CONTENT"
					if c.FromPortID == "last_frame" {
						purpose = "LAST_FRAME"
					}
					found = found || ref.ReferenceKey == c.Binding.ReferenceKey && ref.Purpose == purpose
				}
			}
			if source.RefID != nil && source.Config.Editor == nil && c.FromPortID != "last_frame" {
				found = source.RefID.String() == c.Binding.ReferenceKey
			}
			if !found {
				return ErrInvalidCommand
			}
		}
		key := e.SourceNodeID.String() + "/" + e.TargetNodeID.String() + "/" + c.FromPortID + "/" + c.ToPortID + "/" + string(effectJSON(c.Binding))
		if seen[key] {
			return ErrInvalidCommand
		}
		seen[key] = true
		if c.ToPortID == "settings" || c.ToPortID == "first_frame" || c.ToPortID == "last_frame" {
			key = e.TargetNodeID.String() + "/" + c.ToPortID
			if single[key] {
				return ErrInvalidCommand
			}
			single[key] = true
		}
		adj[e.SourceNodeID] = append(adj[e.SourceNodeID], e.TargetNodeID)
	}
	state := map[uuid.UUID]int{}
	var visit func(uuid.UUID) bool
	visit = func(id uuid.UUID) bool {
		if state[id] == 1 {
			return false
		}
		if state[id] == 2 {
			return true
		}
		state[id] = 1
		for _, next := range adj[id] {
			if !visit(next) {
				return false
			}
		}
		state[id] = 2
		return true
	}
	for id := range nodes {
		if !visit(id) {
			return ErrInvalidCommand
		}
	}
	return nil
}
