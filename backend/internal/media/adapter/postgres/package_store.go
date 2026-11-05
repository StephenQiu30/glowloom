package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func decodePackageFrozen(data []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return application.ErrUnavailable
	}
	return nil
}

// PackageStore owns complete frozen import batches and atomic catalog publication.
type PackageStore struct {
	scopeAuthority
	db        *gorm.DB
	library   *LibraryStore
	guards    LibraryWorkGuardFactory
	clock     func() time.Time
	locations bool
}

// NewPackageStore binds workspace ports to the same media-owned transaction.
func NewPackageStore(db *gorm.DB, project LibraryProjectAccessFactory, guards LibraryWorkGuardFactory, clock func() time.Time) *PackageStore {
	if clock == nil {
		clock = time.Now
	}
	return &PackageStore{db: db, library: NewLibraryStore(db, project, clock), guards: guards, clock: clock}
}

type packageJobRow struct {
	ID, OrgID, ActorID, LibraryID, IdemKey     uuid.UUID
	LibraryKind                                string
	ProjectID                                  *uuid.UUID
	RequestSHA256, ArchiveSHA256, FrozenSHA256 string
	ArchiveBytes                               int64
	Frozen, Result                             []byte
	Status                                     application.PackageStatus
	Revision, LibraryRevision, ProjectRevision int64
	CreatedAt, UpdatedAt                       time.Time
}

func (packageJobRow) TableName() string { return "media.package_job" }

func packageRequestHash(in application.PackageImportRequest) (string, error) {
	body, err := json.Marshal(struct {
		Scope                            domain.LibraryScope
		LibraryRevision, ProjectRevision int64
		Reviewed                         bool
		SHA256                           string
		Bytes                            int64
	}{in.Scope, in.ExpectedRevision, in.ExpectedProjectRevision, in.LocalReviewConfirmed, in.ArchiveSHA256, in.ArchiveBytes})
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:]), nil
}

func packageScope(row packageJobRow) domain.LibraryScope {
	return domain.LibraryScope{Kind: domain.LibraryKind(row.LibraryKind), ProjectID: row.ProjectID}
}

func decodePackagePlan(row packageJobRow) (application.PackagePlan, error) {
	var plan application.PackagePlan
	digest := sha256.Sum256(row.Frozen)
	if hex.EncodeToString(digest[:]) != row.FrozenSHA256 || len(row.Frozen) > 32<<20 || decodePackageFrozen(row.Frozen, &plan) != nil {
		return plan, application.ErrUnavailable
	}
	plan.Request.Key, plan.Request.RequestID = plan.Key, plan.RequestID
	plan.Request.ArchiveSHA256, plan.Request.ArchiveBytes = row.ArchiveSHA256, row.ArchiveBytes
	if plan.Validate() != nil || plan.JobID != row.ID || plan.OrgID != row.OrgID || plan.ActorID != row.ActorID || plan.Key != row.IdemKey || plan.Request.Scope.Kind != domain.LibraryKind(row.LibraryKind) || !sameScopeProject(plan.Request.Scope.ProjectID, row.ProjectID) || !plan.CreatedAt.Equal(row.CreatedAt) {
		return plan, application.ErrUnavailable
	}
	hash, err := packageRequestHash(plan.Request)
	if err != nil || hash != row.RequestSHA256 {
		return plan, application.ErrUnavailable
	}
	return plan, nil
}

