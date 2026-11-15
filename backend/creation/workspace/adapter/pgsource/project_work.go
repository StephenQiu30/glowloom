package pgsource

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

var _ source.ProjectWorkReader = (*Store)(nil)

// ReadProjectWork inspects only Creation-owned durable work and original local
// writer records. It never admits/freezes work or queries a remote owner.
func (s *Store) ReadProjectWork(ctx context.Context, request source.ProjectWorkRequest) (source.ProjectWorkSnapshot, error) {
	if request.Validate() != nil {
		return source.ProjectWorkSnapshot{}, writefence.ErrInvalid
	}
	if s == nil || s.db == nil || s.db.Statement == nil {
		return source.ProjectWorkSnapshot{}, writefence.ErrUnavailable
	}
	if _, outer := s.db.Statement.ConnPool.(gorm.TxCommitter); outer {
		return source.ProjectWorkSnapshot{}, writefence.ErrTransaction
	}
	response := source.ProjectWorkSnapshot{Request: request, BusinessState: source.ProjectWorkIdle}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		owner, err := pgfence.New(tx).Owner(ctx)
		if err != nil {
			return err
		}
		if owner != writefence.WriterCreation {
			return writefence.ErrInvalid
		}
		state, err := readClosure(tx, request.CloseID, false)
		if err != nil {
			return err
		}
		spec := state.Spec
		if !state.GateClosed || spec.ProjectID != request.ProjectID || spec.OrganizationID != request.OrganizationID || spec.ScopeID != request.ScopeID || spec.ProjectAuthority.Epoch != request.ProjectEpoch || state.ManifestFor(writefence.WriterCreation) != request.ManifestSHA256 {
			return writefence.ErrConflict
		}
		var busy bool
		if err := tx.Raw(`SELECT
		 EXISTS(SELECT 1 FROM workspace.project_copy_job WHERE org_id=? AND (source_project_id=? OR target_project_id=?) AND status NOT IN ('succeeded','cancelled')) OR
		 EXISTS(SELECT 1 FROM script.command c JOIN script.command_state st ON st.actor_id=c.actor_id AND st.request_id=c.request_id WHERE c.org_id=? AND c.project_id=? AND st.status='pending') OR
		 EXISTS(SELECT 1 FROM script.import_job j JOIN script.import_state st ON st.job_id=j.id WHERE j.org_id=? AND j.project_id=? AND (st.status IN ('queued','running','cancel_requested') OR st.needs_reconciliation OR st.io_owner_id IS NOT NULL OR st.io_state='unknown')) OR
		 EXISTS(SELECT 1 FROM infra.writer_command WHERE service_owner='creation' AND authorities @> ?::jsonb AND state='admitted')`, request.OrganizationID, request.ProjectID, request.ProjectID, request.OrganizationID, request.ProjectID, request.OrganizationID, request.ProjectID, projectLedgerSelector(request.ProjectID)).Scan(&busy).Error; err != nil {
			return err
		}
		if busy {
			response.BusinessState = source.ProjectWorkBusy
		}
		writer := pgfence.New(tx)
		for _, command := range request.Commands {
			if err := exactCreationAdmission(tx, command, writefence.WriterCreation); err != nil {
				return err
			}
			observation := source.ProjectCommandObservation{Command: command, State: writefence.State("unknown")}
			record, err := writer.Load(ctx, command.ID)
			if errors.Is(err, writefence.ErrNotFound) {
				response.Commands = append(response.Commands, observation)
				continue
			}
			if err != nil {
				return err
			}
			want, _ := command.AuthorityDigest()
			got, err := record.Command.AuthorityDigest()
			if err != nil || want != got || record.Command.SHA256 != command.SHA256 || record.Command.Action != command.Action {
				return writefence.ErrConflict
			}
			observation.State = record.State
			if record.State == writefence.Committed {
				canonical, err := writefence.CanonicalReceipt(record.Receipt)
				if err != nil {
					return err
				}
				sum := sha256.Sum256(canonical)
				observation.ReceiptSHA256 = hex.EncodeToString(sum[:])
			}
			response.Commands = append(response.Commands, observation)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return response, err
}
