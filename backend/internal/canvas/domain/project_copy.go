package domain

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var (
	// ErrInvalidProjectCopy rejects incomplete mappings or malformed frozen content.
	ErrInvalidProjectCopy = errors.New("invalid project canvas copy")
	// ErrUnsupportedProjectCopy preserves unsupported content instead of dropping it.
	ErrUnsupportedProjectCopy = errors.New("unsupported project copy content")
)

// ProjectDocumentCopy is content plus the exact count of removed execution bindings.
type ProjectDocumentCopy struct {
	Document                 Document `json:"document"`
	ClearedOperationBindings int      `json:"cleared_operation_bindings"`
}

// CopyProjectDocument rebuilds a private graph with stable job-scoped identities.
// Media mappings must come from verified owning-module copy receipts.
func CopyProjectDocument(source Document, targetProject, job uuid.UUID, assets map[uuid.UUID]uuid.UUID) (ProjectDocumentCopy, error) {
	if source.ID == uuid.Nil || source.ProjectID == uuid.Nil || targetProject == uuid.Nil || targetProject == source.ProjectID || job == uuid.Nil || source.Revision < 1 || !validTitle(source.Name) {
		return ProjectDocumentCopy{}, ErrInvalidProjectCopy
	}
	var scope map[string]json.RawMessage
	if json.Unmarshal(source.Scope, &scope) != nil || scope == nil || len(scope) != 0 {
		return ProjectDocumentCopy{}, ErrUnsupportedProjectCopy
	}
	raw, err := json.Marshal(source)
	if err != nil {
		return ProjectDocumentCopy{}, fmt.Errorf("%w: encode frozen document", ErrInvalidProjectCopy)
	}
	var result ProjectDocumentCopy
	if err := json.Unmarshal(raw, &result.Document); err != nil {
		return ProjectDocumentCopy{}, fmt.Errorf("%w: decode frozen document", ErrInvalidProjectCopy)
	}
	doc := &result.Document
	for i := range doc.Nodes {
		if doc.Nodes[i].LastOperationID != nil {
			result.ClearedOperationBindings++
			doc.Nodes[i].LastOperationID = nil
		}
		if !editable(doc.Nodes[i]) {
			return ProjectDocumentCopy{}, ErrUnsupportedProjectCopy
		}
	}
	for _, edge := range doc.Edges {
		if !annotation(edge) {
			return ProjectDocumentCopy{}, ErrUnsupportedProjectCopy
		}
	}
	if err := validateCopiedGraph(*doc); err != nil {
		return ProjectDocumentCopy{}, err
	}
	identity := func(scope string, original uuid.UUID) uuid.UUID {
		return uuid.NewSHA1(job, []byte(source.ID.String()+"/"+scope+"/"+original.String()))
	}
	missingAsset := false
	usedTargets := make(map[uuid.UUID]uuid.UUID)
	assetID := func(original uuid.UUID) uuid.UUID {
		id, ok := assets[original]
		previous, reused := usedTargets[id]
		_, sourceIdentity := assets[id]
		if !ok || id == uuid.Nil || id == original || sourceIdentity || reused && previous != original {
			missingAsset = true
		}
		usedTargets[id] = original
		return id
	}
	for i := range doc.Nodes {
		node := &doc.Nodes[i]
		nodeScope := "node/" + node.ID.String()
		node.ID = identity("node", node.ID)
		remapCopyPointer(node.ParentID, func(id uuid.UUID) uuid.UUID { return identity("node", id) })
		if node.RefID != nil {
			remapCopyPointer(node.RefID, assetID)
		}
		if c := node.Config.Generation; c != nil {
			for j := range c.Inputs {
				c.Inputs[j].MediaAssetID = assetID(c.Inputs[j].MediaAssetID)
			}
		}
		if c := node.Config.BatchTable; c != nil {
			for j := range c.ReferenceColumns {
				c.ReferenceColumns[j].ID = identity(nodeScope+"/batch-column", c.ReferenceColumns[j].ID)
			}
			for j := range c.Rows {
				c.Rows[j].ID = identity(nodeScope+"/batch-row", c.Rows[j].ID)
				for _, id := range c.Rows[j].InputNodeIDs {
					remapCopyPointer(id, func(id uuid.UUID) uuid.UUID { return identity("node", id) })
				}
			}
		}
		if c := node.Config.Timeline; c != nil {
			for j := range c.Tracks {
				c.Tracks[j].ID = identity(nodeScope+"/track", c.Tracks[j].ID)
			}
			for j := range c.Clips {
				clip := &c.Clips[j]
				clip.ID = identity(nodeScope+"/clip", clip.ID)
				clip.TrackID = identity(nodeScope+"/track", clip.TrackID)
				remapCopyPointer(clip.NodeID, func(id uuid.UUID) uuid.UUID { return identity("node", id) })
				remapCopyPointer(clip.AssetID, assetID)
			}
		}
		if c := node.Config.Director; c != nil {
			copyDirectorIdentities(c, nodeScope, identity, assetID)
		}
	}
	if missingAsset {
		return ProjectDocumentCopy{}, fmt.Errorf("%w: missing independent media mapping", ErrInvalidProjectCopy)
	}
	for i := range doc.Edges {
		edge := &doc.Edges[i]
		edge.ID = identity("edge", edge.ID)
		edge.SourceNodeID = identity("node", edge.SourceNodeID)
		edge.TargetNodeID = identity("node", edge.TargetNodeID)
	}
	doc.ID, doc.ProjectID, doc.Revision = identity("document", source.ID), targetProject, 1
	if err := validateCopiedGraph(*doc); err != nil {
		return ProjectDocumentCopy{}, err
	}
	return result, nil
}

