package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	storyboarddomain "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

type Store struct{ database *gorm.DB }
type repository struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) WithinTransaction(ctx context.Context, operation func(application.Repository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) DraftInput(ctx context.Context, actor application.Actor, episodeID string, write bool) (storyboarddomain.DraftInput, error) {
	id, err := uuid.Parse(episodeID)
	if err != nil {
		return storyboarddomain.DraftInput{}, application.ErrNotFound
	}
	var episode model.Episode
	if err = repo.database.WithContext(ctx).First(&episode, "id = ?", id).Error; err != nil {
		return storyboarddomain.DraftInput{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, episode.ProjectID, write); err != nil {
		return storyboarddomain.DraftInput{}, err
	}
	if episode.CurrentScriptVersionID == nil {
		return storyboarddomain.DraftInput{}, conflict("Episode has no published script version")
	}
	var project model.Project
	if err = repo.database.WithContext(ctx).First(&project, "id = ?", episode.ProjectID).Error; err != nil {
		return storyboarddomain.DraftInput{}, normalizeNotFound(err)
	}
	var structure model.EpisodeStructure
	if err = repo.database.WithContext(ctx).
		Where("episode_id = ? AND script_version_id = ? AND status = ?", episode.ID, *episode.CurrentScriptVersionID, "confirmed").
		Order("created_at DESC").First(&structure).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return storyboarddomain.DraftInput{}, conflict("Episode structure must be confirmed before storyboard drafting")
		}
		return storyboarddomain.DraftInput{}, err
	}
	var scenes []domain.Scene
	if err = json.Unmarshal(structure.Scenes, &scenes); err != nil {
		return storyboarddomain.DraftInput{}, err
	}
	var bible model.ProductionBible
	if err = repo.database.WithContext(ctx).Where("project_id = ? AND status = ?", episode.ProjectID, "confirmed").Order("confirmed_at DESC").First(&bible).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return storyboarddomain.DraftInput{}, conflict("Production bible must be confirmed before storyboard drafting")
		}
		return storyboarddomain.DraftInput{}, err
	}
	var candidate struct {
		WorldEntries []map[string]any `json:"world_entries"`
	}
	if err = json.Unmarshal(bible.Candidate, &candidate); err != nil {
		return storyboarddomain.DraftInput{}, err
	}
	units := flattenUnits(scenes)
	if len(units) == 0 {
		return storyboarddomain.DraftInput{}, conflict("Confirmed episode structure contains no narrative units")
	}
	resultHash := ""
	if bible.ResultHash != nil {
		resultHash = *bible.ResultHash
	}
	return storyboarddomain.DraftInput{
		WorkspaceID: bible.WorkspaceID.String(), ProjectID: episode.ProjectID.String(), EpisodeID: episode.ID.String(),
		StructureID: structure.ID.String(), ScriptVersionID: episode.CurrentScriptVersionID.String(),
		StructureResultHash: structure.ResultHash, StructureRevision: structure.Revision,
		BibleID: bible.ID.String(), BibleRevision: bible.Revision, BibleResultHash: resultHash,
		TargetDurationMS: episode.TargetDurationMS, AspectRatio: project.AspectRatio, VisualStyle: project.VisualStyle,
		Units: units, WorldEntries: candidate.WorldEntries,
	}, nil
}