func packageJob(row packageJobRow, plan application.PackagePlan) (application.PackageJob, error) {
	result := application.PackageJob{ID: row.ID, Scope: packageScope(row), Status: row.Status, Revision: row.Revision, ItemCount: len(plan.Items), FolderCount: len(plan.Folders), Warnings: plan.Warnings, LibraryRevision: row.LibraryRevision, ProjectRevision: row.ProjectRevision, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.Status == application.PackageStatusSucceeded {
		var stored application.PackageJob
		if decodePackageFrozen(row.Result, &stored) != nil || !packageSameJob(stored, result) {
			return result, application.ErrUnavailable
		}
		return stored, nil
	}
	return result, nil
}

func packageSameJob(a, b application.PackageJob) bool {
	a.CreatedAt, b.CreatedAt = a.CreatedAt.UTC(), b.CreatedAt.UTC()
	a.UpdatedAt, b.UpdatedAt = a.UpdatedAt.UTC(), b.UpdatedAt.UTC()
	x, xe := json.Marshal(a)
	y, ye := json.Marshal(b)
	return xe == nil && ye == nil && bytes.Equal(x, y)
}

// AuthorizePackage keeps body admission on the established current ownership path.
func (s *PackageStore) AuthorizePackage(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, write bool) (string, error) {
	if s == nil || s.db == nil || scope.Validate() != nil {
		return "", application.ErrUnavailable
	}
	aspect := "16:9"
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if _, err := s.library.authorize(ctx, tx, actor, scope, write); err != nil {
			return err
		}
		if scope.Kind == domain.LibraryProject && write {
			var err error
			aspect, err = NewScopedStore(tx, s.scopes).AuthorizeUpload(ctx, actor, *scope.ProjectID)
			return err
		}
		return nil
	})
	return aspect, err
}

func packageCommandLock(tx *gorm.DB, actor, key uuid.UUID) error {
	return lockStorageIdempotencyKey(tx, storageIdempotencyPackage, actor, key)
}

// FindPackageImport reauthorizes before permanent actor/key replay.
func (s *PackageStore) FindPackageImport(ctx context.Context, actor identityapp.Principal, in application.PackageImportRequest) (application.PackageJob, *application.PackagePlan, bool, error) {
	var job application.PackageJob
	var plan *application.PackagePlan
	found := false
	if s == nil || s.db == nil {
		return job, plan, found, application.ErrUnavailable
	}
	hash, err := packageRequestHash(in)
	if err != nil {
		return job, plan, found, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := packageCommandLock(tx, actor.ID, in.Key); err != nil {
			return err
		}
		if _, err := s.library.authorize(ctx, tx, actor, in.Scope, false); err != nil {
			return err
		}
		var row packageJobRow
		read := tx.Where("actor_id = ? AND org_id = ? AND idem_key = ?", actor.ID, actor.OrgID, in.Key).Take(&row)
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return nil
		}
		if read.Error != nil {
			return read.Error
		}
		if row.RequestSHA256 != hash || row.LibraryKind != string(in.Scope.Kind) || !sameScopeProject(row.ProjectID, in.Scope.ProjectID) {
			return application.ErrPackageConflict
		}
		decoded, err := decodePackagePlan(row)
		if err != nil {
			return err
		}
		if err := hydratePackageLocations(ctx, tx, &decoded); err != nil {
			return err
		}
		plan = &decoded
		found = true
		job, err = packageJob(row, decoded)
		if err != nil {
			return err
		}
		prior, err := packageReplayCommand(tx, actor, in.Key, row.ID, hash, "import")
		if err != nil {
			return err
		}
		if prior != nil {
			job = *prior
		}
		return nil
	})
	return job, plan, found, err
}

func (s *PackageStore) scopeLock(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, scope domain.LibraryScope, write bool) (libraryRow, application.LibraryProjectFacts, error) {
	if err := s.requireCurrentActor(tx, actor); err != nil {
		return libraryRow{}, application.LibraryProjectFacts{}, err
	}
	facts, err := s.library.authorize(ctx, tx, actor, scope, write)
	if err != nil {
		return libraryRow{}, facts, err
	}
	id, err := scope.Identity(actor.OrgID, actor.ID)
	if err != nil {
		return libraryRow{}, facts, err
	}
	if write {
		var personal *uuid.UUID
		if scope.Kind == domain.LibraryPersonal {
			personal = &actor.ID
		}
		row := libraryRow{ID: id, Kind: string(scope.Kind), OrgID: actor.OrgID, ProjectID: scope.ProjectID, PersonalActorID: personal}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&row).Error; err != nil {
			return libraryRow{}, facts, err
		}
	}
	row, err := readLibrary(tx, actor, scope, write)
	return row, facts, err
}

