package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func (repo *repository) projectBible(
	ctx context.Context,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	projection *snapshotProjection,
) error {
	var bible model.ProductionBibleVersion
	err := repo.database.WithContext(ctx).
		Where("workspace_id = ? AND project_id = ? AND document_revision_id = ?", workspaceID, projectID, projection.revision.ID).
		Order("version DESC").Order("id").First(&bible).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if bible.DocumentRevisionHash != projection.revision.NormalizedHash {
		return invalidOwnerSnapshot("Production Bible Version source revision has drifted")
	}
	version, err := bibledomain.NewProductionBibleVersion(bibledomain.ProductionBibleVersionInput{
		ID: bible.ID.String(), WorkspaceID: bible.WorkspaceID.String(), ProjectID: bible.ProjectID.String(),
		DocumentRevisionID: bible.DocumentRevisionID.String(), DocumentRevisionHash: bible.DocumentRevisionHash,
		CandidateRevisionID: bible.CandidateRevisionID.String(), CandidateRevisionNo: bible.CandidateRevisionNo,
		CandidateRevisionHash: bible.CandidateRevisionHash, CandidateContentHash: bible.CandidateContentHash,
		Version: bible.Version, ReviewDecisionID: bible.ReviewDecisionID.String(), Snapshot: json.RawMessage(bible.Snapshot),
		CreatedBy: bible.CreatedBy.String(), CreatedAt: bible.CreatedAt,
	})
	if err != nil || version.ContentHash != bible.ContentHash {
		return invalidOwnerSnapshot("Production Bible Version has drifted")
	}
	var candidate bibledomain.StoryReconciliationCandidate
	if err = json.Unmarshal(version.Snapshot, &candidate); err != nil {
		return invalidOwnerSnapshot("Production Bible Version snapshot is invalid")
	}
	materialization, err := repo.loadMaterialization(ctx, workspaceID, projectID, bible)
	if err != nil {
		return err
	}
	projection.bible = &bible
	entities := make(map[string]bibledomain.StoryEntityCandidate, len(candidate.CanonicalEntities))
	for _, entity := range candidate.CanonicalEntities {
		entities[entity.EntityKey] = entity
	}
	assets, err := repo.loadMaterializedAssets(ctx, workspaceID, projectID, materialization)
	if err != nil {
		return err
	}
	specifications, err := repo.loadMaterializedSpecifications(ctx, workspaceID, projectID, bible, materialization)
	if err != nil {
		return err
	}
	states, err := repo.loadMaterializedStates(ctx, workspaceID, projectID, materialization)
	if err != nil {
		return err
	}
	for _, expected := range materialization.Assets {
		record := assets[expected.ID]
		entity, exists := entities[expected.IdentityKey]
		if !exists || entity.Kind != expected.Kind {
			return invalidOwnerSnapshot("materialized Asset has no exact Production Bible entity")
		}
		evidence, evidenceErr := projection.evidenceRefs(entity.Evidence, nil)
		if evidenceErr != nil {
			return evidenceErr
		}
		owner := storygraph.OwnerRef{
			OwnerKind: "asset", OwnerLogicalID: record.ID.String(), OwnerVersionID: record.ID.String(),
			OwnerRevision: int64(record.Revision), ContentHash: record.ContentHash,
		}
		node, nodeErr := newNode(storygraph.NodeTypeAssetIdentity, owner, entity.CanonicalName, nil, evidence, struct {
			AssetID       string `json:"asset_id"`
			EntityKey     string `json:"entity_key"`
			Kind          string `json:"kind"`
			CanonicalName string `json:"canonical_name"`
		}{record.ID.String(), record.IdentityKey, record.Kind, entity.CanonicalName})
		if nodeErr != nil {
			return nodeErr
		}
		if nodeErr = projection.addNode(node); nodeErr != nil {
			return nodeErr
		}
		if nodeErr = projection.addHead(storygraph.OwnerHeadRefFrom(owner)); nodeErr != nil {
			return nodeErr
		}
		projection.identities[record.ID.String()] = node
		if existing, exists := projection.entityKeys[record.IdentityKey]; exists && existing.StoryNodeKey != node.StoryNodeKey {
			return invalidOwnerSnapshot("Production Bible entity key is ambiguous")
		}
		projection.entityKeys[record.IdentityKey] = node
	}
	if err = projectBibleNarrativeNodes(candidate, bible, projection); err != nil {
		return err
	}
	if err = validateBibleClaimReferences(candidate); err != nil {
		return err
	}
	projection.bibleClaims = append([]bibledomain.StoryClaimCandidate(nil), candidate.MergedClaims...)
	for _, expected := range materialization.Specifications {
		record := specifications[expected.ID]
		identity, exists := projection.identities[record.AssetID.String()]
		entity, entityExists := entities[record.EntityKey]
		if !exists || !entityExists {
			return invalidOwnerSnapshot("materialized Specification has no exact Identity")
		}
		evidence, evidenceErr := projection.evidenceRefs(entity.Evidence, nil)
		if evidenceErr != nil {
			return evidenceErr
		}
		nodeType, nodeTypeErr := specificationNodeType(record.Kind)
		if nodeTypeErr != nil {
			return nodeTypeErr
		}
		owner := storygraph.OwnerRef{
			OwnerKind: "production/bible", OwnerLogicalID: "specification:" + record.EntityKey,
			OwnerVersionID: record.ID.String(), OwnerRevision: int64(record.Version), ContentHash: record.ContentHash,
		}
		node, nodeErr := newNode(nodeType, owner, entity.CanonicalName, nil, evidence, struct {
			SpecificationID string          `json:"specification_id"`
			AssetID         string          `json:"asset_id"`
			EntityKey       string          `json:"entity_key"`
			Kind            string          `json:"kind"`
			Version         int             `json:"version"`
			Snapshot        json.RawMessage `json:"snapshot"`
		}{record.ID.String(), record.AssetID.String(), record.EntityKey, record.Kind, record.Version, json.RawMessage(record.Snapshot)})
		if nodeErr != nil {
			return nodeErr
		}
		if nodeErr = projection.addNode(node); nodeErr != nil {
			return nodeErr
		}
		if nodeErr = projection.addHead(storygraph.OwnerHeadRefFrom(owner)); nodeErr != nil {
			return nodeErr
		}
		if nodeErr = projection.linkEvidence(evidence, node, storygraph.EdgeTypeDerivedFrom); nodeErr != nil {
			return nodeErr
		}
		describes, edgeErr := newEdge(storygraph.EdgeTypeDescribesIdentity, identity.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{})
		if edgeErr != nil {
			return edgeErr
		}
		if edgeErr = projection.addEdge(describes); edgeErr != nil {
			return edgeErr
		}
		projection.specs[record.ID.String()] = node
	}
	for _, expected := range materialization.States {
		record := states[expected.ID]
		identity, exists := projection.identities[record.AssetID.String()]
		if !exists {
			return invalidOwnerSnapshot("materialized State has no exact Identity")
		}
		entity := entities[expectedAssetIdentity(materialization, record.AssetID.String())]
		stateEvidence := entity.Evidence
		for _, candidateState := range entity.States {
			if candidateState.StateKey == record.StateKey {
				stateEvidence = candidateState.Evidence
				break
			}
		}
		evidence, evidenceErr := projection.evidenceRefs(stateEvidence, nil)
		if evidenceErr != nil {
			return evidenceErr
		}
		owner := storygraph.OwnerRef{
			OwnerKind: "asset", OwnerLogicalID: record.AssetID.String() + ":" + record.StateKey,
			OwnerVersionID: record.ID.String(), OwnerRevision: int64(record.Revision), ContentHash: record.ContentHash,
		}
		node, nodeErr := newNode(storygraph.NodeTypeAssetState, owner, record.Label, nil, evidence, struct {
			StateID  string          `json:"state_id"`
			AssetID  string          `json:"asset_id"`
			StateKey string          `json:"state_key"`
			Label    string          `json:"label"`
			Revision int             `json:"revision"`
			Snapshot json.RawMessage `json:"snapshot"`
		}{record.ID.String(), record.AssetID.String(), record.StateKey, record.Label, record.Revision, json.RawMessage(record.Snapshot)})
		if nodeErr != nil {
			return nodeErr
		}
		if nodeErr = projection.addNode(node); nodeErr != nil {
			return nodeErr
		}
		if nodeErr = projection.addHead(storygraph.OwnerHeadRefFrom(owner)); nodeErr != nil {
			return nodeErr
		}
		hasState, edgeErr := newEdge(storygraph.EdgeTypeHasState, identity.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{})
		if edgeErr != nil {
			return edgeErr
		}
		if edgeErr = projection.addEdge(hasState); edgeErr != nil {
			return edgeErr
		}
		projection.states[record.ID.String()] = node
	}
	return repo.projectBindings(ctx, workspaceID, projectID, bible, materialization, states, entities, projection)
}

