package gormdb

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func (store *Store) ReadCurrentStructureIdentity(
	ctx context.Context,
	workspaceID, projectID string,
) (domain.StructureIdentitySetVersion, domain.StructureIdentityCollectionReceipt, string, error) {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return domain.StructureIdentitySetVersion{}, domain.StructureIdentityCollectionReceipt{}, "", application.ErrNotFound
	}
	project, err := uuid.Parse(projectID)
	if err != nil {
		return domain.StructureIdentitySetVersion{}, domain.StructureIdentityCollectionReceipt{}, "", application.ErrNotFound
	}

	var version domain.StructureIdentitySetVersion
	var receipt domain.StructureIdentityCollectionReceipt
	var commandReceiptID string
	err = store.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var head model.StructureIdentityScopeHead
		if loadErr := transaction.First(
			&head,
			"workspace_id = ? AND project_id = ?",
			workspace,
			project,
		).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var versionRecord model.StructureIdentitySetVersion
		if loadErr := transaction.First(
			&versionRecord,
			"id = ? AND workspace_id = ? AND project_id = ?",
			head.CurrentVersionID,
			workspace,
			project,
		).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		version, err = structureIdentityVersionDomain(versionRecord)
		if err != nil {
			return err
		}
		collection, buildErr := domain.BuildStructureIdentityCollection(version)
		var headRefs []ownercollection.VersionRef
		if json.Unmarshal(head.CurrentVersionRefs, &headRefs) != nil {
			return structureIdentityQueryDrift("Structure Identity Head refs have drifted")
		}
		rebuiltHead, headErr := domain.NewStructureIdentityScopeHead(collection, version.ID, head.HeadRevision, head.UpdatedAt)
		if buildErr != nil || headErr != nil || head.ScopeKey != rebuiltHead.ScopeKey ||
			head.ScopeRevision != rebuiltHead.ScopeRevision || head.ScopeContentHash != rebuiltHead.ScopeContentHash ||
			head.MemberCount != rebuiltHead.MemberCount || head.MembersHash != rebuiltHead.MembersHash ||
			head.CollectionRootHash != rebuiltHead.CollectionRootHash || head.HeadContentHash != rebuiltHead.HeadContentHash ||
			!reflect.DeepEqual(headRefs, rebuiltHead.CurrentVersionRefs) {
			return structureIdentityQueryDrift("Structure Identity Head has drifted")
		}

		var receiptRecord model.StructureIdentityCollectionReceipt
		if loadErr := transaction.First(
			&receiptRecord,
			"workspace_id = ? AND project_id = ? AND version_id = ?",
			workspace,
			project,
			versionRecord.ID,
		).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var scopes []string
		var members, committed []ownercollection.VersionRef
		if json.Unmarshal(receiptRecord.CoveredScopeKeys, &scopes) != nil ||
			json.Unmarshal(receiptRecord.Members, &members) != nil ||
			json.Unmarshal(receiptRecord.CommittedOwnerRefs, &committed) != nil || len(members) != 1 {
			return structureIdentityQueryDrift("Structure Identity receipt has drifted")
		}
		receipt, buildErr = domain.NewStructureIdentityCollectionReceipt(
			receiptRecord.ID.String(), receiptRecord.CommandID.String(), receiptRecord.IdempotencyKey,
			receiptRecord.ReviewDecisionID.String(), collection, scopes,
			receiptRecord.CommittedAt, receiptRecord.CommittedBy.String(),
		)
		if buildErr != nil || receiptRecord.VersionID.String() != version.ID ||
			receiptRecord.CheckpointKey != receipt.CheckpointKey || receiptRecord.OwnerKind != receipt.OwnerKind ||
			receiptRecord.CollectionFamily != receipt.CollectionFamily || receiptRecord.ScopeKind != receipt.ScopeKind ||
			receiptRecord.ScopeKey != receipt.ScopeKey || receiptRecord.ScopeRevision != receipt.ScopeRevision ||
			receiptRecord.ScopeContentHash != receipt.ScopeContentHash || receiptRecord.MemberCount != receipt.MemberCount ||
			receiptRecord.MembersHash != receipt.MembersHash || receiptRecord.CollectionRootHash != receipt.CollectionRootHash ||
			receiptRecord.ReceiptContentHash != receipt.ReceiptContentHash || !reflect.DeepEqual(members, receipt.Members) ||
			!reflect.DeepEqual(committed, receipt.CommittedOwnerVersionRefs) || receipt.ReviewDecisionID != version.ReviewDecisionID {
			return structureIdentityQueryDrift("Structure Identity receipt does not match its version")
		}
		expectedScopes := make([]string, len(version.SceneRefs))
		for index, scene := range version.SceneRefs {
			expectedScopes[index] = scene.ScopeKey
		}
		slices.Sort(expectedScopes)
		if !slices.Equal(receipt.CoveredScopeKeys, expectedScopes) {
			return structureIdentityQueryDrift("Structure Identity receipt scope has drifted")
		}

		repository := &repository{database: transaction}
		source, loadErr := repository.GetStructureIdentitySource(ctx, projectID, version.DocumentRevisionID, false)
		if loadErr != nil {
			return loadErr
		}
		episodes, loadErr := repository.GetEpisodeLifecycleReceipt(
			ctx,
			version.ProjectEpisodeReceiptID,
			workspaceID,
			projectID,
		)
		if loadErr != nil {
			return loadErr
		}
		expectedVersionHash, hashErr := application.HashStructureIdentityVersionContent(
			version,
			source.DocumentRevisionHash,
			source.SpanIndexHash,
			episodes.CollectionRootHash,
		)
		if hashErr != nil || expectedVersionHash != version.ContentHash {
			return structureIdentityQueryDrift("Structure Identity version content has drifted")
		}
		var commandReceipt model.CommandReceipt
		query := transaction.Where(
			"workspace_id = ? AND operation = ? AND resource_id = ?",
			workspace,
			domain.StructureIdentityCommandOperation,
			versionRecord.ID,
		)
		var commandReceiptCount int64
		if countErr := query.Model(&model.CommandReceipt{}).Count(&commandReceiptCount).Error; countErr != nil {
			return countErr
		}
		if commandReceiptCount != 1 || query.First(&commandReceipt).Error != nil {
			return structureIdentityQueryDrift("Structure Identity command receipt has drifted")
		}
		var commandResult domain.ConfirmStructureIdentitySetResult
		if json.Unmarshal(commandReceipt.Result, &commandResult) != nil ||
			commandResult.CommandReceiptID != commandReceipt.ID.String() ||
			commandResult.CommandOperation != domain.StructureIdentityCommandOperation ||
			commandResult.Version.ID != version.ID || commandResult.Version.ContentHash != version.ContentHash ||
			commandResult.Receipt.ID != receipt.ID || commandResult.Receipt.ReceiptContentHash != receipt.ReceiptContentHash {
			return structureIdentityQueryDrift("Structure Identity command result has drifted")
		}
		commandReceiptID = commandReceipt.ID.String()
		return nil
	})
	return version, receipt, commandReceiptID, err
}

