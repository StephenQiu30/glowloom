package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func (s *PurgeStore) workerActor(tx *gorm.DB, id uuid.UUID) (identityapp.Principal, error) {
	var row struct {
		ActorID, OrgID uuid.UUID
		Role           string
	}
	read := tx.Raw(`SELECT j.actor_id,j.org_id,u.role FROM media.purge_job j JOIN identity."user" u ON u.id=j.actor_id AND u.org_id=j.org_id WHERE j.id=?`, id).Scan(&row)
	if read.Error != nil {
		return identityapp.Principal{}, read.Error
	}
	if read.RowsAffected != 1 {
		return identityapp.Principal{}, application.ErrNotFound
	}
	return identityapp.Principal{ID: row.ActorID, OrgID: row.OrgID, Role: identitydomain.Role(row.Role)}, nil
}

func (s *PurgeStore) withLease(ctx context.Context, lease application.PurgeLease, ended bool, consume func(*gorm.DB, purgeJobRow, libraryRow, application.LibraryProjectFacts) error) error {
	if s == nil || s.db == nil || lease.Fence == uuid.Nil {
		return application.ErrUnavailable
	}
	execution, err := uuid.Parse(lease.Work.ExecutionID)
	if err != nil || execution == uuid.Nil {
		return domain.ErrPurgeConflict
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, lease.Actor); err != nil {
			return err
		}
		facts, err := s.access(ctx, tx, lease.Actor, lease.Scope, true)
		if err != nil {
			return err
		}
		library, err := readLibrary(tx, lease.Actor, lease.Scope, true)
		if err != nil {
			return err
		}
		row, err := s.readJob(tx, lease.Actor, lease.Work.JobID, true)
		if err != nil {
			return err
		}
		if row.Attempt != lease.Work.Attempt || row.WorkerFence == nil || *row.WorkerFence != lease.Fence || row.ExecutionID == nil || *row.ExecutionID != execution || row.LeaseUntil == nil || !row.LeaseUntil.After(s.clock()) || row.ProcessEnded != ended || row.scope().Kind != lease.Scope.Kind || !sameScopeProject(row.ProjectID, lease.Scope.ProjectID) {
			return domain.ErrPurgeConflict
		}
		return consume(tx, row, library, facts)
	})
}

// ClaimPurge refuses any unended prior process even when its lease has expired.
func (s *PurgeStore) ClaimPurge(ctx context.Context, work application.PurgeWorkID) (application.PurgeLease, error) {
	var result application.PurgeLease
	if s == nil || s.db == nil {
		return result, application.ErrUnavailable
	}
	execution, err := uuid.Parse(work.ExecutionID)
	if err != nil || execution == uuid.Nil || work.JobID == uuid.Nil || work.Attempt < 1 {
		return result, domain.ErrPurgeConflict
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		actor, err := s.workerActor(tx, work.JobID)
		if err != nil {
			return err
		}
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		row, err := s.readJob(tx, actor, work.JobID, false)
		if err != nil {
			return err
		}
		if _, err := s.access(ctx, tx, actor, row.scope(), true); err != nil {
			return err
		}
		if _, err := readLibrary(tx, actor, row.scope(), true); err != nil {
			return err
		}
		row, err = s.readJob(tx, actor, work.JobID, true)
		if err != nil {
			return err
		}
		if row.Attempt != work.Attempt {
			return domain.ErrPurgeConflict
		}
		result = application.PurgeLease{Work: work, Actor: actor, Scope: row.scope(), ItemCount: row.ItemCount, StartedAt: s.clock().UTC()}
		result.Job, err = purgeView(tx, row)
		if err != nil {
			return err
		}
		if row.Status == "succeeded" || row.Status == "partial_failed" || row.Status == "failed" || row.Status == "cancelled" {
			result.Done = true
			return nil
		}
		if !row.ProcessEnded || row.ExecutionUnconfirmed || row.Status != "queued" && row.Status != "cancel_requested" {
			return domain.ErrPurgeConflict
		}
		result.Fence = uuid.New()
		return libraryChanged(tx, `UPDATE media.purge_job SET status='running',stage='verifying',worker_fence=?,execution_id=?,lease_until=?,process_ended=false,revision=revision+1,updated_at=? WHERE id=? AND revision=?`, result.Fence, execution, s.clock().Add(45*time.Second).UTC(), s.clock().UTC(), row.ID, row.Revision)
	})
	return result, err
}

