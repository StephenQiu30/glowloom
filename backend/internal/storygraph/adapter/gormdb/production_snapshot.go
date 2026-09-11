package gormdb

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	planningdomain "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	scriptdomain "github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

type productionSnapshotMaterial struct {
	revision         model.DocumentRevision
	spanIndex        model.SourceSpanIndexVersion
	sourceHead       model.ScriptSourceScopeHead
	sourceReceipt    model.ScriptSourceCollectionReceipt
	structure        model.StructureIdentitySetVersion
	structureReceipt model.StructureIdentityCollectionReceipt
	bibleVersion     model.ProductionWorldBibleVersion
	bibleEvidence    []model.ProductionWorldEvidence
	specifications   []model.ProductionWorldSpecification
	bibleClaims      []model.ProductionWorldClaim
	bindings         []model.ProductionWorldBinding
	bindingStates    []model.ProductionWorldBindingState
	episodes         []model.Episode
	episodeRefs      []bibledomain.EpisodeLifecycleRef
	assets           []model.Asset
	states           []model.AssetState
	planningFacts    []planningdomain.ProductionWorldPlanningFact
	rebaseRevision   int64
}

func (repo *repository) LoadProductionOwnerSnapshot(
	ctx context.Context,
	state storygraph.PublicationState,
	productionWorldReceiptID string,
	productionWorldReceiptHash string,
) (storygraph.ProductionOwnerSnapshot, error) {
	receiptID, err := uuid.Parse(productionWorldReceiptID)
	if err != nil {
		return storygraph.ProductionOwnerSnapshot{}, invalidOwnerSnapshot("invalid Production World receipt identity")
	}
	var commandReceipt model.CommandReceipt
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&commandReceipt, "id = ?", receiptID).Error; err != nil {
		return storygraph.ProductionOwnerSnapshot{}, err
	}
	var confirmation worlddomain.ConfirmProductionWorldResult
	if err = json.Unmarshal(commandReceipt.Result, &confirmation); err != nil {
		return storygraph.ProductionOwnerSnapshot{}, invalidOwnerSnapshot("Production World receipt payload has drifted")
	}
	verified, verifyErr := worlddomain.CompleteConfirmProductionWorldResult(confirmation)
	if verifyErr != nil || !reflect.DeepEqual(verified, confirmation) ||
		commandReceipt.WorkspaceID.String() != state.WorkspaceID || commandReceipt.Operation != worlddomain.ConfirmProductionWorldOperation ||
		confirmation.CommandReceiptID != productionWorldReceiptID || confirmation.ReceiptContentHash != productionWorldReceiptHash {
		return storygraph.ProductionOwnerSnapshot{}, invalidOwnerSnapshot("Production World receipt has drifted")
	}

	collections, err := repo.loadProductionWorldCollections(ctx, state, confirmation)
	if err != nil {
		return storygraph.ProductionOwnerSnapshot{}, err
	}
	material, ownerCollections, err := repo.loadProductionSnapshotMaterial(ctx, state, confirmation, collections)
	if err != nil {
		return storygraph.ProductionOwnerSnapshot{}, err
	}
	graph, err := buildProductionGraph(material, ownerCollections)
	if err != nil {
		return storygraph.ProductionOwnerSnapshot{}, invalidOwnerSnapshot(err.Error())
	}
	return storygraph.ProductionOwnerSnapshot{
		Origin: storygraph.OwnerSnapshotOriginConfirmed, WorkspaceID: state.WorkspaceID, ProjectID: state.ProjectID,
		SourceRevisionID: material.revision.ID.String(), SourceRevisionHash: material.revision.NormalizedHash,
		Coverage: storygraph.ProductionCoverageProof{
			Phase:                        storygraph.ProductionCoverageP0,
			StructureIdentityReceiptID:   material.structureReceipt.ID.String(),
			StructureIdentityReceiptHash: material.structureReceipt.ReceiptContentHash,
			ProductionWorldReceiptID:     productionWorldReceiptID,
			ProductionWorldReceiptHash:   productionWorldReceiptHash,
		},
		OwnerCollections: ownerCollections,
		Graph:            storygraph.Snapshot{SchemaVersion: storygraph.ProductionSchemaID, Nodes: graph.nodes, Edges: graph.edges},
	}, nil
}

