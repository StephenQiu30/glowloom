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
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// LibraryWorkGuardFactory binds installed work owners to the caller transaction.
type LibraryWorkGuardFactory func(*gorm.DB) application.LibraryWorkGuards

// TransferStore holds permanent admission, per-item results and private intents.
type TransferStore struct {
	routed bool
	scopeAuthority
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
	SourceScopeID, TargetScopeID, SourceOrgID, TargetOrgID                         *uuid.UUID
	ID, OrgID, ActorID, SourceLibraryID, TargetLibraryID                           uuid.UUID
	SourceKind, TargetKind                                                         string
	SourceProjectID, TargetProjectID, TargetFolderID                               *uuid.UUID
	TargetFolderRevision, SourceRevision, TargetRevision, ProjectRevision          int64
	ManifestSHA256                                                                 string
	ItemCount, Attempt                                                             int
	Status                                                                         domain.TransferStatus
	Stage                                                                          domain.TransferStage
	Revision                                                                       int64
	NeedsReconciliation, CancellationRequested, ExecutionUnconfirmed, ProcessEnded bool
	ExecutionID, WorkerFence                                                       *uuid.UUID
	LeaseUntil                                                                     *time.Time
	CreatedAt, UpdatedAt                                                           time.Time
}

func (transferJobRow) TableName() string { return "media.transfer_job" }

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
	sourceActor, targetActor, err := s.transferActors(tx, actor, source, target, write)
	if err != nil {
		return application.LibraryProjectFacts{}, err
	}
	projectActor := sourceActor
	if target.Kind == domain.LibraryProject {
		projectActor = targetActor
	}
	owner := s.project(tx)
	if owner == nil {
		return application.LibraryProjectFacts{}, application.ErrUnavailable
	}
	id := transferProject(source, target)
	facts, err := owner.Authorize(ctx, projectActor, id, write && target.Kind == domain.LibraryProject)
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
	return lockStorageIdempotencyKey(tx, storageIdempotencyTransfer, actor, key)
}
func (s *TransferStore) transferReplay(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, hash string, action application.TransferAction) (*domain.TransferJob, error) {
	var row transferCommandRecord
	read := tx.Where("actor_id = ? AND idem_key = ?", actor.ID, key).Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if read.Error != nil {
		return nil, read.Error
	}
	if row.OrgID != actor.OrgID || row.RequestSHA256 != hash || row.Action != string(action) {
		return nil, application.ErrLibraryKeyConflict
	}
	var job domain.TransferJob
	if len(row.ResponseBody) > 1<<20 || strictCopyMedia(row.ResponseBody, &job) != nil || job.ID != row.JobID || job.CurrentActorID != actor.ID || job.CurrentOrgID != actor.OrgID || job.Revision < 1 {
		return nil, application.ErrUnavailable
	}
	binding, err := s.readJob(tx, actor, job.ID, false, false)
	if err != nil {
		return nil, err
	}
	source, target := binding.scopes()
	if job.Source.Kind != source.Kind || job.Target.Kind != target.Kind || !sameScopeProject(job.Source.ProjectID, source.ProjectID) || !sameScopeProject(job.Target.ProjectID, target.ProjectID) || job.SourceScopeID != uuid.Nil && job.SourceScopeID != *binding.SourceScopeID || job.TargetScopeID != uuid.Nil && job.TargetScopeID != *binding.TargetScopeID {
		return nil, application.ErrUnavailable
	}
	return &job, nil
}

