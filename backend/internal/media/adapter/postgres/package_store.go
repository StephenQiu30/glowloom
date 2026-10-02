package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
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
	db      *gorm.DB
	library *LibraryStore
	guards  LibraryWorkGuardFactory
	clock   func() time.Time
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
	Status                                     string
	Revision, LibraryRevision, ProjectRevision int64
	CreatedAt, UpdatedAt                       time.Time
}

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
	if row.Status == "succeeded" {
		var stored application.PackageJob
		if decodePackageFrozen(row.Result, &stored) != nil || !packageSameJob(stored, result) {
			return result, application.ErrUnavailable
		}
		return stored, nil
	}
	return result, nil
}

func packageSameJob(a, b application.PackageJob) bool {
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
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if _, err := s.library.authorize(ctx, tx, actor, scope, write); err != nil {
			return err
		}
		if scope.Kind == domain.LibraryProject && write {
			var err error
			aspect, err = NewStore(tx).AuthorizeUpload(ctx, actor, *scope.ProjectID)
			return err
		}
		return nil
	})
	return aspect, err
}

func packageCommandLock(tx *gorm.DB, actor, key uuid.UUID) error {
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "media-package/"+actor.String()+"/"+key.String()).Error
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
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := packageCommandLock(tx, actor.ID, in.Key); err != nil {
			return err
		}
		if _, err := s.library.authorize(ctx, tx, actor, in.Scope, false); err != nil {
			return err
		}
		var row packageJobRow
		read := tx.Raw(`SELECT * FROM media.package_job WHERE actor_id=? AND org_id=? AND idem_key=?`, actor.ID, actor.OrgID, in.Key).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 0 {
			return nil
		}
		if row.RequestSHA256 != hash || row.LibraryKind != string(in.Scope.Kind) || !sameScopeProject(row.ProjectID, in.Scope.ProjectID) {
			return application.ErrPackageConflict
		}
		decoded, err := decodePackagePlan(row)
		if err != nil {
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
	if err := requireCurrentActor(tx, actor); err != nil {
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
		if err := tx.Exec(`INSERT INTO media.library(id,kind,org_id,project_id,personal_actor_id)VALUES(?,?,?,?,?)ON CONFLICT(id)DO NOTHING`, id, scope.Kind, actor.OrgID, scope.ProjectID, personal).Error; err != nil {
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
		if err := requireCurrentActor(tx, actor); err != nil {
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
		read := tx.Raw(`SELECT * FROM media.package_job WHERE actor_id=? AND idem_key=?`, actor.ID, in.Key).Scan(&existing)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 1 {
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
		var active bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.package_job WHERE library_id=? AND status='needs_reconciliation') OR EXISTS(SELECT 1 FROM media.transfer_job WHERE (source_library_id=? OR target_library_id=?) AND (status IN ('queued','running','needs_reconciliation','cancel_requested') OR execution_unconfirmed OR NOT process_ended)) OR EXISTS(SELECT 1 FROM media.purge_job WHERE library_id=? AND (status IN ('queued','running','needs_reconciliation','cancel_requested') OR execution_unconfirmed OR NOT process_ended))`, library.ID, library.ID, library.ID, library.ID).Scan(&active).Error; err != nil {
			return err
		}
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
		row := packageJobRow{ID: plan.JobID, OrgID: actor.OrgID, ActorID: actor.ID, LibraryID: library.ID, LibraryKind: string(in.Scope.Kind), ProjectID: in.Scope.ProjectID, IdemKey: in.Key, RequestSHA256: hash, ArchiveSHA256: in.ArchiveSHA256, ArchiveBytes: in.ArchiveBytes, Frozen: body, FrozenSHA256: hex.EncodeToString(digest[:]), Status: "needs_reconciliation", Revision: 1, LibraryRevision: library.Revision, ProjectRevision: facts.Revision, CreatedAt: plan.CreatedAt, UpdatedAt: plan.CreatedAt}
		if _, err := decodePackagePlan(row); err != nil {
			return err
		}
		if err := libraryChanged(tx, `INSERT INTO media.package_job(id,org_id,actor_id,library_id,library_kind,project_id,idem_key,request_sha256,archive_sha256,archive_bytes,frozen,frozen_sha256,status,revision,library_revision,project_revision,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,'needs_reconciliation',1,?,?,?,?)`, row.ID, row.OrgID, row.ActorID, row.LibraryID, row.LibraryKind, row.ProjectID, row.IdemKey, row.RequestSHA256, row.ArchiveSHA256, row.ArchiveBytes, row.Frozen, row.FrozenSHA256, row.LibraryRevision, row.ProjectRevision, row.CreatedAt, row.UpdatedAt); err != nil {
			return err
		}
		for index, object := range plan.Objects {
			if err := libraryChanged(tx, `INSERT INTO media.package_object(job_id,object_index,object_key,byte_size,content_type,sha256)VALUES(?,?,?,?,?,?)`, row.ID, index, object.Key, object.ByteSize, object.MIMEType, object.SHA256); err != nil {
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
	query := `SELECT * FROM media.package_job WHERE id=? AND org_id=? AND actor_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	read := tx.Raw(query, id, actor.OrgID, actor.ID).Scan(&row)
	if read.Error != nil {
		return row, application.PackagePlan{}, read.Error
	}
	if read.RowsAffected != 1 {
		return row, application.PackagePlan{}, application.ErrNotFound
	}
	plan, err := decodePackagePlan(row)
	return row, plan, err
}

// GetPackage cannot expose another actor's source manifest or old receipt.
func (s *PackageStore) GetPackage(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (application.PackageJob, error) {
	var result application.PackageJob
	if s == nil || s.db == nil || id == uuid.Nil {
		return result, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
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
		if err := tx.Raw(`SELECT count(*) FROM media.package_job WHERE org_id=? AND actor_id=? AND library_id=?`, actor.OrgID, actor.ID, library.ID).Scan(&result.Total).Error; err != nil {
			return err
		}
		var rows []packageJobRow
		if err := tx.Raw(`SELECT * FROM media.package_job WHERE org_id=? AND actor_id=? AND library_id=? ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, actor.OrgID, actor.ID, library.ID, size, (page-1)*size).Scan(&rows).Error; err != nil {
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
	var row struct {
		JobID, OrgID          uuid.UUID
		Action, RequestSHA256 string
		Response              []byte
	}
	read := tx.Raw(`SELECT job_id,org_id,action,request_sha256,response FROM media.package_command WHERE actor_id=? AND idem_key=?`, actor.ID, key).Scan(&row)
	if read.Error != nil {
		return nil, read.Error
	}
	if read.RowsAffected == 0 {
		return nil, nil
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
	err = libraryChanged(tx, `INSERT INTO media.package_command(actor_id,org_id,idem_key,job_id,action,request_sha256,response)VALUES(?,?,?,?,?,?,?)`, actor.ID, actor.OrgID, key, job.ID, action, hash, body)
	return job, err
}

func (s *PackageStore) runPackage(ctx context.Context, actor identityapp.Principal, id uuid.UUID, cancel bool, command *application.PackageControl, work func(application.PackagePlan) error) (application.PackageJob, error) {
	var result application.PackageJob
	if s == nil || s.db == nil || id == uuid.Nil || work == nil {
		return result, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
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
		library, facts, err := s.scopeLock(ctx, tx, actor, packageScope(initial), initial.Status == "needs_reconciliation")
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
		if row.Status != "needs_reconciliation" {
			if cancel && row.Status == "succeeded" {
				return application.ErrPackageConflict
			}
			result, err = savePackageCommand(tx, actor, key, hash, action, result)
			return err
		}
		if !cancel && (library.Revision != plan.Request.ExpectedRevision || facts.Revision != plan.Request.ExpectedProjectRevision) {
			return application.ErrPackageConflict
		}
		var objects []struct {
			ObjectIndex                            int
			ObjectKey, ContentType, SHA256, Status string
			ByteSize                               int64
		}
		if err := tx.Raw(`SELECT * FROM media.package_object WHERE job_id=? ORDER BY object_index FOR UPDATE`, id).Scan(&objects).Error; err != nil {
			return err
		}
		if len(objects) != len(plan.Objects) {
			return application.ErrUnavailable
		}
		for index, object := range objects {
			frozen := plan.Objects[index]
			if object.ObjectIndex != index || object.ObjectKey != frozen.Key || object.ContentType != frozen.MIMEType || object.SHA256 != frozen.SHA256 || object.ByteSize != frozen.ByteSize || object.Status == "removed" && !cancel {
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
		objectStatus := "removed"
		row.Status = "cancelled"
		if !cancel {
			if err := publishPackage(tx, library, plan); err != nil {
				return err
			}
			if err := libraryChanged(tx, `UPDATE media.library SET revision=revision+1,update_time=? WHERE id=? AND revision=?`, now, library.ID, library.Revision); err != nil {
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
			row.Status = "succeeded"
			objectStatus = "verified"
		}
		if err := tx.Exec(`UPDATE media.package_object SET status=? WHERE job_id=?`, objectStatus, id).Error; err != nil {
			return err
		}
		result, err = packageJob(row, plan)
		if row.Status == "succeeded" {
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
		if err := libraryChanged(tx, `UPDATE media.package_job SET status=?,revision=?,library_revision=?,project_revision=?,result=?,updated_at=? WHERE id=? AND status='needs_reconciliation' AND revision=?`, row.Status, row.Revision, row.LibraryRevision, row.ProjectRevision, response, now, id, row.Revision-1); err != nil {
			return err
		}
		safe, _ := json.Marshal(map[string]any{"revision": row.Revision, "status": row.Status, "item_count": len(plan.Items), "folder_count": len(plan.Folders)})
		auditAction := "media.package.imported"
		if cancel {
			auditAction = "media.package.cancelled"
		}
		if err := libraryChanged(tx, `INSERT INTO audit.audit_log(id,org_id,project_id,actor_id,actor_kind,action,object_type,object_id,after,request_id)VALUES(?,?,?,?,'user',?,'media.package',?,?::jsonb,?)`, uuid.NewSHA1(id, []byte(auditAction)), actor.OrgID, row.ProjectID, actor.ID, auditAction, id.String(), string(safe), plan.RequestID.String()); err != nil {
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
			if err := libraryChanged(tx, `INSERT INTO media.library_folder(id,library_id,library_kind,parent_id,name,name_key,position,style,theme,revision,create_time,update_time)VALUES(?,?,?,?,?,?,?,?,?,1,?,?)`, folder.ID, library.ID, folder.Kind, folder.ParentID, folder.Name, folder.NameKey(), folder.Position, folder.Style, folder.Theme, folder.CreatedAt, folder.UpdatedAt); err != nil {
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
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if _, err := s.library.authorize(ctx, tx, actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, false); err != nil {
			return err
		}
		return tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.package_job WHERE org_id=? AND project_id=? AND status='needs_reconciliation')`, actor.OrgID, project).Scan(&busy).Error
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
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.package_job WHERE org_id=? AND library_id=? AND status='needs_reconciliation')`, actor.OrgID, library.ID).Scan(&busy).Error; err != nil {
			return fmt.Errorf("%w: read retained library packages: %w", application.ErrUnavailable, err)
		}
		return nil
	})
	return busy, err
}

// StorageUsageKeys includes retained ZIPs and all possibly written private intents.
func (s *PackageStore) StorageUsageKeys(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope) ([]string, error) {
	keys := []string{}
	if s == nil || s.db == nil {
		return nil, application.ErrUnavailable
	}
	err := s.library.read(ctx, actor, scope, func(tx *gorm.DB, library libraryRow) error {
		if err := tx.Raw(`SELECT o.object_key FROM media.package_object o JOIN media.package_job j ON j.id=o.job_id WHERE j.org_id=? AND j.library_id=? AND o.status<>'removed' ORDER BY o.object_key LIMIT 50001`, actor.OrgID, library.ID).Scan(&keys).Error; err != nil {
			return err
		}
		if len(keys) > 50000 {
			return application.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// HasPackageReferences protects every unpublished reserved asset identity.
func (s *PackageStore) HasPackageReferences(ctx context.Context, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	var found bool
	err := s.library.read(ctx, actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, func(tx *gorm.DB, library libraryRow) error {
		var rows []packageJobRow
		if err := tx.Raw(`SELECT * FROM media.package_job WHERE org_id=? AND library_id=? AND status='needs_reconciliation' ORDER BY id LIMIT 2`, actor.OrgID, library.ID).Scan(&rows).Error; err != nil {
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
		var purging bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.purge_job WHERE library_id=? AND (status IN ('queued','running','needs_reconciliation','cancel_requested') OR execution_unconfirmed OR NOT process_ended))`, library.ID).Scan(&purging).Error; err != nil {
			return err
		}
		if purging {
			return application.ErrPackageConflict
		}
		query, args := packageExportEntries(actor, scope, library.ID)
		var rows []libraryEntryRow
		if err := tx.Raw(query+`SELECT * FROM entries ORDER BY id LIMIT 2501`, args...).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) > application.MaxPackageEntries {
			return application.ErrInvalidPackage
		}
		// Compare identities, not counts: unrelated legacy assets cannot mask a
		// missing declared original or a corrupt cross-scope catalog relation.
		var declared []uuid.UUID
		if err := tx.Raw(`SELECT id FROM media.library_item WHERE library_id=? AND purged_at IS NULL ORDER BY id LIMIT 2501`, library.ID).Scan(&declared).Error; err != nil {
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
		export := application.PackageExport{Manifest: application.LibraryPackageManifest{App: "lanverse-media-library", Version: 1, ExportedAt: s.clock().UTC(), LibraryKind: scope.Kind, Folders: []application.LibraryPackageFolder{}, Items: []application.LibraryPackageItem{}, Files: []application.LibraryPackageFile{}}, Objects: map[string]application.PackageObject{}}
		for _, f := range folders {
			export.Manifest.Folders = append(export.Manifest.Folders, application.LibraryPackageFolder{ID: f.ID, ParentID: f.ParentID, Name: f.Name, Style: f.Style, Theme: f.Theme, Position: f.Position})
		}
		var expanded int64
		for _, row := range rows {
			detail, err := packageExportDetail(row, library.ID)
			if err != nil {
				return err
			}
			item := application.LibraryPackageItem{ID: row.ID.String(), Kind: row.Kind, Metadata: application.LibraryMetadata{PlainText: detail.PlainText, FolderID: row.FolderID, Title: row.Title, Category: row.Category, Tags: detail.Tags, SourceLabel: row.SourceLabel, Note: row.Note, Favorite: row.Favorite}, State: row.CatalogState, TrashedAt: row.TrashedAt, Position: row.Position}
			if row.AssetID != nil {
				a, err := decodePackageExportAsset(row.AssetFacts, row.CatalogState)
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
				export.Objects[filePath] = application.PackageObject{Key: a.ObjectKey, MIMEType: a.MimeType, SHA256: *a.SHA256, ByteSize: a.ByteSize}
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