// AdmitPackageImport stores every immutable intent before private object writes.
func (s *PackageStore) AdmitPackageImport(ctx context.Context, actor identityapp.Principal, in application.PackageImportRequest, plan application.PackagePlan) (application.PackageJob, error) {
	var result application.PackageJob
	if s == nil || s.db == nil || in.Validate() != nil {
		return result, application.ErrUnavailable
	}
	body, err := json.Marshal(plan)
	if err != nil || len(body) > 32<<20 {
		return result, application.ErrInvalidPackage
	}
	digest := sha256.Sum256(body)
	hash, err := packageRequestHash(in)
	if err != nil {
		return result, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := packageCommandLock(tx, actor.ID, in.Key); err != nil {
			return err
		}
		// A public control key may already belong to a different batch/action.
		// Reject that identity before creating an admission or touching objects.
		if _, err := packageReplayCommand(tx, actor, in.Key, plan.JobID, hash, "import"); err != nil {
			return err
		}
		library, facts, err := s.scopeLock(ctx, tx, actor, in.Scope, true)
		if err != nil {
			return err
		}
		var existing packageJobRow
		read := tx.Where("actor_id = ? AND idem_key = ?", actor.ID, in.Key).Take(&existing)
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			read.Error = nil
		}
		if read.Error != nil {
			return read.Error
		}
		if existing.ID != uuid.Nil {
			if existing.OrgID != actor.OrgID || existing.RequestSHA256 != hash {
				return application.ErrPackageConflict
			}
			frozen, err := decodePackagePlan(existing)
			if err != nil {
				return err
			}
			result, err = packageJob(existing, frozen)
			return err
		}
		if library.Revision != in.ExpectedRevision || facts.Revision != in.ExpectedProjectRevision {
			return application.ErrPackageConflict
		}
		if in.Scope.Kind == domain.LibraryProject {
			if s.guards == nil || s.guards(tx) == nil {
				return application.ErrUnavailable
			}
			busy, err := s.guards(tx).HasInflightProjectWork(ctx, actor, *in.Scope.ProjectID)
			if err != nil {
				return err
			}
			if busy {
				return application.ErrPackageConflict
			}
		}
		packageBusy, err := storageExists(tx.Model(&packageJobRow{}).Where("library_id = ? AND status = ?", library.ID, application.PackageStatusNeedsReconciliation))
		if err != nil {
			return err
		}
		transferBusy, err := storageExists(tx.Model(&transferJobRow{}).Where("(source_library_id = ? OR target_library_id = ?) AND (status IN ? OR execution_unconfirmed = ? OR process_ended = ?)", library.ID, library.ID, []domain.TransferStatus{domain.TransferStatusQueued, domain.TransferStatusRunning, domain.TransferStatusNeedsReconciliation, domain.TransferStatusCancelRequested}, true, false))
		if err != nil {
			return err
		}
		purgeBusy, err := storageExists(tx.Model(&purgeJobRow{}).Where("library_id = ? AND (status IN ? OR execution_unconfirmed = ? OR process_ended = ?)", library.ID, []domain.PurgeStatus{domain.PurgeStatusQueued, domain.PurgeStatusRunning, domain.PurgeStatusNeedsReconciliation, domain.PurgeStatusCancelRequested}, true, false))
		if err != nil {
			return err
		}
		active := packageBusy || transferBusy || purgeBusy
		if active {
			return application.ErrPackageConflict
		}
		folders, err := libraryFolders(tx, library.ID, true)
		if err != nil {
			return err
		}
		combined := append(slices.Clone(folders), plan.Folders...)
		if len(combined) > 4096 || len(combined) > 0 && domain.ValidateLibraryFolderPlacement(combined[0], combined) != nil {
			return application.ErrPackageConflict
		}
		row := packageJobRow{ID: plan.JobID, OrgID: actor.OrgID, ActorID: actor.ID, LibraryID: library.ID, LibraryKind: string(in.Scope.Kind), ProjectID: in.Scope.ProjectID, IdemKey: in.Key, RequestSHA256: hash, ArchiveSHA256: in.ArchiveSHA256, ArchiveBytes: in.ArchiveBytes, Frozen: body, FrozenSHA256: hex.EncodeToString(digest[:]), Status: application.PackageStatusNeedsReconciliation, Revision: 1, LibraryRevision: library.Revision, ProjectRevision: facts.Revision, CreatedAt: plan.CreatedAt, UpdatedAt: plan.CreatedAt}
		if _, err := decodePackagePlan(row); err != nil {
			return err
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		locations := map[string]domain.ObjectLocation{}
		if s.locations {
			var err error
			locations, err = admitPackageLocations(ctx, tx, plan)
			if err != nil {
				return err
			}
		}
		for index, object := range plan.Objects {
			objectRecord := packageObjectRecord{JobID: row.ID, ObjectIndex: index, ObjectKey: object.Key, ObjectLocationID: locationIDForKey(locations, object.Key), ByteSize: object.ByteSize, ContentType: object.MIMEType, SHA256: object.SHA256, Status: application.PackageObjectStatusPending}
			if err := tx.Create(&objectRecord).Error; err != nil {
				return err
			}
		}
		result, err = packageJob(row, plan)
		return err
	})
	return result, err
}