func projectBibleNarrativeNodes(
	candidate bibledomain.StoryReconciliationCandidate,
	bible model.ProductionBibleVersion,
	projection *snapshotProjection,
) error {
	ownerBase := storygraph.OwnerRef{
		OwnerKind: "production/bible", OwnerLogicalID: bible.ProjectID.String() + ":narrative",
		OwnerVersionID: bible.ID.String(), OwnerRevision: int64(bible.Version), ContentHash: bible.ContentHash,
	}
	for _, entry := range candidate.CanonicalWorldEntries {
		evidence, err := projection.evidenceRefs(entry.Evidence, nil)
		if err != nil {
			return err
		}
		identityKeys := make([]string, len(entry.EntityKeys))
		for index, entityKey := range entry.EntityKeys {
			identity, exists := projection.entityKeys[entityKey]
			if !exists {
				return invalidOwnerSnapshot("Production Bible world entry references an unknown Identity")
			}
			identityKeys[index] = identity.StoryNodeKey
		}
		nodeType := storygraph.NodeTypeWorldRule
		if strings.EqualFold(strings.TrimSpace(entry.Category), "plot_thread") {
			nodeType = storygraph.NodeTypePlotThread
		}
		owner := ownerBase
		owner.FragmentKey = "world/" + entry.EntryKey
		node, err := newNode(nodeType, owner, entry.Title, nil, evidence, struct {
			EntryKey              string   `json:"entry_key"`
			Category              string   `json:"category"`
			Facts                 []string `json:"facts"`
			Rules                 []string `json:"rules"`
			IdentityStoryNodeKeys []string `json:"identity_story_node_keys"`
			EpisodeNumbers        []int    `json:"episode_numbers"`
			Ambiguities           []string `json:"ambiguities"`
		}{entry.EntryKey, entry.Category, entry.Facts, entry.Rules, identityKeys, entry.EpisodeNumbers, entry.Ambiguities})
		if err != nil {
			return err
		}
		if err = projection.addNode(node); err != nil {
			return err
		}
		if err = projection.addHead(storygraph.OwnerHeadRefFrom(owner)); err != nil {
			return err
		}
		if err = projection.linkEvidence(evidence, node, storygraph.EdgeTypeDerivedFrom); err != nil {
			return err
		}
	}
	for _, arc := range candidate.MergedArcs {
		evidence, err := projection.evidenceRefs(arc.Evidence, nil)
		if err != nil {
			return err
		}
		owner := ownerBase
		owner.FragmentKey = "arc/" + arc.ArcKey
		node, err := newNode(storygraph.NodeTypeStoryArc, owner, arc.Title, nil, evidence, struct {
			ArcKey  string `json:"arc_key"`
			Title   string `json:"title"`
			Summary string `json:"summary"`
		}{arc.ArcKey, arc.Title, arc.Summary})
		if err != nil {
			return err
		}
		if err = projection.addNode(node); err != nil {
			return err
		}
		if err = projection.addHead(storygraph.OwnerHeadRefFrom(owner)); err != nil {
			return err
		}
		if err = projection.linkEvidence(evidence, node, storygraph.EdgeTypeDerivedFrom); err != nil {
			return err
		}
	}
	return nil
}

