package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
)

const productionWorldManifestSchema = "production-world-aggregate-manifest-production"

func (store *Store) EnsureProductionWorldCandidate(
	ctx context.Context,
	value application.ProductionWorldCandidateRecord,
) (application.ProductionWorldCandidateRevision, error) {
	if store == nil || store.database == nil {
		return application.ProductionWorldCandidateRevision{}, errors.New("Production World Candidate store is unavailable")
	}
	identities, err := parseProductionWorldRecordIdentities(value)
	if err != nil {
		return application.ProductionWorldCandidateRevision{}, err
	}
	candidate, _, err := worlddomain.DecodeProductionWorldCandidate(value.Candidate)
	if err != nil || candidate.WorkspaceID != value.WorkspaceID || candidate.ProjectID != value.ProjectID ||
		candidate.ContentHash != value.CandidateContentHash || value.Stage != application.ProductionWorldAssemblyStage {
		return application.ProductionWorldCandidateRevision{}, errors.New("invalid Production World Candidate record")
	}

	var result application.ProductionWorldCandidateRevision
	err = platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		if err := validateProductionWorldRun(transaction, identities); err != nil {
			return err
		}
		if err := validateProductionWorldLeafHeads(transaction, candidate, value.Leaves, identities.workspaceID, identities.projectID); err != nil {
			return err
		}
		manifest, ensureErr := ensureProductionWorldManifest(transaction, value, identities)
		if ensureErr != nil {
			return ensureErr
		}
		result, ensureErr = ensureProductionWorldRevision(transaction, value, candidate, manifest, identities)
		return ensureErr
	})
	return result, err
}

func validateProductionWorldRun(database *gorm.DB, identities productionWorldRecordIdentities) error {
	var run model.WorkflowRun
	if err := database.Clauses(clause.Locking{Strength: "SHARE"}).First(&run, "id = ?", identities.workflowRunID).Error; err != nil {
		return normalizeProductionWorldLeafError(err)
	}
	var node model.NodeRunProjection
	if err := database.Clauses(clause.Locking{Strength: "SHARE"}).First(&node, "id = ?", identities.nodeRunID).Error; err != nil {
		return normalizeProductionWorldLeafError(err)
	}
	if run.WorkspaceID != identities.workspaceID || run.ProjectID != identities.projectID ||
		node.WorkspaceID != identities.workspaceID || node.WorkflowRunID != identities.workflowRunID ||
		node.Executor != "activity.production_world_assembly" {
		return errors.New("Production World aggregate workflow owner has drifted")
	}
	return nil
}

type productionWorldRecordIdentities struct {
	manifestID, candidateRevisionID uuid.UUID
	workspaceID, projectID          uuid.UUID
	workflowRunID, nodeRunID        uuid.UUID
}

func parseProductionWorldRecordIdentities(
	value application.ProductionWorldCandidateRecord,
) (productionWorldRecordIdentities, error) {
	parsed := productionWorldRecordIdentities{}
	items := []struct {
		raw    string
		target *uuid.UUID
	}{
		{value.ManifestID, &parsed.manifestID}, {value.CandidateRevisionID, &parsed.candidateRevisionID},
		{value.WorkspaceID, &parsed.workspaceID}, {value.ProjectID, &parsed.projectID},
		{value.WorkflowRunID, &parsed.workflowRunID}, {value.NodeRunID, &parsed.nodeRunID},
	}
	for _, item := range items {
		identifier, err := uuid.Parse(item.raw)
		if err != nil {
			return productionWorldRecordIdentities{}, errors.New("invalid Production World persistence identity")
		}
		*item.target = identifier
	}
	if value.CreatedAt.IsZero() || len(value.RootInputHash) != 64 || len(value.CandidateContentHash) != 64 || len(value.Leaves) != 3 {
		return productionWorldRecordIdentities{}, errors.New("invalid Production World persistence material")
	}
	return parsed, nil
}

