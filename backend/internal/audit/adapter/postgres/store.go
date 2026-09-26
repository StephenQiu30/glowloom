// Package postgres reads audit records within an explicit organization scope.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/audit/domain"
)

var (
	// ErrOrgIDRequired prevents an unscoped audit query.
	ErrOrgIDRequired = errors.New("audit organization ID is required")
	// ErrInvalidLimit prevents unbounded audit pages.
	ErrInvalidLimit = errors.New("audit page limit must be between 1 and 100")
	// ErrInvalidRange means the upper time bound is not after the lower bound.
	ErrInvalidRange = errors.New("audit time range is invalid")
	// ErrInvalidCursor means the page cursor is incomplete.
	ErrInvalidCursor = errors.New("audit page cursor is invalid")
	// ErrNotFound means the record is not visible in the requested organization.
	ErrNotFound = errors.New("audit record not found")
)

// Store reads audit records; authorization is checked before calling it.
type Store struct {
	db *gorm.DB
}

// NewStore injects the database handle used for audit queries.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Insert appends a validated audit record. Callers may inject a transaction so
// the audit row and its processed-event marker commit together.
func (s *Store) Insert(ctx context.Context, record domain.Record) error {
	var projectID, actorID, before, after, ip any
	if record.ProjectID != nil {
		projectID = record.ProjectID.String()
	}
	if record.ActorID != nil {
		actorID = record.ActorID.String()
	}
	if len(record.Before) > 0 {
		before = string(record.Before)
	}
	if len(record.After) > 0 {
		after = string(record.After)
	}
	if record.IP != "" {
		ip = record.IP
	}
	result := s.db.WithContext(ctx).Exec(`
		INSERT INTO audit.audit_log (
			id, org_id, project_id, actor_id, actor_kind, action,
			object_type, object_id, before, after, request_id, trace_id, ip, create_time
		) VALUES (
			?::uuid, ?::uuid, ?::uuid, ?::uuid, ?, ?, ?, ?, ?::jsonb, ?::jsonb, ?, ?, ?::inet, ?
		)
	`, record.ID.String(), record.OrgID.String(), projectID, actorID, record.ActorKind, record.Action,
		record.ObjectType, record.ObjectID, before, after, record.RequestID, record.TraceID, ip, record.CreateTime.UTC())
	if result.Error != nil {
		return fmt.Errorf("insert audit record: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("insert audit record: inserted %d rows", result.RowsAffected)
	}
	return nil
}

// Get returns one record only when it belongs to orgID.
func (s *Store) Get(ctx context.Context, orgID, recordID uuid.UUID) (domain.Record, error) {
	if orgID == uuid.Nil {
		return domain.Record{}, ErrOrgIDRequired
	}
	var record domain.Record
	result := s.db.WithContext(ctx).Raw(`
		SELECT id, org_id, project_id, actor_id, actor_kind, action,
		       object_type, object_id, before, after, request_id, trace_id,
		       COALESCE(ip::text, '') AS ip, create_time
		FROM audit.audit_log
		WHERE org_id = ?::uuid AND id = ?::uuid
		ORDER BY create_time DESC
		LIMIT 1
	`, orgID.String(), recordID.String()).Scan(&record)
	if result.Error != nil {
		return domain.Record{}, fmt.Errorf("get audit record: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.Record{}, ErrNotFound
	}
	return record, nil
}

// List returns newest-first records under the required organization scope.
func (s *Store) List(ctx context.Context, orgID uuid.UUID, filter domain.Filter) (domain.Page, error) {
	if orgID == uuid.Nil {
		return domain.Page{}, ErrOrgIDRequired
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		return domain.Page{}, ErrInvalidLimit
	}
	if !filter.From.IsZero() && !filter.To.IsZero() && !filter.From.Before(filter.To) {
		return domain.Page{}, ErrInvalidRange
	}
	if filter.Before != nil && (filter.Before.ID == uuid.Nil || filter.Before.CreateTime.IsZero()) {
		return domain.Page{}, ErrInvalidCursor
	}

	var query strings.Builder
	query.WriteString(`
		SELECT id, org_id, project_id, actor_id, actor_kind, action,
		       object_type, object_id, before, after, request_id, trace_id,
		       COALESCE(ip::text, '') AS ip, create_time
		FROM audit.audit_log
		WHERE org_id = ?::uuid`)
	args := []any{orgID.String()}
	if !filter.From.IsZero() {
		query.WriteString(" AND create_time >= ?")
		args = append(args, filter.From.UTC())
	}
	if !filter.To.IsZero() {
		query.WriteString(" AND create_time < ?")
		args = append(args, filter.To.UTC())
	}
	if filter.ActorID != nil {
		query.WriteString(" AND actor_id = ?::uuid")
		args = append(args, filter.ActorID.String())
	}
	if filter.ProjectID != nil {
		query.WriteString(" AND project_id = ?::uuid")
		args = append(args, filter.ProjectID.String())
	}
	if filter.ObjectType != "" {
		query.WriteString(" AND object_type = ?")
		args = append(args, filter.ObjectType)
	}
	if filter.Action != "" {
		query.WriteString(" AND action = ?")
		args = append(args, filter.Action)
	}
	if filter.Before != nil {
		query.WriteString(" AND (create_time, id) < (?, ?::uuid)")
		args = append(args, filter.Before.CreateTime.UTC(), filter.Before.ID.String())
	}
	query.WriteString(" ORDER BY create_time DESC, id DESC LIMIT ?")
	args = append(args, filter.Limit+1)
	records := make([]domain.Record, 0)
	if err := s.db.WithContext(ctx).Raw(query.String(), args...).Scan(&records).Error; err != nil {
		return domain.Page{}, fmt.Errorf("list audit records: %w", err)
	}
	page := domain.Page{Items: records}
	if len(records) > filter.Limit {
		page.Items = records[:filter.Limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &domain.Cursor{CreateTime: last.CreateTime, ID: last.ID}
	}
	return page, nil
}
