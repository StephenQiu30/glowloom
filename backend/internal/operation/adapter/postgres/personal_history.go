package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

// Both request receipts are immutable provenance written with quote creation.
// IDs remain text when read from JSON so malformed legacy receipts cannot cause
// a UUID cast failure, nor can a confirmer be mistaken for the initiating actor.
const personalOperationIDs = `WITH personal_operation_ids AS (
 SELECT ps.operation_id::text AS operation_id FROM operation.prompt_submission ps
 JOIN operation.operation submitted ON submitted.id=ps.operation_id AND submitted.project_id=ps.project_id
 JOIN workspace.project submitted_project ON submitted_project.id=ps.project_id
 WHERE ps.actor_id=? AND submitted_project.org_id=?
 UNION
 SELECT qr.result->>'operation_id' FROM operation.quote_request qr
 WHERE qr.actor_id=? AND qr.org_id=? AND jsonb_typeof(qr.result->'operation_id')='string'
 UNION
 SELECT item->>'operation_id' FROM operation.quote_request qr
 CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(qr.result->'items')='array' THEN qr.result->'items' ELSE '[]'::jsonb END) item
 WHERE qr.actor_id=? AND qr.org_id=? AND jsonb_typeof(item->'operation_id')='string'
) `

type personalHistoryRow struct {
	Summary         publicTaskRow `gorm:"embedded"`
	PromptPreview   string
	HistoryRevision int64
}

func personalHistorySQL(actor identityapp.Principal, input application.PersonalHistoryInput) (string, []any) {
	cte, args := personalHistoryCTE(actor)
	query := cte + publicTaskSelect + `,COALESCE(history.revision,0) AS history_revision,COALESCE((SELECT LEFT(i.text_value,240) FROM operation.operation_input i
 WHERE i.operation_id=o.id AND i.role='prompt' AND NOT i.is_delete ORDER BY i.seq_no LIMIT 1),'') AS prompt_preview` + publicTaskJoin + `
 JOIN workspace.project p ON p.id=o.project_id
 JOIN personal_operation_ids initiated ON initiated.operation_id=o.id::text
 LEFT JOIN operation.personal_history_record history ON history.operation_id=o.id AND history.actor_id=? AND history.org_id=? AND history.scope_id=?
 WHERE history.deleted_at IS NULL AND EXISTS(SELECT 1 FROM history_outputs candidate WHERE candidate.operation_id=o.id`
	filter, filterArgs := personalOutputFilter(input, "candidate")
	query += filter + `) AND p.org_id=? AND p.owner_scope_id=? AND NOT p.is_delete AND p.status IN ('active','archived')
 AND NOT o.is_delete AND o.origin IN ('pipeline','batch','canvas','agent')`
	args = append(args, actor.ID, actor.OrgID, actor.ScopeID)
	args = append(args, filterArgs...)
	args = append(args, actor.OrgID, actor.ScopeID)

	switch input.Kind {
	case "image", "video", "audio":
		query += ` AND o.capability LIKE ?`
		args = append(args, input.Kind+".%")
	case "document":
		query += ` AND (o.capability LIKE ? OR o.capability LIKE ?)`
		args = append(args, "text.%", "document.%")
	}
	if input.Capability != "" {
		query += ` AND o.capability=?`
		args = append(args, input.Capability)
	}
	if input.Search != "" {
		// strpos makes %, _ and backslashes literal user text rather than SQL wildcards.
		query += ` AND (strpos(lower(COALESCE(m.display_name,'')),lower(?))>0 OR EXISTS(
 SELECT 1 FROM operation.operation_input searched WHERE searched.operation_id=o.id AND NOT searched.is_delete
 AND strpos(lower(COALESCE(searched.text_value,'')),lower(?))>0))`
		args = append(args, input.Search, input.Search)
	}
	if input.CreatedFrom != nil {
		query += ` AND o.create_time>=?`
		args = append(args, *input.CreatedFrom)
	}
	if input.CreatedBefore != nil {
		query += ` AND o.create_time<?`
		args = append(args, *input.CreatedBefore)
	}
	direction, comparison := "DESC", "<"
	if input.Order == "asc" {
		direction, comparison = "ASC", ">"
	}
	if input.After != nil {
		query += ` AND (o.create_time,o.id)` + comparison + `(?,?::uuid)`
		args = append(args, input.After.CreateTime, input.After.ID)
	}
	query += ` ORDER BY o.create_time ` + direction + `,o.id ` + direction + ` LIMIT ?`
	return query, append(args, input.Limit+1)
}