func (repo *repository) loadProductionWorldCollections(
	ctx context.Context,
	state storygraph.PublicationState,
	confirmation worlddomain.ConfirmProductionWorldResult,
) ([]worlddomain.CollectionCommitReceipt, error) {
	result := make([]worlddomain.CollectionCommitReceipt, len(confirmation.OrderedCollectionReceiptRefs))
	for index, reference := range confirmation.OrderedCollectionReceiptRefs {
		id, err := uuid.Parse(reference.ID)
		if err != nil {
			return nil, invalidOwnerSnapshot("invalid Production World collection receipt identity")
		}
		var record model.ProductionWorldCollectionReceipt
		if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&record, "id = ?", id).Error; err != nil {
			return nil, err
		}
		var members, committed []worlddomain.CollectionMemberRef
		var scopes []string
		if json.Unmarshal(record.Members, &members) != nil || json.Unmarshal(record.CommittedOwnerVersionRefs, &committed) != nil ||
			json.Unmarshal(record.CoveredScopeKeys, &scopes) != nil {
			return nil, invalidOwnerSnapshot("Production World collection receipt JSON has drifted")
		}
		value, buildErr := worlddomain.NewCollectionCommitReceipt(worlddomain.CollectionCommitReceiptInput{
			ID: record.ID.String(), CommandID: record.CommandID.String(), IdempotencyKey: record.IdempotencyKey,
			WorkspaceID: record.WorkspaceID.String(), ProjectID: record.ProjectID.String(),
			DecisionCheckpointID: record.DecisionCheckpointID.String(), OwnerKind: record.OwnerKind,
			VersionFamily: record.VersionFamily, ScopeKind: record.ScopeKind, ScopeKey: record.ScopeKey,
			ScopeRevision: record.ScopeRevision, ScopeContentHash: record.ScopeContentHash,
			MembersHash: record.MembersHash, CollectionRootHash: record.CollectionRootHash,
			Members: members, CoveredScopeKeys: scopes, CommittedOwnerVersionRefs: committed,
			ReviewDecisionAuditRef: worlddomain.ReviewDecisionAuditRef{ID: record.ReviewDecisionID.String(), Revision: record.ReviewDecisionRevision, ContentHash: record.ReviewDecisionContentHash},
			CommittedAt:            record.CommittedAt, CommittedBy: record.CommittedBy.String(),
		})
		if buildErr != nil || value.ReceiptContentHash != record.ReceiptContentHash ||
			value.WorkspaceID != state.WorkspaceID || value.ProjectID != state.ProjectID ||
			value.CommandID != confirmation.CommandID || reference.OwnerKind != value.OwnerKind ||
			reference.VersionFamily != value.VersionFamily || reference.ScopeKind != value.ScopeKind ||
			reference.ScopeKey != value.ScopeKey || reference.CollectionRootHash != value.CollectionRootHash ||
			reference.ReceiptContentHash != value.ReceiptContentHash {
			return nil, invalidOwnerSnapshot("Production World collection receipt has drifted")
		}
		result[index] = value
	}
	return result, nil
}

func (repo *repository) loadProductionSnapshotMaterial(
	ctx context.Context,
	state storygraph.PublicationState,
	confirmation worlddomain.ConfirmProductionWorldResult,
	worldCollections []worlddomain.CollectionCommitReceipt,
) (productionSnapshotMaterial, []storygraph.OwnerCollectionRef, error) {
	workspaceID, _ := uuid.Parse(state.WorkspaceID)
	projectID, _ := uuid.Parse(state.ProjectID)
	material := productionSnapshotMaterial{}
	bibleCollection := findWorldCollection(worldCollections, "bible_production_world_set")
	if bibleCollection.ID == "" || len(bibleCollection.Members) != 1 {
		return material, nil, invalidOwnerSnapshot("Production World Bible collection is incomplete")
	}
	bibleVersionID, _ := uuid.Parse(bibleCollection.Members[0].VersionID)
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&material.bibleVersion, "id = ?", bibleVersionID).Error; err != nil {
		return material, nil, err
	}
	if material.bibleVersion.WorkspaceID != workspaceID || material.bibleVersion.ProjectID != projectID ||
		material.bibleVersion.ContentHash != bibleCollection.Members[0].ContentHash ||
		material.bibleVersion.Revision != bibleCollection.Members[0].Revision {
		return material, nil, invalidOwnerSnapshot("Production World Bible version has drifted")
	}
	if err := repo.loadProductionBibleFragments(ctx, &material); err != nil {
		return material, nil, err
	}
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&material.structure, "id = ?", material.bibleVersion.StructureIdentitySetVersionID).Error; err != nil {
		return material, nil, err
	}
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where("project_id = ? AND version_id = ?", projectID, material.structure.ID).First(&material.structureReceipt).Error; err != nil {
		return material, nil, err
	}
	if material.structure.WorkspaceID != workspaceID || material.structure.ProjectID != projectID ||
		material.structure.ContentHash != material.bibleVersion.StructureIdentitySetHash ||
		material.structureReceipt.CollectionFamily != bibledomain.StructureIdentityCollectionFamily {
		return material, nil, invalidOwnerSnapshot("Structure Identity proof has drifted")
	}
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&material.revision, "id = ?", material.structure.DocumentRevisionID).Error; err != nil {
		return material, nil, err
	}
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&material.spanIndex, "id = ?", material.structure.SpanIndexID).Error; err != nil {
		return material, nil, err
	}
	if material.revision.WorkspaceID != workspaceID || material.spanIndex.WorkspaceID != workspaceID ||
		material.spanIndex.ProjectID != projectID || material.spanIndex.DocumentRevisionID != material.revision.ID ||
		material.spanIndex.SourceHash != material.revision.NormalizedHash {
		return material, nil, invalidOwnerSnapshot("Script Source proof has drifted")
	}
	if json.Unmarshal(material.structure.EpisodeRefs, &material.episodeRefs) != nil || len(material.episodeRefs) == 0 {
		return material, nil, invalidOwnerSnapshot("Project Episode proof has drifted")
	}
	for _, reference := range material.episodeRefs {
		var episode model.Episode
		if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&episode, "id = ?", reference.EpisodeID).Error; err != nil {
			return material, nil, err
		}
		if episode.WorkspaceID != workspaceID || episode.ProjectID != projectID || episode.Revision != reference.EpisodeRevision || episode.Status != "active" {
			return material, nil, invalidOwnerSnapshot("Project Episode version has drifted")
		}
		material.episodes = append(material.episodes, episode)
	}
	if err := repo.loadProductionAssets(ctx, state, worldCollections, &material); err != nil {
		return material, nil, err
	}
	if err := repo.loadProductionPlanning(ctx, state, worldCollections, &material); err != nil {
		return material, nil, err
	}
	if err := repo.verifyProductionOwnerHeads(ctx, state, worldCollections, &material); err != nil {
		return material, nil, err
	}
	collections, err := buildProductionOwnerCollections(state, material, worldCollections)
	if err != nil {
		return material, nil, err
	}
	_ = confirmation
	return material, collections, nil
}