func validateProductionWorldLeafHeads(
	database *gorm.DB,
	candidate worlddomain.ProductionWorldCandidate,
	leaves []agentcontract.AggregateLeafCandidateRef,
	workspaceID, projectID uuid.UUID,
) error {
	expectedStages := map[string]string{
		candidate.UpstreamCandidates.ProductionEntity.CandidateRevisionID:      "derive_production_entities",
		candidate.UpstreamCandidates.SceneOccurrence.CandidateRevisionID:       "bind_scene_occurrences",
		candidate.UpstreamCandidates.InteractionContinuity.CandidateRevisionID: "reconcile_interaction_continuity",
	}
	for _, leaf := range leaves {
		revisionID, err := uuid.Parse(leaf.CandidateRevisionID)
		if err != nil {
			return errors.New("invalid Production World aggregate leaf")
		}
		var revision model.SceneAnalysisCandidateRevision
		if err = database.Clauses(clause.Locking{Strength: "SHARE"}).First(&revision, "id = ?", revisionID).Error; err != nil {
			return normalizeProductionWorldLeafError(err)
		}
		var head model.SceneAnalysisCandidateHead
		if err = database.Clauses(clause.Locking{Strength: "SHARE"}).First(&head, "stage_instance_key = ?", leaf.StageInstanceKey).Error; err != nil {
			return normalizeProductionWorldLeafError(err)
		}
		var invocation model.SceneAnalysisInvocationRecord
		if err = database.Clauses(clause.Locking{Strength: "SHARE"}).First(&invocation, "id = ?", revision.SourceInvocationID).Error; err != nil {
			return normalizeProductionWorldLeafError(err)
		}
		expectedStage, exists := expectedStages[leaf.CandidateRevisionID]
		if !exists || leaf.ShardKey != "script:full" || revision.WorkspaceID != workspaceID || revision.ProjectID != projectID ||
			revision.StageInstanceKey != leaf.StageInstanceKey || revision.CandidateRevisionHash != leaf.CandidateRevisionHash ||
			head.CurrentRevisionID != revision.ID || head.CurrentCandidateRevisionHash != revision.CandidateRevisionHash ||
			invocation.StageKey != expectedStage || invocation.ShardKey != leaf.ShardKey || invocation.Status != "accepted" {
			return errors.New("Production World aggregate read set is stale")
		}
		delete(expectedStages, leaf.CandidateRevisionID)
	}
	if len(expectedStages) != 0 {
		return errors.New("Production World aggregate read set is incomplete")
	}
	return nil
}

func ensureProductionWorldManifest(
	database *gorm.DB,
	value application.ProductionWorldCandidateRecord,
	identities productionWorldRecordIdentities,
) (model.ShardManifest, error) {
	var existing model.ShardManifest
	err := database.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"workflow_run_id = ? AND node_run_id = ? AND stage = ? AND version = 1",
		identities.workflowRunID, identities.nodeRunID, value.Stage,
	).First(&existing).Error
	if err == nil {
		expected, buildErr := buildProductionWorldManifest(value, identities, existing.ID)
		if buildErr != nil {
			return model.ShardManifest{}, buildErr
		}
		var existingLeaves []agentcontract.AggregateLeafCandidateRef
		if json.Unmarshal(existing.Shards, &existingLeaves) != nil {
			return model.ShardManifest{}, errors.New("Production World aggregate manifest drifted")
		}
		if existing.WorkspaceID != expected.WorkspaceID || existing.WorkflowRunID != expected.WorkflowRunID ||
			existing.NodeRunID != expected.NodeRunID || existing.Stage != expected.Stage || existing.Version != expected.Version ||
			existing.RootInputHash != expected.RootInputHash || existing.CoverageHash != expected.CoverageHash ||
			existing.ManifestHash != expected.ManifestHash || !reflect.DeepEqual(existingLeaves, value.Leaves) {
			return model.ShardManifest{}, errors.New("Production World aggregate manifest drifted")
		}
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ShardManifest{}, err
	}
	manifest, err := buildProductionWorldManifest(value, identities, identities.manifestID)
	if err != nil {
		return model.ShardManifest{}, err
	}
	if err = database.Omit(clause.Associations).Create(&manifest).Error; err != nil {
		return model.ShardManifest{}, err
	}
	return manifest, nil
}

func buildProductionWorldManifest(
	value application.ProductionWorldCandidateRecord,
	identities productionWorldRecordIdentities,
	manifestID uuid.UUID,
) (model.ShardManifest, error) {
	shards, err := json.Marshal(value.Leaves)
	if err != nil {
		return model.ShardManifest{}, err
	}
	coverageHash, err := agentcontract.CanonicalHash(shards)
	if err != nil {
		return model.ShardManifest{}, err
	}
	manifestHash, err := productionWorldCanonicalHash(struct {
		SchemaVersion string                                    `json:"schema_version"`
		ID            string                                    `json:"id"`
		WorkspaceID   string                                    `json:"workspace_id"`
		WorkflowRunID string                                    `json:"workflow_run_id"`
		NodeRunID     string                                    `json:"node_run_id"`
		Stage         string                                    `json:"stage"`
		Version       int64                                     `json:"version"`
		RootInputHash string                                    `json:"root_input_hash"`
		Leaves        []agentcontract.AggregateLeafCandidateRef `json:"leaves"`
		CoverageHash  string                                    `json:"coverage_hash"`
	}{
		productionWorldManifestSchema, manifestID.String(), value.WorkspaceID,
		value.WorkflowRunID, value.NodeRunID, value.Stage, 1, value.RootInputHash, value.Leaves, coverageHash,
	})
	if err != nil {
		return model.ShardManifest{}, err
	}
	return model.ShardManifest{
		ID: manifestID, Version: 1, WorkspaceID: identities.workspaceID,
		WorkflowRunID: identities.workflowRunID, NodeRunID: identities.nodeRunID,
		Stage: value.Stage, RootInputHash: value.RootInputHash, Shards: datatypes.JSON(shards),
		CoverageHash: coverageHash, ManifestHash: manifestHash, CreatedAt: value.CreatedAt,
	}, nil
}

