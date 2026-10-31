package domain

import (
	"encoding/json"
	"slices"

	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// GenerationInput freezes graph-derived visual settings without rewriting the user's composer.
func GenerationInput(doc Document, node Node) (Draft, error) {
	if node.Config.Draft == nil {
		return Draft{}, ErrInvalidCommand
	}
	input, err := withSubjectConnections(doc, node, *node.Config.Draft)
	if err != nil {
		return Draft{}, err
	}
	kit, err := EffectiveCinemaKit(doc, node)
	if err != nil {
		return Draft{}, err
	}
	if kit == nil {
		return input, nil
	}
	if input.Composer == nil {
		return Draft{}, ErrInvalidCommand
	}
	composer := *input.Composer
	composer.CinemaKit = kit
	input.Composer = &composer
	return input, nil
}

// EffectiveCinemaKit resolves actual graph relationships under the document owner's lock.
func EffectiveCinemaKit(doc Document, node Node) (*prompt.CinemaKit, error) {
	kit := prompt.CinemaKit{Version: 1}
	if node.Config.CinemaKit != nil {
		body, _ := json.Marshal(node.Config.CinemaKit)
		if err := json.Unmarshal(body, &kit); err != nil {
			return nil, err
		}
	}
	for _, edge := range doc.Edges {
		if edge.TargetNodeID != node.ID {
			continue
		}
		c, ok := parseEditorConnection(edge.Binding)
		if !ok || !slices.Contains([]string{"camera", "palette", "style", "motion"}, c.ToPortID) {
			continue
		}
		i := nodeIndex(&doc, edge.SourceNodeID)
		if i < 0 || doc.Nodes[i].NodeType != "cinema_studio" || doc.Nodes[i].Config.CinemaKit == nil {
			return nil, ErrInvalidCommand
		}
		source := doc.Nodes[i].Config.CinemaKit
		if source.Validate() != nil || len(source.Kinds()) != 1 || source.Kinds()[0] != c.ToPortID || slices.Contains(kit.Kinds(), c.ToPortID) {
			return nil, ErrInvalidCommand
		}
		switch c.ToPortID {
		case "camera":
			camera := *source.Camera
			kit.Camera = &camera
		case "palette":
			palette := *source.Palette
			palette.Colors = slices.Clone(palette.Colors)
			kit.Palette = &palette
		case "style":
			kit.Style = source.Style
		case "motion":
			kit.Motion = source.Motion
		}
		kit.Sources = append(kit.Sources, prompt.CinemaSource{NodeID: edge.SourceNodeID, Kind: c.ToPortID, Hash: source.Fingerprint()})
	}
	if len(kit.Kinds()) == 0 {
		return nil, nil
	}
	if kit.Validate() != nil || !kit.Supports(editorNodeType(node)+".generate") {
		return nil, ErrInvalidCommand
	}
	slices.SortFunc(kit.Sources, func(a, b prompt.CinemaSource) int {
		if a.Kind < b.Kind {
			return -1
		}
		if a.Kind > b.Kind {
			return 1
		}
		return 0
	})
	return &kit, nil
}
