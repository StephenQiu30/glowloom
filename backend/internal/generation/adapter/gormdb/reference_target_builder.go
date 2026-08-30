package gormdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	commandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	storyboarddomain "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

const referenceTargetBuildOperation = "generation.reference_targets.build"

type referenceTargetBuilderRepository struct{ database *gorm.DB }

func (store *Store) WithinReferenceTargetTransaction(
	ctx context.Context,
	operation func(application.ReferenceTargetBuilderRepository) error,
) error {
	if store == nil || store.database == nil || operation == nil {
		return errors.New("reference target transaction is not configured")
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&referenceTargetBuilderRepository{database: transaction})
	})
}

func (repo *referenceTargetBuilderRepository) LoadReferenceTargetSource(
	ctx context.Context,
	actor application.Actor,
	approvedIntentSetID string,
) (application.ReferenceTargetSource, error) {
	approvedID, err := uuid.Parse(strings.TrimSpace(approvedIntentSetID))
	if err != nil {
		return application.ReferenceTargetSource{}, application.ErrGenerationTargetNotFound
	}
	var receipt model.CommandReceipt
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&receipt, "id = ?", approvedID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return application.ReferenceTargetSource{}, application.ErrGenerationTargetNotFound
		}
		return application.ReferenceTargetSource{}, err
	}
	var approved storyboarddomain.ApprovedIntentSet
	if err = decodeReferenceTargetSourceStrict(receipt.Result, &approved); err != nil {
		return application.ReferenceTargetSource{}, builderConflict("Approved Storyboard Intent receipt is invalid")
	}
	visualHash, visualErr := storyboarddomain.ApprovedIntentVisualRequirementsHash(approved.Scenes)
	contentHash, contentErr := storyboarddomain.ApprovedIntentSetContentHash(approved)
	if visualErr != nil || contentErr != nil || visualHash != approved.VisualRequirementsHash || contentHash != approved.ContentHash ||
		approved.SchemaVersion != "approved-storyboard-intents" || approved.ID != receipt.ID.String() ||
		approved.WorkspaceID != receipt.WorkspaceID.String() || receipt.Operation != "storyboard.freeze_intent_set" ||
		receipt.ResourceID.String() != approved.DraftSetID || len(approved.Scenes) == 0 ||
		!validBuilderUUIDs(approved.ProjectID, approved.DraftSetID, approved.CandidateRevisionID, approved.GraphVersionID,
			approved.ManifestID, approved.ReviewDecisionID) {
		return application.ReferenceTargetSource{}, builderConflict("Approved Storyboard Intent receipt has drifted")
	}
	if err = (&repository{database: repo.database}).AuthorizeProject(
		ctx, actor, approved.WorkspaceID, approved.ProjectID, true,
	); err != nil {
		return application.ReferenceTargetSource{}, err
	}
	var set model.StoryboardDraftSet
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&set, "id = ?", approved.DraftSetID).Error; err != nil {
		return application.ReferenceTargetSource{}, normalizeReferenceTargetNotFound(err)
	}
	if set.WorkspaceID.String() != approved.WorkspaceID || set.ProjectID.String() != approved.ProjectID ||
		set.Status != "intent_frozen" || set.ResultHash == nil || *set.ResultHash != approved.ContentHash ||
		set.Revision != approved.DraftSetRevision+1 || set.CandidateRevisionID == nil ||
		set.CandidateRevisionID.String() != approved.CandidateRevisionID || set.CandidateRevisionHash == nil ||
		*set.CandidateRevisionHash != approved.CandidateRevisionHash || set.GraphVersionID.String() != approved.GraphVersionID ||
		set.GraphVersionNo != approved.GraphVersionNo || set.GraphContentHash != approved.GraphContentHash ||
		set.ManifestID.String() != approved.ManifestID || set.ManifestVersion != approved.ManifestVersion ||
		set.ManifestHash != approved.ManifestHash {
		return application.ReferenceTargetSource{}, builderConflict("Approved Storyboard Intent source has drifted")
	}

	var style agentcontract.StoryboardStyleSnapshotInput
	styleFound := false
	requirements := make([]application.ReferenceTargetRequirement, 0)
	for _, scene := range approved.Scenes {
		sceneRequirements, sceneStyle, loadErr := repo.loadApprovedSceneRequirements(ctx, approved, set, scene)
		if loadErr != nil {
			return application.ReferenceTargetSource{}, loadErr
		}
		if !styleFound {
			style, styleFound = sceneStyle, true
		} else if style != sceneStyle {
			return application.ReferenceTargetSource{}, builderConflict("Approved Storyboard Intents contain mixed style snapshots")
		}
		requirements = append(requirements, sceneRequirements...)
	}
	if !styleFound || len(requirements) == 0 {
		return application.ReferenceTargetSource{}, builderConflict("Approved Storyboard Intents contain no missing reference assets")
	}
	return application.ReferenceTargetSource{
		WorkspaceID: approved.WorkspaceID, ProjectID: approved.ProjectID,
		ApprovedIntentSetRef: domain.FrozenOwnerReference{
			Owner: "storyboard", Resource: "approved_storyboard_intents", ID: approved.ID,
			Revision: 1, ContentHash: approved.ContentHash,
		},
		EffectiveStyleSnapshotRef: domain.FrozenOwnerReference{
			Owner: "preset", Resource: "effective_style_snapshot", ID: style.OwnerVersionID,
			Revision: style.Revision, ContentHash: style.ContentHash,
		},
		VisualStyle: style.VisualStyle, AspectRatio: style.AspectRatio,
		Requirements: requirements, CreatedBy: receipt.CreatedBy.String(),
	}, nil
}

