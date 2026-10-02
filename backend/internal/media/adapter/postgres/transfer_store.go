package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// LibraryWorkGuardFactory binds installed work owners to the caller transaction.
type LibraryWorkGuardFactory func(*gorm.DB) application.LibraryWorkGuards

// TransferStore holds permanent admission, per-item results and private intents.
type TransferStore struct {
	db      *gorm.DB
	project LibraryProjectAccessFactory
	guards  LibraryWorkGuardFactory
	clock   func() time.Time
}

// NewTransferStore explicitly injects both workspace owning ports and its clock.
func NewTransferStore(db *gorm.DB, project LibraryProjectAccessFactory, guards LibraryWorkGuardFactory, clock func() time.Time) *TransferStore {
	if clock == nil {
		clock = time.Now
	}
	return &TransferStore{db: db, project: project, guards: guards, clock: clock}
}

type transferJobRow struct {
	ID, OrgID, ActorID, SourceLibraryID, TargetLibraryID                           uuid.UUID
	SourceKind, TargetKind                                                         string
	SourceProjectID, TargetProjectID, TargetFolderID                               *uuid.UUID
	TargetFolderRevision, SourceRevision, TargetRevision, ProjectRevision          int64
	ManifestSHA256                                                                 string
	ItemCount, Attempt                                                             int
	Status, Stage                                                                  string
	Revision                                                                       int64
	NeedsReconciliation, CancellationRequested, ExecutionUnconfirmed, ProcessEnded bool
	ExecutionID, WorkerFence                                                       *uuid.UUID
	LeaseUntil                                                                     *time.Time
	CreatedAt, UpdatedAt                                                           time.Time
}

