package gormdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planningdomain "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

type snapshotProjection struct {
	document model.ScriptDocument
	revision model.DocumentRevision
	source   storygraph.Node
	bible    *model.ProductionBibleVersion

	nodes []storygraph.Node
	edges []storygraph.Edge
	heads []storygraph.OwnerHeadRef

	nodeKeys     map[string]struct{}
	edgeKeys     map[string]struct{}
	headRefs     map[string]storygraph.OwnerHeadRef
	evidenceKeys map[string]string
	identities   map[string]storygraph.Node
	entityKeys   map[string]storygraph.Node
	specs        map[string]storygraph.Node
	states       map[string]storygraph.Node
	anchors      map[string]storygraph.Node
	anchorNodes  map[string]storygraph.Node
	bibleClaims  []bibledomain.StoryClaimCandidate
}

func newSnapshotProjection(
	document model.ScriptDocument,
	revision model.DocumentRevision,
	source storygraph.Node,
) *snapshotProjection {
	value := &snapshotProjection{
		document: document, revision: revision, source: source,
		nodes: []storygraph.Node{}, edges: []storygraph.Edge{}, heads: []storygraph.OwnerHeadRef{},
		nodeKeys: map[string]struct{}{}, edgeKeys: map[string]struct{}{},
		headRefs: map[string]storygraph.OwnerHeadRef{}, evidenceKeys: map[string]string{},
		identities: map[string]storygraph.Node{}, entityKeys: map[string]storygraph.Node{},
		specs: map[string]storygraph.Node{}, states: map[string]storygraph.Node{},
		anchors: map[string]storygraph.Node{}, anchorNodes: map[string]storygraph.Node{},
	}
	_ = value.addNode(source)
	_ = value.addHead(storygraph.OwnerHeadRefFrom(source.OwnerRef))
	return value
}

func (value *snapshotProjection) addNode(node storygraph.Node) error {
	if _, exists := value.nodeKeys[node.StoryNodeKey]; exists {
		return invalidOwnerSnapshot("formal Owner facts project a duplicate StoryGraph node")
	}
	value.nodeKeys[node.StoryNodeKey] = struct{}{}
	value.nodes = append(value.nodes, node)
	return nil
}

func (value *snapshotProjection) addEdge(edge storygraph.Edge) error {
	if _, exists := value.edgeKeys[edge.EdgeKey]; exists {
		return invalidOwnerSnapshot("formal Owner facts project a duplicate StoryGraph edge")
	}
	value.edgeKeys[edge.EdgeKey] = struct{}{}
	value.edges = append(value.edges, edge)
	return nil
}

func (value *snapshotProjection) addHead(head storygraph.OwnerHeadRef) error {
	key := head.OwnerKind + "\x00" + head.OwnerLogicalID
	if existing, exists := value.headRefs[key]; exists {
		if existing != head {
			return invalidOwnerSnapshot("formal Owner head changed inside one StoryGraph compilation")
		}
		return nil
	}
	value.headRefs[key] = head
	value.heads = append(value.heads, head)
	return nil
}

func (value *snapshotProjection) addAnchor(key string, node storygraph.Node) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return invalidOwnerSnapshot("formal StoryGraph anchor key is empty")
	}
	if existing, exists := value.anchors[key]; exists && existing.StoryNodeKey != node.StoryNodeKey {
		return invalidOwnerSnapshot("formal StoryGraph anchor key is ambiguous")
	}
	value.anchors[key] = node
	value.anchorNodes[node.StoryNodeKey] = node
	return nil
}

