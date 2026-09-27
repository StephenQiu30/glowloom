// Package application coordinates budget commands across domain and persistence.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

const (
	budgetChangedTopic = "lanverse.billing.budget_changed.v1"
	budgetAuditTopic   = "lanverse.audit.recorded.v1"
)

// ErrInvalidChangeBudget means the input or committed result is invalid.
var ErrInvalidChangeBudget = errors.New("invalid change-budget command")

// ChangeBudgetStore reads the budget and atomically commits its revision,
// signed ledger entry, audit event, and budget change event.
type ChangeBudgetStore interface {
	FindBudget(context.Context, identityapp.Principal, uuid.UUID) (domain.Budget, error)
	ChangeBudgetWithEvents(context.Context, identityapp.Principal, domain.Budget, domain.Budget, domain.LedgerEntry, []identityapp.OutboxEvent) (domain.Budget, error)
}

// ChangeBudgetInput sets a project limit against one observed revision.
type ChangeBudgetInput struct {
	ProjectID        uuid.UUID
	LimitMicros      int64
	ExpectedRevision int64
	RequestID        string
}

// ChangeBudgetCommand updates the spending limit and records the change.
type ChangeBudgetCommand struct {
	store ChangeBudgetStore
	now   func() time.Time
}

// NewChangeBudgetCommand injects persistence and a clock.
func NewChangeBudgetCommand(store ChangeBudgetStore, now func() time.Time) *ChangeBudgetCommand {
	return &ChangeBudgetCommand{store: store, now: now}
}

// Execute returns the current budget unchanged for a no-op.
func (c *ChangeBudgetCommand) Execute(ctx context.Context, actor identityapp.Principal, input ChangeBudgetInput) (domain.Budget, error) {
	if c == nil || c.store == nil || c.now == nil {
		return domain.Budget{}, ErrInvalidChangeBudget
	}
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword ||
		(actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer) {
		return domain.Budget{}, identityapp.ErrForbidden
	}
	requestID, err := uuid.Parse(input.RequestID)
	if input.ProjectID == uuid.Nil || input.LimitMicros < 0 ||
		input.ExpectedRevision < 1 || input.ExpectedRevision >= math.MaxInt32 ||
		err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID {
		return domain.Budget{}, ErrInvalidChangeBudget
	}
	before, err := c.store.FindBudget(ctx, actor, input.ProjectID)
	if err != nil {
		return domain.Budget{}, fmt.Errorf("find budget to change: %w", err)
	}
	if before.ProjectID != input.ProjectID || before.Validate() != nil {
		return domain.Budget{}, ErrInvalidChangeBudget
	}
	if before.Revision != input.ExpectedRevision {
		return domain.Budget{}, domain.ErrBudgetRevision
	}
	after := before
	delta, err := after.ChangeLimit(input.LimitMicros)
	if err != nil {
		return domain.Budget{}, err
	}
	if delta == 0 {
		return before, nil
	}
	occurredAt := c.now().UTC()
	if occurredAt.IsZero() {
		return domain.Budget{}, ErrInvalidChangeBudget
	}
	entry := domain.LedgerEntry{
		ID: uuid.New(), ProjectID: before.ProjectID, EntryType: "budget_change",
		AmountMicros: delta, CreateTime: occurredAt, CreateBy: &actor.ID,
	}
	events, err := budgetChangedEvents(actor, before, after, input.RequestID, occurredAt)
	if err != nil {
		return domain.Budget{}, fmt.Errorf("build budget change events: %w", err)
	}
	saved, err := c.store.ChangeBudgetWithEvents(ctx, actor, before, after, entry, events)
	if err != nil {
		return domain.Budget{}, fmt.Errorf("change budget with events: %w", err)
	}
	if saved.Validate() != nil || saved != after {
		return domain.Budget{}, ErrInvalidChangeBudget
	}
	return saved, nil
}

func budgetChangedEvents(actor identityapp.Principal, before, after domain.Budget, requestID string, occurredAt time.Time) ([]identityapp.OutboxEvent, error) {
	available, err := after.AvailableMicros()
	if err != nil {
		return nil, err
	}
	changeID, auditID := uuid.New(), uuid.New()
	change, err := json.Marshal(map[string]any{
		"event_id": changeID, "event_type": budgetChangedTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID, "project_id": after.ProjectID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "budget", "id": after.ID, "revision": after.Revision},
		"data": map[string]any{
			"limit_micros": after.LimitMicros, "available_micros": available,
			"is_overrun": after.IsOverrun,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode budget changed event: %w", err)
	}
	audit, err := json.Marshal(map[string]any{
		"event_id": auditID, "event_type": budgetAuditTopic,
		"occurred_at": occurredAt, "org_id": actor.OrgID, "project_id": after.ProjectID,
		"actor":     map[string]any{"kind": "user", "id": actor.ID},
		"aggregate": map[string]any{"type": "audit", "id": auditID},
		"data": map[string]any{
			"action": "budget.changed", "object": map[string]any{"type": "budget", "id": after.ID},
			"request_id": requestID,
			"before": map[string]any{
				"limit_micros": before.LimitMicros, "revision": before.Revision,
				"is_overrun": before.IsOverrun,
			},
			"after": map[string]any{
				"limit_micros": after.LimitMicros, "revision": after.Revision,
				"is_overrun": after.IsOverrun,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode budget changed audit: %w", err)
	}
	key := after.ProjectID.String()
	return []identityapp.OutboxEvent{
		{ID: changeID, Topic: budgetChangedTopic, PartitionKey: key, Payload: change},
		{ID: auditID, Topic: budgetAuditTopic, PartitionKey: key, Payload: audit},
	}, nil
}
