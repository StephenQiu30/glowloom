package scripts

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

func PrepareNarrativeDraft(current Analysis, revisionID uuid.UUID, revisionNo int) Analysis {
	prepared, err := cloneAnalysis(current)
	if err != nil {
		return Analysis{}
	}
	if revisionNo < 1 {
		revisionNo = 1
	}
	prepared.Narrative = NarrativeRevision{ID: revisionID, RevisionNo: revisionNo}
	prepared.Mentions = mentionsInPublishedEpisodes(prepared)
	refreshNarrative(&prepared)
	return prepared
}

func ReviseNarrative(current Analysis, expectedContentHash string, revisionID uuid.UUID, operations []NarrativeOperation) (Analysis, error) {
	if current.Narrative.ID == uuid.Nil || current.Narrative.RevisionNo < 1 {
		return Analysis{}, errors.New("narrative draft does not exist")
	}
	if expectedContentHash == "" || expectedContentHash != current.Narrative.ContentHash {
		return Analysis{}, errors.New("narrative revision basis is stale")
	}
	if revisionID == uuid.Nil {
		return Analysis{}, errors.New("narrative revision id is required")
	}
	if len(operations) < 1 || len(operations) > 100 {
		return Analysis{}, errors.New("narrative revision requires 1 to 100 operations")
	}
	revised, err := cloneAnalysis(current)
	if err != nil {
		return Analysis{}, err
	}
	for _, operation := range operations {
		if err := applyNarrativeOperation(&revised, operation); err != nil {
			return Analysis{}, err
		}
	}
	revised.Narrative = NarrativeRevision{ID: revisionID, RevisionNo: current.Narrative.RevisionNo + 1}
	refreshNarrative(&revised)
	return revised, nil
}

func applyNarrativeOperation(analysis *Analysis, operation NarrativeOperation) error {
	switch operation.Type {
	case NarrativeOperationUpdateScene:
		_, _, scene := findScene(analysis, operation.SceneID)
		if scene == nil {
			return errors.New("scene does not exist")
		}
		heading := strings.TrimSpace(operation.Heading)
		if len([]rune(heading)) < 1 || len([]rune(heading)) > 200 {
			return errors.New("scene heading must contain 1 to 200 characters")
		}
		scene.Heading = heading
		return nil
	case NarrativeOperationSplitScene:
		return splitScene(analysis, operation)
	case NarrativeOperationMergeScenes:
		return mergeScenes(analysis, operation)
	case NarrativeOperationReorderScenes:
		return reorderScenes(analysis, operation)
	case NarrativeOperationCreateNode:
		return createNarrativeNode(analysis, operation)
	case NarrativeOperationUpdateNode:
		return updateNarrativeNode(analysis, operation)
	case NarrativeOperationDeleteNode:
		return deleteNarrativeNode(analysis, operation.NodeID)
	case NarrativeOperationReorderNodes:
		return reorderNarrativeNodes(analysis, operation)
	case NarrativeOperationIgnoreNode:
		_, node := findNarrativeNode(analysis, operation.NodeID)
		if node == nil {
			return errors.New("narrative node does not exist")
		}
		reason := strings.TrimSpace(operation.IgnoreReason)
		if len([]rune(reason)) < 1 || len([]rune(reason)) > 200 {
			return errors.New("ignored narrative reason must contain 1 to 200 characters")
		}
		node.Status, node.IgnoreReason = NarrativeNodeStatusIgnored, reason
		return nil
	case NarrativeOperationCreateMention:
		return createMention(analysis, operation)
	case NarrativeOperationUpdateMention:
		return updateMention(analysis, operation)
	case NarrativeOperationDeleteMention:
		return deleteMention(analysis, operation.MentionID)
	default:
		return fmt.Errorf("unsupported narrative operation %q", operation.Type)
	}
}