func (value *snapshotProjection) evidenceRefs(
	evidence []bibledomain.Evidence,
	episodePosition *int,
) ([]storygraph.EvidenceRef, error) {
	if len(evidence) == 0 {
		return nil, invalidOwnerSnapshot("formal StoryGraph fact has no source Evidence")
	}
	result := make([]storygraph.EvidenceRef, 0, len(evidence))
	seen := map[storygraph.EvidenceRef]struct{}{}
	for _, item := range evidence {
		if episodePosition != nil && (item.EpisodeNumber == nil || *item.EpisodeNumber != *episodePosition) {
			return nil, invalidOwnerSnapshot("Planning Evidence has an invalid Episode scope")
		}
		ref, err := evidenceRef(value.revision, item.SourceStart, item.SourceEnd)
		if err != nil || ref.TextHash != item.TextHash {
			return nil, invalidOwnerSnapshot("formal Evidence hash has drifted from the source revision")
		}
		runes := []rune(value.revision.NormalizedText)
		if item.ExactAnchor != string(runes[item.SourceStart:item.SourceEnd]) {
			return nil, invalidOwnerSnapshot("formal Evidence anchor has drifted from the source revision")
		}
		if _, duplicate := seen[ref]; duplicate {
			continue
		}
		seen[ref] = struct{}{}
		result = append(result, ref)
		if value.bible != nil {
			if err = value.ensureEvidenceNode(item, ref); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func (value *snapshotProjection) ensureEvidenceNode(
	evidence bibledomain.Evidence,
	ref storygraph.EvidenceRef,
) error {
	key := evidenceProjectionKey(ref)
	if _, exists := value.evidenceKeys[key]; exists {
		return nil
	}
	owner := storygraph.OwnerRef{
		OwnerKind: "production/bible", OwnerLogicalID: value.document.ID.String() + ":evidence",
		FragmentKey:    fmt.Sprintf("range/%012d/%012d/%s", ref.AbsoluteStart, ref.AbsoluteEnd, ref.TextHash),
		OwnerVersionID: value.bible.ID.String(), OwnerRevision: int64(value.bible.Version), ContentHash: value.bible.ContentHash,
	}
	node, err := newNode(storygraph.NodeTypeSourceEvidence, owner, "Source Evidence", nil, []storygraph.EvidenceRef{ref}, struct {
		SourceStart int    `json:"source_start"`
		SourceEnd   int    `json:"source_end"`
		TextHash    string `json:"text_hash"`
		ExactAnchor string `json:"exact_anchor"`
	}{evidence.SourceStart, evidence.SourceEnd, evidence.TextHash, evidence.ExactAnchor})
	if err != nil {
		return err
	}
	if err = value.addNode(node); err != nil {
		return err
	}
	if err = value.addHead(storygraph.OwnerHeadRefFrom(owner)); err != nil {
		return err
	}
	edge, err := newEdge(storygraph.EdgeTypeDerivedFrom, value.source.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{})
	if err != nil {
		return err
	}
	if err = value.addEdge(edge); err != nil {
		return err
	}
	value.evidenceKeys[key] = node.StoryNodeKey
	return nil
}

func (value *snapshotProjection) linkEvidence(
	refs []storygraph.EvidenceRef,
	target storygraph.Node,
	edgeType storygraph.EdgeType,
) error {
	if value.bible == nil {
		return nil
	}
	for _, ref := range refs {
		from, exists := value.evidenceKeys[evidenceProjectionKey(ref)]
		if !exists {
			return invalidOwnerSnapshot("formal Evidence node is missing")
		}
		edge, err := newEdge(edgeType, from, target.StoryNodeKey, storygraph.EdgeQualifier{})
		if err != nil {
			return err
		}
		if err = value.addEdge(edge); err != nil {
			return err
		}
	}
	return nil
}

func evidenceProjectionKey(value storygraph.EvidenceRef) string {
	return fmt.Sprintf("%s:%d:%d:%s", value.DocumentRevisionID, value.AbsoluteStart, value.AbsoluteEnd, value.TextHash)
}

func (repo *repository) LoadOwnerSnapshot(
	ctx context.Context,
	state storygraph.PublicationState,
) (storygraph.OwnerSnapshot, error) {
	workspaceID, err := uuid.Parse(state.WorkspaceID)
	if err != nil {
		return storygraph.OwnerSnapshot{}, err
	}
	projectID, err := uuid.Parse(state.ProjectID)
	if err != nil {
		return storygraph.OwnerSnapshot{}, err
	}
	var documents []model.ScriptDocument
	if err = repo.database.WithContext(ctx).
		Where("workspace_id = ? AND project_id = ? AND status = ?", workspaceID, projectID, "active").
		Order("created_at DESC").Order("id").Find(&documents).Error; err != nil {
		return storygraph.OwnerSnapshot{}, err
	}
	if len(documents) != 1 {
		return storygraph.OwnerSnapshot{}, invalidOwnerSnapshot("StoryGraph MVP requires exactly one active script document")
	}
	document := documents[0]
	var revision model.DocumentRevision
	if err = repo.database.WithContext(ctx).Where("document_id = ?", document.ID).
		Order("version_no DESC").Order("id").First(&revision).Error; err != nil {
		return storygraph.OwnerSnapshot{}, normalizeNotFound(err)
	}
	sourceOwner := storygraph.OwnerRef{
		OwnerKind: "production/script", OwnerLogicalID: document.ID.String(),
		OwnerVersionID: revision.ID.String(), OwnerRevision: int64(revision.VersionNo), ContentHash: revision.NormalizedHash,
	}
	sourceNode, err := newNode(storygraph.NodeTypeSourceRevision, sourceOwner, document.Title, nil, []storygraph.EvidenceRef{}, struct {
		VersionNo         int    `json:"version_no"`
		NormalizerVersion string `json:"normalizer_version"`
		AnalysisStatus    string `json:"analysis_status"`
	}{revision.VersionNo, revision.NormalizerVersion, revision.AnalysisStatus})
	if err != nil {
		return storygraph.OwnerSnapshot{}, err
	}
	projection := newSnapshotProjection(document, revision, sourceNode)
	if err = repo.projectBible(ctx, workspaceID, projectID, projection); err != nil {
		return storygraph.OwnerSnapshot{}, err
	}
	if err = repo.projectEpisodes(ctx, workspaceID, projectID, projection); err != nil {
		return storygraph.OwnerSnapshot{}, err
	}
	if err = projectBibleClaims(projection, projectID); err != nil {
		return storygraph.OwnerSnapshot{}, err
	}
	return storygraph.OwnerSnapshot{
		Origin:      storygraph.OwnerSnapshotOriginConfirmed,
		WorkspaceID: state.WorkspaceID, ProjectID: state.ProjectID,
		SourceRevisionID: revision.ID.String(), SourceRevisionHash: revision.NormalizedHash,
		OwnerHeads: projection.heads,
		Graph:      storygraph.Snapshot{SchemaVersion: storygraph.SchemaVersion, Nodes: projection.nodes, Edges: projection.edges},
	}, nil
}

func (repo *repository) projectEpisodes(
	ctx context.Context,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	projection *snapshotProjection,
) error {
	var episodes []model.Episode
	if err := repo.database.WithContext(ctx).
		Where("workspace_id = ? AND project_id = ? AND status = ?", workspaceID, projectID, "active").
		Order("position").Order("id").Find(&episodes).Error; err != nil {
		return err
	}
	for _, episode := range episodes {
		if err := repo.projectEpisode(ctx, workspaceID, projectID, projection, episode); err != nil {
			return err
		}
	}
	return nil
}

func (repo *repository) projectEpisode(
	ctx context.Context,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	projection *snapshotProjection,
	episode model.Episode,
) error {
	if episode.CurrentScriptVersionID == nil {
		return invalidOwnerSnapshot("active Episode has no published Script Version")
	}
	var scriptVersion model.EpisodeScriptVersion
	if err := repo.database.WithContext(ctx).
		Where("id = ? AND workspace_id = ? AND project_id = ? AND episode_id = ? AND status = ?", *episode.CurrentScriptVersionID, workspaceID, projectID, episode.ID, "published").
		First(&scriptVersion).Error; err != nil {
		return normalizeNotFound(err)
	}
	if scriptVersion.DocumentRevisionID != projection.revision.ID {
		return invalidOwnerSnapshot("Episode Script Version does not reference the current source revision")
	}
	episodeHash, err := storygraph.HashCanonicalValue(struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		Status            string `json:"status"`
		ScriptVersionID   string `json:"script_version_id"`
		ScriptContentHash string `json:"script_content_hash"`
		Position          int    `json:"position"`
		TargetDurationMS  int    `json:"target_duration_ms"`
		Revision          int    `json:"revision"`
	}{episode.ID.String(), episode.Name, episode.Status, scriptVersion.ID.String(), scriptVersion.ContentHash, episode.Position, episode.TargetDurationMS, episode.Revision})
	if err != nil {
		return err
	}
	owner := storygraph.OwnerRef{
		OwnerKind: "production/project", OwnerLogicalID: episode.ID.String(), OwnerVersionID: episode.ID.String(),
		OwnerRevision: int64(episode.Revision), ContentHash: episodeHash,
	}
	evidence, err := evidenceRef(projection.revision, scriptVersion.SourceStart, scriptVersion.SourceEnd)
	if err != nil {
		return err
	}
	position, _ := json.Marshal(struct {
		EpisodePosition int `json:"episode_position"`
	}{episode.Position})
	node, err := newNode(storygraph.NodeTypeEpisode, owner, episode.Name, position, []storygraph.EvidenceRef{evidence}, struct {
		Position          int    `json:"position"`
		TargetDurationMS  int    `json:"target_duration_ms"`
		ScriptVersionID   string `json:"script_version_id"`
		ScriptContentHash string `json:"script_content_hash"`
	}{episode.Position, episode.TargetDurationMS, scriptVersion.ID.String(), scriptVersion.ContentHash})
	if err != nil {
		return err
	}
	if err = projection.addNode(node); err != nil {
		return err
	}
	if err = projection.addHead(storygraph.OwnerHeadRefFrom(owner)); err != nil {
		return err
	}
	derived, err := newEdge(storygraph.EdgeTypeDerivedFrom, projection.source.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{})
	if err != nil {
		return err
	}
	if err = projection.addEdge(derived); err != nil {
		return err
	}
	for _, key := range []string{episode.ID.String(), "episode:" + episode.ID.String(), fmt.Sprintf("episode:%d", episode.Position)} {
		if err = projection.addAnchor(key, node); err != nil {
			return err
		}
	}

	var structure model.EpisodeStructure
	err = repo.database.WithContext(ctx).
		Where("workspace_id = ? AND project_id = ? AND episode_id = ? AND script_version_id = ? AND status = ?", workspaceID, projectID, episode.ID, scriptVersion.ID, "confirmed").
		Order("confirmed_at DESC").Order("created_at DESC").Order("id").First(&structure).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	scenes, err := decodePlanningScenes(structure.Scenes)
	if err != nil {
		return err
	}
	computedHash, err := bibledomain.CanonicalStoryHash(struct {
		Schema string                 `json:"schema"`
		Scenes []planningdomain.Scene `json:"scenes"`
	}{"episode-planning-owner-v1", scenes})
	if err != nil || computedHash != structure.ResultHash {
		return invalidOwnerSnapshot("confirmed Episode Structure result hash has drifted")
	}
	structureOwner := storygraph.OwnerRef{
		OwnerKind: "production/planning", OwnerLogicalID: episode.ID.String(),
		OwnerVersionID: structure.ID.String(), OwnerRevision: int64(structure.Revision), ContentHash: structure.ResultHash,
	}
	if err = projection.addHead(storygraph.OwnerHeadRefFrom(structureOwner)); err != nil {
		return err
	}
	return projectPlanningScenes(projection, episode, node, structureOwner, scenes)
}

func projectPlanningScenes(
	projection *snapshotProjection,
	episode model.Episode,
	episodeNode storygraph.Node,
	structureOwner storygraph.OwnerRef,
	scenes []planningdomain.Scene,
) error {
	fragments := map[string]storygraph.Node{}
	var previousScene *storygraph.Node
	for sceneIndex, scene := range scenes {
		position := episode.Position
		evidence, err := projection.evidenceRefs(scene.Evidence, &position)
		if err != nil {
			return err
		}
		owner := structureOwner
		owner.FragmentKey = "scene/" + scene.TemporaryKey
		businessPosition, _ := json.Marshal(struct {
			EpisodePosition int `json:"episode_position"`
			ScenePosition   int `json:"scene_position"`
		}{episode.Position, scene.Position})
		locationIdentity, locationState, err := projection.resolveOptionalLocation(scene)
		if err != nil {
			return err
		}
		node, err := newNode(storygraph.NodeTypeScene, owner, scene.Heading, businessPosition, evidence, struct {
			FragmentID       string `json:"fragment_id"`
			TemporaryKey     string `json:"temporary_key"`
			Heading          string `json:"heading"`
			SourceStart      int    `json:"source_start"`
			SourceEnd        int    `json:"source_end"`
			LocationIdentity string `json:"location_identity,omitempty"`
			LocationState    string `json:"location_state,omitempty"`
		}{scene.ID, scene.TemporaryKey, scene.Heading, scene.SourceStart, scene.SourceEnd, locationIdentity, locationState})
		if err != nil {
			return err
		}
		if err = projection.addNode(node); err != nil {
			return err
		}
		if err = projection.linkEvidence(evidence, node, storygraph.EdgeTypeDerivedFrom); err != nil {
			return err
		}
		contains, err := newEdge(storygraph.EdgeTypeContains, episodeNode.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: fmt.Sprintf("episode/%s/scenes/%08d", episode.ID, sceneIndex+1)})
		if err != nil {
			return err
		}
		if err = projection.addEdge(contains); err != nil {
			return err
		}
		if previousScene != nil {
			precedes, edgeErr := newEdge(storygraph.EdgeTypePrecedes, previousScene.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: fmt.Sprintf("episode/%s/scenes/%08d", episode.ID, sceneIndex+1)})
			if edgeErr != nil {
				return edgeErr
			}
			if edgeErr = projection.addEdge(precedes); edgeErr != nil {
				return edgeErr
			}
		}
		previousScene = &node
		for _, key := range []string{scene.ID, scene.TemporaryKey} {
			if err = projection.addAnchor(key, node); err != nil {
				return err
			}
		}
		fragments["scene\x00"+scene.ID] = node
		fragments["scene\x00"+scene.TemporaryKey] = node
		if err = projectDialogues(projection, episode, structureOwner, scene, node, fragments); err != nil {
			return err
		}
		if err = projectBeats(projection, episode, structureOwner, scene, node, fragments); err != nil {
			return err
		}
		if err = projectOccurrences(projection, episode, structureOwner, scene, node, fragments); err != nil {
			return err
		}
	}
	for _, scene := range scenes {
		if err := projectClaims(projection, episode, structureOwner, scene, fragments); err != nil {
			return err
		}
	}
	return nil
}

