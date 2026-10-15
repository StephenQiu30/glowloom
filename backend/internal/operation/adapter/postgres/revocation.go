package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// PrepareRevocationInTransaction exposes only locked financial facts after proving
// authority is currently unavailable. It never grants content access.
func (s *Store) PrepareRevocationInTransaction(ctx context.Context, tx *gorm.DB, id uuid.UUID) (application.SettlementFacts, error) {
	if s == nil || s.db == nil || tx == nil {
		return application.SettlementFacts{}, ErrUnavailable
	}
	var row struct{ Status domain.Status }
	r := tx.WithContext(ctx).Raw(`SELECT status FROM operation.operation WHERE id=? AND NOT is_delete FOR UPDATE`, id).Scan(&row)
	if r.Error != nil {
		return application.SettlementFacts{}, r.Error
	}
	if r.RowsAffected != 1 {
		return application.SettlementFacts{}, ErrNotFound
	}
	if !row.Status.IsTerminal() {
		if err := s.requireRevoked(tx.WithContext(ctx), id); err != nil {
			return application.SettlementFacts{}, err
		}
	}
	return s.ReadSettlementFactsInTransaction(ctx, tx, id)
}
func (s *Store) requireRevoked(tx *gorm.DB, id uuid.UUID) error {
	err := s.requireWorker(tx, id, true)
	if errors.Is(err, application.ErrExecutionAuthorityRevoked) {
		return nil
	}
	if err == nil {
		return application.ErrTransitionConflict
	}
	return err
}

// RecordRevocationUnknownInTransaction preserves unresolved supplier facts and
// reservation rather than fabricating a zero-cost terminal outcome.
func (s *Store) RecordRevocationUnknownInTransaction(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	facts, err := s.PrepareRevocationInTransaction(ctx, tx, id)
	if err != nil {
		return err
	}
	if facts.Status.IsTerminal() || facts.Status == domain.StatusUnknown || facts.Status == domain.StatusManual {
		return nil
	}
	switch facts.Status {
	case domain.StatusConfirmed, domain.StatusSubmitting, domain.StatusSubmitted, domain.StatusSucceeded, domain.StatusIngesting, domain.StatusReconciling, domain.StatusCancelling:
	default:
		return ErrTransitionConflict
	}
	var row workflowTransitionRow
	r := tx.WithContext(ctx).Raw(`SELECT id,project_id,batch_id,target_type,target_id,status,provider_request_key FROM operation.operation WHERE id=? AND NOT is_delete FOR UPDATE`, id).Scan(&row)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrNotFound
	}
	if err := tx.Exec(`UPDATE operation.operation SET status='unknown',update_time=now() WHERE id=?`, id).Error; err != nil {
		return err
	}
	return appendWorkflowTransition(tx, row, facts.Status, application.TransitionInput{OperationID: id, From: []domain.Status{facts.Status}, To: domain.StatusUnknown, Reason: "execution_authority_revoked"}, map[string]any{"provider_cost": "unresolved"})
}

func revocationFinalization(input application.FinalizeInput, from domain.Status) bool {
	if input.To != domain.StatusFailed || input.FailureCode != "execution_authority_revoked" {
		return false
	}
	switch from {
	case domain.StatusSucceeded, domain.StatusUnknown, domain.StatusCancelling:
		return true
	}
	return false
}
