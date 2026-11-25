package domain

import "github.com/google/uuid"

// Native timeline annotations visualize a clip's live canvas dependency.
// Imported clips without annotations remain valid legacy input.
func validTimelineRelations(doc Document) bool {
	for _, edge := range doc.Edges {
		index := nodeIndex(&doc, edge.TargetNodeID)
		if index < 0 || doc.Nodes[index].Config.Timeline == nil || len(edge.Binding) != 0 {
			continue
		}
		found := false
		for _, clip := range doc.Nodes[index].Config.Timeline.Clips {
			if clip.NodeID != nil && *clip.NodeID == edge.SourceNodeID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Disconnecting a visual relation must also detach the input in the same candidate.
func validateTimelineRelationChanges(before, after Document) error {
	added := make(map[uuid.UUID]int)
	for _, edge := range after.Edges {
		index := nodeIndex(&after, edge.TargetNodeID)
		if index < 0 || after.Nodes[index].Config.Timeline == nil || len(edge.Binding) != 0 {
			continue
		}
		exists := false
		for _, previous := range before.Edges {
			if previous.SourceNodeID == edge.SourceNodeID && previous.TargetNodeID == edge.TargetNodeID && len(previous.Binding) == 0 {
				exists = true
				break
			}
		}
		if !exists {
			added[edge.TargetNodeID]++
			if added[edge.TargetNodeID] > 50 {
				return ErrInvalidCommand
			}
		}
	}
	for _, edge := range before.Edges {
		old := nodeIndex(&before, edge.TargetNodeID)
		next := nodeIndex(&after, edge.TargetNodeID)
		if old < 0 || next < 0 || before.Nodes[old].Config.Timeline == nil || after.Nodes[next].Config.Timeline == nil || len(edge.Binding) != 0 {
			continue
		}
		retained := false
		for _, candidate := range after.Edges {
			if candidate.SourceNodeID == edge.SourceNodeID && candidate.TargetNodeID == edge.TargetNodeID && len(candidate.Binding) == 0 {
				retained = true
				break
			}
		}
		if retained {
			continue
		}
		for _, clip := range after.Nodes[next].Config.Timeline.Clips {
			if clip.NodeID != nil && *clip.NodeID == edge.SourceNodeID {
				return ErrInvalidCommand
			}
		}
	}
	return nil
}

// ValidateTimelineSourceDuration checks a linked clip against authorized media metadata.
func ValidateTimelineSourceDuration(clip TimelineClip, durationMS *int32) error {
	if clip.Kind != "video" && clip.Kind != "audio" {
		return nil
	}
	if durationMS == nil || *durationMS < 100 || clip.SourceDurationMS == nil || *clip.SourceDurationMS != int64(*durationMS) || clip.SourceStartMS < 0 || clip.DurationMS < 100 || clip.SourceStartMS+clip.DurationMS > int64(*durationMS) {
		return ErrInvalidCommand
	}
	return nil
}
