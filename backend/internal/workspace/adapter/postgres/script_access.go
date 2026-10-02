package postgres

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ProjectContentAccessStore binds script access to its already-open transaction.
type ProjectContentAccessStore struct{ tx *gorm.DB }

// NewProjectContentAccessStore accepts the transaction held by the content owner.
// Calls reject a connection pool, which would release locks before content writes.
func NewProjectContentAccessStore(tx *gorm.DB) *ProjectContentAccessStore {
	return &ProjectContentAccessStore{tx: tx}
}

// Authorize checks current account and project facts, retaining its row lock.
func (s *ProjectContentAccessStore) Authorize(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, write bool) (application.ProjectContentAccess, error) {
	if s == nil || s.tx == nil || s.tx.Statement == nil {
		return application.ProjectContentAccess{}, application.ErrProjectDependencyUnavailable
	}
	if _, transaction := s.tx.Statement.ConnPool.(gorm.TxCommitter); !transaction {
		return application.ProjectContentAccess{}, application.ErrProjectDependencyUnavailable
	}
	if err := requireCurrentActor(s.tx.WithContext(ctx), actor); err != nil {
		return application.ProjectContentAccess{}, err
	}
	if projectID == uuid.Nil {
		return application.ProjectContentAccess{}, application.ErrProjectNotFound
	}
	query := `SELECT id AS project_id,org_id,revision,status FROM workspace.project WHERE id=? AND org_id=? AND NOT is_delete AND status IN ('active','archived')`
	if write {
		query += ` FOR UPDATE`
	} else {
		query += ` FOR SHARE`
	}
	var row struct {
		application.ProjectContentAccess
		Status string
	}
	read := s.tx.WithContext(ctx).Raw(query, projectID, actor.OrgID).Scan(&row)
	if read.Error != nil {
		return application.ProjectContentAccess{}, fmt.Errorf("authorize script project: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return application.ProjectContentAccess{}, application.ErrProjectNotFound
	}
	if write && row.Status != "active" {
		return application.ProjectContentAccess{}, domain.ErrProjectStateConflict
	}
	if row.ProjectID != projectID || row.OrgID != actor.OrgID || row.Revision < 1 {
		return application.ProjectContentAccess{}, application.ErrProjectDependencyUnavailable
	}
	return row.ProjectContentAccess, nil
}

// TouchContent advances the project in the same transaction as script changes.
func (s *ProjectContentAccessStore) TouchContent(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, expected int64) (int64, error) {
	if expected < 1 || expected >= math.MaxInt32 {
		return 0, domain.ErrProjectRevisionConflict
	}
	facts, err := s.Authorize(ctx, actor, projectID, true)
	if err != nil {
		return 0, err
	}
	if facts.Revision != expected {
		return 0, domain.ErrProjectRevisionConflict
	}
	changed := s.tx.WithContext(ctx).Exec(`UPDATE workspace.project SET revision=revision+1,update_time=now() WHERE id=? AND org_id=? AND revision=? AND NOT is_delete AND status='active'`, projectID, actor.OrgID, expected)
	if changed.Error != nil {
		return 0, fmt.Errorf("advance script project revision: %w", changed.Error)
	}
	if changed.RowsAffected != 1 {
		return 0, domain.ErrProjectRevisionConflict
	}
	var updatedAt time.Time
	if err := s.tx.WithContext(ctx).Raw(`SELECT update_time FROM workspace.project WHERE id=? AND org_id=?`, projectID, actor.OrgID).Scan(&updatedAt).Error; err != nil {
		return 0, fmt.Errorf("read content change time: %w", err)
	}
	event, err := application.ProjectContentChangedEvent(actor, projectID, expected+1, updatedAt)
	if err != nil {
		return 0, err
	}
	written := s.tx.WithContext(ctx).Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, event.ID, event.Topic, event.PartitionKey, string(event.Payload))
	if written.Error != nil {
		return 0, fmt.Errorf("persist content invalidation: %w", written.Error)
	}
	if written.RowsAffected != 1 {
		return 0, application.ErrProjectDependencyUnavailable
	}
	return expected + 1, nil
}
