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
	"gorm.io/gorm/clause"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

func (store *Store) WithinEpisodePlanningTransaction(
	ctx context.Context,
	operation func(application.EpisodePlanningRepository) error,
) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) GetEpisodePlanningCandidate(
	ctx context.Context,
	actor application.Actor,
	candidateRevisionID string,
	forUpdate bool,
) (application.EpisodePlanningCandidateSource, error) {
	candidateID, err := uuid.Parse(candidateRevisionID)
	if err != nil {
		return application.EpisodePlanningCandidateSource{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", candidateID)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var revision model.StageCandidateRevision
	if err = query.First(&revision).Error; err != nil {
		return application.EpisodePlanningCandidateSource{}, normalizeNotFound(err)
	}
	var head model.StageCandidateHead
	headQuery := repo.database.WithContext(ctx).
		Where("workspace_id = ? AND stage_instance_key = ?", revision.WorkspaceID, revision.StageInstanceKey)
	if forUpdate {
		headQuery = headQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err = headQuery.First(&head).Error; err != nil {
		return application.EpisodePlanningCandidateSource{}, normalizeNotFound(err)
	}
	if head.CurrentRevisionID != revision.ID || head.CurrentCandidateRevisionHash != revision.CandidateRevisionHash ||
		head.Revision != revision.RevisionNo || revision.OriginKind != "aggregate" ||
		len(revision.AggregateOrigin) == 0 || revision.SourceInvocationID != nil || revision.SourceResultHash != nil {
		return application.EpisodePlanningCandidateSource{}, planningCandidateConflict("Episode Planning Candidate head changed before approval")
	}
	var candidate domain.EpisodePlanningCandidateSet
	if err = json.Unmarshal(revision.Candidate, &candidate); err != nil {
		return application.EpisodePlanningCandidateSource{}, planningCandidateConflict("Episode Planning Candidate schema is invalid")
	}
	contentHash, err := domain.EpisodePlanningCandidateSetContentHash(candidate)
	if err != nil || contentHash != revision.CandidateContentHash {
		return application.EpisodePlanningCandidateSource{}, planningCandidateConflict("Episode Planning Candidate content has drifted")
	}
	var origin agentcontract.AggregateCandidateOrigin
	if err = json.Unmarshal(revision.AggregateOrigin, &origin); err != nil {
		return application.EpisodePlanningCandidateSource{}, planningCandidateConflict("Episode Planning Candidate origin is invalid")
	}
	revisionHash, err := (agentcontract.CandidateRevisionMaterial{
		StageInstanceKey: revision.StageInstanceKey, RevisionNo: revision.RevisionNo,
		OriginKind: "aggregate", AggregateOrigin: &origin, CandidateContentHash: revision.CandidateContentHash,
	}).Hash()
	if err != nil || revisionHash != revision.CandidateRevisionHash {
		return application.EpisodePlanningCandidateSource{}, planningCandidateConflict("Episode Planning Candidate revision has drifted")
	}
	if err = verifyEpisodePlanningLeaves(repo.database.WithContext(ctx), revision.WorkspaceID, origin, forUpdate); err != nil {
		return application.EpisodePlanningCandidateSource{}, err
	}
	bibleID, err := uuid.Parse(candidate.BibleVersionID)
	if err != nil {
		return application.EpisodePlanningCandidateSource{}, planningCandidateConflict("Episode Planning Candidate Bible reference is invalid")
	}
	var bible model.ProductionBibleVersion
	if err = repo.database.WithContext(ctx).First(&bible, "id = ?", bibleID).Error; err != nil {
		return application.EpisodePlanningCandidateSource{}, normalizeNotFound(err)
	}
	if bible.WorkspaceID != revision.WorkspaceID || bible.Version != candidate.BibleVersion ||
		bible.ContentHash != candidate.BibleContentHash {
		return application.EpisodePlanningCandidateSource{}, planningCandidateConflict("Episode Planning Candidate Bible version has drifted")
	}
	if err = authorizeProject(ctx, repo.database, actor, bible.ProjectID, forUpdate); err != nil {
		return application.EpisodePlanningCandidateSource{}, err
	}
	materialization, identities, err := loadEpisodePlanningIdentities(
		ctx, repo.database, revision.WorkspaceID, bible.ProjectID, bible,
	)
	if err != nil {
		return application.EpisodePlanningCandidateSource{}, err
	}
	if materialization.ContentHash != candidate.MaterializationHash {
		return application.EpisodePlanningCandidateSource{}, planningCandidateConflict("Episode Planning Candidate materialization has drifted")
	}
	episodes := make([]application.PlanningEpisodeSource, len(candidate.Episodes))
	for index, root := range candidate.Episodes {
		episodeID, parseErr := uuid.Parse(root.EpisodeID)
		if parseErr != nil {
			return application.EpisodePlanningCandidateSource{}, planningCandidateConflict("Episode Planning Candidate Episode reference is invalid")
		}
		versionID, parseErr := uuid.Parse(root.ScriptVersionID)
		if parseErr != nil {
			return application.EpisodePlanningCandidateSource{}, planningCandidateConflict("Episode Planning Candidate ScriptVersion reference is invalid")
		}
		var episode model.Episode
		if err = repo.database.WithContext(ctx).First(&episode, "id = ?", episodeID).Error; err != nil {
			return application.EpisodePlanningCandidateSource{}, normalizeNotFound(err)
		}
		var version model.EpisodeScriptVersion
		if err = repo.database.WithContext(ctx).First(&version, "id = ?", versionID).Error; err != nil {
			return application.EpisodePlanningCandidateSource{}, normalizeNotFound(err)
		}
		if episode.WorkspaceID != revision.WorkspaceID || episode.ProjectID != bible.ProjectID ||
			episode.Status != "active" || episode.Position != root.EpisodePosition ||
			episode.CurrentScriptVersionID == nil || *episode.CurrentScriptVersionID != version.ID ||
			version.WorkspaceID != revision.WorkspaceID || version.ProjectID != bible.ProjectID ||
			version.EpisodeID != episode.ID || version.Status != "published" ||
			version.ContentHash != bibledomain.SourceTextHash(version.Content) {
			return application.EpisodePlanningCandidateSource{}, planningCandidateConflict("Episode Planning Candidate Episode source has drifted")
		}
		episodes[index] = application.PlanningEpisodeSource{
			EpisodeID: episode.ID.String(), EpisodePosition: episode.Position,
			ScriptVersionID: version.ID.String(), ScriptVersion: version.VersionNo,
			DocumentRevisionID: version.DocumentRevisionID.String(), SourceStart: version.SourceStart,
			SourceEnd: version.SourceEnd, Content: version.Content, ContentHash: version.ContentHash,
		}
	}
	return application.EpisodePlanningCandidateSource{
		CandidateRevisionID: revision.ID.String(), CandidateRevisionHash: revision.CandidateRevisionHash,
		CandidateRevision: revision.RevisionNo, WorkspaceID: revision.WorkspaceID.String(), ProjectID: bible.ProjectID.String(),
		BibleVersionID: bible.ID.String(), BibleVersion: bible.Version, BibleContentHash: bible.ContentHash,
		MaterializationHash: materialization.ContentHash, Candidate: candidate, Episodes: episodes, Identities: identities,
	}, nil
}

func verifyEpisodePlanningLeaves(
	database *gorm.DB,
	workspaceID uuid.UUID,
	origin agentcontract.AggregateCandidateOrigin,
	forUpdate bool,
) error {
	if len(origin.LeafCandidates) == 0 {
		return planningCandidateConflict("Episode Planning Candidate has no aggregate leaves")
	}
	manifestID, err := uuid.Parse(origin.ShardManifestID)
	if err != nil {
		return planningCandidateConflict("Episode Planning Candidate manifest reference is invalid")
	}
	var manifest model.ShardManifest
	if err = database.First(&manifest, "id = ? AND version = ?", manifestID, origin.ManifestVersion).Error; err != nil {
		return normalizeNotFound(err)
	}
	if manifest.WorkspaceID != workspaceID || manifest.Stage != domain.ReconcileEpisodeStage ||
		manifest.ManifestHash != origin.ShardManifestHash {
		return planningCandidateConflict("Episode Planning Candidate manifest has drifted")
	}
	previousKey := ""
	for _, leaf := range origin.LeafCandidates {
		key := leaf.StageInstanceKey + "\x00" + leaf.ShardKey
		if key <= previousKey {
			return planningCandidateConflict("Episode Planning Candidate leaves are not canonical")
		}
		leafID, err := uuid.Parse(leaf.CandidateRevisionID)
		if err != nil {
			return planningCandidateConflict("Episode Planning Candidate leaf reference is invalid")
		}
		var revision model.StageCandidateRevision
		if err = database.First(&revision, "id = ?", leafID).Error; err != nil {
			return normalizeNotFound(err)
		}
		query := database.Where("workspace_id = ? AND stage_instance_key = ?", workspaceID, leaf.StageInstanceKey)
		if forUpdate {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var head model.StageCandidateHead
		if err = query.First(&head).Error; err != nil {
			return normalizeNotFound(err)
		}
		if revision.WorkspaceID != workspaceID || revision.StageInstanceKey != leaf.StageInstanceKey ||
			revision.CandidateRevisionHash != leaf.CandidateRevisionHash ||
			head.CurrentRevisionID != revision.ID || head.CurrentCandidateRevisionHash != revision.CandidateRevisionHash ||
			head.Revision != revision.RevisionNo {
			return planningCandidateConflict("Episode Planning Candidate leaf changed before approval")
		}
		previousKey = key
	}
	return nil
}

func loadEpisodePlanningIdentities(
	ctx context.Context,
	database *gorm.DB,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
	bible model.ProductionBibleVersion,
) (bibledomain.Materialization, []application.PlanningIdentitySource, error) {
	var receipt model.CommandReceipt
	if err := database.WithContext(ctx).
		Where("workspace_id = ? AND operation = ? AND resource_id = ?", workspaceID, "production_bible.materialize_confirmed", bible.ID).
		First(&receipt).Error; err != nil {
		return bibledomain.Materialization{}, nil, planningCandidateConflict("Episode Planning Candidate requires an exact Bible materialization")
	}
	var result struct {
		Materialization bibledomain.Materialization `json:"materialization"`
	}
	if err := json.Unmarshal(receipt.Result, &result); err != nil {
		return bibledomain.Materialization{}, nil, planningCandidateConflict("Production Bible materialization receipt is invalid")
	}
	materialization, err := bibledomain.NewMaterialization(
		result.Materialization.BibleVersionID, result.Materialization.BibleVersionHash,
		result.Materialization.Assets, result.Materialization.Specifications,
		result.Materialization.States, result.Materialization.Bindings,
	)
	if err != nil || !reflect.DeepEqual(materialization, result.Materialization) ||
		materialization.BibleVersionID != bible.ID.String() || materialization.BibleVersionHash != bible.ContentHash {
		return bibledomain.Materialization{}, nil, planningCandidateConflict("Production Bible materialization has drifted")
	}
	assets := make(map[string]bibledomain.MaterializedAsset, len(materialization.Assets))
	for _, expected := range materialization.Assets {
		id, parseErr := uuid.Parse(expected.ID)
		if parseErr != nil {
			return bibledomain.Materialization{}, nil, planningCandidateConflict("Production Bible Asset reference is invalid")
		}
		var record model.Asset
		if err = database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
			return bibledomain.Materialization{}, nil, normalizeNotFound(err)
		}
		actual := bibledomain.MaterializedAsset{
			ID: record.ID.String(), IdentityKey: record.IdentityKey, Kind: record.Kind,
			Revision: record.Revision, ContentHash: record.ContentHash,
		}
		if record.WorkspaceID != workspaceID || record.ProjectID != projectID || actual != expected {
			return bibledomain.Materialization{}, nil, planningCandidateConflict("Production Bible Asset has drifted")
		}
		assets[actual.ID] = actual
	}
	specifications := make(map[string]bibledomain.MaterializedSpecification, len(materialization.Specifications))
	for _, expected := range materialization.Specifications {
		id, parseErr := uuid.Parse(expected.ID)
		if parseErr != nil {
			return bibledomain.Materialization{}, nil, planningCandidateConflict("Production Bible Specification reference is invalid")
		}
		var record model.ProductionBibleSpecificationVersion
		if err = database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
			return bibledomain.Materialization{}, nil, normalizeNotFound(err)
		}
		actual := bibledomain.MaterializedSpecification{
			ID: record.ID.String(), AssetID: record.AssetID.String(), Kind: record.Kind,
			EntityKey: record.EntityKey, Version: record.Version, ContentHash: record.ContentHash,
		}
		if record.WorkspaceID != workspaceID || record.ProjectID != projectID ||
			record.SourceBibleVersionID != bible.ID || actual != expected {
			return bibledomain.Materialization{}, nil, planningCandidateConflict("Production Bible Specification has drifted")
		}
		specifications[actual.ID] = actual
	}
	statesByAsset := make(map[string][]bibledomain.MaterializedState)
	for _, expected := range materialization.States {
		id, parseErr := uuid.Parse(expected.ID)
		if parseErr != nil {
			return bibledomain.Materialization{}, nil, planningCandidateConflict("Production Bible State reference is invalid")
		}
		var record model.AssetState
		if err = database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
			return bibledomain.Materialization{}, nil, normalizeNotFound(err)
		}
		actual := bibledomain.MaterializedState{
			ID: record.ID.String(), AssetID: record.AssetID.String(), StateKey: record.StateKey,
			Revision: record.Revision, ContentHash: record.ContentHash,
		}
		if record.WorkspaceID != workspaceID || record.ProjectID != projectID || actual != expected {
			return bibledomain.Materialization{}, nil, planningCandidateConflict("Production Bible State has drifted")
		}
		statesByAsset[actual.AssetID] = append(statesByAsset[actual.AssetID], actual)
	}
	identities := make([]application.PlanningIdentitySource, len(materialization.Bindings))
	for index, binding := range materialization.Bindings {
		asset, assetExists := assets[binding.AssetID]
		specification, specificationExists := specifications[binding.SpecificationVersionID]
		states := append([]bibledomain.MaterializedState(nil), statesByAsset[binding.AssetID]...)
		slices.SortFunc(states, func(left, right bibledomain.MaterializedState) int {
			return strings.Compare(left.StateKey, right.StateKey)
		})
		if !assetExists || !specificationExists || len(states) == 0 || binding.EntityKey != asset.IdentityKey {
			return bibledomain.Materialization{}, nil, planningCandidateConflict("Production Bible binding has drifted")
		}
		identities[index] = application.PlanningIdentitySource{
			EntityKey: binding.EntityKey, Asset: asset, Specification: specification, States: states,
		}
	}
	slices.SortFunc(identities, func(left, right application.PlanningIdentitySource) int {
		return strings.Compare(left.EntityKey, right.EntityKey)
	})
	return materialization, identities, nil
}

func (repo *repository) CreatePlanningOwnerSet(ctx context.Context, structures []domain.Structure) error {
	if len(structures) == 0 {
		return planningCandidateConflict("Episode Planning owner set is empty")
	}
	records := make([]model.EpisodeStructure, len(structures))
	for index, structure := range structures {
		record, err := structureRecord(structure)
		if err != nil {
			return err
		}
		records[index] = record
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&records).Error
}

func (repo *repository) GetPlanningOwnerSet(
	ctx context.Context,
	actor application.Actor,
	reference application.PlanningOwnerSetReference,
) ([]domain.Structure, error) {
	if len(reference.Structures) == 0 {
		return nil, planningCandidateConflict("Episode Planning owner reference is empty")
	}
	projectID, err := uuid.Parse(reference.ProjectID)
	if err != nil {
		return nil, application.ErrNotFound
	}
	if err = authorizeProject(ctx, repo.database, actor, projectID, false); err != nil {
		return nil, err
	}
	result := make([]domain.Structure, len(reference.Structures))
	for index, expected := range reference.Structures {
		id, parseErr := uuid.Parse(expected.StructureID)
		if parseErr != nil {
			return nil, application.ErrNotFound
		}
		var record model.EpisodeStructure
		if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
			return nil, normalizeNotFound(err)
		}
		value, convertErr := structureDomain(record)
		if convertErr != nil {
			return nil, convertErr
		}
		if value.WorkspaceID != reference.WorkspaceID || value.ProjectID != reference.ProjectID ||
			value.ID != expected.StructureID || value.EpisodeID != expected.EpisodeID ||
			value.ScriptVersionID != expected.ScriptVersionID || value.ResultHash != expected.ResultHash ||
			value.Revision != expected.Revision || value.Status != "confirmed" {
			return nil, planningCandidateConflict("Episode Planning owner Structure has drifted")
		}
		result[index] = value
	}
	return result, nil
}

func (repo *repository) GetPlanningOwnerSetReceipt(
	ctx context.Context,
	receiptID string,
) (platformcommand.Receipt, error) {
	id, err := uuid.Parse(receiptID)
	if err != nil {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	var record model.CommandReceipt
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
		}
		return platformcommand.Receipt{}, err
	}
	return platformcommand.Receipt{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), Operation: record.Operation,
		IdempotencyKey: record.IdempotencyKey, InputHash: record.InputHash, ResourceID: record.ResourceID.String(),
		Result: append([]byte(nil), record.Result...), CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
	}, nil
}

func planningCandidateConflict(message string) error {
	return &application.Error{Code: "resource_conflict", Message: message, Status: 409}
}

var _ application.EpisodePlanningTransactionManager = (*Store)(nil)
var _ application.EpisodePlanningRepository = (*repository)(nil)