func (repo *repository) StoryGraphDraftInput(
	ctx context.Context,
	actor application.Actor,
	graphVersionID, workflowRunID, nodeRunID string,
	write bool,
) (storyboarddomain.StoryGraphDraftInput, error) {
	graphID, err := uuid.Parse(graphVersionID)
	if err != nil {
		return storyboarddomain.StoryGraphDraftInput{}, application.ErrNotFound
	}
	if _, err = uuid.Parse(workflowRunID); err != nil {
		return storyboarddomain.StoryGraphDraftInput{}, conflict("Storyboard Draft requires a real Workflow Run")
	}
	if _, err = uuid.Parse(nodeRunID); err != nil {
		return storyboarddomain.StoryGraphDraftInput{}, conflict("Storyboard Draft requires a real Node Run")
	}
	var graph model.StoryGraphVersion
	if err = repo.database.WithContext(ctx).First(&graph, "id = ?", graphID).Error; err != nil {
		return storyboarddomain.StoryGraphDraftInput{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, graph.ProjectID, write); err != nil {
		return storyboarddomain.StoryGraphDraftInput{}, err
	}
	if graph.Status != "published" || graph.VersionNo < 1 || len(graph.ContentHash) != 64 {
		return storyboarddomain.StoryGraphDraftInput{}, conflict("Storyboard Draft requires a published StoryGraph Version")
	}
	var project model.Project
	if err = repo.database.WithContext(ctx).First(&project, "id = ?", graph.ProjectID).Error; err != nil {
		return storyboarddomain.StoryGraphDraftInput{}, normalizeNotFound(err)
	}
	if project.VisualStyle == nil || strings.TrimSpace(*project.VisualStyle) == "" ||
		strings.TrimSpace(project.AspectRatio) == "" || project.Revision < 1 {
		return storyboarddomain.StoryGraphDraftInput{}, conflict("Storyboard Draft requires a frozen effective visual style")
	}
	style := contract.StoryboardStyleSnapshotInput{
		OwnerVersionID: project.ID.String(), Revision: int64(project.Revision),
		VisualStyle: strings.TrimSpace(*project.VisualStyle), AspectRatio: project.AspectRatio,
	}
	style.ContentHash, err = contract.CanonicalHash(mustMarshal(struct {
		ProjectID, VisualStyle, AspectRatio string
		Revision                            int
	}{project.ID.String(), style.VisualStyle, style.AspectRatio, project.Revision}))
	if err != nil {
		return storyboarddomain.StoryGraphDraftInput{}, err
	}
	var nodes []storygraph.Node
	var edges []storygraph.Edge
	if err = json.Unmarshal(graph.Nodes, &nodes); err != nil {
		return storyboarddomain.StoryGraphDraftInput{}, err
	}
	if err = json.Unmarshal(graph.Edges, &edges); err != nil {
		return storyboarddomain.StoryGraphDraftInput{}, err
	}
	byKey := make(map[string]storygraph.Node, len(nodes))
	for _, node := range nodes {
		byKey[node.StoryNodeKey] = node
	}
	children := make(map[string][]storygraph.Node)
	episodeForScene := make(map[string]storygraph.Node)
	specificationForIdentity := make(map[string]storygraph.Node)
	for _, edge := range edges {
		from, fromFound := byKey[edge.FromNodeKey]
		to, toFound := byKey[edge.ToNodeKey]
		if !fromFound || !toFound {
			return storyboarddomain.StoryGraphDraftInput{}, conflict("Storyboard Draft StoryGraph has a dangling edge")
		}
		switch edge.EdgeType {
		case storygraph.EdgeTypeContains:
			children[from.StoryNodeKey] = append(children[from.StoryNodeKey], to)
			if from.NodeType == storygraph.NodeTypeEpisode && to.NodeType == storygraph.NodeTypeScene {
				episodeForScene[to.StoryNodeKey] = from
			}
		case storygraph.EdgeTypeAnchorsOccurrence:
			if from.NodeType == storygraph.NodeTypeScene && to.NodeType == storygraph.NodeTypeOccurrence {
				children[from.StoryNodeKey] = append(children[from.StoryNodeKey], to)
			}
		case storygraph.EdgeTypeDescribesIdentity:
			specificationForIdentity[from.StoryNodeKey] = to
		}
	}
	scenes := make([]storyboarddomain.SceneDraftInput, 0)
	for _, sceneNode := range nodes {
		if sceneNode.NodeType != storygraph.NodeTypeScene {
			continue
		}
		episodeNode, found := episodeForScene[sceneNode.StoryNodeKey]
		if !found {
			return storyboarddomain.StoryGraphDraftInput{}, conflict("Storyboard Draft Scene has no exact Episode")
		}
		var scenePayload struct {
			Heading string `json:"heading"`
		}
		var scenePosition struct {
			EpisodePosition int `json:"episode_position"`
			ScenePosition   int `json:"scene_position"`
		}
		var episodePayload struct {
			TargetDurationMS int    `json:"target_duration_ms"`
			ScriptVersionID  string `json:"script_version_id"`
		}
		if json.Unmarshal(sceneNode.Payload, &scenePayload) != nil ||
			json.Unmarshal(sceneNode.BusinessPosition, &scenePosition) != nil ||
			json.Unmarshal(episodeNode.Payload, &episodePayload) != nil {
			return storyboarddomain.StoryGraphDraftInput{}, conflict("Storyboard Draft Scene metadata is invalid")
		}
		stage := contract.StoryboardDraftStageInput{
			GraphVersionNo: graph.VersionNo,
			Scene: contract.StoryboardSceneInput{
				StoryNodeKey: sceneNode.StoryNodeKey, OwnerVersionID: sceneNode.OwnerRef.OwnerVersionID,
				OwnerRevision: sceneNode.OwnerRef.OwnerRevision, OwnerHash: sceneNode.OwnerRef.ContentHash,
				EpisodeID: episodeNode.OwnerRef.OwnerLogicalID, EpisodePosition: scenePosition.EpisodePosition,
				ScenePosition: scenePosition.ScenePosition, Heading: scenePayload.Heading,
				Evidence: storyboardEvidence(sceneNode.EvidenceRefs),
			},
			Beats: []contract.StoryboardBeatInput{}, Dialogues: []contract.StoryboardDialogueInput{},
			Occurrences: []contract.StoryboardOccurrenceInput{}, EffectiveStyleSnapshot: style,
			TargetDurationMS: episodePayload.TargetDurationMS, AssetVersions: []contract.StoryboardAssetVersionInput{},
		}
		for _, child := range children[sceneNode.StoryNodeKey] {
			switch child.NodeType {
			case storygraph.NodeTypeNarrativeBeat:
				var payload struct {
					Text string `json:"text"`
				}
				if json.Unmarshal(child.Payload, &payload) != nil {
					return storyboarddomain.StoryGraphDraftInput{}, conflict("Storyboard Draft Beat payload is invalid")
				}
				stage.Beats = append(stage.Beats, contract.StoryboardBeatInput{
					StoryNodeKey: child.StoryNodeKey, Summary: payload.Text, RequiredForCoverage: true,
					Evidence: storyboardEvidence(child.EvidenceRefs),
				})
			case storygraph.NodeTypeDialogue:
				var payload struct {
					Speaker string `json:"speaker"`
					Text    string `json:"text"`
				}
				if json.Unmarshal(child.Payload, &payload) != nil {
					return storyboarddomain.StoryGraphDraftInput{}, conflict("Storyboard Draft Dialogue payload is invalid")
				}
				stage.Dialogues = append(stage.Dialogues, contract.StoryboardDialogueInput{
					StoryNodeKey: child.StoryNodeKey, Speaker: payload.Speaker, Text: payload.Text,
					Evidence: storyboardEvidence(child.EvidenceRefs),
				})
			case storygraph.NodeTypeOccurrence:
				occurrence, occurrenceErr := storyboardOccurrence(child, byKey, specificationForIdentity)
				if occurrenceErr != nil {
					return storyboarddomain.StoryGraphDraftInput{}, occurrenceErr
				}
				stage.Occurrences = append(stage.Occurrences, occurrence)
			}
		}
		sort.Slice(stage.Beats, func(i, j int) bool { return stage.Beats[i].StoryNodeKey < stage.Beats[j].StoryNodeKey })
		sort.Slice(stage.Dialogues, func(i, j int) bool { return stage.Dialogues[i].StoryNodeKey < stage.Dialogues[j].StoryNodeKey })
		sort.Slice(stage.Occurrences, func(i, j int) bool { return stage.Occurrences[i].StoryNodeKey < stage.Occurrences[j].StoryNodeKey })
		if len(stage.Beats) == 0 || len(stage.Occurrences) == 0 {
			return storyboarddomain.StoryGraphDraftInput{}, conflict("Storyboard Draft Scene requires formal Beats and Occurrences")
		}
		scenes = append(scenes, storyboarddomain.SceneDraftInput{
			EpisodeID: episodeNode.OwnerRef.OwnerLogicalID, StructureID: sceneNode.OwnerRef.OwnerVersionID,
			ScriptVersionID: episodePayload.ScriptVersionID, StageInput: stage,
		})
	}
	if len(scenes) == 0 {
		return storyboarddomain.StoryGraphDraftInput{}, conflict("Storyboard Draft StoryGraph contains no Scenes")
	}
	return storyboarddomain.StoryGraphDraftInput{
		WorkspaceID: graph.WorkspaceID.String(), ProjectID: graph.ProjectID.String(),
		WorkflowRunID: workflowRunID, NodeRunID: nodeRunID,
		GraphVersionID: graph.ID.String(), GraphVersionNo: graph.VersionNo, GraphContentHash: graph.ContentHash,
		EffectiveStyleSnapshot: style, Scenes: scenes,
	}, nil
}

func storyboardOccurrence(
	node storygraph.Node,
	byKey map[string]storygraph.Node,
	specificationForIdentity map[string]storygraph.Node,
) (contract.StoryboardOccurrenceInput, error) {
	var payload struct {
		IdentityStoryNodeKey string `json:"identity_story_node_key"`
		StateStoryNodeKey    string `json:"state_story_node_key"`
		Summary              string `json:"summary"`
	}
	if json.Unmarshal(node.Payload, &payload) != nil {
		return contract.StoryboardOccurrenceInput{}, conflict("Storyboard Draft Occurrence payload is invalid")
	}
	identity, identityFound := byKey[payload.IdentityStoryNodeKey]
	state, stateFound := byKey[payload.StateStoryNodeKey]
	specification, specificationFound := specificationForIdentity[payload.IdentityStoryNodeKey]
	if !identityFound || !stateFound || !specificationFound {
		return contract.StoryboardOccurrenceInput{}, conflict("Storyboard Draft Occurrence has no exact Identity, Specification, or State")
	}
	var identityPayload struct {
		AssetID string `json:"asset_id"`
		Kind    string `json:"kind"`
	}
	var specificationPayload struct {
		SpecificationID string `json:"specification_id"`
		AssetID         string `json:"asset_id"`
		Kind            string `json:"kind"`
	}
	var statePayload struct {
		StateID string `json:"state_id"`
		AssetID string `json:"asset_id"`
	}
	if json.Unmarshal(identity.Payload, &identityPayload) != nil ||
		json.Unmarshal(specification.Payload, &specificationPayload) != nil ||
		json.Unmarshal(state.Payload, &statePayload) != nil ||
		identityPayload.AssetID != specificationPayload.AssetID || identityPayload.AssetID != statePayload.AssetID ||
		identityPayload.Kind != specificationPayload.Kind {
		return contract.StoryboardOccurrenceInput{}, conflict("Storyboard Draft Occurrence formal assets have drifted")
	}
	return contract.StoryboardOccurrenceInput{
		StoryNodeKey: node.StoryNodeKey, IdentityStoryNodeKey: identity.StoryNodeKey,
		SpecificationStoryNodeKey: specification.StoryNodeKey, AssetStateStoryNodeKey: state.StoryNodeKey,
		AssetID: identityPayload.AssetID, SpecificationVersionID: specificationPayload.SpecificationID,
		AssetStateID: statePayload.StateID, AssetKind: identityPayload.Kind, Summary: payload.Summary,
		Evidence: storyboardEvidence(node.EvidenceRefs),
	}, nil
}

func storyboardEvidence(values []storygraph.EvidenceRef) []contract.StoryboardEvidenceRef {
	result := make([]contract.StoryboardEvidenceRef, len(values))
	for index, value := range values {
		result[index] = contract.StoryboardEvidenceRef{
			DocumentRevisionID: value.DocumentRevisionID, AbsoluteStart: value.AbsoluteStart,
			AbsoluteEnd: value.AbsoluteEnd, TextHash: value.TextHash,
		}
	}
	return result
}

func mustMarshal(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func flattenUnits(scenes []domain.Scene) []storyboarddomain.Unit {
	sort.Slice(scenes, func(i, j int) bool { return scenes[i].Position < scenes[j].Position })
	units := make([]storyboarddomain.Unit, 0)
	position := 1
	for _, scene := range scenes {
		units = append(units, storyboarddomain.Unit{ID: scene.ID, SceneID: scene.ID, Kind: "scene_heading", Text: scene.Heading, Position: position, Required: true})
		position++
		type item struct {
			id, kind, text string
			dialogueID     *string
			start          int
		}
		items := make([]item, 0, len(scene.NarrativeUnits)+len(scene.Dialogues))
		for _, unit := range scene.NarrativeUnits {
			items = append(items, item{id: unit.ID, kind: unit.Kind, text: unit.Text, start: unit.SourceStart})
		}
		for _, dialogue := range scene.Dialogues {
			id := dialogue.ID
			items = append(items, item{id: id, kind: "dialogue", text: dialogue.Speaker + ": " + dialogue.Text, dialogueID: &id, start: dialogue.SourceStart})
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].start < items[j].start })
		for _, value := range items {
			units = append(units, storyboarddomain.Unit{ID: value.id, SceneID: scene.ID, Kind: value.kind, Text: value.text, DialogueID: value.dialogueID, Position: position, Required: true})
			position++
		}
	}
	return units
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
	return platformcommand.Receipt{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), Operation: record.Operation, IdempotencyKey: record.IdempotencyKey, InputHash: record.InputHash, ResourceID: record.ResourceID.String(), Result: append([]byte(nil), record.Result...), CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt}, nil
}

