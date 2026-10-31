package domain

import (
	"bytes"
	"maps"
	"slices"
	"strconv"

	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// SubjectConnectionReferences derives fixed identities from authoritative graph edges.
// Library pins are expanded by their source owner under the quote/confirm lock.
func SubjectConnectionReferences(doc Document, node Node) ([]prompt.Reference, error) {
	type sourceEdge struct {
		edge       Edge
		connection EditorConnection
	}
	edges := []sourceEdge{}
	for _, edge := range doc.Edges {
		if edge.TargetNodeID != node.ID {
			continue
		}
		c, ok := parseEditorConnection(edge.Binding)
		if !ok || c.ToPortID != "reference_subjects" {
			continue
		}
		edges = append(edges, sourceEdge{edge, c})
	}
	slices.SortFunc(edges, func(a, b sourceEdge) int {
		if a.connection.Order != b.connection.Order {
			return a.connection.Order - b.connection.Order
		}
		return bytes.Compare(a.edge.ID[:], b.edge.ID[:])
	})
	refs := []prompt.Reference{}
	for _, item := range edges {
		index := nodeIndex(&doc, item.edge.SourceNodeID)
		if index < 0 || doc.Nodes[index].NodeType != "subject" || doc.Nodes[index].Config.Subject == nil {
			return nil, ErrInvalidCommand
		}
		source := doc.Nodes[index]
		config := source.Config.Subject
		if config.Local == nil && config.SubjectID == nil || config.Local != nil && config.Local.Primary == nil {
			continue
		}
		if source.InputRevision < 1 {
			return nil, ErrInvalidCommand
		}
		ref := prompt.Reference{ID: "canvas-edge:" + item.edge.ID.String(), BindingGeneration: source.InputRevision, Role: "subject", MediaKind: "image"}
		if config.Local == nil {
			ref.Source = prompt.ReferenceSource{Kind: "subject", SourceID: config.SubjectID.String(), SourceVersion: config.VersionID.String()}
		} else {
			local := config.Local
			ref.MediaKind = local.Primary.Kind
			ref.Source = prompt.ReferenceSource{Kind: "canvas_subject", SourceID: source.ID.String(), SourceVersion: strconv.FormatInt(source.InputRevision, 10), Description: source.Title, Media: []prompt.SourceMedia{}}
			if local.Description != "" {
				ref.Source.Description += "：" + local.Description
			}
			for _, media := range local.Media() {
				m := prompt.SourceMedia{SourceID: media.AssetID.String(), SourceVersion: strconv.FormatInt(media.AssetRevision, 10), MediaKind: media.Kind}
				if media.BindingID != nil {
					m.BindingID = media.BindingID.String()
				}
				ref.Source.Media = append(ref.Source.Media, m)
			}
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func withSubjectConnections(doc Document, node Node, input Draft) (Draft, error) {
	refs, err := SubjectConnectionReferences(doc, node)
	if err != nil || len(refs) == 0 {
		return input, err
	}
	if input.Composer == nil {
		return Draft{}, ErrInvalidCommand
	}
	composer := *input.Composer
	composer.References = slices.Clone(composer.References)
	composer.LabelState.HighWater = maps.Clone(composer.LabelState.HighWater)
	composer.LabelState.Allocations = maps.Clone(composer.LabelState.Allocations)
	for _, ref := range refs {
		// Derived identities cannot be supplied or overwritten by an editable composer.
		if _, exists := composer.LabelState.Allocations[ref.ID]; exists {
			return Draft{}, ErrInvalidCommand
		}
		composer.LabelState.HighWater["subject"]++
		composer.LabelState.Allocations[ref.ID] = prompt.LabelAllocation{LabelKind: "subject", Ordinal: composer.LabelState.HighWater["subject"]}
		ref.Order = len(composer.References)
		composer.References = append(composer.References, ref)
	}
	if composer.Validate() != nil {
		return Draft{}, ErrInvalidCommand
	}
	input.Composer = &composer
	return input, nil
}
