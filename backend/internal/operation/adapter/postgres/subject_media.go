package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// ManagedSubjectMediaReference reauthorizes and signs one exact immutable input, never
// upgrading a bare asset or an ordinary input merely because its role matches.
func (s *Store) ManagedSubjectMediaReference(ctx context.Context, operationID, projectID, inputID uuid.UUID) (application.ProviderReference, error) {
	if s == nil || s.db == nil || operationID == uuid.Nil || projectID == uuid.Nil || inputID == uuid.Nil {
		return application.ProviderReference{}, application.ErrWorkflowInputNotReady
	}
	var ref application.ProviderReference
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireWorker(tx, operationID, true); err != nil {
			return err
		}
		var row operationInputRow
		// Inputs are immutable and the runtime role intentionally has no UPDATE
		// privilege on them. Lock the mutable parent, not the retained input row.
		r := tx.Raw(`SELECT i.id,i.operation_id,i.seq_no,i.role,i.ref_type,i.ref_id,i.ref_version,i.text_value,i.media_asset_id,i.mask_asset_id FROM operation.operation_input i JOIN operation.operation o ON o.id=i.operation_id WHERE i.id=? AND i.operation_id=? AND o.project_id=? AND NOT i.is_delete AND NOT o.is_delete FOR SHARE OF o`, inputID, operationID, projectID).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		input := row.domain()
		owner := s.subjects(tx)
		if r.RowsAffected != 1 || input.RefType != "subject_media" || input.Validate() != nil || owner == nil {
			return application.ErrWorkflowInputNotReady
		}
		if err := owner.VerifyInput(ctx, identity.Principal{}, operationID, projectID, input); err != nil {
			return err
		}
		var err error
		ref, err = owner.SignedInputURL(ctx, operationID, projectID, input, 15*time.Minute)
		return err
	})
	return ref, err
}

// SubjectExecutionActor exposes only the currently authorized durable confirmer
// and project scope to a composition-root subject consumer.
func (s *Store) SubjectExecutionActor(ctx context.Context, operationID, projectID uuid.UUID) (identity.Principal, error) {
	var actor identity.Principal
	if s == nil || s.db == nil {
		return actor, ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireWorker(tx, operationID, true); err != nil {
			return err
		}
		var row struct{ ActorID, OrgID, ScopeID uuid.UUID }
		r := tx.Raw(`SELECT o.confirmed_by AS actor_id,p.org_id,p.owner_scope_id AS scope_id FROM operation.operation o JOIN workspace.project p ON p.id=o.project_id WHERE o.id=? AND o.project_id=? AND NOT o.is_delete AND NOT p.is_delete FOR SHARE OF o,p`, operationID, projectID).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 || row.ActorID == uuid.Nil || row.ScopeID == uuid.Nil {
			return application.ErrExecutionAuthorityRevoked
		}
		actor = identity.Principal{ID: row.ActorID, OrgID: row.OrgID, ScopeID: row.ScopeID}
		return nil
	})
	return actor, err
}
