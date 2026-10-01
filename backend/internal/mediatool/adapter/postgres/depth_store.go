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

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// DepthMediaFactory binds exact pending result cleanup to its media owner.
type DepthMediaFactory func(*gorm.DB) application.DepthDerivedMedia

// DepthStore owns native depth facts and consumes the existing canvas/media ports.
type DepthStore struct {
	db     *gorm.DB
	source TranscriptionSourceFactory
	media  DepthMediaFactory
}

// NewDepthStore injects transaction-bound saved source and pending media owners.
func NewDepthStore(db *gorm.DB, source TranscriptionSourceFactory, media DepthMediaFactory) *DepthStore {
	return &DepthStore{db: db, source: source, media: media}
}

type depthRow struct {
	ID, ProjectID, OrgID, ActorID, CanvasID, NodeID, SourceAssetID                                       uuid.UUID
	ActorRole, ProfileID, SourceSHA256, FrozenSHA256, Status, Stage, ProcessState                        string
	SourceRevision, SourceAssetRevision, Revision                                                        int64
	Attempt                                                                                              int
	Frozen                                                                                               []byte
	ActiveWorker                                                                                         *uuid.UUID
	FailureCode                                                                                          *string
	Retryable, NeedsReconciliation, ExecutionUnconfirmed, CancellationRequested, ReconciliationRequested bool
	CreatedAt, UpdatedAt                                                                                 time.Time
}

func (r depthRow) job() domain.DepthJob {
	return domain.DepthJob{ID: r.ID, ProjectID: r.ProjectID, Source: domain.Source{CanvasID: r.CanvasID, NodeID: r.NodeID, Revision: r.SourceRevision}, SourceAssetID: r.SourceAssetID, SourceAssetRevision: r.SourceAssetRevision, SourceSHA256: r.SourceSHA256, ProfileID: r.ProfileID, Status: domain.DepthStatus(r.Status), Stage: r.Stage, Attempt: r.Attempt, Revision: r.Revision, FailureCode: r.FailureCode, Retryable: r.Retryable, NeedsReconciliation: r.NeedsReconciliation, ExecutionUnconfirmed: r.ExecutionUnconfirmed, CancellationRequested: r.CancellationRequested, ReconciliationRequested: r.ReconciliationRequested, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func (r depthRow) actor() identityapp.Principal {
	return identityapp.Principal{ID: r.ActorID, OrgID: r.OrgID, Role: identitydomain.Role(r.ActorRole)}
}
func (r depthRow) state() domain.DepthExecution {
	s := domain.DepthExecution{Job: r.job(), ProcessState: domain.DepthProcessState(r.ProcessState)}
	if r.ActiveWorker != nil {
		s.WorkerID = *r.ActiveWorker
	}
	return s
}
func (r *depthRow) apply(s domain.DepthExecution) {
	j := s.Job
	r.Status = string(j.Status)
	r.Stage = j.Stage
	r.Attempt = j.Attempt
	r.Revision = j.Revision
	r.ProcessState = string(s.ProcessState)
	if r.ProcessState == "" {
		r.ProcessState = "none"
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
	q := `SELECT * FROM mediatool.depth_job WHERE id=?`
	args := []any{id}
	if project != uuid.Nil {
		q += ` AND project_id=?`
		args = append(args, project)
	}
	if lock {
		q += ` FOR UPDATE`
	}
	res := tx.Raw(q, args...).Scan(&r)
	if res.Error != nil {
		return r, fmt.Errorf("read depth job: %w", res.Error)
	}
	if res.RowsAffected != 1 {
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
	r := tx.Exec(`UPDATE mediatool.depth_job SET status=?,stage=?,attempt=?,revision=?,process_state=?,active_worker=?,failure_code=?,retryable=?,needs_reconciliation=?,execution_unconfirmed=?,cancellation_requested=?,reconciliation_requested=?,updated_at=? WHERE id=? AND revision=?`, after.Status, after.Stage, after.Attempt, after.Revision, after.ProcessState, after.ActiveWorker, after.FailureCode, after.Retryable, after.NeedsReconciliation, after.ExecutionUnconfirmed, after.CancellationRequested, after.ReconciliationRequested, after.UpdatedAt, after.ID, before.Revision)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
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
func (s *DepthStore) validateSource(ctx context.Context, tx *gorm.DB, r depthRow) error {
	f, err := depthFrozen(r)
	if err != nil {
		return err
	}
	assets, err := s.media(tx).Sources(ctx, r.actor(), r.ProjectID, []uuid.UUID{f.Input.AssetID})
	if err != nil {
		return normalize(err)
	}
	if len(assets) != 1 {
		return application.ErrConflict
	}
	a := assets[0]
	if a.Kind != "video" || a.Revision != f.Input.Revision || a.SHA256 == nil || *a.SHA256 != f.Input.SHA256 || a.ObjectKey != f.Input.ObjectKey || a.ByteSize != f.Input.ByteSize || a.MimeType != f.Input.MIMEType {
		return application.ErrConflict
	}
	return nil
}
func depthCommandLock(tx *gorm.DB, actor, key uuid.UUID) error {
	if actor == uuid.Nil || key == uuid.Nil {
		return application.ErrInvalidDepthInput
	}
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "media-depth-command/"+actor.String()+"/"+key.String()).Error
}
func depthReplay(tx *gorm.DB, actor, key uuid.UUID, hash string) (domain.DepthJob, bool, error) {
	var r struct {
		RequestHash string
		Response    []byte
	}
	res := tx.Raw(`SELECT request_hash,response FROM mediatool.depth_command WHERE actor_id=? AND request_id=?`, actor, key).Scan(&r)
	if res.Error != nil {
		return domain.DepthJob{}, false, res.Error
	}
	if res.RowsAffected == 0 {
		return domain.DepthJob{}, false, nil
	}
	if r.RequestHash != hash {
		return domain.DepthJob{}, false, application.ErrConflict
	}
	var j domain.DepthJob
	err := depthJSON(r.Response, &j)
	return j, true, err
}
func depthQuota(tx *gorm.DB, project uuid.UUID) error {
	if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "media-depth-project/"+project.String()).Error; err != nil {
		return err
	}
	var n int
	if err := tx.Raw(`SELECT count(*) FROM mediatool.depth_job WHERE project_id=? AND (status IN ('queued','running','review_required','cancel_requested') OR needs_reconciliation OR execution_unconfirmed)`, project).Scan(&n).Error; err != nil {
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
		job = domain.DepthJob{ID: uuid.New(), ProjectID: project, Source: domain.Source{CanvasID: in.CanvasID, NodeID: in.NodeID, Revision: in.Revision}, SourceAssetID: a.ID, SourceAssetRevision: a.Revision, SourceSHA256: *a.SHA256, ProfileID: domain.DepthProfileID, Status: domain.DepthQueued, Stage: "queued", Attempt: 1, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if err := tx.Exec(`INSERT INTO mediatool.depth_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,source_asset_id,source_asset_revision,source_sha256,profile_id,frozen,frozen_sha256,status,stage,attempt,revision,process_state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?::jsonb,?,'queued','queued',1,1,'none',?,?)`, job.ID, project, actor.OrgID, actor.ID, string(actor.Role), in.CanvasID, in.NodeID, in.Revision, a.ID, a.Revision, *a.SHA256, domain.DepthProfileID, string(body), hex.EncodeToString(sha[:]), now, now).Error; err != nil {
			return err
		}
		return recordDepthCommand(tx, actor, key, hash, "requested", job, "start")
	})
	return job, err
}

var _ application.DepthStore = (*DepthStore)(nil)
