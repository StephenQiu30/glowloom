package domain

import (
	"bytes"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// SubjectConfig pins a unified subject version; absent identities represent an empty node.
// Version is the canvas configuration version, not the subject head's CAS revision.
type SubjectConfig struct {
	Version   int           `json:"version"`
	SubjectID *uuid.UUID    `json:"subject_id,omitempty"`
	VersionID *uuid.UUID    `json:"version_id,omitempty"`
	Local     *SubjectLocal `json:"local,omitempty"`
}

// UnmarshalJSON keeps absent identities distinct from malformed explicit nulls.
func (c *SubjectConfig) UnmarshalJSON(body []byte) error {
	type wire SubjectConfig
	var value wire
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return err
	}
	for _, key := range []string{"subject_id", "version_id"} {
		if raw, ok := fields[key]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return ErrInvalidCommand
		}
	}
	*c = SubjectConfig(value)
	return nil
}

// SubjectMedia retains either an ordinary project asset or a binding from the pinned subject.
type SubjectMedia struct {
	AssetID       uuid.UUID  `json:"asset_id"`
	AssetRevision int64      `json:"asset_revision"`
	Kind          string     `json:"kind"`
	BindingID     *uuid.UUID `json:"binding_id,omitempty"`
	SourceNodeID  *uuid.UUID `json:"source_node_id,omitempty"`
}

// SubjectVoice is the canvas-local audio sample and its display name.
type SubjectVoice struct {
	Name  string       `json:"name"`
	Media SubjectMedia `json:"media"`
}

// SubjectLocal is a complete canvas-owned edit, never a partial library mutation.
type SubjectLocal struct {
	Description string         `json:"description"`
	Primary     *SubjectMedia  `json:"primary,omitempty"`
	References  []SubjectMedia `json:"references"`
	Voice       *SubjectVoice  `json:"voice,omitempty"`
}

// Media returns the ordered physical inputs without creating generation authority.
func (c SubjectLocal) Media() []SubjectMedia {
	result := make([]SubjectMedia, 0, len(c.References)+2)
	if c.Primary != nil {
		result = append(result, *c.Primary)
	}
	result = append(result, c.References...)
	if c.Voice != nil {
		result = append(result, c.Voice.Media)
	}
	return result
}

func validSubject(c SubjectConfig) bool {
	if c.Version != 1 || (c.SubjectID == nil) != (c.VersionID == nil) || c.SubjectID != nil && (*c.SubjectID == uuid.Nil || *c.VersionID == uuid.Nil) {
		return false
	}
	local := c.Local
	if local == nil {
		return true
	}
	if !utf8.ValidString(local.Description) || utf8.RuneCountInString(local.Description) > 8192 || strings.ContainsRune(local.Description, 0) || local.References == nil || len(local.References) > 120 {
		return false
	}
	if local.Primary == nil && (len(local.References) > 0 || local.Voice != nil) {
		return false
	}
	if local.Primary != nil && local.Primary.Kind != "image" && local.Primary.Kind != "video" {
		return false
	}
	for _, media := range local.References {
		if media.Kind != "image" {
			return false
		}
	}
	if local.Voice != nil && (strings.TrimSpace(local.Voice.Name) == "" || !utf8.ValidString(local.Voice.Name) || utf8.RuneCountInString(local.Voice.Name) > 512 || strings.ContainsRune(local.Voice.Name, 0) || local.Voice.Media.Kind != "audio") {
		return false
	}
	seen := map[uuid.UUID]bool{}
	bound := false
	for _, media := range local.Media() {
		if media.AssetID == uuid.Nil || seen[media.AssetID] || media.AssetRevision < 1 || media.AssetRevision > math.MaxInt32 || media.BindingID != nil && (*media.BindingID == uuid.Nil || c.SubjectID == nil) || media.SourceNodeID != nil && (*media.SourceNodeID == uuid.Nil || media.BindingID != nil) {
			return false
		}
		seen[media.AssetID] = true
		bound = bound || media.SourceNodeID != nil
	}
	if bound && (len(local.References) > 3 || local.Primary != nil && local.Primary.Kind == "video" && (len(local.References) > 0 || local.Voice != nil)) {
		return false
	}
	return true
}

// ValidateSubjectSourceDuration checks connected input facts from the media owner.
// Unbound historical/manual subject media retain their existing validation policy.
func ValidateSubjectSourceDuration(kind string, durationMS *int32) error {
	switch kind {
	case "image":
		return nil
	case "audio":
		if durationMS != nil && *durationMS >= 2000 && *durationMS <= 5000 {
			return nil
		}
	case "video":
		if durationMS != nil && *durationMS > 0 && math.Round(float64(*durationMS)/1000) <= 15 {
			return nil
		}
	}
	return ErrInvalidCommand
}