func projectDialogues(
	projection *snapshotProjection,
	episode model.Episode,
	structureOwner storygraph.OwnerRef,
	scene planningdomain.Scene,
	sceneNode storygraph.Node,
	fragments map[string]storygraph.Node,
) error {
	for index, dialogue := range scene.Dialogues {
		position := episode.Position
		evidence, err := projection.evidenceRefs(dialogue.Evidence, &position)
		if err != nil {
			return err
		}
		speakerKey := ""
		if dialogue.SpeakerIdentity != nil {
			speaker, resolveErr := projection.resolveIdentity(*dialogue.SpeakerIdentity)
			if resolveErr != nil {
				return resolveErr
			}
			speakerKey = speaker.StoryNodeKey
		}
		owner := structureOwner
		owner.FragmentKey = "dialogue/" + dialogue.TemporaryKey
		businessPosition, _ := json.Marshal(struct {
			EpisodePosition  int `json:"episode_position"`
			ScenePosition    int `json:"scene_position"`
			DialoguePosition int `json:"dialogue_position"`
		}{episode.Position, scene.Position, index + 1})
		node, err := newNode(storygraph.NodeTypeDialogue, owner, dialogue.Speaker, businessPosition, evidence, struct {
			FragmentID          string `json:"fragment_id"`
			TemporaryKey        string `json:"temporary_key"`
			Speaker             string `json:"speaker"`
			SpeakerStoryNodeKey string `json:"speaker_story_node_key,omitempty"`
			Text                string `json:"text"`
			SourceStart         int    `json:"source_start"`
			SourceEnd           int    `json:"source_end"`
		}{dialogue.ID, dialogue.TemporaryKey, dialogue.Speaker, speakerKey, dialogue.Text, dialogue.SourceStart, dialogue.SourceEnd})
		if err != nil {
			return err
		}
		if err = projection.addNode(node); err != nil {
			return err
		}
		if err = projection.linkEvidence(evidence, node, storygraph.EdgeTypeDerivedFrom); err != nil {
			return err
		}
		contains, err := newEdge(storygraph.EdgeTypeContains, sceneNode.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: fmt.Sprintf("scene/%s/dialogues/%08d", scene.ID, index+1)})
		if err != nil {
			return err
		}
		if err = projection.addEdge(contains); err != nil {
			return err
		}
		fragments["dialogue\x00"+dialogue.ID] = node
		fragments["dialogue\x00"+dialogue.TemporaryKey] = node
	}
	return nil
}