func splitScene(analysis *Analysis, operation NarrativeOperation) error {
	episodeIndex, sceneIndex, scene := findScene(analysis, operation.SceneID)
	if scene == nil {
		return errors.New("scene does not exist")
	}
	boundary := -1
	for index := range scene.Narratives {
		if scene.Narratives[index].ID == strings.TrimSpace(operation.BoundaryNodeID) {
			boundary = index
			break
		}
	}
	if boundary <= 0 || boundary >= len(scene.Narratives) {
		return errors.New("scene split boundary must be an existing non-first narrative node")
	}
	if err := validateNewMemberID(analysis, operation.LeftSceneID); err != nil {
		return err
	}
	if err := validateNewMemberID(analysis, operation.RightSceneID); err != nil {
		return err
	}
	if operation.LeftSceneID == operation.RightSceneID {
		return errors.New("split scene ids must be unique")
	}
	leftHeading, rightHeading := strings.TrimSpace(operation.LeftHeading), strings.TrimSpace(operation.RightHeading)
	if leftHeading == "" || rightHeading == "" {
		return errors.New("split scene headings are required")
	}
	boundaryOffset := scene.Narratives[boundary].Anchor.StartOffset
	left := *scene
	left.ID, left.Heading, left.Anchor.EndOffset = operation.LeftSceneID, leftHeading, boundaryOffset
	left.Narratives = append([]NarrativeUnit(nil), scene.Narratives[:boundary]...)
	right := *scene
	right.ID, right.Heading, right.Anchor.StartOffset = operation.RightSceneID, rightHeading, boundaryOffset
	right.Anchor.Line = scene.Narratives[boundary].Anchor.Line
	right.Narratives = append([]NarrativeUnit(nil), scene.Narratives[boundary:]...)
	episode := &analysis.Episodes[episodeIndex]
	episode.Scenes = append(episode.Scenes[:sceneIndex], append([]Scene{left, right}, episode.Scenes[sceneIndex+1:]...)...)
	for index := range analysis.Mentions {
		if analysis.Mentions[index].SceneID != operation.SceneID {
			continue
		}
		if analysis.Mentions[index].Anchor.StartOffset < boundaryOffset {
			analysis.Mentions[index].SceneID = left.ID
		} else {
			analysis.Mentions[index].SceneID = right.ID
		}
	}
	return nil
}

func mergeScenes(analysis *Analysis, operation NarrativeOperation) error {
	if len(operation.SceneIDs) < 2 {
		return errors.New("merge requires at least two adjacent scenes")
	}
	episodeIndex, start, first := findScene(analysis, operation.SceneIDs[0])
	if first == nil || start+len(operation.SceneIDs) > len(analysis.Episodes[episodeIndex].Scenes) {
		return errors.New("merge scenes do not exist")
	}
	if err := validateNewMemberID(analysis, operation.TargetSceneID); err != nil {
		return err
	}
	heading := strings.TrimSpace(operation.Heading)
	if heading == "" {
		return errors.New("merged scene heading is required")
	}
	merged := *first
	merged.ID, merged.Heading, merged.Narratives = operation.TargetSceneID, heading, nil
	for offset, id := range operation.SceneIDs {
		scene := analysis.Episodes[episodeIndex].Scenes[start+offset]
		if scene.ID != id {
			return errors.New("merge scenes must be adjacent and ordered in one episode")
		}
		merged.Narratives = append(merged.Narratives, scene.Narratives...)
		merged.Anchor.EndOffset = scene.Anchor.EndOffset
	}
	episode := &analysis.Episodes[episodeIndex]
	episode.Scenes = append(episode.Scenes[:start], append([]Scene{merged}, episode.Scenes[start+len(operation.SceneIDs):]...)...)
	mergedSet := make(map[string]bool, len(operation.SceneIDs))
	for _, id := range operation.SceneIDs {
		mergedSet[id] = true
	}
	for index := range analysis.Mentions {
		if mergedSet[analysis.Mentions[index].SceneID] {
			analysis.Mentions[index].SceneID = merged.ID
		}
	}
	return nil
}