func (store *Store) ReadExactStructureIdentity(
	ctx context.Context,
	workspaceID, projectID, versionID string,
) (domain.StructureIdentitySetVersion, error) {
	workspace, workspaceErr := uuid.Parse(workspaceID)
	project, projectErr := uuid.Parse(projectID)
	versionIdentity, versionErr := uuid.Parse(versionID)
	if workspaceErr != nil || projectErr != nil || versionErr != nil {
		return domain.StructureIdentitySetVersion{}, application.ErrNotFound
	}
	var version domain.StructureIdentitySetVersion
	err := store.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var record model.StructureIdentitySetVersion
		if loadErr := transaction.First(
			&record,
			"id = ? AND workspace_id = ? AND project_id = ?",
			versionIdentity,
			workspace,
			project,
		).Error; loadErr != nil {
			return normalizeNotFound(loadErr)
		}
		var decodeErr error
		version, decodeErr = structureIdentityVersionDomain(record)
		if decodeErr != nil {
			return decodeErr
		}
		repository := &repository{database: transaction}
		source, loadErr := repository.GetStructureIdentitySource(ctx, projectID, version.DocumentRevisionID, false)
		if loadErr != nil {
			return loadErr
		}
		episodes, loadErr := repository.GetEpisodeLifecycleReceipt(
			ctx,
			version.ProjectEpisodeReceiptID,
			workspaceID,
			projectID,
		)
		if loadErr != nil {
			return loadErr
		}
		expectedHash, hashErr := application.HashStructureIdentityVersionContent(
			version,
			source.DocumentRevisionHash,
			source.SpanIndexHash,
			episodes.CollectionRootHash,
		)
		if hashErr != nil || expectedHash != version.ContentHash {
			return structureIdentityQueryDrift("Structure Identity version content has drifted")
		}
		return nil
	})
	return version, err
}

func structureIdentityQueryDrift(message string) error {
	return &application.Error{Code: "formal_version_drift", Message: message, Status: 409}
}

var _ application.StructureIdentityReader = (*Store)(nil)
