package gormdb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

type Store struct{ database *gorm.DB }
type repository struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) WithinSerializableTransaction(ctx context.Context, operation func(storygraphapp.Repository) error) error {
	return platformdatabase.WithinSerializableTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) LockPublication(ctx context.Context, actor storygraphapp.Actor, projectID string) (storygraph.PublicationState, error) {
	project, err := repo.authorizeProject(ctx, actor, projectID, true, true)
	if err != nil {
		return storygraph.PublicationState{}, err
	}
	state := storygraph.PublicationState{WorkspaceID: project.WorkspaceID.String(), ProjectID: project.ID.String()}
	var head model.StoryGraphHead
	err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&head, "project_id = ?", project.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return state, nil
	}
	if err != nil {
		return storygraph.PublicationState{}, err
	}
	if head.WorkspaceID != project.WorkspaceID {
		return storygraph.PublicationState{}, errors.New("StoryGraph head workspace has drifted")
	}
	state.CurrentVersionID = head.CurrentVersionID.String()
	state.CurrentContentHash = head.CurrentContentHash
	state.HeadRevision = head.Revision
	return state, nil
}

func (repo *repository) authorizeProject(ctx context.Context, actor storygraphapp.Actor, projectID string, lock, requireWrite bool) (model.Project, error) {
	id, err := uuid.Parse(projectID)
	if err != nil {
		return model.Project{}, storygraphapp.ErrNotFound
	}
	userID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return model.Project{}, unauthenticated()
	}
	var user model.UserAccount
	err = repo.database.WithContext(ctx).First(&user, "id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Project{}, unauthenticated()
	}
	if err != nil {
		return model.Project{}, err
	}
	if user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return model.Project{}, unauthenticated()
	}
	var project model.Project
	query := repo.database.WithContext(ctx)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err = query.First(&project, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Project{}, storygraphapp.ErrNotFound
	}
	if err != nil {
		return model.Project{}, err
	}
	var workspace model.Workspace
	err = repo.database.WithContext(ctx).First(&workspace, "id = ?", project.WorkspaceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Project{}, storygraphapp.ErrNotFound
	}
	if err != nil {
		return model.Project{}, err
	}
	var membership model.Membership
	err = repo.database.WithContext(ctx).Where("workspace_id = ? AND user_id = ? AND status = ?", project.WorkspaceID, userID, "active").First(&membership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Project{}, storygraphapp.ErrNotFound
	}
	if err != nil {
		return model.Project{}, err
	}
	if (requireWrite && membership.Role == "viewer") || workspace.Status != "active" || project.Status != "active" {
		return model.Project{}, forbidden()
	}
	return project, nil
}

