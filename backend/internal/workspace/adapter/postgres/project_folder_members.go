package postgres

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func readPlacement(tx *gorm.DB, actor identityapp.Principal, project uuid.UUID) (domain.FolderPlacement, error) {
	out := domain.FolderPlacement{OrgID: actor.OrgID, ActorID: actor.ID, ProjectID: project}
	read := tx.Raw(`SELECT org_id,actor_id,project_id,folder_id,revision FROM workspace.project_folder_placement WHERE actor_id=? AND org_id=? AND project_id=? FOR UPDATE`, actor.ID, actor.OrgID, project).Scan(&out)
	if read.Error != nil {
		return domain.FolderPlacement{}, fmt.Errorf("read project library assignment: %w", read.Error)
	}
	return out, out.Validate()
}

func bumpFolder(tx *gorm.DB, actor identityapp.Principal, id uuid.UUID, now time.Time) error {
	write := tx.Exec(`UPDATE workspace.project_folder SET revision=revision+1,update_time=? WHERE id=? AND org_id=? AND actor_id=? AND NOT is_delete AND revision<?`, now, id, actor.OrgID, actor.ID, math.MaxInt32)
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected != 1 {
		return domain.ErrProjectFolderRevisionConflict
	}
	return nil
}

func (s *FolderStore) moveProjectFolder(_ context.Context, tx *gorm.DB, actor identityapp.Principal, in application.FolderChangeInput, now time.Time) (application.FolderChangeResult, error) {
	var target *domain.ProjectFolder
	if in.FolderID != uuid.Nil {
		f, err := readFolder(tx, actor, in.FolderID, false)
		if err != nil {
			return application.FolderChangeResult{}, err
		}
		if f.Revision != in.ExpectedRevision {
			return application.FolderChangeResult{}, domain.ErrProjectFolderRevisionConflict
		}
		target = &f
	}
	project, err := readLifecycleProject(tx, actor.OrgID, in.ProjectID, true)
	if err != nil {
		return application.FolderChangeResult{}, err
	}
	if project.Project.IsDelete {
		return application.FolderChangeResult{}, application.ErrProjectNotFound
	}
	if err := project.Project.CanWrite(); err != nil {
		return application.FolderChangeResult{}, err
	}
	if project.Project.Revision != in.ExpectedProjectRevision {
		return application.FolderChangeResult{}, domain.ErrProjectRevisionConflict
	}
	before, err := readPlacement(tx, actor, in.ProjectID)
	if err != nil {
		return application.FolderChangeResult{}, err
	}
	if before.Revision != in.ExpectedPlacementRevision {
		return application.FolderChangeResult{}, domain.ErrProjectFolderRevisionConflict
	}
	if before.FolderID == nil && in.FolderID == uuid.Nil || before.FolderID != nil && *before.FolderID == in.FolderID {
		return application.FolderChangeResult{Folder: target, Placement: &before}, nil
	}
	if before.FolderID != nil {
		if err := bumpFolder(tx, actor, *before.FolderID, now); err != nil {
			return application.FolderChangeResult{}, err
		}
	}
	var id any
	if target != nil {
		if err := bumpFolder(tx, actor, target.ID, now); err != nil {
			return application.FolderChangeResult{}, err
		}
		target.Revision++
		target.UpdateTime = now
		id = target.ID
	}
	after := before
	after.Revision++
	after.FolderID = nil
	if target != nil {
		after.FolderID = &target.ID
	}
	write := tx.Exec(`INSERT INTO workspace.project_folder_placement(org_id,actor_id,project_id,folder_id,revision,create_time,update_time) VALUES(?,?,?,?,?,?,?) ON CONFLICT(actor_id,project_id) DO UPDATE SET folder_id=excluded.folder_id,revision=excluded.revision,update_time=excluded.update_time WHERE workspace.project_folder_placement.org_id=? AND workspace.project_folder_placement.revision=?`, actor.OrgID, actor.ID, in.ProjectID, id, after.Revision, now, now, actor.OrgID, before.Revision)
	if write.Error != nil {
		return application.FolderChangeResult{}, write.Error
	}
	if write.RowsAffected != 1 {
		return application.FolderChangeResult{}, domain.ErrProjectFolderRevisionConflict
	}
	return application.FolderChangeResult{Folder: target, Placement: &after}, nil
}