func (repo *repository) CreateReceipt(ctx context.Context, value platformcommand.Receipt) error {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return err
	}
	resourceID, err := uuid.Parse(value.ResourceID)
	if err != nil {
		return err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return err
	}
	record := model.CommandReceipt{ID: id, WorkspaceID: workspaceID, Operation: value.Operation, IdempotencyKey: value.IdempotencyKey, InputHash: value.InputHash, ResourceID: resourceID, Result: datatypes.JSON(value.Result), CreatedBy: createdBy, CreatedAt: value.CreatedAt}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return conflict("Idempotency key is already in use")
		}
		return err
	}
	return nil
}

func (repo *repository) CreateWorkflow(ctx context.Context, batch storyboarddomain.Batch, invocation storyboarddomain.Invocation) error {
	return errors.New("standalone Storyboard Draft batches are not supported")
}

func (repo *repository) CreateSetWorkflow(
	ctx context.Context,
	set storyboarddomain.DraftSet,
	manifest storyboarddomain.DraftManifest,
	batches []storyboarddomain.Batch,
	invocations []storyboarddomain.Invocation,
) error {
	if len(batches) == 0 || len(batches) != len(invocations) || len(set.Batches) != len(batches) {
		return errors.New("storyboard draft set workflow is incomplete")
	}
	record, err := setRecord(set)
	if err != nil {
		return err
	}
	manifestRecord, err := draftManifestRecord(manifest, set.CreatedAt)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&manifestRecord).Error; err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return err
	}
	for index := range batches {
		if batches[index].ID != set.Batches[index].BatchID || invocations[index].RequestID != batches[index].ID {
			return errors.New("storyboard draft set workflow references have drifted")
		}
		batchRecord, recordErr := batchRecord(batches[index])
		if recordErr != nil {
			return recordErr
		}
		invocationRecord, recordErr := invocationRecord(invocations[index])
		if recordErr != nil {
			return recordErr
		}
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&batchRecord).Error; err != nil {
			return err
		}
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&invocationRecord).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repo *repository) CreateCandidateSet(
	ctx context.Context,
	set storyboarddomain.DraftSet,
	batches []storyboarddomain.Batch,
	now time.Time,
) (string, string, error) {
	items := make([]storyboarddomain.CandidateSetItem, len(batches))
	leaves := make([]contract.AggregateLeafCandidateRef, len(batches))
	for index, batch := range batches {
		if batch.CandidateRevisionID == nil || batch.CandidateRevisionHash == nil {
			return "", "", errors.New("Storyboard Scene candidate revision is incomplete")
		}
		revisionID, err := uuid.Parse(*batch.CandidateRevisionID)
		if err != nil {
			return "", "", err
		}
		var revision model.StageCandidateRevision
		if err = repo.database.WithContext(ctx).First(&revision, "id = ?", revisionID).Error; err != nil {
			return "", "", err
		}
		var head model.StageCandidateHead
		if err = repo.database.WithContext(ctx).First(&head, "stage_instance_key = ?", revision.StageInstanceKey).Error; err != nil {
			return "", "", err
		}
		if revision.CandidateRevisionHash != *batch.CandidateRevisionHash ||
			head.CurrentRevisionID != revision.ID || head.CurrentCandidateRevisionHash != revision.CandidateRevisionHash {
			return "", "", errors.New("Storyboard Scene candidate revision has drifted")
		}
		items[index] = storyboarddomain.CandidateSetItem{
			SceneStoryNodeKey: batch.SceneStoryNodeKey, ShardKey: "scene:" + batch.SceneStoryNodeKey,
			StageInstanceKey: revision.StageInstanceKey, CandidateRevisionID: revision.ID.String(),
			CandidateRevisionHash: revision.CandidateRevisionHash, AssetReadiness: batch.Candidate.AssetReadiness,
		}
		leaves[index] = contract.AggregateLeafCandidateRef{
			StageInstanceKey: revision.StageInstanceKey, ShardKey: "scene:" + batch.SceneStoryNodeKey,
			CandidateRevisionID: revision.ID.String(), CandidateRevisionHash: revision.CandidateRevisionHash,
		}
	}
	_, candidateJSON, contentHash, stageKey, err := storyboarddomain.BuildCandidateSet(set, items)
	if err != nil {
		return "", "", err
	}
	sort.Slice(leaves, func(i, j int) bool {
		if leaves[i].StageInstanceKey != leaves[j].StageInstanceKey {
			return leaves[i].StageInstanceKey < leaves[j].StageInstanceKey
		}
		return leaves[i].ShardKey < leaves[j].ShardKey
	})
	origin := contract.AggregateCandidateOrigin{
		ShardManifestID: set.ManifestID, ManifestVersion: set.ManifestVersion,
		ShardManifestHash: set.ManifestHash, LeafCandidates: leaves,
	}
	revisionHash, err := (contract.CandidateRevisionMaterial{
		StageInstanceKey: stageKey, RevisionNo: 1, OriginKind: "aggregate",
		AggregateOrigin: &origin, CandidateContentHash: contentHash,
	}).Hash()
	if err != nil {
		return "", "", err
	}
	var existing model.StageCandidateHead
	if err = repo.database.WithContext(ctx).First(&existing, "stage_instance_key = ?", stageKey).Error; err == nil {
		if existing.CurrentCandidateRevisionHash != revisionHash {
			return "", "", agentgorm.ErrCandidateResultConflict
		}
		return existing.CurrentRevisionID.String(), revisionHash, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", err
	}
	originJSON, err := json.Marshal(origin)
	if err != nil {
		return "", "", err
	}
	revision := model.StageCandidateRevision{
		ID: uuid.New(), WorkspaceID: uuid.MustParse(set.WorkspaceID), StageInstanceKey: stageKey,
		RevisionNo: 1, OriginKind: "aggregate", AggregateOrigin: datatypes.JSON(originJSON),
		Candidate: datatypes.JSON(candidateJSON), CandidateContentHash: contentHash,
		CandidateRevisionHash: revisionHash, CreatedAt: now,
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&revision).Error; err != nil {
		return "", "", err
	}
	head := model.StageCandidateHead{
		WorkspaceID: revision.WorkspaceID, StageInstanceKey: stageKey,
		CurrentRevisionID: revision.ID, CurrentCandidateRevisionHash: revisionHash,
		Revision: 1, UpdatedAt: now,
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&head).Error; err != nil {
		return "", "", err
	}
	return revision.ID.String(), revisionHash, nil
}