func (repo *repository) LoadOwnerSnapshot(ctx context.Context, state storygraph.PublicationState) (storygraph.OwnerSnapshot, error) {
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
		OwnerVersionID: revision.ID.String(), OwnerRevision: int64(revision.VersionNo),
		ContentHash: revision.NormalizedHash,
	}
	sourceNode, err := newNode(storygraph.NodeTypeSourceRevision, sourceOwner, document.Title, nil, []storygraph.EvidenceRef{}, struct {
		VersionNo         int    `json:"version_no"`
		NormalizerVersion string `json:"normalizer_version"`
		AnalysisStatus    string `json:"analysis_status"`
	}{revision.VersionNo, revision.NormalizerVersion, revision.AnalysisStatus})
	if err != nil {
		return storygraph.OwnerSnapshot{}, err
	}
	nodes := []storygraph.Node{sourceNode}
	edges := make([]storygraph.Edge, 0)
	heads := []storygraph.OwnerHeadRef{storygraph.OwnerHeadRefFrom(sourceOwner)}

	var episodes []model.Episode
	if err = repo.database.WithContext(ctx).Where("workspace_id = ? AND project_id = ? AND status = ?", workspaceID, projectID, "active").
		Order("position").Order("id").Find(&episodes).Error; err != nil {
		return storygraph.OwnerSnapshot{}, err
	}
	for _, episode := range episodes {
		if episode.CurrentScriptVersionID == nil {
			return storygraph.OwnerSnapshot{}, invalidOwnerSnapshot("active Episode has no published Script Version")
		}
		var scriptVersion model.EpisodeScriptVersion
		if err = repo.database.WithContext(ctx).Where("id = ? AND workspace_id = ? AND project_id = ? AND episode_id = ? AND status = ?", *episode.CurrentScriptVersionID, workspaceID, projectID, episode.ID, "published").First(&scriptVersion).Error; err != nil {
			return storygraph.OwnerSnapshot{}, normalizeNotFound(err)
		}
		if scriptVersion.DocumentRevisionID != revision.ID {
			return storygraph.OwnerSnapshot{}, invalidOwnerSnapshot("Episode Script Version does not reference the current source revision")
		}
		episodeHash, hashErr := storygraph.HashCanonicalValue(struct {
			ID                string `json:"id"`
			Name              string `json:"name"`
			Status            string `json:"status"`
			ScriptVersionID   string `json:"script_version_id"`
			ScriptContentHash string `json:"script_content_hash"`
			Position          int    `json:"position"`
			TargetDurationMS  int    `json:"target_duration_ms"`
			Revision          int    `json:"revision"`
		}{episode.ID.String(), episode.Name, episode.Status, scriptVersion.ID.String(), scriptVersion.ContentHash, episode.Position, episode.TargetDurationMS, episode.Revision})
		if hashErr != nil {
			return storygraph.OwnerSnapshot{}, hashErr
		}
		episodeOwner := storygraph.OwnerRef{
			OwnerKind: "production/project", OwnerLogicalID: episode.ID.String(),
			OwnerVersionID: episode.ID.String(), OwnerRevision: int64(episode.Revision), ContentHash: episodeHash,
		}
		episodeEvidence, evidenceErr := evidenceRef(revision, scriptVersion.SourceStart, scriptVersion.SourceEnd)
		if evidenceErr != nil {
			return storygraph.OwnerSnapshot{}, evidenceErr
		}
		episodePosition, encodeErr := json.Marshal(struct {
			EpisodePosition int `json:"episode_position"`
		}{episode.Position})
		if encodeErr != nil {
			return storygraph.OwnerSnapshot{}, encodeErr
		}
		episodeNode, nodeErr := newNode(storygraph.NodeTypeEpisode, episodeOwner, episode.Name, episodePosition, []storygraph.EvidenceRef{episodeEvidence}, struct {
			Position          int    `json:"position"`
			TargetDurationMS  int    `json:"target_duration_ms"`
			ScriptVersionID   string `json:"script_version_id"`
			ScriptContentHash string `json:"script_content_hash"`
		}{episode.Position, episode.TargetDurationMS, scriptVersion.ID.String(), scriptVersion.ContentHash})
		if nodeErr != nil {
			return storygraph.OwnerSnapshot{}, nodeErr
		}
		nodes = append(nodes, episodeNode)
		heads = append(heads, storygraph.OwnerHeadRefFrom(episodeOwner))
		derived, edgeErr := newEdge(storygraph.EdgeTypeDerivedFrom, sourceNode.StoryNodeKey, episodeNode.StoryNodeKey, storygraph.EdgeQualifier{})
		if edgeErr != nil {
			return storygraph.OwnerSnapshot{}, edgeErr
		}
		edges = append(edges, derived)

		var structure model.EpisodeStructure
		err = repo.database.WithContext(ctx).
			Where("workspace_id = ? AND project_id = ? AND episode_id = ? AND script_version_id = ? AND status = ?", workspaceID, projectID, episode.ID, scriptVersion.ID, "confirmed").
			Order("confirmed_at DESC").Order("created_at DESC").Order("id").First(&structure).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return storygraph.OwnerSnapshot{}, err
		}
		structureOwner := storygraph.OwnerRef{
			OwnerKind: "production/planning", OwnerLogicalID: episode.ID.String(),
			OwnerVersionID: structure.ID.String(), OwnerRevision: int64(structure.Revision), ContentHash: structure.ResultHash,
		}
		heads = append(heads, storygraph.OwnerHeadRefFrom(structureOwner))
		scenes, decodeErr := decodeScenes(structure.Scenes)
		if decodeErr != nil {
			return storygraph.OwnerSnapshot{}, decodeErr
		}
		for sceneIndex, scene := range scenes {
			sceneOwner := structureOwner
			sceneOwner.FragmentKey = "scene/" + scene.ID
			sceneEvidence, sceneEvidenceErr := evidenceRef(revision, scene.SourceStart, scene.SourceEnd)
			if sceneEvidenceErr != nil {
				return storygraph.OwnerSnapshot{}, sceneEvidenceErr
			}
			scenePosition, _ := json.Marshal(struct {
				EpisodePosition int `json:"episode_position"`
				ScenePosition   int `json:"scene_position"`
			}{episode.Position, scene.Position})
			sceneNode, sceneErr := newNode(storygraph.NodeTypeScene, sceneOwner, scene.Heading, scenePosition, []storygraph.EvidenceRef{sceneEvidence}, struct {
				Heading     string `json:"heading"`
				SourceStart int    `json:"source_start"`
				SourceEnd   int    `json:"source_end"`
			}{scene.Heading, scene.SourceStart, scene.SourceEnd})
			if sceneErr != nil {
				return storygraph.OwnerSnapshot{}, sceneErr
			}
			nodes = append(nodes, sceneNode)
			containsScene, containsErr := newEdge(storygraph.EdgeTypeContains, episodeNode.StoryNodeKey, sceneNode.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: fmt.Sprintf("episode/%s/scenes/%08d", episode.ID, sceneIndex+1)})
			if containsErr != nil {
				return storygraph.OwnerSnapshot{}, containsErr
			}
			edges = append(edges, containsScene)
			for dialogueIndex, dialogue := range scene.Dialogues {
				dialogueOwner := structureOwner
				dialogueOwner.FragmentKey = "dialogue/" + dialogue.ID
				dialogueEvidence, dialogueEvidenceErr := evidenceRef(revision, dialogue.SourceStart, dialogue.SourceEnd)
				if dialogueEvidenceErr != nil {
					return storygraph.OwnerSnapshot{}, dialogueEvidenceErr
				}
				dialoguePosition, _ := json.Marshal(struct {
					EpisodePosition  int `json:"episode_position"`
					ScenePosition    int `json:"scene_position"`
					DialoguePosition int `json:"dialogue_position"`
				}{episode.Position, scene.Position, dialogueIndex + 1})
				dialogueNode, dialogueErr := newNode(storygraph.NodeTypeDialogue, dialogueOwner, dialogue.Speaker, dialoguePosition, []storygraph.EvidenceRef{dialogueEvidence}, struct {
					Speaker     string `json:"speaker"`
					Text        string `json:"text"`
					SourceStart int    `json:"source_start"`
					SourceEnd   int    `json:"source_end"`
				}{dialogue.Speaker, dialogue.Text, dialogue.SourceStart, dialogue.SourceEnd})
				if dialogueErr != nil {
					return storygraph.OwnerSnapshot{}, dialogueErr
				}
				nodes = append(nodes, dialogueNode)
				containsDialogue, containsErr := newEdge(storygraph.EdgeTypeContains, sceneNode.StoryNodeKey, dialogueNode.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: fmt.Sprintf("scene/%s/dialogues/%08d", scene.ID, dialogueIndex+1)})
				if containsErr != nil {
					return storygraph.OwnerSnapshot{}, containsErr
				}
				edges = append(edges, containsDialogue)
			}
			for beatIndex, beat := range scene.NarrativeUnits {
				beatOwner := structureOwner
				beatOwner.FragmentKey = "narrative-beat/" + beat.ID
				beatEvidence, beatEvidenceErr := evidenceRef(revision, beat.SourceStart, beat.SourceEnd)
				if beatEvidenceErr != nil {
					return storygraph.OwnerSnapshot{}, beatEvidenceErr
				}
				beatPosition, _ := json.Marshal(struct {
					EpisodePosition int `json:"episode_position"`
					ScenePosition   int `json:"scene_position"`
					BeatPosition    int `json:"beat_position"`
				}{episode.Position, scene.Position, beatIndex + 1})
				beatNode, beatErr := newNode(storygraph.NodeTypeNarrativeBeat, beatOwner, beat.Text, beatPosition, []storygraph.EvidenceRef{beatEvidence}, struct {
					Kind        string `json:"kind"`
					Text        string `json:"text"`
					SourceStart int    `json:"source_start"`
					SourceEnd   int    `json:"source_end"`
				}{beat.Kind, beat.Text, beat.SourceStart, beat.SourceEnd})
				if beatErr != nil {
					return storygraph.OwnerSnapshot{}, beatErr
				}
				nodes = append(nodes, beatNode)
				containsBeat, containsErr := newEdge(storygraph.EdgeTypeContains, sceneNode.StoryNodeKey, beatNode.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: fmt.Sprintf("scene/%s/beats/%08d", scene.ID, beatIndex+1)})
				if containsErr != nil {
					return storygraph.OwnerSnapshot{}, containsErr
				}
				edges = append(edges, containsBeat)
			}
		}
	}
	return storygraph.OwnerSnapshot{
		Origin:      storygraph.OwnerSnapshotOriginConfirmed,
		WorkspaceID: state.WorkspaceID, ProjectID: state.ProjectID,
		SourceRevisionID: revision.ID.String(), SourceRevisionHash: revision.NormalizedHash,
		OwnerHeads: heads,
		Graph:      storygraph.Snapshot{SchemaVersion: storygraph.SchemaVersion, Nodes: nodes, Edges: edges},
	}, nil
}

