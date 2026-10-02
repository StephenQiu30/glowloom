package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// LibraryReferenceFactory binds all installed reference owners to the caller Tx.
type LibraryReferenceFactory func(*gorm.DB) application.LibraryReferences

// PurgeStore owns permanent acceptance and exact private removal reservations.
type PurgeStore struct {
	db         *gorm.DB
	project    LibraryProjectAccessFactory
	guards     LibraryWorkGuardFactory
	references LibraryReferenceFactory
	clock      func() time.Time
}

// NewPurgeStore explicitly requires workspace authorization, installed work and
// reference readers. A nil reader is unavailable, never an empty reference set.
func NewPurgeStore(db *gorm.DB, project LibraryProjectAccessFactory, guards LibraryWorkGuardFactory, references LibraryReferenceFactory, clock func() time.Time) *PurgeStore {
	if clock == nil {
		clock = time.Now
	}
	return &PurgeStore{db: db, project: project, guards: guards, references: references, clock: clock}
}

type purgeJobRow struct {
	ID, OrgID, ActorID, LibraryID                                                  uuid.UUID
	ProjectID                                                                      *uuid.UUID
	ScopeKind                                                                      string
	ItemCount, Attempt                                                             int
	Status, Stage                                                                  string
	Revision                                                                       int64
	CancellationRequested, NeedsReconciliation, ExecutionUnconfirmed, ProcessEnded bool
	ExecutionID, WorkerFence                                                       *uuid.UUID
	LeaseUntil                                                                     *time.Time
	CreatedAt, UpdatedAt                                                           time.Time
}

func (r purgeJobRow) scope() domain.LibraryScope {
	return domain.LibraryScope{Kind: domain.LibraryKind(r.ScopeKind), ProjectID: r.ProjectID}
}

func (s *PurgeStore) access(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, scope domain.LibraryScope, write bool) (application.LibraryProjectFacts, error) {
	return (&LibraryStore{project: s.project}).authorize(ctx, tx, actor, scope, write)
}
func purgeLock(tx *gorm.DB, actor, key uuid.UUID) error {
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "media-purge/"+actor.String()+"/"+key.String()).Error
}
func purgeHash(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil || len(body) > 1<<20 {
		return "", domain.ErrInvalidLibrary
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}
func purgeReplay(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, hash, action string) (*domain.PurgeJob, error) {
	var row struct {
		OrgID, JobID          uuid.UUID
		RequestSHA256, Action string
		ResponseBody          []byte
	}
	read := tx.Raw(`SELECT * FROM media.purge_command WHERE actor_id=? AND idem_key=?`, actor.ID, key).Scan(&row)
	if read.Error != nil {
		return nil, read.Error
	}
	if read.RowsAffected == 0 {
		return nil, nil
	}
	if row.OrgID != actor.OrgID || row.RequestSHA256 != hash || row.Action != action {
		return nil, application.ErrLibraryKeyConflict
	}
	var result domain.PurgeJob
	if len(row.ResponseBody) > 1<<20 || strictCopyMedia(row.ResponseBody, &result) != nil || result.CurrentActorID != actor.ID || result.CurrentOrgID != actor.OrgID || result.ID != row.JobID || result.Revision < 1 || result.Scope.Validate() != nil {
		return nil, application.ErrUnavailable
	}
	return &result, nil
}
func (s *PurgeStore) readJob(tx *gorm.DB, actor identityapp.Principal, id uuid.UUID, lock bool) (purgeJobRow, error) {
	query := `SELECT * FROM media.purge_job WHERE id=? AND actor_id=? AND org_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	var row purgeJobRow
	read := tx.Raw(query, id, actor.ID, actor.OrgID).Scan(&row)
	if read.Error != nil {
		return row, read.Error
	}
	if read.RowsAffected != 1 {
		return row, application.ErrNotFound
	}
	if row.scope().Validate() != nil || row.Revision < 1 || row.ItemCount < 1 || row.ItemCount > 200 {
		return row, application.ErrUnavailable
	}
	return row, nil
}
func purgeView(tx *gorm.DB, row purgeJobRow) (domain.PurgeJob, error) {
	items := make([]domain.PurgeItemResult, 0, row.ItemCount)
	if err := tx.Raw(`SELECT item_index AS index,item_id,asset_id,status,failure_code FROM media.purge_item WHERE job_id=? ORDER BY item_index`, row.ID).Scan(&items).Error; err != nil {
		return domain.PurgeJob{}, err
	}
	if len(items) != row.ItemCount {
		return domain.PurgeJob{}, application.ErrUnavailable
	}
	for i, item := range items {
		if item.Index != i || item.ItemID == uuid.Nil {
			return domain.PurgeJob{}, application.ErrUnavailable
		}
	}
	return domain.PurgeJob{ID: row.ID, CurrentActorID: row.ActorID, CurrentOrgID: row.OrgID, Scope: row.scope(), Status: row.Status, Stage: row.Stage, Attempt: row.Attempt, Revision: row.Revision, CancellationRequested: row.CancellationRequested, NeedsReconciliation: row.NeedsReconciliation, ExecutionUnconfirmed: row.ExecutionUnconfirmed, Items: items, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

// GetPurge checks current authority before returning creator-scoped safe facts.
func (s *PurgeStore) GetPurge(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (domain.PurgeJob, error) {
	var result domain.PurgeJob
	if s == nil || s.db == nil {
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
		if _, err := s.access(ctx, tx, actor, row.scope(), false); err != nil {
			return err
		}
		result, err = purgeView(tx, row)
		return err
	})
	return result, err
}

// HasInflightWork is consumed by the workspace lifecycle guard under its project
// lock. An unconfirmed physical worker blocks regardless of displayed status.
func (s *PurgeStore) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if project == uuid.Nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil {
		return false, identityapp.ErrForbidden
	}
	var present bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM media.purge_job j WHERE j.project_id=? AND j.org_id=? AND (j.status IN ('queued','running','needs_reconciliation','cancel_requested') OR j.execution_unconfirmed OR NOT j.process_ended))`, project, actor.OrgID).Scan(&present).Error
	return present, err
}

