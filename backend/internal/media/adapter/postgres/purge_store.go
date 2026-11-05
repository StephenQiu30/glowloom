package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	task "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// LibraryReferenceFactory binds all installed reference owners to the caller Tx.
type LibraryReferenceFactory func(*gorm.DB) application.LibraryReferences

// PurgeStore owns permanent acceptance and exact private removal reservations.
type PurgeStore struct {
	scopeAuthority
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
	Status                                                                         domain.PurgeStatus
	Stage                                                                          domain.PurgeStage
	Revision                                                                       int64
	CancellationRequested, NeedsReconciliation, ExecutionUnconfirmed, ProcessEnded bool
	ExecutionID, WorkerFence                                                       *uuid.UUID
	LeaseUntil                                                                     *time.Time
	CreatedAt, UpdatedAt                                                           time.Time
}

func (purgeJobRow) TableName() string { return "media.purge_job" }

type purgeCommandRecord struct {
	ActorID, OrgID, IdemKey, JobID uuid.UUID
	Action, RequestSHA256          string
	ResponseBody                   []byte
	EventID                        *uuid.UUID
}

func (purgeCommandRecord) TableName() string { return "media.purge_command" }

type purgeItemViewRow struct {
	Index       int `gorm:"column:item_index"`
	ItemID      uuid.UUID
	AssetID     *uuid.UUID
	Status      domain.PurgeItemStatus
	FailureCode *string
}

func (purgeItemViewRow) TableName() string { return "media.purge_item" }

type outboxRecord struct {
	ID           uuid.UUID
	Topic        string
	PartitionKey string
	Payload      jsonDocument
}

func (outboxRecord) TableName() string { return "infra.outbox" }

type purgeAuditRecord struct {
	ID         uuid.UUID
	OrgID      uuid.UUID
	ProjectID  *uuid.UUID
	ActorID    uuid.UUID
	ActorKind  string
	Action     string
	ObjectType string
	ObjectID   string
	After      jsonDocument
}

func (purgeAuditRecord) TableName() string { return "audit.audit_log" }

func (r purgeJobRow) scope() domain.LibraryScope {
	return domain.LibraryScope{Kind: domain.LibraryKind(r.ScopeKind), ProjectID: r.ProjectID}
}