func validateBibleClaimReferences(candidate bibledomain.StoryReconciliationCandidate) error {
	entityKeys := make(map[string]struct{}, len(candidate.CanonicalEntities))
	allKeys := make(map[string]struct{}, len(candidate.CanonicalEntities)+len(candidate.CanonicalWorldEntries)+len(candidate.MergedClaims)+len(candidate.MergedArcs))
	for _, entity := range candidate.CanonicalEntities {
		entityKeys[entity.EntityKey] = struct{}{}
		allKeys[entity.EntityKey] = struct{}{}
	}
	for _, entry := range candidate.CanonicalWorldEntries {
		allKeys[entry.EntryKey] = struct{}{}
	}
	for _, claim := range candidate.MergedClaims {
		allKeys[claim.ClaimKey] = struct{}{}
	}
	for _, arc := range candidate.MergedArcs {
		allKeys[arc.ArcKey] = struct{}{}
	}
	for _, claim := range candidate.MergedClaims {
		seenParticipants := map[string]struct{}{}
		for _, key := range claim.ParticipantKeys {
			if _, exists := entityKeys[key]; !exists {
				return invalidOwnerSnapshot("Production Bible Claim references an unknown candidate Identity")
			}
			if _, duplicate := seenParticipants[key]; duplicate {
				return invalidOwnerSnapshot("Production Bible Claim repeats a candidate participant")
			}
			seenParticipants[key] = struct{}{}
		}
		seenAnchors := map[string]struct{}{}
		for _, key := range claim.AnchorKeys {
			if _, exists := allKeys[key]; !exists {
				return invalidOwnerSnapshot("Production Bible Claim references an unknown candidate anchor")
			}
			if _, duplicate := seenAnchors[key]; duplicate {
				return invalidOwnerSnapshot("Production Bible Claim repeats a candidate anchor")
			}
			seenAnchors[key] = struct{}{}
		}
	}
	return nil
}