func (repo *repository) verifyProductionOwnerHeads(
	ctx context.Context,
	state storygraph.PublicationState,
	worldCollections []worlddomain.CollectionCommitReceipt,
	material *productionSnapshotMaterial,
) error {
	workspaceID, _ := uuid.Parse(state.WorkspaceID)
	projectID, _ := uuid.Parse(state.ProjectID)
	locked := func() *gorm.DB {
		return repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"})
	}

	var sourceHead model.ScriptSourceScopeHead
	if err := locked().First(&sourceHead, "project_id = ?", projectID).Error; err != nil {
		return err
	}
	if sourceHead.WorkspaceID != workspaceID || sourceHead.DocumentLogicalID != material.revision.DocumentID ||
		sourceHead.CurrentDocumentRevisionID != material.revision.ID || sourceHead.CurrentSpanIndexID != material.spanIndex.ID {
		return invalidOwnerSnapshot("Script Source Owner Head has advanced beyond the Production World receipt")
	}
	var sourceReceipt model.ScriptSourceCollectionReceipt
	if err := locked().Where(
		"project_id = ? AND document_revision_id = ? AND span_index_id = ? AND head_revision = ?",
		projectID, material.revision.ID, material.spanIndex.ID, sourceHead.HeadRevision,
	).Order("created_at DESC").First(&sourceReceipt).Error; err != nil {
		return err
	}
	var sourceMembers []ownercollection.VersionRef
	if json.Unmarshal(sourceReceipt.Members, &sourceMembers) != nil {
		return invalidOwnerSnapshot("Script Source Collection member set has drifted")
	}
	rebuiltSource, buildErr := scriptdomain.BuildSourceCollectionRef(
		state.WorkspaceID,
		state.ProjectID,
		sourceHead.HeadRevision,
		scriptdomain.SourceVersionIdentity{
			OwnerKind: "production/script", LogicalID: material.revision.DocumentID.String(),
			VersionID: material.revision.ID.String(), Revision: int64(material.revision.VersionNo),
			ContentHash: material.revision.NormalizedHash, CreatedAt: material.revision.CreatedAt,
		},
		scriptdomain.SourceSpanIndex{
			ID: material.spanIndex.ID.String(), WorkspaceID: state.WorkspaceID, ProjectID: state.ProjectID,
			DocumentRevisionID: material.spanIndex.DocumentRevisionID.String(), SourceHash: material.spanIndex.SourceHash,
			ContentHash: material.spanIndex.ContentHash,
		},
	)
	if buildErr != nil || sourceReceipt.WorkspaceID != workspaceID || sourceReceipt.HeadHash != sourceHead.HeadHash ||
		sourceReceipt.MembersHash != rebuiltSource.MembersHash || sourceReceipt.CollectionRootHash != rebuiltSource.CollectionRootHash ||
		!reflect.DeepEqual(sourceMembers, rebuiltSource.Members) {
		return invalidOwnerSnapshot("Script Source Collection proof has drifted")
	}
	material.sourceHead = sourceHead
	material.sourceReceipt = sourceReceipt

	var structureHead model.StructureIdentityScopeHead
	if err := locked().First(&structureHead, "project_id = ?", projectID).Error; err != nil {
		return err
	}
	if structureHead.WorkspaceID != workspaceID || structureHead.CurrentVersionID != material.structure.ID {
		return invalidOwnerSnapshot("Structure Identity Owner Head has advanced beyond the Production World receipt")
	}

	var activeEpisodes []model.Episode
	if err := locked().Where("project_id = ? AND status = ?", projectID, "active").Order("position").Order("id").Find(&activeEpisodes).Error; err != nil {
		return err
	}
	if len(activeEpisodes) != len(material.episodeRefs) {
		return invalidOwnerSnapshot("Project Episode Owner set has advanced beyond the Production World receipt")
	}
	for index, reference := range material.episodeRefs {
		episode := activeEpisodes[index]
		if episode.WorkspaceID != workspaceID || episode.ID.String() != reference.EpisodeID ||
			episode.Position != reference.Position || episode.Revision != reference.EpisodeRevision ||
			episode.CurrentScriptVersionID == nil || episode.CurrentScriptVersionID.String() != reference.ScriptVersionID {
			return invalidOwnerSnapshot("Project Episode Owner set has advanced beyond the Production World receipt")
		}
		var script model.EpisodeScriptVersion
		if err := locked().First(&script, "id = ?", reference.ScriptVersionID).Error; err != nil {
			return err
		}
		if script.WorkspaceID != workspaceID || script.ProjectID != projectID || script.EpisodeID != episode.ID ||
			script.DocumentRevisionID != material.revision.ID || script.VersionNo != reference.ScriptVersion ||
			script.SourceStart != reference.SourceStart || script.SourceEnd != reference.SourceEnd ||
			script.ContentHash != reference.ContentHash || script.Status != "published" {
			return invalidOwnerSnapshot("Project Episode Owner set has advanced beyond the Production World receipt")
		}
	}

	// Match the Production World apply order so publication never inverts Owner Head locks.
	assetCollection := findWorldCollection(worldCollections, "asset_identity_state_set")
	var assetHead model.AssetIdentityStateScopeHead
	if err := locked().First(&assetHead, "project_id = ?", projectID).Error; err != nil {
		return err
	}
	if assetHead.WorkspaceID != workspaceID || assetHead.ScopeRevision != assetCollection.ScopeRevision ||
		assetHead.ScopeContentHash != assetCollection.ScopeContentHash ||
		assetHead.MembersHash != assetCollection.MembersHash || assetHead.CollectionRootHash != assetCollection.CollectionRootHash {
		return invalidOwnerSnapshot("Asset identity-state Owner Head has advanced beyond the Production World receipt")
	}

	bibleCollection := findWorldCollection(worldCollections, bibledomain.BibleProductionWorldFamily)
	var bibleHead model.ProductionWorldBibleScopeHead
	if err := locked().First(&bibleHead, "project_id = ?", projectID).Error; err != nil {
		return err
	}
	if len(bibleCollection.Members) != 1 || bibleHead.WorkspaceID != workspaceID ||
		bibleHead.CurrentVersionID.String() != bibleCollection.Members[0].VersionID ||
		bibleHead.VersionContentHash != bibleCollection.Members[0].ContentHash ||
		bibleHead.ScopeRevision != bibleCollection.ScopeRevision || bibleHead.MemberCount != bibleCollection.MemberCount ||
		bibleHead.ScopeContentHash != bibleCollection.ScopeContentHash || bibleHead.MembersHash != bibleCollection.MembersHash ||
		bibleHead.CollectionRootHash != bibleCollection.CollectionRootHash {
		return invalidOwnerSnapshot("Production World Bible Owner Head has advanced beyond the Production World receipt")
	}

	for _, collection := range worldCollections {
		if collection.VersionFamily != planningdomain.PlanningSceneCollectionFamily {
			continue
		}
		episodeID, found := strings.CutPrefix(collection.ScopeKey, "episode:")
		if !found {
			return invalidOwnerSnapshot("Planning Scene Owner Head has an invalid Episode scope")
		}
		var planningHead model.ProductionWorldPlanningEpisodeHead
		if err := locked().First(&planningHead, "episode_id = ?", episodeID).Error; err != nil {
			return err
		}
		if planningHead.WorkspaceID != workspaceID || planningHead.ProjectID != projectID ||
			planningHead.ScopeRevision != collection.ScopeRevision || planningHead.MemberCount != collection.MemberCount ||
			planningHead.ScopeContentHash != collection.ScopeContentHash || planningHead.MembersHash != collection.MembersHash ||
			planningHead.CollectionRootHash != collection.CollectionRootHash {
			return invalidOwnerSnapshot("Planning Scene Owner Head has advanced beyond the Production World receipt")
		}
	}

	var rebaseHead model.ProductionWorldPlanningRebaseHead
	if err := locked().First(&rebaseHead, "project_id = ?", projectID).Error; err != nil {
		return err
	}
	if rebaseHead.WorkspaceID != workspaceID || rebaseHead.ScopeKey != "project:"+state.ProjectID ||
		rebaseHead.ScopeRevision != 1 || rebaseHead.HeadRevision != 1 || rebaseHead.MemberCount != 0 ||
		string(rebaseHead.CurrentRootRefs) != "[]" {
		return invalidOwnerSnapshot("Planning Structure Rebase Owner Head has advanced beyond the Production World receipt")
	}
	material.rebaseRevision = rebaseHead.ScopeRevision
	return nil
}