func (s *PackageStore) readPackage(tx *gorm.DB, actor identityapp.Principal, id uuid.UUID, lock bool) (packageJobRow, application.PackagePlan, error) {
	var row packageJobRow
	query := tx.Where("id = ? AND org_id = ? AND actor_id = ?", id, actor.OrgID, actor.ID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	read := query.Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return row, application.PackagePlan{}, application.ErrNotFound
	}
	if read.Error != nil {
		return row, application.PackagePlan{}, read.Error
	}
	plan, err := decodePackagePlan(row)
	if err == nil {
		err = hydratePackageLocations(tx.Statement.Context, tx, &plan)
	}
	return row, plan, err
}

// GetPackage cannot expose another actor's source manifest or old receipt.
func (s *PackageStore) GetPackage(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (application.PackageJob, error) {
	var result application.PackageJob
	if s == nil || s.db == nil || id == uuid.Nil {
		return result, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		row, plan, err := s.readPackage(tx, actor, id, false)
		if err != nil {
			return err
		}
		if _, err := s.library.authorize(ctx, tx, actor, packageScope(row), false); err != nil {
			return err
		}
		result, err = packageJob(row, plan)
		return err
	})
	return result, err
}

// ListPackages applies current actor/org/scope filtering before stable paging.
func (s *PackageStore) ListPackages(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, page, size int) (application.PackagePage, error) {
	result := application.PackagePage{CurrentActorID: actor.ID, CurrentOrgID: actor.OrgID, Scope: scope, Page: page, PageSize: size, Jobs: []application.PackageJob{}}
	if s == nil || s.db == nil {
		return result, application.ErrUnavailable
	}
	if page < 1 || page > 100000 || size < 1 || size > 120 {
		return result, application.ErrInvalidPackage
	}
	err := s.library.read(ctx, actor, scope, func(tx *gorm.DB, library libraryRow) error {
		if err := tx.Model(&packageJobRow{}).Where("org_id = ? AND actor_id = ? AND library_id = ?", actor.OrgID, actor.ID, library.ID).Count(&result.Total).Error; err != nil {
			return err
		}
		var rows []packageJobRow
		if err := tx.Where("org_id = ? AND actor_id = ? AND library_id = ?", actor.OrgID, actor.ID, library.ID).Order("created_at DESC").Order("id DESC").Limit(size).Offset((page - 1) * size).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			plan, err := decodePackagePlan(row)
			if err != nil {
				return err
			}
			job, err := packageJob(row, plan)
			if err != nil {
				return err
			}
			result.Jobs = append(result.Jobs, job)
		}
		return nil
	})
	return result, err
}

// WithPackageImport locks current project, library and batch throughout object
// verification. Connection loss releases the locks but retains admission/intents.
func (s *PackageStore) WithPackageImport(ctx context.Context, actor identityapp.Principal, id uuid.UUID, verify func(application.PackagePlan) error) (application.PackageJob, error) {
	return s.runPackage(ctx, actor, id, false, nil, verify)
}