func (repo *repository) FindReceipt(ctx context.Context, workspaceID, operation, key string) (platformcommand.Receipt, error) {
	id, err := uuid.Parse(workspaceID)
	if err != nil {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	var record model.CommandReceipt
	if err = repo.database.WithContext(ctx).Where("workspace_id = ? AND operation = ? AND idempotency_key = ?", id, operation, key).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
		}
		return platformcommand.Receipt{}, err
	}
	return receiptDomain(record), nil
}

func (repo *repository) GetVersion(ctx context.Context, versionID string) (storygraph.Version, error) {
	id, err := uuid.Parse(versionID)
	if err != nil {
		return storygraph.Version{}, storygraphapp.ErrNotFound
	}
	var record model.StoryGraphVersion
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return storygraph.Version{}, normalizeNotFound(err)
	}
	return versionDomain(record)
}

func (repo *repository) CreateVersion(ctx context.Context, value storygraph.Version) error {
	record, err := versionRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) SwitchHead(ctx context.Context, expected storygraph.PublicationState, version storygraph.Version) (storygraph.Head, error) {
	workspaceID, err := uuid.Parse(version.WorkspaceID)
	if err != nil {
		return storygraph.Head{}, err
	}
	projectID, err := uuid.Parse(version.ProjectID)
	if err != nil {
		return storygraph.Head{}, err
	}
	versionID, err := uuid.Parse(version.ID)
	if err != nil {
		return storygraph.Head{}, err
	}
	head := model.StoryGraphHead{
		WorkspaceID: workspaceID, ProjectID: projectID, CurrentVersionID: versionID,
		CurrentContentHash: version.ContentHash, Revision: version.VersionNo, UpdatedAt: version.PublishedAt,
	}
	if expected.HeadRevision == 0 {
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&head).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return storygraph.Head{}, storygraphapp.ErrHeadCAS
			}
			return storygraph.Head{}, err
		}
	} else {
		currentVersionID, parseErr := uuid.Parse(expected.CurrentVersionID)
		if parseErr != nil {
			return storygraph.Head{}, parseErr
		}
		result := repo.database.WithContext(ctx).Model(&model.StoryGraphHead{}).
			Where("project_id = ? AND revision = ? AND current_version_id = ? AND current_content_hash = ?", projectID, expected.HeadRevision, currentVersionID, expected.CurrentContentHash).
			Updates(map[string]any{"current_version_id": versionID, "current_content_hash": version.ContentHash, "revision": version.VersionNo, "updated_at": version.PublishedAt})
		if result.Error != nil {
			return storygraph.Head{}, result.Error
		}
		if result.RowsAffected != 1 {
			return storygraph.Head{}, storygraphapp.ErrHeadCAS
		}
	}
	return storygraph.Head{
		WorkspaceID: version.WorkspaceID, ProjectID: version.ProjectID,
		CurrentVersionID: version.ID, CurrentContentHash: version.ContentHash,
		Revision: version.VersionNo, UpdatedAt: version.PublishedAt,
	}, nil
}