func projectBeats(
	projection *snapshotProjection,
	episode model.Episode,
	structureOwner storygraph.OwnerRef,
	scene planningdomain.Scene,
	sceneNode storygraph.Node,
	fragments map[string]storygraph.Node,
) error {
	for index, beat := range scene.NarrativeUnits {
		position := episode.Position
		evidence, err := projection.evidenceRefs(beat.Evidence, &position)
		if err != nil {
			return err
		}
		participantKeys := make([]string, len(beat.Participants))
		for participantIndex, participant := range beat.Participants {
			identity, resolveErr := projection.resolveIdentity(participant)
			if resolveErr != nil {
				return resolveErr
			}
			participantKeys[participantIndex] = identity.StoryNodeKey
		}
		owner := structureOwner
		owner.FragmentKey = "narrative-beat/" + beat.TemporaryKey
		businessPosition, _ := json.Marshal(struct {
			EpisodePosition int `json:"episode_position"`
			ScenePosition   int `json:"scene_position"`
			BeatPosition    int `json:"beat_position"`
		}{episode.Position, scene.Position, index + 1})
		node, err := newNode(storygraph.NodeTypeNarrativeBeat, owner, beat.Text, businessPosition, evidence, struct {
			FragmentID   string   `json:"fragment_id"`
			TemporaryKey string   `json:"temporary_key"`
			Kind         string   `json:"kind"`
			Text         string   `json:"text"`
			SourceStart  int      `json:"source_start"`
			SourceEnd    int      `json:"source_end"`
			Participants []string `json:"participants"`
		}{beat.ID, beat.TemporaryKey, beat.Kind, beat.Text, beat.SourceStart, beat.SourceEnd, participantKeys})
		if err != nil {
			return err
		}
		if err = projection.addNode(node); err != nil {
			return err
		}
		if err = projection.linkEvidence(evidence, node, storygraph.EdgeTypeDerivedFrom); err != nil {
			return err
		}
		contains, err := newEdge(storygraph.EdgeTypeContains, sceneNode.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: fmt.Sprintf("scene/%s/beats/%08d", scene.ID, index+1)})
		if err != nil {
			return err
		}
		if err = projection.addEdge(contains); err != nil {
			return err
		}
		fragments["beat\x00"+beat.ID] = node
		fragments["beat\x00"+beat.TemporaryKey] = node
		for _, key := range []string{beat.ID, beat.TemporaryKey} {
			if err = projection.addAnchor(key, node); err != nil {
				return err
			}
		}
	}
	return nil
}

