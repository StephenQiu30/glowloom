package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

const publicTaskSelect = `SELECT o.id, o.project_id, o.batch_id, o.capability, o.mode,
 COALESCE(m.model_key,'') AS model_key, COALESCE(m.display_name,'') AS model_name,
 COALESCE(o.target_type,'') AS target_type, o.target_id, o.origin, o.status,
 o.quote_micros, o.settled_micros, o.failure_code, o.retryable,
 EXISTS(SELECT 1 FROM operation.operation_event AS e WHERE e.operation_id=o.id
   AND e.reason='user_cancel_requested' AND NOT e.is_delete) AS cancel_requested,
 o.create_time, o.update_time, o.started_at, o.finished_at,o.source_context::text AS source_json`
const publicTaskJoin = ` FROM operation.operation AS o
 LEFT JOIN catalog.model_profile_version AS v ON v.id=o.model_profile_version_id
 LEFT JOIN catalog.model_profile AS m ON m.id=v.model_profile_id`

type publicTaskDetailRow struct {
	Summary               publicTaskRow `gorm:"embedded"`
	Params                string
	OutputCount           int32
	QuoteDetail           string
	QuoteExpiresAt        *time.Time
	ReusedFromID          *uuid.UUID
	PromptPreparationJSON *string
}

type publicTaskRow struct {
	ID, ProjectID                                             uuid.UUID
	BatchID                                                   *uuid.UUID
	Capability, Mode, ModelKey, ModelName, TargetType, Origin string
	TargetID                                                  *uuid.UUID
	Status                                                    domain.Status
	QuoteMicros, SettledMicros                                *int64
	FailureCode                                               *string
	Retryable                                                 *bool
	CancelRequested                                           bool
	CreateTime, UpdateTime                                    time.Time
	StartedAt, FinishedAt                                     *time.Time
	SourceJSON                                                *string
}

func (r publicTaskRow) summary() (application.TaskSummary, error) {
	item := application.TaskSummary{ID: r.ID, ProjectID: r.ProjectID, BatchID: r.BatchID, Capability: r.Capability, Mode: r.Mode, ModelKey: r.ModelKey, ModelName: r.ModelName, TargetType: r.TargetType, TargetID: r.TargetID, Origin: r.Origin, Status: r.Status, QuoteMicros: r.QuoteMicros, SettledMicros: r.SettledMicros, FailureCode: r.FailureCode, Retryable: r.Retryable, CancelRequested: r.CancelRequested, CreateTime: r.CreateTime, UpdateTime: r.UpdateTime, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt}
	if r.SourceJSON != nil {
		var source domain.CanvasSource
		if err := json.Unmarshal([]byte(*r.SourceJSON), &source); err != nil || !source.Valid() {
			return application.TaskSummary{}, fmt.Errorf("invalid stored task source")
		}
		item.Source = &source
	}
	return item, nil
}

type publicBatchRow struct {
	ID, ProjectID                                         uuid.UUID
	Kind                                                  string
	Status                                                domain.BatchStatus
	TotalCount, SucceededCount, FailedCount, UnknownCount int32
	QuoteTotalMicros                                      int64
	PausedReason                                          *string
	CancelRequestedAt                                     *time.Time
}

