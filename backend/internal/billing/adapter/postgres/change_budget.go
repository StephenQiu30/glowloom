package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// ErrProjectNotWritable means the project is archived or being purged.
var ErrProjectNotWritable = errors.New("project is not writable")

// ChangeBudgetWithEvents commits one limit change, its ledger fact and both
// durable events under the same project and budget locks.
func (s *Store) ChangeBudgetWithEvents(ctx context.Context, actor identityapp.Principal, before, after domain.Budget, entry domain.LedgerEntry, events []identityapp.OutboxEvent) (domain.Budget, error) {
	if s == nil || s.db == nil {
		return domain.Budget{}, ErrUnavailable
	}
	if before.Validate() != nil || after.Validate() != nil ||
		before.ID != after.ID || before.ProjectID != after.ProjectID ||
		before.Revision+1 != after.Revision || !validBudgetChangeEntry(actor, before, after, entry) ||
		!validBudgetChangedEvents(actor, before, after, entry, events) {
		return domain.Budget{}, domain.ErrInvalidBudget
	}
	var saved domain.Budget
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		var project struct {
			Status   string
			IsDelete bool
		}
		result := tx.Raw(`
			SELECT status, is_delete FROM workspace.project
			WHERE id = ?::uuid AND org_id = ?::uuid FOR SHARE
		`, before.ProjectID.String(), actor.OrgID.String()).Scan(&project)
		if result.Error != nil {
			return fmt.Errorf("lock budget project: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		if project.Status != "active" || project.IsDelete {
			return ErrProjectNotWritable
		}
		var current domain.Budget
		result = tx.Raw(`
			SELECT id, project_id, limit_micros, reserved_micros,
			       settled_micros, is_overrun, revision
			FROM billing.budget
			WHERE id = ?::uuid AND project_id = ?::uuid AND NOT is_delete
			FOR UPDATE
		`, before.ID.String(), before.ProjectID.String()).Scan(&current)
		if result.Error != nil {
			return fmt.Errorf("lock project budget: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		if current != before {
			return domain.ErrBudgetRevision
		}
		expected := current
		if _, err := expected.ChangeLimit(after.LimitMicros); err != nil {
			return err
		}
		if expected != after {
			return domain.ErrInvalidBudget
		}
		result = tx.Exec(`
			UPDATE billing.budget
			SET limit_micros = ?, is_overrun = ?, revision = revision + 1,
			    update_by = ?::uuid
			WHERE id = ?::uuid AND project_id = ?::uuid AND revision = ?
			  AND NOT is_delete
		`, after.LimitMicros, after.IsOverrun, actor.ID.String(),
			after.ID.String(), after.ProjectID.String(), before.Revision)
		if result.Error != nil {
			return fmt.Errorf("update project budget: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return domain.ErrBudgetRevision
		}
		result = tx.Exec(`
			INSERT INTO billing.ledger_entry
			  (id, project_id, entry_type, amount_micros, create_time, create_by)
			VALUES (?::uuid, ?::uuid, 'budget_change', ?, ?, ?::uuid)
		`, entry.ID.String(), entry.ProjectID.String(), entry.AmountMicros,
			entry.CreateTime, actor.ID.String())
		if result.Error != nil {
			return fmt.Errorf("insert budget change ledger: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("insert budget change ledger: inserted %d rows", result.RowsAffected)
		}
		for _, event := range events {
			result = tx.Exec(`
				INSERT INTO infra.outbox (id, topic, partition_key, payload)
				VALUES (?::uuid, ?, ?, ?::jsonb)
			`, event.ID.String(), event.Topic, event.PartitionKey, string(event.Payload))
			if result.Error != nil {
				return fmt.Errorf("insert budget event %s: %w", event.Topic, result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("insert budget event %s: inserted %d rows", event.Topic, result.RowsAffected)
			}
		}
		result = tx.Raw(`
			SELECT id, project_id, limit_micros, reserved_micros,
			       settled_micros, is_overrun, revision
			FROM billing.budget
			WHERE id = ?::uuid AND project_id = ?::uuid AND NOT is_delete
		`, after.ID.String(), after.ProjectID.String()).Scan(&saved)
		if result.Error != nil {
			return fmt.Errorf("read updated budget: %w", result.Error)
		}
		if result.RowsAffected != 1 || saved != after {
			return domain.ErrInvalidBudget
		}
		return nil
	})
	if err != nil {
		return domain.Budget{}, fmt.Errorf("change budget transaction: %w", err)
	}
	return saved, nil
}

func validBudgetChangeEntry(actor identityapp.Principal, before, after domain.Budget, entry domain.LedgerEntry) bool {
	return entry.ID != uuid.Nil && entry.ProjectID == before.ProjectID &&
		entry.EntryType == "budget_change" && entry.AmountMicros == after.LimitMicros-before.LimitMicros &&
		entry.CreateBy != nil && *entry.CreateBy == actor.ID && !entry.CreateTime.IsZero() &&
		entry.OperationID == nil && entry.EpisodeID == nil && entry.ShotID == nil &&
		entry.ModelKey == nil && entry.Region == nil && entry.OrigCurrency == nil &&
		entry.OrigAmountMicros == nil && entry.FXRate == nil && entry.Note == ""
}

type budgetEvent struct {
	EventID    uuid.UUID `json:"event_id"`
	EventType  string    `json:"event_type"`
	OccurredAt time.Time `json:"occurred_at"`
	OrgID      uuid.UUID `json:"org_id"`
	ProjectID  uuid.UUID `json:"project_id"`
	Actor      struct {
		Kind string    `json:"kind"`
		ID   uuid.UUID `json:"id"`
	} `json:"actor"`
	Aggregate struct {
		Type     string    `json:"type"`
		ID       uuid.UUID `json:"id"`
		Revision *int64    `json:"revision,omitempty"`
	} `json:"aggregate"`
	Data json.RawMessage `json:"data"`
}

type budgetChangedData struct {
	LimitMicros     *int64 `json:"limit_micros"`
	AvailableMicros *int64 `json:"available_micros"`
	IsOverrun       *bool  `json:"is_overrun"`
}

type budgetAuditSummary struct {
	LimitMicros *int64 `json:"limit_micros"`
	Revision    *int64 `json:"revision"`
	IsOverrun   *bool  `json:"is_overrun"`
}

type budgetAuditData struct {
	Action string `json:"action"`
	Object struct {
		Type string    `json:"type"`
		ID   uuid.UUID `json:"id"`
	} `json:"object"`
	RequestID string             `json:"request_id"`
	Before    budgetAuditSummary `json:"before"`
	After     budgetAuditSummary `json:"after"`
}

func validBudgetChangedEvents(actor identityapp.Principal, before, after domain.Budget, entry domain.LedgerEntry, events []identityapp.OutboxEvent) bool {
	if len(events) != 2 || events[0].ID == uuid.Nil || events[1].ID == uuid.Nil ||
		events[0].ID == events[1].ID ||
		events[0].Topic != "lanverse.billing.budget_changed.v1" ||
		events[1].Topic != "lanverse.audit.recorded.v1" {
		return false
	}
	for _, event := range events {
		if event.PartitionKey != after.ProjectID.String() || len(event.Payload) == 0 || len(event.Payload) > 32*1024 {
			return false
		}
	}
	var changed, audited budgetEvent
	var changeData budgetChangedData
	var auditData budgetAuditData
	if !decodeBudgetJSON(events[0].Payload, &changed) ||
		!decodeBudgetJSON(changed.Data, &changeData) ||
		!decodeBudgetJSON(events[1].Payload, &audited) ||
		!decodeBudgetJSON(audited.Data, &auditData) {
		return false
	}
	available, err := after.AvailableMicros()
	if err != nil || changeData.LimitMicros == nil || changeData.AvailableMicros == nil ||
		changeData.IsOverrun == nil || *changeData.LimitMicros != after.LimitMicros ||
		*changeData.AvailableMicros != available || *changeData.IsOverrun != after.IsOverrun {
		return false
	}
	if changed.EventID != events[0].ID || changed.EventType != events[0].Topic ||
		changed.OccurredAt.IsZero() || !changed.OccurredAt.Equal(entry.CreateTime) ||
		changed.OrgID != actor.OrgID || changed.ProjectID != after.ProjectID ||
		changed.Actor.Kind != "user" || changed.Actor.ID != actor.ID ||
		changed.Aggregate.Type != "budget" || changed.Aggregate.ID != after.ID ||
		changed.Aggregate.Revision == nil || *changed.Aggregate.Revision != after.Revision {
		return false
	}
	if audited.EventID != events[1].ID || audited.EventType != events[1].Topic ||
		!audited.OccurredAt.Equal(changed.OccurredAt) ||
		audited.OrgID != actor.OrgID || audited.ProjectID != after.ProjectID ||
		audited.Actor.Kind != "user" || audited.Actor.ID != actor.ID ||
		audited.Aggregate.Type != "audit" || audited.Aggregate.ID != events[1].ID ||
		audited.Aggregate.Revision != nil || auditData.Action != "budget.changed" ||
		auditData.Object.Type != "budget" || auditData.Object.ID != after.ID {
		return false
	}
	requestID, err := uuid.Parse(auditData.RequestID)
	if err != nil || requestID == uuid.Nil || requestID.String() != auditData.RequestID {
		return false
	}
	return matchesBudgetAuditSummary(auditData.Before, before) &&
		matchesBudgetAuditSummary(auditData.After, after)
}

func matchesBudgetAuditSummary(summary budgetAuditSummary, budget domain.Budget) bool {
	return summary.LimitMicros != nil && summary.Revision != nil && summary.IsOverrun != nil &&
		*summary.LimitMicros == budget.LimitMicros && *summary.Revision == budget.Revision &&
		*summary.IsOverrun == budget.IsOverrun
}

func decodeBudgetJSON(payload []byte, target any) bool {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil && decoder.Decode(new(any)) == io.EOF
}
