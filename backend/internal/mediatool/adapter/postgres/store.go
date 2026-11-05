// Package postgres persists local export facts and consumes transaction-bound owners.
package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	task "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// TimelineFactory binds the source owner to the job command's database transaction.
type TimelineFactory func(*gorm.DB) application.TimelineReader

// MediaFactory binds metadata and review to the same database transaction.
type MediaFactory func(*gorm.DB) application.DerivedMedia

// Store never reads or writes another bounded context's business tables.
type Store struct {
	db       *gorm.DB
	timeline TimelineFactory
	media    MediaFactory
	storage  StorageFactory
}

// NewStore injects the database and two owning-context ports explicitly.
func NewStore(db *gorm.DB, timeline TimelineFactory, media MediaFactory) *Store {
	return &Store{db: db, timeline: timeline, media: media}
}

type jobRow struct {
	ID                                          uuid.UUID `gorm:"primaryKey"`
	ProjectID, OrgID, ActorID, CanvasID, NodeID uuid.UUID
	ActorRole                                   *string
	OutputKind                                  string
	SourceRevision                              int64
	Frozen                                      jsonDocument
	Status                                      domain.Status
	Stage                                       domain.ExportStage
	Progress, Attempt                           int
	Revision                                    int64
	ActiveWorker                                *uuid.UUID
	AssetID                                     *uuid.UUID
	ArtifactID                                  *uuid.UUID
	SHA256, FailureCode                         *string
	CreatedAt, UpdatedAt                        time.Time
}

func (jobRow) TableName() string { return "mediatool.export_job" }