func (repo *repository) loadProductionBibleFragments(ctx context.Context, material *productionSnapshotMaterial) error {
	var evidenceRefs, specificationRefs, claimRefs, bindingRefs []bibledomain.ProductionWorldFragmentRef
	if json.Unmarshal(material.bibleVersion.EvidenceRefs, &evidenceRefs) != nil ||
		json.Unmarshal(material.bibleVersion.SpecificationRefs, &specificationRefs) != nil ||
		json.Unmarshal(material.bibleVersion.ClaimRefs, &claimRefs) != nil ||
		json.Unmarshal(material.bibleVersion.BindingRefs, &bindingRefs) != nil {
		return invalidOwnerSnapshot("Production World Bible refs have drifted")
	}
	for _, reference := range evidenceRefs {
		var value model.ProductionWorldEvidence
		if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&value, "id = ?", reference.ID).Error; err != nil {
			return err
		}
		if value.ContentHash != reference.ContentHash || value.Revision != reference.Revision || value.SubjectKey != reference.BusinessKey {
			return invalidOwnerSnapshot("Production World Evidence ref has drifted")
		}
		material.bibleEvidence = append(material.bibleEvidence, value)
	}
	for _, reference := range specificationRefs {
		var value model.ProductionWorldSpecification
		if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&value, "id = ?", reference.ID).Error; err != nil {
			return err
		}
		if value.ContentHash != reference.ContentHash || value.Revision != reference.Revision || value.SpecificationKey != reference.BusinessKey {
			return invalidOwnerSnapshot("Production World Specification ref has drifted")
		}
		material.specifications = append(material.specifications, value)
	}
	for _, reference := range claimRefs {
		if err := repo.loadProductionBibleClaimChain(ctx, material, reference); err != nil {
			return err
		}
	}
	for _, reference := range bindingRefs {
		var value model.ProductionWorldBinding
		if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&value, "id = ?", reference.ID).Error; err != nil {
			return err
		}
		if value.ContentHash != reference.ContentHash || value.Revision != reference.Revision || value.IdentityKey != reference.BusinessKey {
			return invalidOwnerSnapshot("Production World Binding ref has drifted")
		}
		material.bindings = append(material.bindings, value)
	}
	if len(material.bindings) > 0 {
		ids := make([]uuid.UUID, len(material.bindings))
		for index := range material.bindings {
			ids[index] = material.bindings[index].ID
		}
		if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where("binding_id IN ?", ids).Order("binding_id, position").Find(&material.bindingStates).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repo *repository) loadProductionBibleClaimChain(ctx context.Context, material *productionSnapshotMaterial, reference bibledomain.ProductionWorldFragmentRef) error {
	for _, existing := range material.bibleClaims {
		if existing.ID.String() == reference.ID {
			if existing.ContentHash != reference.ContentHash || existing.Revision != reference.Revision || existing.ClaimKey != reference.BusinessKey || existing.ClaimType != reference.Kind {
				return invalidOwnerSnapshot("Production World Claim chain has drifted")
			}
			return nil
		}
	}
	var value model.ProductionWorldClaim
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&value, "id = ?", reference.ID).Error; err != nil {
		return err
	}
	if value.WorkspaceID != material.bibleVersion.WorkspaceID || value.ProjectID != material.bibleVersion.ProjectID ||
		value.ContentHash != reference.ContentHash || value.Revision != reference.Revision || value.ClaimKey != reference.BusinessKey || value.ClaimType != reference.Kind {
		return invalidOwnerSnapshot("Production World Claim ref has drifted")
	}
	var narrative *bibledomain.ProductionWorldNarrativeClaim
	if err := json.Unmarshal(value.Narrative, &narrative); err != nil {
		return invalidOwnerSnapshot("Production World Narrative Claim has drifted")
	}
	material.bibleClaims = append(material.bibleClaims, value)
	if narrative != nil && narrative.SupersedesClaim != nil {
		if narrative.SupersedesClaim.ID == value.ID.String() || narrative.SupersedesClaim.Kind != value.ClaimType ||
			narrative.SupersedesClaim.BusinessKey != value.ClaimKey || narrative.SupersedesClaim.Revision != value.Revision-1 {
			return invalidOwnerSnapshot("Production World Narrative Claim predecessor has drifted")
		}
		if err := repo.loadProductionBibleClaimChain(ctx, material, *narrative.SupersedesClaim); err != nil {
			return err
		}
	}
	for _, existing := range material.bibleEvidence {
		if existing.ID == value.EvidenceID {
			return nil
		}
	}
	var evidence model.ProductionWorldEvidence
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&evidence, "id = ?", value.EvidenceID).Error; err != nil {
		return err
	}
	if evidence.WorkspaceID != value.WorkspaceID || evidence.ProjectID != value.ProjectID || evidence.ContentHash != value.EvidenceHash || evidence.SubjectKey != value.ClaimKey {
		return invalidOwnerSnapshot("Production World Claim Evidence has drifted")
	}
	material.bibleEvidence = append(material.bibleEvidence, evidence)
	return nil
}