func (repo *repository) CreateReceipt(ctx context.Context, value platformcommand.Receipt) error {
	record, err := receiptRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) CreateOutbox(ctx context.Context, value storygraph.OutboxEvent) error {
	record, err := outboxRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

type sceneValue struct {
	ID             string               `json:"id"`
	Heading        string               `json:"heading"`
	Position       int                  `json:"position"`
	SourceStart    int                  `json:"source_start"`
	SourceEnd      int                  `json:"source_end"`
	Dialogues      []dialogueValue      `json:"dialogues"`
	NarrativeUnits []narrativeUnitValue `json:"narrative_units"`
	Tasks          []json.RawMessage    `json:"tasks"`
}

type dialogueValue struct {
	ID          string `json:"id"`
	Speaker     string `json:"speaker"`
	Text        string `json:"text"`
	SourceStart int    `json:"source_start"`
	SourceEnd   int    `json:"source_end"`
}

type narrativeUnitValue struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Text        string `json:"text"`
	SourceStart int    `json:"source_start"`
	SourceEnd   int    `json:"source_end"`
}

func decodeScenes(raw []byte) ([]sceneValue, error) {
	var scenes []sceneValue
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&scenes); err != nil || scenes == nil {
		return nil, invalidOwnerSnapshot("confirmed Episode Structure is invalid")
	}
	for index, scene := range scenes {
		if strings.TrimSpace(scene.ID) == "" || scene.Position != index+1 || strings.TrimSpace(scene.Heading) == "" || scene.SourceStart < 0 || scene.SourceEnd <= scene.SourceStart {
			return nil, invalidOwnerSnapshot("confirmed Episode Structure contains an invalid Scene")
		}
		for _, dialogue := range scene.Dialogues {
			if strings.TrimSpace(dialogue.ID) == "" || strings.TrimSpace(dialogue.Speaker) == "" || strings.TrimSpace(dialogue.Text) == "" {
				return nil, invalidOwnerSnapshot("confirmed Episode Structure contains an invalid Dialogue")
			}
		}
		for _, beat := range scene.NarrativeUnits {
			if strings.TrimSpace(beat.ID) == "" || strings.TrimSpace(beat.Kind) == "" || strings.TrimSpace(beat.Text) == "" {
				return nil, invalidOwnerSnapshot("confirmed Episode Structure contains an invalid Narrative Beat")
			}
		}
	}
	return scenes, nil
}

