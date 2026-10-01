package postgres

import (
	"context"
	"reflect"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// CopyPlacementFreeze binds personal classification without changing project content identity.
type CopyPlacementFreeze struct {
	Placement      domain.FolderPlacement
	FolderRevision int64
}
type copyPlacementSnapshot struct {
	Source               domain.FolderPlacement
	SourceFolderRevision int64
	Target               domain.FolderPlacement
}

func requireCopyPlacementTransaction(tx *gorm.DB) error {
	if tx == nil || tx.Statement == nil {
		return application.ErrProjectDependencyUnavailable
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return application.ErrProjectDependencyUnavailable
	}
	return nil
}

// FreezeCopyPlacement runs after the actor library lock and before the source project lock.
func FreezeCopyPlacement(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, source uuid.UUID, expect *application.CopyPlacementExpectation) (CopyPlacementFreeze, error) {
	if err := requireCopyPlacementTransaction(tx); err != nil {
		return CopyPlacementFreeze{}, err
	}
	if source == uuid.Nil || expect != nil && expect.Validate() != nil {
		return CopyPlacementFreeze{}, domain.ErrInvalidProjectCopy
	}
	tx = tx.WithContext(ctx)
	p, err := readPlacement(tx, actor, source)
	if err != nil {
		return CopyPlacementFreeze{}, err
	}
	out := CopyPlacementFreeze{Placement: p}
	if p.FolderID != nil {
		f, err := readFolder(tx, actor, *p.FolderID, false)
		if err != nil {
			return out, err
		}
		out.FolderRevision = f.Revision
	}
	if expect != nil && (expect.ExpectedPlacementRevision != p.Revision || !reflect.DeepEqual(expect.FolderID, p.FolderID) || expect.ExpectedFolderRevision != out.FolderRevision) {
		return CopyPlacementFreeze{}, domain.ErrProjectFolderRevisionConflict
	}
	return out, nil
}

// AttachCopyPlacement admits the invisible target in the same transaction and updates membership CAS.
func AttachCopyPlacement(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, target uuid.UUID, frozen CopyPlacementFreeze, now time.Time) (domain.FolderPlacement, error) {
	if err := requireCopyPlacementTransaction(tx); err != nil {
		return domain.FolderPlacement{}, err
	}
	source := frozen.Placement
	if source.Validate() != nil || source.OrgID != actor.OrgID || source.ActorID != actor.ID || target == uuid.Nil || target == source.ProjectID || now.IsZero() || source.FolderID == nil && frozen.FolderRevision != 0 || source.FolderID != nil && frozen.FolderRevision < 1 {
		return domain.FolderPlacement{}, domain.ErrInvalidProjectCopy
	}
	tx = tx.WithContext(ctx)
	var folder any
	if source.FolderID != nil {
		folder = *source.FolderID
		write := tx.Exec(`UPDATE workspace.project_folder SET revision=revision+1,update_time=? WHERE id=? AND org_id=? AND actor_id=? AND revision=? AND revision<2147483647 AND NOT is_delete`, now, folder, actor.OrgID, actor.ID, frozen.FolderRevision)
		if write.Error != nil {
			return domain.FolderPlacement{}, write.Error
		}
		if write.RowsAffected != 1 {
			return domain.FolderPlacement{}, domain.ErrProjectFolderRevisionConflict
		}
	}
	out := domain.FolderPlacement{OrgID: actor.OrgID, ActorID: actor.ID, ProjectID: target, FolderID: source.FolderID, Revision: 1}
	// Selecting the just-created copying target prevents accidental attachment of an unrelated project.
	write := tx.Exec(`INSERT INTO workspace.project_folder_placement(org_id,actor_id,project_id,folder_id,revision,create_time,update_time)SELECT ?,?,p.id,?,1,?,? FROM workspace.project p WHERE p.id=? AND p.org_id=? AND p.status='copying' AND NOT p.is_delete`, actor.OrgID, actor.ID, folder, now, now, target, actor.OrgID)
	if write.Error != nil {
		return domain.FolderPlacement{}, write.Error
	}
	if write.RowsAffected != 1 {
		return domain.FolderPlacement{}, domain.ErrInvalidProjectCopy
	}
	return out, nil
}

func verifyCopyPlacement(tx *gorm.DB, job domain.ProjectCopyJob, frozen copyPlacementSnapshot) error {
	source, target := frozen.Source, frozen.Target
	if source.Validate() != nil || target.Validate() != nil || source.OrgID != job.OrgID || source.ActorID != job.ActorID || source.ProjectID != job.SourceProjectID || target.OrgID != job.OrgID || target.ActorID != job.ActorID || target.ProjectID != job.TargetProjectID || target.Revision != 1 || !reflect.DeepEqual(source.FolderID, target.FolderID) || source.FolderID == nil && frozen.SourceFolderRevision != 0 || source.FolderID != nil && frozen.SourceFolderRevision < 1 {
		return domain.ErrInvalidProjectCopy
	}
	var actual domain.FolderPlacement
	read := tx.Raw(`SELECT org_id,actor_id,project_id,folder_id,revision FROM workspace.project_folder_placement WHERE org_id=? AND actor_id=? AND project_id=? FOR SHARE`, job.OrgID, job.ActorID, job.TargetProjectID).Scan(&actual)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 || !reflect.DeepEqual(actual, target) {
		return domain.ErrInvalidProjectCopy
	}
	return nil
}