func projectOccurrences(
	projection *snapshotProjection,
	episode model.Episode,
	structureOwner storygraph.OwnerRef,
	scene planningdomain.Scene,
	sceneNode storygraph.Node,
	fragments map[string]storygraph.Node,
) error {
	for index, occurrence := range scene.Occurrences {
		if occurrence.SceneID != scene.ID {
			return invalidOwnerSnapshot("Occurrence does not reference its exact Scene")
		}
		position := episode.Position
		evidence, err := projection.evidenceRefs(occurrence.Evidence, &position)
		if err != nil {
			return err
		}
		identity, err := projection.resolveIdentity(occurrence.Identity)
		if err != nil {
			return err
		}
		state, err := projection.resolveState(occurrence.Identity.AssetID, occurrence.State)
		if err != nil {
			return err
		}
		owner := structureOwner
		owner.FragmentKey = "occurrence/" + occurrence.TemporaryKey
		businessPosition, _ := json.Marshal(struct {
			EpisodePosition    int `json:"episode_position"`
			ScenePosition      int `json:"scene_position"`
			OccurrencePosition int `json:"occurrence_position"`
		}{episode.Position, scene.Position, index + 1})
		node, err := newNode(storygraph.NodeTypeOccurrence, owner, occurrence.Summary, businessPosition, evidence, struct {
			FragmentID           string `json:"fragment_id"`
			TemporaryKey         string `json:"temporary_key"`
			SceneID              string `json:"scene_id"`
			IdentityStoryNodeKey string `json:"identity_story_node_key"`
			StateStoryNodeKey    string `json:"state_story_node_key"`
			Summary              string `json:"summary"`
			SourceStart          int    `json:"source_start"`
			SourceEnd            int    `json:"source_end"`
		}{occurrence.ID, occurrence.TemporaryKey, occurrence.SceneID, identity.StoryNodeKey, state.StoryNodeKey, occurrence.Summary, occurrence.SourceStart, occurrence.SourceEnd})
		if err != nil {
			return err
		}
		if err = projection.addNode(node); err != nil {
			return err
		}
		if err = projection.linkEvidence(evidence, node, storygraph.EdgeTypeDerivedFrom); err != nil {
			return err
		}
		anchor, err := newEdge(storygraph.EdgeTypeAnchorsOccurrence, sceneNode.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{})
		if err != nil {
			return err
		}
		if err = projection.addEdge(anchor); err != nil {
			return err
		}
		instantiates, err := newEdge(storygraph.EdgeTypeInstantiatesOccurrence, state.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{})
		if err != nil {
			return err
		}
		if err = projection.addEdge(instantiates); err != nil {
			return err
		}
		fragments["occurrence\x00"+occurrence.ID] = node
		fragments["occurrence\x00"+occurrence.TemporaryKey] = node
		for _, key := range []string{occurrence.ID, occurrence.TemporaryKey} {
			if err = projection.addAnchor(key, node); err != nil {
				return err
			}
		}
	}
	return nil
}