func remapCopyPointer(value *uuid.UUID, remap func(uuid.UUID) uuid.UUID) {
	if value != nil {
		*value = remap(*value)
	}
}

func validateCopiedGraph(doc Document) error {
	commands := []Command{{Type: "SetViewport", Viewport: &doc.Viewport}}
	if len(doc.Nodes) > 0 {
		commands = append(commands, Command{Type: "AddNodes", Nodes: doc.Nodes})
	}
	if len(doc.Edges) > 0 {
		commands = append(commands, Command{Type: "Connect", Edges: doc.Edges})
	}
	if _, err := Apply(Document{}, commands); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidProjectCopy, err)
	}
	return nil
}

func copyDirectorIdentities(c *DirectorConfig, scope string, identity func(string, uuid.UUID) uuid.UUID, assetID func(uuid.UUID) uuid.UUID) {
	id := func(kind string, old uuid.UUID) uuid.UUID { return identity(scope+"/director/"+kind, old) }
	nodeID := func(old uuid.UUID) uuid.UUID { return identity("node", old) }
	c.ID = id("scene", c.ID)
	c.ActiveShotID = id("shot", c.ActiveShotID)
	if c.Cover != nil {
		c.Cover.AssetID = assetID(c.Cover.AssetID)
		c.Cover.ShotID = id("shot", c.Cover.ShotID)
	}
	if c.Panorama != nil {
		c.Panorama.AssetID = assetID(c.Panorama.AssetID)
	}
	for i := range c.Objects {
		object := &c.Objects[i]
		objectScope := "object/" + object.ID.String()
		object.ID = id("object", object.ID)
		remapCopyPointer(object.SourceNodeID, nodeID)
		remapCopyPointer(object.AssetID, assetID)
		remapCopyPointer(object.ActiveMotionClipID, func(old uuid.UUID) uuid.UUID { return id(objectScope+"/motion", old) })
		for j := range object.MotionClips {
			object.MotionClips[j].ID = id(objectScope+"/motion", object.MotionClips[j].ID)
		}
		for j := range object.Keyframes {
			object.Keyframes[j].ID = id(objectScope+"/frame", object.Keyframes[j].ID)
		}
		for j := range object.BoneTracks {
			track := &object.BoneTracks[j]
			for k := range track.Keyframes {
				track.Keyframes[k].ID = id(objectScope+"/bone/"+track.Bone, track.Keyframes[k].ID)
			}
		}
	}
	for i := range c.Cameras {
		camera := &c.Cameras[i]
		cameraScope := "camera/" + camera.ID.String()
		camera.ID = id("camera", camera.ID)
		remapCopyPointer(camera.FollowObjectID, func(old uuid.UUID) uuid.UUID { return id("object", old) })
		remapCopyPointer(camera.LookAtObjectID, func(old uuid.UUID) uuid.UUID { return id("object", old) })
		for j := range camera.Keyframes {
			camera.Keyframes[j].ID = id(cameraScope+"/frame", camera.Keyframes[j].ID)
		}
	}
	for i := range c.Lights {
		c.Lights[i].ID = id("light", c.Lights[i].ID)
	}
	for i := range c.Shots {
		shot := &c.Shots[i]
		shot.ID = id("shot", shot.ID)
		shot.CameraID = id("camera", shot.CameraID)
		for j := range shot.Screenshots {
			shot.Screenshots[j].ID = id("screenshot", shot.Screenshots[j].ID)
			shot.Screenshots[j].AssetID = assetID(shot.Screenshots[j].AssetID)
		}
	}
}
