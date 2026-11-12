package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	billingdomain "github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
)

type callRow struct {
	ReceiptID, RootRunID, SubmissionID                             uuid.UUID
	TaskID, CallID, CheckpointID, ContractVersion, InputHash, Name string
	Kind                                                           domain.CallKind
	Status                                                         domain.CallStatus
	Attempt, MaxMicros, CostMicros                                 int64
	Result                                                         []byte
}

func (r callRow) receipt() domain.CallReceipt {
	return domain.CallReceipt{MaxMicros: r.MaxMicros, Kind: string(r.Kind), Name: r.Name, ReceiptID: r.ReceiptID, RootRunID: r.RootRunID, SubmissionID: r.SubmissionID, Call: domain.CallIdentity{TaskID: r.TaskID, CallID: r.CallID, CheckpointID: r.CheckpointID, Attempt: r.Attempt, ContractVersion: r.ContractVersion, InputHash: r.InputHash}, CallResult: domain.CallResult{Status: string(r.Status), Result: r.Result, CostMicros: r.CostMicros}}
}
func findCall(tx *gorm.DB, rid, segment uuid.UUID, c domain.CallIdentity) (callRow, bool, error) {
	var row callRow
	q := tx.Raw(`SELECT * FROM agent_business.call_receipt WHERE root_run_id=? AND submission_id=? AND task_id=? AND call_id=? AND checkpoint_id=? AND attempt=? AND contract_version=?`, rid, segment, c.TaskID, c.CallID, c.CheckpointID, c.Attempt, c.ContractVersion).Scan(&row)
	return row, q.RowsAffected == 1, q.Error
}
func validCall(in domain.CallInput, kind string) bool {
	limit := domain.MaxToolRequestBytes
	if kind == domain.CallKindModel {
		limit = domain.MaxModelRequestBytes
	}
	c := in.Call
	return in.MaxMicros >= 0 && c.TaskID != "" && len(c.TaskID) <= 256 && c.CallID != "" && len(c.CallID) <= 256 && c.CheckpointID != "" && len(c.CheckpointID) <= 256 && c.Attempt > 0 && c.Attempt <= 9007199254740991 && c.ContractVersion != "" && len(c.ContractVersion) <= 100 && len(in.Input) <= limit && json.Valid(in.Input) && c.InputHash == domain.Hash(in.Input)
}

// ExecuteCall commits a stable outbound intent before any external request. Unknown
// responses remain queryable and are never automatically sent again.
func (s *Store) ExecuteCall(ctx context.Context, kind, name string, in domain.CallInput, execute app.Executor) (domain.CallReceipt, error) {
	return s.executeCall(ctx, kind, name, in, nil, execute)
}