func projectClaims(
	projection *snapshotProjection,
	episode model.Episode,
	structureOwner storygraph.OwnerRef,
	scene planningdomain.Scene,
	fragments map[string]storygraph.Node,
) error {
	for _, claim := range scene.Claims {
		nodeType := storygraph.NodeType("")
		switch claim.ClaimType {
		case "causal":
			nodeType = storygraph.NodeTypeCausalClaim
		case "continuity":
			nodeType = storygraph.NodeTypeContinuityClaim
		default:
			return invalidOwnerSnapshot("Planning Claim type is not owned by Episode Planning")
		}
		if claim.Scope != "episode:"+episode.ID.String() || claim.Status != "confirmed" ||
			!slices.Contains([]string{"positive", "negative"}, claim.Polarity) {
			return invalidOwnerSnapshot("Planning Claim scope or assertion state is invalid")
		}
		participants := make([]storygraph.ClaimParticipant, len(claim.Participants))
		for index, participant := range claim.Participants {
			identity, err := projection.resolveIdentity(participant.Identity)
			if err != nil {
				return err
			}
			participants[index] = storygraph.ClaimParticipant{Role: participant.Role, StoryNodeKey: identity.StoryNodeKey}
		}
		anchors := make([]string, len(claim.Anchors))
		anchorNodes := make([]storygraph.Node, len(claim.Anchors))
		for index, anchor := range claim.Anchors {
			kind := anchor.Kind
			if kind == "narrative_beat" {
				kind = "beat"
			}
			if !slices.Contains([]string{"scene", "beat", "occurrence"}, kind) {
				return invalidOwnerSnapshot("Planning Claim anchor kind is not semantic")
			}
			node, exists := fragments[kind+"\x00"+anchor.FragmentID]
			if !exists {
				node, exists = fragments[kind+"\x00"+anchor.TemporaryKey]
			}
			if !exists {
				return invalidOwnerSnapshot("Planning Claim anchor does not reference an exact graph fragment")
			}
			anchors[index], anchorNodes[index] = node.StoryNodeKey, node
		}
		position := episode.Position
		evidence, err := projection.evidenceRefs(claim.Evidence, &position)
		if err != nil {
			return err
		}
		owner := structureOwner
		owner.FragmentKey = "claim/" + claim.TemporaryKey
		payload := storygraph.ClaimPayload{
			Predicate: claim.ClaimType, Participants: participants, Anchors: anchors,
			ValidScope: storygraph.ClaimScope{Kind: "episode", OwnerLogicalID: episode.ID.String()},
			Polarity:   claim.Polarity, Status: "asserted",
		}
		node, err := newNode(nodeType, owner, claim.ClaimType, nil, evidence, payload)
		if err != nil {
			return err
		}
		if err = projection.addNode(node); err != nil {
			return err
		}
		if err = projection.linkEvidence(evidence, node, storygraph.EdgeTypeSupports); err != nil {
			return err
		}
		for _, participant := range participants {
			edge, edgeErr := newEdge(storygraph.EdgeTypeClaimParticipant, participant.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{ParticipantRole: participant.Role})
			if edgeErr != nil {
				return edgeErr
			}
			if edgeErr = projection.addEdge(edge); edgeErr != nil {
				return edgeErr
			}
		}
		for index, anchorNode := range anchorNodes {
			edge, edgeErr := newEdge(storygraph.EdgeTypeClaimAnchor, anchorNode.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: claim.Anchors[index].Role})
			if edgeErr != nil {
				return edgeErr
			}
			if edgeErr = projection.addEdge(edge); edgeErr != nil {
				return edgeErr
			}
		}
	}
	return nil
}

