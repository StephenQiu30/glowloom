package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

var (
	// ErrInvalidPublicQuery means task navigation has invalid filters or scope.
	ErrInvalidPublicQuery = errors.New("invalid public operation query")
	// ErrPublicNotFound hides absent and other-project task records.
	ErrPublicNotFound = errors.New("public operation not found")
)

// TaskSummary contains safe task facts, never raw provider receipts or credentials.
type TaskSummary struct {
	ID              uuid.UUID            `json:"id"`
	ProjectID       uuid.UUID            `json:"project_id"`
	BatchID         *uuid.UUID           `json:"batch_id" extensions:"x-nullable"`
	Capability      string               `json:"capability"`
	Mode            string               `json:"mode"`
	ModelKey        string               `json:"model_key"`
	ModelName       string               `json:"model_name"`
	TargetType      string               `json:"target_type"`
	TargetID        *uuid.UUID           `json:"target_id" extensions:"x-nullable"`
	Source          *domain.CanvasSource `json:"source" extensions:"x-nullable"`
	Origin          string               `json:"origin"`
	Status          domain.Status        `json:"status"`
	QuoteMicros     *int64               `json:"quote_micros" extensions:"x-nullable"`
	SettledMicros   *int64               `json:"settled_micros" extensions:"x-nullable"`
	FailureCode     *string              `json:"failure_code" extensions:"x-nullable"`
	Retryable       *bool                `json:"retryable" extensions:"x-nullable"`
	CancelRequested bool                 `json:"cancel_requested"`
	CreateTime      time.Time            `json:"create_time"`
	UpdateTime      time.Time            `json:"update_time"`
	StartedAt       *time.Time           `json:"started_at" extensions:"x-nullable"`
	FinishedAt      *time.Time           `json:"finished_at" extensions:"x-nullable"`
}

// TaskInput exposes only frozen prompt and project-owned media identities.
type TaskInput struct {
	Sequence     int32      `json:"sequence"`
	Role         string     `json:"role"`
	Text         *string    `json:"text" extensions:"x-nullable"`
	MediaAssetID *uuid.UUID `json:"media_asset_id" extensions:"x-nullable"`
	MaskAssetID  *uuid.UUID `json:"mask_asset_id" extensions:"x-nullable"`
}

// TaskOutput describes a candidate; preview authorization remains with media.
type TaskOutput struct {
	ID               uuid.UUID  `json:"id"`
	Sequence         int32      `json:"sequence"`
	Kind             string     `json:"kind"`
	MediaAssetID     *uuid.UUID `json:"media_asset_id" extensions:"x-nullable"`
	ModerationStatus string     `json:"moderation_status"`
	CreateTime       time.Time  `json:"create_time"`
}

// TaskEvent exposes lifecycle transitions with a stable reason, not private detail.
type TaskEvent struct {
	ID         uuid.UUID `json:"id"`
	FromStatus *string   `json:"from_status" extensions:"x-nullable"`
	ToStatus   string    `json:"to_status"`
	Reason     *string   `json:"reason" extensions:"x-nullable"`
	CreateTime time.Time `json:"create_time"`
}

// TaskDetail is the persisted task snapshot used after refresh or reconnection.
type TaskDetail struct {
	TaskSummary
	Params         json.RawMessage `json:"params" swaggertype:"object"`
	OutputCount    int32           `json:"output_count"`
	QuoteDetail    json.RawMessage `json:"quote_detail" swaggertype:"object"`
	QuoteExpiresAt *time.Time      `json:"quote_expires_at" extensions:"x-nullable"`
	ReusedFromID   *uuid.UUID      `json:"reused_from_id" extensions:"x-nullable"`
	Inputs         []TaskInput     `json:"inputs"`
	Outputs        []TaskOutput    `json:"outputs"`
	Events         []TaskEvent     `json:"events"`
}

// TaskCursor is an internal keyset position; HTTP owns its binding and encoding.
type TaskCursor struct {
	ID         uuid.UUID
	CreateTime time.Time
}

// ListTasksInput scopes a task page to one authorized project.
type ListTasksInput struct {
	CanvasID, NodeID, RowID *uuid.UUID
	ProjectID               uuid.UUID
	Status                  string
	Capability              string
	ModelKey                string
	Origin                  string
	Limit                   int
	After                   *TaskCursor
}

// TaskPage holds one bounded page plus the next internal position.
type TaskPage struct {
	Items []TaskSummary
	Next  *TaskCursor
}

