package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

var (
	// ErrInvalidTransitionInput means a workflow state write is incomplete.
	ErrInvalidTransitionInput = application.ErrInvalidTransitionInput
	// ErrTransitionConflict means the persisted state differs from the expected one.
	ErrTransitionConflict = application.ErrTransitionConflict
	// ErrTerminalTransitionRequiresSettlement keeps terminal status and billing atomic.
	ErrTerminalTransitionRequiresSettlement = errors.New("terminal operation transition requires settlement")
)

// TransitionWorkflowOperation conditionally writes status, private history, and
// a project-scoped realtime event in one transaction. An exact replay is read
// only; terminal transitions must use the settlement transaction instead.
func (s *Store) TransitionWorkflowOperation(ctx context.Context, input application.TransitionInput) (domain.Status, error) {
	if s == nil || s.db == nil {
		return "", ErrUnavailable
	}
	if err := validateTransitionInput(input); err != nil {
		return "", err
	}
	var status domain.Status
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row workflowTransitionRow
		result := tx.Raw(`
			SELECT id, project_id, batch_id, target_type, target_id, status,
			       provider_request_key
			FROM operation.operation
			WHERE id = ?::uuid AND NOT is_delete
			FOR UPDATE
		`, input.OperationID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("lock workflow operation: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		current := domain.Status(row.Status)
		if current == input.To {
			if !matchesTransitionReplay(row, input) {
				return ErrTransitionConflict
			}
			if input.ProviderTaskID != nil {
				var recorded struct{ ProviderTaskID *string }
				result = tx.Raw(`
					SELECT detail ->> 'provider_task_id' AS provider_task_id
					FROM operation.operation_event
					WHERE operation_id = ?::uuid AND to_status = ?
					  AND detail ? 'provider_task_id' AND NOT is_delete
					ORDER BY create_time DESC, id DESC LIMIT 1
				`, input.OperationID.String(), string(input.To)).Scan(&recorded)
				if result.Error != nil {
					return fmt.Errorf("read replay provider task: %w", result.Error)
				}
				if result.RowsAffected != 1 || recorded.ProviderTaskID == nil || *recorded.ProviderTaskID != *input.ProviderTaskID {
					return ErrTransitionConflict
				}
			}
			status = current
			return nil
		}
		if !containsTransitionFrom(input.From, current) {
			return ErrTransitionConflict
		}
		if err := current.CanTransitionTo(input.To); err != nil {
			return err
		}
		if row.TargetType == nil || *row.TargetType == "" {
			return ErrInvalidTransitionInput
		}
		if input.To == domain.StatusSubmitting {
			if err := checkWorkflowInputsForSubmission(tx, input.OperationID, row.ProjectID); err != nil {
				return err
			}
		}
		requestKey := row.ProviderRequestKey
		if input.ProviderRequestKey != nil {
			if requestKey != nil && *requestKey != *input.ProviderRequestKey {
				return ErrTransitionConflict
			}
			requestKey = input.ProviderRequestKey
		}
		result = tx.Exec(`
			UPDATE operation.operation
			SET status = ?, provider_request_key = ?,
			    started_at = CASE WHEN ? = 'submitting' THEN COALESCE(started_at, now()) ELSE started_at END,
			    update_time = now()
			WHERE id = ?::uuid AND status = ? AND NOT is_delete
		`, string(input.To), requestKey, string(input.To), input.OperationID.String(), row.Status)
		if result.Error != nil {
			return fmt.Errorf("transition workflow operation: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrTransitionConflict
		}
		detail := map[string]any{}
		if input.ProviderTaskID != nil {
			detail["provider_task_id"] = *input.ProviderTaskID
		}
		if err := appendWorkflowTransition(tx, row, current, input, detail); err != nil {
			return err
		}
		status = input.To
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("transition workflow operation transaction: %w", err)
	}
	return status, nil
}

type workflowTransitionRow struct {
	ID                 uuid.UUID
	ProjectID          uuid.UUID
	BatchID            *uuid.UUID
	TargetType         *string
	TargetID           *uuid.UUID
	Status             string
	ProviderRequestKey *string
}

func validateTransitionInput(input application.TransitionInput) error {
	if input.OperationID == uuid.Nil || len(input.From) == 0 || len(input.From) > 8 ||
		input.To.IsTerminal() || len(input.Reason) > 128 ||
		(input.ProviderRequestKey != nil && (strings.TrimSpace(*input.ProviderRequestKey) == "" || len(*input.ProviderRequestKey) > 256)) ||
		(input.ProviderTaskID != nil && (strings.TrimSpace(*input.ProviderTaskID) == "" || len(*input.ProviderTaskID) > 256)) {
		if input.To.IsTerminal() {
			return ErrTerminalTransitionRequiresSettlement
		}
		return ErrInvalidTransitionInput
	}
	if input.To == domain.StatusSubmitting && input.ProviderRequestKey == nil {
		return ErrInvalidTransitionInput
	}
	if input.To == domain.StatusSubmitted && input.ProviderTaskID == nil {
		return ErrInvalidTransitionInput
	}
	manualRecovery := input.To == domain.StatusIngesting && len(input.From) == 1 && input.From[0] == domain.StatusManual
	if input.ProviderTaskID != nil && input.To != domain.StatusSubmitted && !manualRecovery {
		return ErrInvalidTransitionInput
	}
	if manualRecovery && input.ProviderTaskID == nil {
		return ErrInvalidTransitionInput
	}
	for _, from := range input.From {
		if from.IsTerminal() || from == input.To || from.CanTransitionTo(input.To) != nil {
			return ErrInvalidTransitionInput
		}
	}
	return nil
}

func matchesTransitionReplay(row workflowTransitionRow, input application.TransitionInput) bool {
	if input.ProviderRequestKey != nil && (row.ProviderRequestKey == nil || *row.ProviderRequestKey != *input.ProviderRequestKey) {
		return false
	}
	return true
}

func containsTransitionFrom(from []domain.Status, current domain.Status) bool {
	for _, candidate := range from {
		if candidate == current {
			return true
		}
	}
	return false
}

func appendWorkflowTransition(tx *gorm.DB, row workflowTransitionRow, from domain.Status, input application.TransitionInput, detail map[string]any) error {
	privateDetail, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("encode private transition detail: %w", err)
	}
	eventID := uuid.New()
	result := tx.Exec(`
		INSERT INTO operation.operation_event
		  (id, operation_id, from_status, to_status, reason, detail)
		VALUES (?::uuid, ?::uuid, ?, ?, ?, ?::jsonb)
	`, eventID.String(), row.ID.String(), string(from), string(input.To), input.Reason, string(privateDetail))
	if result.Error != nil {
		return fmt.Errorf("insert operation transition event: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("insert operation transition event: wrote %d rows", result.RowsAffected)
	}
	const topic = "lanverse.operation.status_changed.v1"
	data := struct {
		BatchID    *uuid.UUID `json:"batch_id,omitempty"`
		TargetType string     `json:"target_type"`
		TargetID   *uuid.UUID `json:"target_id,omitempty"`
		Status     string     `json:"status"`
	}{row.BatchID, *row.TargetType, row.TargetID, string(input.To)}
	payload, err := encodeWorkflowEvent(tx, row, eventID, topic, data)
	if err != nil {
		return fmt.Errorf("encode operation status event: %w", err)
	}
	result = tx.Exec(`
		INSERT INTO infra.outbox (id, topic, partition_key, payload)
		VALUES (?::uuid, ?, ?, ?::jsonb)
	`, eventID.String(), topic, row.ProjectID.String(), string(payload))
	if result.Error != nil {
		return fmt.Errorf("insert operation status outbox: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("insert operation status outbox: wrote %d rows", result.RowsAffected)
	}
	if input.To == domain.StatusManual {
		return appendWorkflowSpecialEvent(tx, row, "lanverse.operation.manual.v1", struct {
			Status string `json:"status"`
		}{Status: string(input.To)})
	}
	return nil
}

func appendWorkflowSpecialEvent(tx *gorm.DB, row workflowTransitionRow, topic string, data any) error {
	eventID := uuid.New()
	payload, err := encodeWorkflowEvent(tx, row, eventID, topic, data)
	if err != nil {
		return err
	}
	result := tx.Exec(`
		INSERT INTO infra.outbox (id, topic, partition_key, payload)
		VALUES (?::uuid, ?, ?, ?::jsonb)
	`, eventID.String(), topic, row.ProjectID.String(), string(payload))
	if result.Error != nil {
		return fmt.Errorf("insert operation event %s: %w", topic, result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("insert operation event %s: wrote %d rows", topic, result.RowsAffected)
	}
	return nil
}

func encodeWorkflowEvent(tx *gorm.DB, row workflowTransitionRow, eventID uuid.UUID, topic string, data any) ([]byte, error) {
	var project struct{ OrgID uuid.UUID }
	result := tx.Raw(`SELECT org_id FROM workspace.project WHERE id = ?::uuid`, row.ProjectID.String()).Scan(&project)
	if result.Error != nil {
		return nil, fmt.Errorf("read workflow event organization: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return nil, ErrNotFound
	}
	payload, err := json.Marshal(struct {
		EventID    uuid.UUID `json:"event_id"`
		EventType  string    `json:"event_type"`
		OccurredAt time.Time `json:"occurred_at"`
		OrgID      uuid.UUID `json:"org_id"`
		ProjectID  uuid.UUID `json:"project_id"`
		Actor      struct {
			Kind string     `json:"kind"`
			ID   *uuid.UUID `json:"id"`
		} `json:"actor"`
		Aggregate struct {
			Type string    `json:"type"`
			ID   uuid.UUID `json:"id"`
		} `json:"aggregate"`
		Data any `json:"data"`
	}{
		EventID: eventID, EventType: topic, OccurredAt: time.Now().UTC(),
		OrgID: project.OrgID, ProjectID: row.ProjectID,
		Actor: struct {
			Kind string     `json:"kind"`
			ID   *uuid.UUID `json:"id"`
		}{Kind: "system"},
		Aggregate: struct {
			Type string    `json:"type"`
			ID   uuid.UUID `json:"id"`
		}{Type: "operation", ID: row.ID},
		Data: data,
	})
	if err != nil {
		return nil, fmt.Errorf("encode workflow event %s: %w", topic, err)
	}
	return payload, nil
}