// The first ownership check must take the scope lock strength needed by the
// entire transaction. Concurrent read-to-write upgrades can deadlock.
func (s *TransferStore) readJob(tx *gorm.DB, actor identityapp.Principal, id uuid.UUID, lock, writeScope bool) (transferJobRow, error) {
	var row transferJobRow
	query := tx.Where("id = ? AND org_id = ? AND actor_id = ?", id, actor.OrgID, actor.ID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	read := query.Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return row, application.ErrNotFound
	}
	if read.Error != nil {
		return row, read.Error
	}
	source, target := row.scopes()
	if source.Validate() != nil || target.Validate() != nil || source.Kind == target.Kind || row.Revision < 1 || row.Attempt < 1 || row.ItemCount < 1 || row.ItemCount > 200 {
		return row, application.ErrUnavailable
	}
	if row.SourceScopeID == nil || row.TargetScopeID == nil || row.SourceOrgID == nil || row.TargetOrgID == nil {
		return row, application.ErrNotFound
	}
	sa, ta, err := s.transferActors(tx, actor, source, target, writeScope)
	if err != nil {
		return row, err
	}
	if sa.ScopeID != *row.SourceScopeID || ta.ScopeID != *row.TargetScopeID || sa.OrgID != *row.SourceOrgID || ta.OrgID != *row.TargetOrgID {
		return row, application.ErrNotFound
	}
	return row, nil
}
func transferView(tx *gorm.DB, row transferJobRow) (domain.TransferJob, error) {
	if row.SourceScopeID == nil || row.TargetScopeID == nil || row.SourceOrgID == nil || row.TargetOrgID == nil {
		return domain.TransferJob{}, application.ErrNotFound
	}
	source, target := row.scopes()
	job := domain.TransferJob{ID: row.ID, CurrentActorID: row.ActorID, CurrentOrgID: row.OrgID, Source: source, Target: target, SourceScopeID: *row.SourceScopeID, TargetScopeID: *row.TargetScopeID, TargetFolderID: row.TargetFolderID, Status: row.Status, Stage: row.Stage, Attempt: row.Attempt, Revision: row.Revision, NeedsReconciliation: row.NeedsReconciliation, CancellationRequested: row.CancellationRequested, ExecutionUnconfirmed: row.ExecutionUnconfirmed, Items: []domain.TransferItemResult{}, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC()}
	var rows []transferItemViewRow
	if err := tx.Where("job_id = ?", row.ID).Order("item_index").Find(&rows).Error; err != nil {
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
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		row, err := s.readJob(tx, actor, id, false, false)
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
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if scope.Kind == domain.LibraryPersonal {
			if err := s.requirePersonal(tx, actor, false); err != nil {
				return err
			}
		}
		if scope.Kind == domain.LibraryProject {
			if s.project == nil || s.project(tx) == nil {
				return application.ErrUnavailable
			}
			if _, err := s.project(tx).Authorize(ctx, actor, *scope.ProjectID, false); err != nil {
				return err
			}
		}
		query := tx.Where("org_id = ? AND actor_id = ?", actor.OrgID, actor.ID)
		if scope.Kind == domain.LibraryProject {
			query = query.Where("source_project_id = ? OR target_project_id = ?", *scope.ProjectID, *scope.ProjectID)
		}
		var rows []transferJobRow
		if err := query.Order("created_at DESC").Order("id").Limit(limit).Offset((page - 1) * limit).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if row.SourceScopeID == nil || row.TargetScopeID == nil || row.SourceOrgID == nil || row.TargetOrgID == nil {
				continue
			}
			if _, err := s.readJob(tx, actor, row.ID, false, false); err != nil {
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

func (s *TransferStore) ensureTransferLibraries(tx *gorm.DB, actor identityapp.Principal, source, target domain.LibraryScope) (libraryRow, libraryRow, error) {
	sourceActor, targetActor, err := s.transferActors(tx, actor, source, target, true)
	if err != nil {
		return libraryRow{}, libraryRow{}, err
	}
	first, _ := source.Identity(sourceActor.OrgID, actor.ID)
	second, _ := target.Identity(targetActor.OrgID, actor.ID)
	scopes := []domain.LibraryScope{source, target}
	if bytes.Compare(first[:], second[:]) > 0 {
		scopes[0], scopes[1] = scopes[1], scopes[0]
	}
	byID := make(map[uuid.UUID]libraryRow, 2)
	for _, scope := range scopes {
		sideActor := sourceActor
		if scope.Kind == target.Kind {
			sideActor = targetActor
		}
		id, _ := scope.Identity(sideActor.OrgID, actor.ID)
		var personal *uuid.UUID
		if scope.Kind == domain.LibraryPersonal {
			personal = &actor.ID
		}
		// Ensure both scope identities exist before binding durable transfer rows.
		library := libraryRow{ID: id, Kind: string(scope.Kind), OrgID: sideActor.OrgID, ProjectID: scope.ProjectID, PersonalActorID: personal}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&library).Error; err != nil {
			return libraryRow{}, libraryRow{}, err
		}
		row, err := readLibrary(tx, sideActor, scope, scope.Kind == target.Kind)
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
	query := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ? AND personal_actor_id IS NULL AND personal_org_id IS NULL", id)
	if scope.Kind == domain.LibraryPersonal {
		query = tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(map[string]any{"id": id, "project_id": nil, "personal_org_id": actor.OrgID, "personal_actor_id": actor.ID})
	} else {
		query = query.Where("project_id = ?", scope.ProjectID)
	}
	var row assetRow
	read := query.Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.LibraryMediaFile{}, application.ErrNotFound
	}
	if read.Error != nil {
		return application.LibraryMediaFile{}, read.Error
	}
	var rends []renditionRow
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("media_asset_id = ? AND is_delete = ?", id, false).Order("kind").Order("id").Limit(17).Find(&rends).Error; err != nil {
		return application.LibraryMediaFile{}, err
	}
	if len(rends) > 16 {
		return application.LibraryMediaFile{}, application.ErrUnavailable
	}
	if err := hydrateCopySourceAsset(tx, &row); err != nil {
		return application.LibraryMediaFile{}, err
	}
	result := application.LibraryMediaFile{Asset: row.domain(), Renditions: []domain.Rendition{}}
	for _, r := range rends {
		if err := hydrateCopySourceRendition(tx, &r); err != nil {
			return application.LibraryMediaFile{}, err
		}
		result.Renditions = append(result.Renditions, r.domain())
	}
	return result, nil
}

// NewScopedTransferStore binds live scope authority explicitly.
func NewScopedTransferStore(db *gorm.DB, project LibraryProjectAccessFactory, guards LibraryWorkGuardFactory, clock func() time.Time, scopes ScopeAccessFactory) *TransferStore {
	s := NewTransferStore(db, project, guards, clock)
	s.scopeAuthority = scopeAuthority{scopes: scopes}
	return s
}

func (s *TransferStore) transferActors(tx *gorm.DB, actor identityapp.Principal, source, target domain.LibraryScope, write bool) (identityapp.Principal, identityapp.Principal, error) {
	access, err := s.accessScope(tx)
	if err != nil {
		return actor, actor, err
	}
	transfer, ok := access.(application.TransferScopeAccess)
	if !ok {
		return actor, actor, application.ErrUnavailable
	}
	sa, err := transfer.TransferActor(tx.Statement.Context, actor, source, false)
	if err != nil {
		return actor, actor, err
	}
	ta, err := transfer.TransferActor(tx.Statement.Context, actor, target, write)
	if err != nil {
		return actor, actor, err
	}
	return sa, ta, nil
}

// WithRoutedStorage pins targets and requires exact saved physical source facts.
func (s *TransferStore) WithRoutedStorage() *TransferStore {
	clone := *s
	clone.routed = true
	return &clone
}