// ControlPackageImport owns permanent current-head recovery and cancellation.
func (s *PackageStore) ControlPackageImport(ctx context.Context, actor identityapp.Principal, id uuid.UUID, command application.PackageControl, cancel bool, work func(application.PackagePlan) error) (application.PackageJob, error) {
	if command.Validate() != nil {
		return application.PackageJob{}, application.ErrInvalidPackage
	}
	return s.runPackage(ctx, actor, id, cancel, &command, work)
}

func packageReplayCommand(tx *gorm.DB, actor identityapp.Principal, key, id uuid.UUID, hash, action string) (*application.PackageJob, error) {
	var row packageCommandRecord
	read := tx.Where("actor_id = ? AND idem_key = ?", actor.ID, key).Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if read.Error != nil {
		return nil, read.Error
	}
	if row.OrgID != actor.OrgID || row.JobID != id || row.Action != action || row.RequestSHA256 != hash {
		return nil, application.ErrPackageConflict
	}
	var job application.PackageJob
	if decodePackageFrozen(row.Response, &job) != nil || job.ID != id {
		return nil, application.ErrUnavailable
	}
	return &job, nil
}

func savePackageCommand(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, hash, action string, job application.PackageJob) (application.PackageJob, error) {
	prior, err := packageReplayCommand(tx, actor, key, job.ID, hash, action)
	if err != nil {
		return job, err
	}
	if prior != nil {
		return *prior, nil
	}
	body, err := json.Marshal(job)
	if err != nil || len(body) > 1<<20 {
		return job, application.ErrUnavailable
	}
	command := packageCommandRecord{ActorID: actor.ID, OrgID: actor.OrgID, IdemKey: key, JobID: job.ID, Action: action, RequestSHA256: hash, Response: body}
	err = tx.Create(&command).Error
	return job, err
}