func (s *PurgeStore) access(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, scope domain.LibraryScope, write bool) (application.LibraryProjectFacts, error) {
	return (&LibraryStore{project: s.project, scopeAuthority: s.scopeAuthority}).authorize(ctx, tx, actor, scope, write)
}
func purgeLock(tx *gorm.DB, actor, key uuid.UUID) error {
	return lockStorageIdempotencyKey(tx, storageIdempotencyPurge, actor, key)
}
func purgeHash(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil || len(body) > 1<<20 {
		return "", domain.ErrInvalidLibrary
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}
func purgeReplay(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, hash string, action application.PurgeAction) (*domain.PurgeJob, error) {
	var row purgeCommandRecord
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
	var result domain.PurgeJob
	if len(row.ResponseBody) > 1<<20 || strictCopyMedia(row.ResponseBody, &result) != nil || result.CurrentActorID != actor.ID || result.CurrentOrgID != actor.OrgID || result.ID != row.JobID || result.Revision < 1 || result.Scope.Validate() != nil {
		return nil, application.ErrUnavailable
	}
	return &result, nil
}
func (s *PurgeStore) readJob(tx *gorm.DB, actor identityapp.Principal, id uuid.UUID, lock bool) (purgeJobRow, error) {
	var row purgeJobRow
	query := tx.Where("id = ? AND actor_id = ? AND org_id = ?", id, actor.ID, actor.OrgID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	read := query.Take(&row)
	if read.Error != nil {
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return row, application.ErrNotFound
		}
		return row, read.Error
	}
	if row.scope().Validate() != nil || row.Revision < 1 || row.ItemCount < 1 || row.ItemCount > 200 {
		return row, application.ErrUnavailable
	}
	return row, nil
}
func purgeView(tx *gorm.DB, row purgeJobRow) (domain.PurgeJob, error) {
	var rows []purgeItemViewRow
	if err := tx.Where("job_id = ?", row.ID).Order("item_index").Find(&rows).Error; err != nil {
		return domain.PurgeJob{}, err
	}
	items := make([]domain.PurgeItemResult, 0, len(rows))
	for _, item := range rows {
		items = append(items, domain.PurgeItemResult{Index: item.Index, ItemID: item.ItemID, AssetID: item.AssetID, Status: item.Status, FailureCode: item.FailureCode})
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
		if err := s.requireCurrentActor(tx, actor); err != nil {
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
	var count int64
	err := s.db.WithContext(ctx).Model(&purgeJobRow{}).
		Where("project_id = ? AND org_id = ? AND (status IN ? OR execution_unconfirmed = ? OR process_ended = ?)", project, actor.OrgID, []domain.PurgeStatus{domain.PurgeStatusQueued, domain.PurgeStatusRunning, domain.PurgeStatusNeedsReconciliation, domain.PurgeStatusCancelRequested}, true, false).
		Limit(1).Count(&count).Error
	return count > 0, err
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
	packageBusy, err := NewScopedPackageStore(tx, s.project, nil, s.clock, s.scopes).HasInflightLibraryPackageWork(ctx, actor, scope)
	if err != nil {
		return err
	}
	blocked = blocked || packageBusy
	busy, err := storageExists(tx.Model(&transferJobRow{}).Where("(source_library_id = ? OR target_library_id = ?) AND (status IN ? OR execution_unconfirmed = ? OR process_ended = ?)", library, library, []domain.TransferStatus{domain.TransferStatusQueued, domain.TransferStatusRunning, domain.TransferStatusNeedsReconciliation, domain.TransferStatusCancelRequested}, true, false))
	if err != nil {
		return err
	}
	blocked = blocked || busy
	query := tx.Model(&assetRow{}).Where("status IN ? AND is_delete = ?", []domain.Status{domain.StatusUploading, domain.StatusProcessing}, false)
	if scope.Kind == domain.LibraryPersonal {
		query = query.Where("project_id IS NULL AND personal_org_id = ? AND personal_actor_id = ?", actor.OrgID, actor.ID)
	} else {
		query = query.Where("project_id = ?", *scope.ProjectID)
	}
	busy, err = storageExists(query)
	if err != nil {
		return err
	}
	if blocked || busy {
		return domain.ErrPurgeConflict
	}
	return nil
}

// PurgeTopic carries permanent owning commands, not model generation operations.
const PurgeTopic = "glowloom.media.purge_command.v1"

func recordPurgeCommand(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, hash string, action application.PurgeAction, job domain.PurgeJob, emit bool) error {
	response, err := json.Marshal(job)
	if err != nil || len(response) > 1<<20 {
		return application.ErrUnavailable
	}
	var eventID *uuid.UUID
	if emit {
		id := uuid.NewSHA1(key, []byte("media-purge/"+actor.ID.String()+"/"+string(action)))
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
		if action == application.PurgeActionCancel {
			if err := taskruntime.RequestStop(tx.Statement.Context, tx, task.Purge, map[string]any{"job_id": job.ID, "attempt": job.Attempt}); err != nil {
				return err
			}
		}
		if err := taskruntime.EnqueueReference(tx.Statement.Context, tx, task.Purge, id.String(), time.Now().UTC(), application.PurgeWorkID{JobID: job.ID, Attempt: job.Attempt}); err != nil {
			return err
		}
		outbox := outboxRecord{ID: id, Topic: PurgeTopic, PartitionKey: job.ID.String(), Payload: jsonDocument(payload)}
		if err := requireOne(tx.Create(&outbox), application.ErrUnavailable); err != nil {
			return err
		}
	}
	command := purgeCommandRecord{ActorID: actor.ID, OrgID: actor.OrgID, IdemKey: key, JobID: job.ID, Action: string(action), RequestSHA256: hash, ResponseBody: response, EventID: eventID}
	if err := requireOne(tx.Create(&command), application.ErrUnavailable); err != nil {
		return err
	}
	safe, _ := json.Marshal(struct {
		ID       uuid.UUID          `json:"id"`
		Revision int64              `json:"revision"`
		Count    int                `json:"count"`
		Status   domain.PurgeStatus `json:"status"`
	}{job.ID, job.Revision, len(job.Items), job.Status})
	id := uuid.NewSHA1(key, []byte("media-purge-audit/"+actor.ID.String()+"/"+string(action)))
	audit := purgeAuditRecord{ID: id, OrgID: actor.OrgID, ProjectID: job.Scope.ProjectID, ActorID: actor.ID, ActorKind: "user", Action: "media.purge." + string(action), ObjectType: "media.purge", ObjectID: job.ID.String(), After: jsonDocument(safe)}
	if err := requireOne(tx.Create(&audit), application.ErrUnavailable); err != nil {
		return fmt.Errorf("record permanent purge audit: %w", err)
	}
	return nil
}

// NewScopedPurgeStore binds live scope authority explicitly.
func NewScopedPurgeStore(db *gorm.DB, project LibraryProjectAccessFactory, guards LibraryWorkGuardFactory, references LibraryReferenceFactory, clock func() time.Time, scopes ScopeAccessFactory) *PurgeStore {
	s := NewPurgeStore(db, project, guards, references, clock)
	s.scopeAuthority = scopeAuthority{scopes: scopes}
	return s
}
