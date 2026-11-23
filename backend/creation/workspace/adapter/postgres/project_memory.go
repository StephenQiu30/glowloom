package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

const memoryHeads = `SELECT DISTINCT ON (key) id,project_id,key,revision,statement,withdrawn,confirmed_by,confirmed_at
	FROM workspace.project_memory_revision WHERE project_id=? ORDER BY key,revision DESC`

// ProjectMemoryVersion identifies the ordered active revision set by its immutable IDs.
func ProjectMemoryVersion(items []application.ProjectMemory) string {
	hash := sha256.New()
	for _, item := range items {
		_, _ = hash.Write(item.ID[:])
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// ReadActiveProjectMemories reads only the heads of an already-authorized project.
// The caller holds the project lock through this read.
func ReadActiveProjectMemories(tx *gorm.DB, project uuid.UUID) ([]application.ProjectMemory, error) {
	items := make([]application.ProjectMemory, 0)
	err := tx.Raw(`SELECT * FROM (`+memoryHeads+`) heads WHERE NOT withdrawn ORDER BY key`, project).Scan(&items).Error
	return items, err
}

func (s *Store) readMemory(ctx context.Context, actor identity.Principal, project uuid.UUID, fn func(*gorm.DB) error) error {
	if s == nil || s.db == nil || s.scopes == nil {
		return source.ErrProjectDependencyUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access := NewScopedProjectContentAccessStore(tx, s.scopes(tx))
		if _, err := access.Authorize(ctx, actor, project, false); err != nil {
			return err
		}
		return fn(tx)
	})
}

// ListProjectMemories returns active decisions even for a read-only project member.
func (s *Store) ListProjectMemories(ctx context.Context, actor identity.Principal, project uuid.UUID) (application.ProjectMemoryView, error) {
	var view application.ProjectMemoryView
	err := s.readMemory(ctx, actor, project, func(tx *gorm.DB) error {
		var err error
		view.Items, err = ReadActiveProjectMemories(tx, project)
		if err != nil {
			return err
		}
		// Check edit membership without upgrading the existing shared row lock.
		_, err = s.scopes(tx).Resolve(ctx, actor, actor.ScopeID, true)
		if errors.Is(err, identity.ErrForbidden) || errors.Is(err, source.ErrScopeNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return tx.Raw(`SELECT status='active' FROM workspace.project WHERE id=?`, project).Scan(&view.CanEdit).Error
	})
	return view, err
}

// ProjectMemoryHistory pages immutable versions without changing the active value.
func (s *Store) ProjectMemoryHistory(ctx context.Context, actor identity.Principal, project uuid.UUID, key string, before int64) ([]application.ProjectMemory, error) {
	items := make([]application.ProjectMemory, 0)
	err := s.readMemory(ctx, actor, project, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT id,project_id,key,revision,statement,withdrawn,confirmed_by,confirmed_at
			FROM workspace.project_memory_revision WHERE project_id=? AND key=? AND (?=0 OR revision<?)
			ORDER BY revision DESC LIMIT 30`, project, key, before, before).Scan(&items).Error
	})
	return items, err
}

// ChangeProjectMemory appends one revision and its durable idempotency receipt.
func (s *Store) ChangeProjectMemory(ctx context.Context, actor identity.Principal, project uuid.UUID, key string, request uuid.UUID, input application.ProjectMemoryChange, now time.Time) (application.ProjectMemory, error) {
	var result application.ProjectMemory
	if input.ExpectedRevision == nil {
		return result, application.ErrInvalidProjectMemory
	}
	if s == nil || s.db == nil || s.scopes == nil {
		return result, source.ErrProjectDependencyUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access := NewScopedProjectContentAccessStore(tx, s.scopes(tx))
		if _, err := access.Authorize(ctx, actor, project, true); err != nil {
			return err
		}
		read := tx.Raw(`SELECT id,project_id,key,revision,statement,withdrawn,confirmed_by,confirmed_at
			FROM workspace.project_memory_revision WHERE project_id=? AND confirmed_by=? AND request_id=?`, project, actor.ID, request).Scan(&result)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 0 {
			if result.Key != key || result.Revision-1 != *input.ExpectedRevision || result.Statement != input.Statement || result.Withdrawn != input.Withdraw {
				return source.ErrIdempotencyConflict
			}
			return nil
		}

		var current application.ProjectMemory
		if err := tx.Raw(`SELECT revision,withdrawn FROM workspace.project_memory_revision WHERE project_id=? AND key=?
			ORDER BY revision DESC LIMIT 1`, project, key).Scan(&current).Error; err != nil {
			return err
		}
		if current.Revision != *input.ExpectedRevision || (input.Withdraw && (current.Revision == 0 || current.Withdrawn)) {
			return domain.ErrProjectRevisionConflict
		}
		if !input.Withdraw && (current.Revision == 0 || current.Withdrawn) {
			var count int
			if err := tx.Raw(`SELECT count(*) FROM (`+memoryHeads+`) heads WHERE NOT withdrawn`, project).Scan(&count).Error; err != nil {
				return err
			}
			if count >= application.MaxActiveProjectMemories {
				return application.ErrProjectMemoryLimit
			}
		}
		return tx.Raw(`INSERT INTO workspace.project_memory_revision
			(id,project_id,key,revision,statement,withdrawn,confirmed_by,confirmed_at,request_id)
			VALUES (?,?,?,?,?,?,?,?,?) RETURNING id,project_id,key,revision,statement,withdrawn,confirmed_by,confirmed_at`,
			uuid.New(), project, key, current.Revision+1, input.Statement, input.Withdraw, actor.ID, now, request).Scan(&result).Error
	})
	if err != nil {
		return application.ProjectMemory{}, fmt.Errorf("change project memory: %w", err)
	}
	return result, nil
}