func (value *snapshotProjection) resolveIdentity(reference planningdomain.PlanningIdentityReference) (storygraph.Node, error) {
	node, exists := value.identities[reference.AssetID]
	specification, specificationExists := value.specs[reference.SpecificationID]
	if !exists || !specificationExists || node.OwnerRef.OwnerRevision != int64(reference.AssetRevision) ||
		node.OwnerRef.ContentHash != reference.AssetContentHash || specification.OwnerRef.OwnerRevision != int64(reference.SpecificationVersion) ||
		specification.OwnerRef.ContentHash != reference.SpecificationHash {
		return storygraph.Node{}, invalidOwnerSnapshot("Planning Identity does not reference exact materialized Bible facts")
	}
	var payload struct {
		EntityKey string `json:"entity_key"`
		Kind      string `json:"kind"`
	}
	if err := json.Unmarshal(node.Payload, &payload); err != nil || payload.EntityKey != reference.EntityKey || payload.Kind != reference.Kind {
		return storygraph.Node{}, invalidOwnerSnapshot("Planning Identity semantic key has drifted")
	}
	return node, nil
}

func (value *snapshotProjection) resolveState(assetID string, reference planningdomain.PlanningStateReference) (storygraph.Node, error) {
	node, exists := value.states[reference.ID]
	if !exists || node.OwnerRef.OwnerRevision != int64(reference.Revision) || node.OwnerRef.ContentHash != reference.ContentHash {
		return storygraph.Node{}, invalidOwnerSnapshot("Planning State does not reference an exact materialized Bible fact")
	}
	var payload struct {
		AssetID  string `json:"asset_id"`
		StateKey string `json:"state_key"`
	}
	if err := json.Unmarshal(node.Payload, &payload); err != nil || payload.AssetID != assetID || payload.StateKey != reference.StateKey {
		return storygraph.Node{}, invalidOwnerSnapshot("Planning State semantic key has drifted")
	}
	return node, nil
}

