package application

import (
	"context"
	"fmt"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

// Handler routes supported Kafka topics to their project-scoped projections.
type Handler struct {
	operation *OperationStatusHandler
	project   *ProjectChangedHandler
}

// NewHandler injects the shared deduplication store and realtime sink.
func NewHandler(processed ProcessedStore, sink Sink) *Handler {
	return &Handler{
		operation: NewOperationStatusHandler(processed, sink),
		project:   NewProjectChangedHandler(processed, sink),
	}
}

// Handle dispatches one record without accepting undeclared topics.
func (h *Handler) Handle(ctx context.Context, record inbox.Record) error {
	switch record.Topic {
	case OperationStatusTopic:
		return h.operation.Handle(ctx, record)
	case ProjectChangedTopic:
		return h.project.Handle(ctx, record)
	default:
		return fmt.Errorf("%w: unsupported topic %s", ErrInvalidEvent, record.Topic)
	}
}

var _ inbox.Handler = (*Handler)(nil)
