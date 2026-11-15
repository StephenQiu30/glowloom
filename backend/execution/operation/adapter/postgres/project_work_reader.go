package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// ProjectWorkStore uses only Execution tables. The source must durably close its
// admission gate before inspecting; a technical receipt alone never closes work.
type ProjectWorkStore struct{ db *gorm.DB }

// NewProjectWorkStore binds responsibility reads to a restricted Execution pool.
func NewProjectWorkStore(db *gorm.DB) *ProjectWorkStore { return &ProjectWorkStore{db: db} }

// ReadProjectWork inspects all local responsibilities in one read-only snapshot.
func (s *ProjectWorkStore) ReadProjectWork(ctx context.Context, request application.ProjectWorkRequest) (application.ProjectWorkSnapshot, error) {
	out := application.ProjectWorkSnapshot{Request: request, BusinessState: application.ProjectWorkUnknown}
	if err := request.Validate(); err != nil {
		return out, err
	}
	if s == nil || s.db == nil || s.db.Statement == nil {
		return out, writefence.ErrUnavailable
	}
	if _, ok := s.db.Statement.ConnPool.(gorm.TxCommitter); ok {
		return out, writefence.ErrTransaction
	}
	owner, err := pgfence.New(s.db).Owner(ctx)
	if err != nil {
		return out, err
	}
	if owner != writefence.WriterExecution {
		return out, application.ErrFrozenInputOwner
	}
	// A read-only repeatable snapshot makes commands, deliveries, supplier facts
	// and funding agree without holding any SQL lock during a network call.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state, err := readProjectBusinessWork(ctx, tx, request)
		if err != nil {
			return err
		}
		out.BusinessState = state
		out.Commands = make([]application.ProjectCommandObservation, 0, len(request.Commands))
		for _, command := range request.Commands {
			observation := application.ProjectCommandObservation{Command: command, State: writefence.State("unknown")}
			if !knownProjectWorkAction(command.Action) {
				out.BusinessState = unresolvedProjectWork(out.BusinessState)
				out.Commands = append(out.Commands, observation)
				continue
			}
			record, err := pgfence.New(tx).Load(ctx, command.ID)
			if errors.Is(err, writefence.ErrNotFound) {
				out.BusinessState = unresolvedProjectWork(out.BusinessState)
				out.Commands = append(out.Commands, observation)
				continue
			}
			if err != nil {
				return err
			}
			if !application.SameFrozenInputCommand(record.Command, command) {
				return writefence.ErrConflict
			}
			observation.State = record.State
			if len(record.Receipt) != 0 {
				digest := sha256.Sum256(record.Receipt)
				observation.ReceiptSHA256 = hex.EncodeToString(digest[:])
			}
			switch record.State {
			case writefence.Admitted:
				out.BusinessState = application.ProjectWorkBusy
			case writefence.Frozen, writefence.Committed:
			default:
				out.BusinessState = unresolvedProjectWork(out.BusinessState)
			}
			out.Commands = append(out.Commands, observation)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, err
}

func knownProjectWorkAction(action string) bool {
	switch action {
	case "execution.frozen_input.accept", "creation.frozen_input.cancel", "execution.operation_output.adopt", "execution.provider_artifact.collect":
		return true
	default:
		return false
	}
}
func unresolvedProjectWork(state application.ProjectWorkState) application.ProjectWorkState {
	if state == application.ProjectWorkBusy {
		return state
	}
	return application.ProjectWorkUnknown
}

func readProjectBusinessWork(ctx context.Context, tx *gorm.DB, request application.ProjectWorkRequest) (application.ProjectWorkState, error) {
	// Deletion labels do not discharge physical or financial responsibility.
	var busy bool
	err := tx.Raw(`SELECT
	 EXISTS(SELECT 1 FROM operation.operation WHERE project_id=? AND status IN ('confirmed','submitting','submitted','succeeded','ingesting','unknown','reconciling','manual','cancelling'))
	 OR EXISTS(SELECT 1 FROM billing.reservation WHERE project_id=? AND (status='held' OR closed_at IS NULL))
	 OR EXISTS(SELECT 1 FROM billing.budget WHERE project_id=? AND reserved_micros>0)
	 OR EXISTS(SELECT 1 FROM operation.output_delivery d JOIN operation.operation o ON o.id=d.operation_id WHERE o.project_id=? AND d.state='prepared')
	 OR EXISTS(SELECT 1 FROM operation.provider_artifact_delivery d WHERE d.project_id=? AND d.state IN ('collecting','prepared'))
	 OR EXISTS(SELECT 1 FROM operation.prompt_submission s LEFT JOIN operation.operation o ON o.id=s.operation_id WHERE s.project_id=? AND (s.state='quoted' OR (s.state='accepted' AND (o.id IS NULL OR o.status NOT IN ('completed','failed','cancelled','expired')))))`, request.ProjectID, request.ProjectID, request.ProjectID, request.ProjectID, request.ProjectID, request.ProjectID).Scan(&busy).Error
	if err != nil {
		return application.ProjectWorkUnknown, err
	}
	state := application.ProjectWorkIdle
	if busy {
		state = application.ProjectWorkBusy
	}
	projectAuthority, _ := json.Marshal([]map[string]string{{"issuer": "creation", "kind": "project", "id": request.ProjectID.String()}})
	var commands struct{ Admitted, Unresolved bool }
	if err := tx.Raw(`SELECT
	 EXISTS(SELECT 1 FROM infra.writer_command c WHERE c.service_owner=infra.runtime_owner() AND c.authorities @> ?::jsonb AND c.state='admitted') AS admitted,
	 EXISTS(SELECT 1 FROM infra.writer_command c WHERE c.service_owner=infra.runtime_owner() AND c.authorities @> ?::jsonb AND (c.action NOT IN ('execution.frozen_input.accept','creation.frozen_input.cancel','execution.operation_output.adopt','execution.provider_artifact.collect') OR (c.action='execution.frozen_input.accept' AND c.state='committed' AND NOT EXISTS(SELECT 1 FROM operation.frozen_input_acceptance a WHERE a.command_id=c.id)))) AS unresolved`, string(projectAuthority), string(projectAuthority)).Scan(&commands).Error; err != nil {
		return state, err
	}
	if commands.Admitted {
		state = application.ProjectWorkBusy
	}
	if commands.Unresolved {
		state = unresolvedProjectWork(state)
	}
	var fence struct{ Epoch int64 }
	fenced := tx.Raw(`SELECT epoch FROM infra.writer_fence WHERE service_owner=infra.runtime_owner() AND issuer='creation' AND authority_kind='project' AND authority_id=?`, request.ProjectID).Scan(&fence)
	if fenced.Error != nil {
		return state, fenced.Error
	}
	if fenced.RowsAffected != 1 || fence.Epoch != request.ProjectEpoch {
		state = unresolvedProjectWork(state)
	}
	// Terminal labels do not establish a known charge. Reuse the same cost
	// evidence evaluator as the real finalizer, including original task coverage.
	var lastOperation uuid.UUID
	for {
		var operations []struct {
			ID            uuid.UUID
			Status        string
			ReusedFromID  *uuid.UUID
			SettledMicros *int64
		}
		if err := tx.Raw(`SELECT o.id,o.status,o.reused_from_id,o.settled_micros FROM operation.operation o WHERE o.project_id=? AND o.id>? AND EXISTS(SELECT 1 FROM operation.provider_call c WHERE c.operation_id=o.id) ORDER BY o.id LIMIT 128`, request.ProjectID, lastOperation).Scan(&operations).Error; err != nil {
			return state, err
		}
		if len(operations) == 0 {
			break
		}
		lastOperation = operations[len(operations)-1].ID
		for _, operation := range operations {
			_, err := readProviderCost(ctx, tx, operation.ID, providerCostOperation{ProjectID: request.ProjectID, Status: operation.Status, ReusedFromID: operation.ReusedFromID})
			if errors.Is(err, application.ErrProviderCostUnknown) || errors.Is(err, application.ErrManualResolutionUnverified) {
				state = unresolvedProjectWork(state)
				continue
			}
			if err != nil {
				return state, err
			}
			if operation.SettledMicros == nil {
				state = unresolvedProjectWork(state)
			}
		}
	}
	var orphan bool
	if err := tx.Raw(`SELECT
	 EXISTS(SELECT 1 FROM billing.ledger_entry l WHERE l.project_id=? AND l.entry_type='reserve' AND NOT EXISTS(SELECT 1 FROM billing.reservation r WHERE r.operation_id=l.operation_id AND r.project_id=l.project_id AND r.status IN ('settled','released') AND r.closed_at IS NOT NULL))
	 OR EXISTS(SELECT 1 FROM operation.provider_call c WHERE c.project_id=? AND NOT EXISTS(SELECT 1 FROM operation.operation o WHERE o.id=c.operation_id AND o.project_id=c.project_id))
	 OR EXISTS(SELECT 1 FROM billing.reservation r LEFT JOIN operation.operation o ON o.id=r.operation_id AND o.project_id=r.project_id WHERE r.project_id=? AND r.status IN ('settled','released') AND (o.id IS NULL OR (o.confirmed_at IS NOT NULL AND (o.settled_micros IS NULL OR NOT EXISTS(SELECT 1 FROM billing.ledger_entry l WHERE l.operation_id=o.id AND l.project_id=o.project_id AND l.entry_type='settle' AND l.amount_micros=o.settled_micros AND NOT l.is_delete)))))`, request.ProjectID, request.ProjectID, request.ProjectID).Scan(&orphan).Error; err != nil {
		return state, err
	}
	if orphan {
		state = unresolvedProjectWork(state)
	}
	var lastInput uuid.UUID
	for {
		var accepted []struct {
			HandoffID, CommandID, OperationID, OrgID, ScopeID uuid.UUID
			CommandSHA256, SnapshotSHA256                     string
		}
		if err := tx.Raw(`SELECT handoff_id,command_id,operation_id,command_sha256,snapshot_sha256,(snapshot->>'org_id')::uuid AS org_id,(snapshot->>'scope_id')::uuid AS scope_id FROM operation.frozen_input_acceptance WHERE snapshot->>'project_id'=? AND handoff_id>? ORDER BY handoff_id LIMIT 128`, request.ProjectID.String(), lastInput).Scan(&accepted).Error; err != nil {
			return state, err
		}
		if len(accepted) == 0 {
			break
		}
		lastInput = accepted[len(accepted)-1].HandoffID
		for _, input := range accepted {
			if input.OrgID != request.OrganizationID || input.ScopeID != request.ScopeID {
				return state, writefence.ErrConflict
			}
			var closed bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM operation.operation WHERE id=? AND project_id=? AND status IN ('completed','failed','cancelled','expired') AND (confirmed_at IS NULL OR settled_micros IS NOT NULL))`, input.OperationID, request.ProjectID).Scan(&closed).Error; err != nil {
				return state, err
			}
			if closed {
				continue
			}
			var cancel struct {
				CancelCommandID uuid.UUID
				Proof           json.RawMessage
			}
			result := tx.Raw(`SELECT cancel_command_id,proof FROM operation.frozen_input_cancellation WHERE handoff_id=? AND command_id=? AND snapshot_sha256=?`, input.HandoffID, input.CommandID, input.SnapshotSHA256).Scan(&cancel)
			if result.Error != nil {
				return state, result.Error
			}
			if result.RowsAffected != 1 {
				state = unresolvedProjectWork(state)
				continue
			}
			record, err := pgfence.New(tx).Load(ctx, cancel.CancelCommandID)
			if err != nil {
				return state, err
			}
			key := application.FrozenInputKey{HandoffID: input.HandoffID, CommandID: input.CommandID, CommandSHA256: input.CommandSHA256, SnapshotSHA256: input.SnapshotSHA256}
			proof, err := decodeFrozenCancelRecord(record, key)
			if err != nil || proof.Outcome != "cancelled_input" {
				return state, writefence.ErrConflict
			}
			stored, err := writefence.CanonicalReceipt(cancel.Proof)
			if err != nil || string(stored) != string(record.Receipt) {
				return state, writefence.ErrConflict
			}
		}
	}
	return state, nil
}

var _ application.ProjectWorkReader = (*ProjectWorkStore)(nil)
