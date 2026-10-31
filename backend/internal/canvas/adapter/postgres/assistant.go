package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// AssistantStore uses the canvas project's transaction-bound authority.
type AssistantStore struct {
	projectAuthority
	db *gorm.DB
}

// NewAssistantStore injects storage and transaction-scoped project authority.
func NewAssistantStore(db *gorm.DB, project ProjectAccessFactory) *AssistantStore {
	return &AssistantStore{projectAuthority: projectAuthority{project: project}, db: db}
}

type assistantRow struct {
	ID, CanvasID, ProjectID uuid.UUID
	Title                   string
	Revision                int64
	Turns                   []byte
	UpdatedAt               time.Time
}

func (r assistantRow) session() (application.AssistantSession, error) {
	out := application.AssistantSession{ID: r.ID, CanvasID: r.CanvasID, ProjectID: r.ProjectID, Title: r.Title, Revision: r.Revision, UpdatedAt: r.UpdatedAt, Turns: []application.AssistantTurn{}}
	err := json.Unmarshal(r.Turns, &out.Turns)
	return out, err
}
func (s *AssistantStore) access(tx *gorm.DB, a identity.Principal, canvas uuid.UUID, write bool) (uuid.UUID, error) {
	p, err := documentProject(tx, canvas)
	if err != nil {
		return uuid.Nil, err
	}
	if err = s.authorize(tx, a, p, write); err != nil {
		return uuid.Nil, err
	}
	var count int64
	// Keep deletion and schema changes outside an in-flight write.
	if err = tx.Raw(`SELECT count(*) FROM (SELECT id FROM canvas.document WHERE id=? AND NOT is_delete AND schema_version=1 FOR SHARE) d`, canvas).Scan(&count).Error; err != nil {
		return uuid.Nil, err
	}
	if count != 1 {
		return uuid.Nil, application.ErrNotFound
	}
	return p, nil
}
func readAssistant(tx *gorm.DB, a identity.Principal, canvas, id uuid.UUID, lock bool) (application.AssistantSession, error) {
	query := `SELECT id,canvas_id,project_id,title,revision,turns,updated_at FROM canvas.assistant_session WHERE id=? AND canvas_id=? AND actor_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	var row assistantRow
	r := tx.Raw(query, id, canvas, a.ID).Scan(&row)
	if r.Error != nil {
		return application.AssistantSession{}, r.Error
	}
	if r.RowsAffected != 1 {
		return application.AssistantSession{}, application.ErrNotFound
	}
	return row.session()
}

// ListAssistant returns only the authenticated caller's canvas conversations.
func (s *AssistantStore) ListAssistant(ctx context.Context, a identity.Principal, canvas uuid.UUID) ([]application.AssistantSession, error) {
	out := []application.AssistantSession{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.access(tx, a, canvas, false); err != nil {
			return err
		}
		var rows []assistantRow
		if err := tx.Raw(`SELECT id,canvas_id,project_id,title,revision,turns,updated_at FROM canvas.assistant_session WHERE canvas_id=? AND actor_id=? ORDER BY updated_at DESC,id LIMIT 50`, canvas, a.ID).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			item, err := row.session()
			if err != nil {
				return err
			}
			if err := resolveAssistantOperation(tx, a, &item); err != nil {
				return err
			}
			out = append(out, item)
		}
		return nil
	})
	return out, err
}

// ReadAssistant also resolves the latest durable submission of the current turn.
func (s *AssistantStore) ReadAssistant(ctx context.Context, a identity.Principal, canvas, id uuid.UUID) (application.AssistantSession, error) {
	var out application.AssistantSession
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.access(tx, a, canvas, false); err != nil {
			return err
		}
		var err error
		out, err = readAssistant(tx, a, canvas, id, false)
		if err == nil {
			err = resolveAssistantOperation(tx, a, &out)
		}
		return err
	})
	return out, err
}

// resolveAssistantOperation reads the durable receipt, including a cleared slot.
func resolveAssistantOperation(tx *gorm.DB, a identity.Principal, out *application.AssistantSession) error {
	if len(out.Turns) == 0 {
		return nil
	}
	last := &out.Turns[len(out.Turns)-1]
	var operation struct{ ID uuid.UUID }
	if err := tx.Raw(`SELECT o.id FROM operation.prompt_submission sub JOIN operation.operation o ON o.id=sub.operation_id AND o.project_id=sub.project_id WHERE sub.actor_id=? AND sub.project_id=? AND sub.owner_key=? AND (sub.state='accepted' OR sub.resolution='acknowledge_result') AND NOT o.is_delete ORDER BY sub.epoch DESC LIMIT 1`, a.ID, out.ProjectID, "draft/"+last.DraftID.String()).Scan(&operation).Error; err != nil {
		return err
	}
	if operation.ID != uuid.Nil {
		last.OperationID = &operation.ID
	}
	return nil
}

// CreateAssistant serializes bounded session creation with replay by identity.
func (s *AssistantStore) CreateAssistant(ctx context.Context, a identity.Principal, canvas uuid.UUID, in application.AssistantCreate) (application.AssistantSession, error) {
	var out application.AssistantSession
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := s.access(tx, a, canvas, true)
		if err != nil {
			return err
		}
		if err = tx.Exec(`SELECT pg_advisory_xact_lock(69361,hashtext(?))`, a.ID.String()+canvas.String()).Error; err != nil {
			return err
		}
		var count int64
		if err = tx.Raw(`SELECT count(*) FROM canvas.assistant_session WHERE canvas_id=? AND actor_id=?`, canvas, a.ID).Scan(&count).Error; err != nil {
			return err
		}
		var existing assistantRow
		r := tx.Raw(`SELECT id,canvas_id,project_id,title,revision,turns,updated_at FROM canvas.assistant_session WHERE id=? AND actor_id=? AND canvas_id=?`, in.ID, a.ID, canvas).Scan(&existing)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 1 {
			if existing.Title != in.Title {
				return application.ErrIdempotencyConflict
			}
			out, err = existing.session()
			return err
		}
		var empty bool
		if err := tx.Raw(`SELECT EXISTS (
			SELECT 1 FROM canvas.assistant_session s
			WHERE s.canvas_id=? AND s.actor_id=? AND NOT EXISTS (
				SELECT 1 FROM jsonb_array_elements(s.turns) turn
				JOIN operation.prompt_submission sub ON sub.owner_key='draft/' || (turn->>'draft_id')
				AND sub.actor_id=s.actor_id AND sub.project_id=s.project_id
				JOIN operation.operation o ON o.id=sub.operation_id AND o.project_id=sub.project_id
				WHERE (sub.state='accepted' OR sub.resolution='acknowledge_result') AND NOT o.is_delete
			)
		)`, canvas, a.ID).Scan(&empty).Error; err != nil {
			return err
		}
		if empty {
			return application.ErrAssistantEmptySession
		}
		if count >= 50 {
			return domain.ErrInvalidCommand
		}
		if err = tx.Exec(`INSERT INTO canvas.assistant_session(id,canvas_id,project_id,actor_id,title) VALUES(?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, in.ID, canvas, p, a.ID, in.Title).Error; err != nil {
			return err
		}
		out, err = readAssistant(tx, a, canvas, in.ID, false)
		return err
	})
	return out, err
}

