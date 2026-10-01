package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// FolderMediaFactory binds media eligibility locks to the directory transaction.
type FolderMediaFactory func(*gorm.DB) application.FolderMediaReader

// FolderStore owns personal classification and guarded whole-folder recycling.
type FolderStore struct {
	db    *gorm.DB
	work  ProjectWorkFactory
	media FolderMediaFactory
}

// NewFolderStore explicitly injects current work and cover ownership evidence.
func NewFolderStore(db *gorm.DB, work ProjectWorkFactory, media FolderMediaFactory) *FolderStore {
	return &FolderStore{db: db, work: work, media: media}
}

// LockProjectLibrary serializes one actor's assignments before taking project locks.
// Copy admission must take the same lock before its source project FOR UPDATE.
func LockProjectLibrary(ctx context.Context, tx *gorm.DB, actor identityapp.Principal) error {
	if tx == nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil {
		return application.ErrProjectDependencyUnavailable
	}
	var locked int
	if err := tx.WithContext(ctx).Raw(`SELECT 1 FROM pg_advisory_xact_lock(69370,hashtext(?))`, actor.OrgID.String()+":"+actor.ID.String()).Scan(&locked).Error; err != nil {
		return fmt.Errorf("lock personal project library: %w", err)
	}
	return nil
}

type folderRow struct {
	ID, OrgID, ActorID           uuid.UUID
	Name                         string
	CoverProjectID, CoverAssetID *uuid.UUID
	Revision                     int64
	IsDelete                     bool
	DeleteTime                   *time.Time
	CreateTime, UpdateTime       time.Time
}

func (r folderRow) folder() (domain.ProjectFolder, error) {
	f := domain.ProjectFolder{ID: r.ID, OrgID: r.OrgID, ActorID: r.ActorID, Name: r.Name, Revision: r.Revision, IsDelete: r.IsDelete, DeleteTime: r.DeleteTime, CreateTime: r.CreateTime, UpdateTime: r.UpdateTime}
	if r.CoverProjectID != nil && r.CoverAssetID != nil {
		f.Cover = &domain.FolderCover{ProjectID: *r.CoverProjectID, AssetID: *r.CoverAssetID}
	} else if r.CoverProjectID != nil || r.CoverAssetID != nil {
		return f, application.ErrProjectDependencyUnavailable
	}
	return f, f.Validate()
}

