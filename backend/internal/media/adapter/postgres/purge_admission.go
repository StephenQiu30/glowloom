package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
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
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := purgeLock(tx, actor.ID, in.Key); err != nil {
			return err
		}
		if _, err := s.access(ctx, tx, actor, in.Scope, false); err != nil {
			return err
		}
		var err error
		cached, err = purgeReplay(tx, actor, in.Key, hash, "create")
		return err
	})
	if err != nil {
		return result, err
	}
	if cached != nil {
		return *cached, nil
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := purgeLock(tx, actor.ID, in.Key); err != nil {
			return err
		}
		facts, err := s.access(ctx, tx, actor, in.Scope, true)
		if err != nil {
			return err
		}
		replay, err := purgeReplay(tx, actor, in.Key, hash, "create")
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
			if item.Revision != wanted.Revision || item.State != "trashed" || item.Revision >= 2147483647 {
				return domain.ErrPurgeConflict
			}
			var purged bool
			if err := tx.Raw(`SELECT purged_at IS NOT NULL FROM media.library_item WHERE id=? AND library_id=?`, item.ID, library.ID).Scan(&purged).Error; err != nil {
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
		if err := libraryChanged(tx, `INSERT INTO media.purge_job(id,org_id,actor_id,library_id,scope_kind,project_id,item_count,status,stage,created_at,updated_at) VALUES(?,?,?,?,?,?,?,'queued','frozen',?,?)`, job, actor.OrgID, actor.ID, library.ID, string(in.Scope.Kind), in.Scope.ProjectID, len(in.Items), now, now); err != nil {
			return err
		}
		accepted := 0
		for index, wanted := range in.Items {
			item := items[wanted.ID]
			frozen := application.FrozenPurgeItem{ItemID: item.ID, CatalogRevision: item.Revision, Renditions: []domain.Rendition{}}
			state := "queued"
			var failure *string
			if item.AssetID != nil && in.Scope.Kind == domain.LibraryProject {
				if s.references == nil || s.references(tx) == nil {
					return application.ErrUnavailable
				}
				used, err := s.references(tx).HasMediaReferences(ctx, actor, *in.Scope.ProjectID, *item.AssetID)
				if err != nil {
					return err
				}
				if used {
					state = "blocked"
					code := "in_use"
					failure = &code
				}
			}
			var file *application.LibraryMediaFile
			if state == "queued" && item.AssetID != nil {
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
				if err := libraryChanged(tx, `UPDATE media.media_asset SET is_delete=true,delete_time=?,purge_after=?,revision=?,update_time=? WHERE id=? AND revision=? AND NOT is_delete`, deleted.DeleteTime, deleted.PurgeAfter, deleted.Revision, deleted.UpdateTime, deleted.ID, actual.Asset.Revision); err != nil {
					return err
				}
				frozen.Asset, frozen.Renditions = &deleted, actual.Renditions
				file = &actual
			}
			body, err := json.Marshal(frozen)
			if err != nil || len(body) > 1<<20 {
				return application.ErrUnavailable
			}
			if err := libraryChanged(tx, `INSERT INTO media.purge_item(job_id,item_index,item_id,asset_id,frozen,status,failure_code) VALUES(?,?,?,?,?,?,?)`, job, index, item.ID, item.AssetID, body, state, failure); err != nil {
				return err
			}
			if file != nil {
				if err := libraryChanged(tx, `INSERT INTO media.purge_object(job_id,item_index,object_key,rendition_kind,byte_size,sha256) VALUES(?,?,?,'',?,?)`, job, index, file.Asset.ObjectKey, file.Asset.ByteSize, file.Asset.SHA256); err != nil {
					return err
				}
				for _, rend := range file.Renditions {
					if err := libraryChanged(tx, `INSERT INTO media.purge_object(job_id,item_index,object_key,rendition_kind,byte_size) VALUES(?,?,?,?,?)`, job, index, rend.ObjectKey, string(rend.Kind), rend.ByteSize); err != nil {
						return err
					}
				}
			}
			if state == "queued" {
				accepted++
			}
		}
		if accepted > 0 {
			if err := libraryChanged(tx, `UPDATE media.library SET revision=revision+1,update_time=? WHERE id=? AND revision=?`, now, library.ID, library.Revision); err != nil {
				return err
			}
			if in.Scope.Kind == domain.LibraryProject {
				if _, err := s.project(tx).TouchContent(ctx, actor, facts.ProjectID, facts.Revision); err != nil {
					return err
				}
			}
		} else if err := libraryChanged(tx, `UPDATE media.purge_job SET status='failed',stage='completed' WHERE id=?`, job); err != nil {
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
		return recordPurgeCommand(tx, actor, in.Key, hash, "create", result, accepted > 0)
	})
	return result, err
}