func (repo *repository) loadProductionAssets(
	ctx context.Context,
	state storygraph.PublicationState,
	worldCollections []worlddomain.CollectionCommitReceipt,
	material *productionSnapshotMaterial,
) error {
	collection := findWorldCollection(worldCollections, "asset_identity_state_set")
	if collection.ID == "" {
		return invalidOwnerSnapshot("Asset identity-state collection is missing")
	}
	for _, reference := range collection.Members {
		switch {
		case strings.HasPrefix(reference.LogicalID, "state_"):
			var value model.AssetState
			if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&value, "id = ?", reference.VersionID).Error; err != nil {
				return err
			}
			if reference.OwnerKind != "asset" || value.ContentHash != reference.ContentHash || int64(value.Revision) != reference.Revision || value.StateKey != reference.LogicalID {
				return invalidOwnerSnapshot("AssetState collection member has drifted")
			}
			material.states = append(material.states, value)
		default:
			var value model.Asset
			if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&value, "id = ?", reference.VersionID).Error; err != nil {
				return err
			}
			if reference.OwnerKind != "asset" || value.ContentHash != reference.ContentHash || int64(value.Revision) != reference.Revision || value.IdentityKey != reference.LogicalID {
				return invalidOwnerSnapshot("Asset collection member has drifted")
			}
			material.assets = append(material.assets, value)
		}
	}
	if len(material.assets) == 0 || len(material.states) == 0 {
		return invalidOwnerSnapshot("Asset identity-state collection is incomplete")
	}
	_ = state
	return nil
}