func (r transferJobRow) scopes() (domain.LibraryScope, domain.LibraryScope) {
	return domain.LibraryScope{Kind: domain.LibraryKind(r.SourceKind), ProjectID: r.SourceProjectID}, domain.LibraryScope{Kind: domain.LibraryKind(r.TargetKind), ProjectID: r.TargetProjectID}
}
func transferProject(source, target domain.LibraryScope) uuid.UUID {
	if source.ProjectID != nil {
		return *source.ProjectID
	}
	if target.ProjectID != nil {
		return *target.ProjectID
	}
	return uuid.Nil
}
func (s *TransferStore) access(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, source, target domain.LibraryScope, write bool) (application.LibraryProjectFacts, error) {
	if source.Validate() != nil || target.Validate() != nil || source.Kind == target.Kind || s.project == nil {
		return application.LibraryProjectFacts{}, application.ErrUnavailable
	}
	owner := s.project(tx)
	if owner == nil {
		return application.LibraryProjectFacts{}, application.ErrUnavailable
	}
	id := transferProject(source, target)
	facts, err := owner.Authorize(ctx, actor, id, write && target.Kind == domain.LibraryProject)
	if err != nil {
		return facts, err
	}
	if facts.ProjectID != id || facts.OrgID != actor.OrgID || facts.Revision < 1 {
		return facts, application.ErrUnavailable
	}
	return facts, nil
}
func transferHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil || len(data) > 1<<20 {
		return "", domain.ErrInvalidLibrary
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
func transferLock(tx *gorm.DB, actor, key uuid.UUID) error {
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "media-transfer/"+actor.String()+"/"+key.String()).Error
}
func transferReplay(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, hash, action string) (*domain.TransferJob, error) {
	var row struct {
		OrgID, JobID          uuid.UUID
		RequestSHA256, Action string
		ResponseBody          []byte
	}
	read := tx.Raw(`SELECT * FROM media.transfer_command WHERE actor_id=? AND idem_key=?`, actor.ID, key).Scan(&row)
	if read.Error != nil {
		return nil, read.Error
	}
	if read.RowsAffected == 0 {
		return nil, nil
	}
	if row.OrgID != actor.OrgID || row.RequestSHA256 != hash || row.Action != action {
		return nil, application.ErrLibraryKeyConflict
	}
	var job domain.TransferJob
	if len(row.ResponseBody) > 1<<20 || strictCopyMedia(row.ResponseBody, &job) != nil || job.ID != row.JobID || job.CurrentActorID != actor.ID || job.CurrentOrgID != actor.OrgID || job.Revision < 1 {
		return nil, application.ErrUnavailable
	}
	return &job, nil
}
func (s *TransferStore) readJob(tx *gorm.DB, actor identityapp.Principal, id uuid.UUID, lock bool) (transferJobRow, error) {
	var row transferJobRow
	query := `SELECT * FROM media.transfer_job WHERE id=? AND org_id=? AND actor_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	read := tx.Raw(query, id, actor.OrgID, actor.ID).Scan(&row)
	if read.Error != nil {
		return row, read.Error
	}
	if read.RowsAffected != 1 {
		return row, application.ErrNotFound
	}
	source, target := row.scopes()
	if source.Validate() != nil || target.Validate() != nil || source.Kind == target.Kind || row.Revision < 1 || row.Attempt < 1 || row.ItemCount < 1 || row.ItemCount > 200 {
		return row, application.ErrUnavailable
	}
	return row, nil
}
func transferView(tx *gorm.DB, row transferJobRow) (domain.TransferJob, error) {
	source, target := row.scopes()
	job := domain.TransferJob{ID: row.ID, CurrentActorID: row.ActorID, CurrentOrgID: row.OrgID, Source: source, Target: target, TargetFolderID: row.TargetFolderID, Status: row.Status, Stage: row.Stage, Attempt: row.Attempt, Revision: row.Revision, NeedsReconciliation: row.NeedsReconciliation, CancellationRequested: row.CancellationRequested, ExecutionUnconfirmed: row.ExecutionUnconfirmed, Items: []domain.TransferItemResult{}, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC()}
	var rows []struct {
		ItemIndex                  int
		SourceItemID, TargetItemID uuid.UUID
		TargetAssetID              *uuid.UUID
		Status                     string
		FailureCode                *string
	}
	if err := tx.Raw(`SELECT item_index,source_item_id,target_item_id,target_asset_id,status,failure_code FROM media.transfer_item WHERE job_id=? ORDER BY item_index`, row.ID).Scan(&rows).Error; err != nil {
		return job, err
	}
	if len(rows) != row.ItemCount {
		return job, application.ErrUnavailable
	}
	for i, r := range rows {
		if r.ItemIndex != i {
			return job, application.ErrUnavailable
		}
		job.Items = append(job.Items, domain.TransferItemResult{Index: r.ItemIndex, SourceItemID: r.SourceItemID, TargetItemID: r.TargetItemID, TargetAssetID: r.TargetAssetID, Status: r.Status, FailureCode: r.FailureCode})
	}
	return job, nil
}

// GetTransfer returns only the authenticated creator's current safe view.
func (s *TransferStore) GetTransfer(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (domain.TransferJob, error) {
	var result domain.TransferJob
	if s == nil || s.db == nil || id == uuid.Nil {
		return result, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		row, err := s.readJob(tx, actor, id, false)
		if err != nil {
			return err
		}
		source, target := row.scopes()
		if _, err := s.access(ctx, tx, actor, source, target, false); err != nil {
			return err
		}
		result, err = transferView(tx, row)
		return err
	})
	return result, err
}

// ListTransfers filters creator/organization and the exact authorized scope.
func (s *TransferStore) ListTransfers(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, page, limit int) ([]domain.TransferJob, error) {
	if s == nil || s.db == nil {
		return nil, application.ErrUnavailable
	}
	if scope.Validate() != nil || page < 1 || page > 10000 || limit < 1 || limit > 100 {
		return nil, application.ErrInvalidQuery
	}
	jobs := []domain.TransferJob{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if scope.Kind == domain.LibraryProject {
			if s.project == nil || s.project(tx) == nil {
				return application.ErrUnavailable
			}
			if _, err := s.project(tx).Authorize(ctx, actor, *scope.ProjectID, false); err != nil {
				return err
			}
		}
		query := `SELECT * FROM media.transfer_job WHERE org_id=? AND actor_id=?`
		args := []any{actor.OrgID, actor.ID}
		if scope.Kind == domain.LibraryProject {
			query += ` AND (source_project_id=? OR target_project_id=?)`
			args = append(args, *scope.ProjectID, *scope.ProjectID)
		}
		query += ` ORDER BY created_at DESC,id LIMIT ? OFFSET ?`
		args = append(args, limit, (page-1)*limit)
		var rows []transferJobRow
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			source, target := row.scopes()
			if _, err := s.access(ctx, tx, actor, source, target, false); err != nil {
				if errors.Is(err, application.ErrNotFound) {
					continue
				}
				return err
			}
			job, err := transferView(tx, row)
			if err != nil {
				return err
			}
			jobs = append(jobs, job)
		}
		return nil
	})
	return jobs, err
}

func ensureTransferLibraries(tx *gorm.DB, actor identityapp.Principal, source, target domain.LibraryScope) (libraryRow, libraryRow, error) {
	first, _ := source.Identity(actor.OrgID, actor.ID)
	second, _ := target.Identity(actor.OrgID, actor.ID)
	scopes := []domain.LibraryScope{source, target}
	if bytes.Compare(first[:], second[:]) > 0 {
		scopes[0], scopes[1] = scopes[1], scopes[0]
	}
	byID := make(map[uuid.UUID]libraryRow, 2)
	for _, scope := range scopes {
		id, _ := scope.Identity(actor.OrgID, actor.ID)
		var personal *uuid.UUID
		if scope.Kind == domain.LibraryPersonal {
			personal = &actor.ID
		}
		if err := tx.Exec(`INSERT INTO media.library(id,kind,org_id,project_id,personal_actor_id,revision) VALUES(?,?,?,?,?,0) ON CONFLICT(id) DO NOTHING`, id, string(scope.Kind), actor.OrgID, scope.ProjectID, personal).Error; err != nil {
			return libraryRow{}, libraryRow{}, err
		}
		row, err := readLibrary(tx, actor, scope, true)
		if err != nil {
			return row, row, err
		}
		byID[id] = row
	}
	return byID[first], byID[second], nil
}

func transferFrozenBytes(items []application.FrozenTransferItem) (string, [][]byte, error) {
	encoded := make([][]byte, 0, len(items))
	all := make([]byte, 0)
	for _, item := range items {
		body, err := json.Marshal(item)
		if err != nil || len(body) > 1<<20 {
			return "", nil, application.ErrUnavailable
		}
		encoded = append(encoded, body)
		all = append(all, body...)
		all = append(all, '\n')
		if len(all) > 16<<20 {
			return "", nil, application.ErrUnavailable
		}
	}
	hash := sha256.Sum256(all)
	return hex.EncodeToString(hash[:]), encoded, nil
}

func transferSourceFile(tx *gorm.DB, actor identityapp.Principal, scope domain.LibraryScope, id uuid.UUID) (application.LibraryMediaFile, error) {
	query := `SELECT * FROM media.media_asset WHERE id=? AND project_id=? AND personal_actor_id IS NULL AND personal_org_id IS NULL FOR SHARE`
	args := []any{id, scope.ProjectID}
	if scope.Kind == domain.LibraryPersonal {
		query = `SELECT * FROM media.media_asset WHERE id=? AND project_id IS NULL AND personal_org_id=? AND personal_actor_id=? FOR SHARE`
		args = []any{id, actor.OrgID, actor.ID}
	}
	var row assetRow
	read := tx.Raw(query, args...).Scan(&row)
	if read.Error != nil {
		return application.LibraryMediaFile{}, read.Error
	}
	if read.RowsAffected != 1 {
		return application.LibraryMediaFile{}, application.ErrNotFound
	}
	var rends []renditionRow
	if err := tx.Raw(`SELECT * FROM media.rendition WHERE media_asset_id=? AND NOT is_delete ORDER BY kind,id LIMIT 17 FOR SHARE`, id).Scan(&rends).Error; err != nil {
		return application.LibraryMediaFile{}, err
	}
	if len(rends) > 16 {
		return application.LibraryMediaFile{}, application.ErrUnavailable
	}
	result := application.LibraryMediaFile{Asset: row.domain(), Renditions: []domain.Rendition{}}
	for _, r := range rends {
		result.Renditions = append(result.Renditions, r.domain())
	}
	return result, nil
}