func (value *snapshotProjection) resolveOptionalLocation(scene planningdomain.Scene) (string, string, error) {
	if scene.LocationIdentity == nil && scene.LocationState == nil {
		return "", "", nil
	}
	if scene.LocationIdentity == nil {
		return "", "", invalidOwnerSnapshot("Scene location State has no Identity")
	}
	identity, err := value.resolveIdentity(*scene.LocationIdentity)
	if err != nil {
		return "", "", err
	}
	stateKey := ""
	if scene.LocationState != nil {
		state, stateErr := value.resolveState(scene.LocationIdentity.AssetID, *scene.LocationState)
		if stateErr != nil {
			return "", "", stateErr
		}
		stateKey = state.StoryNodeKey
	}
	return identity.StoryNodeKey, stateKey, nil
}

func decodePlanningScenes(raw []byte) ([]planningdomain.Scene, error) {
	var scenes []planningdomain.Scene
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&scenes); err != nil || scenes == nil {
		return nil, invalidOwnerSnapshot("confirmed Episode Structure is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, invalidOwnerSnapshot("confirmed Episode Structure contains multiple JSON values")
	}
	for index, scene := range scenes {
		if _, err := uuid.Parse(scene.ID); err != nil || strings.TrimSpace(scene.TemporaryKey) == "" ||
			scene.Position != index+1 || strings.TrimSpace(scene.Heading) == "" || scene.SourceStart < 0 || scene.SourceEnd <= scene.SourceStart {
			return nil, invalidOwnerSnapshot("confirmed Episode Structure contains an invalid Scene")
		}
		for _, dialogue := range scene.Dialogues {
			if _, err := uuid.Parse(dialogue.ID); err != nil || strings.TrimSpace(dialogue.TemporaryKey) == "" ||
				strings.TrimSpace(dialogue.Speaker) == "" || strings.TrimSpace(dialogue.Text) == "" {
				return nil, invalidOwnerSnapshot("confirmed Episode Structure contains an invalid Dialogue")
			}
		}
		for _, beat := range scene.NarrativeUnits {
			if _, err := uuid.Parse(beat.ID); err != nil || strings.TrimSpace(beat.TemporaryKey) == "" ||
				strings.TrimSpace(beat.Kind) == "" || strings.TrimSpace(beat.Text) == "" {
				return nil, invalidOwnerSnapshot("confirmed Episode Structure contains an invalid Narrative Beat")
			}
		}
		for _, occurrence := range scene.Occurrences {
			if _, err := uuid.Parse(occurrence.ID); err != nil || strings.TrimSpace(occurrence.TemporaryKey) == "" ||
				strings.TrimSpace(occurrence.Summary) == "" {
				return nil, invalidOwnerSnapshot("confirmed Episode Structure contains an invalid Occurrence")
			}
		}
		for _, claim := range scene.Claims {
			if _, err := uuid.Parse(claim.ID); err != nil || strings.TrimSpace(claim.TemporaryKey) == "" ||
				len(claim.Participants) < 2 || len(claim.Anchors) == 0 {
				return nil, invalidOwnerSnapshot("confirmed Episode Structure contains an invalid Claim")
			}
		}
	}
	return scenes, nil
}