func (repo *referenceTargetBuilderRepository) loadApprovedSceneRequirements(
	ctx context.Context,
	approved storyboarddomain.ApprovedIntentSet,
	set model.StoryboardDraftSet,
	scene storyboarddomain.ApprovedIntentScene,
) ([]application.ReferenceTargetRequirement, agentcontract.StoryboardStyleSnapshotInput, error) {
	if !validBuilderUUIDs(scene.BatchID, scene.EpisodeID, scene.StructureID, scene.ScriptVersionID, scene.CandidateRevisionID) {
		return nil, agentcontract.StoryboardStyleSnapshotInput{}, builderConflict("Approved Storyboard Intent Scene identity is invalid")
	}
	var batch model.StoryboardDraftBatch
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&batch, "id = ?", scene.BatchID).Error; err != nil {
		return nil, agentcontract.StoryboardStyleSnapshotInput{}, normalizeReferenceTargetNotFound(err)
	}
	if batch.WorkspaceID.String() != approved.WorkspaceID || batch.ProjectID.String() != approved.ProjectID ||
		batch.WorkflowRunID != set.WorkflowRunID || batch.NodeRunID != set.NodeRunID ||
		batch.ManifestID != set.ManifestID || batch.ManifestVersion != set.ManifestVersion ||
		batch.GraphVersionID != set.GraphVersionID || batch.GraphVersionNo != set.GraphVersionNo ||
		batch.SceneStoryNodeKey != scene.SceneStoryNodeKey || batch.EpisodeID.String() != scene.EpisodeID ||
		batch.StructureID.String() != scene.StructureID || batch.ScriptVersionID.String() != scene.ScriptVersionID ||
		batch.Status != scene.AssetReadiness || batch.ResultHash == nil || batch.CandidateRevisionID == nil ||
		batch.CandidateRevisionID.String() != scene.CandidateRevisionID || batch.CandidateRevisionHash == nil ||
		*batch.CandidateRevisionHash != scene.CandidateRevisionHash {
		return nil, agentcontract.StoryboardStyleSnapshotInput{}, builderConflict("Approved Storyboard Intent Scene has drifted")
	}
	var invocation model.AgentInvocation
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"request_type = ? AND request_id = ?", "storyboard_scene_draft", batch.ID,
	).First(&invocation).Error; err != nil {
		return nil, agentcontract.StoryboardStyleSnapshotInput{}, normalizeReferenceTargetNotFound(err)
	}
	var revision model.StageCandidateRevision
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(
		&revision, "id = ?", scene.CandidateRevisionID,
	).Error; err != nil {
		return nil, agentcontract.StoryboardStyleSnapshotInput{}, normalizeReferenceTargetNotFound(err)
	}
	request, err := agentgorm.StageInvocation(invocation)
	if err != nil || revision.WorkspaceID != set.WorkspaceID || revision.OriginKind != "invocation" ||
		revision.RevisionNo != 1 || revision.SourceInvocationID == nil || *revision.SourceInvocationID != invocation.ID ||
		revision.SourceResultHash == nil || batch.ResultHash == nil || *revision.SourceResultHash != *batch.ResultHash ||
		revision.CandidateContentHash != *batch.ResultHash || revision.CandidateRevisionHash != scene.CandidateRevisionHash ||
		revision.StageInstanceKey != invocation.StageInstanceKey || invocation.WorkspaceID != set.WorkspaceID || invocation.WorkflowRunID == nil ||
		*invocation.WorkflowRunID != set.WorkflowRunID || invocation.NodeRunID == nil || *invocation.NodeRunID != set.NodeRunID ||
		invocation.ShardManifestID == nil || *invocation.ShardManifestID != set.ManifestID ||
		invocation.ShardManifestVersion == nil || *invocation.ShardManifestVersion != set.ManifestVersion ||
		invocation.Stage != "draft_storyboard" || request.Payload.Stage != invocation.Stage ||
		invocation.ShardKey != "scene:"+scene.SceneStoryNodeKey || request.Payload.ShardKey != invocation.ShardKey ||
		invocation.InputHash != batch.InputHash || invocation.Status != "succeeded" || invocation.ResultHash == nil ||
		*invocation.ResultHash != *batch.ResultHash || invocation.CandidateType == nil ||
		*invocation.CandidateType != "storyboard_row_candidate" || request.Payload.WorkspaceID != approved.WorkspaceID ||
		request.Payload.ProjectID != approved.ProjectID || request.Payload.BaseStoryGraphVersionID != approved.GraphVersionID ||
		request.Payload.BaseStoryGraphHash != approved.GraphContentHash || request.Payload.ShardManifestRef.ManifestID != approved.ManifestID ||
		request.Payload.ShardManifestRef.Version != approved.ManifestVersion || request.Payload.ShardManifestRef.Hash != approved.ManifestHash {
		return nil, agentcontract.StoryboardStyleSnapshotInput{}, builderConflict("Approved Storyboard Intent invocation has drifted")
	}
	var stageInput agentcontract.StoryboardDraftStageInput
	if err = decodeReferenceTargetSourceStrict(request.Payload.StageInput, &stageInput); err != nil ||
		stageInput.Scene.StoryNodeKey != scene.SceneStoryNodeKey || stageInput.GraphVersionNo != approved.GraphVersionNo {
		return nil, agentcontract.StoryboardStyleSnapshotInput{}, builderConflict("Approved Storyboard Intent stage input has drifted")
	}
	occurrences := make(map[string]agentcontract.StoryboardOccurrenceInput, len(stageInput.Occurrences))
	for _, occurrence := range stageInput.Occurrences {
		occurrences[occurrence.StoryNodeKey] = occurrence
	}
	requirements := make([]application.ReferenceTargetRequirement, 0)
	for _, intent := range scene.ShotIntents {
		for _, requirement := range intent.VisualRequirements {
			if requirement.AssetReadiness == "ready" && requirement.AssetVersionRef != nil {
				continue
			}
			occurrence, found := occurrences[requirement.OccurrenceStoryNodeKey]
			if !found || requirement.AssetReadiness != "needs_asset" || requirement.AssetVersionRef != nil ||
				requirement.IdentityStoryNodeKey != occurrence.IdentityStoryNodeKey ||
				requirement.SpecificationStoryNodeKey != occurrence.SpecificationStoryNodeKey ||
				requirement.AssetStateStoryNodeKey != occurrence.AssetStateStoryNodeKey ||
				requirement.AssetID != occurrence.AssetID || requirement.SpecificationVersionID != occurrence.SpecificationVersionID ||
				requirement.AssetStateID != occurrence.AssetStateID {
				return nil, agentcontract.StoryboardStyleSnapshotInput{}, builderConflict("Approved visual requirement has drifted")
			}
			resolved, resolveErr := repo.resolveReferenceRequirement(ctx, approved, occurrence, requirement.RequiredViewRoles)
			if resolveErr != nil {
				return nil, agentcontract.StoryboardStyleSnapshotInput{}, resolveErr
			}
			requirements = append(requirements, resolved)
		}
	}
	return requirements, stageInput.EffectiveStyleSnapshot, nil
}

