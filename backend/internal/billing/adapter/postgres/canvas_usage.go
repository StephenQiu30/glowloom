package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ReadCanvasUsage joins immutable operation origin to the owning ledger after project authorization.
// The caller holds a repeatable-read transaction; no reservations or overage are user charges.
func (s *Store) ReadCanvasUsage(ctx context.Context, actor identity.Principal, project, canvas uuid.UUID, page int) (application.CanvasUsage, error) {
	result := application.CanvasUsage{Entries: []application.CanvasCharge{}}
	if page < 1 || page > 10000 {
		return result, identity.ErrForbidden
	}
	tx := s.db.WithContext(ctx)
	if err := s.requireProject(tx, actor, project, false); err != nil {
		return result, err
	}
	var totals struct{ TaskCount, SettledMicros string }
	const origin = `o.project_id=? AND o.origin='canvas' AND o.source_context->>'canvas_id'=? AND o.confirmed_at IS NOT NULL`
	if err := tx.Raw(`SELECT count(*)::text AS task_count FROM operation.operation o WHERE `+origin, project, canvas.String()).Scan(&totals).Error; err != nil {
		return result, err
	}
	result.TaskCount = totals.TaskCount
	const charges = `FROM billing.ledger_entry l JOIN operation.operation o ON o.id=l.operation_id AND o.project_id=l.project_id WHERE l.project_id=? AND l.entry_type='settle' AND NOT l.is_delete AND ` + origin
	if err := tx.Raw(`SELECT COALESCE(sum(l.amount_micros),0)::text AS settled_micros `+charges, project, project, canvas.String()).Scan(&totals).Error; err != nil {
		return result, err
	}
	if err := tx.Raw(`SELECT l.id,l.operation_id,l.amount_micros::text AS amount_micros,l.create_time AS created_at,COALESCE(l.model_key,'') AS model_key `+charges+` ORDER BY l.create_time DESC,l.id DESC LIMIT 21 OFFSET ?`, project, project, canvas.String(), (page-1)*20).Scan(&result.Entries).Error; err != nil {
		return result, err
	}
	result.SettledMicros = totals.SettledMicros
	if len(result.Entries) > 20 {
		next := page + 1
		result.NextPage = &next
		result.Entries = result.Entries[:20]
	}
	return result, nil
}
