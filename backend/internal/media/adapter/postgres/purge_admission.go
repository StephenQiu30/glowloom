package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// CreatePurge freezes SQL ownership before dispatch. Current authority precedes
// permanent replay; only new commands evaluate the displayed catalog revisions.
func (s *PurgeStore) CreatePurge(ctx context.Context, actor identityapp.Principal, in application.PurgeInput) (domain.PurgeJob, error) {
	var result domain.PurgeJob
	if s == nil || s.db == nil {
		return result, application.ErrUnavailable
	}
	if err := in.Validate(); err != nil {
		return result, err
	}
	hash, err := purgeHash(in)
	if err != nil {
		return result, err
	}
	var cached *domain.PurgeJob
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := purgeLock(tx, actor.ID, in.Key); err != nil {
			return err
		}
		if _, err := s.access(ctx, tx, actor, in.Scope, false); err != nil {
			return err
		}
		var err error
		cached, err = purgeReplay(tx, actor, in.Key, hash, application.PurgeActionCreate)
		return err
	})
	if err != nil {
		return result, err
	}
	if cached != nil {
		return *cached, nil
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := purgeLock(tx, actor.ID, in.Key); err != nil {
			return err
		}
		facts, err := s.access(ctx, tx, actor, in.Scope, true)
		if err != nil {
			return err
		}
		replay, err := purgeReplay(tx, actor, in.Key, hash, application.PurgeActionCreate)
		if err != nil {
			return err
		}
		if replay != nil {
			result = *replay
			return nil
		}
		if facts.Revision != in.ExpectedProjectRevision {
			return domain.ErrPurgeConflict
		}
		library, err := readLibrary(tx, actor, in.Scope, true)
		if err != nil {
			return err
		}
		if library.Revision != in.ExpectedRevision || library.Revision < 1 || library.Revision >= 2147483647 {
			return domain.ErrPurgeConflict
		}
		if err := s.scopeBusy(ctx, tx, actor, in.Scope, library.ID); err != nil {
			return err
		}
		ids := make([]uuid.UUID, 0, len(in.Items))
		for _, item := range in.Items {
			ids = append(ids, item.ID)
		}
		items, err := lockLibraryItems(tx, actor, in.Scope, library.ID, ids)
		if err != nil {
			return err
		}
		for _, wanted := range in.Items {
			item := items[wanted.ID]
			if item.Revision != wanted.Revision || item.State != domain.LibraryItemStateTrashed || item.Revision >= 2147483647 {
				return domain.ErrPurgeConflict
			}
			purged, err := storageExists(tx.Model(&libraryItemRow{}).Where("id = ? AND library_id = ? AND purged_at IS NOT NULL", item.ID, library.ID))
			if err != nil {
				return err
			}
			if purged {
				return domain.ErrPurgeConflict
			}
		}
		job := uuid.NewSHA1(in.Key, []byte("media-purge-job/"+actor.ID.String()))
		now := s.clock().UTC().Truncate(time.Microsecond)
		if now.IsZero() {
			return application.ErrUnavailable
		}
		jobRecord := purgeJobRow{ID: job, OrgID: actor.OrgID, ActorID: actor.ID, LibraryID: library.ID, ScopeKind: string(in.Scope.Kind), ProjectID: in.Scope.ProjectID, ItemCount: len(in.Items), Status: domain.PurgeStatusQueued, Stage: domain.PurgeStageFrozen, Attempt: 1, Revision: 1, ProcessEnded: true, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&jobRecord).Error; err != nil {
			return err
		}
		accepted := 0
		for index, wanted := range in.Items {
			item := items[wanted.ID]
			frozen := application.FrozenPurgeItem{ItemID: item.ID, CatalogRevision: item.Revision, Renditions: []domain.Rendition{}}
			state := domain.PurgeItemStatusQueued
			var failure *string
			if item.AssetID != nil {
				if s.references == nil || s.references(tx) == nil {
					return application.ErrUnavailable
				}
				project := uuid.Nil
				if in.Scope.ProjectID != nil {
					project = *in.Scope.ProjectID
				}
				used, err := s.references(tx).HasMediaReferences(ctx, actor, project, *item.AssetID)
				if err != nil {
					return err
				}
				if used {
					state = domain.PurgeItemStatusBlocked
					code := "in_use"
					failure = &code
				}
			}
			var file *application.LibraryMediaFile
			if state == domain.PurgeItemStatusQueued && item.AssetID != nil {
				actual, err := transferSourceFile(tx, actor, in.Scope, *item.AssetID)
				if err != nil {
					return err
				}
				if err := validatePurgeFile(actual); err != nil {
					return err
				}
				deleted, err := actual.Asset.Delete(now)
				if err != nil {
					return err
				}
				if err := updateOne(tx, "media.media_asset", map[string]any{"id": deleted.ID, "revision": actual.Asset.Revision, "is_delete": false}, map[string]any{"is_delete": true, "delete_time": deleted.DeleteTime, "purge_after": deleted.PurgeAfter, "revision": deleted.Revision, "update_time": deleted.UpdateTime}, application.ErrLibraryConflict); err != nil {
					return err
				}
				frozen.Asset, frozen.Renditions = &deleted, actual.Renditions
				file = &actual
			}
			body, err := json.Marshal(frozen)
			if err != nil || len(body) > 1<<20 {
				return application.ErrUnavailable
			}
			purgeItem := purgeItemRow{JobID: job, ItemIndex: index, ItemID: item.ID, AssetID: item.AssetID, Frozen: body, Status: state, FailureCode: failure}
			if err := tx.Create(&purgeItem).Error; err != nil {
				return err
			}
			if file != nil {
				object := purgeObjectRow{JobID: job, ItemIndex: index, ObjectKey: file.Asset.ObjectKey, ObjectLocationID: locationID(file.Asset.ObjectLocation), RenditionKind: "", ByteSize: &file.Asset.ByteSize, SHA256: file.Asset.SHA256}
				if err := tx.Create(&object).Error; err != nil {
					return err
				}
				for _, rend := range file.Renditions {
					object := purgeObjectRow{JobID: job, ItemIndex: index, ObjectKey: rend.ObjectKey, ObjectLocationID: locationID(rend.ObjectLocation), RenditionKind: string(rend.Kind), ByteSize: rend.ByteSize}
					if err := tx.Create(&object).Error; err != nil {
						return err
					}
				}
			}
			if state == domain.PurgeItemStatusQueued {
				accepted++
			}
		}
		if accepted > 0 {
			if err := updateOne(tx, "media.library", map[string]any{"id": library.ID, "revision": library.Revision}, map[string]any{"revision": library.Revision + 1, "update_time": now}, application.ErrLibraryConflict); err != nil {
				return err
			}
			if in.Scope.Kind == domain.LibraryProject {
				if _, err := s.project(tx).TouchContent(ctx, actor, facts.ProjectID, facts.Revision); err != nil {
					return err
				}
			}
		} else if err := updateOne(tx, "media.purge_job", map[string]any{"id": job}, map[string]any{"status": domain.PurgeStatusFailed, "stage": domain.PurgeStageCompleted}, application.ErrLibraryConflict); err != nil {
			return err
		}
		row, err := s.readJob(tx, actor, job, false)
		if err != nil {
			return err
		}
		result, err = purgeView(tx, row)
		if err != nil {
			return err
		}
		return recordPurgeCommand(tx, actor, in.Key, hash, application.PurgeActionCreate, result, accepted > 0)
	})
	return result, err
}
