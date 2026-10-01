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

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
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
}

// NewStore injects the database and two owning-context ports explicitly.
func NewStore(db *gorm.DB, timeline TimelineFactory, media MediaFactory) *Store {
	return &Store{db: db, timeline: timeline, media: media}
}

type jobRow struct {
	ID, ProjectID, OrgID, ActorID, CanvasID, NodeID uuid.UUID
	ActorRole                                       string
	SourceRevision                                  int64
	Frozen                                          []byte
	Status, Stage                                   string
	Progress, Attempt                               int
	Revision                                        int64
	ActiveWorker                                    *uuid.UUID
	AssetID                                         *uuid.UUID
	SHA256, FailureCode                             *string
	CreatedAt, UpdatedAt                            time.Time
}

func (r jobRow) public() domain.ExportJob {
	return domain.ExportJob{ID: r.ID, ProjectID: r.ProjectID, Source: domain.Source{CanvasID: r.CanvasID, NodeID: r.NodeID, Revision: r.SourceRevision}, Status: domain.Status(r.Status), Stage: r.Stage, Progress: r.Progress, Attempt: r.Attempt, Revision: r.Revision, AssetID: r.AssetID, SHA256: r.SHA256, FailureCode: r.FailureCode, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func (r jobRow) actor() identityapp.Principal {
	return identityapp.Principal{ID: r.ActorID, OrgID: r.OrgID, Role: identitydomain.Role(r.ActorRole)}
}
func readJob(tx *gorm.DB, project, id uuid.UUID, lock bool) (jobRow, error) {
	var row jobRow
	query := `SELECT * FROM mediatool.export_job WHERE id=?`
	args := []any{id}
	if project != uuid.Nil {
		query += ` AND project_id=?`
		args = append(args, project)
	}
	if lock {
		query += ` FOR UPDATE`
	}
	r := tx.Raw(query, args...).Scan(&row)
	if r.Error != nil {
		return row, fmt.Errorf("read media export: %w", r.Error)
	}
	if r.RowsAffected != 1 {
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
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "media-export-command/"+actor.String()+"/"+key.String()).Error
}
func commandReplay(tx *gorm.DB, actor, key uuid.UUID, hash string) (domain.ExportJob, bool, error) {
	var receipt struct {
		RequestHash string
		Response    []byte
	}
	r := tx.Raw(`SELECT request_hash,response FROM mediatool.export_command WHERE actor_id=? AND request_id=?`, actor, key).Scan(&receipt)
	if r.Error != nil {
		return domain.ExportJob{}, false, r.Error
	}
	if r.RowsAffected == 0 {
		return domain.ExportJob{}, false, nil
	}
	if receipt.RequestHash != hash {
		return domain.ExportJob{}, false, application.ErrConflict
	}
	var job domain.ExportJob
	if err := json.Unmarshal(receipt.Response, &job); err != nil {
		return job, false, err
	}
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
		const topic = "lanverse.mediatool.export_command.v1"
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
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, id, topic, actor.OrgID.String(), string(payload)).Error; err != nil {
			return err
		}
	}
	if err := tx.Exec(`INSERT INTO mediatool.export_command(actor_id,request_id,job_id,request_hash,response,event_id,event_action,event_attempt,created_at) VALUES(?,?,?,?,?::jsonb,?,?,?,?)`, actor.ID, key, job.ID, hash, string(response), eventID, eventAction, eventAttempt, now).Error; err != nil {
		return err
	}
	auditBody, err := json.Marshal(struct {
		ID             uuid.UUID     `json:"id"`
		ProjectID      uuid.UUID     `json:"project_id"`
		CanvasID       uuid.UUID     `json:"canvas_id"`
		NodeID         uuid.UUID     `json:"node_id"`
		SourceRevision int64         `json:"source_revision"`
		Status         domain.Status `json:"status"`
		Stage          string        `json:"stage"`
		Progress       int           `json:"progress"`
		Attempt        int           `json:"attempt"`
		Revision       int64         `json:"revision"`
		AssetID        *uuid.UUID    `json:"asset_id"`
		SHA256         *string       `json:"sha256"`
		FailureCode    *string       `json:"failure_code"`
		CreatedAt      string        `json:"created_at"`
		UpdatedAt      string        `json:"updated_at"`
	}{job.ID, job.ProjectID, job.Source.CanvasID, job.Source.NodeID, job.Source.Revision, job.Status, job.Stage, job.Progress, job.Attempt, job.Revision, job.AssetID, job.SHA256, job.FailureCode, job.CreatedAt.Format(time.RFC3339Nano), job.UpdatedAt.Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	auditID := uuid.NewSHA1(key, []byte("media-export-audit/"+actor.ID.String()))
	const auditTopic = "lanverse.audit.recorded.v1"
	auditEvent, err := json.Marshal(map[string]any{"event_id": auditID, "event_type": auditTopic, "occurred_at": now, "org_id": actor.OrgID, "project_id": job.ProjectID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": auditID}, "data": map[string]any{"action": "media.export_" + action, "object": map[string]any{"type": "media_export", "id": job.ID.String()}, "before": nil, "after": json.RawMessage(auditBody), "request_id": key.String()}})
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, auditID, auditTopic, job.ProjectID.String(), string(auditEvent)).Error
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
	hash, err := commandHash("create", project, uuid.Nil, input)
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
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "media-export-project/"+project.String()).Error; err != nil {
			return err
		}
		var active int64
		if err := tx.Raw(`SELECT count(*) FROM mediatool.export_job WHERE project_id=? AND status IN ('queued','running','cancel_requested')`, project).Scan(&active).Error; err != nil {
			return err
		}
		if active >= 2 {
			return application.ErrConflict
		}
		timeline, err := s.timeline(tx).FreezeTimeline(ctx, actor, project, input.CanvasID, input.NodeID, input.Revision)
		if err != nil {
			return normalize(err)
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
		raw, err := json.Marshal(frozen)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		job = domain.ExportJob{ID: uuid.New(), ProjectID: project, Source: domain.Source{CanvasID: input.CanvasID, NodeID: input.NodeID, Revision: input.Revision}, Status: domain.Queued, Stage: "queued", Attempt: 1, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if err := tx.Exec(`INSERT INTO mediatool.export_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,frozen,status,stage,progress,attempt,revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?::jsonb,'queued','queued',0,1,1,?,?)`, job.ID, project, actor.OrgID, actor.ID, string(actor.Role), input.CanvasID, input.NodeID, input.Revision, string(raw), now, now).Error; err != nil {
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

// List uses an ID keyset and never places private frozen media keys in responses.
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
		q := `SELECT * FROM mediatool.export_job WHERE project_id=?`
		args := []any{input.ProjectID}
		for _, f := range []struct {
			col string
			id  uuid.UUID
		}{{"canvas_id", input.CanvasID}, {"node_id", input.NodeID}, {"id", input.After}} {
			if f.id != uuid.Nil {
				op := "="
				if f.col == "id" {
					op = "<"
				}
				q += " AND " + f.col + op + "?"
				args = append(args, f.id)
			}
		}
		q += ` ORDER BY id DESC LIMIT ?`
		args = append(args, input.Limit)
		return tx.Raw(q, args...).Scan(&rows).Error
	})
	jobs := make([]domain.ExportJob, 0, len(rows))
	for _, row := range rows {
		jobs = append(jobs, row.public())
	}
	return jobs, err
}