func draftManifestRecord(value storyboarddomain.DraftManifest, createdAt time.Time) (model.ShardManifest, error) {
	id, err := uuid.Parse(value.ManifestID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	workflowRunID, err := uuid.Parse(value.WorkflowRunID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	nodeRunID, err := uuid.Parse(value.NodeRunID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	shards, err := json.Marshal(value.Shards)
	if err != nil {
		return model.ShardManifest{}, err
	}
	return model.ShardManifest{
		ID: id, Version: value.Version, WorkspaceID: workspaceID,
		WorkflowRunID: workflowRunID, NodeRunID: nodeRunID, Stage: value.Stage,
		RootInputHash: value.RootInputHash, Shards: datatypes.JSON(shards),
		CoverageHash: value.CoverageHash, ManifestHash: value.ManifestHash, CreatedAt: createdAt,
	}, nil
}

func (repo *repository) GetSet(ctx context.Context, actor application.Actor, setID string, forUpdate bool) (storyboarddomain.DraftSet, error) {
	id, err := uuid.Parse(setID)
	if err != nil {
		return storyboarddomain.DraftSet{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", id)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.StoryboardDraftSet
	if err = query.First(&record).Error; err != nil {
		return storyboarddomain.DraftSet{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, forUpdate); err != nil {
		return storyboarddomain.DraftSet{}, err
	}
	return setDomain(record)
}

func (repo *repository) SaveSet(ctx context.Context, value storyboarddomain.DraftSet) error {
	record, err := setRecord(value)
	if err != nil {
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.StoryboardDraftSet{}).Where("id = ?", record.ID).Updates(map[string]any{
		"status": record.Status, "result_hash": record.ResultHash,
		"candidate_revision_id": record.CandidateRevisionID, "candidate_revision_hash": record.CandidateRevisionHash,
		"batches":  record.Batches,
		"revision": record.Revision, "updated_at": record.UpdatedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (repo *repository) GetBatch(ctx context.Context, actor application.Actor, batchID string, forUpdate bool) (storyboarddomain.Batch, error) {
	id, err := uuid.Parse(batchID)
	if err != nil {
		return storyboarddomain.Batch{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", id)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.StoryboardDraftBatch
	if err = query.First(&record).Error; err != nil {
		return storyboarddomain.Batch{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, forUpdate); err != nil {
		return storyboarddomain.Batch{}, err
	}
	return batchDomain(record)
}

func (repo *repository) GetLatestBatch(ctx context.Context, actor application.Actor, episodeID string) (storyboarddomain.Batch, error) {
	id, err := uuid.Parse(episodeID)
	if err != nil {
		return storyboarddomain.Batch{}, application.ErrNotFound
	}
	var episode model.Episode
	if err = repo.database.WithContext(ctx).First(&episode, "id = ?", id).Error; err != nil {
		return storyboarddomain.Batch{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, episode.ProjectID, false); err != nil {
		return storyboarddomain.Batch{}, err
	}
	var record model.StoryboardDraftBatch
	if err = repo.database.WithContext(ctx).Where("episode_id = ?", id).Order("created_at DESC").Order("id DESC").First(&record).Error; err != nil {
		return storyboarddomain.Batch{}, normalizeNotFound(err)
	}
	return batchDomain(record)
}

func (repo *repository) SaveBatch(ctx context.Context, value storyboarddomain.Batch) error {
	record, err := batchRecord(value)
	if err != nil {
		return err
	}
	result := repo.database.WithContext(ctx).Model(&model.StoryboardDraftBatch{}).Where("id = ?", record.ID).Updates(map[string]any{
		"status": record.Status, "result_hash": record.ResultHash,
		"candidate_revision_id": record.CandidateRevisionID, "candidate_revision_hash": record.CandidateRevisionHash,
		"candidate": record.Candidate, "decisions": record.Decisions, "error": record.Error,
		"revision": record.Revision, "approved_by": record.ApprovedBy, "approved_at": record.ApprovedAt,
		"applied_at": record.AppliedAt, "updated_at": record.UpdatedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (repo *repository) LockEpisode(ctx context.Context, actor application.Actor, episodeID string) error {
	id, err := uuid.Parse(episodeID)
	if err != nil {
		return application.ErrNotFound
	}
	var episode model.Episode
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&episode, "id = ?", id).Error; err != nil {
		return normalizeNotFound(err)
	}
	return authorizeProject(ctx, repo.database, actor, episode.ProjectID, true)
}

func (repo *repository) CreateShots(ctx context.Context, batch storyboarddomain.Batch, shots []storyboarddomain.Shot) error {
	record, err := batchRecord(batch)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Model(&model.StoryboardShot{}).Where("episode_id = ? AND status = ?", record.EpisodeID, "active").Updates(map[string]any{"status": "archived", "updated_at": record.UpdatedAt}).Error; err != nil {
		return err
	}
	records := make([]model.StoryboardShot, len(shots))
	for index, shot := range shots {
		records[index], err = shotRecord(shot)
		if err != nil {
			return err
		}
	}
	if len(records) > 0 {
		if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&records).Error; err != nil {
			return err
		}
	}
	return repo.database.WithContext(ctx).Model(&model.StoryboardDraftBatch{}).Where("id = ?", record.ID).Updates(map[string]any{"status": record.Status, "applied_at": record.AppliedAt, "revision": record.Revision, "updated_at": record.UpdatedAt}).Error
}

func (repo *repository) ListShots(ctx context.Context, actor application.Actor, episodeID string) ([]storyboarddomain.Shot, error) {
	id, err := uuid.Parse(episodeID)
	if err != nil {
		return nil, application.ErrNotFound
	}
	var episode model.Episode
	if err = repo.database.WithContext(ctx).First(&episode, "id = ?", id).Error; err != nil {
		return nil, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, episode.ProjectID, false); err != nil {
		return nil, err
	}
	var records []model.StoryboardShot
	if err = repo.database.WithContext(ctx).Where("episode_id = ? AND status = ?", id, "active").Order("position").Order("id").Find(&records).Error; err != nil {
		return nil, err
	}
	values := make([]storyboarddomain.Shot, len(records))
	for index, record := range records {
		values[index], err = shotDomain(record)
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

func (repo *repository) CreateExportSetWorkflow(
	ctx context.Context,
	value storyboarddomain.ExportSet,
	exports []storyboarddomain.Export,
) error {
	record, err := exportSetRecord(value)
	if err != nil {
		return err
	}
	records := make([]model.StoryboardExport, len(exports))
	for index, export := range exports {
		records[index], err = exportRecord(export)
		if err != nil {
			return err
		}
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&records).Error
}

func (repo *repository) GetExportSet(ctx context.Context, actor application.Actor, exportSetID string) (storyboarddomain.ExportSet, error) {
	id, err := uuid.Parse(exportSetID)
	if err != nil {
		return storyboarddomain.ExportSet{}, application.ErrNotFound
	}
	var record model.StoryboardExportSet
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return storyboarddomain.ExportSet{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, false); err != nil {
		return storyboarddomain.ExportSet{}, err
	}
	return exportSetDomain(record)
}

func (repo *repository) CreateExport(ctx context.Context, value storyboarddomain.Export) error {
	record, err := exportRecord(value)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) GetExport(ctx context.Context, actor application.Actor, exportID string) (storyboarddomain.Export, error) {
	id, err := uuid.Parse(exportID)
	if err != nil {
		return storyboarddomain.Export{}, application.ErrNotFound
	}
	var record model.StoryboardExport
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return storyboarddomain.Export{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, record.ProjectID, false); err != nil {
		return storyboarddomain.Export{}, err
	}
	return exportDomain(record)
}

func (repo *repository) GetLatestExport(ctx context.Context, actor application.Actor, episodeID string) (storyboarddomain.Export, error) {
	id, err := uuid.Parse(episodeID)
	if err != nil {
		return storyboarddomain.Export{}, application.ErrNotFound
	}
	var episode model.Episode
	if err = repo.database.WithContext(ctx).First(&episode, "id = ?", id).Error; err != nil {
		return storyboarddomain.Export{}, normalizeNotFound(err)
	}
	if err = authorizeProject(ctx, repo.database, actor, episode.ProjectID, false); err != nil {
		return storyboarddomain.Export{}, err
	}
	var record model.StoryboardExport
	if err = repo.database.WithContext(ctx).Where("episode_id = ?", id).Order("created_at DESC").Order("id DESC").First(&record).Error; err != nil {
		return storyboarddomain.Export{}, normalizeNotFound(err)
	}
	return exportDomain(record)
}

func (store *Store) ClaimNext(ctx context.Context, now, leaseExpiresAt time.Time) (storyboarddomain.Invocation, bool, error) {
	if !leaseExpiresAt.After(now) {
		return storyboarddomain.Invocation{}, false, errors.New("agent invocation lease must expire after claim time")
	}
	var result storyboarddomain.Invocation
	found := false
	err := platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var record model.AgentInvocation
		err := transaction.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("kind = ? AND stage = ?", "storygraph_stage", "draft_storyboard").
			Where("status = ? OR (status = ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?))", "queued", "running", now).
			Order("created_at").First(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = transaction.Model(&record).Updates(map[string]any{"status": "running", "attempts": gorm.Expr("attempts + 1"), "claim_version": gorm.Expr("claim_version + 1"), "lease_expires_at": leaseExpiresAt, "started_at": now, "completed_at": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		if err = transaction.Model(&model.StoryboardDraftBatch{}).Where("id = ?", record.RequestID).Updates(map[string]any{"status": "running", "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		record.Status, record.StartedAt, record.Attempts = "running", &now, record.Attempts+1
		record.ClaimVersion, record.LeaseExpiresAt = record.ClaimVersion+1, &leaseExpiresAt
		result = invocationDomain(record)
		found = true
		return nil
	})
	return result, found, err
}

func (store *Store) CompleteInvocation(ctx context.Context, invocationID string, claimVersion int, result contract.StageResult, candidate storyboarddomain.Candidate, now time.Time) (bool, error) {
	id, err := uuid.Parse(invocationID)
	if err != nil {
		return false, application.ErrNotFound
	}
	executorJSON, err := json.Marshal(result.Executor)
	if err != nil {
		return false, err
	}
	candidateJSON, err := json.Marshal(candidate)
	if err != nil {
		return false, err
	}
	applied := false
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var invocation model.AgentInvocation
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invocation, "id = ?", id).Error; err != nil {
			return normalizeNotFound(err)
		}
		if invocation.Status != "running" || invocation.Kind != "storygraph_stage" || invocation.Stage != "draft_storyboard" || invocation.ClaimVersion != claimVersion || invocation.LeaseExpiresAt == nil || !now.Before(*invocation.LeaseExpiresAt) {
			return nil
		}
		request, err := agentgorm.StageInvocation(invocation)
		if err != nil {
			return err
		}
		revision, err := agentgorm.AcceptInvocationCandidate(transaction, invocation, request, result, now)
		if err != nil {
			return err
		}
		if err = transaction.Model(&invocation).Updates(map[string]any{"status": "succeeded", "result_hash": result.ResultHash, "candidate_type": result.CandidateType, "candidate": datatypes.JSON(result.Candidate), "executor": datatypes.JSON(executorJSON), "error": nil, "lease_expires_at": nil, "completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		batchStatus := "ready"
		if candidate.AssetReadiness == "needs_asset" {
			batchStatus = "needs_asset"
		}
		if err := transaction.Model(&model.StoryboardDraftBatch{}).Where("id = ?", invocation.RequestID).Updates(map[string]any{
			"status": batchStatus, "result_hash": result.ResultHash,
			"candidate_revision_id": revision.ID, "candidate_revision_hash": revision.CandidateRevisionHash,
			"candidate": datatypes.JSON(candidateJSON), "error": nil,
			"revision": gorm.Expr("revision + 1"), "updated_at": now,
		}).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

func (store *Store) FailInvocation(ctx context.Context, invocationID string, claimVersion int, outcome, code, summary string, retryable bool, now time.Time) (bool, error) {
	if outcome != "failed" && outcome != "unknown" {
		outcome = "unknown"
	}
	id, err := uuid.Parse(invocationID)
	if err != nil {
		return false, application.ErrNotFound
	}
	errorJSON, err := json.Marshal(map[string]any{"code": code, "summary": summary, "retryable": retryable})
	if err != nil {
		return false, err
	}
	applied := false
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		var invocation model.AgentInvocation
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invocation, "id = ?", id).Error; err != nil {
			return normalizeNotFound(err)
		}
		if invocation.Status != "running" || invocation.ClaimVersion != claimVersion || invocation.LeaseExpiresAt == nil || !now.Before(*invocation.LeaseExpiresAt) {
			return nil
		}
		if err := transaction.Model(&invocation).Updates(map[string]any{"status": outcome, "error": datatypes.JSON(errorJSON), "lease_expires_at": nil, "completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := transaction.Model(&model.StoryboardDraftBatch{}).Where("id = ?", invocation.RequestID).Updates(map[string]any{"status": outcome, "error": datatypes.JSON(errorJSON), "revision": gorm.Expr("revision + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

func authorizeProject(ctx context.Context, database *gorm.DB, actor application.Actor, projectID uuid.UUID, write bool) error {
	userID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return unauthenticated()
	}
	var user model.UserAccount
	if err = database.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil || user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return unauthenticated()
	}
	var project model.Project
	if err = database.WithContext(ctx).First(&project, "id = ?", projectID).Error; err != nil {
		return application.ErrNotFound
	}
	var workspace model.Workspace
	if err = database.WithContext(ctx).First(&workspace, "id = ?", project.WorkspaceID).Error; err != nil {
		return application.ErrNotFound
	}
	var membership model.Membership
	if err = database.WithContext(ctx).Where("workspace_id = ? AND user_id = ? AND status = ?", project.WorkspaceID, userID, "active").First(&membership).Error; err != nil {
		return application.ErrNotFound
	}
	if write && (membership.Role == "viewer" || workspace.Status != "active" || project.Status != "active") {
		return &application.Error{Code: "forbidden", Message: "Action is not allowed", Status: 403}
	}
	return nil
}

func batchRecord(value storyboarddomain.Batch) (model.StoryboardDraftBatch, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	episodeID, err := uuid.Parse(value.EpisodeID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	structureID, err := uuid.Parse(value.StructureID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	versionID, err := uuid.Parse(value.ScriptVersionID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	workflowRunID, err := uuid.Parse(value.WorkflowRunID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	nodeRunID, err := uuid.Parse(value.NodeRunID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	manifestID, err := uuid.Parse(value.ManifestID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	graphVersionID, err := uuid.Parse(value.GraphVersionID)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	var approvedBy *uuid.UUID
	if value.ApprovedBy != nil {
		parsed, parseErr := uuid.Parse(*value.ApprovedBy)
		if parseErr != nil {
			return model.StoryboardDraftBatch{}, parseErr
		}
		approvedBy = &parsed
	}
	var candidateRevisionID *uuid.UUID
	if value.CandidateRevisionID != nil {
		parsed, parseErr := uuid.Parse(*value.CandidateRevisionID)
		if parseErr != nil {
			return model.StoryboardDraftBatch{}, parseErr
		}
		candidateRevisionID = &parsed
	}
	candidate, err := json.Marshal(value.Candidate)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	decisions, err := json.Marshal(value.Decisions)
	if err != nil {
		return model.StoryboardDraftBatch{}, err
	}
	return model.StoryboardDraftBatch{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID,
		StructureID: structureID, ScriptVersionID: versionID,
		WorkflowRunID: workflowRunID, NodeRunID: nodeRunID,
		ManifestID: manifestID, ManifestVersion: value.ManifestVersion,
		GraphVersionID: graphVersionID, GraphVersionNo: value.GraphVersionNo,
		SceneStoryNodeKey: value.SceneStoryNodeKey, Status: value.Status, InputHash: value.InputHash,
		ResultHash: value.ResultHash, CandidateRevisionID: candidateRevisionID,
		CandidateRevisionHash: value.CandidateRevisionHash,
		Candidate:             datatypes.JSON(candidate), Decisions: datatypes.JSON(decisions), Error: datatypes.JSON(value.Error),
		Revision: value.Revision, ApprovedBy: approvedBy, ApprovedAt: value.ApprovedAt,
		AppliedAt: value.AppliedAt, CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func setRecord(value storyboarddomain.DraftSet) (model.StoryboardDraftSet, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.StoryboardDraftSet{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.StoryboardDraftSet{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.StoryboardDraftSet{}, err
	}
	workflowRunID, err := uuid.Parse(value.WorkflowRunID)
	if err != nil {
		return model.StoryboardDraftSet{}, err
	}
	nodeRunID, err := uuid.Parse(value.NodeRunID)
	if err != nil {
		return model.StoryboardDraftSet{}, err
	}
	graphVersionID, err := uuid.Parse(value.GraphVersionID)
	if err != nil {
		return model.StoryboardDraftSet{}, err
	}
	manifestID, err := uuid.Parse(value.ManifestID)
	if err != nil {
		return model.StoryboardDraftSet{}, err
	}
	var candidateRevisionID *uuid.UUID
	if value.CandidateRevisionID != nil {
		parsed, parseErr := uuid.Parse(*value.CandidateRevisionID)
		if parseErr != nil {
			return model.StoryboardDraftSet{}, parseErr
		}
		candidateRevisionID = &parsed
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.StoryboardDraftSet{}, err
	}
	batches, err := json.Marshal(value.Batches)
	if err != nil {
		return model.StoryboardDraftSet{}, err
	}
	return model.StoryboardDraftSet{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID,
		WorkflowRunID: workflowRunID, NodeRunID: nodeRunID,
		GraphVersionID: graphVersionID, GraphVersionNo: value.GraphVersionNo,
		GraphContentHash: value.GraphContentHash, ManifestID: manifestID,
		ManifestVersion: value.ManifestVersion, ManifestHash: value.ManifestHash,
		Status: value.Status, InputHash: value.InputHash, ResultHash: value.ResultHash,
		CandidateRevisionID: candidateRevisionID, CandidateRevisionHash: value.CandidateRevisionHash,
		Batches:  datatypes.JSON(batches),
		Revision: value.Revision, CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func setDomain(record model.StoryboardDraftSet) (storyboarddomain.DraftSet, error) {
	var batches []storyboarddomain.DraftSetBatch
	if err := json.Unmarshal(record.Batches, &batches); err != nil {
		return storyboarddomain.DraftSet{}, err
	}
	return storyboarddomain.DraftSet{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(),
		GraphVersionID: record.GraphVersionID.String(), GraphVersionNo: record.GraphVersionNo,
		GraphContentHash: record.GraphContentHash, ManifestID: record.ManifestID.String(),
		ManifestVersion: record.ManifestVersion, ManifestHash: record.ManifestHash,
		Status: record.Status, InputHash: record.InputHash, ResultHash: record.ResultHash,
		CandidateRevisionID:   optionalUUIDString(record.CandidateRevisionID),
		CandidateRevisionHash: record.CandidateRevisionHash,
		Batches:               batches, Revision: record.Revision, CreatedBy: record.CreatedBy.String(),
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}, nil
}

func batchDomain(record model.StoryboardDraftBatch) (storyboarddomain.Batch, error) {
	candidate := storyboarddomain.Candidate{Shots: []storyboarddomain.DraftShot{}}
	if len(record.Candidate) > 0 {
		if err := json.Unmarshal(record.Candidate, &candidate); err != nil {
			return storyboarddomain.Batch{}, err
		}
	}
	decisions := map[string]string{}
	if len(record.Decisions) > 0 {
		if err := json.Unmarshal(record.Decisions, &decisions); err != nil {
			return storyboarddomain.Batch{}, err
		}
	}
	var approvedBy *string
	if record.ApprovedBy != nil {
		value := record.ApprovedBy.String()
		approvedBy = &value
	}
	return storyboarddomain.Batch{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		EpisodeID: record.EpisodeID.String(), StructureID: record.StructureID.String(), ScriptVersionID: record.ScriptVersionID.String(),
		WorkflowRunID: record.WorkflowRunID.String(), NodeRunID: record.NodeRunID.String(),
		ManifestID: record.ManifestID.String(), ManifestVersion: record.ManifestVersion,
		GraphVersionID: record.GraphVersionID.String(), GraphVersionNo: record.GraphVersionNo,
		SceneStoryNodeKey: record.SceneStoryNodeKey, Status: record.Status, InputHash: record.InputHash,
		ResultHash: record.ResultHash, CandidateRevisionID: optionalUUIDString(record.CandidateRevisionID),
		CandidateRevisionHash: record.CandidateRevisionHash,
		Candidate:             candidate, Decisions: decisions, Error: append([]byte(nil), record.Error...),
		Revision: record.Revision, ApprovedBy: approvedBy, ApprovedAt: record.ApprovedAt, AppliedAt: record.AppliedAt,
		CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}, nil
}

func optionalUUIDString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	encoded := value.String()
	return &encoded
}

func shotRecord(value storyboarddomain.Shot) (model.StoryboardShot, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	episodeID, err := uuid.Parse(value.EpisodeID)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	batchID, err := uuid.Parse(value.BatchID)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	units, err := json.Marshal(value.NarrativeUnitIDs)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	spec, err := json.Marshal(value.Spec)
	if err != nil {
		return model.StoryboardShot{}, err
	}
	return model.StoryboardShot{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID, BatchID: batchID, ProposalKey: value.ProposalKey, Position: value.Position, Title: value.Title, NarrativeUnitIDs: datatypes.JSON(units), Spec: datatypes.JSON(spec), ContentHash: value.ContentHash, Status: value.Status, Revision: value.Revision, CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}

func shotDomain(record model.StoryboardShot) (storyboarddomain.Shot, error) {
	var units []string
	if err := json.Unmarshal(record.NarrativeUnitIDs, &units); err != nil {
		return storyboarddomain.Shot{}, err
	}
	var spec map[string]any
	if err := json.Unmarshal(record.Spec, &spec); err != nil {
		return storyboarddomain.Shot{}, err
	}
	return storyboarddomain.Shot{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(), EpisodeID: record.EpisodeID.String(), BatchID: record.BatchID.String(), ProposalKey: record.ProposalKey, Title: record.Title, Position: record.Position, Revision: record.Revision, NarrativeUnitIDs: units, Spec: spec, ContentHash: record.ContentHash, Status: record.Status, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}

func exportSetRecord(value storyboarddomain.ExportSet) (model.StoryboardExportSet, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.StoryboardExportSet{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.StoryboardExportSet{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.StoryboardExportSet{}, err
	}
	draftSetID, err := uuid.Parse(value.DraftSetID)
	if err != nil {
		return model.StoryboardExportSet{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.StoryboardExportSet{}, err
	}
	exports, err := json.Marshal(value.Exports)
	if err != nil {
		return model.StoryboardExportSet{}, err
	}
	return model.StoryboardExportSet{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, DraftSetID: draftSetID,
		DraftSetRevision: value.DraftSetRevision, Status: value.Status, InputHash: value.InputHash,
		ContentHash: value.ContentHash, Exports: datatypes.JSON(exports), Revision: value.Revision,
		CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func exportSetDomain(record model.StoryboardExportSet) (storyboarddomain.ExportSet, error) {
	var exports []storyboarddomain.ExportSetReference
	if err := json.Unmarshal(record.Exports, &exports); err != nil {
		return storyboarddomain.ExportSet{}, err
	}
	return storyboarddomain.ExportSet{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
		DraftSetID: record.DraftSetID.String(), DraftSetRevision: record.DraftSetRevision,
		Status: record.Status, InputHash: record.InputHash, ContentHash: record.ContentHash,
		Exports: exports, Revision: record.Revision, CreatedBy: record.CreatedBy.String(),
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}, nil
}

func exportRecord(value storyboarddomain.Export) (model.StoryboardExport, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	episodeID, err := uuid.Parse(value.EpisodeID)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	manifest, err := json.Marshal(value.Manifest)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	files, err := json.Marshal(value.Files)
	if err != nil {
		return model.StoryboardExport{}, err
	}
	var exportSetID *uuid.UUID
	if value.ExportSetID != nil {
		parsed, parseErr := uuid.Parse(*value.ExportSetID)
		if parseErr != nil {
			return model.StoryboardExport{}, parseErr
		}
		exportSetID = &parsed
	}
	return model.StoryboardExport{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, ExportSetID: exportSetID, EpisodeID: episodeID, Status: value.Status, InputHash: value.InputHash, ContentHash: value.ContentHash, Manifest: datatypes.JSON(manifest), Files: datatypes.JSON(files), Package: append([]byte(nil), value.Package...), Revision: value.Revision, CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}

func exportDomain(record model.StoryboardExport) (storyboarddomain.Export, error) {
	var manifest map[string]any
	if err := json.Unmarshal(record.Manifest, &manifest); err != nil {
		return storyboarddomain.Export{}, err
	}
	var files []storyboarddomain.ExportFile
	if err := json.Unmarshal(record.Files, &files); err != nil {
		return storyboarddomain.Export{}, err
	}
	var exportSetID *string
	if record.ExportSetID != nil {
		value := record.ExportSetID.String()
		exportSetID = &value
	}
	return storyboarddomain.Export{ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(), ExportSetID: exportSetID, EpisodeID: record.EpisodeID.String(), Status: record.Status, InputHash: record.InputHash, ContentHash: record.ContentHash, Manifest: manifest, Files: files, Package: append([]byte(nil), record.Package...), Revision: record.Revision, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}

func invocationRecord(value storyboarddomain.Invocation) (model.AgentInvocation, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	requestID, err := uuid.Parse(value.RequestID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	workflowRunID, err := uuid.Parse(value.WorkflowRunID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	nodeRunID, err := uuid.Parse(value.NodeRunID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	manifestID, err := uuid.Parse(value.ManifestID)
	if err != nil {
		return model.AgentInvocation{}, err
	}
	manifestVersion := value.ManifestVersion
	return model.AgentInvocation{
		ID: id, WorkspaceID: workspaceID, WorkflowRunID: &workflowRunID, NodeRunID: &nodeRunID,
		ShardManifestID: &manifestID, ShardManifestVersion: &manifestVersion,
		RequestType: "storyboard_scene_draft", RequestID: requestID,
		Kind: value.Kind, WireSchemaVersion: contract.StoryGraphWireSchemaVersion, Stage: value.Stage,
		ShardKey: value.ShardKey, StageInstanceKey: value.StageInstanceKey, ShardManifestHash: value.ManifestHash,
		InputHash: value.InputHash, ExecutionPolicy: datatypes.JSON(value.ExecutionPolicy), Payload: datatypes.JSON(value.Payload),
		Status: value.Status, Attempts: value.Attempts, ClaimVersion: value.ClaimVersion,
		LeaseExpiresAt: value.LeaseExpiresAt, CreatedAt: value.CreatedAt, UpdatedAt: value.CreatedAt,
	}, nil
}

func invocationDomain(record model.AgentInvocation) storyboarddomain.Invocation {
	result := storyboarddomain.Invocation{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), RequestID: record.RequestID.String(),
		Kind: record.Kind, Stage: record.Stage, ShardKey: record.ShardKey, InputHash: record.InputHash,
		StageInstanceKey: record.StageInstanceKey, ManifestHash: record.ShardManifestHash,
		ExecutionPolicy: append([]byte(nil), record.ExecutionPolicy...), Payload: append([]byte(nil), record.Payload...),
		Status: record.Status, Attempts: record.Attempts, ClaimVersion: record.ClaimVersion,
		LeaseExpiresAt: record.LeaseExpiresAt, CreatedAt: record.CreatedAt,
	}
	if record.WorkflowRunID != nil {
		result.WorkflowRunID = record.WorkflowRunID.String()
	}
	if record.NodeRunID != nil {
		result.NodeRunID = record.NodeRunID.String()
	}
	if record.ShardManifestID != nil {
		result.ManifestID = record.ShardManifestID.String()
	}
	if record.ShardManifestVersion != nil {
		result.ManifestVersion = *record.ShardManifestVersion
	}
	return result
}

func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	return err
}
func unauthenticated() error {
	return &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login"}
}
func conflict(message string) error {
	return &application.Error{Code: "resource_conflict", Message: message, Status: 409}
}