// ExecuteToolIntent commits the original external-write identity and business
// authority before any I/O. Replays never invoke admission or execution again.
func (s *Store) ExecuteToolIntent(ctx context.Context, name string, in domain.CallInput, admit func(*gorm.DB, domain.Scope, uuid.UUID) error, execute app.Executor) (domain.CallReceipt, error) {
	return s.executeCall(ctx, domain.CallKindTool, name, in, admit, execute)
}
func (s *Store) executeCall(ctx context.Context, kind, name string, in domain.CallInput, admit func(*gorm.DB, domain.Scope, uuid.UUID) error, execute app.Executor) (domain.CallReceipt, error) {
	if !validCall(in, kind) || execute == nil || name == "" || (kind != domain.CallKindModel && kind != domain.CallKindTool) {
		return domain.CallReceipt{}, domain.ErrInvalid
	}
	var result domain.CallReceipt
	var scope domain.Scope
	created := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, in.Grant.RootRunID)
		if err != nil {
			return err
		}
		if err = s.worker(tx, a); err != nil {
			return err
		}
		old, found, err := findCall(tx, a.RootRunID, in.Grant.SubmissionID, in.Call)
		if err != nil {
			return err
		}
		if found {
			if old.InputHash != in.Call.InputHash || old.Kind != domain.CallKind(kind) || old.Name != name || old.MaxMicros != in.MaxMicros {
				return domain.ErrConflict
			}
			result = old.receipt()
			return nil
		}
		g, err := s.validate(tx, a, in.Grant)
		if err != nil {
			return err
		}
		if kind == domain.CallKindModel && a.CostProjectionUnavailable {
			return domain.ErrUnavailable
		}
		reservation := in.MaxMicros
		if kind == domain.CallKindTool {
			reservation = 0
		}
		if reservation > spendableMicros(a) || kind == domain.CallKindModel && a.ReservedMicros > a.BudgetMicros {
			return domain.ErrBudget
		}
		// A run never starts a new paid request while an earlier model outcome is unknown.
		if kind == domain.CallKindModel {
			var n int64
			if err = tx.Raw(`SELECT count(*) FROM agent_business.call_receipt WHERE root_run_id=? AND kind=? AND status IN (?,?)`, a.RootRunID, domain.CallKindModel, domain.CallStatusInFlight, domain.CallStatusUnknown).Scan(&n).Error; err != nil {
				return err
			}
			if n > 0 {
				return domain.ErrBlocked
			}
		}
		result = domain.CallReceipt{MaxMicros: in.MaxMicros, Kind: kind, Name: name, ReceiptID: uuid.New(), RootRunID: a.RootRunID, SubmissionID: in.Grant.SubmissionID, Call: in.Call, CallResult: domain.CallResult{Status: domain.CallStatusInFlight, Result: json.RawMessage(`{}`)}}
		if err = tx.Exec(`INSERT INTO agent_business.call_receipt(receipt_id,root_run_id,submission_id,task_id,call_id,checkpoint_id,attempt,contract_version,input_hash,kind,name,max_micros,status) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, result.ReceiptID, a.RootRunID, in.Grant.SubmissionID, in.Call.TaskID, in.Call.CallID, in.Call.CheckpointID, in.Call.Attempt, in.Call.ContractVersion, in.Call.InputHash, kind, name, in.MaxMicros, domain.CallStatusInFlight).Error; err != nil {
			return err
		}
		if admit != nil {
			if err = admit(tx, g.Scope, result.ReceiptID); err != nil {
				return err
			}
		}
		if err = tx.Exec(`UPDATE agent_business.run_authorization SET reserved_micros=reserved_micros+? WHERE root_run_id=?`, reservation, a.RootRunID).Error; err != nil {
			return err
		}
		scope = g.Scope
		created = true
		return nil
	})
	if err != nil || !created {
		return result, err
	}
	out, executeErr := execute(ctx, scope, result.ReceiptID)
	if executeErr != nil {
		out = domain.CallResult{Status: domain.CallStatusUnknown, Result: json.RawMessage(`{"code":"business_result_unknown"}`)}
	}
	if out.Status != domain.CallStatusCompleted && out.Status != domain.CallStatusFailed && out.Status != domain.CallStatusUnknown {
		out = domain.CallResult{Status: domain.CallStatusUnknown, Result: json.RawMessage(`{"code":"invalid_business_receipt"}`)}
	}
	if !json.Valid(out.Result) {
		out.Result = json.RawMessage(`{}`)
	}
	if out.CostMicros < 0 || kind == domain.CallKindTool && out.CostMicros != 0 {
		out.Status = domain.CallStatusUnknown
		out.CostMicros = 0
		out.Result = json.RawMessage(`{"code":"cost_requires_reconciliation"}`)
	}
	// A client disconnect cannot erase an already returned provider receipt.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err = s.db.WithContext(saveCtx).Transaction(func(tx *gorm.DB) error {
		var completeErr error
		result, completeErr = CompleteCall(tx, result.ReceiptID, out)
		return completeErr
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

// ReadCall reads a saved call receipt using its stable invocation identity.
func (s *Store) ReadCall(ctx context.Context, rid, segment uuid.UUID, c domain.CallIdentity) (domain.CallReceipt, error) {
	var result domain.CallReceipt
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, rid)
		if err != nil {
			return err
		}
		if err = s.worker(tx, a); err != nil {
			return err
		}
		row, found, err := findCall(tx, rid, segment, c)
		if err != nil {
			return err
		}
		if !found {
			return domain.ErrNotFound
		}
		if row.InputHash != c.InputHash {
			return domain.ErrConflict
		}
		result = row.receipt()
		return nil
	})
	return result, err
}

// ExecuteAtomicTool keeps grant fencing, the formal business command and its
// receipt in one transaction. Adapters must bind their stores to tx.
func (s *Store) ExecuteAtomicTool(ctx context.Context, name string, in domain.CallInput, execute func(*gorm.DB, domain.Scope, uuid.UUID) (domain.CallResult, error)) (domain.CallReceipt, error) {
	if !validCall(in, domain.CallKindTool) || execute == nil {
		return domain.CallReceipt{}, domain.ErrInvalid
	}
	var result domain.CallReceipt
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := load(tx, in.Grant.RootRunID)
		if err != nil {
			return err
		}
		if err = s.worker(tx, a); err != nil {
			return err
		}
		old, found, err := findCall(tx, a.RootRunID, in.Grant.SubmissionID, in.Call)
		if err != nil {
			return err
		}
		if found {
			if old.InputHash != in.Call.InputHash || old.Kind != domain.CallKindTool || old.Name != name || old.MaxMicros != in.MaxMicros {
				return domain.ErrConflict
			}
			result = old.receipt()
			return nil
		}
		g, err := s.validate(tx, a, in.Grant)
		if err != nil {
			return err
		}
		id := uuid.New()
		out, err := execute(tx, g.Scope, id)
		if err != nil {
			return err
		}
		if out.Status != domain.CallStatusCompleted || out.CostMicros < 0 || out.CostMicros > in.MaxMicros || !json.Valid(out.Result) {
			return domain.ErrInvalid
		}
		// A formal media confirmation commits the original reservation upper
		// bound, while the tool itself has no actual charge. Never infer this
		// identity from a tool name or JSON output.
		commitment := out.CostMicros
		var media struct {
			ProjectID    uuid.UUID
			AmountMicros int64
			Status       billingdomain.ReservationStatus
		}
		linked := tx.Raw(`SELECT o.project_id,r.amount_micros,r.status FROM agent_business.operation_commitment link
          JOIN operation.operation o ON o.id=link.operation_id AND NOT o.is_delete
          JOIN billing.reservation r ON r.id=o.reservation_id AND r.operation_id=o.id AND r.project_id=o.project_id AND NOT r.is_delete
          WHERE link.receipt_id=?`, id).Scan(&media)
		if linked.Error != nil {
			return linked.Error
		}
		if linked.RowsAffected == 1 {
			if media.ProjectID != g.Scope.ProjectID || media.Status != billingdomain.ReservationHeld || out.CostMicros != 0 {
				return domain.ErrInvalid
			}
			commitment = media.AmountMicros
		}
		if commitment > 0 && a.CostProjectionUnavailable {
			return domain.ErrUnavailable
		}
		if commitment > in.MaxMicros || commitment > spendableMicros(a) {
			return domain.ErrBudget
		}
		result = domain.CallReceipt{MaxMicros: in.MaxMicros, Kind: domain.CallKindTool, Name: name, ReceiptID: id, RootRunID: a.RootRunID, SubmissionID: g.SubmissionID, Call: in.Call, CallResult: out}
		if err := tx.Exec(`UPDATE agent_business.run_authorization SET reserved_micros=reserved_micros+? WHERE root_run_id=?`, commitment, a.RootRunID).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO agent_business.call_receipt(receipt_id,root_run_id,submission_id,task_id,call_id,checkpoint_id,attempt,contract_version,input_hash,kind,name,max_micros,status,result,cost_micros) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?::jsonb,?)`, id, a.RootRunID, g.SubmissionID, in.Call.TaskID, in.Call.CallID, in.Call.CheckpointID, in.Call.Attempt, in.Call.ContractVersion, in.Call.InputHash, domain.CallKindTool, name, in.MaxMicros, domain.CallStatusCompleted, string(out.Result), out.CostMicros).Error
	})
	return result, err
}

