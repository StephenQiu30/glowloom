package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	contract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	commandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
)

type referenceTargetRepository struct {
	referenceAuthorizationRepository
}

func (store *Store) WithinReferenceGenerationTarget(ctx context.Context, operation func(application.ReferenceGenerationTargetRepository) error) error {
	if store == nil || store.database == nil {
		return errors.New("Reference generation Target store is unavailable")
	}
	return platformdatabase.WithinSerializableTransaction(ctx, store.database, func(tx *gorm.DB) error {
		return operation(&referenceTargetRepository{referenceAuthorizationRepository{repository{database: tx}}})
	})
}

func (repo *referenceTargetRepository) FindReferenceAuthorization(ctx context.Context, id string) (platformcommand.Receipt, error) {
	return commandgorm.FindByID(ctx, repo.database, id)
}

func (repo *referenceTargetRepository) FindReferenceGenerationTargetReceipt(ctx context.Context, workspaceID, targetID string) (platformcommand.Receipt, error) {
	var records []model.CommandReceipt
	if err := repo.database.WithContext(ctx).Select("id").Where("workspace_id = ? AND operation = ? AND resource_id = ?", workspaceID, application.BuildReferenceGenerationTargetOperation, targetID).Limit(2).Find(&records).Error; err != nil {
		return platformcommand.Receipt{}, err
	}
	if len(records) != 1 {
		return platformcommand.Receipt{}, errors.New("Reference generation Target publication receipt is missing or ambiguous")
	}
	return commandgorm.FindByID(ctx, repo.database, records[0].ID.String())
}

func (repo *referenceTargetRepository) ValidateReferenceGenerationCapabilities(ctx context.Context, input contract.ReferenceBriefInput) error {
	var record model.EffectivePolicySnapshot
	ref := input.EffectivePolicySnapshotRef
	if err := repo.database.WithContext(ctx).Where("id = ? AND workspace_id = ? AND project_id = ? AND content_hash = ?", ref.OwnerVersionID, input.WorkspaceID, input.ProjectID, ref.OwnerContentHash).First(&record).Error; err != nil {
		return err
	}
	// The accepted Brief reader already checks this exact immutable Policy and its current Head.
	var policy presetdomain.EffectivePolicySnapshot
	if canonical.Decode(record.Content, &policy) != nil || presetdomain.ValidateCapabilityManifest(policy.CapabilityManifest) != nil || policy.ContentHash != ref.OwnerContentHash {
		return errors.New("preset_capability_missing")
	}
	if !slices.ContainsFunc(policy.CapabilityManifest, func(value presetdomain.Capability) bool {
		return value.TargetKind == input.TargetKind && slices.Equal(value.ViewRoles, input.RequiredViewRoles)
	}) || !slices.ContainsFunc(policy.PurposeProfiles, func(value presetdomain.PurposeProfile) bool { return value.TargetKind == input.TargetKind }) {
		return errors.New("preset_capability_missing")
	}
	return nil
}

func (repo *referenceTargetRepository) PublishInitialReferenceGenerationTarget(ctx context.Context, target application.ReferenceGenerationTarget) error {
	raw, err := json.Marshal(target)
	if err != nil {
		return err
	}
	if _, err = application.DecodeReferenceGenerationTarget(raw); err != nil {
		return err
	}
	source, err := json.Marshal(target.ReferencePlanTargetRef)
	if err != nil {
		return err
	}
	policy, err := json.Marshal(target.EffectivePolicySnapshotRef)
	if err != nil {
		return err
	}
	id, idErr := uuid.Parse(target.ID)
	workspace, workspaceErr := uuid.Parse(target.WorkspaceID)
	project, projectErr := uuid.Parse(target.ProjectID)
	actor, actorErr := uuid.Parse(target.CreatedBy)
	if err = errors.Join(idErr, workspaceErr, projectErr, actorErr); err != nil {
		return err
	}
	record := model.GenerationTarget{ID: id, WorkspaceID: workspace, ProjectID: project, Kind: "reference_plan", SourceOwnerRef: source, SourceContentHash: target.ReferencePlanTargetRef.OwnerContentHash, PolicySnapshotRef: policy, PolicyContentHash: target.EffectivePolicySnapshotRef.OwnerContentHash, Payload: raw, TargetHash: target.ContentHash, Revision: target.Revision, CreatedBy: actor, CreatedAt: target.CreatedAt}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return err
	}
	head, err := referenceTargetHead(target)
	if err != nil {
		return err
	}
	created := repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&head)
	if created.Error != nil {
		return created.Error
	}
	if created.RowsAffected != 1 {
		return errors.New("Reference generation Target Head conflicts with expected initial revision")
	}
	return nil
}