func newNode(nodeType storygraph.NodeType, owner storygraph.OwnerRef, label string, position json.RawMessage, evidence []storygraph.EvidenceRef, payload any) (storygraph.Node, error) {
	key, err := storygraph.DeriveStoryNodeKey(nodeType, owner)
	if err != nil {
		return storygraph.Node{}, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return storygraph.Node{}, err
	}
	return storygraph.Node{
		StoryNodeKey: key, NodeType: nodeType, OwnerRef: owner, Label: label,
		BusinessPosition: position, EvidenceRefs: evidence, Payload: encoded,
	}, nil
}

func newEdge(edgeType storygraph.EdgeType, from, to string, qualifier storygraph.EdgeQualifier) (storygraph.Edge, error) {
	key, err := storygraph.DeriveEdgeKey(edgeType, from, to, qualifier)
	if err != nil {
		return storygraph.Edge{}, err
	}
	return storygraph.Edge{EdgeKey: key, EdgeType: edgeType, FromNodeKey: from, ToNodeKey: to, Qualifier: qualifier}, nil
}

func evidenceRef(revision model.DocumentRevision, start, end int) (storygraph.EvidenceRef, error) {
	runes := []rune(revision.NormalizedText)
	if start < 0 || end <= start || end > len(runes) {
		return storygraph.EvidenceRef{}, invalidOwnerSnapshot("Owner evidence range is outside the source revision")
	}
	hash := sha256.Sum256([]byte(string(runes[start:end])))
	return storygraph.EvidenceRef{
		DocumentRevisionID: revision.ID.String(), AbsoluteStart: start, AbsoluteEnd: end,
		TextHash: hex.EncodeToString(hash[:]),
	}, nil
}