// AppendAssistant freezes a terminal execution before binding the next input.
func (s *AssistantStore) AppendAssistant(ctx context.Context, a identity.Principal, canvas, id uuid.UUID, in application.AssistantAppend) (application.AssistantSession, error) {
	var out application.AssistantSession
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := s.access(tx, a, canvas, true)
		if err != nil {
			return err
		}
		out, err = readAssistant(tx, a, canvas, id, true)
		if err != nil {
			return err
		}
		for index, turn := range out.Turns {
			if turn.DraftID == in.DraftID {
				execution := turn.Execution
				if execution == "" {
					execution = "discussion"
				}
				if turn.Skill != in.Skill || execution != in.Execution || (index == 0 && in.PreviousOperationID != nil) || (index > 0 && (in.PreviousOperationID == nil || out.Turns[index-1].OperationID == nil || *in.PreviousOperationID != *out.Turns[index-1].OperationID)) {
					return application.ErrIdempotencyConflict
				}
				return nil
			}
		}
		if out.Revision != in.ExpectedRevision {
			return &application.RevisionConflict{CurrentRevision: out.Revision}
		}
		if len(out.Turns) >= 64 {
			return domain.ErrInvalidCommand
		}
		if len(out.Turns) == 0 && in.PreviousOperationID != nil {
			return domain.ErrInvalidCommand
		}
		if err = tx.Exec(`SELECT pg_advisory_xact_lock(69362,hashtext(?))`, in.DraftID.String()).Error; err != nil {
			return err
		}
		var count int64
		if err = tx.Raw(`SELECT count(*) FROM (SELECT id FROM workspace.prompt_draft WHERE id=? AND actor_id=? AND project_id=? FOR SHARE) d`, in.DraftID, a.ID, p).Scan(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return application.ErrNotFound
		}
		// A draft cannot become a mutable input shared by two conversations.
		if err = tx.Raw(`SELECT count(*) FROM canvas.assistant_session WHERE actor_id=? AND turns @> ?::jsonb`, a.ID, `[{"draft_id":"`+in.DraftID.String()+`"}]`).Scan(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return application.ErrIdempotencyConflict
		}
		if len(out.Turns) > 0 {
			last := &out.Turns[len(out.Turns)-1]
			var operation struct {
				ID     uuid.UUID
				Status string
			}
			r := tx.Raw(`SELECT o.id,o.status FROM operation.prompt_submission sub JOIN operation.operation o ON o.id=sub.operation_id AND o.project_id=sub.project_id WHERE sub.actor_id=? AND sub.project_id=? AND sub.owner_key=? AND NOT o.is_delete ORDER BY sub.epoch DESC LIMIT 1 FOR SHARE OF sub,o`, a.ID, p, "draft/"+last.DraftID.String()).Scan(&operation)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 || !assistantTerminal(operation.Status) {
				return application.ErrAssistantTurnActive
			}
			if in.PreviousOperationID == nil || operation.ID != *in.PreviousOperationID {
				return &application.RevisionConflict{CurrentRevision: out.Revision}
			}
			last.OperationID = &operation.ID
		}
		out.Turns = append(out.Turns, application.AssistantTurn{DraftID: in.DraftID, Skill: in.Skill, Execution: in.Execution})
		body, err := json.Marshal(out.Turns)
		if err != nil {
			return err
		}
		if err = tx.Exec(`UPDATE canvas.assistant_session SET turns=?::jsonb,revision=revision+1,updated_at=now() WHERE id=?`, string(body), id).Error; err != nil {
			return err
		}
		out, err = readAssistant(tx, a, canvas, id, false)
		return err
	})
	return out, err
}
func assistantTerminal(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "expired":
		return true
	default:
		return false
	}
}
