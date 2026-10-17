package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// StorageRemoval identifies one cancelled work's durable physical cleanup debt.
type StorageRemoval struct {
	IntentID uuid.UUID
	Location domain.ObjectLocation
}

// StorageRemovalRepository retains ownership and arbiters acceptance/removal.
type StorageRemovalRepository interface {
	PendingWorkRemovals(context.Context, int) ([]StorageRemoval, error)
	BeginWorkRemoval(context.Context, uuid.UUID, domain.ObjectLocation) (bool, error)
	ConfirmWorkRemoval(context.Context, uuid.UUID, domain.ObjectLocation) error
}

// StorageRemovalObjects never interprets a bare key using the active profile.
type StorageRemovalObjects interface {
	Remove(context.Context, domain.ObjectRef) error
	Stat(context.Context, domain.ObjectRef) (ObjectInfo, error)
}

// StorageCleanup retries only fenced cancelled work; unknown results retain debt.
type StorageCleanup struct {
	repo    StorageRemovalRepository
	objects StorageRemovalObjects
}

// NewStorageCleanup injects exact-ref I/O and the owning persistent fence.
func NewStorageCleanup(repo StorageRemovalRepository, objects StorageRemovalObjects) *StorageCleanup {
	return &StorageCleanup{repo: repo, objects: objects}
}

// ReconcileBatch never clears debt on Delete success alone: it requires an
// authoritative missing-object result, preserving 403 and unknown outcomes.
func (s *StorageCleanup) ReconcileBatch(ctx context.Context, limit int) (int, error) {
	if s == nil || s.repo == nil || s.objects == nil || limit < 1 || limit > 1000 {
		return 0, ErrUnavailable
	}
	items, err := s.repo.PendingWorkRemovals(ctx, limit)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, item := range items {
		if item.IntentID == uuid.Nil || item.Location.ID == uuid.Nil || item.Location.Ref.Validate() != nil {
			return removed, ErrObjectMismatch
		}
		allowed, err := s.repo.BeginWorkRemoval(ctx, item.IntentID, item.Location)
		if err != nil {
			return removed, err
		}
		if !allowed {
			continue
		}
		removeCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		err = s.objects.Remove(removeCtx, item.Location.Ref)
		if err == nil || errors.Is(err, ErrObjectMissing) {
			_, err = s.objects.Stat(removeCtx, item.Location.Ref)
			if errors.Is(err, ErrObjectMissing) {
				err = s.repo.ConfirmWorkRemoval(ctx, item.IntentID, item.Location)
				if err == nil {
					removed++
				}
			} else if err == nil {
				err = ErrObjectMismatch
			}
		}
		cancel()
		if err != nil {
			return removed, err
		}
	}
	return removed, nil
}