func reorderScenes(analysis *Analysis, operation NarrativeOperation) error {
	episodeIndex := episodeIndexByKey(analysis.Episodes, operation.EpisodeKey)
	if episodeIndex < 0 {
		return errors.New("episode does not exist")
	}
	scenes := analysis.Episodes[episodeIndex].Scenes
	if len(operation.OrderedSceneIDs) != len(scenes) {
		return errors.New("scene reorder must contain the complete scene id set")
	}
	byID := make(map[string]Scene, len(scenes))
	for _, scene := range scenes {
		byID[scene.ID] = scene
	}
	ordered := make([]Scene, 0, len(scenes))
	seen := map[string]bool{}
	for _, id := range operation.OrderedSceneIDs {
		if seen[id] {
			return errors.New("scene reorder ids must be unique")
		}
		scene, ok := byID[id]
		if !ok {
			return errors.New("scene reorder contains an unknown scene")
		}
		seen[id] = true
		ordered = append(ordered, scene)
	}
	analysis.Episodes[episodeIndex].Scenes = ordered
	return nil
}

func createNarrativeNode(analysis *Analysis, operation NarrativeOperation) error {
	_, _, scene := findScene(analysis, operation.SceneID)
	if scene == nil {
		return errors.New("scene does not exist")
	}
	if _, node := findNarrativeNode(analysis, operation.NodeID); node != nil {
		return errors.New("narrative node id already exists")
	}
	if _, err := uuid.Parse(operation.NodeID); err != nil {
		return errors.New("narrative node id must be a UUID")
	}
	if err := validateNarrativeNodeInput(operation.NodeKind, operation.Text, operation.Anchor, *scene); err != nil {
		return err
	}
	scene.Narratives = append(scene.Narratives, NarrativeUnit{
		ID: operation.NodeID, Kind: operation.NodeKind, Text: strings.TrimSpace(operation.Text),
		Speaker: strings.TrimSpace(operation.Speaker), Anchor: operation.Anchor, Status: NarrativeNodeStatusActive,
	})
	sort.SliceStable(scene.Narratives, func(left, right int) bool {
		return scene.Narratives[left].Anchor.StartOffset < scene.Narratives[right].Anchor.StartOffset
	})
	return nil
}

func updateNarrativeNode(analysis *Analysis, operation NarrativeOperation) error {
	scene, node := findNarrativeNode(analysis, operation.NodeID)
	if node == nil || scene == nil {
		return errors.New("narrative node does not exist")
	}
	if err := validateNarrativeNodeInput(operation.NodeKind, operation.Text, operation.Anchor, *scene); err != nil {
		return err
	}
	node.Kind, node.Text, node.Speaker, node.Anchor = operation.NodeKind, strings.TrimSpace(operation.Text), strings.TrimSpace(operation.Speaker), operation.Anchor
	node.Status, node.IgnoreReason = NarrativeNodeStatusActive, ""
	return nil
}

func deleteNarrativeNode(analysis *Analysis, nodeID string) error {
	for episodeIndex := range analysis.Episodes {
		for sceneIndex := range analysis.Episodes[episodeIndex].Scenes {
			nodes := analysis.Episodes[episodeIndex].Scenes[sceneIndex].Narratives
			for nodeIndex := range nodes {
				if nodes[nodeIndex].ID == strings.TrimSpace(nodeID) {
					analysis.Episodes[episodeIndex].Scenes[sceneIndex].Narratives = append(nodes[:nodeIndex], nodes[nodeIndex+1:]...)
					return nil
				}
			}
		}
	}
	return errors.New("narrative node does not exist")
}

func reorderNarrativeNodes(analysis *Analysis, operation NarrativeOperation) error {
	_, _, scene := findScene(analysis, operation.SceneID)
	if scene == nil {
		return errors.New("scene does not exist")
	}
	if len(operation.OrderedNodeIDs) != len(scene.Narratives) {
		return errors.New("node reorder must contain the complete node id set")
	}
	byID := make(map[string]NarrativeUnit, len(scene.Narratives))
	for _, node := range scene.Narratives {
		byID[node.ID] = node
	}
	ordered := make([]NarrativeUnit, 0, len(scene.Narratives))
	seen := map[string]bool{}
	for _, id := range operation.OrderedNodeIDs {
		if seen[id] {
			return errors.New("node reorder ids must be unique")
		}
		node, ok := byID[id]
		if !ok {
			return errors.New("node reorder contains an unknown node")
		}
		seen[id] = true
		ordered = append(ordered, node)
	}
	scene.Narratives = ordered
	return nil
}

