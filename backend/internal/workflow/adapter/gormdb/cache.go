package gormdb

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func (store *Store) FindNodeCache(ctx context.Context, workspaceID, cacheKey string) (domain.NodeCacheEntry, error) {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil || len(cacheKey) != 64 {
		return domain.NodeCacheEntry{}, application.ErrNotFound
	}
	if _, err = hex.DecodeString(cacheKey); err != nil {
		return domain.NodeCacheEntry{}, application.ErrNotFound
	}
	var persisted model.NodeCacheEntry
	if err = store.database.WithContext(ctx).
		Where("workspace_id = ? AND cache_key = ?", workspace, cacheKey).First(&persisted).Error; err != nil {
		return domain.NodeCacheEntry{}, normalizeNotFound(err)
	}
	return nodeCacheDomain(persisted)
}

func (store *Store) EnsureNodeCache(ctx context.Context, desired domain.NodeCacheEntry) (domain.NodeCacheEntry, error) {
	record, err := nodeCacheRecord(desired)
	if err != nil {
		return domain.NodeCacheEntry{}, err
	}
	if err = store.database.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "workspace_id"}, {Name: "cache_key"}}, DoNothing: true,
	}).Omit(clause.Associations).Create(&record).Error; err != nil {
		return domain.NodeCacheEntry{}, err
	}
	var persisted model.NodeCacheEntry
	if err = store.database.WithContext(ctx).
		Where("workspace_id = ? AND cache_key = ?", record.WorkspaceID, record.CacheKey).
		First(&persisted).Error; err != nil {
		return domain.NodeCacheEntry{}, normalizeNotFound(err)
	}
	if !sameNodeCacheFact(persisted, record) {
		return domain.NodeCacheEntry{}, fmt.Errorf("node cache key %s has a different immutable fact", record.CacheKey)
	}
	return nodeCacheDomain(persisted)
}

func nodeCacheRecord(value domain.NodeCacheEntry) (model.NodeCacheEntry, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.NodeCacheEntry{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.NodeCacheEntry{}, err
	}
	sourceRunID, err := uuid.Parse(value.SourceWorkflowRunID)
	if err != nil {
		return model.NodeCacheEntry{}, err
	}
	sourceNodeID, err := uuid.Parse(value.SourceNodeRunID)
	if err != nil {
		return model.NodeCacheEntry{}, err
	}
	normalizedMaterial, cacheKey, err := domain.BuildNodeCacheKey(value.KeyMaterial)
	if err != nil || cacheKey != value.CacheKey {
		return model.NodeCacheEntry{}, errors.New("node cache key does not match its material")
	}
	material, err := json.Marshal(normalizedMaterial)
	if err != nil {
		return model.NodeCacheEntry{}, err
	}
	output, outputHash, err := domain.CanonicalNodeOutput(value.Output)
	if err != nil || outputHash != value.OutputHash || value.CreatedAt.IsZero() {
		return model.NodeCacheEntry{}, errors.New("node cache output fact is invalid")
	}
	return model.NodeCacheEntry{
		ID: id, WorkspaceID: workspaceID, CacheKey: cacheKey, KeyMaterial: datatypes.JSON(material),
		Output: datatypes.JSON(output), OutputHash: outputHash,
		SourceWorkflowRunID: sourceRunID, SourceNodeRunID: sourceNodeID, CreatedAt: value.CreatedAt.UTC(),
	}, nil
}

func nodeCacheDomain(value model.NodeCacheEntry) (domain.NodeCacheEntry, error) {
	var material domain.NodeCacheKeyMaterial
	if err := json.Unmarshal(value.KeyMaterial, &material); err != nil {
		return domain.NodeCacheEntry{}, err
	}
	normalizedMaterial, cacheKey, err := domain.BuildNodeCacheKey(material)
	if err != nil || cacheKey != value.CacheKey {
		return domain.NodeCacheEntry{}, errors.New("persisted node cache key material is invalid")
	}
	output, outputHash, err := domain.CanonicalNodeOutput(json.RawMessage(value.Output))
	if err != nil || outputHash != value.OutputHash {
		return domain.NodeCacheEntry{}, errors.New("persisted node cache output is invalid")
	}
	return domain.NodeCacheEntry{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), CacheKey: value.CacheKey,
		KeyMaterial: normalizedMaterial, Output: output, OutputHash: value.OutputHash,
		SourceWorkflowRunID: value.SourceWorkflowRunID.String(), SourceNodeRunID: value.SourceNodeRunID.String(),
		CreatedAt: value.CreatedAt,
	}, nil
}

func sameNodeCacheFact(left, right model.NodeCacheEntry) bool {
	var leftMaterial, rightMaterial domain.NodeCacheKeyMaterial
	if json.Unmarshal(left.KeyMaterial, &leftMaterial) != nil || json.Unmarshal(right.KeyMaterial, &rightMaterial) != nil {
		return false
	}
	_, leftKey, leftKeyErr := domain.BuildNodeCacheKey(leftMaterial)
	_, rightKey, rightKeyErr := domain.BuildNodeCacheKey(rightMaterial)
	_, leftOutputHash, leftOutputErr := domain.CanonicalNodeOutput(json.RawMessage(left.Output))
	_, rightOutputHash, rightOutputErr := domain.CanonicalNodeOutput(json.RawMessage(right.Output))
	return leftKeyErr == nil && rightKeyErr == nil && leftOutputErr == nil && rightOutputErr == nil &&
		left.WorkspaceID == right.WorkspaceID && left.CacheKey == right.CacheKey &&
		left.CacheKey == leftKey && leftKey == rightKey &&
		left.OutputHash == leftOutputHash && leftOutputHash == rightOutputHash && right.OutputHash == rightOutputHash
}

var _ application.NodeCacheRepository = (*Store)(nil)