func (repo *referenceTargetRepository) FindReferenceGenerationTarget(ctx context.Context, workspaceID, projectID, id string) (application.ReferenceGenerationTarget, error) {
	var record model.GenerationTarget
	if err := repo.database.WithContext(ctx).Where("id = ? AND workspace_id = ? AND project_id = ? AND kind = ?", id, workspaceID, projectID, "reference_plan").First(&record).Error; err != nil {
		return application.ReferenceGenerationTarget{}, err
	}
	value, err := application.DecodeReferenceGenerationTarget(json.RawMessage(record.Payload))
	if err != nil {
		return application.ReferenceGenerationTarget{}, err
	}
	var source, policy contract.ReferencePlanOwnerRef
	if canonical.Decode(record.SourceOwnerRef, &source) != nil || canonical.Decode(record.PolicySnapshotRef, &policy) != nil ||
		!reflect.DeepEqual(source, value.ReferencePlanTargetRef) || !reflect.DeepEqual(policy, value.EffectivePolicySnapshotRef) || record.SourceContentHash != source.OwnerContentHash || record.PolicyContentHash != policy.OwnerContentHash ||
		value.ID != record.ID.String() || value.WorkspaceID != record.WorkspaceID.String() || value.ProjectID != record.ProjectID.String() || value.ContentHash != record.TargetHash || value.Revision != record.Revision || value.CreatedBy != record.CreatedBy.String() || !value.CreatedAt.Equal(record.CreatedAt) {
		return application.ReferenceGenerationTarget{}, errors.New("persisted Reference generation Target has drifted")
	}
	return value, nil
}

func (repo *referenceTargetRepository) ValidateReferenceGenerationTargetHead(ctx context.Context, target application.ReferenceGenerationTarget) error {
	expected, err := referenceTargetHead(target)
	if err != nil {
		return err
	}
	var head model.GenerationReferenceTargetHead
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where("workspace_id = ? AND project_id = ? AND plan_version_id = ? AND reference_target_version_id = ?", expected.WorkspaceID, expected.ProjectID, expected.PlanVersionID, expected.ReferenceTargetVersionID).First(&head).Error; err != nil {
		return err
	}
	if head.CurrentTargetID != expected.CurrentTargetID || head.CurrentTargetHash != expected.CurrentTargetHash || head.Revision != expected.Revision {
		return errors.New("Reference generation Target Head has drifted")
	}
	return nil
}

func referenceTargetHead(target application.ReferenceGenerationTarget) (model.GenerationReferenceTargetHead, error) {
	values := make([]uuid.UUID, 5)
	for index, id := range []string{target.WorkspaceID, target.ProjectID, target.ApprovedReferencePlanVersionRef.OwnerVersionID, target.ReferencePlanTargetRef.OwnerVersionID, target.ID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil {
			return model.GenerationReferenceTargetHead{}, errors.New("invalid Reference generation Target Head scope")
		}
		values[index] = parsed
	}
	return model.GenerationReferenceTargetHead{WorkspaceID: values[0], ProjectID: values[1], PlanVersionID: values[2], ReferenceTargetVersionID: values[3], CurrentTargetID: values[4], CurrentTargetHash: target.ContentHash, Revision: target.GenerationRound}, nil
}

var _ application.ReferenceGenerationTargetTransactions = (*Store)(nil)