func (s *PackageStore) runPackage(ctx context.Context, actor identityapp.Principal, id uuid.UUID, cancel bool, command *application.PackageControl, work func(application.PackagePlan) error) (application.PackageJob, error) {
	var result application.PackageJob
	if s == nil || s.db == nil || id == uuid.Nil || work == nil {
		return result, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if command != nil {
			if err := packageCommandLock(tx, actor.ID, command.Key); err != nil {
				return err
			}
		}
		initial, _, err := s.readPackage(tx, actor, id, false)
		if err != nil {
			return err
		}
		library, facts, err := s.scopeLock(ctx, tx, actor, packageScope(initial), initial.Status == application.PackageStatusNeedsReconciliation)
		if err != nil {
			return err
		}
		row, plan, err := s.readPackage(tx, actor, id, true)
		if err != nil {
			return err
		}
		result, err = packageJob(row, plan)
		if err != nil {
			return err
		}
		key, hash, action := row.IdemKey, row.RequestSHA256, "import"
		if command != nil {
			key = command.Key
			action = "reconcile"
			if cancel {
				action = "cancel"
			}
			body, err := json.Marshal(struct {
				ID       uuid.UUID
				Action   string
				Revision int64
			}{id, action, command.ExpectedRevision})
			if err != nil {
				return err
			}
			digest := sha256.Sum256(body)
			hash = hex.EncodeToString(digest[:])
			prior, err := packageReplayCommand(tx, actor, key, id, hash, action)
			if err != nil {
				return err
			}
			if prior != nil {
				result = *prior
				return nil
			}
			if row.Revision != command.ExpectedRevision {
				return application.ErrPackageConflict
			}
		}
		if row.Status != application.PackageStatusNeedsReconciliation {
			if cancel && row.Status == application.PackageStatusSucceeded {
				return application.ErrPackageConflict
			}
			result, err = savePackageCommand(tx, actor, key, hash, action, result)
			return err
		}
		if !cancel && (library.Revision != plan.Request.ExpectedRevision || facts.Revision != plan.Request.ExpectedProjectRevision) {
			return application.ErrPackageConflict
		}
		var objects []packageObjectRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ?", id).Order("object_index").Find(&objects).Error; err != nil {
			return err
		}
		if len(objects) != len(plan.Objects) {
			return application.ErrUnavailable
		}
		for index, object := range objects {
			frozen := plan.Objects[index]
			if object.ObjectIndex != index || object.ObjectKey != frozen.Key || object.ContentType != frozen.MIMEType || object.SHA256 != frozen.SHA256 || object.ByteSize != frozen.ByteSize || object.Status == application.PackageObjectStatusRemoved && !cancel {
				return application.ErrUnavailable
			}
		}
		// Verification failures leave every predeclared key protected. We never
		// delete or publish a subset as compensation for an uncertain network write.
		if err := work(plan); err != nil {
			result, err = savePackageCommand(tx, actor, key, hash, action, result)
			return err
		}
		now := s.clock().UTC().Truncate(time.Microsecond)
		row.Revision++
		row.UpdatedAt = now
		objectStatus := application.PackageObjectStatusRemoved
		row.Status = application.PackageStatusCancelled
		if !cancel {
			if err := acceptPackageLocations(ctx, tx, &plan); err != nil {
				return err
			}
			if err := publishPackage(tx, library, plan); err != nil {
				return err
			}
			if err := updateOne(tx, "media.library", map[string]any{"id": library.ID, "revision": library.Revision}, map[string]any{"revision": library.Revision + 1, "update_time": now}, application.ErrLibraryConflict); err != nil {
				return err
			}
			row.LibraryRevision = library.Revision + 1
			if row.ProjectID != nil {
				next, err := s.library.project(tx).TouchContent(ctx, actor, *row.ProjectID, facts.Revision)
				if err != nil {
					return err
				}
				row.ProjectRevision = next
			}
			row.Status = application.PackageStatusSucceeded
			objectStatus = application.PackageObjectStatusVerified
		}
		if err := tx.Model(&packageObjectRecord{}).Where("job_id = ?", id).Update("status", objectStatus).Error; err != nil {
			return err
		}
		result, err = packageJob(row, plan)
		if row.Status == application.PackageStatusSucceeded {
			row.Result = nil
			result = application.PackageJob{ID: row.ID, Scope: packageScope(row), Status: row.Status, Revision: row.Revision, ItemCount: len(plan.Items), FolderCount: len(plan.Folders), Warnings: plan.Warnings, LibraryRevision: row.LibraryRevision, ProjectRevision: row.ProjectRevision, CreatedAt: row.CreatedAt, UpdatedAt: now}
			err = nil
		}
		if err != nil {
			return err
		}
		var response []byte
		if !cancel {
			response, err = json.Marshal(result)
			if err != nil {
				return err
			}
		}
		if err := updateOne(tx, "media.package_job", map[string]any{"id": id, "status": application.PackageStatusNeedsReconciliation, "revision": row.Revision - 1}, map[string]any{"status": row.Status, "revision": row.Revision, "library_revision": row.LibraryRevision, "project_revision": row.ProjectRevision, "result": response, "updated_at": now}, application.ErrLibraryConflict); err != nil {
			return err
		}
		safe, _ := json.Marshal(map[string]any{"revision": row.Revision, "status": row.Status, "item_count": len(plan.Items), "folder_count": len(plan.Folders)})
		auditAction := "media.package.imported"
		if cancel {
			auditAction = "media.package.cancelled"
		}
		audit := uploadAuditRecord{ID: uuid.NewSHA1(id, []byte(auditAction)), OrgID: actor.OrgID, ProjectID: row.ProjectID, ActorID: actor.ID, ActorKind: "user", Action: auditAction, ObjectType: "media.package", ObjectID: id.String(), After: jsonDocument(safe), RequestID: plan.RequestID.String()}
		if err := tx.Omit("CreateTime", "UpdateTime").Create(&audit).Error; err != nil {
			return err
		}
		result, err = savePackageCommand(tx, actor, key, hash, action, result)
		return err
	})
	return result, err
}