// BatchDetail includes all children of the bounded creation batch.
type BatchDetail struct {
	ID                uuid.UUID          `json:"id"`
	ProjectID         uuid.UUID          `json:"project_id"`
	Kind              string             `json:"kind"`
	Status            domain.BatchStatus `json:"status"`
	TotalCount        int32              `json:"total_count"`
	SucceededCount    int32              `json:"succeeded_count"`
	FailedCount       int32              `json:"failed_count"`
	UnknownCount      int32              `json:"unknown_count"`
	QuoteTotalMicros  int64              `json:"quote_total_micros"`
	PausedReason      *string            `json:"paused_reason" extensions:"x-nullable"`
	CancelRequestedAt *time.Time         `json:"cancel_requested_at" extensions:"x-nullable"`
	Items             []TaskSummary      `json:"items"`
}

// PublicQueryStore rechecks current actor and project rights with each snapshot.
type PublicQueryStore interface {
	ListPublicTasks(context.Context, identityapp.Principal, ListTasksInput) (TaskPage, error)
	ReadPublicTask(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (TaskDetail, error)
	ReadPublicBatch(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (BatchDetail, error)
}

// PublicQuery validates public navigation before reading durable task facts.
type PublicQuery struct{ store PublicQueryStore }

// NewPublicQuery injects the current-rights task reader.
func NewPublicQuery(store PublicQueryStore) *PublicQuery { return &PublicQuery{store: store} }

func publicActor(actor identityapp.Principal) error {
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword || (actor.Role != identitydomain.RoleProducer && actor.Role != identitydomain.RoleAdmin) {
		return identityapp.ErrForbidden
	}
	return nil
}

// List reads tasks with stable keyset ordering and bounded filters.
func (q *PublicQuery) List(ctx context.Context, actor identityapp.Principal, input ListTasksInput) (TaskPage, error) {
	if err := publicActor(actor); err != nil {
		return TaskPage{}, err
	}
	if q == nil || q.store == nil || input.ProjectID == uuid.Nil || input.Limit < 0 || input.Limit > 200 || len(input.Capability) > 128 || len(input.ModelKey) > 256 ||
		(input.After != nil && (input.After.ID == uuid.Nil || input.After.CreateTime.IsZero())) {
		return TaskPage{}, ErrInvalidPublicQuery
	}
	for _, id := range []*uuid.UUID{input.CanvasID, input.NodeID, input.RowID} {
		if id != nil && *id == uuid.Nil {
			return TaskPage{}, ErrInvalidPublicQuery
		}
	}
	if (input.NodeID != nil && input.CanvasID == nil) || (input.RowID != nil && input.NodeID == nil) {
		return TaskPage{}, ErrInvalidPublicQuery
	}
	if input.Status != "" && domain.Status(input.Status).CanTransitionTo(domain.Status(input.Status)) != nil {
		return TaskPage{}, ErrInvalidPublicQuery
	}
	switch input.Origin {
	case "", "pipeline", "batch", "canvas", "agent", "upload", "system":
	default:
		return TaskPage{}, ErrInvalidPublicQuery
	}
	if input.Limit == 0 {
		input.Limit = 50
	}
	page, err := q.store.ListPublicTasks(ctx, actor, input)
	if err != nil {
		return TaskPage{}, fmt.Errorf("list public tasks: %w", err)
	}
	return page, nil
}

// Task reads the same persisted snapshot after refresh and reconnect.
func (q *PublicQuery) Task(ctx context.Context, actor identityapp.Principal, projectID, taskID uuid.UUID) (TaskDetail, error) {
	if err := publicActor(actor); err != nil {
		return TaskDetail{}, err
	}
	if q == nil || q.store == nil || projectID == uuid.Nil || taskID == uuid.Nil {
		return TaskDetail{}, ErrInvalidPublicQuery
	}
	result, err := q.store.ReadPublicTask(ctx, actor, projectID, taskID)
	if err != nil {
		return TaskDetail{}, fmt.Errorf("read public task: %w", err)
	}
	return result, nil
}

// Batch returns the persisted parent and all of its bounded children.
func (q *PublicQuery) Batch(ctx context.Context, actor identityapp.Principal, projectID, batchID uuid.UUID) (BatchDetail, error) {
	if err := publicActor(actor); err != nil {
		return BatchDetail{}, err
	}
	if q == nil || q.store == nil || projectID == uuid.Nil || batchID == uuid.Nil {
		return BatchDetail{}, ErrInvalidPublicQuery
	}
	result, err := q.store.ReadPublicBatch(ctx, actor, projectID, batchID)
	if err != nil {
		return BatchDetail{}, fmt.Errorf("read public batch: %w", err)
	}
	return result, nil
}