// HeartbeatPurge renews only the current authorized physical owner.
func (s *PurgeStore) HeartbeatPurge(ctx context.Context, lease application.PurgeLease) error {
	return s.withLease(ctx, lease, false, func(tx *gorm.DB, row purgeJobRow, _ libraryRow, _ application.LibraryProjectFacts) error {
		return libraryChanged(tx, `UPDATE media.purge_job SET lease_until=?,updated_at=? WHERE id=? AND worker_fence=?`, s.clock().Add(45*time.Second).UTC(), s.clock().UTC(), row.ID, lease.Fence)
	})
}

type purgeItemRow struct {
	ItemID uuid.UUID
	Frozen []byte
	Status string
}

func readPurgeItem(tx *gorm.DB, id uuid.UUID, index int) (purgeItemRow, application.FrozenPurgeItem, error) {
	var row purgeItemRow
	var frozen application.FrozenPurgeItem
	if index < 0 || index > 199 {
		return row, frozen, application.ErrNotFound
	}
	read := tx.Raw(`SELECT item_id,frozen,status FROM media.purge_item WHERE job_id=? AND item_index=? FOR UPDATE`, id, index).Scan(&row)
	if read.Error != nil {
		return row, frozen, read.Error
	}
	if read.RowsAffected != 1 || strictCopyMedia(row.Frozen, &frozen) != nil || frozen.ItemID != row.ItemID || frozen.CatalogRevision < 1 || len(frozen.Renditions) > 16 {
		return row, frozen, application.ErrUnavailable
	}
	return row, frozen, nil
}

func validatePurgeItem(tx *gorm.DB, row purgeJobRow, frozen application.FrozenPurgeItem) error {
	var item struct {
		Revision     int64
		CatalogState string
		PurgedAt     *time.Time
	}
	read := tx.Raw(`SELECT revision,catalog_state,purged_at FROM media.library_item WHERE id=? AND library_id=? FOR UPDATE`, frozen.ItemID, row.LibraryID).Scan(&item)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 || item.PurgedAt != nil || item.Revision != frozen.CatalogRevision || item.CatalogState != "trashed" {
		return domain.ErrPurgeConflict
	}
	if frozen.Asset == nil {
		return nil
	}
	if frozen.Asset.Validate() != nil || !frozen.Asset.IsDelete || frozen.Asset.ID != frozen.ItemID ||
		row.ProjectID != nil && (frozen.Asset.ProjectID != *row.ProjectID || frozen.Asset.Personal != nil) ||
		row.ProjectID == nil && (frozen.Asset.ProjectID != uuid.Nil || frozen.Asset.Personal == nil || frozen.Asset.Personal.OrgID != row.OrgID || frozen.Asset.Personal.ActorID != row.ActorID) {
		return application.ErrObjectMismatch
	}
	var actual assetRow
	read = tx.Raw(`SELECT * FROM media.media_asset WHERE id=? FOR UPDATE`, frozen.Asset.ID).Scan(&actual)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 || !equalCopyMedia(actual.domain(), *frozen.Asset) || !actual.IsDelete {
		return application.ErrObjectMismatch
	}
	var rends []renditionRow
	if err := tx.Raw(`SELECT * FROM media.rendition WHERE media_asset_id=? AND NOT is_delete ORDER BY kind,id FOR UPDATE`, frozen.Asset.ID).Scan(&rends).Error; err != nil {
		return err
	}
	if len(rends) != len(frozen.Renditions) {
		return application.ErrObjectMismatch
	}
	byID := make(map[uuid.UUID]domain.Rendition, len(rends))
	for _, r := range rends {
		byID[r.ID] = r.domain()
	}
	for _, r := range frozen.Renditions {
		if !equalCopyMedia(r, byID[r.ID]) {
			return application.ErrObjectMismatch
		}
	}
	return nil
}