func (repo *repository) loadProductionPlanning(
	ctx context.Context,
	state storygraph.PublicationState,
	worldCollections []worlddomain.CollectionCommitReceipt,
	material *productionSnapshotMaterial,
) error {
	for _, collection := range worldCollections {
		if collection.VersionFamily != planningdomain.PlanningSceneCollectionFamily {
			continue
		}
		for _, reference := range collection.Members {
			fact, err := repo.loadExactPlanningFact(ctx, reference)
			if err != nil {
				return err
			}
			if fact.WorkspaceID != state.WorkspaceID || fact.ProjectID != state.ProjectID {
				return invalidOwnerSnapshot("Planning collection member crosses Project scope")
			}
			material.planningFacts = append(material.planningFacts, fact)
		}
	}
	if len(material.planningFacts) == 0 {
		return invalidOwnerSnapshot("Planning Scene collections are incomplete")
	}
	return nil
}

func (repo *repository) loadExactPlanningFact(ctx context.Context, reference worlddomain.CollectionMemberRef) (planningdomain.ProductionWorldPlanningFact, error) {
	id, err := uuid.Parse(reference.VersionID)
	if err != nil || reference.OwnerKind != "production/planning" {
		return planningdomain.ProductionWorldPlanningFact{}, invalidOwnerSnapshot("invalid Planning Owner Version")
	}
	load := func(value any) error {
		return repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(value, "id = ?", id).Error
	}
	var fact planningdomain.ProductionWorldPlanningFact
	switch {
	case strings.HasPrefix(reference.LogicalID, "scene:"):
		var value model.ProductionWorldPlanningScene
		if err = load(&value); err == nil {
			fact = planningFact(value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, "scene", value.BusinessKey, value.Revision, value.Payload, value.ContentHash, value.CreatedBy, value.CreatedAt)
		}
	case strings.HasPrefix(reference.LogicalID, "dialogue:"):
		var value model.ProductionWorldPlanningDialogue
		if err = load(&value); err == nil {
			fact = planningFact(value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, "dialogue", value.BusinessKey, value.Revision, value.Payload, value.ContentHash, value.CreatedBy, value.CreatedAt)
		}
	case strings.HasPrefix(reference.LogicalID, "beat:"):
		var value model.ProductionWorldPlanningBeat
		if err = load(&value); err == nil {
			fact = planningFact(value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, "narrative_beat", value.BusinessKey, value.Revision, value.Payload, value.ContentHash, value.CreatedBy, value.CreatedAt)
		}
	case strings.HasPrefix(reference.LogicalID, "occurrence:"):
		var value model.ProductionWorldPlanningOccurrence
		if err = load(&value); err == nil {
			fact = planningFact(value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, "occurrence", value.BusinessKey, value.Revision, value.Payload, value.ContentHash, value.CreatedBy, value.CreatedAt)
		}
	case strings.HasPrefix(reference.LogicalID, "interaction:"), strings.HasPrefix(reference.LogicalID, "continuity:"):
		var value model.ProductionWorldPlanningClaim
		if err = load(&value); err == nil {
			fact = planningFact(value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, "continuity_claim", value.BusinessKey, value.Revision, value.Payload, value.ContentHash, value.CreatedBy, value.CreatedAt)
		}
	default:
		return planningdomain.ProductionWorldPlanningFact{}, invalidOwnerSnapshot("unknown Planning Owner Version kind")
	}
	if err != nil {
		return planningdomain.ProductionWorldPlanningFact{}, err
	}
	if fact.BusinessKey != reference.LogicalID || int64(fact.Revision) != reference.Revision || fact.ContentHash != reference.ContentHash ||
		planningdomain.ValidateProductionWorldPlanningFact(fact) != nil {
		return planningdomain.ProductionWorldPlanningFact{}, invalidOwnerSnapshot("Planning Owner Version has drifted")
	}
	return fact, nil
}

