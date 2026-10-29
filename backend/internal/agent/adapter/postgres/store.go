// Package postgres persists account-private Agent conversations.
package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	agentapp "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	op "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// AccessFactory binds project authorization to the session transaction.
type AccessFactory func(*gorm.DB) op.ProjectAccess

// Store persists private conversations under project and actor authority.
type Store struct {
	operations OperationsFactory
	db         *gorm.DB
	access     AccessFactory
}

// NewStore constructs session persistence with explicit dependencies.
func NewStore(db *gorm.DB, access AccessFactory, operations OperationsFactory) *Store {
	return &Store{db: db, access: access, operations: operations}
}

func (s *Store) authorize(tx *gorm.DB, actor identity.Principal, pid uuid.UUID, write bool) error {
	if s.access == nil || actor.ID == uuid.Nil || pid == uuid.Nil {
		return identity.ErrForbidden
	}
	return s.access(tx).AuthorizeOperation(tx.Statement.Context, actor, pid, write)
}

type sessionRow struct {
	ID, ProjectID, ActorID uuid.UUID
	Title                  string
	Revision               int64
	UpdatedAt              time.Time
	Turns                  []byte
}

func (r sessionRow) decode() (domain.Session, error) {
	s := domain.Session{ID: r.ID, ProjectID: r.ProjectID, ActorID: r.ActorID, Title: r.Title, Revision: r.Revision, UpdatedAt: r.UpdatedAt, Turns: []domain.Turn{}}
	if json.Unmarshal(r.Turns, &s.Turns) != nil || len(s.Turns) > 20 {
		return s, domain.ErrInvalid
	}
	return s, nil
}

// Create replays an identical creation request for the same actor.
func (s *Store) Create(ctx context.Context, actor identity.Principal, pid, key uuid.UUID, title string) (domain.Session, error) {
	var out domain.Session
	if key == uuid.Nil || title == "" || len(title) > 240 {
		return out, domain.ErrInvalid
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(tx, actor, pid, true); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO agent.session(id,project_id,actor_id,request_id,title) VALUES(?,?,?,?,?) ON CONFLICT(project_id,actor_id,request_id) DO NOTHING`, uuid.New(), pid, actor.ID, key, title).Error; err != nil {
			return err
		}
		var row sessionRow
		if err := tx.Raw(`SELECT id,project_id,actor_id,title,revision,updated_at,turns FROM agent.session WHERE project_id=? AND actor_id=? AND request_id=?`, pid, actor.ID, key).Scan(&row).Error; err != nil {
			return err
		}
		if row.Title != title {
			return domain.ErrConflict
		}
		var err error
		out, err = row.decode()
		return err
	})
	return out, err
}

// List returns recent conversation metadata for the initiating actor.
func (s *Store) List(ctx context.Context, actor identity.Principal, pid uuid.UUID) ([]domain.Session, error) {
	out := []domain.Session{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(tx, actor, pid, false); err != nil {
			return err
		}
		var rows []sessionRow
		if err := tx.Raw(`SELECT id,project_id,actor_id,title,revision,updated_at FROM agent.session WHERE project_id=? AND actor_id=? ORDER BY updated_at DESC,id LIMIT 50`, pid, actor.ID).Scan(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			out = append(out, domain.Session{ID: r.ID, ProjectID: r.ProjectID, ActorID: r.ActorID, Title: r.Title, Revision: r.Revision, UpdatedAt: r.UpdatedAt, Turns: []domain.Turn{}})
		}
		return nil
	})
	return out, err
}

// Read returns one private conversation under current project authority.
func (s *Store) Read(ctx context.Context, actor identity.Principal, pid, sid uuid.UUID) (domain.Session, error) {
	return s.withSession(ctx, actor, pid, sid, false, nil)
}

// Mutate serializes a session transition and advances its revision.
func (s *Store) Mutate(ctx context.Context, actor identity.Principal, pid, sid uuid.UUID, fn func(*domain.Session, agentapp.Operations) error) (domain.Session, error) {
	return s.withSession(ctx, actor, pid, sid, true, fn)
}

func (s *Store) withSession(ctx context.Context, actor identity.Principal, pid, sid uuid.UUID, write bool, fn func(*domain.Session, agentapp.Operations) error) (domain.Session, error) {
	var out domain.Session
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(tx, actor, pid, write); err != nil {
			return err
		}
		query := `SELECT id,project_id,actor_id,title,revision,updated_at,turns FROM agent.session WHERE id=? AND project_id=? AND actor_id=?`
		if write {
			query += " FOR UPDATE"
		}
		var row sessionRow
		r := tx.Raw(query, sid, pid, actor.ID).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return domain.ErrNotFound
		}
		var err error
		out, err = row.decode()
		if err != nil {
			return err
		}
		if !write {
			return nil
		}
		before, _ := json.Marshal(out.Turns)
		if s.operations == nil {
			return identity.ErrForbidden
		}
		if err := fn(&out, s.operations(tx)); err != nil {
			return err
		}
		after, err := json.Marshal(out.Turns)
		if err != nil || len(after) > 1048576 || len(out.Turns) > 20 {
			return domain.ErrInvalid
		}
		if bytes.Equal(before, after) {
			return nil
		}
		out.Revision++
		out.UpdatedAt = time.Now().UTC()
		return tx.Exec(`UPDATE agent.session SET turns=?::jsonb,revision=?,updated_at=? WHERE id=? AND project_id=? AND actor_id=?`, string(after), out.Revision, out.UpdatedAt, sid, pid, actor.ID).Error
	})
	return out, err
}

// OperationsFactory binds operation effects to the session transaction.
type OperationsFactory func(*gorm.DB) agentapp.Operations
