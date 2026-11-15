package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// FrozenInputStore is an independent Execution consumer. Source RPC always
// finishes before its own Tx starts; no Creation/Identity/Media SQL is available.
type FrozenInputStore struct {
	db     *gorm.DB
	source application.FrozenInputs
}

// NewFrozenInputStore injects Execution's restricted connection and authenticated source port.
func NewFrozenInputStore(db *gorm.DB, source application.FrozenInputs) *FrozenInputStore {
	return &FrozenInputStore{db, source}
}

var _ application.FrozenInputAcceptance = (*FrozenInputStore)(nil)

func (s *FrozenInputStore) ready(ctx context.Context) error {
	if s == nil || s.db == nil || s.db.Statement == nil || s.source == nil {
		return writefence.ErrUnavailable
	}
	if _, ok := s.db.Statement.ConnPool.(gorm.TxCommitter); ok {
		return writefence.ErrTransaction
	}
	owner, err := pgfence.New(s.db).Owner(ctx)
	if err != nil {
		return err
	}
	if owner != writefence.WriterExecution {
		return application.ErrFrozenInputOwner
	}
	return nil
}
func lockFrozenAcceptance(tx *gorm.DB, id uuid.UUID) error {
	return tx.Exec(`SELECT pg_advisory_xact_lock(69368,hashtext(?))`, id.String()).Error
}
func (s *FrozenInputStore) sourceInput(ctx context.Context, key application.FrozenInputKey) (application.FrozenInput, error) {
	in, err := s.source.ReadFrozenInput(ctx, key)
	if err != nil {
		return in, err
	}
	if in.Key != key {
		return in, application.ErrFrozenInputConflict
	}
	return in, in.Validate()
}
func readFrozenAcceptance(tx *gorm.DB, key application.FrozenInputKey) (application.FrozenInputReceipt, error) {
	var row struct {
		CommandID                     uuid.UUID
		CommandSHA256, SnapshotSHA256 string
		Receipt                       json.RawMessage
	}
	r := tx.Raw(`SELECT command_id,command_sha256,snapshot_sha256,receipt FROM operation.frozen_input_acceptance WHERE handoff_id=?`, key.HandoffID).Scan(&row)
	var receipt application.FrozenInputReceipt
	if r.Error != nil {
		return receipt, r.Error
	}
	if r.RowsAffected != 1 {
		return receipt, writefence.ErrNotFound
	}
	if row.CommandID != key.CommandID || row.CommandSHA256 != key.CommandSHA256 || row.SnapshotSHA256 != key.SnapshotSHA256 || json.Unmarshal(row.Receipt, &receipt) != nil || receipt.HandoffID != key.HandoffID || receipt.CommandID != key.CommandID || receipt.CommandSHA256 != key.CommandSHA256 || receipt.SnapshotSHA256 != key.SnapshotSHA256 {
		return receipt, application.ErrFrozenInputConflict
	}
	return receipt, nil
}
func checkFrozenCancellation(tx *gorm.DB, id uuid.UUID) error {
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM operation.frozen_input_cancellation WHERE handoff_id=?`, id).Scan(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return application.ErrFrozenInputCancelled
	}
	return nil
}

// AcceptFrozenInput binds immutable input only. A lost ACK replays the original
// local receipt without contacting Creation, even after source edits/revocation.
func (s *FrozenInputStore) AcceptFrozenInput(ctx context.Context, key application.FrozenInputKey) (application.FrozenInputReceipt, error) {
	var receipt application.FrozenInputReceipt
	if err := s.ready(ctx); err != nil {
		return receipt, err
	}
	// Check the durable local outcome first, under the same handoff serialization
	// used by cancellation. A cancellation marker always wins over ACK recovery.
	var recovered bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockFrozenAcceptance(tx, key.HandoffID); err != nil {
			return err
		}
		if err := checkFrozenCancellation(tx, key.HandoffID); err != nil {
			return err
		}
		var err error
		receipt, err = readFrozenAcceptance(tx, key)
		if errors.Is(err, writefence.ErrNotFound) {
			return nil
		}
		recovered = err == nil
		return err
	})
	if err != nil || recovered {
		return receipt, err
	}
	in, err := s.sourceInput(ctx, key)
	if err != nil {
		return receipt, err
	}
	if in.AdoptionBlocked {
		return receipt, application.ErrFrozenInputCancelled
	}
	// Source policy admission is already persistent. Do not bootstrap a target
	// command from a caller's epochs, a readable draft or a current Principal.
	original, err := pgfence.New(s.db).Load(ctx, key.CommandID)
	if err != nil {
		return receipt, err
	}
	if !application.SameFrozenInputCommand(original.Command, in.Command) {
		return receipt, application.ErrFrozenInputConflict
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockFrozenAcceptance(tx, key.HandoffID); err != nil {
			return err
		}
		if err := checkFrozenCancellation(tx, key.HandoffID); err != nil {
			return err
		}
		fences := pgfence.New(s.db)
		if err := fences.Guard(ctx, tx, in.Command); errors.Is(err, writefence.ErrCommitted) {
			var err error
			receipt, err = readFrozenAcceptance(tx, key)
			return err
		} else if err != nil {
			return err
		}
		receipt = application.NewFrozenInputReceipt(in)
		raw, _ := json.Marshal(receipt)
		r := tx.Exec(`INSERT INTO operation.frozen_input_acceptance(handoff_id,command_id,command_sha256,snapshot_sha256,snapshot,operation_id,submission_id,receipt) VALUES(?,?,?,?,?::jsonb,?,?,?::jsonb) ON CONFLICT DO NOTHING`, key.HandoffID, key.CommandID, key.CommandSHA256, key.SnapshotSHA256, string(in.Snapshot), receipt.OperationID, receipt.SubmissionID, string(raw))
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrFrozenInputConflict
		}
		if err := taskruntime.EnqueueReference(ctx, tx, "execution.frozen_input.accepted", key.HandoffID.String(), time.Now(), map[string]any{"handoff_id": key.HandoffID}); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, receipt.ReceiptID, "glowloom.execution.frozen_input.accepted", in.ProjectID.String(), string(raw)).Error; err != nil {
			return err
		}
		return fences.Complete(ctx, tx, in.Command, raw)
	})
	return receipt, err
}

// AcceptedFrozenInput acquires the same handoff gate as cancellation and retains
// it through the caller's real quote/confirmation transaction. It performs no RPC.
// Only this adapter-level capability accepts an existing Tx; the RPC-facing store
// still owns its independent transactions and rejects an outer Tx constructor.
func (s *FrozenInputStore) AcceptedFrozenInput(ctx context.Context, tx *gorm.DB, key application.FrozenInputKey) (json.RawMessage, application.FrozenInputReceipt, error) {
	var receipt application.FrozenInputReceipt
	if s == nil || s.db == nil || tx == nil || tx.Statement == nil {
		return nil, receipt, writefence.ErrTransaction
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, receipt, writefence.ErrTransaction
	}
	owner, err := pgfence.New(tx).Owner(ctx)
	if err != nil {
		return nil, receipt, err
	}
	if owner != writefence.WriterExecution {
		return nil, receipt, application.ErrFrozenInputOwner
	}
	tx = tx.WithContext(ctx)
	if err := lockFrozenAcceptance(tx, key.HandoffID); err != nil {
		return nil, receipt, err
	}
	if err := checkFrozenCancellation(tx, key.HandoffID); err != nil {
		return nil, receipt, err
	}
	receipt, err = readFrozenAcceptance(tx, key)
	if err != nil {
		return nil, receipt, err
	}
	var row struct{ Snapshot json.RawMessage }
	r := tx.Raw(`SELECT snapshot FROM operation.frozen_input_acceptance WHERE handoff_id=?`, key.HandoffID).Scan(&row)
	if r.Error != nil {
		return nil, receipt, r.Error
	}
	if r.RowsAffected != 1 {
		return nil, receipt, writefence.ErrNotFound
	}
	var compact bytes.Buffer
	if json.Compact(&compact, row.Snapshot) != nil {
		return nil, receipt, application.ErrFrozenInputConflict
	}
	canonical, err := writefence.CanonicalReceipt(compact.Bytes())
	if err != nil {
		return nil, receipt, application.ErrFrozenInputConflict
	}
	digest := sha256.Sum256(canonical)
	if hex.EncodeToString(digest[:]) != key.SnapshotSHA256 {
		return nil, receipt, application.ErrFrozenInputConflict
	}
	return canonical, receipt, nil
}

// CheckFrozenInputFunding is the required local cancellation gate for the real
// funding use case. Call it before touching an operation/reservation and keep the
// supplied Tx until its own Guard/Complete and reserve/Outbox commit. This check
// does not replace that use case's current policy or independently admitted command.
func (s *FrozenInputStore) CheckFrozenInputFunding(ctx context.Context, tx *gorm.DB, key application.FrozenInputKey, expectedRequestSHA256 string) error {
	if key.CommandSHA256 != expectedRequestSHA256 {
		return application.ErrFrozenInputConflict
	}
	if _, _, err := s.AcceptedFrozenInput(ctx, tx, key); err != nil {
		return err
	}
	record, err := pgfence.New(tx).Load(ctx, key.CommandID)
	if err != nil {
		return err
	}
	if record.State != writefence.Committed || record.Command.SHA256 != expectedRequestSHA256 {
		return application.ErrFrozenInputConflict
	}
	authorities := append([]writefence.Authority{}, record.Command.Authorities...)
	sort.Slice(authorities, func(i, j int) bool { return authorities[i].Key() < authorities[j].Key() })
	for _, a := range authorities {
		var row struct {
			Epoch   int64
			Blocked bool
		}
		r := tx.WithContext(ctx).Raw(`SELECT epoch,blocked FROM infra.writer_fence WHERE service_owner=infra.runtime_owner() AND issuer=? AND authority_kind=? AND authority_id=? FOR UPDATE`, a.Issuer, a.Kind, a.ID).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 || row.Blocked || row.Epoch != a.Epoch {
			return writefence.ErrFenced
		}
	}
	return nil
}

// This ledger has no invented reserved flag. Once real quote/confirmation adopts
// its deterministic identity, cancellation inspects Execution's actual local money
// and operation records while holding the same gate used by funding.
func checkFrozenInputReservation(tx *gorm.DB, handoff uuid.UUID) error {
	operation := uuid.NewSHA1(handoff, []byte("execution.operation"))
	var row struct {
		ReservationID *uuid.UUID
		ConfirmedAt   *time.Time
		Status        string
	}
	r := tx.Raw(`SELECT reservation_id,confirmed_at,status FROM operation.operation WHERE id=? FOR UPDATE`, operation).Scan(&row)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected > 0 && (row.ReservationID != nil || row.ConfirmedAt != nil || row.Status != "draft" && row.Status != "quoted" && row.Status != "expired" && row.Status != "cancelled") {
		return application.ErrSubmissionAccepted
	}
	var reservations []uuid.UUID
	if err := tx.Raw(`SELECT id FROM billing.reservation WHERE operation_id=? FOR SHARE`, operation).Scan(&reservations).Error; err != nil {
		return err
	}
	if len(reservations) > 0 {
		return application.ErrSubmissionAccepted
	}
	var accepted []uuid.UUID
	if err := tx.Raw(`SELECT id FROM operation.prompt_submission WHERE id=? AND state='accepted' FOR SHARE`, uuid.NewSHA1(handoff, []byte("execution.submission"))).Scan(&accepted).Error; err != nil {
		return err
	}
	if len(accepted) > 0 {
		return application.ErrSubmissionAccepted
	}
	return nil
}

func decodeFrozenCancelRecord(record writefence.Record, key application.FrozenInputKey) (application.FrozenInputCancelProof, error) {
	var proof application.FrozenInputCancelProof
	if record.State != writefence.Committed || json.Unmarshal(record.Receipt, &proof) != nil || proof.Key != key || proof.CancelCommandID != record.Command.ID || proof.Original.Command.ID != key.CommandID || proof.Original.Command.SHA256 != key.CommandSHA256 {
		return proof, application.ErrFrozenInputConflict
	}
	if proof.Original.State == writefence.Frozen && proof.Outcome == "not_received_frozen" {
		if string(proof.Original.Receipt) != "null" && len(proof.Original.Receipt) != 0 {
			return proof, application.ErrFrozenInputConflict
		}
		proof.Original.Receipt = nil
	} else if proof.Original.State != writefence.Committed || proof.Outcome != "cancelled_input" {
		return proof, application.ErrFrozenInputConflict
	}
	return proof, nil
}

// CancelFrozenInput closes future adoption atomically, including an already
// received snapshot. It does not cancel an operation or reservation. The new
// ledger is intentionally not wired into the public quote/confirm money path.
func (s *FrozenInputStore) CancelFrozenInput(ctx context.Context, key application.FrozenInputKey, command writefence.Command) (application.FrozenInputCancelProof, error) {
	var proof application.FrozenInputCancelProof
	if err := s.ready(ctx); err != nil {
		return proof, err
	}
	if command.Action != "creation.frozen_input.cancel" || command.ID == key.CommandID {
		return proof, application.ErrFrozenInputInvalid
	}
	// Exact local proof recovery must survive a source-side ACK loss or outage.
	var recovered bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockFrozenAcceptance(tx, key.HandoffID); err != nil {
			return err
		}
		var row struct {
			CancelCommandID uuid.UUID
			Proof           json.RawMessage
		}
		r := tx.Raw(`SELECT cancel_command_id,proof FROM operation.frozen_input_cancellation WHERE handoff_id=?`, key.HandoffID).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return nil
		}
		if row.CancelCommandID != command.ID || json.Unmarshal(row.Proof, &proof) != nil || proof.Key != key {
			return application.ErrFrozenInputConflict
		}
		record, err := pgfence.New(tx).Load(ctx, command.ID)
		if err != nil || record.State != writefence.Committed || !application.SameFrozenInputCommand(record.Command, command) {
			return application.ErrFrozenInputConflict
		}
		proof, err = decodeFrozenCancelRecord(record, key)
		if err != nil {
			return err
		}
		recovered = true
		return nil
	})
	if err != nil || recovered {
		return proof, err
	}
	in, err := s.sourceInput(ctx, key)
	if err != nil {
		return proof, err
	}
	if !in.AdoptionBlocked || in.CancelCommand == nil || !application.SameFrozenInputCommand(*in.CancelCommand, command) {
		return proof, application.ErrFrozenInputConflict
	}
	// The source's durable block authenticates this exact closure. The target's
	// independent cancellation command still must have been admitted beforehand.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockFrozenAcceptance(tx, key.HandoffID); err != nil {
			return err
		}
		fences := pgfence.New(s.db)
		if err := fences.Guard(ctx, tx, command); errors.Is(err, writefence.ErrCommitted) {
			record, err := pgfence.New(tx).Load(ctx, command.ID)
			if err != nil {
				return err
			}
			proof, err = decodeFrozenCancelRecord(record, key)
			return err
		} else if err != nil {
			return err
		}
		if err := checkFrozenInputReservation(tx, key.HandoffID); err != nil {
			return err
		}
		binding, err := in.Command.AuthorityDigest()
		if err != nil {
			return err
		}
		authorities, _ := json.Marshal(in.Command.Authorities)
		// A durable tombstone for an authenticated, already-blocked source command
		// prevents a delayed original delivery even if target admission never arrived.
		if err := tx.Exec(`INSERT INTO infra.writer_command(id,request_sha256,action,authority_set_sha256,authorities,state) VALUES(?,?,?,?,?::jsonb,'frozen') ON CONFLICT(service_owner,id) DO NOTHING`, in.Command.ID, in.Command.SHA256, in.Command.Action, binding, string(authorities)).Error; err != nil {
			return err
		}
		original, err := pgfence.New(tx).Load(ctx, in.Command.ID)
		if err != nil {
			return err
		}
		if !application.SameFrozenInputCommand(original.Command, in.Command) {
			return application.ErrFrozenInputConflict
		}
		original, err = pgfence.New(tx).Freeze(ctx, in.Command.ID)
		if err != nil {
			return err
		}
		proof = application.FrozenInputCancelProof{Key: key, CancelCommandID: command.ID, Original: original}
		switch original.State {
		case writefence.Frozen:
			proof.Outcome = "not_received_frozen"
		case writefence.Committed:
			saved, err := readFrozenAcceptance(tx, key)
			if err != nil || saved != application.NewFrozenInputReceipt(in) {
				return application.ErrFrozenInputConflict
			}
			proof.Outcome = "cancelled_input"
		default:
			return application.ErrFrozenInputConflict
		}
		raw, _ := json.Marshal(proof)
		r := tx.Exec(`INSERT INTO operation.frozen_input_cancellation(handoff_id,cancel_command_id,command_id,snapshot_sha256,proof) VALUES(?,?,?,?,?::jsonb) ON CONFLICT DO NOTHING`, key.HandoffID, command.ID, key.CommandID, key.SnapshotSHA256, string(raw))
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrFrozenInputConflict
		}
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, uuid.NewSHA1(key.HandoffID, []byte("execution.frozen_input.cancelled")), "glowloom.execution.frozen_input.cancelled", in.ProjectID.String(), string(raw)).Error; err != nil {
			return err
		}
		return fences.Complete(ctx, tx, command, raw)
	})
	return proof, err
}
