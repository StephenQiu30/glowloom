package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

type outputAttemptWindow struct {
	CommandID      uuid.UUID `json:"command_id"`
	MediaCommandID uuid.UUID `json:"media_command_id"`
	PayloadSHA256  string    `json:"payload_sha256"`
	TaskKind       string    `json:"task_kind"`
	BusinessID     string    `json:"business_id"`
	Owner          string    `json:"owner"`
	Epoch          int64     `json:"epoch"`
}

func outputWindow(ctx context.Context, req application.OutputDeliveryRequest) taskruntime.Job {
	if job, ok := taskruntime.CurrentJob(ctx); ok {
		return job
	}
	w := req.Publication.Output.Worker
	return taskruntime.Job{Kind: w.Kind, BusinessID: w.TaskID.String(), Owner: w.Owner, Epoch: w.Epoch}
}

// recordOutputAttemptWindow retains the actual first transfer's lease in immutable
// local operation history. Recovery never rewrites the original request's worker.
func recordOutputAttemptWindow(ctx context.Context, tx *gorm.DB, req application.OutputDeliveryRequest) error {
	job := outputWindow(ctx, req)
	if err := taskruntime.Fence(ctx, tx, job); err != nil {
		return err
	}
	facts := outputAttemptWindow{CommandID: req.Command.ID, MediaCommandID: req.Publication.Command.ID, PayloadSHA256: req.Publication.Output.Digest(), TaskKind: job.Kind, BusinessID: job.BusinessID, Owner: job.Owner, Epoch: job.Epoch}
	raw, err := json.Marshal(facts)
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO operation.operation_event(id,operation_id,from_status,to_status,reason,detail) VALUES(?,?,'ingesting','ingesting','output_publication_attempt',?::jsonb)`, uuid.NewSHA1(req.Command.ID, []byte("output-publication-attempt")), req.Publication.Output.OperationID, string(raw)).Error
}

func fenceOriginalOutputAttempt(ctx context.Context, tx *gorm.DB, req application.OutputDeliveryRequest) error {
	var row struct{ Detail json.RawMessage }
	r := tx.Raw(`SELECT detail FROM operation.operation_event WHERE id=? AND operation_id=? AND reason='output_publication_attempt' AND NOT is_delete`, uuid.NewSHA1(req.Command.ID, []byte("output-publication-attempt")), req.Publication.Output.OperationID).Scan(&row)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return taskruntime.ErrLeaseLost
	}
	var facts outputAttemptWindow
	if json.Unmarshal(row.Detail, &facts) != nil || facts.CommandID != req.Command.ID || facts.MediaCommandID != req.Publication.Command.ID || facts.PayloadSHA256 != req.Publication.Output.Digest() || facts.BusinessID != req.Publication.Output.OperationID.String() || (facts.TaskKind != "operation.run" && facts.TaskKind != "operation.ingest") {
		return taskruntime.ErrLeaseLost
	}
	return taskruntime.Fence(ctx, tx, taskruntime.Job{Kind: facts.TaskKind, BusinessID: facts.BusinessID, Owner: facts.Owner, Epoch: facts.Epoch})
}
