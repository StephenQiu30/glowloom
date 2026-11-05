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
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

// DepthMediaFactory binds exact pending result cleanup to its media owner.
type DepthMediaFactory func(*gorm.DB) application.DepthDerivedMedia

// DepthStore owns native depth facts and consumes the existing canvas/media ports.
type DepthStore struct {
	db      *gorm.DB
	source  TranscriptionSourceFactory
	media   DepthMediaFactory
	storage StorageFactory
}

// NewDepthStore injects transaction-bound saved source and pending media owners.
func NewDepthStore(db *gorm.DB, source TranscriptionSourceFactory, media DepthMediaFactory) *DepthStore {
	return &DepthStore{db: db, source: source, media: media}
}

type depthRow struct {
	ID                                                                                                   uuid.UUID `gorm:"primaryKey"`
	ActorRole                                                                                            *string
	ProjectID, OrgID, ActorID, CanvasID, NodeID, SourceAssetID                                           uuid.UUID
	ProfileID, SourceSHA256, FrozenSHA256                                                                string
	Status                                                                                               domain.DepthStatus
	Stage                                                                                                domain.DepthStage
	ProcessState                                                                                         domain.DepthProcessState
	SourceRevision, SourceAssetRevision, Revision                                                        int64
	Attempt                                                                                              int
	Frozen                                                                                               jsonDocument
	ActiveWorker                                                                                         *uuid.UUID
	FailureCode                                                                                          *string
	Retryable, NeedsReconciliation, ExecutionUnconfirmed, CancellationRequested, ReconciliationRequested bool
	CreatedAt, UpdatedAt                                                                                 time.Time
}

func (depthRow) TableName() string { return "mediatool.depth_job" }