func versionRecord(value storygraph.Version) (model.StoryGraphVersion, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.StoryGraphVersion{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.StoryGraphVersion{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.StoryGraphVersion{}, err
	}
	sourceRevisionID, err := uuid.Parse(value.SourceRevisionID)
	if err != nil {
		return model.StoryGraphVersion{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.StoryGraphVersion{}, err
	}
	var parentVersionID *uuid.UUID
	if value.ParentVersionID != nil {
		parsed, parseErr := uuid.Parse(*value.ParentVersionID)
		if parseErr != nil {
			return model.StoryGraphVersion{}, parseErr
		}
		parentVersionID = &parsed
	}
	ownerHeads, err := json.Marshal(value.OwnerHeads)
	if err != nil {
		return model.StoryGraphVersion{}, err
	}
	nodes, err := json.Marshal(value.Nodes)
	if err != nil {
		return model.StoryGraphVersion{}, err
	}
	edges, err := json.Marshal(value.Edges)
	if err != nil {
		return model.StoryGraphVersion{}, err
	}
	return model.StoryGraphVersion{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, VersionNo: value.VersionNo,
		ParentVersionID: parentVersionID, ParentContentHash: value.ParentContentHash,
		SourceRevisionID: sourceRevisionID, SourceRevisionHash: value.SourceRevisionHash,
		OwnerHeadRefs: ownerHeads, OwnerSetHash: value.OwnerSetHash, SchemaVersion: value.SchemaVersion,
		Nodes: nodes, Edges: edges, TopologyHash: value.TopologyHash, ContentHash: value.ContentHash,
		Status: value.Status, PublishedAt: value.PublishedAt, CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}

func versionDomain(record model.StoryGraphVersion) (storygraph.Version, error) {
	var ownerHeads []storygraph.OwnerHeadRef
	var nodes []storygraph.Node
	var edges []storygraph.Edge
	if err := json.Unmarshal(record.OwnerHeadRefs, &ownerHeads); err != nil {
		return storygraph.Version{}, err
	}
	if err := json.Unmarshal(record.Nodes, &nodes); err != nil {
		return storygraph.Version{}, err
	}
	if err := json.Unmarshal(record.Edges, &edges); err != nil {
		return storygraph.Version{}, err
	}
	var parentVersionID *string
	if record.ParentVersionID != nil {
		value := record.ParentVersionID.String()
		parentVersionID = &value
	}
	return storygraph.Version{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		VersionNo: record.VersionNo, ParentVersionID: parentVersionID, ParentContentHash: record.ParentContentHash,
		SourceRevisionID: record.SourceRevisionID.String(), SourceRevisionHash: record.SourceRevisionHash,
		OwnerHeads: ownerHeads, OwnerSetHash: record.OwnerSetHash, SchemaVersion: record.SchemaVersion,
		Nodes: nodes, Edges: edges, TopologyHash: record.TopologyHash, ContentHash: record.ContentHash,
		Status: record.Status, PublishedAt: record.PublishedAt, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
	}, nil
}

func receiptRecord(value platformcommand.Receipt) (model.CommandReceipt, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.CommandReceipt{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.CommandReceipt{}, err
	}
	resourceID, err := uuid.Parse(value.ResourceID)
	if err != nil {
		return model.CommandReceipt{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.CommandReceipt{}, err
	}
	return model.CommandReceipt{
		ID: id, WorkspaceID: workspaceID, Operation: value.Operation, IdempotencyKey: value.IdempotencyKey,
		InputHash: value.InputHash, ResourceID: resourceID, Result: datatypes.JSON(value.Result),
		CreatedBy: createdBy, CreatedAt: value.CreatedAt,
	}, nil
}

func receiptDomain(record model.CommandReceipt) platformcommand.Receipt {
	return platformcommand.Receipt{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), Operation: record.Operation,
		IdempotencyKey: record.IdempotencyKey, InputHash: record.InputHash, ResourceID: record.ResourceID.String(),
		Result: append([]byte(nil), record.Result...), CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
	}
}

func outboxRecord(value storygraph.OutboxEvent) (model.OutboxEvent, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.OutboxEvent{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.OutboxEvent{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.OutboxEvent{}, err
	}
	receiptID, err := uuid.Parse(value.SourceReceiptID)
	if err != nil {
		return model.OutboxEvent{}, err
	}
	return model.OutboxEvent{
		ID: id, EventType: value.EventType, EventVersion: value.EventVersion,
		WorkspaceID: workspaceID, ProjectID: projectID,
		AggregateKind: value.AggregateKind, AggregateID: value.AggregateID, AggregateRevision: value.AggregateRevision,
		SourceReceiptID: receiptID, Payload: datatypes.JSON(value.Payload), PayloadHash: value.PayloadHash,
		Status: value.Status, Attempts: value.Attempts, OccurredAt: value.OccurredAt, CreatedAt: value.CreatedAt,
	}, nil
}

func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return storygraphapp.ErrNotFound
	}
	return err
}

func unauthenticated() error {
	return &storygraphapp.Error{Code: "unauthenticated", Message: "Authentication is required", Status: 401}
}

func forbidden() error {
	return &storygraphapp.Error{Code: "forbidden", Message: "Action is not allowed", Status: 403}
}

func invalidOwnerSnapshot(message string) error {
	return &storygraphapp.Error{Code: "invalid_owner_snapshot", Message: message, Status: 422}
}