func publishPackage(tx *gorm.DB, library libraryRow, plan application.PackagePlan) error {
	remaining := append([]domain.LibraryFolder{}, plan.Folders...)
	inserted := make(map[uuid.UUID]bool, len(remaining))
	for len(remaining) > 0 {
		next := []domain.LibraryFolder{}
		advanced := false
		for _, folder := range remaining {
			if folder.ParentID != nil && !inserted[*folder.ParentID] {
				next = append(next, folder)
				continue
			}
			if folder.LibraryID != library.ID || folder.Kind != domain.LibraryKind(library.Kind) || folder.Validate() != nil {
				return application.ErrUnavailable
			}
			row := libraryFolderRow{ID: folder.ID, LibraryID: library.ID, LibraryKind: string(folder.Kind), ParentID: folder.ParentID, Name: folder.Name, NameKey: folder.NameKey(), Position: folder.Position, Style: folder.Style, Theme: folder.Theme, Revision: 1, CreateTime: folder.CreatedAt, UpdateTime: folder.UpdatedAt}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			inserted[folder.ID] = true
			advanced = true
		}
		if !advanced {
			return application.ErrUnavailable
		}
		remaining = next
	}
	for _, asset := range plan.Assets {
		rends := []domain.Rendition{}
		for _, rend := range plan.Renditions {
			if rend.MediaAssetID == asset.ID {
				rends = append(rends, rend)
			}
		}
		if err := insertTransferAsset(tx, asset, rends); err != nil {
			return err
		}
	}
	for _, item := range plan.Items {
		if item.LibraryID != library.ID || item.Validate() != nil {
			return application.ErrUnavailable
		}
		if err := persistLibraryItem(tx, item, 0); err != nil {
			return err
		}
	}
	return nil
}

var _ application.PackageRepository = (*PackageStore)(nil)

// HasInflightPackageWork is the owning lifecycle port for every unknown batch.
func (s *PackageStore) HasInflightPackageWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	var busy bool
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if _, err := s.library.authorize(ctx, tx, actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, false); err != nil {
			return err
		}
		value, err := storageExists(tx.Model(&packageJobRow{}).Where("org_id = ? AND project_id = ? AND status = ?", actor.OrgID, project, application.PackageStatusNeedsReconciliation))
		busy = value
		return err
	})
	return busy, err
}

// HasInflightLibraryPackageWork is the owning guard for either authorized library.
// It never calls the project guard factory and can use the caller's transaction.
func (s *PackageStore) HasInflightLibraryPackageWork(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope) (bool, error) {
	if s == nil || s.db == nil || s.library == nil {
		return false, application.ErrUnavailable
	}
	var busy bool
	err := s.library.read(ctx, actor, scope, func(tx *gorm.DB, library libraryRow) error {
		var err error
		busy, err = storageExists(tx.Model(&packageJobRow{}).Where("org_id = ? AND library_id = ? AND status = ?", actor.OrgID, library.ID, application.PackageStatusNeedsReconciliation))
		if err != nil {
			return fmt.Errorf("%w: read retained library packages: %w", application.ErrUnavailable, err)
		}
		return nil
	})
	return busy, err
}

// HasPackageReferences protects every unpublished reserved asset identity.
func (s *PackageStore) HasPackageReferences(ctx context.Context, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	var found bool
	err := s.library.read(ctx, actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, func(tx *gorm.DB, library libraryRow) error {
		var rows []packageJobRow
		if err := tx.Where("org_id = ? AND library_id = ? AND status = ?", actor.OrgID, library.ID, application.PackageStatusNeedsReconciliation).Order("id").Limit(2).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 1 {
			return application.ErrUnavailable
		}
		for _, row := range rows {
			plan, err := decodePackagePlan(row)
			if err != nil {
				return err
			}
			for _, a := range plan.Assets {
				if a.ID == asset {
					found = true
				}
			}
		}
		return nil
	})
	return found, err
}