// CompleteCall commits an authoritative business outcome with its accounting.
// Model settlement adapters call it inside their own transaction, closing the
// crash window between billing settlement and the Agent business receipt.
func CompleteCall(tx *gorm.DB, id uuid.UUID, out domain.CallResult) (domain.CallReceipt, error) {
	var reference struct{ RootRunID uuid.UUID }
	q := tx.Raw(`SELECT root_run_id FROM agent_business.call_receipt WHERE receipt_id=?`, id).Scan(&reference)
	if q.Error != nil {
		return domain.CallReceipt{}, q.Error
	}
	if q.RowsAffected != 1 {
		return domain.CallReceipt{}, domain.ErrNotFound
	}
	if _, err := load(tx, reference.RootRunID); err != nil {
		return domain.CallReceipt{}, err
	}
	var row callRow
	q = tx.Raw(`SELECT * FROM agent_business.call_receipt WHERE receipt_id=? FOR UPDATE`, id).Scan(&row)
	if q.Error != nil {
		return domain.CallReceipt{}, q.Error
	}
	if !json.Valid(out.Result) || out.CostMicros < 0 || (row.Kind == domain.CallKindTool && out.CostMicros != 0) || (out.Status != domain.CallStatusCompleted && out.Status != domain.CallStatusFailed && out.Status != domain.CallStatusUnknown) {
		return domain.CallReceipt{}, domain.ErrInvalid
	}
	if row.Status == domain.CallStatusCompleted || row.Status == domain.CallStatusFailed {
		if row.Status != domain.CallStatus(out.Status) || row.CostMicros != out.CostMicros || domain.Hash(row.Result) != domain.Hash(out.Result) {
			return domain.CallReceipt{}, domain.ErrConflict
		}
		return row.receipt(), nil
	}
	if err := tx.Exec(`UPDATE agent_business.call_receipt SET status=?,result=?::jsonb,cost_micros=? WHERE receipt_id=?`, out.Status, string(out.Result), out.CostMicros, id).Error; err != nil {
		return domain.CallReceipt{}, err
	}
	if out.Status == domain.CallStatusCompleted || out.Status == domain.CallStatusFailed {
		reservation := row.MaxMicros
		if row.Kind == domain.CallKindTool {
			if out.CostMicros != 0 {
				return domain.CallReceipt{}, domain.ErrInvalid
			}
			reservation = 0
		}
		if err := tx.Exec(`UPDATE agent_business.run_authorization SET reserved_micros=reserved_micros-?+? WHERE root_run_id=?`, reservation, out.CostMicros, row.RootRunID).Error; err != nil {
			return domain.CallReceipt{}, err
		}
	}
	row.Status = domain.CallStatus(out.Status)
	row.Result = out.Result
	row.CostMicros = out.CostMicros
	return row.receipt(), nil
}