func planningFact(id, workspaceID, projectID, episodeID uuid.UUID, kind, key string, revision int, payload []byte, contentHash string, createdBy uuid.UUID, createdAt time.Time) planningdomain.ProductionWorldPlanningFact {
	return planningdomain.ProductionWorldPlanningFact{
		ID: id.String(), WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), EpisodeID: episodeID.String(),
		Kind: kind, BusinessKey: key, Revision: revision, Payload: append([]byte(nil), payload...), ContentHash: contentHash,
		CreatedBy: createdBy.String(), CreatedAt: createdAt.UTC(),
	}
}

func findWorldCollection(values []worlddomain.CollectionCommitReceipt, family string) worlddomain.CollectionCommitReceipt {
	for _, value := range values {
		if value.VersionFamily == family {
			return value
		}
	}
	return worlddomain.CollectionCommitReceipt{}
}

func buildProductionOwnerCollections(
	state storygraph.PublicationState,
	material productionSnapshotMaterial,
	worldCollections []worlddomain.CollectionCommitReceipt,
) ([]storygraph.OwnerCollectionRef, error) {
	identity := func(ownerKind, family, logicalID, versionID string, revision int64, hash string, createdAt time.Time) storygraph.OwnerVersionIdentity {
		return storygraph.OwnerVersionIdentity{WorkspaceID: state.WorkspaceID, ProjectID: state.ProjectID, OwnerKind: ownerKind,
			VersionFamily: family, LogicalID: logicalID, VersionID: versionID, Revision: revision, ContentHash: hash, CreatedAt: createdAt.UTC()}
	}
	collections := make([]storygraph.OwnerCollectionRef, 0, 7)
	appendCollection := func(ownerKind, family, scopeKind, scopeKey string, revision int64, members []storygraph.OwnerVersionIdentity) error {
		value, err := storygraph.BuildOwnerCollectionRef(storygraph.OwnerCollectionRef{WorkspaceID: state.WorkspaceID, ProjectID: state.ProjectID,
			OwnerKind: ownerKind, VersionFamily: family, ScopeKind: scopeKind, ScopeKey: scopeKey, ScopeRevision: revision, Members: members})
		if err == nil {
			collections = append(collections, value)
		}
		return err
	}
	sourceMembers := []storygraph.OwnerVersionIdentity{
		identity("production/script", scriptdomain.SourceCollectionFamily, material.revision.DocumentID.String(), material.revision.ID.String(), int64(material.revision.VersionNo), material.revision.NormalizedHash, material.revision.CreatedAt),
		identity("production/script", scriptdomain.SourceCollectionFamily, material.revision.DocumentID.String()+":span-index", material.spanIndex.ID.String(), int64(material.revision.VersionNo), material.spanIndex.ContentHash, material.spanIndex.CreatedAt),
	}
	if err := appendCollection("production/script", scriptdomain.SourceCollectionFamily, "project", "project:"+state.ProjectID, material.sourceHead.HeadRevision, sourceMembers); err != nil {
		return nil, err
	}
	if collections[len(collections)-1].CollectionRootHash != material.sourceReceipt.CollectionRootHash {
		return nil, invalidOwnerSnapshot("Script Source Collection root has drifted")
	}
	episodeMembers := make([]storygraph.OwnerVersionIdentity, len(material.episodeRefs))
	for index, reference := range material.episodeRefs {
		episodeMembers[index] = identity("production/project", "project_episode_set", reference.EpisodeID, reference.EpisodeID, int64(reference.EpisodeRevision), reference.ContentHash, material.episodes[index].CreatedAt)
	}
	if err := appendCollection("production/project", "project_episode_set", "project", "project:"+state.ProjectID, int64(material.structure.Version), episodeMembers); err != nil {
		return nil, err
	}
	if err := appendCollection("production/bible", "bible_structure_identity_set", "project", "project:"+state.ProjectID, int64(material.structure.Version), []storygraph.OwnerVersionIdentity{
		identity("production/bible", "bible_structure_identity_set", state.ProjectID, material.structure.ID.String(), int64(material.structure.Version), material.structure.ContentHash, material.structure.CreatedAt),
	}); err != nil {
		return nil, err
	}
	bibleCollection := findWorldCollection(worldCollections, "bible_production_world_set")
	if err := appendCollection("production/bible", "bible_production_world_set", "project", "project:"+state.ProjectID, bibleCollection.ScopeRevision, []storygraph.OwnerVersionIdentity{
		identity("production/bible", "bible_production_world_set", state.ProjectID, material.bibleVersion.ID.String(), material.bibleVersion.Revision, material.bibleVersion.ContentHash, material.bibleVersion.CreatedAt),
	}); err != nil {
		return nil, err
	}
	assetCollection := findWorldCollection(worldCollections, "asset_identity_state_set")
	assetMembers := make([]storygraph.OwnerVersionIdentity, 0, len(material.assets)+len(material.states))
	for _, value := range material.assets {
		assetMembers = append(assetMembers, identity("asset", "asset_identity_state_set", value.IdentityKey, value.ID.String(), int64(value.Revision), value.ContentHash, value.CreatedAt))
	}
	for _, value := range material.states {
		assetMembers = append(assetMembers, identity("asset", "asset_identity_state_set", value.StateKey, value.ID.String(), int64(value.Revision), value.ContentHash, value.CreatedAt))
	}
	if err := appendCollection("asset", "asset_identity_state_set", "project", "project:"+state.ProjectID, assetCollection.ScopeRevision, assetMembers); err != nil {
		return nil, err
	}
	for _, worldCollection := range worldCollections {
		if worldCollection.VersionFamily != "planning_scene_set" {
			continue
		}
		members := make([]storygraph.OwnerVersionIdentity, 0, len(worldCollection.Members))
		for _, reference := range worldCollection.Members {
			for _, fact := range material.planningFacts {
				if fact.ID == reference.VersionID {
					members = append(members, identity("production/planning", "planning_scene_set", fact.BusinessKey, fact.ID, int64(fact.Revision), fact.ContentHash, fact.CreatedAt))
					break
				}
			}
		}
		if len(members) != len(worldCollection.Members) {
			return nil, invalidOwnerSnapshot("Planning collection coverage has drifted")
		}
		if err := appendCollection("production/planning", "planning_scene_set", "episode", worldCollection.ScopeKey, worldCollection.ScopeRevision, members); err != nil {
			return nil, err
		}
	}
	if err := appendCollection("production/planning", "planning_structure_rebase_set", "project", "project:"+state.ProjectID, material.rebaseRevision, []storygraph.OwnerVersionIdentity{}); err != nil {
		return nil, err
	}
	slices.SortFunc(collections, func(left, right storygraph.OwnerCollectionRef) int {
		return strings.Compare(left.OwnerKind+"\x00"+left.VersionFamily+"\x00"+left.ScopeKey, right.OwnerKind+"\x00"+right.VersionFamily+"\x00"+right.ScopeKey)
	})
	return collections, nil
}

var _ storygraphapp.Repository = (*repository)(nil)