func (s *PurgeStore) scopeBusy(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, scope domain.LibraryScope, library uuid.UUID) error {
	blocked := false
	if scope.Kind == domain.LibraryProject {
		if s.guards == nil || s.guards(tx) == nil {
			return application.ErrUnavailable
		}
		busy, err := s.guards(tx).HasInflightProjectWork(ctx, actor, *scope.ProjectID)
		if err != nil {
			return err
		}
		blocked = blocked || busy
	}
	// The package reader authorizes this same current library transaction. Its
	// factory is unused, avoiding lifecycle recursion and a second connection.
	packageBusy, err := NewPackageStore(tx, s.project, nil, s.clock).HasInflightLibraryPackageWork(ctx, actor, scope)
	if err != nil {
		return err
	}
	blocked = blocked || packageBusy
	var busy bool
	query := `SELECT EXISTS(SELECT 1 FROM media.transfer_job WHERE (source_library_id=? OR target_library_id=?) AND (status IN ('queued','running','needs_reconciliation','cancel_requested') OR execution_unconfirmed OR NOT process_ended))`
	if err := tx.Raw(query, library, library).Scan(&busy).Error; err != nil {
		return err
	}
	blocked = blocked || busy
	ownership, args := `project_id=?`, []any{scope.ProjectID}
	if scope.Kind == domain.LibraryPersonal {
		ownership, args = `project_id IS NULL AND personal_org_id=? AND personal_actor_id=?`, []any{actor.OrgID, actor.ID}
	}
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.media_asset WHERE `+ownership+` AND status IN ('uploading','processing') AND NOT is_delete)`, args...).Scan(&busy).Error; err != nil {
		return err
	}
	if blocked || busy {
		return domain.ErrPurgeConflict
	}
	return nil
}

// PurgeTopic carries permanent owning commands, not model generation operations.
const PurgeTopic = "lanverse.media.purge_command.v1"

func recordPurgeCommand(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, hash, action string, job domain.PurgeJob, emit bool) error {
	response, err := json.Marshal(job)
	if err != nil || len(response) > 1<<20 {
		return application.ErrUnavailable
	}
	var eventID *uuid.UUID
	if emit {
		id := uuid.NewSHA1(key, []byte("media-purge/"+actor.ID.String()+"/"+action))
		eventID = &id
		payload, err := json.Marshal(struct {
			EventID    uuid.UUID                 `json:"event_id"`
			EventType  string                    `json:"event_type"`
			OrgID      uuid.UUID                 `json:"org_id"`
			OccurredAt time.Time                 `json:"occurred_at"`
			Data       application.PurgeDelivery `json:"data"`
		}{id, PurgeTopic, actor.OrgID, time.Now().UTC(), application.PurgeDelivery{PurgeWorkID: application.PurgeWorkID{JobID: job.ID, Attempt: job.Attempt}, EventID: id, RequestID: key, ActorID: actor.ID, OrgID: actor.OrgID, Action: action}})
		if err != nil {
			return err
		}
		if err := libraryChanged(tx, `INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, id, PurgeTopic, job.ID.String(), string(payload)); err != nil {
			return err
		}
	}
	if err := libraryChanged(tx, `INSERT INTO media.purge_command(actor_id,org_id,idem_key,job_id,action,request_sha256,response_body,event_id) VALUES(?,?,?,?,?,?,?,?)`, actor.ID, actor.OrgID, key, job.ID, action, hash, response, eventID); err != nil {
		return err
	}
	safe, _ := json.Marshal(struct {
		ID       uuid.UUID `json:"id"`
		Revision int64     `json:"revision"`
		Count    int       `json:"count"`
		Status   string    `json:"status"`
	}{job.ID, job.Revision, len(job.Items), job.Status})
	id := uuid.NewSHA1(key, []byte("media-purge-audit/"+actor.ID.String()+"/"+action))
	if err := libraryChanged(tx, `INSERT INTO audit.audit_log(id,org_id,project_id,actor_id,actor_kind,action,object_type,object_id,after) VALUES(?,?,?,?,'user',?,'media.purge',?,?::jsonb)`, id, actor.OrgID, job.Scope.ProjectID, actor.ID, "media.purge."+action, job.ID.String(), string(safe)); err != nil {
		return fmt.Errorf("record permanent purge audit: %w", err)
	}
	return nil
}