func ensureProductionWorldRevision(
	database *gorm.DB,
	value application.ProductionWorldCandidateRecord,
	candidate worlddomain.ProductionWorldCandidate,
	manifest model.ShardManifest,
	identities productionWorldRecordIdentities,
) (application.ProductionWorldCandidateRevision, error) {
	stageInstanceKey, err := productionWorldCanonicalHash(struct {
		Stage        string `json:"stage"`
		WorkspaceID  string `json:"workspace_id"`
		ProjectID    string `json:"project_id"`
		ManifestHash string `json:"manifest_hash"`
	}{value.Stage, value.WorkspaceID, value.ProjectID, manifest.ManifestHash})
	if err != nil {
		return application.ProductionWorldCandidateRevision{}, err
	}
	origin := agentcontract.AggregateCandidateOrigin{
		ShardManifestID: manifest.ID.String(), ManifestVersion: manifest.Version,
		ShardManifestHash: manifest.ManifestHash, LeafCandidates: value.Leaves,
	}
	revisionHash, err := (agentcontract.CandidateRevisionMaterial{
		StageInstanceKey: stageInstanceKey, RevisionNo: 1, OriginKind: "aggregate",
		AggregateOrigin: &origin, CandidateContentHash: candidate.ContentHash,
	}).Hash()
	if err != nil {
		return application.ProductionWorldCandidateRevision{}, err
	}
	var existingHead model.StageCandidateHead
	err = database.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existingHead, "stage_instance_key = ?", stageInstanceKey).Error
	if err == nil {
		if existingHead.WorkspaceID != identities.workspaceID ||
			existingHead.CurrentCandidateRevisionHash != revisionHash || existingHead.Revision != 1 {
			return application.ProductionWorldCandidateRevision{}, errors.New("Production World Candidate head conflicts with aggregate")
		}
		var existing model.StageCandidateRevision
		if err = database.First(&existing, "id = ?", existingHead.CurrentRevisionID).Error; err != nil {
			return application.ProductionWorldCandidateRevision{}, err
		}
		var persistedOrigin agentcontract.AggregateCandidateOrigin
		persistedCandidate, _, decodeErr := worlddomain.DecodeProductionWorldCandidate(json.RawMessage(existing.Candidate))
		if json.Unmarshal(existing.AggregateOrigin, &persistedOrigin) != nil || decodeErr != nil ||
			existing.WorkspaceID != identities.workspaceID || existing.StageInstanceKey != stageInstanceKey ||
			existing.RevisionNo != 1 || existing.OriginKind != "aggregate" ||
			existing.CandidateContentHash != candidate.ContentHash || existing.CandidateRevisionHash != revisionHash ||
			persistedCandidate.ContentHash != candidate.ContentHash || !reflect.DeepEqual(persistedOrigin, origin) {
			return application.ProductionWorldCandidateRevision{}, errors.New("Production World Candidate revision drifted")
		}
		return productionWorldRevisionDomain(existing), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ProductionWorldCandidateRevision{}, err
	}
	originJSON, err := json.Marshal(origin)
	if err != nil {
		return application.ProductionWorldCandidateRevision{}, err
	}
	revision := model.StageCandidateRevision{
		ID: identities.candidateRevisionID, WorkspaceID: identities.workspaceID,
		StageInstanceKey: stageInstanceKey, RevisionNo: 1, OriginKind: "aggregate",
		AggregateOrigin: datatypes.JSON(originJSON), Candidate: datatypes.JSON(value.Candidate),
		CandidateContentHash: candidate.ContentHash, CandidateRevisionHash: revisionHash, CreatedAt: value.CreatedAt,
	}
	if err = database.Omit(clause.Associations).Create(&revision).Error; err != nil {
		return application.ProductionWorldCandidateRevision{}, err
	}
	head := model.StageCandidateHead{
		WorkspaceID: identities.workspaceID, StageInstanceKey: stageInstanceKey,
		CurrentRevisionID: revision.ID, CurrentCandidateRevisionHash: revisionHash,
		Revision: 1, UpdatedAt: value.CreatedAt,
	}
	if err = database.Omit(clause.Associations).Create(&head).Error; err != nil {
		return application.ProductionWorldCandidateRevision{}, err
	}
	return productionWorldRevisionDomain(revision), nil
}

func productionWorldRevisionDomain(value model.StageCandidateRevision) application.ProductionWorldCandidateRevision {
	return application.ProductionWorldCandidateRevision{
		ID: value.ID.String(), StageInstanceKey: value.StageInstanceKey, Revision: value.RevisionNo,
		Candidate: json.RawMessage(value.Candidate), CandidateContentHash: value.CandidateContentHash,
		CandidateRevisionHash: value.CandidateRevisionHash,
	}
}

func productionWorldCanonicalHash(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return agentcontract.CanonicalHash(encoded)
}

func normalizeProductionWorldLeafError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("Production World aggregate read set is stale")
	}
	return fmt.Errorf("read Production World aggregate leaf: %w", err)
}