func readFolder(tx *gorm.DB, actor identityapp.Principal, id uuid.UUID, deleted bool) (domain.ProjectFolder, error) {
	var row folderRow
	query := `SELECT id,org_id,actor_id,name,cover_project_id,cover_asset_id,revision,is_delete,delete_time,create_time,update_time FROM workspace.project_folder WHERE id=? AND org_id=? AND actor_id=?`
	if !deleted {
		query += ` AND NOT is_delete`
	}
	read := tx.Raw(query+` FOR UPDATE`, id, actor.OrgID, actor.ID).Scan(&row)
	if read.Error != nil {
		return domain.ProjectFolder{}, fmt.Errorf("read personal project directory: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return domain.ProjectFolder{}, domain.ErrProjectFolderNotFound
	}
	return row.folder()
}

// ChangeFolder commits all project transitions, assignments and receipts atomically.
func (s *FolderStore) ChangeFolder(ctx context.Context, actor identityapp.Principal, in application.FolderChangeInput, now time.Time) (application.FolderChangeResult, error) {
	if s == nil || s.db == nil || now.IsZero() {
		return application.FolderChangeResult{}, application.ErrProjectDependencyUnavailable
	}
	if err := in.Validate(); err != nil {
		return application.FolderChangeResult{}, err
	}
	request := struct {
		Contract string
		Org      uuid.UUID
		Input    application.FolderChangeInput
	}{"workspace.folder.v1", actor.OrgID, in}
	request.Input.RequestID = ""
	request.Input.IdempotencyKey = uuid.Nil
	if request.Input.Name != nil {
		name := strings.TrimSpace(*request.Input.Name)
		request.Input.Name = &name
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return application.FolderChangeResult{}, fmt.Errorf("encode directory fingerprint: %w", err)
	}
	hash := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(hash[:])
	var out application.FolderChangeResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := LockProjectLibrary(ctx, tx, actor); err != nil {
			return err
		}
		var receipt struct {
			OrgID         uuid.UUID
			RequestSHA256 string
			ResponseBody  json.RawMessage
		}
		read := tx.Raw(`SELECT org_id,request_sha256,response_body FROM workspace.project_folder_command WHERE actor_id=? AND idem_key=?`, actor.ID, in.IdempotencyKey).Scan(&receipt)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 1 {
			if receipt.OrgID != actor.OrgID || receipt.RequestSHA256 != fingerprint {
				return application.ErrIdempotencyConflict
			}
			if err := json.Unmarshal(receipt.ResponseBody, &out); err != nil {
				return fmt.Errorf("decode directory receipt: %w", err)
			}
			return nil
		}
		var lockedAt time.Time
		if err := tx.Raw(`SELECT clock_timestamp()`).Scan(&lockedAt).Error; err != nil {
			return err
		}
		if lockedAt.After(now) {
			now = lockedAt.UTC()
		}
		switch in.Action {
		case "create":
			out, err = s.createFolder(ctx, tx, actor, in, now)
		case "patch":
			out, err = s.patchFolder(ctx, tx, actor, in, now)
		case "move":
			out, err = s.moveProjectFolder(ctx, tx, actor, in, now)
		case "recycle":
			out, err = s.recycleFolder(ctx, tx, actor, in, now)
		}
		if err != nil {
			return err
		}
		if err := folderAudit(tx, actor, in, out, now); err != nil {
			return err
		}
		body, err := json.Marshal(out)
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_folder_command(id,org_id,actor_id,idem_key,action,request_sha256,response_body,create_time) VALUES(?,?,?,?,?,?,?::jsonb,?)`, uuid.New(), actor.OrgID, actor.ID, in.IdempotencyKey, in.Action, fingerprint, string(body), now).Error
	})
	if err != nil {
		return application.FolderChangeResult{}, fmt.Errorf("personal directory transaction: %w", err)
	}
	return out, nil
}

func (s *FolderStore) requireCover(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, cover *domain.FolderCover) error {
	if cover == nil {
		return nil
	}
	if s.media == nil {
		return application.ErrProjectDependencyUnavailable
	}
	reader := s.media(tx)
	if reader == nil {
		return application.ErrProjectDependencyUnavailable
	}
	asset, err := reader.Reference(ctx, actor, cover.ProjectID, cover.AssetID)
	if err != nil {
		return fmt.Errorf("verify directory cover: %w", err)
	}
	if asset.ID != cover.AssetID || asset.ProjectID != cover.ProjectID || asset.Kind != "image" {
		return domain.ErrProjectFolderCoverUnavailable
	}
	return nil
}

func (s *FolderStore) createFolder(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, in application.FolderChangeInput, now time.Time) (application.FolderChangeResult, error) {
	if err := s.requireCover(ctx, tx, actor, in.Cover); err != nil {
		return application.FolderChangeResult{}, err
	}
	f := domain.ProjectFolder{ID: uuid.New(), OrgID: actor.OrgID, ActorID: actor.ID, Name: strings.TrimSpace(*in.Name), Cover: in.Cover, Revision: 1, CreateTime: now, UpdateTime: now}
	var project, asset any
	if f.Cover != nil {
		project = f.Cover.ProjectID
		asset = f.Cover.AssetID
	}
	if err := tx.Exec(`INSERT INTO workspace.project_folder(id,org_id,actor_id,name,cover_project_id,cover_asset_id,create_time,update_time) VALUES(?,?,?,?,?,?,?,?)`, f.ID, f.OrgID, f.ActorID, f.Name, project, asset, now, now).Error; err != nil {
		return application.FolderChangeResult{}, err
	}
	return application.FolderChangeResult{Folder: &f}, nil
}

func (s *FolderStore) patchFolder(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, in application.FolderChangeInput, now time.Time) (application.FolderChangeResult, error) {
	f, err := readFolder(tx, actor, in.FolderID, false)
	if err != nil {
		return application.FolderChangeResult{}, err
	}
	if f.Revision != in.ExpectedRevision {
		return application.FolderChangeResult{}, domain.ErrProjectFolderRevisionConflict
	}
	if in.Name != nil {
		f.Name = strings.TrimSpace(*in.Name)
	}
	if in.SetCover {
		if err := s.requireCover(ctx, tx, actor, in.Cover); err != nil {
			return application.FolderChangeResult{}, err
		}
		f.Cover = in.Cover
	}
	f.Revision++
	f.UpdateTime = now
	var project, asset any
	if f.Cover != nil {
		project = f.Cover.ProjectID
		asset = f.Cover.AssetID
	}
	write := tx.Exec(`UPDATE workspace.project_folder SET name=?,cover_project_id=?,cover_asset_id=?,revision=?,update_time=? WHERE id=? AND org_id=? AND actor_id=? AND revision=? AND NOT is_delete`, f.Name, project, asset, f.Revision, now, f.ID, actor.OrgID, actor.ID, in.ExpectedRevision)
	if write.Error != nil {
		return application.FolderChangeResult{}, write.Error
	}
	if write.RowsAffected != 1 {
		return application.FolderChangeResult{}, domain.ErrProjectFolderRevisionConflict
	}
	return application.FolderChangeResult{Folder: &f}, nil
}

var _ application.ProjectFoldersStore = (*FolderStore)(nil)