// StartPurgeItem rechecks current ownership and reference readers before I/O.
func (s *PurgeStore) StartPurgeItem(ctx context.Context, lease application.PurgeLease, index int) (bool, error) {
	done := false
	err := s.withLease(ctx, lease, false, func(tx *gorm.DB, row purgeJobRow, library libraryRow, facts application.LibraryProjectFacts) error {
		state, frozen, err := readPurgeItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status == "blocked" || state.Status == "succeeded" || state.Status == "cancelled" {
			done = true
			return nil
		}
		if row.CancellationRequested {
			var err error
			done, err = s.cancelPendingItem(ctx, tx, row, lease.Actor, index, library, facts)
			if err != nil || done {
				return err
			}
		}
		if err := validatePurgeItem(tx, row, frozen); err != nil {
			return err
		}
		if frozen.Asset != nil && row.ProjectID != nil {
			if s.references == nil || s.references(tx) == nil {
				return application.ErrUnavailable
			}
			used, err := s.references(tx).HasMediaReferences(ctx, lease.Actor, *row.ProjectID, frozen.Asset.ID)
			if err != nil {
				return err
			}
			if used {
				return domain.ErrPurgeConflict
			}
		}
		return libraryChanged(tx, `UPDATE media.purge_item SET status='running',failure_code=NULL WHERE job_id=? AND item_index=? AND status IN ('queued','running','needs_reconciliation')`, row.ID, index)
	})
	return done, err
}

// EndPurgePhysical proves only cessation under the original fence. Revoking the
// creator does not prevent this fact; it grants no read or destructive access.
func (s *PurgeStore) EndPurgePhysical(ctx context.Context, lease application.PurgeLease) error {
	if s == nil || s.db == nil || lease.Fence == uuid.Nil {
		return application.ErrUnavailable
	}
	execution, err := uuid.Parse(lease.Work.ExecutionID)
	if err != nil {
		return domain.ErrPurgeConflict
	}
	return libraryChanged(s.db.WithContext(ctx), `UPDATE media.purge_job SET process_ended=true,execution_unconfirmed=false,revision=revision+1,updated_at=? WHERE id=? AND actor_id=? AND org_id=? AND attempt=? AND worker_fence=? AND execution_id=?`, s.clock().UTC(), lease.Work.JobID, lease.Actor.ID, lease.Actor.OrgID, lease.Work.Attempt, lease.Fence, execution)
}

// FailPurgeItem keeps even a known corrupt source occupied until explicit
// reconciliation. It never restores a partially deleted original as readable.
func (s *PurgeStore) FailPurgeItem(ctx context.Context, lease application.PurgeLease, index int, code string) error {
	return s.withLease(ctx, lease, false, func(tx *gorm.DB, row purgeJobRow, library libraryRow, facts application.LibraryProjectFacts) error {
		state, _, err := readPurgeItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status == "succeeded" || state.Status == "blocked" || state.Status == "cancelled" {
			return domain.ErrPurgeConflict
		}
		if code == "cancelled" && row.CancellationRequested {
			done, err := s.cancelPendingItem(ctx, tx, row, lease.Actor, index, library, facts)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
		if err := libraryChanged(tx, `UPDATE media.purge_item SET status='needs_reconciliation',failure_code=? WHERE job_id=? AND item_index=?`, code, row.ID, index); err != nil {
			return err
		}
		return libraryChanged(tx, `UPDATE media.purge_job SET needs_reconciliation=true,revision=revision+1,updated_at=? WHERE id=?`, s.clock().UTC(), row.ID)
	})
}