// validSubjectRelations keeps a node binding and its accepted asset pin in agreement.
// Replacing a source requires an explicit rebind or detach, never silently stale media.
func validSubjectRelations(doc Document) bool {
	nodes := make(map[uuid.UUID]Node, len(doc.Nodes))
	for _, node := range doc.Nodes {
		nodes[node.ID] = node
	}
	for _, target := range doc.Nodes {
		if target.Config.Subject == nil || target.Config.Subject.Local == nil {
			continue
		}
		seen := make(map[uuid.UUID]bool)
		for _, media := range target.Config.Subject.Local.Media() {
			if media.SourceNodeID == nil {
				continue
			}
			source, ok := nodes[*media.SourceNodeID]
			if !ok || source.ID == target.ID || seen[source.ID] || !subjectMediaSourceMatches(source, media) {
				return false
			}
			seen[source.ID] = true
			count := 0
			for _, edge := range doc.Edges {
				if edge.SourceNodeID == source.ID && edge.TargetNodeID == target.ID && nativeSubjectRelation(edge) {
					count++
				}
			}
			if count != 1 {
				return false
			}
		}
	}
	return true
}

func subjectMediaSourceMatches(source Node, media SubjectMedia) bool {
	if source.NodeType != media.Kind {
		return false
	}
	if source.Config.Editor != nil {
		count := 0
		for _, ref := range source.Config.Editor.Files {
			if ref.Purpose != "CONTENT" {
				continue
			}
			count++
			if ref.FileID != media.AssetID || ref.Version != strconv.FormatInt(media.AssetRevision, 10) {
				return false
			}
		}
		return count == 1 && (source.RefID == nil || *source.RefID == media.AssetID)
	}
	return source.RefType == "media_asset" && source.RefID != nil && *source.RefID == media.AssetID
}

func nativeSubjectRelation(edge Edge) bool {
	return edge.EdgeType == "annotation" && edge.Role == "" && len(bytes.TrimSpace(edge.Binding)) == 0
}

func subjectHasSource(node Node, source uuid.UUID) bool {
	if node.Config.Subject == nil || node.Config.Subject.Local == nil {
		return false
	}
	for _, media := range node.Config.Subject.Local.Media() {
		if media.SourceNodeID != nil && *media.SourceNodeID == source {
			return true
		}
	}
	return false
}

// validateSubjectRelationChanges preserves historical visual annotations while
// requiring new domain relations and explicit detaches to remain one edit.
func validateSubjectRelationChanges(before, after Document) error {
	for _, edge := range after.Edges {
		if !nativeSubjectRelation(edge) {
			continue
		}
		target := nodeIndex(&after, edge.TargetNodeID)
		if target < 0 || after.Nodes[target].NodeType != "subject" {
			continue
		}
		existed := slices.ContainsFunc(before.Edges, func(previous Edge) bool {
			return previous.ID == edge.ID && previous.SourceNodeID == edge.SourceNodeID && previous.TargetNodeID == edge.TargetNodeID && nativeSubjectRelation(previous)
		})
		previousTarget := nodeIndex(&before, edge.TargetNodeID)
		previouslyBound := previousTarget >= 0 && subjectHasSource(before.Nodes[previousTarget], edge.SourceNodeID)
		if (!existed || previouslyBound) && !subjectHasSource(after.Nodes[target], edge.SourceNodeID) {
			return ErrInvalidCommand
		}
	}
	return nil
}

func clearSubjectReferences(nodes []Node, removed map[uuid.UUID]bool) {
	for i, node := range nodes {
		if node.Config.Subject == nil || node.Config.Subject.Local == nil {
			continue
		}
		config := *node.Config.Subject
		local := *config.Local
		local.References = slices.Clone(local.References)
		detach := func(media *SubjectMedia) {
			if media.SourceNodeID != nil && removed[*media.SourceNodeID] {
				media.SourceNodeID = nil
			}
		}
		if local.Primary != nil {
			primary := *local.Primary
			detach(&primary)
			local.Primary = &primary
		}
		for j := range local.References {
			detach(&local.References[j])
		}
		if local.Voice != nil {
			voice := *local.Voice
			detach(&voice.Media)
			local.Voice = &voice
		}
		config.Local = &local
		nodes[i].Config.Subject = &config
	}
}

func sameSubjectInput(a, b Node) bool {
	left, _ := json.Marshal(a.Config.Subject)
	right, _ := json.Marshal(b.Config.Subject)
	return a.Title == b.Title && bytes.Equal(left, right)
}

func advanceSubjectInput(document Document, node *Node, operations []Operation) error {
	old := nodeIndex(&document, node.ID)
	if old >= 0 && sameSubjectInput(document.Nodes[old], *node) {
		node.InputRevision = document.Nodes[old].InputRevision
		return nil
	}
	previous := int64(0)
	if old >= 0 {
		previous = document.Nodes[old].InputRevision
	}
	for _, op := range operations {
		if op.Type == "restore_node" && op.Node != nil && op.Node.ID == node.ID {
			previous = max(previous, op.Node.InputRevision)
		}
	}
	if previous >= math.MaxInt32 {
		return operationError("input_revision_limit")
	}
	node.InputRevision = previous + 1
	return nil
}