// ListPersonalHistory reads only proven caller-initiated tasks in the current
// space. The existing project authorizer rechecks session, membership and row
// locks for every represented project before any summaries or outputs escape.
func (s *Store) ListPersonalHistory(ctx context.Context, actor identityapp.Principal, input application.PersonalHistoryInput) (application.PersonalHistoryPage, error) {
	if s == nil || s.db == nil {
		return application.PersonalHistoryPage{}, ErrUnavailable
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.ScopeID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1 || s.requestAccess == nil {
		return application.PersonalHistoryPage{}, identityapp.ErrForbidden
	}
	if err := input.Normalize(); err != nil {
		return application.PersonalHistoryPage{}, err
	}
	var page application.PersonalHistoryPage
	err := db.ReadSnapshot(ctx, s.db, func(tx *gorm.DB) error {
		page = application.PersonalHistoryPage{Items: []application.PersonalHistoryItem{}}
		query, args := personalHistorySQL(actor, input)
		var rows []personalHistoryRow
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read personal operation history: %w", err)
		}
		authorized := make(map[uuid.UUID]bool)
		for _, row := range rows {
			if !authorized[row.Summary.ProjectID] {
				if err := s.requirePublicProject(tx, actor, row.Summary.ProjectID, false); err != nil {
					return err
				}
				authorized[row.Summary.ProjectID] = true
			}
		}
		publicationAllowed := false
		if len(rows) > 0 {
			if access, ok := s.requestAccess(tx).(application.HistoryPublicationAccess); ok {
				var err error
				publicationAllowed, err = access.CanPublishHistory(ctx, actor)
				if err != nil {
					return err
				}
			}
		}
		more := len(rows) > input.Limit
		if more {
			rows = rows[:input.Limit]
		}
		ids := make([]uuid.UUID, 0, len(rows))
		positions := make(map[uuid.UUID]int, len(rows))
		for _, row := range rows {
			summary, err := row.Summary.summary()
			if err != nil {
				return err
			}
			positions[summary.ID] = len(page.Items)
			ids = append(ids, summary.ID)
			page.Items = append(page.Items, application.PersonalHistoryItem{TaskSummary: summary, PromptPreview: row.PromptPreview, HistoryRevision: row.HistoryRevision, Outputs: []application.PersonalHistoryOutput{}})
		}
		if more {
			last := page.Items[len(page.Items)-1]
			page.Next = &application.TaskCursor{ID: last.ID, CreateTime: last.CreateTime}
		}
		if len(ids) == 0 {
			return nil
		}
		var outputs []struct {
			OperationID        uuid.UUID
			Output             application.PersonalHistoryOutput `gorm:"embedded"`
			SuperResolution    bool
			FrameInterpolation bool
		}
		cte, args := personalHistoryCTE(actor)
		filter, filterArgs := personalOutputFilter(input, "candidate")
		if filter != "" {
			filter = " AND (candidate.capability LIKE 'video.%' OR (" + strings.TrimPrefix(filter, " AND ") + "))"
		}
		args = append(args, ids)
		args = append(args, filterArgs...)
		if err := tx.Raw(cte+`SELECT candidate.* FROM history_outputs candidate WHERE candidate.operation_id IN ? AND candidate.id IS NOT NULL`+filter+` ORDER BY candidate.operation_id,candidate.sequence,candidate.id`, args...).Scan(&outputs).Error; err != nil {
			return fmt.Errorf("read personal history outputs: %w", err)
		}

		for _, row := range outputs {
			position, ok := positions[row.OperationID]
			if !ok {
				return application.ErrInvalidPublicQuery
			}
			row.Output.CanPublish = row.Output.CanPublish && publicationAllowed
			row.Output.ProcessTypes = []string{}
			if row.SuperResolution {
				row.Output.ProcessTypes = append(row.Output.ProcessTypes, "super_resolution")
			}
			if row.FrameInterpolation {
				row.Output.ProcessTypes = append(row.Output.ProcessTypes, "frame_interpolation")
			}
			page.Items[position].Outputs = append(page.Items[position].Outputs, row.Output)
		}
		return nil
	})
	if err != nil {
		return application.PersonalHistoryPage{}, err
	}
	return page, nil
}

var _ application.PersonalHistoryStore = (*Store)(nil)