func createMention(analysis *Analysis, operation NarrativeOperation) error {
	if findMentionIndex(analysis.Mentions, operation.MentionID) >= 0 {
		return errors.New("mention id already exists")
	}
	mention, err := mentionFromOperation(analysis, operation)
	if err != nil {
		return err
	}
	analysis.Mentions = append(analysis.Mentions, mention)
	return nil
}

func updateMention(analysis *Analysis, operation NarrativeOperation) error {
	index := findMentionIndex(analysis.Mentions, operation.MentionID)
	if index < 0 {
		return errors.New("mention does not exist")
	}
	mention, err := mentionFromOperation(analysis, operation)
	if err != nil {
		return err
	}
	analysis.Mentions[index] = mention
	return nil
}

func deleteMention(analysis *Analysis, mentionID string) error {
	index := findMentionIndex(analysis.Mentions, mentionID)
	if index < 0 {
		return errors.New("mention does not exist")
	}
	analysis.Mentions = append(analysis.Mentions[:index], analysis.Mentions[index+1:]...)
	return nil
}

func mentionFromOperation(analysis *Analysis, operation NarrativeOperation) (ProductionElementMention, error) {
	if _, err := uuid.Parse(operation.MentionID); err != nil {
		return ProductionElementMention{}, errors.New("mention id must be a UUID")
	}
	_, _, scene := findScene(analysis, operation.SceneID)
	if scene == nil {
		return ProductionElementMention{}, errors.New("mention scene does not exist")
	}
	if !validElementType(operation.ElementType) {
		return ProductionElementMention{}, errors.New("mention element type is invalid")
	}
	surface := strings.TrimSpace(operation.SurfaceText)
	if len([]rune(surface)) < 1 || len([]rune(surface)) > 120 {
		return ProductionElementMention{}, errors.New("mention surface text must contain 1 to 120 characters")
	}
	if !anchorWithin(operation.Anchor, scene.Anchor) {
		return ProductionElementMention{}, errors.New("mention anchor must be inside its scene")
	}
	return ProductionElementMention{
		ID: operation.MentionID, SceneID: operation.SceneID, ElementType: operation.ElementType,
		SurfaceText: surface, Status: "active", Anchor: operation.Anchor,
	}, nil
}