func projectBibleClaims(projection *snapshotProjection, projectID uuid.UUID) error {
	if projection.bible == nil || len(projection.bibleClaims) == 0 {
		return nil
	}
	ownerBase := storygraph.OwnerRef{
		OwnerKind: "production/bible", OwnerLogicalID: projectID.String() + ":narrative",
		OwnerVersionID: projection.bible.ID.String(), OwnerRevision: int64(projection.bible.Version),
		ContentHash: projection.bible.ContentHash,
	}
	for _, claim := range projection.bibleClaims {
		nodeType := storygraph.NodeType("")
		switch claim.ClaimType {
		case "relationship":
			nodeType = storygraph.NodeTypeRelationshipClaim
		case "foreshadowing":
			nodeType = storygraph.NodeTypeForeshadowingClaim
		default:
			return invalidOwnerSnapshot("Production Bible Claim type belongs to another Owner")
		}
		if len(claim.ParticipantKeys) < 2 || len(claim.ParticipantKeys) > 3 {
			return invalidOwnerSnapshot("Production Bible Claim requires subject and object participants")
		}
		participants := make([]storygraph.ClaimParticipant, len(claim.ParticipantKeys))
		for index, entityKey := range claim.ParticipantKeys {
			identity, exists := projection.entityKeys[entityKey]
			if !exists {
				return invalidOwnerSnapshot("Production Bible Claim references an unknown Identity")
			}
			role := "participant"
			if index == 0 {
				role = "subject"
			} else if index == 1 {
				role = "object"
			}
			participants[index] = storygraph.ClaimParticipant{Role: role, StoryNodeKey: identity.StoryNodeKey}
		}
		polarity := claim.Polarity
		if polarity == "mixed" || polarity == "unknown" {
			polarity = "neutral"
		}
		status := "asserted"
		if claim.Status == "ambiguous" || claim.Polarity == "unknown" {
			status = "uncertain"
		} else if claim.Status != "proposed" {
			return invalidOwnerSnapshot("Production Bible Claim is conflicted")
		}
		evidence, err := projection.evidenceRefs(claim.Evidence, nil)
		if err != nil {
			return err
		}
		anchorNodes, err := projection.bibleClaimAnchors(evidence)
		if err != nil {
			return err
		}
		anchors := make([]string, len(anchorNodes))
		for index, anchor := range anchorNodes {
			anchors[index] = anchor.StoryNodeKey
		}
		scope, err := bibleClaimScope(claim.Scope, projectID, projection)
		if err != nil {
			return err
		}
		owner := ownerBase
		owner.FragmentKey = "claim/" + claim.ClaimKey
		payload := storygraph.ClaimPayload{
			Predicate: claim.ClaimType, Participants: participants, Anchors: anchors,
			ValidScope: scope, Polarity: polarity, Status: status,
		}
		node, err := newNode(nodeType, owner, claim.ClaimKey, nil, evidence, payload)
		if err != nil {
			return err
		}
		if err = projection.addNode(node); err != nil {
			return err
		}
		if err = projection.addHead(storygraph.OwnerHeadRefFrom(owner)); err != nil {
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
		for index, anchor := range anchorNodes {
			role := "context"
			if index == 0 {
				role = "primary"
			}
			edge, edgeErr := newEdge(storygraph.EdgeTypeClaimAnchor, anchor.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: role})
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

func (projection *snapshotProjection) bibleClaimAnchors(
	evidence []storygraph.EvidenceRef,
) ([]storygraph.Node, error) {
	matchedEvidence := make([]bool, len(evidence))
	anchors := make([]storygraph.Node, 0)
	for _, node := range projection.anchorNodes {
		if node.NodeType != storygraph.NodeTypeScene {
			continue
		}
		matchedNode := false
		for evidenceIndex, claimEvidence := range evidence {
			for _, nodeEvidence := range node.EvidenceRefs {
				if nodeEvidence.DocumentRevisionID == claimEvidence.DocumentRevisionID &&
					nodeEvidence.AbsoluteStart < claimEvidence.AbsoluteEnd && claimEvidence.AbsoluteStart < nodeEvidence.AbsoluteEnd {
					matchedEvidence[evidenceIndex] = true
					matchedNode = true
					break
				}
			}
		}
		if matchedNode {
			anchors = append(anchors, node)
		}
	}
	if len(anchors) == 0 || slices.Contains(matchedEvidence, false) {
		return nil, invalidOwnerSnapshot("Production Bible Claim Evidence has no exact Planning Scene anchor")
	}
	slices.SortFunc(anchors, func(left, right storygraph.Node) int {
		return strings.Compare(left.StoryNodeKey, right.StoryNodeKey)
	})
	return anchors, nil
}

func bibleClaimScope(
	raw string,
	projectID uuid.UUID,
	projection *snapshotProjection,
) (storygraph.ClaimScope, error) {
	if raw == "project" || raw == "project:"+projectID.String() {
		return storygraph.ClaimScope{Kind: "project", OwnerLogicalID: projectID.String()}, nil
	}
	if strings.HasPrefix(raw, "episode:") {
		anchor, exists := projection.anchors[raw]
		if exists && anchor.NodeType == storygraph.NodeTypeEpisode {
			return storygraph.ClaimScope{Kind: "episode", OwnerLogicalID: anchor.OwnerRef.OwnerLogicalID}, nil
		}
	}
	return storygraph.ClaimScope{}, invalidOwnerSnapshot("Production Bible Claim has an invalid scope")
}

func (repo *repository) loadMaterialization(
	ctx context.Context,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	bible model.ProductionBibleVersion,
) (bibledomain.Materialization, error) {
	var receipt model.CommandReceipt
	if err := repo.database.WithContext(ctx).
		Where("workspace_id = ? AND operation = ? AND resource_id = ?", workspaceID, "production_bible.materialize_confirmed", bible.ID).
		Order("created_at DESC").First(&receipt).Error; err != nil {
		return bibledomain.Materialization{}, invalidOwnerSnapshot("Production Bible Version has no exact materialization Receipt")
	}
	var result struct {
		Materialization bibledomain.Materialization `json:"materialization"`
	}
	if err := json.Unmarshal(receipt.Result, &result); err != nil {
		return bibledomain.Materialization{}, invalidOwnerSnapshot("Production Bible materialization Receipt is invalid")
	}
	rebuilt, err := bibledomain.NewMaterialization(
		result.Materialization.BibleVersionID, result.Materialization.BibleVersionHash,
		result.Materialization.Assets, result.Materialization.Specifications,
		result.Materialization.States, result.Materialization.Bindings,
	)
	if err != nil || !reflect.DeepEqual(rebuilt, result.Materialization) || rebuilt.BibleVersionID != bible.ID.String() ||
		rebuilt.BibleVersionHash != bible.ContentHash || receipt.WorkspaceID != workspaceID || receipt.ResourceID != bible.ID {
		return bibledomain.Materialization{}, invalidOwnerSnapshot("Production Bible materialization Receipt has drifted")
	}
	if bible.ProjectID != projectID {
		return bibledomain.Materialization{}, invalidOwnerSnapshot("Production Bible materialization project has drifted")
	}
	return rebuilt, nil
}

func (repo *repository) loadMaterializedAssets(
	ctx context.Context,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	materialization bibledomain.Materialization,
) (map[string]model.Asset, error) {
	result := make(map[string]model.Asset, len(materialization.Assets))
	for _, expected := range materialization.Assets {
		id, err := uuid.Parse(expected.ID)
		if err != nil {
			return nil, invalidOwnerSnapshot("Production Bible materialization contains an invalid Asset ID")
		}
		var record model.Asset
		if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
			return nil, normalizeNotFound(err)
		}
		value, rebuildErr := assetdomain.NewAsset(assetdomain.AssetInput{
			ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
			Kind: record.Kind, IdentityKey: record.IdentityKey, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
		})
		if rebuildErr != nil || value.ContentHash != record.ContentHash || record.WorkspaceID != workspaceID || record.ProjectID != projectID ||
			bibledomain.MaterializedAssetRef(value) != expected {
			return nil, invalidOwnerSnapshot("materialized Asset has drifted")
		}
		result[expected.ID] = record
	}
	return result, nil
}

func (repo *repository) loadMaterializedSpecifications(
	ctx context.Context,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	bible model.ProductionBibleVersion,
	materialization bibledomain.Materialization,
) (map[string]model.ProductionBibleSpecificationVersion, error) {
	result := make(map[string]model.ProductionBibleSpecificationVersion, len(materialization.Specifications))
	for _, expected := range materialization.Specifications {
		id, err := uuid.Parse(expected.ID)
		if err != nil {
			return nil, invalidOwnerSnapshot("Production Bible materialization contains an invalid Specification ID")
		}
		var record model.ProductionBibleSpecificationVersion
		if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
			return nil, normalizeNotFound(err)
		}
		value, rebuildErr := bibledomain.NewSpecificationVersion(bibledomain.SpecificationVersionInput{
			ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
			AssetID: record.AssetID.String(), Kind: record.Kind, EntityKey: record.EntityKey, Version: record.Version,
			SourceBibleVersionID: record.SourceBibleVersionID.String(), Snapshot: json.RawMessage(record.Snapshot),
			CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
		})
		if rebuildErr != nil || value.ContentHash != record.ContentHash || record.WorkspaceID != workspaceID || record.ProjectID != projectID ||
			record.SourceBibleVersionID != bible.ID || bibledomain.MaterializedSpecificationRef(value) != expected {
			return nil, invalidOwnerSnapshot("materialized Specification has drifted")
		}
		result[expected.ID] = record
	}
	return result, nil
}

func (repo *repository) loadMaterializedStates(
	ctx context.Context,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	materialization bibledomain.Materialization,
) (map[string]model.AssetState, error) {
	result := make(map[string]model.AssetState, len(materialization.States))
	for _, expected := range materialization.States {
		id, err := uuid.Parse(expected.ID)
		if err != nil {
			return nil, invalidOwnerSnapshot("Production Bible materialization contains an invalid State ID")
		}
		var record model.AssetState
		if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
			return nil, normalizeNotFound(err)
		}
		value, rebuildErr := assetdomain.NewAssetState(assetdomain.AssetStateInput{
			ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
			AssetID: record.AssetID.String(), StateKey: record.StateKey, Label: record.Label, Revision: record.Revision,
			Snapshot: json.RawMessage(record.Snapshot), CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
		})
		if rebuildErr != nil || value.ContentHash != record.ContentHash || record.WorkspaceID != workspaceID || record.ProjectID != projectID ||
			bibledomain.MaterializedStateRef(value) != expected {
			return nil, invalidOwnerSnapshot("materialized State has drifted")
		}
		result[expected.ID] = record
	}
	return result, nil
}