func (repo *referenceTargetBuilderRepository) resolveReferenceRequirement(
	ctx context.Context,
	approved storyboarddomain.ApprovedIntentSet,
	occurrence agentcontract.StoryboardOccurrenceInput,
	requiredViewRoles []string,
) (application.ReferenceTargetRequirement, error) {
	var asset model.Asset
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&asset, "id = ?", occurrence.AssetID).Error; err != nil {
		return application.ReferenceTargetRequirement{}, normalizeReferenceTargetNotFound(err)
	}
	assetValue, err := assetdomain.NewAsset(assetdomain.AssetInput{
		ID: asset.ID.String(), WorkspaceID: asset.WorkspaceID.String(), ProjectID: asset.ProjectID.String(),
		Kind: asset.Kind, IdentityKey: asset.IdentityKey, CreatedBy: asset.CreatedBy.String(), CreatedAt: asset.CreatedAt,
	})
	if err != nil || assetValue.Revision != asset.Revision || assetValue.ContentHash != asset.ContentHash ||
		asset.WorkspaceID.String() != approved.WorkspaceID || asset.ProjectID.String() != approved.ProjectID || asset.Kind != occurrence.AssetKind {
		return application.ReferenceTargetRequirement{}, builderConflict("Approved visual requirement Asset has drifted")
	}
	var specification model.ProductionBibleSpecificationVersion
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&specification, "id = ?", occurrence.SpecificationVersionID).Error; err != nil {
		return application.ReferenceTargetRequirement{}, normalizeReferenceTargetNotFound(err)
	}
	specificationValue, err := bibledomain.NewSpecificationVersion(bibledomain.SpecificationVersionInput{
		ID: specification.ID.String(), WorkspaceID: specification.WorkspaceID.String(), ProjectID: specification.ProjectID.String(),
		AssetID: specification.AssetID.String(), Kind: specification.Kind, EntityKey: specification.EntityKey,
		Version: specification.Version, SourceBibleVersionID: specification.SourceBibleVersionID.String(),
		Snapshot: append([]byte(nil), specification.Snapshot...), CreatedBy: specification.CreatedBy.String(), CreatedAt: specification.CreatedAt,
	})
	if err != nil || specificationValue.ContentHash != specification.ContentHash || specification.AssetID != asset.ID ||
		specification.WorkspaceID != asset.WorkspaceID || specification.ProjectID != asset.ProjectID || specification.Kind != asset.Kind {
		return application.ReferenceTargetRequirement{}, builderConflict("Approved visual requirement SpecificationVersion has drifted")
	}
	var state model.AssetState
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, "id = ?", occurrence.AssetStateID).Error; err != nil {
		return application.ReferenceTargetRequirement{}, normalizeReferenceTargetNotFound(err)
	}
	stateValue, err := assetdomain.NewAssetState(assetdomain.AssetStateInput{
		ID: state.ID.String(), WorkspaceID: state.WorkspaceID.String(), ProjectID: state.ProjectID.String(), AssetID: state.AssetID.String(),
		StateKey: state.StateKey, Label: state.Label, Revision: state.Revision, Snapshot: append([]byte(nil), state.Snapshot...),
		CreatedBy: state.CreatedBy.String(), CreatedAt: state.CreatedAt,
	})
	if err != nil || stateValue.ContentHash != state.ContentHash || state.AssetID != asset.ID ||
		state.WorkspaceID != asset.WorkspaceID || state.ProjectID != asset.ProjectID {
		return application.ReferenceTargetRequirement{}, builderConflict("Approved visual requirement AssetState has drifted")
	}
	return application.ReferenceTargetRequirement{
		AssetID: asset.ID.String(), AssetKind: asset.Kind, RequiredViewRoles: append([]string(nil), requiredViewRoles...),
		SpecificationVersionRef: domain.FrozenOwnerReference{
			Owner: "production", Resource: "production_bible_specification_version", ID: specification.ID.String(),
			Revision: int64(specification.Version), ContentHash: specification.ContentHash,
		},
		SpecificationSnapshot: append([]byte(nil), specificationValue.Snapshot...),
		AssetStateRef: domain.FrozenOwnerReference{
			Owner: "asset", Resource: "asset_state", ID: state.ID.String(),
			Revision: int64(state.Revision), ContentHash: state.ContentHash,
		},
		AssetStateSnapshot: append([]byte(nil), stateValue.Snapshot...),
	}, nil
}