func refreshNarrative(analysis *Analysis) {
	issues := make([]NarrativeIssue, 0)
	sceneIDs, nodeIDs, mentionIDs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for episodeIndex := range analysis.Episodes {
		episode := &analysis.Episodes[episodeIndex]
		if episode.Decision == "ignored" {
			continue
		}
		previousSceneStart := -1
		for sceneIndex := range episode.Scenes {
			scene := &episode.Scenes[sceneIndex]
			if sceneIDs[scene.ID] {
				issues = append(issues, NarrativeIssue{Code: "duplicate_scene_id", Message: "场景 ID 重复", SceneID: scene.ID})
			}
			sceneIDs[scene.ID] = true
			if !anchorWithin(scene.Anchor, episode.Anchor) {
				anchor := scene.Anchor
				issues = append(issues, NarrativeIssue{Code: "scene_crosses_episode", Message: "场景范围越过剧集边界", SceneID: scene.ID, Anchor: &anchor})
			}
			if previousSceneStart > scene.Anchor.StartOffset {
				issues = append(issues, NarrativeIssue{Code: "scene_out_of_order", Message: "场景顺序与来源锚点不一致", SceneID: scene.ID})
			}
			previousSceneStart = scene.Anchor.StartOffset
			activeNodes := 0
			previousNodeStart := -1
			for nodeIndex := range scene.Narratives {
				node := &scene.Narratives[nodeIndex]
				if node.Status == "" {
					node.Status = NarrativeNodeStatusActive
				}
				if nodeIDs[node.ID] {
					issues = append(issues, NarrativeIssue{Code: "duplicate_node_id", Message: "叙事节点 ID 重复", SceneID: scene.ID, NodeID: node.ID})
				}
				nodeIDs[node.ID] = true
				if node.Status == NarrativeNodeStatusIgnored {
					if strings.TrimSpace(node.IgnoreReason) == "" {
						issues = append(issues, NarrativeIssue{Code: "unnamed_ignored_node", Message: "忽略节点必须记录理由", SceneID: scene.ID, NodeID: node.ID})
					}
					continue
				}
				activeNodes++
				if !validNarrativeNodeKind(node.Kind) {
					issues = append(issues, NarrativeIssue{Code: "invalid_node_type", Message: "叙事节点类型无效", SceneID: scene.ID, NodeID: node.ID})
				}
				if strings.TrimSpace(node.Text) == "" {
					issues = append(issues, NarrativeIssue{Code: "empty_node", Message: "叙事节点正文为空", SceneID: scene.ID, NodeID: node.ID})
				}
				if node.Kind == NarrativeNodeDialogue && strings.TrimSpace(node.Speaker) == "" {
					anchor := node.Anchor
					issues = append(issues, NarrativeIssue{Code: "unknown_speaker", Message: "对白缺少明确说话人", SceneID: scene.ID, NodeID: node.ID, Anchor: &anchor})
				}
				if !anchorWithin(node.Anchor, scene.Anchor) {
					anchor := node.Anchor
					issues = append(issues, NarrativeIssue{Code: "node_anchor_outside_scene", Message: "叙事节点锚点越过场景", SceneID: scene.ID, NodeID: node.ID, Anchor: &anchor})
				}
				if previousNodeStart > node.Anchor.StartOffset {
					issues = append(issues, NarrativeIssue{Code: "node_out_of_order", Message: "叙事节点顺序与来源锚点不一致", SceneID: scene.ID, NodeID: node.ID})
				}
				previousNodeStart = node.Anchor.StartOffset
			}
			if activeNodes == 0 {
				issues = append(issues, NarrativeIssue{Code: "empty_scene", Message: "场景没有可发布的叙事节点", SceneID: scene.ID})
			}
		}
	}
	for index := range analysis.Mentions {
		mention := &analysis.Mentions[index]
		if mentionIDs[mention.ID] {
			issues = append(issues, NarrativeIssue{Code: "duplicate_mention_id", Message: "Mention ID 重复", MentionID: mention.ID})
		}
		mentionIDs[mention.ID] = true
		_, _, scene := findScene(analysis, mention.SceneID)
		if scene == nil || !anchorWithin(mention.Anchor, scene.Anchor) {
			anchor := mention.Anchor
			issues = append(issues, NarrativeIssue{Code: "mention_anchor_invalid", Message: "Mention 未绑定有效场景锚点", MentionID: mention.ID, Anchor: &anchor})
		}
		if !validElementType(mention.ElementType) || strings.TrimSpace(mention.SurfaceText) == "" {
			issues = append(issues, NarrativeIssue{Code: "mention_invalid", Message: "Mention 类型或来源文本无效", MentionID: mention.ID})
		}
	}
	for _, scope := range analysis.ParseReport.FailedScopes {
		if strings.TrimSpace(scope) != "" {
			issues = append(issues, NarrativeIssue{Code: "partial_source_scope", Message: "来源范围解析未完成：" + scope})
		}
	}
	analysis.Narrative.Issues = issues
	analysis.Narrative.Status = NarrativeStatusReady
	analysis.Narrative.Completeness = "complete"
	if len(analysis.ParseReport.FailedScopes) > 0 {
		analysis.Narrative.Completeness = "partial"
	}
	if len(issues) > 0 {
		analysis.Narrative.Status = NarrativeStatusBlocked
	}
	analysis.Narrative.ContentHash = hashNarrativeContent(*analysis)
}