func (r jobRow) public() domain.ExportJob {
	var frozen domain.FrozenExport
	_ = json.Unmarshal(r.Frozen, &frozen)
	return domain.ExportJob{Settings: frozen.Settings, ID: r.ID, ProjectID: r.ProjectID, OutputKind: domain.OutputKind(r.OutputKind).Effective(), Source: domain.Source{CanvasID: r.CanvasID, NodeID: r.NodeID, Revision: r.SourceRevision}, Status: r.Status, Stage: r.Stage, Progress: r.Progress, Attempt: r.Attempt, Revision: r.Revision, AssetID: r.AssetID, ArtifactID: r.ArtifactID, SHA256: r.SHA256, FailureCode: r.FailureCode, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func (r jobRow) actor() identityapp.Principal {
	return identityapp.Principal{ID: r.ActorID, OrgID: r.OrgID}
}
func readJob(tx *gorm.DB, project, id uuid.UUID, lock bool) (jobRow, error) {
	var row jobRow
	query := tx.Model(&jobRow{}).Where("id = ?", id)
	if project != uuid.Nil {
		query = query.Where("project_id = ?", project)
	}
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, application.ErrNotFound
		}
		return row, fmt.Errorf("read media export: %w", err)
	}
	if row.ID == uuid.Nil {
		return row, application.ErrNotFound
	}
	return row, nil
}
func (s *Store) authorize(ctx context.Context, tx *gorm.DB, actor identityapp.Principal, project uuid.UUID, write bool) error {
	if s.media == nil {
		return application.ErrUnavailable
	}
	return normalize(s.media(tx).AuthorizeProject(ctx, actor, project, write))
}
func normalize(err error) error {
	var revision *canvasapp.RevisionConflict
	switch {
	case err == nil:
		return nil
	case errors.Is(err, mediaapp.ErrNotFound) || errors.Is(err, canvasapp.ErrNotFound):
		return application.ErrNotFound
	case errors.As(err, &revision) || errors.Is(err, workspacedomain.ErrProjectStateConflict):
		return application.ErrConflict
	case errors.Is(err, canvasdomain.ErrInvalidCommand):
		return application.ErrInvalidExport
	default:
		return err
	}
}
func commandHash(action string, project, id uuid.UUID, input any) (string, error) {
	raw, err := json.Marshal(struct {
		Action      string
		Project, ID uuid.UUID
		Input       any
	}{action, project, id, input})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
func commandLock(tx *gorm.DB, actor, key uuid.UUID) error {
	if key == uuid.Nil || actor == uuid.Nil {
		return application.ErrInvalidExport
	}
	return lockMediatoolPrincipal(tx, actor)
}
func commandReplay(tx *gorm.DB, actor, key uuid.UUID, hash string) (domain.ExportJob, bool, error) {
	var receipt exportCommandRecord
	err := tx.Where("actor_id = ? AND request_id = ?", actor, key).Take(&receipt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ExportJob{}, false, nil
	}
	if err != nil {
		return domain.ExportJob{}, false, err
	}
	if receipt.RequestHash != hash {
		return domain.ExportJob{}, false, application.ErrConflict
	}
	var job domain.ExportJob
	if err := json.Unmarshal(receipt.Response, &job); err != nil {
		return job, false, err
	}
	job.OutputKind = job.OutputKind.Effective()
	return job, true, nil
}
func recordCommand(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, hash, action string, job domain.ExportJob, emit string) error {
	now := time.Now().UTC()
	response, err := json.Marshal(job)
	if err != nil {
		return err
	}
	var eventID *uuid.UUID
	var eventAction *string
	var eventAttempt *int
	if emit != "" {
		id := uuid.NewSHA1(key, []byte("media-export/"+actor.ID.String()+"/"+emit))
		eventID = &id
		eventAction = &emit
		eventAttempt = &job.Attempt
		delivery := application.Delivery{WorkID: application.WorkID{JobID: job.ID, Attempt: job.Attempt}, EventID: id, RequestID: key, ActorID: actor.ID, OrgID: actor.OrgID, ProjectID: job.ProjectID, Action: emit}
		const topic = "glowloom.mediatool.export_command.v1"
		payload, err := json.Marshal(struct {
			EventID    uuid.UUID            `json:"event_id"`
			EventType  string               `json:"event_type"`
			OccurredAt time.Time            `json:"occurred_at"`
			OrgID      uuid.UUID            `json:"org_id"`
			Data       application.Delivery `json:"data"`
		}{id, topic, now, actor.OrgID, delivery})
		if err != nil {
			return err
		}
		if emit == "cancel" {
			if err := taskruntime.RequestStop(tx.Statement.Context, tx, task.Export, map[string]any{"job_id": job.ID, "attempt": job.Attempt}); err != nil {
				return err
			}
		}
		if err := taskruntime.EnqueueReference(tx.Statement.Context, tx, task.Export, id.String(), time.Now().UTC(), delivery.WorkID); err != nil {
			return err
		}
		if err := tx.Create(&mediatoolOutboxRecord{ID: id, Topic: topic, PartitionKey: actor.OrgID.String(), Payload: jsonDocument(payload)}).Error; err != nil {
			return err
		}
	}
	if err := tx.Create(&exportCommandRecord{ActorID: actor.ID, RequestID: key, JobID: job.ID, RequestHash: hash, Response: jsonDocument(response), EventID: eventID, EventAction: eventAction, EventAttempt: eventAttempt, CreatedAt: now}).Error; err != nil {
		return err
	}
	auditBody, err := json.Marshal(struct {
		ID             uuid.UUID          `json:"id"`
		ProjectID      uuid.UUID          `json:"project_id"`
		OutputKind     domain.OutputKind  `json:"output_kind"`
		CanvasID       uuid.UUID          `json:"canvas_id"`
		NodeID         uuid.UUID          `json:"node_id"`
		SourceRevision int64              `json:"source_revision"`
		Status         domain.Status      `json:"status"`
		Stage          domain.ExportStage `json:"stage"`
		Progress       int                `json:"progress"`
		Attempt        int                `json:"attempt"`
		Revision       int64              `json:"revision"`
		AssetID        *uuid.UUID         `json:"asset_id"`
		ArtifactID     *uuid.UUID         `json:"artifact_id"`
		SHA256         *string            `json:"sha256"`
		FailureCode    *string            `json:"failure_code"`
		CreatedAt      string             `json:"created_at"`
		UpdatedAt      string             `json:"updated_at"`
	}{job.ID, job.ProjectID, job.OutputKind, job.Source.CanvasID, job.Source.NodeID, job.Source.Revision, job.Status, job.Stage, job.Progress, job.Attempt, job.Revision, job.AssetID, job.ArtifactID, job.SHA256, job.FailureCode, job.CreatedAt.Format(time.RFC3339Nano), job.UpdatedAt.Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	auditID := uuid.NewSHA1(key, []byte("media-export-audit/"+actor.ID.String()))
	const auditTopic = "glowloom.audit.recorded.v1"
	auditEvent, err := json.Marshal(map[string]any{"event_id": auditID, "event_type": auditTopic, "occurred_at": now, "org_id": actor.OrgID, "project_id": job.ProjectID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": auditID}, "data": map[string]any{"action": "media.export_" + action, "object": map[string]any{"type": "media_export", "id": job.ID.String()}, "before": nil, "after": json.RawMessage(auditBody), "request_id": key.String()}})
	if err != nil {
		return err
	}
	return tx.Create(&mediatoolOutboxRecord{ID: auditID, Topic: auditTopic, PartitionKey: job.ProjectID.String(), Payload: jsonDocument(auditEvent)}).Error
}

// Create atomically freezes the saved canvas, original metadata, receipt and outbox.
func (s *Store) Create(ctx context.Context, actor identityapp.Principal, project, key uuid.UUID, input application.CreateInput) (domain.ExportJob, error) {
	var job domain.ExportJob
	if s == nil || s.db == nil || s.timeline == nil {
		return job, application.ErrUnavailable
	}
	if project == uuid.Nil || input.CanvasID == uuid.Nil || input.NodeID == uuid.Nil || input.Revision < 1 {
		return job, application.ErrInvalidExport
	}
	kind := input.OutputKind.Effective()
	if !kind.Valid() {
		return job, application.ErrInvalidExport
	}
	if input.Settings != nil {
		settings, err := application.NormalizeExportSettings(*input.Settings)
		if err != nil {
			return job, err
		}
		input.Settings = &settings
	}
	canonical := input
	canonical.OutputKind = kind
	if kind == domain.OutputVideo {
		// Preserve the original default-video request fingerprint, including
		// durable command receipts created before audio extraction existed.
		canonical.OutputKind = ""
	}
	hash, err := commandHash("create", project, uuid.Nil, canonical)
	if err != nil {
		return job, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, true); err != nil {
			return err
		}
		if err := commandLock(tx, actor.ID, key); err != nil {
			return err
		}
		replay, found, err := commandReplay(tx, actor.ID, key, hash)
		if err != nil {
			return err
		}
		if found {
			job = replay
			return nil
		}
		if err := lockMediatoolProject(tx, project); err != nil {
			return err
		}
		var active int64
		if err := tx.Model(&jobRow{}).Where("project_id = ? AND status IN ?", project, []domain.Status{domain.Queued, domain.Running, domain.CancelRequested}).Count(&active).Error; err != nil {
			return err
		}
		if active >= 2 {
			return application.ErrConflict
		}
		timeline, err := s.timeline(tx).FreezeTimeline(ctx, actor, project, input.CanvasID, input.NodeID, input.Revision)
		if err != nil {
			return normalize(err)
		}
		if kind == domain.OutputVideo && input.Settings == nil {
			if _, _, err := timeline.VideoDimensions(); err != nil {
				return fmt.Errorf("%w: %w", application.ErrInvalidExport, err)
			}
		}
		ids, err := application.SourceIDs(timeline)
		if err != nil {
			return err
		}
		assets, err := s.media(tx).Sources(ctx, actor, project, ids)
		if err != nil {
			return normalize(err)
		}
		frozen, err := application.FreezeInputs(timeline, assets)
		if err != nil {
			return err
		}
		jobID := uuid.New()
		if kind == domain.OutputJianying {
			frozen.ExportID = &jobID
		}
		frozen.OutputKind = kind
		if input.Settings != nil {
			if err := application.ApplyExportSettings(&frozen, *input.Settings); err != nil {
				return err
			}
		}
		if kind.IsArtifact() {
			frozen.Source = &domain.Source{CanvasID: input.CanvasID, NodeID: input.NodeID, Revision: input.Revision}
			if err := application.ValidateProjectExport(frozen); err != nil {
				return err
			}
		}
		if kind == domain.OutputAudio && !application.HasAudibleClip(timeline) {
			return application.ErrNoAudio
		}
		raw, err := json.Marshal(frozen)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		job = domain.ExportJob{Settings: frozen.Settings, ID: jobID, ProjectID: project, OutputKind: kind, Source: domain.Source{CanvasID: input.CanvasID, NodeID: input.NodeID, Revision: input.Revision}, Status: domain.Queued, Stage: domain.ExportStageQueued, Attempt: 1, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&jobRow{ID: job.ID, ProjectID: project, OrgID: actor.OrgID, ActorID: actor.ID, CanvasID: input.CanvasID, NodeID: input.NodeID, OutputKind: string(kind), SourceRevision: input.Revision, Frozen: jsonDocument(raw), Status: domain.Queued, Stage: domain.ExportStageQueued, Progress: 0, Attempt: 1, Revision: 1, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			return err
		}
		if err := pinTool(ctx, tx, s.storage, mediaapp.StorageWorkKindExport, project, job.ID, job.Attempt); err != nil {
			return err
		}
		return recordCommand(tx, actor, key, hash, "requested", job, "start")
	})
	return job, err
}

// Get returns only currently scoped durable facts, including archived-project reads.
func (s *Store) Get(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (domain.ExportJob, error) {
	var job domain.ExportJob
	if s == nil || s.db == nil {
		return job, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, project, false); err != nil {
			return err
		}
		row, err := readJob(tx, project, id, false)
		if err == nil {
			job = row.public()
		}
		return err
	})
	return job, err
}

// List restores newest creation first with a stable time/ID keyset; private inputs stay private.
func (s *Store) List(ctx context.Context, actor identityapp.Principal, input application.ListInput) ([]domain.ExportJob, error) {
	if s == nil || s.db == nil {
		return nil, application.ErrUnavailable
	}
	if input.ProjectID == uuid.Nil || input.Limit < 1 || input.Limit > 201 {
		return nil, application.ErrInvalidExport
	}
	var rows []jobRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, actor, input.ProjectID, false); err != nil {
			return err
		}
		query := tx.Model(&jobRow{}).Where("project_id = ?", input.ProjectID)
		if input.CanvasID != uuid.Nil {
			query = query.Where("canvas_id = ?", input.CanvasID)
		}
		if input.NodeID != uuid.Nil {
			query = query.Where("node_id = ?", input.NodeID)
		}
		if input.After != uuid.Nil {
			var cursor jobRow
			if err := tx.Model(&jobRow{}).Select("id", "created_at").Where("id = ? AND project_id = ?", input.After, input.ProjectID).Take(&cursor).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			query = query.Where("(created_at, id) < (?, ?)", cursor.CreatedAt, cursor.ID)
		}
		return query.Order("created_at DESC").Order("id DESC").Limit(input.Limit).Find(&rows).Error
	})
	jobs := make([]domain.ExportJob, 0, len(rows))
	for _, row := range rows {
		jobs = append(jobs, row.public())
	}
	return jobs, err
}