func (r depthRow) job() domain.DepthJob {
	return domain.DepthJob{ID: r.ID, ProjectID: r.ProjectID, Source: domain.Source{CanvasID: r.CanvasID, NodeID: r.NodeID, Revision: r.SourceRevision}, SourceAssetID: r.SourceAssetID, SourceAssetRevision: r.SourceAssetRevision, SourceSHA256: r.SourceSHA256, ProfileID: r.ProfileID, Status: r.Status, Stage: r.Stage, Attempt: r.Attempt, Revision: r.Revision, FailureCode: r.FailureCode, Retryable: r.Retryable, NeedsReconciliation: r.NeedsReconciliation, ExecutionUnconfirmed: r.ExecutionUnconfirmed, CancellationRequested: r.CancellationRequested, ReconciliationRequested: r.ReconciliationRequested, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func (r depthRow) actor() identityapp.Principal {
	return identityapp.Principal{ID: r.ActorID, OrgID: r.OrgID}
}
func (r depthRow) state() domain.DepthExecution {
	s := domain.DepthExecution{Job: r.job(), ProcessState: r.ProcessState}
	if r.ActiveWorker != nil {
		s.WorkerID = *r.ActiveWorker
	}
	return s
}
func (r *depthRow) apply(s domain.DepthExecution) {
	j := s.Job
	r.Status = j.Status
	r.Stage = j.Stage
	r.Attempt = j.Attempt
	r.Revision = j.Revision
	r.ProcessState = s.ProcessState
	if r.ProcessState == "" {
		r.ProcessState = domain.DepthProcessNone
	}
	r.ActiveWorker = nil
	if s.WorkerID != uuid.Nil {
		r.ActiveWorker = &s.WorkerID
	}
	r.Retryable = j.Retryable
	r.NeedsReconciliation = j.NeedsReconciliation
	r.ExecutionUnconfirmed = j.ExecutionUnconfirmed
	r.CancellationRequested = j.CancellationRequested
	r.FailureCode = j.FailureCode
	r.UpdatedAt = time.Now().UTC()
}

func depthJSON(body []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return domain.ErrInvalidDepthJob
	}
	return nil
}
func depthFrozen(row depthRow) (domain.FrozenDepth, error) {
	var f domain.FrozenDepth
	if depthJSON(row.Frozen, &f) != nil || f.Validate() != nil || f.ProfileID != row.ProfileID || f.Input.AssetID != row.SourceAssetID || f.Input.Revision != row.SourceAssetRevision || f.Input.SHA256 != row.SourceSHA256 {
		return f, domain.ErrInvalidDepthJob
	}
	body, err := json.Marshal(f)
	if err != nil {
		return f, err
	}
	h := sha256.Sum256(body)
	if hex.EncodeToString(h[:]) != row.FrozenSHA256 {
		return f, domain.ErrInvalidDepthJob
	}
	return f, nil
}
func readDepth(tx *gorm.DB, project, id uuid.UUID, lock bool) (depthRow, error) {
	var r depthRow
	query := tx.Model(&depthRow{}).Where("id = ?", id)
	if project != uuid.Nil {
		query = query.Where("project_id = ?", project)
	}
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Take(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r, application.ErrNotFound
		}
		return r, fmt.Errorf("read depth job: %w", err)
	}
	if r.ID == uuid.Nil {
		return r, application.ErrNotFound
	}
	if _, err := depthFrozen(r); err != nil {
		return r, err
	}
	return r, nil
}
func saveDepth(tx *gorm.DB, before, after depthRow) error {
	if after.Revision != before.Revision+1 {
		return domain.ErrInvalidDepthJob
	}
	result := tx.Model(&depthRow{}).Where("id = ? AND revision = ?", after.ID, before.Revision).Updates(map[string]any{
		"status": after.Status, "stage": after.Stage, "attempt": after.Attempt, "revision": after.Revision,
		"process_state": after.ProcessState, "active_worker": after.ActiveWorker, "failure_code": after.FailureCode,
		"retryable": after.Retryable, "needs_reconciliation": after.NeedsReconciliation,
		"execution_unconfirmed": after.ExecutionUnconfirmed, "cancellation_requested": after.CancellationRequested,
		"reconciliation_requested": after.ReconciliationRequested, "updated_at": after.UpdatedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return application.ErrConflict
	}
	return nil
}
func (s *DepthStore) authorize(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, project uuid.UUID, write bool) error {
	if s == nil || s.media == nil {
		return application.ErrUnavailable
	}
	return normalize(s.media(tx).AuthorizeProject(ctx, actor, project, write))
}
func (s *DepthStore) validateSource(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, r depthRow) ([]mediadomain.MediaAsset, error) {
	f, err := depthFrozen(r)
	if err != nil {
		return nil, err
	}
	assets, err := s.media(tx).Sources(ctx, actor, r.ProjectID, []uuid.UUID{f.Input.AssetID})
	if err != nil {
		return nil, normalize(err)
	}
	if len(assets) != 1 {
		return nil, application.ErrConflict
	}
	a := assets[0]
	if a.ID != f.Input.AssetID || a.ProjectID != r.ProjectID || a.Kind != "video" || a.Revision != f.Input.Revision || a.SHA256 == nil || *a.SHA256 != f.Input.SHA256 || a.ObjectKey != f.Input.ObjectKey || a.ByteSize != f.Input.ByteSize || a.MimeType != f.Input.MIMEType {
		return nil, application.ErrConflict
	}
	return assets, nil
}
func depthCommandLock(tx *gorm.DB, actor, key uuid.UUID) error {
	if actor == uuid.Nil || key == uuid.Nil {
		return application.ErrInvalidDepthInput
	}
	return lockMediatoolPrincipal(tx, actor)
}
func depthReplay(tx *gorm.DB, actor, key uuid.UUID, hash string) (domain.DepthJob, bool, error) {
	var receipt depthCommandRecord
	err := tx.Where("actor_id = ? AND request_id = ?", actor, key).Take(&receipt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.DepthJob{}, false, nil
	}
	if err != nil {
		return domain.DepthJob{}, false, err
	}
	if receipt.RequestHash != hash {
		return domain.DepthJob{}, false, application.ErrConflict
	}
	var j domain.DepthJob
	err = depthJSON(receipt.Response, &j)
	return j, true, err
}
func depthQuota(tx *gorm.DB, project uuid.UUID) error {
	if err := lockMediatoolProject(tx, project); err != nil {
		return err
	}
	var n int64
	if err := tx.Model(&depthRow{}).Where("project_id = ? AND (status IN ? OR needs_reconciliation = ? OR execution_unconfirmed = ?)", project, []domain.DepthStatus{domain.DepthQueued, domain.DepthRunning, domain.DepthReviewRequired, domain.DepthCancelRequested}, true, true).Count(&n).Error; err != nil {
		return err
	}
	if n >= 2 {
		return application.ErrConflict
	}
	return nil
}

// Create freezes a saved formal video, permanent receipt and outbox atomically.
func (s *DepthStore) Create(ctx context.Context, actor identityapp.Principal, project, key uuid.UUID, in application.DepthCreateInput) (domain.DepthJob, error) {
	var job domain.DepthJob
	if s == nil || s.db == nil || s.media == nil {
		return job, application.ErrUnavailable
	}
	if project == uuid.Nil || in.CanvasID == uuid.Nil || in.NodeID == uuid.Nil || in.Revision < 1 {
		return job, application.ErrInvalidDepthInput
	}
	hash, err := commandHash("depth_create", project, uuid.Nil, in)
	if err != nil {
		return job, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, true); err != nil {
			return err
		}
		if err := depthCommandLock(tx, actor.ID, key); err != nil {
			return err
		}
		prior, found, err := depthReplay(tx, actor.ID, key, hash)
		if err != nil {
			return err
		}
		if found {
			job = prior
			return nil
		}
		if s.source == nil {
			return application.ErrUnavailable
		}
		if err := depthQuota(tx, project); err != nil {
			return err
		}
		id, err := s.source(tx).FreezeTranscriptionSource(ctx, actor, project, in.CanvasID, in.NodeID, in.Revision)
		if err != nil {
			if errors.Is(normalize(err), application.ErrInvalidExport) {
				return application.ErrInvalidDepthInput
			}
			return normalize(err)
		}
		assets, err := s.media(tx).Sources(ctx, actor, project, []uuid.UUID{id})
		if err != nil {
			return normalize(err)
		}
		if len(assets) != 1 || assets[0].ID != id || assets[0].Kind != "video" || assets[0].SHA256 == nil {
			return application.ErrInvalidDepthInput
		}
		a := assets[0]
		f := domain.FrozenDepth{ProfileID: domain.DepthProfileID, Input: domain.FrozenSource{AssetID: a.ID, Revision: a.Revision, Kind: string(a.Kind), ObjectKey: a.ObjectKey, MIMEType: a.MimeType, ByteSize: a.ByteSize, SHA256: *a.SHA256, Width: a.Width, Height: a.Height, DurationMS: a.DurationMS}}
		if f.Validate() != nil {
			return application.ErrInvalidDepthInput
		}
		body, err := json.Marshal(f)
		if err != nil {
			return err
		}
		sha := sha256.Sum256(body)
		now := time.Now().UTC().Truncate(time.Microsecond)
		job = domain.DepthJob{ID: uuid.New(), ProjectID: project, Source: domain.Source{CanvasID: in.CanvasID, NodeID: in.NodeID, Revision: in.Revision}, SourceAssetID: a.ID, SourceAssetRevision: a.Revision, SourceSHA256: *a.SHA256, ProfileID: domain.DepthProfileID, Status: domain.DepthQueued, Stage: domain.DepthStageQueued, Attempt: 1, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if err := pinTool(ctx, tx, s.storage, mediaapp.StorageWorkKindDepth, project, job.ID, job.Attempt); err != nil {
			return err
		}
		if err := tx.Create(&depthRow{ID: job.ID, ProjectID: project, OrgID: actor.OrgID, ActorID: actor.ID, CanvasID: in.CanvasID, NodeID: in.NodeID, SourceRevision: in.Revision, SourceAssetID: a.ID, SourceAssetRevision: a.Revision, SourceSHA256: *a.SHA256, ProfileID: domain.DepthProfileID, Frozen: jsonDocument(body), FrozenSHA256: hex.EncodeToString(sha[:]), Status: domain.DepthQueued, Stage: domain.DepthStageQueued, Attempt: 1, Revision: 1, ProcessState: domain.DepthProcessNone, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			return err
		}
		return recordDepthCommand(tx, actor, key, hash, "requested", job, "start")
	})
	return job, err
}

var _ application.DepthStore = (*DepthStore)(nil)