func (repo *repository) projectBindings(
	ctx context.Context,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	bible model.ProductionBibleVersion,
	materialization bibledomain.Materialization,
	states map[string]model.AssetState,
	entities map[string]bibledomain.StoryEntityCandidate,
	projection *snapshotProjection,
) error {
	statesByAsset := map[string][]bibledomain.MaterializedState{}
	for _, state := range materialization.States {
		statesByAsset[state.AssetID] = append(statesByAsset[state.AssetID], state)
	}
	assetsByID := map[string]bibledomain.MaterializedAsset{}
	for _, asset := range materialization.Assets {
		assetsByID[asset.ID] = asset
	}
	specificationsByID := map[string]bibledomain.MaterializedSpecification{}
	for _, specification := range materialization.Specifications {
		specificationsByID[specification.ID] = specification
	}
	for _, expected := range materialization.Bindings {
		id, err := uuid.Parse(expected.ID)
		if err != nil {
			return invalidOwnerSnapshot("Production Bible materialization contains an invalid Binding ID")
		}
		var record model.ProductionBinding
		if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
			return normalizeNotFound(err)
		}
		var stateLinks []model.ProductionBindingState
		if err = repo.database.WithContext(ctx).
			Where(&model.ProductionBindingState{ProductionBindingID: record.ID}).
			Order("position").Find(&stateLinks).Error; err != nil {
			return err
		}
		linkedStates := make([]bibledomain.MaterializedState, len(stateLinks))
		for index, link := range stateLinks {
			state, exists := states[link.AssetStateID.String()]
			if !exists || link.Position != index+1 || state.AssetID != record.AssetID {
				return invalidOwnerSnapshot("materialized Production Binding State has drifted")
			}
			linkedStates[index] = bibledomain.MaterializedState{
				ID: state.ID.String(), AssetID: state.AssetID.String(), StateKey: state.StateKey,
				Revision: state.Revision, ContentHash: state.ContentHash,
			}
		}
		if !reflect.DeepEqual(linkedStates, statesByAsset[record.AssetID.String()]) {
			return invalidOwnerSnapshot("materialized Production Binding State set has drifted")
		}
		value, rebuildErr := bibledomain.NewProductionBinding(bibledomain.ProductionBindingInput{
			ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
			BibleVersionID: record.BibleVersionID.String(), BibleVersionHash: record.BibleVersionHash,
			EntityKey: record.EntityKey, Asset: assetsByID[record.AssetID.String()],
			Specification: specificationsByID[record.SpecificationVersionID.String()],
			States:        linkedStates, CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
		})
		if rebuildErr != nil || value.ContentHash != record.ContentHash || record.WorkspaceID != workspaceID || record.ProjectID != projectID ||
			record.BibleVersionID != bible.ID || bibledomain.MaterializedBindingRef(value) != expected {
			return invalidOwnerSnapshot("materialized Production Binding has drifted")
		}
		identity, identityExists := projection.identities[record.AssetID.String()]
		specification, specificationExists := projection.specs[record.SpecificationVersionID.String()]
		entity, entityExists := entities[record.EntityKey]
		if !identityExists || !specificationExists || !entityExists {
			return invalidOwnerSnapshot("materialized Production Binding has no exact upstream facts")
		}
		evidence, evidenceErr := projection.evidenceRefs(entity.Evidence, nil)
		if evidenceErr != nil {
			return evidenceErr
		}
		owner := storygraph.OwnerRef{
			OwnerKind: "production/bible", OwnerLogicalID: "binding:" + record.EntityKey,
			OwnerVersionID: record.ID.String(), OwnerRevision: int64(record.Revision), ContentHash: record.ContentHash,
		}
		node, nodeErr := newNode(storygraph.NodeTypeProductionBinding, owner, entity.CanonicalName, nil, evidence, struct {
			BindingID       string   `json:"binding_id"`
			BibleVersionID  string   `json:"bible_version_id"`
			EntityKey       string   `json:"entity_key"`
			AssetID         string   `json:"asset_id"`
			SpecificationID string   `json:"specification_id"`
			StateIDs        []string `json:"state_ids"`
		}{record.ID.String(), record.BibleVersionID.String(), record.EntityKey, record.AssetID.String(), record.SpecificationVersionID.String(), materializedStateIDs(linkedStates)})
		if nodeErr != nil {
			return nodeErr
		}
		if nodeErr = projection.addNode(node); nodeErr != nil {
			return nodeErr
		}
		if nodeErr = projection.addHead(storygraph.OwnerHeadRefFrom(owner)); nodeErr != nil {
			return nodeErr
		}
		for _, input := range []struct {
			Node      storygraph.Node
			Qualifier string
		}{{identity, "asset"}, {specification, "specification"}} {
			edge, edgeErr := newEdge(storygraph.EdgeTypeMaterializes, input.Node.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: input.Qualifier})
			if edgeErr != nil {
				return edgeErr
			}
			if edgeErr = projection.addEdge(edge); edgeErr != nil {
				return edgeErr
			}
		}
		for _, state := range linkedStates {
			stateNode, exists := projection.states[state.ID]
			if !exists {
				return invalidOwnerSnapshot("materialized Production Binding has an unknown State")
			}
			edge, edgeErr := newEdge(storygraph.EdgeTypeMaterializes, stateNode.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "state"})
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

func specificationNodeType(kind string) (storygraph.NodeType, error) {
	switch kind {
	case assetdomain.AssetKindCharacter:
		return storygraph.NodeTypeCharacterSpecification, nil
	case assetdomain.AssetKindLocation:
		return storygraph.NodeTypeLocationSpecification, nil
	case assetdomain.AssetKindProp:
		return storygraph.NodeTypePropSpecification, nil
	default:
		return "", invalidOwnerSnapshot("materialized Specification has an invalid Asset kind")
	}
}

func expectedAssetIdentity(materialization bibledomain.Materialization, assetID string) string {
	for _, asset := range materialization.Assets {
		if asset.ID == assetID {
			return asset.IdentityKey
		}
	}
	return ""
}

func materializedStateIDs(values []bibledomain.MaterializedState) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.ID
	}
	return result
}