// InspectPackageExport holds current authorized rows while all actual bytes read.
func (s *PackageStore) InspectPackageExport(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, consume func(application.PackageExport) error) error {
	if s == nil || s.db == nil || consume == nil {
		return application.ErrUnavailable
	}
	return s.library.read(ctx, actor, scope, func(tx *gorm.DB, library libraryRow) error {
		folders, err := libraryFolders(tx, library.ID, false)
		if err != nil {
			return err
		}
		purging, err := storageExists(tx.Model(&purgeJobRow{}).Where("library_id = ? AND (status IN ? OR execution_unconfirmed = ? OR process_ended = ?)", library.ID, []domain.PurgeStatus{domain.PurgeStatusQueued, domain.PurgeStatusRunning, domain.PurgeStatusNeedsReconciliation, domain.PurgeStatusCancelRequested}, true, false))
		if err != nil {
			return err
		}
		if purging {
			return application.ErrPackageConflict
		}
		rows, err := packageExportEntries(tx, actor, scope, library.ID)
		if err != nil {
			return err
		}
		if len(rows) > application.MaxPackageEntries {
			return application.ErrInvalidPackage
		}
		// Compare identities, not counts: unrelated assets cannot mask a
		// missing declared original or a corrupt cross-scope catalog relation.
		var declared []uuid.UUID
		if err := tx.Model(&libraryItemRow{}).Where("library_id = ? AND purged_at IS NULL", library.ID).Order("id").Limit(2501).Pluck("id", &declared).Error; err != nil {
			return err
		}
		if len(declared) > application.MaxPackageEntries {
			return application.ErrInvalidPackage
		}
		present := make(map[uuid.UUID]bool, len(rows))
		for _, row := range rows {
			if present[row.ID] {
				return application.ErrUnavailable
			}
			present[row.ID] = true
		}
		for _, id := range declared {
			if !present[id] {
				return application.ErrPackageIncomplete
			}
		}
		export := application.PackageExport{Manifest: application.LibraryPackageManifest{App: application.LibraryPackageApplication, Version: application.LibraryPackageVersion, ExportedAt: s.clock().UTC(), LibraryKind: scope.Kind, Folders: []application.LibraryPackageFolder{}, Items: []application.LibraryPackageItem{}, Files: []application.LibraryPackageFile{}}, Objects: map[string]application.PackageObject{}}
		for _, f := range folders {
			export.Manifest.Folders = append(export.Manifest.Folders, application.LibraryPackageFolder{ID: f.ID, ParentID: f.ParentID, Name: f.Name, Style: f.Style, Theme: f.Theme, Position: f.Position})
		}
		var expanded int64
		for _, row := range rows {
			detail, err := packageExportDetail(tx, row, library.ID)
			if err != nil {
				return err
			}
			item := application.LibraryPackageItem{ID: row.ID.String(), Kind: row.Kind, Metadata: application.LibraryMetadata{PlainText: detail.PlainText, FolderID: row.FolderID, Title: row.Title, Category: row.Category, Tags: detail.Tags, SourceLabel: row.SourceLabel, Note: row.Note, Favorite: row.Favorite}, State: row.CatalogState, TrashedAt: row.TrashedAt, Position: row.Position}
			if row.AssetID != nil {
				a, err := decodePackageExportAsset(tx, row.Asset, row.CatalogState)
				if err != nil {
					return err
				}
				if a.SHA256 == nil || a.ContainsRealPerson || a.ConsentRecordID != nil {
					return application.ErrPackageUnsupportedKind
				}
				name, err := packageExportFileName(a)
				if err != nil {
					return err
				}
				filePath := fmt.Sprintf("files/%s/%s", a.ID, name)
				item.FilePath = &filePath
				export.Manifest.Files = append(export.Manifest.Files, application.LibraryPackageFile{Path: filePath, FileName: name, MIMEType: a.MimeType, ByteSize: a.ByteSize, SHA256: *a.SHA256})
				export.Objects[filePath] = application.PackageObject{Location: a.ObjectLocation, Key: a.ObjectKey, MIMEType: a.MimeType, SHA256: *a.SHA256, ByteSize: a.ByteSize}
				expanded += a.ByteSize
				if expanded > application.MaxPackageExpandedBytes {
					return application.ErrInvalidPackage
				}
			}
			export.Manifest.Items = append(export.Manifest.Items, item)
		}
		return consume(export)
	})
}

// NewScopedPackageStore binds live scope authority explicitly.
func NewScopedPackageStore(db *gorm.DB, project LibraryProjectAccessFactory, guards LibraryWorkGuardFactory, clock func() time.Time, scopes ScopeAccessFactory) *PackageStore {
	s := NewPackageStore(db, project, guards, clock)
	s.scopeAuthority = scopeAuthority{scopes: scopes}
	s.library.scopeAuthority = s.scopeAuthority
	return s
}