func hashNarrativeContent(analysis Analysis) string {
	parts := []string{analysis.SourceHash, analysis.Breakdown.SegmentationHash}
	for _, episode := range analysis.Episodes {
		if episode.Decision == "ignored" {
			continue
		}
		parts = append(parts, "episode:"+episode.TemporaryKey)
		for _, scene := range episode.Scenes {
			parts = append(parts, fmt.Sprintf("scene:%s:%s:%d:%d", scene.ID, scene.Heading, scene.Anchor.StartOffset, scene.Anchor.EndOffset))
			for _, node := range scene.Narratives {
				parts = append(parts, fmt.Sprintf("node:%s:%s:%s:%s:%s:%d:%d", node.ID, node.Kind, node.Status, node.Speaker, node.Text, node.Anchor.StartOffset, node.Anchor.EndOffset))
				if node.Status == NarrativeNodeStatusIgnored {
					parts = append(parts, "ignored:"+node.IgnoreReason)
				}
			}
		}
	}
	mentions := append([]ProductionElementMention(nil), analysis.Mentions...)
	sort.Slice(mentions, func(left, right int) bool { return mentions[left].ID < mentions[right].ID })
	for _, mention := range mentions {
		parts = append(parts, fmt.Sprintf("mention:%s:%s:%s:%s:%s:%d:%d", mention.ID, mention.SceneID, mention.ElementType, mention.SurfaceText, mention.Status, mention.Anchor.StartOffset, mention.Anchor.EndOffset))
	}
	return HashContent(strings.Join(parts, "|"))
}

func mentionsInPublishedEpisodes(analysis Analysis) []ProductionElementMention {
	publishedScenes := map[string]bool{}
	for _, episode := range analysis.Episodes {
		if episode.Decision == "ignored" {
			continue
		}
		for _, scene := range episode.Scenes {
			publishedScenes[scene.ID] = true
		}
	}
	result := make([]ProductionElementMention, 0, len(analysis.Mentions))
	for _, mention := range analysis.Mentions {
		if publishedScenes[mention.SceneID] {
			result = append(result, mention)
		}
	}
	return result
}

func findScene(analysis *Analysis, sceneID string) (int, int, *Scene) {
	for episodeIndex := range analysis.Episodes {
		for sceneIndex := range analysis.Episodes[episodeIndex].Scenes {
			if analysis.Episodes[episodeIndex].Scenes[sceneIndex].ID == strings.TrimSpace(sceneID) {
				return episodeIndex, sceneIndex, &analysis.Episodes[episodeIndex].Scenes[sceneIndex]
			}
		}
	}
	return -1, -1, nil
}

func findNarrativeNode(analysis *Analysis, nodeID string) (*Scene, *NarrativeUnit) {
	for episodeIndex := range analysis.Episodes {
		for sceneIndex := range analysis.Episodes[episodeIndex].Scenes {
			scene := &analysis.Episodes[episodeIndex].Scenes[sceneIndex]
			for nodeIndex := range scene.Narratives {
				if scene.Narratives[nodeIndex].ID == strings.TrimSpace(nodeID) {
					return scene, &scene.Narratives[nodeIndex]
				}
			}
		}
	}
	return nil, nil
}

func validateNewMemberID(analysis *Analysis, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return errors.New("scene id must be a UUID")
	}
	if _, _, scene := findScene(analysis, id); scene != nil {
		return errors.New("scene id already exists")
	}
	return nil
}

func validateNarrativeNodeInput(kind NarrativeNodeKind, text string, anchor Anchor, scene Scene) error {
	if !validNarrativeNodeKind(kind) {
		return errors.New("narrative node type is invalid")
	}
	if len([]rune(strings.TrimSpace(text))) < 1 || len([]rune(strings.TrimSpace(text))) > 20_000 {
		return errors.New("narrative node text must contain 1 to 20000 characters")
	}
	if !anchorWithin(anchor, scene.Anchor) {
		return errors.New("narrative node anchor must be inside its scene")
	}
	return nil
}

func validNarrativeNodeKind(kind NarrativeNodeKind) bool {
	switch kind {
	case NarrativeNodeBeat, NarrativeNodeDialogue, NarrativeNodeAction, NarrativeNodeNarration:
		return true
	default:
		return false
	}
}

func validElementType(value string) bool {
	switch value {
	case "character", "location", "prop", "costume":
		return true
	default:
		return false
	}
}

func anchorWithin(candidate, container Anchor) bool {
	return candidate.StartOffset >= container.StartOffset && candidate.EndOffset > candidate.StartOffset && candidate.EndOffset <= container.EndOffset
}

func findMentionIndex(mentions []ProductionElementMention, id string) int {
	for index := range mentions {
		if mentions[index].ID == strings.TrimSpace(id) {
			return index
		}
	}
	return -1
}
