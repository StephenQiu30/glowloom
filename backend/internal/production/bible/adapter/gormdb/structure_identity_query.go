package gormdb

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func (store *Store) ReadCurrentStructureIdentity(
	ctx context.Context,
	workspaceID, projectID string,
) (domain.StructureIdentitySetVersion, domain.StructureIdentityCollectionReceipt, error) {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return domain.StructureIdentitySetVersion{}, domain.StructureIdentityCollectionReceipt{}, application.ErrNotFound
	}
	project, err := uuid.Parse(projectID)
	if err != nil {
		return domain.StructureIdentitySetVersion{}, domain.StructureIdentityCollectionReceipt{}, application.ErrNotFound
	}

	var version domain.StructureIdentitySetVersion
	var receipt domain.StructureIdentityCollectionReceipt
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
		if head.HeadRevision != int64(version.Version) || head.HeadHash != version.ContentHash {
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
		var members []struct {
			VersionID   string `json:"version_id"`
			ContentHash string `json:"content_hash"`
		}
		if json.Unmarshal(receiptRecord.CoveredScopeKeys, &scopes) != nil ||
			json.Unmarshal(receiptRecord.Members, &members) != nil || len(members) != 1 {
			return structureIdentityQueryDrift("Structure Identity receipt has drifted")
		}
		receipt = domain.StructureIdentityCollectionReceipt{
			ID: receiptRecord.ID.String(), CheckpointKey: receiptRecord.CheckpointKey,
			CollectionFamily: receiptRecord.CollectionFamily, VersionID: receiptRecord.VersionID.String(),
			VersionContentHash: members[0].ContentHash, ReviewDecisionID: receiptRecord.ReviewDecisionID.String(),
			CoveredScopeKeys: scopes, CollectionRootHash: receiptRecord.CollectionRootHash,
			ReceiptContentHash: receiptRecord.ReceiptContentHash,
		}
		if members[0].VersionID != version.ID || receipt.VersionID != version.ID ||
			receipt.VersionContentHash != version.ContentHash || receipt.ReviewDecisionID != version.ReviewDecisionID ||
			receipt.CheckpointKey != domain.StructureIdentityCheckpointKey ||
			receipt.CollectionFamily != domain.StructureIdentityCollectionFamily {
			return structureIdentityQueryDrift("Structure Identity receipt does not match its version")
		}
		expectedScopes := make([]string, len(version.SceneRefs))
		for index, scene := range version.SceneRefs {
			expectedScopes[index] = scene.ScopeKey
		}
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
		expectedCollectionRoot, hashErr := platformcommand.InputHash(struct {
			Family, VersionID, VersionHash string
			CoveredScopeKeys               []string
		}{domain.StructureIdentityCollectionFamily, version.ID, version.ContentHash, expectedScopes})
		if hashErr != nil || expectedCollectionRoot != receipt.CollectionRootHash {
			return structureIdentityQueryDrift("Structure Identity collection root has drifted")
		}
		expectedReceiptHash, hashErr := platformcommand.InputHash(struct {
			CheckpointKey, Family, VersionID, VersionHash, ReviewDecisionID, CollectionRootHash string
			CoveredScopeKeys                                                                    []string
		}{
			domain.StructureIdentityCheckpointKey,
			domain.StructureIdentityCollectionFamily,
			version.ID,
			version.ContentHash,
			version.ReviewDecisionID,
			receipt.CollectionRootHash,
			expectedScopes,
		})
		if hashErr != nil || expectedReceiptHash != receipt.ReceiptContentHash {
			return structureIdentityQueryDrift("Structure Identity receipt content has drifted")
		}
		return nil
	})
	return version, receipt, err
}

func structureIdentityQueryDrift(message string) error {
	return &application.Error{Code: "formal_version_drift", Message: message, Status: 409}
}

var _ application.StructureIdentityReader = (*Store)(nil)