func (repo *referenceTargetBuilderRepository) FindReferenceTargetBuildReceipt(
	ctx context.Context,
	workspaceID, key string,
) (platformcommand.Receipt, error) {
	return commandgorm.Find(ctx, repo.database, workspaceID, referenceTargetBuildOperation, key)
}

func (repo *referenceTargetBuilderRepository) EnsureReferenceTargetBuildReceipt(
	ctx context.Context,
	receipt platformcommand.Receipt,
) (platformcommand.Receipt, error) {
	return commandgorm.Ensure(ctx, repo.database, receipt)
}

func (repo *referenceTargetBuilderRepository) EnsureGenerationTarget(
	ctx context.Context,
	target domain.GenerationTarget,
) (domain.GenerationTarget, error) {
	return (&TargetStore{database: repo.database}).Ensure(ctx, target)
}

func (repo *referenceTargetBuilderRepository) FindGenerationTarget(
	ctx context.Context,
	targetID string,
) (domain.GenerationTarget, error) {
	return findGenerationTarget(ctx, repo.database, targetID)
}

func decodeReferenceTargetSourceStrict(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("JSON contains multiple values")
	}
	return nil
}

func validBuilderUUIDs(values ...string) bool {
	for _, value := range values {
		if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
			return false
		}
	}
	return true
}

func normalizeReferenceTargetNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return builderConflict("Approved reference target source is incomplete")
	}
	return err
}

func builderConflict(message string) error {
	return &application.Error{Code: "state_conflict", Message: message, Status: 409}
}

var _ application.ReferenceTargetBuilderTransactions = (*Store)(nil)
var _ application.ReferenceTargetBuilderRepository = (*referenceTargetBuilderRepository)(nil)
