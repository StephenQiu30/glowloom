package pgsource

import (
	"context"

	"gorm.io/gorm"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

type closureAuthorityHead struct {
	Epoch             int64
	CoordinationID    uuid.UUID
	Blocked, Complete bool
}

func readClosureAuthority(tx *gorm.DB, state source.ProjectClosureState) (closureAuthorityHead, error) {
	var head closureAuthorityHead
	read := tx.Raw(`SELECT epoch,coordination_id,blocked,complete FROM infra.authority_head WHERE service_owner='creation' AND issuer='creation' AND authority_kind='project' AND authority_id=? FOR UPDATE`, state.Spec.ProjectID).Scan(&head)
	if read.Error != nil {
		return head, read.Error
	}
	if read.RowsAffected != 1 {
		return head, writefence.ErrFenced
	}
	return head, nil
}

// BeginProjectClosureAbort freezes only the rejected close control. It does not
// cancel another business responsibility or reopen an unknown higher authority.
func (s *Store) BeginProjectClosureAbort(ctx context.Context, state source.ProjectClosureState) (writefence.Control, bool, error) {
	var control writefence.Control
	reopen := false
	err := s.projectTransaction(ctx, func(tx *gorm.DB) error {
		current, err := readClosure(tx, state.Spec.ID, true)
		if err != nil {
			return err
		}
		if current.Cycle != state.Cycle {
			return taskruntime.ErrLeaseLost
		}
		if err := fenceClosureJob(ctx, tx, current, currentClosureJob(ctx)); err != nil {
			return err
		}
		if current.Aborted {
			return nil
		}
		if current.Result != nil {
			return writefence.ErrCommitted
		}
		if err := checkClosureGate(tx, current); err != nil {
			return err
		}
		if err := tx.Exec(`SELECT id FROM workspace.project WHERE id=? FOR UPDATE`, current.Spec.ProjectID).Error; err != nil {
			return err
		}
		head, err := readClosureAuthority(tx, current)
		if err != nil {
			return err
		}
		closed, opened := current.Spec.Control(), current.Spec.ReopenControl()
		switch {
		case head.Epoch == current.Spec.ProjectAuthority.Epoch && head.Complete:
		case head.Epoch == closed.Epoch && head.CoordinationID == closed.CoordinationID && head.Blocked && head.Complete:
			reopen = true
			control = opened
			coordination, err := pgfence.NewAuthorityStore(tx).BeginTx(ctx, tx, closed.Authority, opened.CoordinationID, false)
			if err != nil {
				return err
			}
			if coordination.Control != opened {
				return writefence.ErrConflict
			}
		case head.Epoch == opened.Epoch && head.CoordinationID == opened.CoordinationID && (!head.Complete || head.Blocked==opened.Blocked):
			coordination, err := pgfence.NewAuthorityStore(tx).LoadCoordination(ctx, opened.CoordinationID)
			if err != nil {
				return err
			}
			if coordination.Control != opened {
				return writefence.ErrConflict
			}
			reopen = true
			control = opened
		default:
			return writefence.ErrFenced
		}
		record, err := pgfence.New(tx).Load(ctx, current.Spec.Command.ID)
		if err != nil {
			return err
		}
		expected, _ := current.Spec.Command.AuthorityDigest()
		actual, err := record.Command.AuthorityDigest()
		if err != nil || expected != actual || record.Command.SHA256 != current.Spec.Command.SHA256 || record.Command.Action != current.Spec.Command.Action {
			return writefence.ErrConflict
		}
		if record.State == writefence.Committed {
			return writefence.ErrCommitted
		}
		frozen, err := pgfence.New(tx).Freeze(ctx, current.Spec.Command.ID)
		if err != nil {
			return err
		}
		if frozen.State != writefence.Frozen {
			return writefence.ErrConflict
		}
		return tx.Exec(`UPDATE workspace.project_closure SET phase='aborting' WHERE id=? AND cycle=? AND phase IN ('draining','closing','aborting')`, current.Spec.ID, current.Cycle).Error
	})
	return control, reopen, err
}

// CompleteProjectClosureAbort releases admission only after the original reopen
// authority is complete at every writer, or before project authority ever moved.
func (s *Store) CompleteProjectClosureAbort(ctx context.Context, state source.ProjectClosureState, control writefence.Control) error {
	return s.projectTransaction(ctx, func(tx *gorm.DB) error {
		current, err := readClosure(tx, state.Spec.ID, true)
		if err != nil {
			return err
		}
		if current.Cycle != state.Cycle {
			return taskruntime.ErrLeaseLost
		}
		if err := fenceClosureJob(ctx, tx, current, currentClosureJob(ctx)); err != nil {
			return err
		}
		if current.Aborted {
			return nil
		}
		if !current.Aborting {
			return writefence.ErrConflict
		}
		if err := checkClosureGate(tx, current); err != nil {
			return err
		}
		head, err := readClosureAuthority(tx, current)
		if err != nil {
			return err
		}
		if control.CoordinationID == uuid.Nil {
			if head.Epoch != current.Spec.ProjectAuthority.Epoch || !head.Complete {
				return writefence.ErrFenced
			}
		} else {
			expected := current.Spec.ReopenControl()
			if control != expected || head.Epoch != expected.Epoch || head.CoordinationID != expected.CoordinationID || head.Blocked!=expected.Blocked || !head.Complete {
				return writefence.ErrFenced
			}
		}
		record, err := pgfence.New(tx).Load(ctx, current.Spec.Command.ID)
		if err != nil || record.State != writefence.Frozen {
			if err != nil {
				return err
			}
			return writefence.ErrConflict
		}
		if err := tx.Exec(`UPDATE workspace.project_closure SET phase='aborted',completed_at=statement_timestamp() WHERE id=? AND cycle=? AND phase='aborting'`, current.Spec.ID, current.Cycle).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE workspace.project_admission_gate SET blocked=false,updated_at=statement_timestamp() WHERE project_id=? AND close_id=? AND cycle=?`, current.Spec.ProjectID, current.Spec.ID, current.Cycle).Error
	})
}