func (s *FolderStore) recycleFolder(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, in application.FolderChangeInput, now time.Time) (application.FolderChangeResult, error) {
	f, err := readFolder(tx, actor, in.FolderID, false)
	if err != nil {
		return application.FolderChangeResult{}, err
	}
	if f.Revision != in.ExpectedRevision {
		return application.FolderChangeResult{}, domain.ErrProjectFolderRevisionConflict
	}
	if s.work == nil {
		return application.FolderChangeResult{}, application.ErrProjectDependencyUnavailable
	}
	guard := s.work(tx)
	if guard == nil {
		return application.FolderChangeResult{}, application.ErrProjectDependencyUnavailable
	}
	// Lock even already-deleted members so concurrent Restore cannot slip into
	// an unobserved gap between the membership query and the lifecycle update.
	var ids []uuid.UUID
	if err := tx.Raw(`SELECT p.id FROM workspace.project p JOIN workspace.project_folder_placement m ON m.project_id=p.id AND m.org_id=p.org_id WHERE m.org_id=? AND m.actor_id=? AND m.folder_id=? ORDER BY p.id FOR UPDATE OF p`, actor.OrgID, actor.ID, f.ID).Scan(&ids).Error; err != nil {
		return application.FolderChangeResult{}, fmt.Errorf("lock complete directory membership: %w", err)
	}
	out := application.FolderChangeResult{RecycledProjectIDs: []uuid.UUID{}}
	for _, id := range ids {
		before, err := readLifecycleProject(tx, actor.OrgID, id, true)
		if err != nil {
			return application.FolderChangeResult{}, err
		}
		blocked, err := guard.HasInflightWork(ctx, actor, id)
		if err != nil {
			return application.FolderChangeResult{}, fmt.Errorf("read directory member work: %w", err)
		}
		if blocked {
			return application.FolderChangeResult{}, domain.ErrProjectHasInflightOperations
		}
		if before.Project.IsDelete {
			continue
		}
		change := application.ProjectChangeInput{Action: "delete", IdempotencyKey: in.IdempotencyKey, Patch: application.UpdateProjectInput{ProjectID: id, ExpectedRevision: before.Project.Revision, RequestID: in.RequestID}}
		after, events, err := application.PrepareProjectChange(actor, change, before.Project, now, false)
		if err != nil {
			return application.FolderChangeResult{}, err
		}
		write := tx.Exec(`UPDATE workspace.project SET is_delete=?,delete_time=?,purge_after=?,revision=? WHERE id=? AND org_id=? AND revision=? AND NOT is_delete`, after.IsDelete, after.DeleteTime, after.PurgeAfter, after.Revision, id, actor.OrgID, before.Project.Revision)
		if write.Error != nil {
			return application.FolderChangeResult{}, write.Error
		}
		if write.RowsAffected != 1 {
			return application.FolderChangeResult{}, domain.ErrProjectRevisionConflict
		}
		for _, event := range events {
			if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, event.ID, event.Topic, event.PartitionKey, string(event.Payload)).Error; err != nil {
				return application.FolderChangeResult{}, err
			}
		}
		out.RecycledProjectIDs = append(out.RecycledProjectIDs, id)
	}
	if err := tx.Exec(`UPDATE workspace.project_folder_placement SET folder_id=NULL,revision=revision+1,update_time=? WHERE org_id=? AND actor_id=? AND folder_id=?`, now, actor.OrgID, actor.ID, f.ID).Error; err != nil {
		return application.FolderChangeResult{}, err
	}
	f.IsDelete = true
	f.DeleteTime = &now
	f.Revision++
	f.UpdateTime = now
	write := tx.Exec(`UPDATE workspace.project_folder SET is_delete=true,delete_time=?,revision=?,update_time=? WHERE id=? AND org_id=? AND actor_id=? AND revision=? AND NOT is_delete`, now, f.Revision, now, f.ID, actor.OrgID, actor.ID, in.ExpectedRevision)
	if write.Error != nil {
		return application.FolderChangeResult{}, write.Error
	}
	if write.RowsAffected != 1 {
		return application.FolderChangeResult{}, domain.ErrProjectFolderRevisionConflict
	}
	out.Folder = &f
	return out, nil
}