func requirePublicProject(tx *gorm.DB, actor identityapp.Principal, projectID uuid.UUID, active bool) error {
	if err := requireCurrentActor(tx, actor); err != nil {
		return err
	}
	var found int
	r := tx.Raw(`SELECT 1 FROM workspace.project WHERE id=?::uuid AND org_id=?::uuid
 AND NOT is_delete AND (NOT ? OR status='active') FOR SHARE`, projectID, actor.OrgID, active).Scan(&found)
	if r.Error != nil {
		return fmt.Errorf("check public task project: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.ErrPublicNotFound
	}
	return nil
}

// ListPublicTasks obtains an authorized page in a repeatable snapshot.
func (s *Store) ListPublicTasks(ctx context.Context, actor identityapp.Principal, input application.ListTasksInput) (application.TaskPage, error) {
	page := application.TaskPage{Items: []application.TaskSummary{}}
	if s == nil || s.db == nil {
		return page, ErrUnavailable
	}
	if input.ProjectID == uuid.Nil || input.Limit < 1 || input.Limit > 200 {
		return page, application.ErrInvalidPublicQuery
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requirePublicProject(tx, actor, input.ProjectID, false); err != nil {
			return err
		}
		query := publicTaskSelect + publicTaskJoin + ` WHERE o.project_id=?::uuid AND NOT o.is_delete
 AND (?='' OR o.status=?) AND (?='' OR o.capability=?) AND (?='' OR m.model_key=?) AND (?='' OR o.origin=?)`
		args := []any{input.ProjectID, input.Status, input.Status, input.Capability, input.Capability, input.ModelKey, input.ModelKey, input.Origin, input.Origin}
		for _, field := range []struct {
			name string
			id   *uuid.UUID
		}{{"canvas_id", input.CanvasID}, {"node_id", input.NodeID}, {"row_id", input.RowID}} {
			if field.id != nil {
				query += ` AND o.source_context->>'` + field.name + `'=?`
				args = append(args, field.id.String())
			}
		}
		if input.After != nil {
			query += ` AND (o.create_time,o.id)<(?,?::uuid)`
			args = append(args, input.After.CreateTime, input.After.ID)
		}
		query += ` ORDER BY o.create_time DESC,o.id DESC LIMIT ?`
		args = append(args, input.Limit+1)
		var rows []publicTaskRow
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read public task page: %w", err)
		}
		for _, row := range rows {
			item, err := row.summary()
			if err != nil {
				return err
			}
			page.Items = append(page.Items, item)
		}
		if len(page.Items) > input.Limit {
			page.Items = page.Items[:input.Limit]
			last := page.Items[len(page.Items)-1]
			page.Next = &application.TaskCursor{ID: last.ID, CreateTime: last.CreateTime}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	return page, err
}

// ReadPublicTask reads the task and child facts without exposing provider evidence.
func (s *Store) ReadPublicTask(ctx context.Context, actor identityapp.Principal, projectID, operationID uuid.UUID) (application.TaskDetail, error) {
	result := application.TaskDetail{Inputs: []application.TaskInput{}, Outputs: []application.TaskOutput{}, Events: []application.TaskEvent{}}
	if s == nil || s.db == nil {
		return result, ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requirePublicProject(tx, actor, projectID, false); err != nil {
			return err
		}
		var task publicTaskDetailRow
		row := tx.Raw(publicTaskSelect+`,o.params::text AS params,o.output_count,o.quote_detail::text AS quote_detail,
 o.quote_expires_at,o.reused_from_id,o.prompt_preparation::text AS prompt_preparation_json`+publicTaskJoin+` WHERE o.id=?::uuid AND o.project_id=?::uuid AND NOT o.is_delete`, operationID, projectID).Scan(&task)
		if row.Error != nil {
			return fmt.Errorf("read public task: %w", row.Error)
		}
		if row.RowsAffected != 1 {
			return application.ErrPublicNotFound
		}
		var err error
		result.TaskSummary, err = task.Summary.summary()
		if err != nil {
			return err
		}
		result.Params, result.QuoteDetail = json.RawMessage(task.Params), json.RawMessage(task.QuoteDetail)
		result.OutputCount, result.QuoteExpiresAt, result.ReusedFromID = task.OutputCount, task.QuoteExpiresAt, task.ReusedFromID
		if task.PromptPreparationJSON != nil {
			var preparation domain.PromptPreparation
			if err := json.Unmarshal([]byte(*task.PromptPreparationJSON), &preparation); err != nil || preparation.ValidateFor(result.Capability) != nil {
				return fmt.Errorf("invalid stored prompt preparation")
			}
			result.PromptPreparation = &preparation
		}
		if len(result.Params) == 0 {
			result.Params = json.RawMessage(`{}`)
		}
		if len(result.QuoteDetail) == 0 {
			result.QuoteDetail = json.RawMessage(`{}`)
		}
		if err := tx.Raw(`SELECT seq_no AS sequence,role,text_value AS text,media_asset_id,mask_asset_id
 FROM operation.operation_input WHERE operation_id=?::uuid AND NOT is_delete ORDER BY seq_no`, operationID).Scan(&result.Inputs).Error; err != nil {
			return fmt.Errorf("read public task inputs: %w", err)
		}
		if result.PromptPreparation != nil {
			if len(result.Inputs) == 0 || result.Inputs[0].Sequence != 0 || result.Inputs[0].Role != "prompt" || result.Inputs[0].Text == nil {
				return fmt.Errorf("missing frozen prepared prompt")
			}
			if quotePromptDigest(*result.Inputs[0].Text) != result.PromptPreparation.ContentSHA256 {
				return fmt.Errorf("frozen prepared prompt digest mismatch")
			}
		}
		if err := tx.Raw(`SELECT id,seq_no AS sequence,kind,media_asset_id,moderation_status,create_time
 FROM operation.operation_output WHERE operation_id=?::uuid AND project_id=?::uuid AND NOT is_delete ORDER BY seq_no`, operationID, projectID).Scan(&result.Outputs).Error; err != nil {
			return fmt.Errorf("read public task candidates: %w", err)
		}
		if err := tx.Raw(`SELECT id,from_status,to_status,reason,create_time FROM operation.operation_event
 WHERE operation_id=?::uuid AND NOT is_delete ORDER BY create_time,id LIMIT 1000`, operationID).Scan(&result.Events).Error; err != nil {
			return fmt.Errorf("read public task history: %w", err)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	return result, err
}

// ReadPublicBatch reads all bounded children within the same authorized snapshot.
func (s *Store) ReadPublicBatch(ctx context.Context, actor identityapp.Principal, projectID, batchID uuid.UUID) (application.BatchDetail, error) {
	result := application.BatchDetail{Items: []application.TaskSummary{}}
	if s == nil || s.db == nil {
		return result, ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requirePublicProject(tx, actor, projectID, false); err != nil {
			return err
		}
		var batch publicBatchRow
		row := tx.Raw(`SELECT id,project_id,kind,status,total_count,succeeded_count,failed_count,unknown_count,
 quote_total_micros,paused_reason,cancel_requested_at FROM operation.batch
 WHERE id=?::uuid AND project_id=?::uuid AND NOT is_delete`, batchID, projectID).Scan(&batch)
		if row.Error != nil {
			return fmt.Errorf("read public batch: %w", row.Error)
		}
		if row.RowsAffected != 1 {
			return application.ErrPublicNotFound
		}
		result.ID, result.ProjectID, result.Kind, result.Status = batch.ID, batch.ProjectID, batch.Kind, batch.Status
		result.TotalCount, result.SucceededCount, result.FailedCount, result.UnknownCount = batch.TotalCount, batch.SucceededCount, batch.FailedCount, batch.UnknownCount
		result.QuoteTotalMicros, result.PausedReason, result.CancelRequestedAt = batch.QuoteTotalMicros, batch.PausedReason, batch.CancelRequestedAt
		var rows []publicTaskRow
		if err := tx.Raw(publicTaskSelect+publicTaskJoin+` WHERE o.batch_id=?::uuid AND o.project_id=?::uuid AND NOT o.is_delete ORDER BY o.create_time,o.id LIMIT 301`, batchID, projectID).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read public batch items: %w", err)
		}
		for _, row := range rows {
			item, err := row.summary()
			if err != nil {
				return err
			}
			result.Items = append(result.Items, item)
		}
		if len(result.Items) > 300 {
			return fmt.Errorf("public batch exceeds bound")
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	return result, err
}

var _ application.PublicQueryStore = (*Store)(nil)
