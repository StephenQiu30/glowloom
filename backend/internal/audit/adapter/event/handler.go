// Package event writes validated audit events and consumer markers atomically.
package event

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	pgaudit "github.com/StephenQiu30/lanverse/backend/internal/audit/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

// ProcessedStore owns a transaction joining deduplication and the audit effect.
type ProcessedStore interface {
	ProcessOnce(context.Context, string, string, func(context.Context, *gorm.DB) error) (bool, error)
}

// Handler handles only audit.recorded.v1 events.
type Handler struct {
	processed ProcessedStore
	parser    *application.Parser
}

// NewHandler injects the deduplication store and action summary policy.
func NewHandler(processed ProcessedStore, parser *application.Parser) *Handler {
	return &Handler{processed: processed, parser: parser}
}

// Handle validates and inserts one record in the same transaction as its marker.
func (h *Handler) Handle(ctx context.Context, record inbox.Record) error {
	auditRecord, err := h.parser.Parse(record)
	if err != nil {
		return err
	}
	_, err = h.processed.ProcessOnce(ctx, "audit", auditRecord.ID.String(), func(ctx context.Context, tx *gorm.DB) error {
		return pgaudit.NewStore(tx).Insert(ctx, auditRecord)
	})
	if err != nil {
		return fmt.Errorf("store audit event %s: %w", auditRecord.ID, err)
	}
	return nil
}

var _ inbox.Handler = (*Handler)(nil)
